package ipc

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/configstore"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func TestLoginAgentIdentityCompletesOIDCLoopbackAndRestoresRuntimeJWT(t *testing.T) {
	p, _, _, _ := vaultPlatform(t)
	ctx := context.Background()
	signInFixture(t, p)

	callbackAddr := freeLocalAddr(t)
	p.identityCallbackAddr = callbackAddr

	var tokenRequestSeen bool
	oidc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeJSON(t, w, map[string]string{
				"issuer":                 "https://issuer.example.test",
				"authorization_endpoint": "https://issuer.example.test/authorize",
				"token_endpoint":         "http://" + r.Host + "/oauth/token",
			})
		case "/oauth/token":
			tokenRequestSeen = true
			user, password, ok := r.BasicAuth()
			if !ok || user != "client-1" || password != "client-secret" {
				t.Fatalf("unexpected client auth: ok=%v user=%q password=%q", ok, user, password)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if got := r.Form.Get("grant_type"); got != "authorization_code" {
				t.Fatalf("grant_type %q", got)
			}
			if got := r.Form.Get("code"); got != "auth-code" {
				t.Fatalf("code %q", got)
			}
			if got := r.Form.Get("redirect_uri"); got != "http://"+callbackAddr+"/oauth/callback" {
				t.Fatalf("redirect_uri %q", got)
			}
			if r.Form.Get("code_verifier") == "" {
				t.Fatal("missing PKCE verifier")
			}
			writeJSON(t, w, map[string]any{
				"id_token":      "id-token-secret",
				"refresh_token": "refresh-token-secret",
				"expires_in":    3600,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer oidc.Close()

	identityAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected Identity method: %s", r.Method)
		}
		switch r.URL.Query().Get("Action") {
		case "GetUserPool":
			writeJSON(t, w, map[string]any{"Result": map[string]any{"UserPool": map[string]any{
				"DiscoveryUrl": oidc.URL + "/.well-known/openid-configuration",
				"IssuerUrl":    "https://issuer.example.test",
			}}})
		case "ListUserPoolClients":
			writeJSON(t, w, map[string]any{"Result": map[string]any{"Data": []map[string]any{{"Uid": "client-1"}}}})
		case "GetUserPoolClient":
			writeJSON(t, w, map[string]any{"Result": map[string]any{"UserPoolClient": map[string]any{
				"ClientUid":           "client-1",
				"ClientSecret":        "client-secret",
				"AllowedCallbackUrls": []string{"http://" + callbackAddr + "/oauth/callback"},
			}}})
		default:
			t.Fatalf("unexpected Identity action: %s", r.URL.Query().Get("Action"))
		}
	}))
	defer identityAPI.Close()
	p.identityOpenAPIEndpoint = identityAPI.URL
	p.identityHTTPClient = identityAPI.Client()
	p.openExternal = func(raw string) error {
		authURL, err := url.Parse(raw)
		if err != nil {
			return err
		}
		if got := authURL.Query().Get("client_id"); got != "client-1" {
			t.Fatalf("client_id %q", got)
		}
		if got := authURL.Query().Get("redirect_uri"); got != "http://"+callbackAddr+"/oauth/callback" {
			t.Fatalf("redirect_uri %q", got)
		}
		if got := authURL.Query().Get("code_challenge_method"); got != "S256" {
			t.Fatalf("code_challenge_method %q", got)
		}
		if got := authURL.Query().Get("scope"); !strings.Contains(got, "offline_access") {
			t.Fatalf("scope %q", got)
		}
		if authURL.Query().Get("code_challenge") == "" || authURL.Query().Get("nonce") == "" {
			t.Fatal("missing OIDC nonce or PKCE challenge")
		}
		callback := authURL.Query().Get("redirect_uri") + "?state=" + url.QueryEscape(authURL.Query().Get("state")) + "&code=auth-code"
		resp, err := http.Get(callback)
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil
	}

	var prewarmSeen bool
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity/sessions/put":
			prewarmSeen = true
			if r.Header.Get("Authorization") != "Bearer runtime-key" {
				t.Fatalf("missing runtime key: %q", r.Header.Get("Authorization"))
			}
			if r.Header.Get("X-Jwt-Token") != "" {
				t.Fatalf("prewarm leaked business jwt header: %q", r.Header.Get("X-Jwt-Token"))
			}
			var body struct {
				IDToken      string `json:"idToken"`
				RefreshToken string `json:"refreshToken"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.IDToken != "id-token-secret" || body.RefreshToken != "refresh-token-secret" {
				t.Fatalf("unexpected Runtime prewarm body: %+v", body)
			}
			writeJSON(t, w, map[string]string{"senderId": "user-1"})
		case "/api/v1/sessions":
			if r.Header.Get("X-Jwt-Token") != "id-token-secret" {
				t.Fatalf("runtime request missing restored JWT: %q", r.Header.Get("X-Jwt-Token"))
			}
			io.WriteString(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer runtime.Close()
	epoch, bindFailure := p.Connection.Bind(connection.Binding{Endpoint: runtime.URL, RuntimeID: "runtime-1", RuntimeKey: "runtime-key"}, true)
	if bindFailure != nil {
		t.Fatal(bindFailure)
	}

	result := p.LoginAgentIdentity(ctx, dto.AgentIdentityLoginRequest{
		RuntimeID:       "runtime-1",
		UserPoolID:      configstore.DefaultAgentIdentityUserPoolID,
		Passphrase:      "correct horse battery",
		ConnectionEpoch: epoch,
	})
	if !result.OK || result.Value == nil || !result.Value.Exists {
		t.Fatalf("login failed: %+v", result)
	}
	if result.Value.UserPoolID != configstore.DefaultAgentIdentityUserPoolID || result.Value.Subject != "user-1" || result.Value.ClientID != "client-1" {
		t.Fatalf("unexpected saved identity status: %+v", result.Value)
	}
	if !tokenRequestSeen || !prewarmSeen {
		t.Fatalf("tokenRequestSeen=%v prewarmSeen=%v", tokenRequestSeen, prewarmSeen)
	}
	state := p.Connection.State()
	if state.ConnectionEpoch == epoch || !state.HasBusinessIdentity {
		t.Fatalf("business auth did not advance connection state: %+v", state)
	}
	resp, failure := p.Connection.Request(ctx, state.ConnectionEpoch, http.MethodGet, "/api/v1/sessions", nil, nil)
	if resp != nil {
		resp.Body.Close()
	}
	if failure != nil {
		t.Fatal(failure)
	}
	statusBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"id-token-secret", "refresh-token-secret", "correct horse battery"} {
		if strings.Contains(string(statusBytes), secret) {
			t.Fatalf("login status leaked secret %q", secret)
		}
	}
}

func freeLocalAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().String()
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}
