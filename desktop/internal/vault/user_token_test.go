package vault

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/configstore"
)

func TestUserTokenSaveStatusUnlockAndForget(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "openmpa.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	service := NewWithIterations(store, testIterations)
	ctx := context.Background()
	token := UserToken{
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
	if err := service.SaveUserToken(ctx, []byte("correct horse battery"), token); err != nil {
		t.Fatal(err)
	}
	status, err := service.UserTokenStatus(ctx, token.RuntimeID)
	if err != nil || !status.Exists || status.Subject != "user-1" || status.ExpiresAt != token.ExpiresAt {
		t.Fatalf("unsafe or incorrect status: %+v %v", status, err)
	}
	unlocked, err := service.UnlockUserToken(ctx, []byte("correct horse battery"), token.RuntimeID)
	if err != nil || unlocked != token {
		t.Fatalf("unlock failed: %+v %v", unlocked, err)
	}
	if err := service.ForgetUserToken(ctx, token.RuntimeID); err != nil {
		t.Fatal(err)
	}
	status, err = service.UserTokenStatus(ctx, token.RuntimeID)
	if err != nil || status.Exists {
		t.Fatalf("token survived forget: %+v %v", status, err)
	}
}

func TestUserTokenStoredRecordContainsNoPlaintext(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "openmpa.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	service := NewWithIterations(store, testIterations)
	token := UserToken{
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
	if err := service.SaveUserToken(context.Background(), []byte("correct horse battery"), token); err != nil {
		t.Fatal(err)
	}
	record, err := store.LoadUserTokenVault(context.Background(), token.RuntimeID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"id-token-secret", "refresh-token-secret", "correct horse battery"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("plaintext leaked into token vault record: %s", secret)
		}
	}
}

func TestUserTokenWrongPassphraseAndTamperingFailClosed(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "openmpa.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	service := NewWithIterations(store, testIterations)
	ctx := context.Background()
	token := UserToken{
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
	if err := service.SaveUserToken(ctx, []byte("correct horse battery"), token); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UnlockUserToken(ctx, []byte("wrong horse battery"), token.RuntimeID); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("wrong passphrase accepted: %v", err)
	}
	record, err := store.LoadUserTokenVault(ctx, token.RuntimeID)
	if err != nil {
		t.Fatal(err)
	}
	record.Subject = "user-2"
	if _, err := openUserToken([]byte("correct horse battery"), record); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("tampered metadata accepted: %v", err)
	}
	record, err = store.LoadUserTokenVault(ctx, token.RuntimeID)
	if err != nil {
		t.Fatal(err)
	}
	record.UserPoolID = "00000000-0000-0000-0000-000000000000"
	if _, err := openUserToken([]byte("correct horse battery"), record); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("tampered user pool accepted: %v", err)
	}
}
