package session

import (
	"context"
	"encoding/json"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSSEEOFWithoutTerminalIsInterrupted(t *testing.T) {
	if err := ParseSSE(strings.NewReader("event: message\ndata: {\"id\":\"e\",\"invocationId\":\"i\",\"content\":{\"parts\":[{\"text\":\"partial\"}]}}\n\n"), dto.SubscribeRequest{InvocationID: "i"}, func(dto.ClientEvent) error { return nil }); err == nil {
		t.Fatal("EOF falsely terminal")
	}
}
func TestAcceptedReceiptSurvivesReopenAndRecoverDoesNotRun(t *testing.T) {
	var runs atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/run") {
			runs.Add(1)
			w.Write([]byte(`{"sessionId":"s","invocationId":"i"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()
	c := connection.New()
	epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope"}, true)
	dir := t.TempDir()
	s := New(c, NewPendingStore(func() (string, error) { return dir, nil }))
	out := s.Submit(context.Background(), dto.SubmitRequest{ConnectionEpoch: epoch, SessionID: "s", OperationID: "o", IdempotencyKey: "key", Content: "content"})
	if !out.OK {
		t.Fatal(out.Error)
	}
	reopened := New(c, NewPendingStore(func() (string, error) { return dir, nil }))
	list := reopened.Pending(context.Background(), dto.ScopeRequest{ConnectionEpoch: epoch})
	if !list.OK || len(list.Value.Operations) != 1 || list.Value.Operations[0].InvocationID != "i" {
		t.Fatal("accepted receipt lost")
	}
	recover := reopened.Recover(context.Background(), dto.RecoverRequest{ConnectionEpoch: epoch, OperationID: "o", Mode: "subscribeAccepted"})
	if !recover.OK || recover.Value.InvocationID != "i" || runs.Load() != 1 {
		t.Fatal("recovery started new run")
	}
}
func TestUnknownOrdinaryRecoverCannotRetryAndReviewMustBeExplicit(t *testing.T) {
	store := NewPendingStore(func() (string, error) { return t.TempDir(), nil })
	c := connection.New()
	epoch, _ := c.Bind(connection.Binding{Endpoint: "https://example.invalid", Scope: "scope"}, false)
	s := New(c, store)
	store.Put(Pending{Scope: "scope", SessionID: "s", OperationID: "o", IdempotencyKey: "original", Content: "original"})
	if s.Recover(context.Background(), dto.RecoverRequest{ConnectionEpoch: epoch, OperationID: "o", Mode: "retryDurable"}).OK {
		t.Fatal("ordinary unknown retried")
	}
	if s.ResolvePending(context.Background(), dto.ResolvePendingRequest{ConnectionEpoch: epoch, OperationID: "o", Mode: "userReviewed"}).OK {
		t.Fatal("pending silently deleted")
	}
	if !s.ResolvePending(context.Background(), dto.ResolvePendingRequest{ConnectionEpoch: epoch, OperationID: "o", Mode: "userReviewed", Confirm: true}).OK {
		t.Fatal("explicit review cannot resolve")
	}
}
func TestDurableRetryKeepsOriginalKeyContentAndVersion(t *testing.T) {
	var runs atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/execution-config") {
			w.Write([]byte(`{"sessionId":"s","profileRevision":1,"revision":2,"etag":"\"2\"","effectiveRefs":{}}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/run") {
			var b struct {
				Content string `json:"content"`
				Version int    `json:"executionConfigVersion"`
			}
			json.NewDecoder(r.Body).Decode(&b)
			if b.Content != "original" || b.Version != 2 || r.Header.Get("Idempotency-Key") != "original-key" {
				t.Error("recovery payload changed")
			}
			if runs.Add(1) == 1 {
				h, _, _ := w.(http.Hijacker).Hijack()
				h.Close()
				return
			}
			io.WriteString(w, `{"sessionId":"s","invocationId":"same"}`)
		}
	}))
	defer server.Close()
	c := connection.New()
	epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope"}, true)
	s := New(c, NewPendingStore(func() (string, error) { return t.TempDir(), nil }))
	out := s.Submit(context.Background(), dto.SubmitRequest{ConnectionEpoch: epoch, SessionID: "s", OperationID: "o", IdempotencyKey: "original-key", Content: "original", ExecutionConfigVersion: "2"})
	if out.OK {
		t.Fatal("expected unknown")
	}
	recovered := s.Recover(context.Background(), dto.RecoverRequest{ConnectionEpoch: epoch, OperationID: "o", Mode: "retryDurable"})
	if !recovered.OK || recovered.Value.InvocationID != "same" || runs.Load() != 2 {
		t.Fatalf("durable retry %+v", recovered)
	}
}
func TestNonTerminalEOFEmitsInterruptedAndPreservesAcceptedPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "event: heartbeat\ndata: {}\n\n") }))
	defer server.Close()
	c := connection.New()
	epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope"}, true)
	store := NewPendingStore(func() (string, error) { return t.TempDir(), nil })
	store.Put(Pending{Scope: "scope", SessionID: "s", OperationID: "o", Content: "content", IdempotencyKey: "key"})
	store.Update("scope", "o", func(p *Pending) error { p.State = "accepted"; p.InvocationID = "i"; return nil })
	s := New(c, store)
	out := s.stream(context.Background(), dto.SubscribeRequest{ConnectionEpoch: epoch, SessionID: "s", InvocationID: "i"}, func(dto.ClientEvent) error { return nil })
	list, _ := store.List("scope")
	if out.OK || out.Error.Code != "streamInterrupted" || len(list) != 1 || list[0].InvocationID != "i" {
		t.Fatal("EOF lost accepted run")
	}
}
func TestCursor410DoesNotRunOrSilentlyClearCursor(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sessions/s/sse" {
			t.Error("unexpected run/history request")
		}
		var req struct {
			Last       string `json:"lastEventId"`
			Invocation string `json:"invocationId"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		requests = append(requests, req.Last)
		if len(requests) == 1 {
			w.WriteHeader(410)
			io.WriteString(w, `{"detail":{"code":"cursor_expired"}}`)
			return
		}
		io.WriteString(w, "event: done\ndata: {\"invocationId\":\"i\",\"terminal\":true}\n\n")
	}))
	defer server.Close()
	c := connection.New()
	epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope"}, true)
	s := New(c, NewPendingStore(func() (string, error) { return t.TempDir(), nil }))
	req := dto.SubscribeRequest{ConnectionEpoch: epoch, SessionID: "s", InvocationID: "i", LastEventID: "original-cursor"}
	first := s.stream(context.Background(), req, func(dto.ClientEvent) error { return nil })
	if first.OK || first.Error.HTTPStatus != 410 || first.Error.RetryDisposition != "history" {
		t.Fatal("cursor expiry lost")
	}
	req.LastEventID = ""
	second := s.stream(context.Background(), req, func(dto.ClientEvent) error { return nil })
	if !second.OK || len(requests) != 2 || requests[0] != "original-cursor" || requests[1] != "" {
		t.Fatal("cursor reset not exact")
	}
}
func TestResolveTerminalRequiresMatchingServiceTerminalAndHistory(t *testing.T) {
	historyInvocation := "wrong"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"events":[{"id":"e","invocationId":"`+historyInvocation+`","partial":false}]}`)
	}))
	defer server.Close()
	c := connection.New()
	epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope"}, true)
	store := NewPendingStore(func() (string, error) { return t.TempDir(), nil })
	store.Put(Pending{Scope: "scope", SessionID: "s", OperationID: "o", Content: "content", IdempotencyKey: "key"})
	store.Update("scope", "o", func(p *Pending) error { p.State = "accepted"; p.InvocationID = "i"; return nil })
	s := New(c, store)
	request := dto.ResolvePendingRequest{ConnectionEpoch: epoch, OperationID: "o", Mode: "verifiedTerminal", Confirm: true}
	if s.ResolvePending(context.Background(), request).OK {
		t.Fatal("receipt assumed terminal")
	}
	s.observeTerminal(context.Background(), dto.SubscribeRequest{ConnectionEpoch: epoch, SessionID: "s", InvocationID: "i"})
	if s.ResolvePending(context.Background(), request).OK {
		t.Fatal("wrong invocation history accepted")
	}
	historyInvocation = "i"
	s.observeTerminal(context.Background(), dto.SubscribeRequest{ConnectionEpoch: epoch, SessionID: "s", InvocationID: "i"})
	if !s.ResolvePending(context.Background(), request).OK {
		t.Fatal("verified matching terminal not resolved")
	}
}
func TestReadHistoryMarksMatchingPendingTerminalVerified(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/sessions/s":
			io.WriteString(w, `{"id":"s","title":"Chat","status":"running"}`)
		case "/api/v1/sessions/s/events":
			io.WriteString(w, `{"events":[{"id":"done-1","invocationId":"i","terminal":true}]}`)
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c := connection.New()
	epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope"}, true)
	store := NewPendingStore(func() (string, error) { return t.TempDir(), nil })
	if err := store.Put(Pending{Scope: "scope", SessionID: "s", OperationID: "o", Content: "content", IdempotencyKey: "key"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Update("scope", "o", func(p *Pending) error { p.State = "accepted"; p.InvocationID = "i"; return nil }); err != nil {
		t.Fatal(err)
	}
	s := New(c, store)
	read := s.Read(context.Background(), dto.SessionRequest{ConnectionEpoch: epoch, SessionID: "s"})
	if !read.OK {
		t.Fatalf("read failed %+v", read)
	}
	pending := s.Pending(context.Background(), dto.ScopeRequest{ConnectionEpoch: epoch})
	if !pending.OK || len(pending.Value.Operations) != 1 || !pending.Value.Operations[0].TerminalVerified {
		t.Fatalf("pending not terminal verified %+v", pending)
	}
}
func TestUnterminatedSSEFrameCannotProveTerminal(t *testing.T) {
	emitted := 0
	err := ParseSSE(strings.NewReader("event: done\ndata: {\"invocationId\":\"i\",\"terminal\":true}"), dto.SubscribeRequest{InvocationID: "i"}, func(dto.ClientEvent) error { emitted++; return nil })
	if err == nil || emitted != 0 {
		t.Fatal("incomplete frame promoted to terminal")
	}
}
