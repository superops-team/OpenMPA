package deployment

import (
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDurableUncertainOperationRecoveredAfterRestart(t *testing.T) {
	_, p, target := setup()
	target.Scope = "1234567890/user/principal"
	path := filepath.Join(t.TempDir(), "private", "operations.json")
	s := New(p, func() Target { return *target }, path)
	i := review(t, s)
	p.writeError = dto.Failure("outcomeUnknown", "platform", "verifyOnly", 0)
	s.UpdateRuntimeImage(runtimeRequest(i))
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(b), "SecretKey") || strings.Contains(string(b), "Envs") {
		t.Fatalf("ledger secret %s", b)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("ledger mode %v", info.Mode())
	}
	target.ConnectionEpoch = 8
	target.PlatformEpoch = 3
	p.epoch = 3
	restarted := New(p, func() Target { return *target }, path)
	r := restarted.ReadUpdate(ReadUpdateRequest{ConnectionEpoch: 8, OperationID: "runtime-1"})
	if !r.OK || r.Value.State != "uncertain" || r.Value.ConnectionEpoch != 8 || len(p.runtimeWrites) != 1 {
		t.Fatalf("recovery %+v", r)
	}
	newReview := restarted.ReadImages(ScopeRequest{ConnectionEpoch: 8})
	req := runtimeRequest(*newReview.Value)
	req.ConnectionEpoch = 8
	req.OperationID = "runtime-2"
	if restarted.UpdateRuntimeImage(req).OK || len(p.runtimeWrites) != 1 {
		t.Fatal("restart bypassed unknown guard")
	}
	p.runtime.Image = "registry.example/mpa:new"
	p.runtime.Version = "v2"
	r = restarted.ReadUpdate(ReadUpdateRequest{ConnectionEpoch: 8, OperationID: "runtime-1"})
	if !r.Value.RuntimeReady || r.Value.Accepted || r.Value.BusinessHealthVerified {
		t.Fatalf("readback evidence %+v", r)
	}
}
func TestOperationIDsAreScopedByAuthenticatedCloudPrincipal(t *testing.T) {
	_, p, target := setup()
	target.Scope = "account/user/first"
	path := filepath.Join(t.TempDir(), "operations.json")
	s := New(p, func() Target { return *target }, path)
	i := review(t, s)
	s.UpdateRuntimeImage(runtimeRequest(i))
	target.Scope = "account/user/second"
	target.ConnectionEpoch++
	r := s.ReadUpdate(ReadUpdateRequest{ConnectionEpoch: 3, OperationID: "runtime-1"})
	if r.OK {
		t.Fatal("cross scope receipt leak")
	}
	rr := s.ReadImages(ScopeRequest{ConnectionEpoch: 3})
	req := runtimeRequest(*rr.Value)
	req.ConnectionEpoch = 3
	if !s.UpdateRuntimeImage(req).OK || len(p.runtimeWrites) != 2 {
		t.Fatal("operation ID incorrectly collides across scopes")
	}
}
func TestLedgerCorruptionOrUnsafePathRejectsWrite(t *testing.T) {
	for _, mode := range []string{"corrupt", "symlink", "world-readable"} {
		t.Run(mode, func(t *testing.T) {
			_, p, target := setup()
			target.Scope = "account/user/one"
			dir := t.TempDir()
			path := filepath.Join(dir, "operations.json")
			if mode == "symlink" {
				other := filepath.Join(dir, "other")
				os.WriteFile(other, []byte("{}"), 0600)
				os.Symlink(other, path)
			} else {
				permissions := os.FileMode(0600)
				if mode == "world-readable" {
					permissions = 0644
				}
				os.WriteFile(path, []byte("corrupt"), permissions)
			}
			s := New(p, func() Target { return *target }, path)
			i := review(t, s)
			r := s.UpdateRuntimeImage(runtimeRequest(i))
			if r.OK || len(p.runtimeWrites) != 0 {
				t.Fatalf("unsafe ledger writes %+v", r)
			}
		})
	}
}
func TestMissingWorkerTimestampRejectsReview(t *testing.T) {
	s, p, _ := setup()
	p.tool.UpdatedAt = ""
	if s.ReadImages(ScopeRequest{ConnectionEpoch: 2}).OK {
		t.Fatal("missing fingerprint timestamp accepted")
	}
}

func TestConstructorDoesNotResolveOrTouchApplicationData(t *testing.T) {
	_, p, target := setup()
	calls := 0
	s := NewWithLedger(p, func() Target { return *target }, func() (string, error) { calls++; return filepath.Join(t.TempDir(), "operations.json"), nil })
	if calls != 0 {
		t.Fatal("constructor resolved app data")
	}
	s.ReadImages(ScopeRequest{ConnectionEpoch: 2})
	if calls != 1 {
		t.Fatal("first call did not lazily resolve")
	}
}
func TestConcurrentLedgerChangeRejectsWrite(t *testing.T) {
	_, p, target := setup()
	target.Scope = "account/user/principal"
	path := filepath.Join(t.TempDir(), "operations.json")
	s := New(p, func() Target { return *target }, path)
	i := review(t, s)
	os.WriteFile(path, []byte(`{"schemaVersion":1,"operations":[]}`), 0600)
	if s.UpdateRuntimeImage(runtimeRequest(i)).OK || len(p.runtimeWrites) != 0 {
		t.Fatal("unobserved external ledger overwritten")
	}
}
