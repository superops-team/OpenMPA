package configstore

import (
	"context"
	"database/sql"
	"errors"
	"github.com/egoist/mygo"
	"os"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "openmpa.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestDefaultsAndPersistence(t *testing.T) {
	store := openTest(t)
	snapshot, err := store.Snapshot(context.Background())
	if err != nil || snapshot.Language != SystemValue() || snapshot.Region != DefaultRegion || snapshot.Project != DefaultProject || snapshot.Runtime != "" {
		t.Fatal(snapshot, err)
	}
	if err := store.SetLanguage(context.Background(), "en"); err != nil {
		t.Fatal(err)
	}
	path := store.Path()
	store.Close()
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	snapshot, err = reopened.Snapshot(context.Background())
	if err != nil || snapshot.Language != "en" {
		t.Fatal(snapshot, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
}

func SystemValue() string { return DefaultLanguage }

func TestDirectoryChangeAtomicallyInvalidatesRuntime(t *testing.T) {
	store := openTest(t)
	ctx := context.Background()
	if err := store.SetRuntime(ctx, "account/User/principal", "cn-beijing", "default", "r-one"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetDirectory(ctx, "cn-shanghai", "default"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(ctx)
	if err != nil || snapshot.Region != "cn-shanghai" || snapshot.Runtime != "" || snapshot.PrincipalScope != "" {
		t.Fatal(snapshot, err)
	}
}

func TestRuntimePreferencePersistsPrincipalScopeAtomically(t *testing.T) {
	store := openTest(t)
	ctx := context.Background()
	if err := store.SetRuntime(ctx, "account/User/principal", "cn-beijing", "default", "r-one"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(ctx)
	if err != nil || snapshot.PrincipalScope != "account/User/principal" || snapshot.Region != "cn-beijing" || snapshot.Project != "default" || snapshot.Runtime != "r-one" {
		t.Fatalf("Runtime preference tuple was not stored atomically: %+v %v", snapshot, err)
	}
}

func TestLegacyRuntimeRewriteCannotReuseStalePrincipalScope(t *testing.T) {
	store := openTest(t)
	ctx := context.Background()
	if err := store.SetRuntime(ctx, "account-one/User/principal-one", "cn-beijing", "default", "r-one"); err != nil {
		t.Fatal(err)
	}
	// Simulate v0.1.0, which only knew the Runtime ID key.
	if err := store.put(ctx, RuntimeKey, "r-two"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(ctx)
	if err != nil || snapshot.Runtime != "r-two" || snapshot.PrincipalScope != "" {
		t.Fatalf("legacy rewrite reused an unrelated principal scope: %+v %v", snapshot, err)
	}
}

func TestClearRuntimePreservesCommittedDirectory(t *testing.T) {
	store := openTest(t)
	ctx := context.Background()
	if err := store.SetRuntime(ctx, "account/User/principal", "cn-beijing", "project-one", "r-one"); err != nil {
		t.Fatal(err)
	}
	if err := store.ClearRuntime(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(ctx)
	if err != nil || snapshot.Region != "cn-beijing" || snapshot.Project != "project-one" || snapshot.Runtime != "" || snapshot.PrincipalScope != "" {
		t.Fatalf("clearing Runtime changed the committed directory: %+v %v", snapshot, err)
	}
}

func TestWindowStateStoreRoundTrip(t *testing.T) {
	store := openTest(t)
	want := map[string]mygo.WindowState{"main": {X: -100, Y: 40, Width: 1280, Height: 850, Maximized: true}}
	if err := store.SaveWindowStates(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadWindowStates()
	if err != nil || got["main"] != want["main"] {
		t.Fatal(got, err)
	}
}

func TestSettingsRejectSecretsAndUnknownSchema(t *testing.T) {
	store := openTest(t)
	if err := store.SetDirectory(context.Background(), "https://host?token=SECRET", "default"); err == nil {
		t.Fatal("credential-bearing region accepted")
	}
	if err := store.SetRuntime(context.Background(), "account/User/principal", "cn-beijing", "default", "SECRET\nvalue"); err == nil {
		t.Fatal("unsafe Runtime ID accepted")
	}
	if err := store.SetRuntime(context.Background(), "account/User/principal/extra", "cn-beijing", "default", "r-one"); err == nil {
		t.Fatal("invalid principal scope accepted")
	}
	path := store.Path()
	if _, err := store.db.Exec("PRAGMA user_version = 3"); err != nil {
		t.Fatal(err)
	}
	store.Close()
	if _, err := Open(path); err == nil {
		t.Fatal("newer database accepted")
	}
}

func testVaultRecord(account string) VaultRecord {
	return VaultRecord{
		Version:    1,
		KDF:        "pbkdf2-sha256",
		Iterations: 100000,
		Salt:       make([]byte, 16),
		Nonce:      make([]byte, 12),
		Ciphertext: []byte("ciphertext-only"),
		AccountID:  account,
		Region:     "cn-beijing",
		UpdatedAt:  1700000000000,
	}
}

func TestVaultRecordRoundTripAndReplacement(t *testing.T) {
	store := openTest(t)
	ctx := context.Background()
	if _, err := store.LoadVault(ctx); !errors.Is(err, ErrNoVault) {
		t.Fatalf("expected missing vault, got %v", err)
	}
	if err := store.SaveVault(ctx, testVaultRecord("account-1")); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadVault(ctx)
	if err != nil || loaded.AccountID != "account-1" || string(loaded.Ciphertext) != "ciphertext-only" {
		t.Fatalf("bad load: %+v %v", loaded, err)
	}
	if err := store.SaveVault(ctx, testVaultRecord("account-2")); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadVault(ctx)
	if err != nil || loaded.AccountID != "account-2" {
		t.Fatalf("replacement failed: %+v %v", loaded, err)
	}
	var rows int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM credential_vault").Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("vault table is not a singleton: rows=%d err=%v", rows, err)
	}
	if err := store.DeleteVault(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadVault(ctx); !errors.Is(err, ErrNoVault) {
		t.Fatalf("vault survived delete: %v", err)
	}
}

func TestVaultRecordValidation(t *testing.T) {
	store := openTest(t)
	ctx := context.Background()
	cases := []func(*VaultRecord){
		func(r *VaultRecord) { r.Salt = make([]byte, 15) },
		func(r *VaultRecord) { r.Nonce = make([]byte, 11) },
		func(r *VaultRecord) { r.Iterations = 99999 },
		func(r *VaultRecord) { r.Ciphertext = nil },
		func(r *VaultRecord) { r.AccountID = "account/1" },
		func(r *VaultRecord) { r.Region = "cn-beijing\n" },
	}
	for index, mutate := range cases {
		record := testVaultRecord("account-1")
		mutate(&record)
		if err := store.SaveVault(ctx, record); err == nil {
			t.Fatalf("case %d accepted an invalid record", index)
		}
	}
}

func TestVaultTableMigratesFromVersionOne(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openmpa.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetLanguage(context.Background(), "en"); err != nil {
		t.Fatal(err)
	}
	store.Close()

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("DROP TABLE credential_vault"); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 1"); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	snapshot, err := reopened.Snapshot(context.Background())
	if err != nil || snapshot.Language != "en" {
		t.Fatalf("settings lost during migration: %+v %v", snapshot, err)
	}
	var version int
	if err := reopened.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("schema version = %d, err=%v", version, err)
	}
	if _, err := reopened.LoadVault(context.Background()); !errors.Is(err, ErrNoVault) {
		t.Fatalf("migrated vault table not empty: %v", err)
	}
}
