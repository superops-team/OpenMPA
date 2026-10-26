// Package runtimeconfig exposes only existing AgentKit deployment settings.
package runtimeconfig

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type Values struct {
	Description    string `json:"description"`
	CPUMilli       int    `json:"cpuMilli"`
	MemoryMB       int    `json:"memoryMb"`
	MinInstance    int    `json:"minInstance"`
	MaxInstance    int    `json:"maxInstance"`
	MaxConcurrency int    `json:"maxConcurrency"`
}
type ScopeRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
}
type Snapshot struct {
	RuntimeID   string `json:"runtimeId"`
	Version     int    `json:"version"`
	Status      string `json:"status"`
	Values      Values `json:"values"`
	Fingerprint string `json:"fingerprint"`
}
type UpdateRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	OperationID     string `json:"operationId"`
	Fingerprint     string `json:"fingerprint"`
	Values          Values `json:"values"`
	Confirm         bool   `json:"confirm"`
}
type Operation struct {
	OperationID     string `json:"operationId"`
	Scope           string `json:"scope"`
	RuntimeID       string `json:"runtimeId"`
	Region          string `json:"region"`
	Project         string `json:"project"`
	PreviousVersion int    `json:"previousVersion"`
	Target          Values `json:"target"`
	State           string `json:"state"`
}
type ReadRequest struct {
	ScopeRequest
	OperationID string `json:"operationId"`
}
type SnapshotResult struct {
	OK    bool           `json:"ok"`
	Value *Snapshot      `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type OperationResult struct {
	OK    bool           `json:"ok"`
	Value *Operation     `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type Provider interface {
	GetRuntime(context.Context, dto.RuntimeRequest) (cloud.RuntimeRecord, *dto.SafeError)
	Action(context.Context, string, string, any, any) *dto.SafeError
}
type Service struct {
	mu       sync.Mutex
	provider Provider
	target   func() deployment.Target
	path     func() (string, error)
	reviews  map[string]Snapshot
}

func New(p Provider, target func() deployment.Target, path func() (string, error)) *Service {
	return &Service{provider: p, target: target, path: path, reviews: map[string]Snapshot{}}
}
func failure(code string) *dto.SafeError { return dto.Failure(code, "platform", "never", 0) }
func values(r cloud.RuntimeRecord) Values {
	return Values{r.Description, r.CPUMilli, r.MemoryMB, r.MinInstance, r.MaxInstance, r.MaxConcurrency}
}
func valid(v Values) bool {
	return len(v.Description) <= 4096 && !strings.ContainsRune(v.Description, 0) && v.CPUMilli >= 100 && v.CPUMilli <= 64000 && v.MemoryMB >= 128 && v.MemoryMB <= 262144 && v.MinInstance >= 0 && v.MaxInstance >= 1 && v.MaxInstance <= 1000 && v.MinInstance <= v.MaxInstance && v.MaxConcurrency >= 1 && v.MaxConcurrency <= 100000
}
func validID(id string) bool {
	if len(id) < 1 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-", r) {
			return false
		}
	}
	return true
}
func (s *Service) scope(epoch uint64) (deployment.Target, *dto.SafeError) {
	t := s.target()
	if t.RuntimeID == "" || t.Scope == "" {
		return t, failure("notConnected")
	}
	if t.ConnectionEpoch != epoch {
		return t, failure("staleConnection")
	}
	return t, nil
}
func (s *Service) read(t deployment.Target) (Snapshot, *dto.SafeError) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, e := s.provider.GetRuntime(ctx, dto.RuntimeRequest{Region: t.Region, ProjectName: t.ProjectName, RuntimeID: t.RuntimeID})
	if e != nil {
		return Snapshot{}, e
	}
	if s.target() != t {
		return Snapshot{}, failure("staleConnection")
	}
	if r.RuntimeID != t.RuntimeID || r.CurrentVersionNumber < 1 || !valid(values(r)) {
		return Snapshot{}, failure("invalidResponse")
	}
	out := Snapshot{RuntimeID: r.RuntimeID, Version: r.CurrentVersionNumber, Status: r.Status, Values: values(r)}
	b, _ := json.Marshal(struct {
		Target   deployment.Target
		Snapshot Snapshot
	}{t, out})
	h := sha256.Sum256(b)
	out.Fingerprint = hex.EncodeToString(h[:])
	return out, nil
}
func (s *Service) Read(r ScopeRequest) SnapshotResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, e := s.scope(r.ConnectionEpoch)
	if e != nil {
		return SnapshotResult{Error: e}
	}
	v, e := s.read(t)
	if e != nil {
		return SnapshotResult{Error: e}
	}
	if len(s.reviews) >= 32 {
		s.reviews = map[string]Snapshot{}
	}
	s.reviews[v.Fingerprint] = v
	return SnapshotResult{OK: true, Value: &v}
}
func (s *Service) ledger(t deployment.Target, id string) (string, error) {
	dir, e := s.path()
	if e != nil {
		return "", e
	}
	h := sha256.Sum256([]byte(t.Scope + "\x00" + t.Region + "\x00" + t.ProjectName + "\x00" + t.RuntimeID + "\x00" + id))
	return filepath.Join(dir, hex.EncodeToString(h[:])+".json"), nil
}
func load(path string) (*Operation, error) {
	info, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16384 {
		return nil, os.ErrInvalid
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 16385))
	d.DisallowUnknownFields()
	var op Operation
	if d.Decode(&op) != nil || d.Decode(new(any)) != io.EOF || !validID(op.OperationID) || !valid(op.Target) || op.RuntimeID == "" || op.PreviousVersion < 1 {
		return nil, os.ErrInvalid
	}
	return &op, nil
}
func save(path string, op Operation, exclusive bool) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	b, e := json.Marshal(op)
	if e != nil {
		return e
	}
	if !exclusive {
		if _, e := load(path); e != nil {
			return e
		}
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".runtime-config-")
	if e != nil {
		return e
	}
	temporary := f.Name()
	defer os.Remove(temporary)
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if exclusive {
		if e = os.Link(temporary, path); e != nil {
			return e
		}
	} else if e = os.Rename(temporary, path); e != nil {
		return e
	}
	dir, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer dir.Close()
	return dir.Sync()
}
func matches(op *Operation, t deployment.Target) bool {
	return op.Scope == t.Scope && op.RuntimeID == t.RuntimeID && op.Region == t.Region && op.Project == t.ProjectName
}
func (s *Service) Update(r UpdateRequest) OperationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, e := s.scope(r.ConnectionEpoch)
	if e != nil {
		return OperationResult{Error: e}
	}
	if !r.Confirm || !validID(r.OperationID) || !valid(r.Values) {
		return OperationResult{Error: failure("invalidInput")}
	}
	path, err := s.ledger(t, r.OperationID)
	if err != nil {
		return OperationResult{Error: failure("unavailable")}
	}
	if old, err := load(path); err == nil {
		if !matches(old, t) || old.Target != r.Values {
			return OperationResult{Error: failure("conflict")}
		}
		return OperationResult{OK: true, Value: old}
	} else if !os.IsNotExist(err) {
		return OperationResult{Error: failure("unavailable")}
	}
	before, ok := s.reviews[r.Fingerprint]
	if !ok {
		return OperationResult{Error: failure("conflict")}
	}
	current, e := s.read(t)
	if e != nil {
		return OperationResult{Error: e}
	}
	if current.Fingerprint != r.Fingerprint || current.Values == r.Values {
		return OperationResult{Error: failure("conflict")}
	}
	if entries, err := os.ReadDir(filepath.Dir(path)); err == nil {
		if len(entries) > 1024 {
			return OperationResult{Error: failure("unavailable")}
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			prior, err := load(filepath.Join(filepath.Dir(path), entry.Name()))
			if err != nil {
				return OperationResult{Error: failure("unavailable")}
			}
			if matches(prior, t) && (prior.State == "uncertain" || prior.State == "accepted") {
				if current.Version == prior.PreviousVersion || current.Values != prior.Target || !strings.EqualFold(current.Status, "Ready") {
					return OperationResult{Error: failure("conflict")}
				}
			}
		}
	} else if !os.IsNotExist(err) {
		return OperationResult{Error: failure("unavailable")}
	}
	op := Operation{OperationID: r.OperationID, Scope: t.Scope, RuntimeID: t.RuntimeID, Region: t.Region, Project: t.ProjectName, PreviousVersion: before.Version, Target: r.Values, State: "uncertain"}
	if save(path, op, true) != nil {
		return OperationResult{Error: failure("unavailable")}
	}
	if s.target() != t {
		return OperationResult{Error: failure("staleConnection")}
	}
	input := struct {
		RuntimeID      string `json:"RuntimeId"`
		Description    string `json:"Description"`
		CPUMilli       int    `json:"CpuMilli"`
		MemoryMB       int    `json:"MemoryMb"`
		MinInstance    int    `json:"MinInstance"`
		MaxInstance    int    `json:"MaxInstance"`
		MaxConcurrency int    `json:"MaxConcurrency"`
		Release        bool   `json:"ReleaseEnable"`
	}{t.RuntimeID, r.Values.Description, r.Values.CPUMilli, r.Values.MemoryMB, r.Values.MinInstance, r.Values.MaxInstance, r.Values.MaxConcurrency, true}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e = s.provider.Action(ctx, t.Region, "UpdateRuntime", input, nil)
	if e == nil {
		op.State = "accepted"
	} else if e.Code == "denied" || e.Code == "invalidInput" || e.Code == "authRequired" {
		op.State = "failed"
	}
	// The durable uncertain intent is sufficient recovery if saving this receipt fails.
	_ = save(path, op, false)
	return OperationResult{OK: true, Value: &op}
}
func (s *Service) ReadUpdate(r ReadRequest) OperationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, e := s.scope(r.ConnectionEpoch)
	if e != nil {
		return OperationResult{Error: e}
	}
	if !validID(r.OperationID) {
		return OperationResult{Error: failure("invalidInput")}
	}
	path, err := s.ledger(t, r.OperationID)
	if err != nil {
		return OperationResult{Error: failure("unavailable")}
	}
	op, err := load(path)
	if err != nil {
		return OperationResult{Error: failure("unavailable")}
	}
	if !matches(op, t) {
		return OperationResult{Error: failure("denied")}
	}
	current, e := s.read(t)
	if e != nil {
		return OperationResult{Error: e}
	}
	if current.Version != op.PreviousVersion && current.Values == op.Target && strings.EqualFold(current.Status, "Ready") {
		op.State = "ready"
	}
	return OperationResult{OK: true, Value: op}
}
