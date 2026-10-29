package ipc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/configstore"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func testUserTokenVaultSaveRequest() dto.UserTokenVaultSaveRequest {
	return dto.UserTokenVaultSaveRequest{
		Passphrase:   "correct horse battery",
		RuntimeID:    "runtime-1",
		UserPoolID:   configstore.DefaultAgentIdentityUserPoolID,
		Issuer:       "https://issuer.example",
		ClientID:     "client-1",
		Subject:      "user-1",
		MPAID:        "mi-1",
		SpaceID:      "space-1",
		IDToken:      "id-token-secret",
		RefreshToken: "refresh-token-secret",
		ExpiresAt:    "2026-10-29T12:00:00Z",
	}
}

func TestAgentIdentityConfigRequiresValidUserPool(t *testing.T) {
	p, _, _, _ := vaultPlatform(t)
	ctx := context.Background()
	config := p.AgentIdentityConfig(ctx)
	if !config.OK || config.Value == nil || config.Value.UserPoolID != "" || config.Value.SuggestedUserPoolID != configstore.DefaultAgentIdentityUserPoolID {
		t.Fatalf("unexpected initial config: %+v", config)
	}
	if result := p.SaveAgentIdentityConfig(ctx, dto.AgentIdentityConfigRequest{UserPoolID: "not-a-user-pool"}); result.OK || result.Error == nil || result.Error.Code != "invalidInput" {
		t.Fatalf("invalid user pool accepted: %+v", result)
	}
	if result := p.SaveAgentIdentityConfig(ctx, dto.AgentIdentityConfigRequest{UserPoolID: configstore.DefaultAgentIdentityUserPoolID}); !result.OK || result.Value == nil || result.Value.UserPoolID != configstore.DefaultAgentIdentityUserPoolID {
		t.Fatalf("save config failed: %+v", result)
	}
}

func TestUserTokenVaultSaveStatusUnlockAndForget(t *testing.T) {
	p, _, store, _ := vaultPlatform(t)
	ctx := context.Background()
	request := testUserTokenVaultSaveRequest()

	if result := p.SaveUserTokenVault(ctx, dto.UserTokenVaultSaveRequest{Passphrase: "short", RuntimeID: "runtime-1", UserPoolID: configstore.DefaultAgentIdentityUserPoolID}); result.OK || result.Error == nil || result.Error.Code != "invalidInput" {
		t.Fatalf("weak token vault passphrase accepted: %+v", result)
	}
	if result := p.SaveUserTokenVault(ctx, request); !result.OK || result.Value == nil || !result.Value.Exists || result.Value.UserPoolID != request.UserPoolID || result.Value.Subject != "user-1" || result.Value.ExpiresAt != request.ExpiresAt {
		t.Fatalf("save failed: %+v", result)
	}
	if result := p.UserTokenVaultStatus(dto.UserTokenVaultRequest{RuntimeID: request.RuntimeID}); !result.OK || result.Value == nil || !result.Value.Exists || result.Value.RuntimeID != request.RuntimeID {
		t.Fatalf("status failed: %+v", result)
	}
	if result := p.UnlockUserTokenVault(ctx, dto.UserTokenVaultUnlockRequest{RuntimeID: request.RuntimeID, Passphrase: "wrong horse battery"}); result.OK || result.Error == nil || result.Error.Code != "vaultWrongPassphrase" {
		t.Fatalf("wrong passphrase accepted: %+v", result)
	}
	if result := p.UnlockUserTokenVault(ctx, dto.UserTokenVaultUnlockRequest{RuntimeID: request.RuntimeID, Passphrase: request.Passphrase}); !result.OK || result.Value == nil || !result.Value.Exists || result.Value.Subject != request.Subject {
		t.Fatalf("unlock failed: %+v", result)
	}
	for _, suffix := range []string{"", "-wal"} {
		bytes, err := os.ReadFile(store.Path() + suffix)
		if err != nil {
			continue
		}
		for _, secret := range []string{request.IDToken, request.RefreshToken, request.Passphrase} {
			if strings.Contains(string(bytes), secret) {
				t.Fatalf("plaintext %q found in %s", secret, store.Path()+suffix)
			}
		}
	}
	if result := p.ForgetUserTokenVault(dto.UserTokenVaultRequest{RuntimeID: request.RuntimeID}); !result.OK {
		t.Fatalf("forget failed: %+v", result)
	}
	if result := p.UserTokenVaultStatus(dto.UserTokenVaultRequest{RuntimeID: request.RuntimeID}); !result.OK || result.Value == nil || result.Value.Exists {
		t.Fatalf("token vault survived forget: %+v", result)
	}
	if result := p.UnlockUserTokenVault(ctx, dto.UserTokenVaultUnlockRequest{RuntimeID: request.RuntimeID, Passphrase: request.Passphrase}); result.Error == nil || result.Error.Code != "vaultMissing" {
		t.Fatalf("missing token vault not reported: %+v", result)
	}
}

func TestUserTokenVaultRequiresConfiguredUserPool(t *testing.T) {
	p, _, _, _ := vaultPlatform(t)
	ctx := context.Background()
	request := testUserTokenVaultSaveRequest()
	request.UserPoolID = ""
	if result := p.SaveUserTokenVault(ctx, request); result.OK || result.Error == nil || result.Error.Code != "agentIdentityUserPoolRequired" {
		t.Fatalf("missing user pool accepted: %+v", result)
	}
}

func TestUserTokenVaultSaveCannotChangeConfiguredUserPool(t *testing.T) {
	p, _, _, _ := vaultPlatform(t)
	ctx := context.Background()
	if result := p.SaveAgentIdentityConfig(ctx, dto.AgentIdentityConfigRequest{UserPoolID: configstore.DefaultAgentIdentityUserPoolID}); !result.OK {
		t.Fatal(result.Error)
	}
	request := testUserTokenVaultSaveRequest()
	request.UserPoolID = "00000000-0000-0000-0000-000000000000"
	if result := p.SaveUserTokenVault(ctx, request); result.OK || result.Error == nil || result.Error.Code != "agentIdentityUserPoolMismatch" {
		t.Fatalf("mismatched user pool accepted: %+v", result)
	}
}

func TestUserTokenVaultRejectsMismatchedConfiguredUserPool(t *testing.T) {
	p, _, _, _ := vaultPlatform(t)
	ctx := context.Background()
	request := testUserTokenVaultSaveRequest()
	if result := p.SaveUserTokenVault(ctx, request); !result.OK {
		t.Fatal(result.Error)
	}
	if result := p.SaveAgentIdentityConfig(ctx, dto.AgentIdentityConfigRequest{UserPoolID: "00000000-0000-0000-0000-000000000000"}); !result.OK {
		t.Fatal(result.Error)
	}
	if result := p.UnlockUserTokenVault(ctx, dto.UserTokenVaultUnlockRequest{RuntimeID: request.RuntimeID, Passphrase: request.Passphrase}); result.OK || result.Error == nil || result.Error.Code != "agentIdentityUserPoolMismatch" {
		t.Fatalf("mismatched user pool accepted: %+v", result)
	}
}

func TestUserTokenVaultStatusProjectsNoSecrets(t *testing.T) {
	p, _, _, _ := vaultPlatform(t)
	request := testUserTokenVaultSaveRequest()
	if result := p.SaveUserTokenVault(context.Background(), request); !result.OK {
		t.Fatal(result.Error)
	}
	status := p.UserTokenVaultStatus(dto.UserTokenVaultRequest{RuntimeID: request.RuntimeID})
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{request.IDToken, request.RefreshToken, request.Passphrase} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("status leaked %q", secret)
		}
	}
}

func TestUserTokenVaultUnlockCanRestoreConnectionJWT(t *testing.T) {
	p, _, _, _ := vaultPlatform(t)
	ctx := context.Background()
	request := testUserTokenVaultSaveRequest()
	if result := p.SaveUserTokenVault(ctx, request); !result.OK {
		t.Fatal(result.Error)
	}
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer runtime-key" {
			t.Fatalf("missing runtime key: %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Jwt-Token") != request.IDToken || r.Header.Get("X-User-Id") != "" {
			t.Fatalf("wrong business identity headers: jwt=%q user=%q", r.Header.Get("X-Jwt-Token"), r.Header.Get("X-User-Id"))
		}
		io.WriteString(w, `{}`)
	}))
	defer runtime.Close()
	epoch, err := p.Connection.Bind(connection.Binding{Endpoint: runtime.URL, RuntimeID: request.RuntimeID, RuntimeKey: "runtime-key"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result := p.UnlockUserTokenVault(ctx, dto.UserTokenVaultUnlockRequest{RuntimeID: request.RuntimeID, Passphrase: request.Passphrase, ConnectionEpoch: epoch}); !result.OK || result.Value == nil || !result.Value.Exists {
		t.Fatalf("unlock failed: %+v", result)
	}
	state := p.Connection.State()
	resp, failure := p.Connection.Request(ctx, state.ConnectionEpoch, http.MethodGet, "/api/v1/sessions", nil, nil)
	if resp != nil {
		resp.Body.Close()
	}
	if failure != nil {
		t.Fatal(failure)
	}
}
