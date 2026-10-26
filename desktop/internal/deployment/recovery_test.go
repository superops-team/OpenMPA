package deployment

import (
	"bytes"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"os"
	"path/filepath"
	"testing"
)

func TestListUpdatesRecoversOriginalIDOfflineAfterRestartWithoutWriting(t *testing.T) {
	_, p, target := setup()
	target.Scope = "account/user/principal"
	target.CredentialProfileID = "principal"
	target.ToolID = "t-test"
	path := filepath.Join(t.TempDir(), "operations.json")
	s := New(p, func() Target { return *target }, path)
	i := review(t, s)
	p.writeError = dto.Failure("outcomeUnknown", "platform", "verifyOnly", 0)
	s.UpdateRuntimeImage(runtimeRequest(i))
	before, _ := os.ReadFile(path)
	target.ConnectionEpoch = 9
	target.PlatformEpoch = 4
	p.epoch = 4
	p.readError = dto.Failure("unavailable", "platform", "readOnly", 503)
	restarted := New(p, func() Target { return *target }, path)
	runtimeReads, toolReads := p.runtimeReads, p.toolReads
	r := restarted.ListUpdates(ScopeRequest{ConnectionEpoch: 9})
	if !r.OK || r.Value == nil || len(r.Value.Operations) != 1 {
		t.Fatalf("offline discovery %+v", r)
	}
	if p.runtimeReads != runtimeReads || p.toolReads != toolReads {
		t.Fatal("offline discovery called cloud GET")
	}
	op := r.Value.Operations[0]
	if op.OperationID != "runtime-1" || op.State != "uncertain" || op.ConnectionEpoch != 9 || r.Value.ConnectionEpoch != 9 {
		t.Fatalf("original operation %+v", op)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("query rewrote ledger")
	}
	if _, e := os.Stat(path + ".lock"); !os.IsNotExist(e) {
		t.Fatal("query created write lock")
	}
	read := restarted.ReadUpdate(ReadUpdateRequest{ConnectionEpoch: 9, OperationID: op.OperationID})
	if !read.OK || read.Value.OperationID != op.OperationID || read.Value.State != "uncertain" || len(p.runtimeWrites) != 1 {
		t.Fatalf("unreachable recovery %+v", read)
	}
}
func TestListUpdatesAndReadUpdateIsolateScopeProfileAndCurrentTool(t *testing.T) {
	for _, change := range []struct {
		name   string
		mutate func(*Target)
	}{
		{"account", func(t *Target) { t.Scope = "other/user/principal" }},
		{"profile", func(t *Target) { t.CredentialProfileID = "different-profile" }},
		{"tool", func(t *Target) { t.ToolID = "t-other" }},
		{"runtime", func(t *Target) { t.RuntimeID = "r-other" }},
		{"region", func(t *Target) { t.Region = "cn-shanghai" }},
		{"missing-scope", func(t *Target) { t.Scope = "" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			_, p, target := setup()
			target.Scope = "account/user/principal"
			target.CredentialProfileID = "principal"
			target.ToolID = "t-test"
			s := New(p, func() Target { return *target }, filepath.Join(t.TempDir(), "operations.json"))
			i := review(t, s)
			s.UpdateRuntimeImage(runtimeRequest(i))
			change.mutate(target)
			target.ConnectionEpoch++
			r := s.ListUpdates(ScopeRequest{ConnectionEpoch: 3})
			if r.OK && len(r.Value.Operations) > 0 {
				t.Fatalf("foreign receipt listed %+v", r)
			}
			read := s.ReadUpdate(ReadUpdateRequest{ConnectionEpoch: 3, OperationID: "runtime-1"})
			if read.OK {
				t.Fatalf("foreign receipt read %+v", read)
			}
		})
	}
}
func TestListUpdatesMissingLedgerDoesNotCreateDirectoryOrContactCloud(t *testing.T) {
	_, p, target := setup()
	target.Scope = "account/user/principal"
	path := filepath.Join(t.TempDir(), "never-created", "operations.json")
	p.readError = dto.Failure("unavailable", "platform", "readOnly", 503)
	s := New(p, func() Target { return *target }, path)
	r := s.ListUpdates(ScopeRequest{ConnectionEpoch: 2})
	if !r.OK || r.Value.Operations == nil || len(r.Value.Operations) != 0 {
		t.Fatalf("empty list %+v", r)
	}
	if _, e := os.Stat(filepath.Dir(path)); !os.IsNotExist(e) {
		t.Fatal("query created directory")
	}
}
func TestListUpdatesRejectsStaleEpochAndCorruptLedger(t *testing.T) {
	_, p, target := setup()
	target.Scope = "account/user/principal"
	s := New(p, func() Target { return *target })
	if s.ListUpdates(ScopeRequest{ConnectionEpoch: 1}).OK {
		t.Fatal("stale epoch list accepted")
	}
	path := filepath.Join(t.TempDir(), "operations.json")
	os.WriteFile(path, []byte("corrupt"), 0600)
	s = New(p, func() Target { return *target }, path)
	if s.ListUpdates(ScopeRequest{ConnectionEpoch: 2}).OK {
		t.Fatal("corrupt ledger list accepted")
	}
}
func TestRetryOriginalOperationAtNewEpochReturnsOriginalReceiptWithoutWrite(t *testing.T) {
	s, p, target := setup()
	target.Scope = "account/user/principal"
	target.CredentialProfileID = "principal"
	target.ToolID = "t-test"
	i := review(t, s)
	req := runtimeRequest(i)
	p.writeError = dto.Failure("outcomeUnknown", "platform", "verifyOnly", 0)
	s.UpdateRuntimeImage(req)
	target.ConnectionEpoch = 7
	req.ConnectionEpoch = 7
	r := s.UpdateRuntimeImage(req)
	if !r.OK || r.Value.ConnectionEpoch != 7 || r.Value.OperationID != req.OperationID || len(p.runtimeWrites) != 1 {
		t.Fatalf("replay %+v", r)
	}
}
func TestLegacyUnattributedProfileReceiptCannotBeExposedToNewProfile(t *testing.T) {
	_, p, target := setup()
	target.Scope = "account/user/principal"
	path := filepath.Join(t.TempDir(), "operations.json")
	s := New(p, func() Target { return *target }, path)
	i := review(t, s)
	s.UpdateRuntimeImage(runtimeRequest(i))
	target.CredentialProfileID = "principal"
	target.ToolID = "t-test"
	target.ConnectionEpoch++
	restarted := New(p, func() Target { return *target }, path)
	r := restarted.ListUpdates(ScopeRequest{ConnectionEpoch: 3})
	if !r.OK || len(r.Value.Operations) != 0 {
		t.Fatalf("legacy receipt attributed across profile binding: %+v", r)
	}
	if restarted.ReadUpdate(ReadUpdateRequest{ConnectionEpoch: 3, OperationID: "runtime-1"}).OK {
		t.Fatal("legacy receipt read across profile binding")
	}
}
func TestListUpdatesHasNoMutableAliases(t *testing.T) {
	s, p, target := setup()
	target.Scope = "account/user/principal"
	i := review(t, s)
	s.UpdateRuntimeImage(runtimeRequest(i))
	r := s.ListUpdates(ScopeRequest{ConnectionEpoch: 2})
	r.Value.Operations[0].OperationID = "renderer-changed"
	r.Value.Operations[0].TargetImage = "registry.example/mpa:fake"
	next := s.ListUpdates(ScopeRequest{ConnectionEpoch: 2})
	if next.Value.Operations[0].OperationID != "runtime-1" || next.Value.Operations[0].TargetImage != "registry.example/mpa:new" || len(p.runtimeWrites) != 1 {
		t.Fatalf("mutable receipt: %+v", next)
	}
}
