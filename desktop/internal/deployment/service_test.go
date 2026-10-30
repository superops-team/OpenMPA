package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeProvider struct {
	epoch           uint64
	runtime         RuntimeSnapshot
	tool            ToolSnapshot
	runtimeWrites   []UpdateRuntimeInput
	toolWrites      []UpdateToolInput
	writeError      *dto.SafeError
	readError       *dto.SafeError
	afterRuntimeGet func()
	runtimeReads    int
	toolReads       int
}

func (p *fakeProvider) PlatformEpoch() uint64 { return p.epoch }
func (p *fakeProvider) GetRuntime(context.Context, string, string, string) (RuntimeSnapshot, *dto.SafeError) {
	p.runtimeReads++
	r := p.runtime
	if p.afterRuntimeGet != nil {
		p.afterRuntimeGet()
	}
	return r, p.readError
}
func (p *fakeProvider) GetTool(context.Context, string, string, string) (ToolSnapshot, *dto.SafeError) {
	p.toolReads++
	return p.tool, p.readError
}
func (p *fakeProvider) UpdateRuntime(_ context.Context, _ string, r UpdateRuntimeInput) *dto.SafeError {
	p.runtimeWrites = append(p.runtimeWrites, r)
	return p.writeError
}
func (p *fakeProvider) UpdateTool(_ context.Context, _ string, r UpdateToolInput) *dto.SafeError {
	p.toolWrites = append(p.toolWrites, r)
	return p.writeError
}
func setup() (*Service, *fakeProvider, *Target) {
	p := &fakeProvider{epoch: 1, runtime: RuntimeSnapshot{RuntimeID: "r-test", Version: "v1", Image: "registry.example/mpa:old", Status: "Ready", ToolID: "t-test"}, tool: ToolSnapshot{ToolID: "t-test", Image: "registry.example/worker:old", Status: "Ready", UpdatedAt: "2026-10-25T12:00:00Z"}}
	target := &Target{Region: "cn-beijing", ProjectName: "default", RuntimeID: "r-test", ConnectionEpoch: 2, PlatformEpoch: 1}
	return New(p, func() Target { return *target }), p, target
}
func review(t *testing.T, s *Service) Images {
	t.Helper()
	r := s.ReadImages(ScopeRequest{ConnectionEpoch: 2})
	if !r.OK || r.Value == nil {
		t.Fatalf("read: %+v", r)
	}
	return *r.Value
}
func runtimeRequest(i Images) RuntimeImageRequest {
	return RuntimeImageRequest{ConnectionEpoch: 2, OperationID: "runtime-1", TargetImage: "registry.example/mpa:new", ReviewFingerprint: i.ReviewFingerprint, ReleaseEnable: true}
}
func workerRequest(i Images) WorkerImageRequest {
	return WorkerImageRequest{ConnectionEpoch: 2, OperationID: "worker-1", TargetImage: "registry.example/worker:new", ReviewFingerprint: i.ReviewFingerprint}
}

func TestOCIImageValidation(t *testing.T) {
	for _, v := range []string{"registry.example/repository/mpa:v1", "registry.example:5000/mpa:v1", "registry.example/mpa@sha256:" + strings.Repeat("a", 64), "mpa:v1", "team/mpa:v1"} {
		if !validImage(v) {
			t.Errorf("valid image rejected: %q", v)
		}
	}
	for _, v := range []string{"", "registry.example/mpa", "https://registry.example/mpa:v1", "user:password@registry.example/mpa:v1", "registry.example/mpa:v1?token=secret", "registry.example/mpa:v1#secret", "registry.example/mpa:v1\n", "registry.example/mpa:v1;id", "$(id)", "registry.example/mpa@sha256:abc", "registry.example/../mpa:v1", "Registry.example/MPA:v1", "registry.example/mpa:v1 --flag", "registry.example/mpa@sha256:" + strings.Repeat("z", 64)} {
		if validImage(v) {
			t.Errorf("unsafe image accepted: %q", v)
		}
	}
}
func TestUpdateRuntimeMinimalPayloadAndAcceptedSeparateFromReady(t *testing.T) {
	s, p, _ := setup()
	i := review(t, s)
	r := s.UpdateRuntimeImage(runtimeRequest(i))
	if !r.OK || r.Value == nil || !r.Value.Accepted || r.Value.State != "accepted" || r.Value.RuntimeReady {
		t.Fatalf("receipt: %+v", r)
	}
	b, _ := json.Marshal(p.runtimeWrites[0])
	if string(b) != `{"RuntimeId":"r-test","ArtifactUrl":"registry.example/mpa:new","ReleaseEnable":true}` {
		t.Fatalf("payload %s", b)
	}
	before := s.ReadUpdate(ReadUpdateRequest{ConnectionEpoch: 2, OperationID: "runtime-1"})
	if before.Value.State != "pending" || before.Value.RuntimeReady {
		t.Fatalf("old config reported ready: %+v", before)
	}
	p.runtime.Image = "registry.example/mpa:new"
	p.runtime.Version = "v2"
	p.runtime.Status = "Ready"
	after := s.ReadUpdate(ReadUpdateRequest{ConnectionEpoch: 2, OperationID: "runtime-1"})
	if !after.Value.RuntimeReady || after.Value.BusinessHealthVerified || after.Value.State != "pending" {
		t.Fatalf("business health unverified: %+v", after)
	}
}
func TestUpdateWorkerMinimalPayloadAndExistingSandboxUnverified(t *testing.T) {
	s, p, _ := setup()
	i := review(t, s)
	r := s.UpdateWorkerImage(workerRequest(i))
	if !r.OK || !r.Value.Accepted {
		t.Fatalf("write: %+v", r)
	}
	b, _ := json.Marshal(p.toolWrites[0])
	if string(b) != `{"ToolId":"t-test","ImageUrl":"registry.example/worker:new"}` {
		t.Fatalf("payload %s", b)
	}
	p.tool.Image = "registry.example/worker:new"
	p.tool.UpdatedAt = "2026-10-25T12:01:00Z"
	after := s.ReadUpdate(ReadUpdateRequest{ConnectionEpoch: 2, OperationID: "worker-1"})
	if !after.Value.WorkerConfigUpdated || after.Value.ExistingSandboxesVerified {
		t.Fatalf("sandbox claim: %+v", after)
	}
}
func TestPreflightRejectsObservedAssociationAndFingerprintChanges(t *testing.T) {
	for _, mutate := range []func(*fakeProvider){func(p *fakeProvider) { p.runtime.ToolID = "t-other" }, func(p *fakeProvider) { p.tool.Image = "registry.example/worker:external" }, func(p *fakeProvider) { p.tool.UpdatedAt = "2026-10-25T12:02:00Z" }, func(p *fakeProvider) { p.runtime.Version = "v2" }, func(p *fakeProvider) { p.runtime.Status = "Updating" }} {
		s, p, _ := setup()
		i := review(t, s)
		mutate(p)
		r := s.UpdateWorkerImage(workerRequest(i))
		if r.OK || r.Error == nil || r.Error.Code != "conflict" || len(p.toolWrites) != 0 {
			t.Fatalf("expected conflict: %+v", r)
		}
	}
}
func TestRequireServerReviewAndCurrentEpoch(t *testing.T) {
	s, p, target := setup()
	req := runtimeRequest(Images{ReviewFingerprint: "forged"})
	if s.UpdateRuntimeImage(req).OK {
		t.Fatal("forged review accepted")
	}
	i := review(t, s)
	req = runtimeRequest(i)
	target.ConnectionEpoch++
	if s.UpdateRuntimeImage(req).OK {
		t.Fatal("stale scope accepted")
	}
	if len(p.runtimeWrites) != 0 {
		t.Fatal("unexpected write")
	}
}
func TestScopeChangeDuringPreflightCannotWrite(t *testing.T) {
	s, p, target := setup()
	i := review(t, s)
	p.afterRuntimeGet = func() { target.RuntimeID = "r-other"; target.ConnectionEpoch++ }
	r := s.UpdateRuntimeImage(runtimeRequest(i))
	if r.OK || len(p.runtimeWrites) != 0 {
		t.Fatalf("stale write: %+v", r)
	}
}
func TestUncertainKeepsOriginalOperationAndNeverResubmits(t *testing.T) {
	s, p, _ := setup()
	i := review(t, s)
	p.writeError = dto.Failure("outcomeUnknown", "platform", "verifyOnly", 0)
	req := runtimeRequest(i)
	r := s.UpdateRuntimeImage(req)
	if !r.OK || r.Value.State != "uncertain" || r.Value.Accepted {
		t.Fatalf("uncertain: %+v", r)
	}
	s.UpdateRuntimeImage(req)
	read := s.ReadUpdate(ReadUpdateRequest{ConnectionEpoch: 2, OperationID: req.OperationID})
	if read.Value.State != "uncertain" {
		t.Fatalf("unchanged config cannot resolve unknown submission: %+v", read)
	}
	if len(p.runtimeWrites) != 1 {
		t.Fatalf("resubmitted %d times", len(p.runtimeWrites))
	}
	other := req
	other.TargetImage = "registry.example/mpa:other"
	if s.UpdateRuntimeImage(other).OK {
		t.Fatal("operation ID reuse changed intent")
	}
	other = req
	other.OperationID = "runtime-2"
	if s.UpdateRuntimeImage(other).OK || len(p.runtimeWrites) != 1 {
		t.Fatal("new ID bypassed unknown operation guard")
	}
}
func TestInvalidImagesNeverWrite(t *testing.T) {
	s, p, _ := setup()
	i := review(t, s)
	req := runtimeRequest(i)
	req.TargetImage = "registry.example/mpa:new?secret=yes"
	if s.UpdateRuntimeImage(req).OK || len(p.runtimeWrites) != 0 {
		t.Fatal("invalid image wrote")
	}
}
func TestSecretsProjectionAndSafeErrors(t *testing.T) {
	s, p, _ := setup()
	i := review(t, s)
	b, _ := json.Marshal(i)
	for _, term := range []string{"Envs", "RuntimeKey", "TOS", "SecretKey"} {
		if strings.Contains(string(b), term) {
			t.Fatalf("secret-bearing field %s", term)
		}
	}
	p.readError = &dto.SafeError{Code: "denied", Message: "SecretKey=do-not-show Envs TOS", HTTPStatus: 403, RequestID: "secret?key", CurrentState: &dto.CurrentState{Status: "SecretKey=do-not-show"}}
	r := s.ReadImages(ScopeRequest{ConnectionEpoch: 2})
	b, _ = json.Marshal(r)
	if r.OK || strings.Contains(string(b), "do-not-show") || strings.Contains(string(b), "secret?key") {
		t.Fatalf("secret error %s", b)
	}
}

type httpProvider struct {
	*fakeProvider
	url string
}

func (p *httpProvider) UpdateRuntime(ctx context.Context, _ string, r UpdateRuntimeInput) *dto.SafeError {
	return p.post(ctx, "UpdateRuntime", r)
}
func (p *httpProvider) UpdateTool(ctx context.Context, _ string, r UpdateToolInput) *dto.SafeError {
	return p.post(ctx, "UpdateTool", r)
}
func (p *httpProvider) post(ctx context.Context, action string, input interface{}) *dto.SafeError {
	b, _ := json.Marshal(input)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.url+"?Action="+action, bytes.NewReader(b))
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		return dto.Failure("outcomeUnknown", "platform", "verifyOnly", 0)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}
func TestHTTPPayloadContract(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		b, _ := io.ReadAll(r.Body)
		want := `{"RuntimeId":"r-test","ArtifactUrl":"registry.example/mpa:new","ReleaseEnable":true}`
		if r.URL.Query().Get("Action") == "UpdateTool" {
			want = `{"ToolId":"t-test","ImageUrl":"registry.example/worker:new"}`
		}
		if string(b) != want {
			t.Errorf("HTTP minimal payload: %s", b)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"Result":{},"Envs":{"SecretKey":"DO_NOT_COPY"},"TOS":{"key":"DO_NOT_COPY"}}`)
	}))
	defer server.Close()
	_, f, target := setup()
	p := &httpProvider{f, server.URL}
	s := New(p, func() Target { return *target })
	i := review(t, s)
	if !s.UpdateRuntimeImage(runtimeRequest(i)).OK || !s.UpdateWorkerImage(workerRequest(i)).OK {
		t.Fatal("HTTP update failed")
	}
	if calls != 2 {
		t.Fatalf("calls %d", calls)
	}
}
func TestWorkerReadyRequiresPlatformReadyStatus(t *testing.T) {
	s, p, _ := setup()
	i := review(t, s)
	s.UpdateWorkerImage(workerRequest(i))
	p.tool.Image = "registry.example/worker:new"
	p.tool.Status = "Updating"
	r := s.ReadUpdate(ReadUpdateRequest{ConnectionEpoch: 2, OperationID: "worker-1"})
	if !r.Value.WorkerConfigUpdated || r.Value.State == "ready" {
		t.Fatalf("updating worker claimed ready: %+v", r)
	}
	p.tool.Status = "Failed"
	r = s.ReadUpdate(ReadUpdateRequest{ConnectionEpoch: 2, OperationID: "worker-1"})
	if r.Value.State != "failed" {
		t.Fatalf("failed worker: %+v", r)
	}
}
func TestReadbackCannotRetainStaleReadyFlags(t *testing.T) {
	s, p, _ := setup()
	i := review(t, s)
	s.UpdateWorkerImage(workerRequest(i))
	p.tool.Image = "registry.example/worker:new"
	s.ReadUpdate(ReadUpdateRequest{ConnectionEpoch: 2, OperationID: "worker-1"})
	p.tool.Image = "registry.example/worker:external"
	r := s.ReadUpdate(ReadUpdateRequest{ConnectionEpoch: 2, OperationID: "worker-1"})
	if r.Value.State != "conflict" || r.Value.WorkerConfigUpdated {
		t.Fatalf("stale ready flag: %+v", r)
	}
}

func TestReadyRuntimeConfigurationAllowsNextReviewedImage(t *testing.T) {
	s, p, _ := setup()
	i := review(t, s)
	s.UpdateRuntimeImage(runtimeRequest(i))
	p.runtime.Image = "registry.example/mpa:new"
	p.runtime.Version = "v2"
	s.ReadUpdate(ReadUpdateRequest{ConnectionEpoch: 2, OperationID: "runtime-1"})
	i = review(t, s)
	req := runtimeRequest(i)
	req.OperationID = "runtime-2"
	req.TargetImage = "registry.example/mpa:next"
	if !s.UpdateRuntimeImage(req).OK || len(p.runtimeWrites) != 2 {
		t.Fatal("platform ready config permanently blocked next review")
	}
}
func TestPlatformDenialIsSafeAndHasNoAlternateRetry(t *testing.T) {
	s, p, _ := setup()
	i := review(t, s)
	p.writeError = &dto.SafeError{Code: "denied", HTTPStatus: 403, Message: "SecretKey=DO_NOT_SHOW"}
	req := runtimeRequest(i)
	r := s.UpdateRuntimeImage(req)
	if r.OK || r.Error == nil || r.Error.Code != "denied" || r.Error.HTTPStatus != 403 || strings.Contains(r.Error.Message, "DO_NOT_SHOW") {
		t.Fatalf("denial %+v", r)
	}
	r = s.ReadUpdate(ReadUpdateRequest{ConnectionEpoch: 2, OperationID: req.OperationID})
	if !r.OK || r.Value.State != "failed" || r.Value.Accepted || len(p.runtimeWrites) != 1 {
		t.Fatalf("denial retry %+v", r)
	}
}
