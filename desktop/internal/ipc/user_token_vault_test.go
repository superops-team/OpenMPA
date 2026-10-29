package ipc

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func testUserTokenVaultSaveRequest() dto.UserTokenVaultSaveRequest {
	return dto.UserTokenVaultSaveRequest{
		Passphrase:   "correct horse battery",
		RuntimeID:    "runtime-1",
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

func TestUserTokenVaultSaveStatusUnlockAndForget(t *testing.T) {
	p, _, store, _ := vaultPlatform(t)
	ctx := context.Background()
	request := testUserTokenVaultSaveRequest()

	if result := p.SaveUserTokenVault(ctx, dto.UserTokenVaultSaveRequest{Passphrase: "short", RuntimeID: "runtime-1"}); result.OK || result.Error == nil || result.Error.Code != "invalidInput" {
		t.Fatalf("weak token vault passphrase accepted: %+v", result)
	}
	if result := p.SaveUserTokenVault(ctx, request); !result.OK || result.Value == nil || !result.Value.Exists || result.Value.Subject != "user-1" || result.Value.ExpiresAt != request.ExpiresAt {
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
