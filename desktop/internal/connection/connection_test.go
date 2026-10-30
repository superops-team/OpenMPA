package connection

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPrefixAndSecretSlots(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/runtime/abc/api/v1/sessions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer runtime-only" || r.Header.Get("X-User-Id") != "explicit-user" {
			t.Error("wrong authentication slots")
		}
		if r.Header.Get("X-Security-Token") != "" {
			t.Error("cloud secret leaked")
		}
		w.Write([]byte(`{"sessions":[]}`))
	}))
	defer server.Close()
	c := New()
	epoch, err := c.Bind(Binding{Endpoint: server.URL + "/runtime/abc", RuntimeID: "abc", Scope: "scope", RuntimeKey: "runtime-only"}, true)
	if err != nil {
		t.Fatal(err)
	}
	c.SetIdentity(epoch, "explicit-user")
	epoch = c.State().ConnectionEpoch
	resp, e := c.Request(context.Background(), epoch, "GET", "/api/v1/sessions", nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
}
func TestRedirectNeverCarriesSecrets(t *testing.T) {
	hit := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer server.Close()
	c := New()
	epoch, _ := c.Bind(Binding{Endpoint: server.URL, RuntimeKey: "secret"}, true)
	r, e := c.Request(context.Background(), epoch, "GET", "/ping", nil, nil)
	if r != nil {
		r.Body.Close()
	}
	if e == nil || hit {
		t.Fatal("redirect permitted")
	}
}
func TestEpochRejectsLateResponse(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-release; io.WriteString(w, `{}`) }))
	defer server.Close()
	c := New()
	epoch, _ := c.Bind(Binding{Endpoint: server.URL}, true)
	done := make(chan bool)
	go func() {
		r, e := c.Request(context.Background(), epoch, "GET", "/ping", nil, nil)
		if r != nil {
			r.Body.Close()
		}
		done <- e != nil
	}()
	<-started
	c.Disconnect()
	close(release)
	if !<-done {
		t.Fatal("stale response accepted")
	}
}
func TestEpochCancellationStopsBlockedStreamBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	c := New()
	epoch, _ := c.Bind(Binding{Endpoint: server.URL}, true)
	resp, e := c.Request(context.Background(), epoch, "GET", "/ping", nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := io.ReadAll(resp.Body); done <- e }()
	c.Disconnect()
	if e := <-done; e == nil {
		t.Fatal("blocked read did not cancel")
	}
	resp.Body.Close()
}
func TestFileQueryDoesNotChangeFixedPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sessions/id/files/download" || r.URL.Query().Get("path") != "filename..md" {
			t.Error("query changed")
		}
	}))
	defer server.Close()
	c := New()
	epoch, _ := c.Bind(Binding{Endpoint: server.URL}, true)
	r, e := c.Request(context.Background(), epoch, "GET", "/api/v1/sessions/id/files/download?path=filename..md", nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
}
func TestNumericConfigConflictKeepsSafeCurrentState(t *testing.T) {
	e := ErrorFromResponse(412, []byte(`{"detail":{"code":"execution_config_changed","currentState":{"executionConfigVersion":7,"status":"idle"}}}`))
	if e.Code != "execution_config_changed" || e.CurrentState == nil || e.CurrentState.ExecutionConfigVersion != "7" {
		t.Fatalf("numeric config state discarded: %+v", e)
	}
}
func TestMemoryErrorContractPreservesOnlySafeCurrentState(t *testing.T) {
	raw := []byte(`{"detail":{"code":"memory_operation_pending","message":"SECRET provider body","currentState":{"memoryRef":"m_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","operationRef":"mop_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","operationState":"pending","providerState":"unknown","content":"SECRET content","token":"SECRET token"}}}`)
	e := ErrorFromResponse(409, raw)
	wire, _ := json.Marshal(e)
	if e.Code != "memory_operation_pending" || e.CurrentState == nil || !strings.Contains(string(wire), `"memoryRef":"m_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`) || !strings.Contains(string(wire), `"operationRef":"mop_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"`) || !strings.Contains(string(wire), `"providerState":"unknown"`) || strings.Contains(string(wire), "SECRET") {
		t.Fatalf("memory contract discarded or leaked: %s", wire)
	}
}
func TestMemoryCurrentStateRejectsUnsafeRefsAndStates(t *testing.T) {
	for _, state := range []string{`{"memoryRef":"https://secret.invalid/token?key=SECRET","operationState":"pending"}`, `{"memoryRef":"m_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","operationRef":"SECRET\nvalue"}`, `{"memoryRef":"m_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","operationState":"SECRET provider dump"}`, `{"memoryRef":"m_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","providerState":"SECRET provider dump"}`} {
		e := ErrorFromResponse(409, []byte(`{"detail":{"code":"memory_operation_pending","currentState":`+state+`}}`))
		wire, _ := json.Marshal(e)
		if e.CurrentState != nil || strings.Contains(string(wire), "SECRET") {
			t.Fatalf("unsafe memory state projected: %s", wire)
		}
	}
}
func TestMemoryCodesRemainSpecific(t *testing.T) {
	for _, code := range []string{"memory_not_found", "session_not_found", "memory_readonly", "idempotency_conflict", "revision_conflict", "provider_revision_conflict", "memory_operation_pending", "memory_index_pending", "memory_delete_unconfirmed", "memory_provider_unavailable", "memory_freshness_degraded"} {
		t.Run(code, func(t *testing.T) {
			e := ErrorFromResponse(409, []byte(`{"detail":{"code":"`+code+`","message":"SECRET"}}`))
			if e.Code != code || strings.Contains(e.Message, "SECRET") {
				t.Fatalf("code lost: %+v", e)
			}
		})
	}
}

func TestMemoryPendingStateRequiresExactReferencesAndAllFields(t *testing.T) {
	ref := "m_" + strings.Repeat("a", 32)
	op := "mop_" + strings.Repeat("b", 64)
	for _, state := range []string{`{"memoryRef":"m_aabb","operationRef":"` + op + `","operationState":"pending","providerState":"unknown"}`, `{"memoryRef":"` + ref + `","operationRef":"mop_aabb","operationState":"pending","providerState":"unknown"}`, `{"memoryRef":"` + ref + `","operationRef":"` + op + `","operationState":"pending"}`, `{"memoryRef":"` + ref + `","operationRef":"` + op + `","operationState":"pending","providerState":null}`, `{"memoryRef":"` + ref + `","memoryRef":"` + ref + `","operationRef":"` + op + `","operationState":"pending","providerState":"unknown"}`} {
		e := ErrorFromResponse(409, []byte(`{"detail":{"code":"memory_operation_pending","currentState":`+state+`}}`))
		if e.Code != "memory_operation_pending" || e.CurrentState != nil {
			t.Fatalf("incomplete pending state retained: %+v", e)
		}
	}
}
func TestReusedTCPWriteDoesNotImplicitlyReplay(t *testing.T) {
	var posts atomic.Int32
	var getAddress string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			getAddress = r.RemoteAddr
			io.WriteString(w, `{}`)
			return
		}
		if r.RemoteAddr != getAddress {
			t.Error("fixture did not reuse warm TCP connection")
		}
		posts.Add(1)
		io.ReadAll(r.Body)
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	}))
	defer server.Close()
	c := New()
	epoch, _ := c.Bind(Binding{Endpoint: server.URL}, true)
	warm, e := c.Request(context.Background(), epoch, "GET", "/ping", nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	io.ReadAll(warm.Body)
	warm.Body.Close()
	headers := http.Header{}
	headers.Set("Idempotency-Key", "original-key")
	resp, e := c.Request(context.Background(), epoch, "POST", "/api/v1/sessions/s/run", []byte(`{"content":"original"}`), headers)
	if resp != nil {
		resp.Body.Close()
	}
	if e == nil || e.Code != "outcomeUnknown" || posts.Load() != 1 {
		t.Fatalf("implicit replay posts=%d error=%+v", posts.Load(), e)
	}
}
