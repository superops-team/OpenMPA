package trace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/connection"
)

func TestReadTraceUsesTheSessionScopedRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/runtime-console/sessions/session-1/trace":
			if r.Method != http.MethodGet || r.URL.Query().Get("invocationId") != "inv-1" {
				t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
			}
			w.Write([]byte(`{"traceVersion":"1.2","sessionId":"session-1","invocationId":"inv-1","run":{"status":"success"},"coverage":{"complete":true},"steps":[{"stepId":"root","sequence":1,"kind":"run.total","title":"Turn","status":"success","startedAt":"2026-10-27T00:00:00Z"}]}`))
		case "/api/v1/runtime-console/sessions/session-1/lifecycle", "/api/v1/runtime-console/sessions/session-1/worker", "/api/v1/runtime-console/sessions/session-1/diagnostics":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	c := connection.New()
	epoch, e := c.Bind(connection.Binding{Endpoint: server.URL}, true)
	if e != nil {
		t.Fatal(e)
	}
	out := New(c).Read(context.Background(), ReadRequest{ConnectionEpoch: epoch, SessionID: "session-1", InvocationID: "inv-1"})
	if !out.OK || out.Value == nil || out.Value.SessionID != "session-1" || len(out.Value.Steps) != 1 {
		t.Fatalf("bad trace %+v", out)
	}
	invalid := New(c).Read(context.Background(), ReadRequest{ConnectionEpoch: epoch, SessionID: "", InvocationID: "inv-1"})
	if invalid.OK {
		t.Fatal("empty session accepted")
	}
}

func TestReadTraceUsesLocalCacheBeforeRemote(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = cache.SaveTrace("session-1", "inv-1", Trace{
		TraceVersion: "1.2",
		SessionID:    "session-1",
		InvocationID: "inv-1",
		Steps:        []Step{{StepID: "cached", Sequence: 1, Kind: "run.total", Title: "Cached", Status: "success", StartedAt: "2026-10-27T00:00:00Z"}},
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("remote request should not be made for cached trace: %s", r.URL.String())
	}))
	defer server.Close()
	c := connection.New()
	epoch, e := c.Bind(connection.Binding{Endpoint: server.URL}, true)
	if e != nil {
		t.Fatal(e)
	}
	out := NewWithCache(c, cache).Read(context.Background(), ReadRequest{ConnectionEpoch: epoch, SessionID: "session-1", InvocationID: "inv-1"})
	if !out.OK || len(out.Value.Steps) != 1 || out.Value.Steps[0].StepID != "cached" {
		t.Fatalf("cache miss %+v", out)
	}
}

func TestReadTracePreservesCacheWithoutProjectionVersion(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := cache.dir("session-1", "inv-1")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "trace.json"), []byte(`{"traceVersion":"1.2","sessionId":"session-1","invocationId":"inv-1","steps":[{"stepId":"old","sequence":1,"kind":"run.total","title":"Old","status":"success","startedAt":"2026-10-27T00:00:00Z"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/api/v1/runtime-console/sessions/session-1/trace":
			w.Write([]byte(`{"traceVersion":"1.2","sessionId":"session-1","invocationId":"inv-1","run":{"status":"success"},"coverage":{"complete":true},"steps":[{"stepId":"fresh","sequence":1,"kind":"run.total","title":"Fresh","status":"success","startedAt":"2026-10-27T00:00:00Z"}]}`))
		case "/api/v1/runtime-console/sessions/session-1/lifecycle", "/api/v1/runtime-console/sessions/session-1/worker", "/api/v1/runtime-console/sessions/session-1/diagnostics":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected request %s", r.URL.String())
		}
	}))
	defer server.Close()
	c := connection.New()
	epoch, e := c.Bind(connection.Binding{Endpoint: server.URL}, true)
	if e != nil {
		t.Fatal(e)
	}
	out := NewWithCache(c, cache).Read(context.Background(), ReadRequest{ConnectionEpoch: epoch, SessionID: "session-1", InvocationID: "inv-1"})
	if !out.OK || out.Value.Steps[0].StepID != "old" || requests != 0 {
		t.Fatalf("saved history must survive a renderer upgrade without a remote read: result=%+v requests=%d", out, requests)
	}
	refreshed := NewWithCache(c, cache).Read(context.Background(), ReadRequest{ConnectionEpoch: epoch, SessionID: "session-1", InvocationID: "inv-1", ForceRefresh: true})
	if !refreshed.OK || requests == 0 || len(refreshed.Value.Steps) != 2 {
		t.Fatalf("explicit refresh must fetch new data and preserve saved evidence: %+v", refreshed)
	}
}

func TestReadTraceEnrichesWorkerLifecycleAndBinding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/runtime-console/sessions/session-1/trace":
			w.Write([]byte(`{"traceVersion":"1.2","sessionId":"session-1","invocationId":"inv-1","run":{"status":"success","startedAt":"2026-10-27T00:00:00Z","wallDurationMs":1000},"coverage":{"complete":true},"steps":[{"stepId":"root","sequence":1,"kind":"run.total","title":"Turn","status":"success","startedAt":"2026-10-27T00:00:00Z","durationMs":1000}]}`))
		case "/api/v1/runtime-console/sessions/session-1/lifecycle":
			if r.URL.Query().Get("invocationId") != "inv-1" {
				t.Fatalf("missing invocation id: %s", r.URL.String())
			}
			w.Write([]byte(`{"sessionId":"session-1","invocationId":"inv-1","status":"success","stages":[{"key":"worker.start_turn","title":"Worker turn 启动","status":"success","startedAt":"2026-10-27T00:00:00.100Z","endedAt":"2026-10-27T00:00:00.200Z","durationMs":100,"details":{"workerTurnId":"turn-1"}},{"key":"worker.stream.total","title":"Worker 事件流","status":"success","startedAt":"2026-10-27T00:00:00.200Z","endedAt":"2026-10-27T00:00:00.900Z","durationMs":700}]}`))
		case "/api/v1/runtime-console/sessions/session-1/worker":
			w.Write([]byte(`{"sessionId":"session-1","bindings":[{"invocationId":"inv-1","status":"running","subagentSessionId":"worker-session-1","activeSubagentRunId":"turn-1","subagentProfileId":"profile-a","metadata":{"workerTurnId":"turn-1"}},{"invocationId":"inv-later","status":"running","subagentSessionId":"worker-session-later","activeSubagentRunId":"turn-later"}],"workerSessionSummary":{"uniqueWorkerSessionCount":2}}`))
		case "/api/v1/runtime-console/sessions/session-1/diagnostics":
			w.Write([]byte(`{"items":[{"source":"worker_live","invocationId":"inv-1","workerSessionId":"worker-session-1","workerTurnId":"turn-1","status":"partial","sources":[{"source":"trajectory","status":"partial","evidence":{"sessionId":"worker-session-1","turnId":"turn-1","events":[{"eventId":"worker-tool-1","type":"codex.tool.result","payload":{"sessionId":"worker-session-1","turnId":"turn-1","toolName":"exec_command","status":"completed","commandId":"call-1","durationMs":617,"exitCode":0}},{"eventId":"worker-usage-1","type":"codex.usage.updated","payload":{"sessionId":"worker-session-1","turnId":"turn-1","status":"completed","totalTokens":42,"firstContentFrameMs":2107}}]}}]}]}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	c := connection.New()
	epoch, e := c.Bind(connection.Binding{Endpoint: server.URL}, true)
	if e != nil {
		t.Fatal(e)
	}
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out := NewWithCache(c, cache).Read(context.Background(), ReadRequest{ConnectionEpoch: epoch, SessionID: "session-1", InvocationID: "inv-1"})
	if !out.OK {
		t.Fatalf("trace failed %+v", out)
	}
	if cached, ok := cache.LoadTrace("session-1", "inv-1"); !ok || len(cached.Steps) != len(out.Value.Steps) {
		t.Fatalf("trace was not cached: ok=%v cached=%+v", ok, cached)
	}
	if _, err := os.Stat(filepath.Join(cache.dir("session-1", "inv-1"), "diagnostics.json")); err != nil {
		t.Fatalf("diagnostics evidence was not cached: %v", err)
	}
	seen := map[string]Step{}
	for _, step := range out.Value.Steps {
		seen[step.Kind] = step
	}
	if seen["worker.start_turn"].Category != "worker" || seen["worker.stream.total"].Category != "worker" {
		t.Fatalf("worker lifecycle not projected: %+v", out.Value.Steps)
	}
	binding := seen["worker.binding"]
	if binding.Category != "worker" || binding.Correlation["worker"] == nil || binding.Details["projectionSource"] != "runtime-console.worker" {
		t.Fatalf("worker binding not projected: %+v", binding)
	}
	if binding.Details["invocationId"] != "inv-1" {
		t.Fatal("another turn's worker was included")
	}
	tool := seen["worker.codex.tool.result"]
	if tool.Category != "worker" || tool.Title != "exec_command" || tool.DurationMS == nil || *tool.DurationMS != 617 || tool.Correlation["worker"] == nil || tool.Details["projectionSource"] != "runtime-console.diagnostics.worker_live.trajectory" {
		t.Fatalf("worker trajectory tool not projected: %+v", tool)
	}
	if tool.StartedAt != "" {
		t.Fatal("missing worker timestamp was fabricated from the parent turn")
	}
	usage := seen["worker.codex.usage.updated"]
	if usage.Category != "worker" || usage.Title != "Worker model usage" || usage.Details["totalTokens"] != float64(42) || usage.Details["firstContentFrameMs"] != float64(2107) {
		t.Fatalf("worker trajectory usage not projected: %+v", usage)
	}
}

func TestReadTraceProjectsNestedWorkerDiagnosticSteps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/runtime-console/sessions/session-1/trace":
			w.Write([]byte(`{"traceVersion":"1.2","sessionId":"session-1","invocationId":"inv-1","run":{"status":"success","startedAt":"2026-10-27T00:00:00Z","wallDurationMs":1000},"coverage":{"complete":true},"steps":[{"stepId":"root","sequence":1,"kind":"run.total","title":"Turn","status":"success","startedAt":"2026-10-27T00:00:00Z","durationMs":1000}]}`))
		case "/api/v1/runtime-console/sessions/session-1/lifecycle", "/api/v1/runtime-console/sessions/session-1/worker":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v1/runtime-console/sessions/session-1/diagnostics":
			w.Write([]byte(`{"items":[{"source":"worker_live","invocationId":"inv-1","workerSessionId":"worker-session-1","workerTurnId":"turn-1","sources":[{"source":"trajectory","status":"partial","evidence":{"trajectory":{"steps":[{"stepId":"worker-step-1","kind":"codex.tool.result","title":"exec_command","status":"completed","startedAt":"2026-10-27T00:00:00.300Z","durationMs":333,"details":{"commandId":"call-1","exitCode":0}}]}}}]}]}`))
		default:
			t.Fatalf("unexpected request %s", r.URL.String())
		}
	}))
	defer server.Close()
	c := connection.New()
	epoch, e := c.Bind(connection.Binding{Endpoint: server.URL}, true)
	if e != nil {
		t.Fatal(e)
	}
	out := New(c).Read(context.Background(), ReadRequest{ConnectionEpoch: epoch, SessionID: "session-1", InvocationID: "inv-1"})
	if !out.OK {
		t.Fatalf("trace failed %+v", out)
	}
	seen := map[string]Step{}
	for _, step := range out.Value.Steps {
		seen[step.StepID] = step
	}
	step := seen["worker:trajectory:worker-step-1"]
	if step.Kind != "worker.codex.tool.result" || step.Title != "exec_command" || step.DurationMS == nil || *step.DurationMS != 333 || step.Category != "worker" {
		t.Fatalf("nested worker diagnostic step not projected: %+v", out.Value.Steps)
	}
}

func TestCacheDeletesSessionsAndSweepsExpiredData(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = cache.SaveTrace("session-1", "inv-1", Trace{TraceVersion: "1.2", SessionID: "session-1", InvocationID: "inv-1"}); err != nil {
		t.Fatal(err)
	}
	if err = cache.DeleteSession("session-1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.LoadTrace("session-1", "inv-1"); ok {
		t.Fatal("deleted session trace survived")
	}
	if err = cache.SaveTrace("session-2", "inv-2", Trace{TraceVersion: "1.2", SessionID: "session-2", InvocationID: "inv-2"}); err != nil {
		t.Fatal(err)
	}
	cache.ttl = time.Hour
	cache.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if err = cache.Sweep(); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.LoadTrace("session-2", "inv-2"); ok {
		t.Fatal("expired session trace survived")
	}
}

func TestReadTracePreservesTheBackend404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"detail":"session not found"}`))
	}))
	defer server.Close()
	c := connection.New()
	epoch, e := c.Bind(connection.Binding{Endpoint: server.URL}, true)
	if e != nil {
		t.Fatal(e)
	}
	out := New(c).Read(context.Background(), ReadRequest{ConnectionEpoch: epoch, SessionID: "session-1", InvocationID: "inv-1"})
	if out.OK || out.Error == nil || out.Error.HTTPStatus != 404 {
		t.Fatalf("404 was not preserved: %+v", out)
	}
}
