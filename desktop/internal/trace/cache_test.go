package trace

import (
	"context"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestPrefetchKeepsFollowingWorkerAfterReturningToCaller(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Write([]byte(`{"traceVersion":"1.4","sessionId":"session","invocationId":"turn","run":{"status":"running"},"coverage":{"active":true,"workerTrajectory":{"status":"available"}},"steps":[{"stepId":"worker-progress","kind":"codex.turn.progress","category":"worker","status":"success"}]}`))
		} else {
			w.Write([]byte(`{"traceVersion":"1.4","sessionId":"session","invocationId":"turn","run":{"status":"success"},"coverage":{"active":false,"workerTrajectory":{"status":"unavailable"}},"steps":[{"stepId":"terminal","kind":"run.total","status":"success"}]}`))
		}
	}))
	defer server.Close()
	c := connection.New()
	epoch, failure := c.Bind(connection.Binding{Endpoint: server.URL}, true)
	if failure != nil {
		t.Fatal(failure)
	}
	defer c.Disconnect()
	service := NewWithCache(c, cache)
	if !service.Prefetch(context.Background(), ReadRequest{ConnectionEpoch: epoch, SessionID: "session", InvocationID: "turn"}).OK {
		t.Fatal("prefetch was not scheduled")
	}
	deadline := time.Now().Add(7 * time.Second)
	for time.Now().Before(deadline) {
		value, ok := cache.LoadTrace("session", "turn")
		if ok && value.Run["status"] == "success" {
			if len(value.Steps) != 2 || requests.Load() != 2 {
				t.Fatalf("worker evidence was lost at teardown: %+v", value)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("native prefetch stopped before worker completion")
}

func TestActiveTraceCacheContinuesFetchingWorkerCompletion(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	initial := Trace{TraceVersion: "1.4", SessionID: "session", InvocationID: "turn", Run: map[string]any{"status": "running"}, Coverage: map[string]any{"active": true}, Steps: []Step{{StepID: "worker-start", Kind: "codex.turn.started", Category: "worker", Status: "success"}}}
	if err = cache.SaveTrace("session", "turn", initial); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/runtime-console/sessions/session/trace" {
			<-release
			w.Write([]byte(`{"traceVersion":"1.4","sessionId":"session","invocationId":"turn","run":{"status":"success"},"coverage":{"active":false,"complete":true},"steps":[{"stepId":"worker-complete","kind":"codex.turn.completed","category":"worker","status":"success"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	c := connection.New()
	epoch, failure := c.Bind(connection.Binding{Endpoint: server.URL}, true)
	if failure != nil {
		t.Fatal(failure)
	}
	service := NewWithCache(c, cache)
	got := service.Read(context.Background(), ReadRequest{ConnectionEpoch: epoch, SessionID: "session", InvocationID: "turn"})
	// Read must return existing data before the held remote request completes.
	if !got.OK || !traceStillActive(*got.Value) {
		close(release)
		t.Fatalf("cached initial trace unavailable: %+v", got)
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		fresh, ok := cache.LoadTrace("session", "turn")
		if ok && !traceStillActive(fresh) {
			if len(fresh.Steps) != 2 {
				t.Fatalf("worker history or completion missing: %+v", fresh.Steps)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("active cached trace never reached worker completion")
}

func TestLatePrefetchCannotRestoreDeletedSession(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = cache.DeleteSession("deleted"); err != nil {
		t.Fatal(err)
	}
	if err = cache.SaveTrace("deleted", "turn", Trace{SessionID: "deleted", InvocationID: "turn"}); err != nil {
		t.Fatal(err)
	}
	if err = cache.SaveDiagnostics("deleted", "turn", map[string]any{"items": []any{}}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(cache.dir("deleted", "turn")); !os.IsNotExist(err) {
		t.Fatalf("deleted data reappeared: %v", err)
	}
}

func TestSweepUsesTurnFileAgeAndKeepsRecentlyUpdatedTurns(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	cache.now = func() time.Time { return now }
	cache.ttl = time.Hour
	for _, id := range []string{"old", "updated"} {
		if err = cache.SaveTrace("session", id, Trace{SessionID: "session", InvocationID: id}); err != nil {
			t.Fatal(err)
		}
		old := now.Add(-2 * time.Hour)
		if err = os.Chtimes(filepath.Join(cache.dir("session", id), "trace.json"), old, old); err != nil {
			t.Fatal(err)
		}
	}
	// Updating an existing turn doesn't update its parent session's mtime.
	if err = cache.SaveDiagnostics("session", "updated", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-2 * time.Hour)
	if err = os.Chtimes(filepath.Dir(cache.dir("session", "updated")), old, old); err != nil {
		t.Fatal(err)
	}
	if err = cache.Sweep(); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.LoadTrace("session", "old"); ok {
		t.Fatal("expired turn survived")
	}
	if _, ok := cache.LoadTrace("session", "updated"); !ok {
		t.Fatal("recent turn was deleted by its parent directory age")
	}
}
