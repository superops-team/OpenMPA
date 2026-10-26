package session

import (
	"context"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSSEMultilineAndSafeProjection(t *testing.T) {
	var out []dto.ClientEvent
	text := "event: message\r\ndata: {\"id\":\"e1\",\"invocationId\":\"i1\",\r\ndata: \"content\":{\"parts\":[{\"text\":\"你好\"},{\"thought\":true,\"text\":\"hidden reasoning\"},{\"functionCall\":{\"id\":\"t1\",\"name\":\"safe_tool\",\"args\":{\"secret\":\"NEVER\"}}}]}}\r\n\r\nevent: done\ndata: {\"invocationId\":\"i1\",\"terminal\":true}\n\n"
	err := ParseSSE(strings.NewReader(text), dto.SubscribeRequest{SessionID: "s1", InvocationID: "i1"}, func(e dto.ClientEvent) error { out = append(out, e); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 || out[0].Text != "你好" || out[1].ToolName != "safe_tool" || out[2].Kind != "done" {
		t.Fatalf("bad projection: %+v", out)
	}
}
func TestPendingPreservesOriginalOperation(t *testing.T) {
	store := NewPendingStore(func() (string, error) { return t.TempDir(), nil })
	p := Pending{Scope: "a", SessionID: "s", OperationID: "o", IdempotencyKey: "original", Content: "original content", ConfigVersion: "v1"}
	if err := store.Put(p); err != nil {
		t.Fatal(err)
	}
	p.Content = "changed"
	if err := store.Put(p); err == nil {
		t.Fatal("operation overwritten")
	}
	list, err := store.List("a")
	if err != nil || len(list) != 1 || list[0].Content != "original content" {
		t.Fatalf("pending %+v %v", list, err)
	}
}
func TestUnknownCompatibleRunIsNotResubmitted(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(404)
			return
		}
		calls.Add(1)
		h, _, _ := w.(http.Hijacker).Hijack()
		h.Close()
	}))
	defer server.Close()
	conn := connection.New()
	epoch, _ := conn.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope"}, true)
	store := NewPendingStore(func() (string, error) { return t.TempDir(), nil })
	service := New(conn, store)
	req := dto.SubmitRequest{ConnectionEpoch: epoch, SessionID: "s", OperationID: "op", IdempotencyKey: "key", Content: "original", ExecutionConfigVersion: "2"}
	for i := 0; i < 2; i++ {
		out := service.Submit(context.Background(), req)
		if out.OK || out.Error.Code != "outcomeUnknown" {
			t.Fatalf("bad unknown: %+v", out)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("unknown run sent %d times", calls.Load())
	}
	list, _ := store.List("scope")
	if len(list) != 1 || list[0].IdempotencyKey != "key" || list[0].Content != "original" || list[0].ConfigVersion != "2" {
		t.Fatalf("pending changed: %+v", list)
	}
}
func TestTerminalDoneEndsConsumptionAndWrongInvocationDoesNot(t *testing.T) {
	var out []dto.ClientEvent
	text := "event: done\ndata: {\"invocationId\":\"old\",\"terminal\":true}\n\nevent: done\ndata: {\"invocationId\":\"current\",\"terminal\":true}\n\nevent: message\ndata: {\"id\":\"late\",\"content\":{\"parts\":[{\"text\":\"late\"}]}}\n\n"
	err := ParseSSE(strings.NewReader(text), dto.SubscribeRequest{InvocationID: "current"}, func(e dto.ClientEvent) error { out = append(out, e); return nil })
	if err != nil || len(out) != 2 || out[0].Terminal || !out[1].Terminal {
		t.Fatalf("terminal mismatch %+v %v", out, err)
	}
}
func TestExecutionConfigReadsActualRevisionAndETagHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "\"revision-7\"")
		w.Write([]byte(`{"appName":"mpa","sessionId":"s","revision":7,"etag":"body-value","profileRevision":3,"effectiveRefs":{"model":{"id":"safe-model"}}}`))
	}))
	defer server.Close()
	conn := connection.New()
	epoch, _ := conn.Bind(connection.Binding{Endpoint: server.URL}, true)
	s := New(conn, NewPendingStore(func() (string, error) { return t.TempDir(), nil }))
	out := s.ExecutionConfig(context.Background(), dto.SessionRequest{ConnectionEpoch: epoch, SessionID: "s"})
	if !out.OK || out.Value.ExecutionConfigVersion != "7" || !out.Value.ProfileBound || out.Value.ETag != "\"revision-7\"" || out.Value.Model != "safe-model" {
		t.Fatalf("record mapping %+v", out)
	}
}
