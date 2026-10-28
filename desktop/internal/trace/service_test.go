package trace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
		case "/api/v1/runtime-console/sessions/session-1/lifecycle", "/api/v1/runtime-console/sessions/session-1/worker":
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
			w.Write([]byte(`{"sessionId":"session-1","bindings":[{"status":"running","subagentSessionId":"worker-session-1","activeSubagentRunId":"turn-1","subagentProfileId":"profile-a","metadata":{"workerTurnId":"turn-1"}}],"workerSessionSummary":{"uniqueWorkerSessionCount":1}}`))
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
	if !out.OK {
		t.Fatalf("trace failed %+v", out)
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
