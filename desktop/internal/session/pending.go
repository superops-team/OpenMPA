package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"os"
	"path/filepath"
	"sync"
)

var ErrPendingExists = errors.New("operation already saved")

type Pending struct {
	AttachmentRefs   []string        `json:"attachmentRefs"`
	AssistantID      string          `json:"assistantId,omitempty"`
	AssistantProfile *dto.BotProfile `json:"assistantProfile,omitempty"`
	Scope            string          `json:"scope"`
	SessionID        string          `json:"sessionId"`
	OperationID      string          `json:"operationId"`
	IdempotencyKey   string          `json:"idempotencyKey"`
	Content          string          `json:"content"`
	ConfigVersion    string          `json:"configVersion"`
	State            string          `json:"state"`
	InvocationID     string          `json:"invocationId,omitempty"`
	DurableBot       bool            `json:"durableBot"`
	TraceEnabled     bool            `json:"traceEnabled"`
	TerminalVerified bool            `json:"terminalVerified"`
}
type PendingStore struct {
	mu       sync.Mutex
	path     func() (string, error)
	resolved string
}

func NewPendingStore(path func() (string, error)) *PendingStore { return &PendingStore{path: path} }
func (s *PendingStore) directory() (string, error) {
	if s.resolved == "" {
		p, e := s.path()
		if e != nil {
			return "", e
		}
		s.resolved = p
	}
	if e := os.MkdirAll(s.resolved, 0700); e != nil {
		return "", e
	}
	return s.resolved, nil
}
func pendingFile(scope, op string) string {
	h := sha256.Sum256([]byte(scope + "\x00" + op))
	return hex.EncodeToString(h[:]) + ".json"
}
func equalBotProfile(a, b *dto.BotProfile) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && string(left) == string(right)
}
func (s *PendingStore) Put(p Pending) error {
	if !validAttachmentRefs(p.AttachmentRefs) {
		return fmt.Errorf("invalid attachment refs")
	}
	p.AttachmentRefs = cloneAttachmentRefs(p.AttachmentRefs)
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, e := s.directory()
	if e != nil {
		return e
	}
	name := filepath.Join(dir, pendingFile(p.Scope, p.OperationID))
	if previous, e := os.ReadFile(name); e == nil {
		var old Pending
		if json.Unmarshal(previous, &old) != nil {
			return fmt.Errorf("invalid pending store")
		}
		if old.SessionID != p.SessionID || old.IdempotencyKey != p.IdempotencyKey || old.Content != p.Content || old.ConfigVersion != p.ConfigVersion || old.AssistantID != p.AssistantID || !equalBotProfile(old.AssistantProfile, p.AssistantProfile) || !equalAttachmentRefs(old.AttachmentRefs, p.AttachmentRefs) {
			return fmt.Errorf("pending mismatch")
		}
		return ErrPendingExists
	}
	p.State = "outcomeUnknown"
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(dir, ".pending-")
	if e != nil {
		return e
	}
	defer os.Remove(tmp.Name())
	if e = tmp.Chmod(0600); e == nil {
		_, e = tmp.Write(b)
	}
	if e == nil {
		e = tmp.Sync()
	}
	closeErr := tmp.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	return os.Rename(tmp.Name(), name)
}
func (s *PendingStore) Delete(scope, op string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, e := s.directory()
	if e != nil {
		return e
	}
	e = os.Remove(filepath.Join(dir, pendingFile(scope, op)))
	if os.IsNotExist(e) {
		return nil
	}
	return e
}
func (s *PendingStore) List(scope string) ([]Pending, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, e := s.directory()
	if e != nil {
		return nil, e
	}
	files, e := os.ReadDir(dir)
	if e != nil {
		return nil, e
	}
	out := []Pending{}
	for _, f := range files {
		if filepath.Ext(f.Name()) != ".json" {
			continue
		}
		b, e := os.ReadFile(filepath.Join(dir, f.Name()))
		if e != nil {
			return nil, e
		}
		var p Pending
		if json.Unmarshal(b, &p) != nil {
			return nil, fmt.Errorf("invalid pending store")
		}
		if !validAttachmentRefs(p.AttachmentRefs) {
			return nil, fmt.Errorf("invalid attachment refs")
		}
		p.AttachmentRefs = cloneAttachmentRefs(p.AttachmentRefs)
		if p.Scope == scope {
			out = append(out, p)
		}
	}
	return out, nil
}
func (p Pending) Summary() dto.PendingSummary {
	return dto.PendingSummary{AttachmentRefs: cloneAttachmentRefs(p.AttachmentRefs), SessionID: p.SessionID, OperationID: p.OperationID, IdempotencyKey: p.IdempotencyKey, Content: p.Content, ExecutionConfigVersion: p.ConfigVersion, State: p.State, InvocationID: p.InvocationID, RecoveryDisposition: p.disposition(), TerminalVerified: p.TerminalVerified}
}

func (p Pending) disposition() string {
	if p.InvocationID != "" {
		return "subscribeAccepted"
	}
	if p.DurableBot {
		return "retryDurable"
	}
	return "verifyOnly"
}
func (s *PendingStore) Update(scope, operation string, change func(*Pending) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, e := s.directory()
	if e != nil {
		return e
	}
	name := filepath.Join(dir, pendingFile(scope, operation))
	data, e := os.ReadFile(name)
	if e != nil {
		return e
	}
	var p Pending
	if json.Unmarshal(data, &p) != nil || p.Scope != scope || p.OperationID != operation {
		return fmt.Errorf("invalid pending store")
	}
	original := p
	original.AttachmentRefs = cloneAttachmentRefs(p.AttachmentRefs)
	if e = change(&p); e != nil {
		return e
	}
	if p.Scope != original.Scope || p.SessionID != original.SessionID || p.OperationID != original.OperationID || p.IdempotencyKey != original.IdempotencyKey || p.Content != original.Content || p.ConfigVersion != original.ConfigVersion || p.AssistantID != original.AssistantID || !equalBotProfile(p.AssistantProfile, original.AssistantProfile) || !equalAttachmentRefs(p.AttachmentRefs, original.AttachmentRefs) {
		return fmt.Errorf("pending immutable payload changed")
	}
	p.AttachmentRefs = cloneAttachmentRefs(p.AttachmentRefs)
	data, e = json.Marshal(p)
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(dir, ".pending-update-")
	if e != nil {
		return e
	}
	defer os.Remove(tmp.Name())
	if e = tmp.Chmod(0600); e == nil {
		_, e = tmp.Write(data)
	}
	if e == nil {
		e = tmp.Sync()
	}
	ce := tmp.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(tmp.Name(), name)
	}
	return e
}
