package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func TestConfigurationPatchUsesExistingCategoriesAndOriginalETag(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "PATCH" || r.URL.Path != "/api/v1/sessions/s/execution-config" || r.Header.Get("If-Match") != "\"7\"" {
			t.Error("changed existing API contract")
		}
		var body struct {
			Changes []configChange `json:"changes"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("invalid body")
		}
		if len(body.Changes) != 2 || body.Changes[0].Category != "model" || body.Changes[1].Category != "mcpServers" {
			t.Fatal("unsupported categories")
		}
		model := body.Changes[0].Value.(map[string]any)
		if model["id"] != "model-id" {
			t.Error("model shape")
		}
		w.Header().Set("ETag", "\"8\"")
		w.Write([]byte(`{"revision":8,"profileRevision":1,"effectiveRefs":{"model":{"id":"model-id"},"mcpServers":[{"id":"server-id"}]},"overrides":{"model":{"mode":"replace"},"mcpServers":{"mode":"replace"}}}`))
	}))
	defer server.Close()
	c := connection.New()
	epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL}, true)
	s := New(c, NewPendingStore(func() (string, error) { return t.TempDir(), nil }))
	r := s.UpdateExecutionConfig(context.Background(), dto.ExecutionConfigUpdateRequest{ConnectionEpoch: epoch, SessionID: "s", ETag: "\"7\"", ModelMode: "replace", Model: "model-id", MCPMode: "replace", MCPServers: []string{"server-id"}})
	if !r.OK || r.Value.Model != "model-id" || r.Value.ETag != "\"8\"" || r.Value.ExecutionConfigVersion != "8" || len(r.Value.MCPServers) != 1 || calls != 1 {
		t.Fatalf("bad result %+v", r)
	}
	invalid := s.UpdateExecutionConfig(context.Background(), dto.ExecutionConfigUpdateRequest{ConnectionEpoch: epoch, SessionID: "s", ETag: "\"8\"", ModelMode: "agentsMd", MCPMode: "inherit"})
	if invalid.OK || calls != 1 {
		t.Fatal("unsupported category reached network")
	}
}

func TestConfigurationConflictDoesNotRetry(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(412)
		w.Write([]byte(`{"detail":{"code":"execution_config_changed"}}`))
	}))
	defer server.Close()
	c := connection.New()
	epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL}, true)
	s := New(c, NewPendingStore(func() (string, error) { return t.TempDir(), nil }))
	r := s.UpdateExecutionConfig(context.Background(), dto.ExecutionConfigUpdateRequest{ConnectionEpoch: epoch, SessionID: "s", ETag: "\"1\"", ModelMode: "inherit", MCPMode: "inherit"})
	if r.OK || r.Error.HTTPStatus != 412 || calls != 1 {
		t.Fatal("CAS conflict retried or hidden")
	}
}

func TestEnsureConfigurationRepairsLegacySessionOnlyOnExplicitCall(t *testing.T) {
	var paths []string
	configReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/sessions/s/execution-config":
			configReads++
			if configReads == 1 {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"detail":"missing"}`))
				return
			}
			w.Header().Set("ETag", `"1"`)
			w.Write([]byte(`{"revision":1,"profileRevision":1,"effectiveRefs":{"model":{"id":"default-model"},"mcpServers":[]}}`))
		case "/api/v1/runtime-console/overview":
			w.Write([]byte(`{"agent":{"id":"mi-current"}}`))
		case "/api/v1/sessions":
			var input dto.CreateRequest
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&input) != nil || input.SessionID != "s" || input.MPAInstanceID != "mi-current" {
				t.Fatal("legacy repair did not preserve the exact Session and current agent")
			}
			w.Write([]byte(`{"sessionId":"s"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c := connection.New()
	epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL}, true)
	s := New(c, NewPendingStore(func() (string, error) { return t.TempDir(), nil }))
	if len(paths) != 0 {
		t.Fatal("repair ran without explicit invocation")
	}
	result := s.EnsureExecutionConfig(context.Background(), dto.SessionRequest{ConnectionEpoch: epoch, SessionID: "s"})
	if !result.OK || result.Value.ExecutionConfigVersion != "1" || result.Value.Model != "default-model" {
		t.Fatalf("legacy configuration was not repaired: %+v", result)
	}
	want := []string{"GET /api/v1/sessions/s/execution-config", "GET /api/v1/runtime-console/overview", "POST /api/v1/sessions", "GET /api/v1/sessions/s/execution-config"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("repair sequence %v", paths)
	}
}

func TestEnsureConfigurationNeverRecreatesAnEditableSession(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("ETag", `"3"`)
		w.Write([]byte(`{"revision":3,"profileRevision":1,"effectiveRefs":{"model":{"id":"current"},"mcpServers":[]}}`))
	}))
	defer server.Close()
	c := connection.New()
	epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL}, true)
	result := New(c, NewPendingStore(func() (string, error) { return t.TempDir(), nil })).EnsureExecutionConfig(context.Background(), dto.SessionRequest{ConnectionEpoch: epoch, SessionID: "s"})
	if !result.OK || requests != 1 {
		t.Fatalf("editable Session was mutated: result=%+v requests=%d", result, requests)
	}
}

func TestModelCatalogWithoutCloudClientFails(t *testing.T) {
	c := connection.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"capabilities":{"extensions":[{"uri":"urn:veadk:mpa:model-selection:v1","params":{"models":["model"]}}]}}`))
	}))
	defer server.Close()
	epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope", RuntimeID: "runtime"}, true)
	out := New(c, NewPendingStore(func() (string, error) { return t.TempDir(), nil })).ModelCatalog(context.Background(), dto.SessionRequest{ConnectionEpoch: epoch, SessionID: "s"})
	if out.OK || out.Error == nil || out.Error.Code != "modelCatalogUnavailable" {
		t.Fatalf("unexpected catalog without cloud client: %+v", out)
	}
}

func TestConfirmedDiagnosticModelDoesNotAttemptSessionRepair(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/sessions/s/execution-config":
			w.WriteHeader(404)
		case "/api/v1/runtime-console/admin/sessions/s/runs":
			w.Write([]byte(`{"sessionId":"s","runs":[]}`))
		default:
			t.Errorf("diagnostic history must not be recreated: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	c := connection.New()
	epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL}, true)
	s := New(c, NewPendingStore(func() (string, error) { return t.TempDir(), nil }))
	s.markTraceSession("s")
	result := s.EnsureExecutionConfig(context.Background(), dto.SessionRequest{ConnectionEpoch: epoch, SessionID: "s"})
	if result.Error == nil || result.Error.Code != "diagnosticModelUnsupported" {
		t.Fatalf("diagnostic capability not identified: %+v", result)
	}
}
