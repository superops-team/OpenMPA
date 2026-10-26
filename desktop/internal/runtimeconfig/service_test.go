package runtimeconfig

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type fixture struct {
	current cloud.RuntimeRecord
	writes  int
	body    map[string]any
	err     *dto.SafeError
}

func (f *fixture) GetRuntime(context.Context, dto.RuntimeRequest) (cloud.RuntimeRecord, *dto.SafeError) {
	return f.current, nil
}
func (f *fixture) Action(_ context.Context, _ string, action string, body any, _ any) *dto.SafeError {
	f.writes++
	b, _ := json.Marshal(body)
	_ = json.Unmarshal(b, &f.body)
	if action != "UpdateRuntime" {
		panic("unexpected action")
	}
	return f.err
}
func setup(t *testing.T) (*Service, *fixture, deployment.Target, string) {
	t.Helper()
	p := &fixture{current: cloud.RuntimeRecord{RuntimeID: "r", CurrentVersionNumber: 7, Status: "Ready", CPUMilli: 1000, MemoryMB: 1024, MinInstance: 0, MaxInstance: 1, MaxConcurrency: 10}}
	target := deployment.Target{Scope: "account", RuntimeID: "r", Region: "cn-beijing", ProjectName: "default", ConnectionEpoch: 2}
	dir := t.TempDir()
	s := New(p, func() deployment.Target { return target }, func() (string, error) { return dir, nil })
	return s, p, target, dir
}
func TestSettingsUpdateUsesExistingFieldsAndUnknownNeverResubmits(t *testing.T) {
	s, p, target, dir := setup(t)
	read := s.Read(ScopeRequest{ConnectionEpoch: 2})
	if !read.OK {
		t.Fatal(read.Error)
	}
	v := read.Value.Values
	v.Description = "Updated"
	p.err = failure("outcomeUnknown")
	request := UpdateRequest{ConnectionEpoch: 2, OperationID: "original", Fingerprint: read.Value.Fingerprint, Values: v, Confirm: true}
	first := s.Update(request)
	if !first.OK || first.Value.State != "uncertain" || p.writes != 1 {
		t.Fatal(first)
	}
	for _, key := range []string{"ArtifactUrl", "ToolId", "Envs", "AuthorizerConfiguration"} {
		if _, found := p.body[key]; found {
			t.Fatal("unrelated configuration overwritten")
		}
	}
	if len(p.body) != 8 {
		t.Fatal("unexpected cloud payload")
	}
	reopened := New(p, func() deployment.Target { return target }, func() (string, error) { return dir, nil })
	same := reopened.Update(request)
	if !same.OK || p.writes != 1 {
		t.Fatal("original intent resubmitted")
	}
	p.current.Description = v.Description
	p.current.CurrentVersionNumber = 8
	verified := reopened.ReadUpdate(ReadRequest{ScopeRequest: ScopeRequest{ConnectionEpoch: 2}, OperationID: "original"})
	if !verified.OK || verified.Value.State != "ready" || p.writes != 1 {
		t.Fatal("configuration readback failed")
	}
}
func TestSettingsConflictAndUnconfirmedInputDoNotWrite(t *testing.T) {
	s, p, _, _ := setup(t)
	read := s.Read(ScopeRequest{ConnectionEpoch: 2})
	v := read.Value.Values
	v.MemoryMB = 2048
	input := UpdateRequest{ConnectionEpoch: 2, OperationID: "op", Fingerprint: read.Value.Fingerprint, Values: v}
	if s.Update(input).OK || p.writes != 0 {
		t.Fatal("unconfirmed write")
	}
	input.Confirm = true
	p.current.CurrentVersionNumber++
	if s.Update(input).OK || p.writes != 0 {
		t.Fatal("external configuration overwritten")
	}
	if s.Read(ScopeRequest{ConnectionEpoch: 1}).OK {
		t.Fatal("stale connection")
	}
}
