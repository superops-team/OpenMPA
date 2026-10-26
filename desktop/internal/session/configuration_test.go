package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
