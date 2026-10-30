package ipc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/configstore"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/vault"
)

type vaultTransport struct{ account string }

func (f *vaultTransport) RoundTrip(*http.Request) (*http.Response, error) {
	body := `{"Result":{"AccountId":"` + f.account + `","UserId":"user-one","IdentityType":"User"}}`
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func vaultPlatform(t *testing.T) (*Platform, *vault.Service, *configstore.Store, *vaultTransport) {
	t.Helper()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "openmpa.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	service := vault.NewWithIterations(store, vault.MinIterations)
	transport := &vaultTransport{account: "account-one"}
	p := NewPlatformWithVault(cloud.NewWithTransport(transport), connection.New(), service)
	return p, service, store, transport
}

func signInFixture(t *testing.T, p *Platform) {
	t.Helper()
	result := p.SignIn(context.Background(), dto.SignInRequest{Region: "cn-beijing", AccessKey: "AK_FIXTURE", SecretKey: "SK_FIXTURE"})
	if !result.OK {
		t.Fatal(result.Error)
	}
}

func TestVaultSaveUnlockAndForget(t *testing.T) {
	p, _, store, _ := vaultPlatform(t)
	ctx := context.Background()
	signInFixture(t, p)

	if result := p.SaveVault(ctx, dto.VaultPassphraseRequest{Passphrase: "short"}); result.OK || result.Error == nil || result.Error.Code != "invalidInput" {
		t.Fatalf("weak passphrase accepted: %+v", result)
	}
	if result := p.SaveVault(ctx, dto.VaultPassphraseRequest{Passphrase: "correct horse battery"}); !result.OK || result.Value == nil || !result.Value.Exists || result.Value.AccountID != "account-one" || result.Value.Region != "cn-beijing" {
		t.Fatalf("save failed: %+v", result)
	}

	// The database and its WAL contain ciphertext only.
	for _, suffix := range []string{"", "-wal"} {
		bytes, err := os.ReadFile(store.Path() + suffix)
		if err != nil {
			continue
		}
		for _, secret := range []string{"AK_FIXTURE", "SK_FIXTURE", "correct horse battery"} {
			if strings.Contains(string(bytes), secret) {
				t.Fatalf("plaintext %q found in %s", secret, store.Path()+suffix)
			}
		}
	}

	if result := p.SignOut(); !result.OK {
		t.Fatal(result.Error)
	}
	if p.Cloud.State().SignedIn {
		t.Fatal("sign out did not clear the in-memory credential")
	}
	if result := p.UnlockVault(ctx, dto.VaultPassphraseRequest{Passphrase: "wrong horse battery"}); result.OK || result.Error == nil || result.Error.Code != "vaultWrongPassphrase" {
		t.Fatalf("wrong passphrase accepted: %+v", result)
	}
	if p.Cloud.State().SignedIn {
		t.Fatal("wrong passphrase signed in")
	}
	if result := p.UnlockVault(ctx, dto.VaultPassphraseRequest{Passphrase: "correct horse battery"}); !result.OK || result.Value == nil || !result.Value.SignedIn || result.Value.Principal == nil || result.Value.Principal.AccountID != "account-one" {
		t.Fatalf("unlock failed: %+v", result)
	}
	if result := p.ForgetVault(); !result.OK {
		t.Fatalf("forget failed: %+v", result)
	}
	if result := p.VaultStatus(); !result.OK || result.Value == nil || result.Value.Exists {
		t.Fatalf("vault survived forget: %+v", result)
	}
	if result := p.UnlockVault(ctx, dto.VaultPassphraseRequest{Passphrase: "correct horse battery"}); result.Error == nil || result.Error.Code != "vaultMissing" {
		t.Fatalf("missing vault not reported: %+v", result)
	}
}

func TestVaultRequiresSignedInCredential(t *testing.T) {
	p, _, _, _ := vaultPlatform(t)
	result := p.SaveVault(context.Background(), dto.VaultPassphraseRequest{Passphrase: "correct horse battery"})
	if result.OK || result.Error == nil || result.Error.Code != "vaultNotSignedIn" {
		t.Fatalf("save without sign-in accepted: %+v", result)
	}
}

func TestVaultRejectsAccountMismatch(t *testing.T) {
	p, service, _, _ := vaultPlatform(t)
	ctx := context.Background()
	signInFixture(t, p)
	payload := vault.Payload{AccessKey: "AK_FIXTURE", SecretKey: "SK_FIXTURE"}
	if err := service.Save(ctx, []byte("correct horse battery"), payload, "account-other", "cn-beijing"); err != nil {
		t.Fatal(err)
	}
	if result := p.SignOut(); !result.OK {
		t.Fatal(result.Error)
	}
	result := p.UnlockVault(ctx, dto.VaultPassphraseRequest{Passphrase: "correct horse battery"})
	if result.OK || result.Error == nil || result.Error.Code != "vaultAccountMismatch" {
		t.Fatalf("account mismatch accepted: %+v", result)
	}
	if p.Cloud.State().SignedIn {
		t.Fatal("mismatched account remained signed in")
	}
}

func TestVaultStatusProjectsNoSecrets(t *testing.T) {
	p, _, _, _ := vaultPlatform(t)
	signInFixture(t, p)
	if result := p.SaveVault(context.Background(), dto.VaultPassphraseRequest{Passphrase: "correct horse battery"}); !result.OK {
		t.Fatal(result.Error)
	}
	status := p.VaultStatus()
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"AK_FIXTURE", "SK_FIXTURE", "correct horse battery"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("status leaked %q", secret)
		}
	}
}

func TestBytePlusVaultPreservesProviderAndLegacyPayloadStillUnlocks(t *testing.T) {
	p, service, _, _ := vaultPlatform(t)
	ctx := context.Background()
	state := p.SignIn(ctx, dto.SignInRequest{Provider: "byteplus", Region: "ap-southeast-1", AccessKey: "AK_FIXTURE", SecretKey: "SK_FIXTURE"})
	if !state.OK {
		t.Fatal(state.Error)
	}
	pass := dto.VaultPassphraseRequest{Passphrase: "correct horse battery"}
	if saved := p.SaveVault(ctx, pass); !saved.OK {
		t.Fatal(saved.Error)
	}
	p.SignOut()
	restored := p.UnlockVault(ctx, pass)
	if !restored.OK || restored.Value.Provider != "byteplus" || restored.Value.Principal.Scope != state.Value.Principal.Scope {
		t.Fatalf("provider lost: %+v", restored)
	}
	// Existing vaults have no provider property in their encrypted payload.
	if err := service.Save(ctx, []byte(pass.Passphrase), vault.Payload{AccessKey: "AK_FIXTURE", SecretKey: "SK_FIXTURE"}, "account-one", "cn-beijing"); err != nil {
		t.Fatal(err)
	}
	p.SignOut()
	legacy := p.UnlockVault(ctx, pass)
	if !legacy.OK || legacy.Value.Provider != "volcengine" || legacy.Value.Principal.Scope != "account-one/User/user-one" {
		t.Fatalf("legacy vault broken: %+v", legacy)
	}
}
