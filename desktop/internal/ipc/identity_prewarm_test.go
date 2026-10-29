package ipc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func TestIdentityPrewarmUsesRuntimeKeyWithoutExistingBusinessHeaders(t *testing.T) {
	var received map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/base/identity/sessions/put" || r.Method != http.MethodPost {
			t.Fatalf("wrong prewarm request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer runtime-key" {
			t.Fatalf("runtime key missing: %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Jwt-Token") != "" || r.Header.Get("X-User-Id") != "" {
			t.Fatalf("business identity leaked into prewarm headers: jwt=%q user=%q", r.Header.Get("X-Jwt-Token"), r.Header.Get("X-User-Id"))
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, `{"senderId":"sender-1"}`)
	}))
	defer server.Close()

	client := connection.New()
	epoch, err := client.Bind(connection.Binding{Endpoint: server.URL + "/base", RuntimeKey: "runtime-key"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SetBusinessAuth(dto.IdentityRequest{ConnectionEpoch: epoch, AuthMode: "legacyJwt", LegacyJWT: "stale-business-jwt"}); err != nil {
		t.Fatal(err)
	}
	epoch = client.State().ConnectionEpoch
	result := (&Connection{Client: client}).PrewarmIdentity(context.Background(), dto.IdentityPrewarmRequest{
		ConnectionEpoch: epoch,
		IDToken:         "id-token",
		RefreshToken:    "refresh-token",
	})
	if !result.OK || result.Value == nil || result.Value.SenderID != "sender-1" || result.Error != nil {
		t.Fatalf("prewarm failed: %+v", result)
	}
	if received["idToken"] != "id-token" || received["refreshToken"] != "refresh-token" {
		t.Fatalf("wrong prewarm payload: %+v", received)
	}
}

func TestIdentityPrewarmFailureDoesNotExposeTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"detail":"bad id-token-secret refresh-token-secret"}`)
	}))
	defer server.Close()

	client := connection.New()
	epoch, err := client.Bind(connection.Binding{Endpoint: server.URL, RuntimeKey: "runtime-key"}, true)
	if err != nil {
		t.Fatal(err)
	}
	result := (&Connection{Client: client}).PrewarmIdentity(context.Background(), dto.IdentityPrewarmRequest{
		ConnectionEpoch: epoch,
		IDToken:         "id-token-secret",
		RefreshToken:    "refresh-token-secret",
	})
	wire, _ := json.Marshal(result)
	if result.OK || result.Error == nil || strings.Contains(string(wire), "id-token-secret") || strings.Contains(string(wire), "refresh-token-secret") {
		t.Fatalf("unsafe prewarm failure: %s", wire)
	}
}
