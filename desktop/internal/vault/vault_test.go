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

const testIterations = MinIterations

func testService(t *testing.T) *Service {
	t.Helper()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "openmpa.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return NewWithIterations(store, testIterations)
}

func testPayload() Payload {
	return Payload{AccessKey: "AK_TEST_VALUE", SecretKey: "SK_TEST_VALUE", SecurityToken: "STS_TEST_VALUE"}
}

func TestSaveStatusUnlockAndForget(t *testing.T) {
	service := testService(t)
	ctx := context.Background()
	status, err := service.Status(ctx)
	if err != nil || status.Exists {
		t.Fatalf("unexpected initial status: %+v %v", status, err)
	}
	if err := service.Save(ctx, []byte("correct horse battery"), testPayload(), "account-1", "cn-beijing"); err != nil {
		t.Fatal(err)
	}
	status, err = service.Status(ctx)
	if err != nil || !status.Exists || status.AccountID != "account-1" || status.Region != "cn-beijing" || status.UpdatedAt == 0 {
		t.Fatalf("bad status: %+v %v", status, err)
	}
	payload, record, err := service.Unlock(ctx, []byte("correct horse battery"))
	if err != nil {
		t.Fatal(err)
	}
	if payload != testPayload() || record.AccountID != "account-1" || record.Region != "cn-beijing" {
		t.Fatalf("unexpected unlock result: %+v %+v", payload, record)
	}
	if err := service.Forget(ctx); err != nil {
		t.Fatal(err)
	}
	status, err = service.Status(ctx)
	if err != nil || status.Exists {
		t.Fatalf("vault survived Forget: %+v %v", status, err)
	}
	if _, _, err := service.Unlock(ctx, []byte("correct horse battery")); !errors.Is(err, ErrMissing) {
		t.Fatalf("expected missing vault, got %v", err)
	}
}

func TestWrongPassphraseAndTamperingFailClosed(t *testing.T) {
	service := testService(t)
	ctx := context.Background()
	if err := service.Save(ctx, []byte("correct horse battery"), testPayload(), "account-1", "cn-beijing"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Unlock(ctx, []byte("wrong horse battery")); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("wrong passphrase accepted: %v", err)
	}
	record, err := service.store.LoadVault(ctx)
	if err != nil {
		t.Fatal(err)
	}
	record.Ciphertext[0] ^= 1
	if _, err := open([]byte("correct horse battery"), record); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("tampered ciphertext accepted: %v", err)
	}
	record, _ = service.store.LoadVault(ctx)
	record.AccountID = "account-2"
	if _, err := open([]byte("correct horse battery"), record); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("swapped metadata accepted: %v", err)
	}
	record, _ = service.store.LoadVault(ctx)
	record.Iterations = MinIterations - 1
	if _, err := open([]byte("correct horse battery"), record); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("weak KDF record accepted: %v", err)
	}
	record, _ = service.store.LoadVault(ctx)
	record.KDF = "md5"
	if _, err := open([]byte("correct horse battery"), record); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("unknown KDF accepted: %v", err)
	}
}

func TestValidationRejectsWeakInputs(t *testing.T) {
	service := testService(t)
	ctx := context.Background()
	for _, passphrase := range []string{"short", "        "} {
		if err := service.Save(ctx, []byte(passphrase), testPayload(), "account-1", "cn-beijing"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("weak passphrase accepted: %q %v", passphrase, err)
		}
	}
	bad := testPayload()
	bad.SecretKey = ""
	if err := service.Save(ctx, []byte("correct horse battery"), bad, "account-1", "cn-beijing"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing secret accepted: %v", err)
	}
	if err := service.Save(ctx, []byte("correct horse battery"), testPayload(), "account/1", "cn-beijing"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unsafe account accepted: %v", err)
	}
	weak := NewWithIterations(service.store, MinIterations-1)
	if err := weak.Save(ctx, []byte("correct horse battery"), testPayload(), "account-1", "cn-beijing"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("weak KDF parameters accepted: %v", err)
	}
}

func TestStoredRecordContainsNoPlaintext(t *testing.T) {
	service := testService(t)
	ctx := context.Background()
	if err := service.Save(ctx, []byte("correct horse battery"), testPayload(), "account-1", "cn-beijing"); err != nil {
		t.Fatal(err)
	}
	record, err := service.store.LoadVault(ctx)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"AK_TEST_VALUE", "SK_TEST_VALUE", "STS_TEST_VALUE", "correct horse battery"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("plaintext leaked into stored record: %s", secret)
		}
	}
}
