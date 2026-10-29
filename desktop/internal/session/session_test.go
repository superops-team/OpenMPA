package session

import (
	"context"
	"encoding/json"
	"github.com/superops-team/OpenMPA/desktop/internal/configstore"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSSEMultilineAndSafeProjection(t *testing.T) {
	var out []dto.ClientEvent
	text := "event: message\r\ndata: {\"id\":\"e1\",\"invocationId\":\"i1\",\r\ndata: \"content\":{\"parts\":[{\"text\":\"你好\"},{\"thought\":true,\"text\":\"hidden reasoning\"},{\"functionCall\":{\"id\":\"t1\",\"name\":\"read\",\"args\":{\"file_path\":\"src/client/desktop/Chat.tsx\",\"secret\":\"NEVER\"}}},{\"functionResponse\":{\"id\":\"t1\",\"name\":\"read\",\"response\":{\"content\":\"NEVER RETURN RAW\"}}}]}}\r\n\r\nevent: done\ndata: {\"invocationId\":\"i1\",\"terminal\":true}\n\n"
	err := ParseSSE(strings.NewReader(text), dto.SubscribeRequest{SessionID: "s1", InvocationID: "i1"}, func(e dto.ClientEvent) error { out = append(out, e); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 5 || out[0].Text != "你好" || out[1].Kind != "reasoning" || out[1].Text != "hidden reasoning" || out[1].ReasoningVisibility != "summary" || out[2].ToolName != "read" || out[2].ToolStatus != "running" || out[2].ToolSubject != "src/client/desktop/Chat.tsx" || out[3].Kind != "toolResult" || out[3].ToolStatus != "success" || out[4].Kind != "done" {
		t.Fatalf("bad projection: %+v", out)
	}
	for _, event := range out {
		encoded, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if strings.Contains(string(encoded), "NEVER") {
			t.Fatalf("unsafe projection: %s", encoded)
		}
		if event.Kind != "done" && (event.PartIndex == nil || event.SchemaVersion != 2) {
			t.Fatalf("missing v2 identity: %+v", event)
		}
	}
}

func TestCanonicalToolEventPreservesSafeCommandAndDuration(t *testing.T) {
	payload := `{"eventId":"100","type":"codex.tool.result","payload":{"invocationId":"invocation","commandId":"call-1","toolName":"commandExecution","status":"completed","durationMs":423,"outputPreview":"5\n","safeCommandSummary":{"commandPreview":"/bin/bash -lc 'ps aux | wc -l'","parameters":[{"name":"workdir","value":"/data/workspace"}]}}}`
	events, err := Project([]byte(payload), "message", dto.SubscribeRequest{SessionID: "session", InvocationID: "invocation"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events %+v", events)
	}
	event := events[0]
	if event.Kind != "toolResult" || event.ToolName != "commandExecution" || event.ToolID != "call-1" || event.ToolStatus != "success" {
		t.Fatalf("bad tool projection %+v", event)
	}
	if event.DurationMS == nil || *event.DurationMS != 423 {
		t.Fatalf("duration not preserved %+v", event.DurationMS)
	}
	if event.SafeCommandSummary == nil || event.SafeCommandSummary.CommandPreview != "/bin/bash -lc 'ps aux | wc -l'" || event.ToolSubject != event.SafeCommandSummary.CommandPreview {
		t.Fatalf("safe command summary missing %+v", event)
	}
	if event.OutputPreview != "5\n" {
		t.Fatalf("output preview missing %+v", event)
	}
}

func TestCanonicalMessageDeltaProjectsAsPartialAnswer(t *testing.T) {
	payload := `{"eventId":"101","type":"codex.turn.message_delta","payload":{"invocationId":"invocation","delta":"partial answer"}}`
	events, err := Project([]byte(payload), "message", dto.SubscribeRequest{SessionID: "session", InvocationID: "invocation"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events %+v", events)
	}
	event := events[0]
	if event.Kind != "text" || event.Text != "partial answer" || !event.Partial || event.Completeness != "partial" {
		t.Fatalf("message delta was not projected as partial answer: %+v", event)
	}
}

func TestToolSubjectRejectsURLAndCredentialLikeValues(t *testing.T) {
	for _, value := range []string{
		"https://example.com/file?token=NEVER",
		"Bearer NEVER",
		"password=NEVER",
	} {
		payload := `{"id":"event","invocationId":"invocation","content":{"parts":[{"functionCall":{"id":"tool","name":"read","args":{"file_path":` + strconv.Quote(value) + `}}}]}}`
		events, err := Project([]byte(payload), "message", dto.SubscribeRequest{SessionID: "session", InvocationID: "invocation"})
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != 1 || events[0].ToolSubject != "" {
			t.Fatalf("unsafe subject %q: %+v", value, events)
		}
	}
}
func TestProductConversationCreatesNormalSessionWithCurrentAgentProfile(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/runtime-console/overview":
			io.WriteString(w, `{"agent":{"id":"mi-current"}}`)
		case "/api/v1/sessions":
			var input dto.CreateRequest
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&input) != nil || input.MPAInstanceID != "mi-current" {
				t.Fatal("product conversation was not bound to the current agent")
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"sessionId":"normal-session","createdAt":"2026-10-28T00:00:00Z","mpaInstanceId":"mi-current","executionConfigRevision":1}`)
		case "/api/v1/sessions/normal-session/execution-config":
			w.Header().Set("ETag", `"1"`)
			io.WriteString(w, `{"revision":1,"profileRevision":1,"effectiveRefs":{"model":{"id":"default-model"},"mcpServers":[]}}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	conn := connection.New()
	epoch, _ := conn.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope"}, true)
	service := New(conn, NewPendingStore(func() (string, error) { return t.TempDir(), nil }))
	created := service.Create(context.Background(), dto.CreateRequest{ConnectionEpoch: epoch})
	if !created.OK || created.Value.SessionID != "normal-session" || created.Value.MPAInstanceID != "mi-current" || created.Value.TraceEnabled {
		t.Fatalf("bad product Session %+v", created)
	}
	config := service.ExecutionConfig(context.Background(), dto.SessionRequest{ConnectionEpoch: epoch, SessionID: created.Value.SessionID})
	if !config.OK || config.Value.ExecutionConfigVersion != "1" {
		t.Fatalf("new product Session is not model configurable: %+v", config)
	}
	want := []string{"GET /api/v1/runtime-console/overview", "POST /api/v1/sessions", "GET /api/v1/sessions/normal-session/execution-config"}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls\n%s", strings.Join(calls, "\n"))
	}
}
func TestCreatedSessionsUseRuntimeConsoleTracePath(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/runtime-console/admin/debug-sessions":
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"sessionId":"debug-session","createdAt":"2026-10-27T00:00:00Z"}`)
		case r.Method == "POST" && r.URL.Path == "/api/v1/runtime-console/admin/sessions/debug-session/run":
			var body struct {
				Content string `json:"content"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Content != "hello" {
				t.Fatal("bad run body")
			}
			io.WriteString(w, `{"sessionId":"debug-session","invocationId":"trace-run"}`)
		case r.Method == "POST" && r.URL.Path == "/api/v1/runtime-console/admin/sessions/debug-session/sse":
			io.WriteString(w, "event: done\ndata: {\"invocationId\":\"trace-run\",\"terminal\":true}\n\n")
		case r.Method == "GET" && r.URL.Path == "/api/v1/runtime-console/admin/sessions/debug-session/events":
			io.WriteString(w, `{"sessionId":"debug-session","events":[{"id":"done-1","invocationId":"trace-run","terminal":true}]}`)
		case r.Method == "POST" && r.URL.Path == "/api/v1/runtime-console/admin/sessions/debug-session/abort":
			io.WriteString(w, `{"sessionId":"debug-session","aborted":true}`)
		case r.Method == "DELETE" && r.URL.Path == "/api/v1/runtime-console/admin/sessions/debug-session":
			io.WriteString(w, `{"sessionId":"debug-session"}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	conn := connection.New()
	epoch, _ := conn.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope"}, true)
	service := New(conn, NewPendingStore(func() (string, error) { return t.TempDir(), nil }))

	created := service.CreateTraceSession(context.Background(), dto.CreateRequest{ConnectionEpoch: epoch})
	if !created.OK || created.Value.SessionID != "debug-session" || !created.Value.TraceEnabled {
		t.Fatalf("bad create %+v", created)
	}
	run := service.Submit(context.Background(), dto.SubmitRequest{ConnectionEpoch: epoch, SessionID: "debug-session", OperationID: "op", IdempotencyKey: "key", Content: "hello"})
	if !run.OK || run.Value.InvocationID != "trace-run" {
		t.Fatalf("bad run %+v", run)
	}
	stream := service.stream(context.Background(), dto.SubscribeRequest{ConnectionEpoch: epoch, SessionID: "debug-session", InvocationID: "trace-run"}, func(dto.ClientEvent) error { return nil })
	if !stream.OK {
		t.Fatalf("bad stream %+v", stream)
	}
	abort := service.Abort(context.Background(), dto.SessionRequest{ConnectionEpoch: epoch, SessionID: "debug-session"})
	if !abort.OK || !abort.Value.Aborted {
		t.Fatalf("bad abort %+v", abort)
	}
	deleted := service.Delete(context.Background(), dto.SessionRequest{ConnectionEpoch: epoch, SessionID: "debug-session"})
	if !deleted.OK {
		t.Fatalf("bad delete %+v", deleted)
	}
	want := []string{
		"POST /api/v1/runtime-console/admin/debug-sessions",
		"POST /api/v1/runtime-console/admin/sessions/debug-session/run",
		"POST /api/v1/runtime-console/admin/sessions/debug-session/sse",
		"GET /api/v1/runtime-console/admin/sessions/debug-session/events",
		"POST /api/v1/runtime-console/admin/sessions/debug-session/abort",
		"DELETE /api/v1/runtime-console/admin/sessions/debug-session",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls\n%s", strings.Join(calls, "\n"))
	}
}

func TestOrdinarySessionStreamDoesNotProbeAdminConsole(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method != "POST" || r.URL.Path != "/api/v1/sessions/s/sse" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, "event: done\ndata: {\"invocationId\":\"i\",\"terminal\":true}\n\n")
	}))
	defer server.Close()
	conn := connection.New()
	epoch, _ := conn.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope"}, true)
	service := New(conn, NewPendingStore(func() (string, error) { return t.TempDir(), nil }))
	out := service.stream(context.Background(), dto.SubscribeRequest{ConnectionEpoch: epoch, SessionID: "s", InvocationID: "i"}, func(dto.ClientEvent) error { return nil })
	if !out.OK || strings.Join(calls, "\n") != "POST /api/v1/sessions/s/sse" {
		t.Fatalf("ordinary stream changed %+v calls=%v", out, calls)
	}
}

func TestStreamArchivesOnlyMatchingInvocationBeforeDelivery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "event: message\ndata: {\"id\":\"foreign\",\"invocationId\":\"other\",\"sourceType\":\"codex.tool.result\",\"payload\":{\"toolName\":\"exec_command\",\"commandId\":\"other-tool\",\"status\":\"completed\"}}\n\nevent: message\ndata: {\"id\":\"result\",\"invocationId\":\"i\",\"sourceType\":\"codex.tool.result\",\"payload\":{\"toolName\":\"exec_command\",\"commandId\":\"tool\",\"status\":\"completed\",\"durationMs\":7}}\n\nevent: done\ndata: {\"invocationId\":\"i\",\"terminal\":true}\n\n")
	}))
	defer server.Close()
	conn := connection.New()
	epoch, _ := conn.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope"}, true)
	service := New(conn, NewPendingStore(func() (string, error) { return t.TempDir(), nil }))
	archived := []dto.ClientEvent{}
	SetEventHook(service, func(event dto.ClientEvent) { archived = append(archived, event) })
	result := service.stream(context.Background(), dto.SubscribeRequest{ConnectionEpoch: epoch, SessionID: "s", InvocationID: "i"}, func(event dto.ClientEvent) error {
		if event.EventID == "result" && (len(archived) != 1 || archived[0].SourceType != "codex.tool.result" || archived[0].DurationMS == nil || *archived[0].DurationMS != 7) {
			t.Error("event delivered before correct evidence was archived")
		}
		return nil
	})
	if !result.OK || len(archived) != 2 {
		t.Fatalf("archive boundary mismatch: %+v %d", result, len(archived))
	}
}

func TestDeleteRunsLocalCleanupHookAfterRemoteDelete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/api/v1/sessions/s" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"sessionId":"s"}`)
	}))
	defer server.Close()
	conn := connection.New()
	epoch, _ := conn.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope"}, true)
	service := New(conn, NewPendingStore(func() (string, error) { return t.TempDir(), nil }))
	cleaned := ""
	SetDeleteHook(service, func(sessionID string) { cleaned = sessionID })
	deleted := service.Delete(context.Background(), dto.SessionRequest{ConnectionEpoch: epoch, SessionID: "s"})
	if !deleted.OK || cleaned != "s" {
		t.Fatalf("delete hook not run deleted=%+v cleaned=%q", deleted, cleaned)
	}
}

func TestSessionListUsesCacheAndForceRefreshUpdatesIt(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/api/v1/sessions":
			if calls.Load() == 1 {
				io.WriteString(w, `{"sessions":[{"id":"remote-1","title":"Remote one","status":"idle"}]}`)
			} else {
				io.WriteString(w, `{"sessions":[{"id":"remote-2","title":"Remote two","status":"idle"}]}`)
			}
		case "/api/v1/runtime-console/admin/sessions":
			io.WriteString(w, `{"sessions":[]}`)
		default:
			t.Fatalf("unexpected request %s", r.URL.String())
		}
	}))
	defer server.Close()
	conn := connection.New()
	epoch, _ := conn.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope"}, true)
	cache, err := configstore.Open(filepath.Join(t.TempDir(), "openmpa.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	service := NewWithCache(conn, NewPendingStore(func() (string, error) { return t.TempDir(), nil }), cache)

	first := service.List(context.Background(), dto.ScopeRequest{ConnectionEpoch: epoch})
	if !first.OK || first.Value.Sessions[0].ID != "remote-1" {
		t.Fatalf("first %+v", first)
	}
	cached := service.List(context.Background(), dto.ScopeRequest{ConnectionEpoch: epoch})
	if !cached.OK || cached.Value.Sessions[0].ID != "remote-1" || calls.Load() != 2 {
		t.Fatalf("cache not used %+v calls=%d", cached, calls.Load())
	}
	refreshed := service.List(context.Background(), dto.ScopeRequest{ConnectionEpoch: epoch, ForceRefresh: true})
	if !refreshed.OK || refreshed.Value.Sessions[0].ID != "remote-2" {
		t.Fatalf("refresh %+v", refreshed)
	}
}

func TestSubmitIncludesLocalAssistantConfiguration(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/sessions/s/run":
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			io.WriteString(w, `{"sessionId":"s","invocationId":"i"}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	conn := connection.New()
	epoch, _ := conn.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope", RuntimeID: "runtime-a"}, true)
	cache, err := configstore.Open(filepath.Join(t.TempDir(), "openmpa.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	if _, err := cache.SaveAssistantConfig(context.Background(), "scope", dto.AssistantConfig{
		AssistantID: "runtime-a",
		AgentsMD:    "Use AGENTS.md",
		Skills: []dto.AssistantSkill{{
			ID:     "skill-a",
			Name:   "skill-a",
			Source: "local",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	service := NewWithCache(conn, NewPendingStore(func() (string, error) { return t.TempDir(), nil }), cache)
	run := service.Submit(context.Background(), dto.SubmitRequest{ConnectionEpoch: epoch, SessionID: "s", OperationID: "op", IdempotencyKey: "key", Content: "hello", AssistantID: "runtime-a"})
	if !run.OK {
		t.Fatalf("run failed %+v", run)
	}
	if body["agentsMd"] != "Use AGENTS.md" {
		t.Fatalf("agentsMd missing: %+v", body)
	}
	skills, ok := body["skills"].([]any)
	if !ok || len(skills) != 1 || skills[0] != "skill-a" {
		t.Fatalf("skills missing: %+v", body)
	}
	profile, ok := body["botProfile"].(map[string]any)
	if !ok || profile["agentsMd"] != "Use AGENTS.md" {
		t.Fatalf("bot profile missing: %+v", body)
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
