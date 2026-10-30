package deployment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A stable scope is the authenticated cloud principal scope. Epochs protect live
// IPC; stable scope/resource identity controls access to receipts after restart.
func sameResource(a, b Target) bool {
	if a.Scope == "" || b.Scope == "" {
		return a == b
	}
	return a.Scope == b.Scope && a.CredentialProfileID == b.CredentialProfileID && a.ToolID == b.ToolID && a.Region == b.Region && a.ProjectName == b.ProjectName && a.RuntimeID == b.RuntimeID
}
func operationKey(t Target, id string) string {
	b, _ := json.Marshal([]string{t.Scope, t.CredentialProfileID, t.ToolID, t.Region, t.ProjectName, t.RuntimeID, id})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type ledger struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operations    []ledgerOperation `json:"operations"`
}
type ledgerOperation struct {
	Target      Target    `json:"target"`
	Operation   Operation `json:"operation"`
	Fingerprint string    `json:"fingerprint"`
	Release     bool      `json:"release"`
	Before      Images    `json:"before"`
}

func (s *Service) loadLedger() error {
	info, e := os.Lstat(s.ledgerPath)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 2<<20 {
		return errors.New("invalid ledger file")
	}
	b, e := os.ReadFile(s.ledgerPath)
	if e != nil {
		return e
	}
	s.ledgerHash = sha256.Sum256(b)
	s.ledgerExists = true
	var l ledger
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&l) != nil || d.Decode(new(interface{})) != io.EOF || l.SchemaVersion != 1 || len(l.Operations) > 1024 {
		return errors.New("invalid ledger content")
	}
	for _, v := range l.Operations {
		if !validLedgerOperation(v) {
			return errors.New("unsafe ledger projection")
		}
		key := operationKey(v.Target, v.Operation.OperationID)
		if _, duplicate := s.operations[key]; duplicate {
			return errors.New("duplicate operation")
		}
		v.Operation.Message = "Recovered the saved operation. Read configuration to verify its current state."
		s.operations[key] = &tracked{v.Target, v.Operation, v.Fingerprint, v.Release, v.Before}
	}
	return nil
}
func validLedgerOperation(v ledgerOperation) bool {
	_, timestampError := time.Parse(time.RFC3339Nano, v.Before.WorkerUpdatedAt)
	if v.Target.Scope == "" || len(v.Target.Scope) > 512 || strings.ContainsAny(v.Target.Scope, "\r\n?=&") || !safeIdentifier.MatchString(v.Target.Region) || !safeIdentifier.MatchString(v.Target.ProjectName) || !safeIdentifier.MatchString(v.Target.RuntimeID) || !safeIdentifier.MatchString(v.Operation.OperationID) || len(v.Fingerprint) != 64 || !digest.MatchString(v.Fingerprint) || timestampError != nil {
		return false
	}
	if (v.Target.CredentialProfileID != "" && !safeIdentifier.MatchString(v.Target.CredentialProfileID)) || (v.Target.ToolID != "" && v.Target.ToolID != v.Operation.ToolID) {
		return false
	}
	if v.Operation.RuntimeID != v.Target.RuntimeID || v.Before.RuntimeID != v.Target.RuntimeID || v.Operation.ToolID != v.Before.ToolID || !safeIdentifier.MatchString(v.Operation.ToolID) || !validImage(v.Operation.PreviousImage) || !validImage(v.Operation.TargetImage) || !validImage(v.Before.RuntimeImage) || !validImage(v.Before.WorkerImage) || !safeIdentifier.MatchString(v.Before.RuntimeVersion) || !safeIdentifier.MatchString(v.Before.RuntimeStatus) || !safeIdentifier.MatchString(v.Before.WorkerStatus) || !safeIdentifier.MatchString(v.Operation.PreviousVersion) || v.Operation.ExistingSandboxesVerified || v.Operation.BusinessHealthVerified {
		return false
	}
	if v.Operation.Kind != "runtime" && v.Operation.Kind != "worker" {
		return false
	}
	switch v.Operation.State {
	case "uncertain", "accepted", "pending", "ready", "failed", "conflict":
	default:
		return false
	}
	return len(v.Operation.Message) < 1024
}
func (s *Service) saveLedger() (result error) {
	if s.ledgerPath == "" {
		return nil
	}
	defer func() {
		if result != nil {
			s.ledgerError = true
		}
	}()
	if s.ledgerError {
		return errors.New("ledger unavailable")
	}
	l := ledger{SchemaVersion: 1, Operations: []ledgerOperation{}}
	for _, v := range s.operations {
		entry := ledgerOperation{v.target, v.operation, v.fingerprint, v.release, v.before}
		if !validLedgerOperation(entry) {
			return errors.New("unsafe ledger projection")
		}
		l.Operations = append(l.Operations, entry)
	}
	b, e := json.Marshal(l)
	if e != nil {
		return e
	}
	if len(b) > 2<<20 {
		return errors.New("ledger capacity reached")
	}
	dir := filepath.Dir(s.ledgerPath)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	// An exclusive temporary lock serializes writers. A crash can leave it behind;
	// recovery then fails closed instead of silently overwriting another receipt.
	lock, e := os.OpenFile(s.ledgerPath+".lock", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer func() { lock.Close(); os.Remove(s.ledgerPath + ".lock") }()
	if info, e := os.Lstat(s.ledgerPath); e == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
		return errors.New("unsafe ledger destination")
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	current, e := os.ReadFile(s.ledgerPath)
	if e == nil {
		if !s.ledgerExists || sha256.Sum256(current) != s.ledgerHash {
			return errors.New("ledger changed externally")
		}
	} else if !os.IsNotExist(e) || s.ledgerExists {
		return errors.New("ledger unavailable or removed")
	}
	f, e := os.CreateTemp(dir, ".deployment-operations-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	if e = os.Rename(name, s.ledgerPath); e != nil {
		return e
	}
	s.ledgerHash = sha256.Sum256(b)
	s.ledgerExists = true
	directory, e := os.Open(dir)
	if e != nil {
		return e
	}
	defer directory.Close()
	return directory.Sync()
}
