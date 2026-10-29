package configstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestSessionListCacheRoundTripAndExpiry(t *testing.T) {
	store := openTest(t)
	ctx := context.Background()
	payload := []byte(`{"sessions":[{"id":"s","title":"Cached"}]}`)
	if err := store.SaveSessionListCache(ctx, "scope", payload); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.LoadSessionListCache(ctx, "scope", time.Hour)
	if err != nil || !ok || string(got) != string(payload) {
		t.Fatalf("cache miss: ok=%v err=%v got=%s", ok, err, string(got))
	}
	time.Sleep(2 * time.Millisecond)
	expired, ok, err := store.LoadSessionListCache(ctx, "scope", time.Nanosecond)
	if err != nil || ok || expired != nil {
		t.Fatalf("expired cache returned: ok=%v err=%v got=%s", ok, err, string(expired))
	}
}

func TestAssistantConfigRoundTripAndValidation(t *testing.T) {
	store := openTest(t)
	ctx := context.Background()
	empty, ok, err := store.LoadAssistantConfig(ctx, "scope", "assistant-a")
	if err != nil || ok || empty.AssistantID != "assistant-a" || len(empty.Skills) != 0 {
		t.Fatalf("empty config mismatch ok=%v config=%+v err=%v", ok, empty, err)
	}
	saved, err := store.SaveAssistantConfig(ctx, "scope", dto.AssistantConfig{
		AssistantID: "assistant-a",
		AgentsMD:    "You are careful.",
		Skills: []dto.AssistantSkill{{
			ID:          "skill-a",
			Name:        "skill-a",
			Description: "Local skill",
			Source:      "local",
			Content:     "Use skill-a.",
		}},
	})
	if err != nil || saved.UpdatedAt == "" {
		t.Fatalf("save failed %+v %v", saved, err)
	}
	loaded, ok, err := store.LoadAssistantConfig(ctx, "scope", "assistant-a")
	if err != nil || !ok || loaded.AgentsMD != "You are careful." || len(loaded.Skills) != 1 || loaded.Skills[0].ID != "skill-a" || loaded.UpdatedAt == "" {
		t.Fatalf("load failed ok=%v config=%+v err=%v", ok, loaded, err)
	}
	if _, err := store.SaveAssistantConfig(ctx, "scope", dto.AssistantConfig{AssistantID: "assistant-a", AgentsMD: strings.Repeat("x", 32001)}); err == nil {
		t.Fatal("oversized AGENTS.md accepted")
	}
	if _, err := store.SaveAssistantConfig(ctx, "scope", dto.AssistantConfig{AssistantID: "assistant-a", Skills: []dto.AssistantSkill{{ID: "dup", Name: "A", Source: "local"}, {ID: "dup", Name: "B", Source: "local"}}}); err == nil {
		t.Fatal("duplicate skill accepted")
	}
}

func TestRuntimeListCacheRoundTripAndExpiry(t *testing.T) {
	store := openTest(t)
	ctx := context.Background()
	request := dto.RuntimeListRequest{Region: "cn-beijing", ProjectName: "default"}
	list := dto.RuntimeList{Complete: true, Runtimes: []dto.RuntimeSummary{{
		RuntimeID: "r-one",
		Name:      "Runtime one",
		Status:    "Ready",
	}}}
	if err := store.SaveRuntimeListCache(ctx, "account/User/principal", request, list); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.LoadRuntimeListCache(ctx, "account/User/principal", request, time.Hour)
	if err != nil || !ok || len(got.Runtimes) != 1 || got.Runtimes[0].RuntimeID != "r-one" {
		t.Fatalf("cache miss ok=%v list=%+v err=%v", ok, got, err)
	}
	time.Sleep(2 * time.Millisecond)
	expired, ok, err := store.LoadRuntimeListCache(ctx, "account/User/principal", request, time.Nanosecond)
	if err != nil || ok || len(expired.Runtimes) != 0 {
		t.Fatalf("expired cache returned ok=%v list=%+v err=%v", ok, expired, err)
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
	if _, err := store.db.Exec(fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion+1)); err != nil {
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

func testUserTokenVaultRecord(runtimeID string) UserTokenVaultRecord {
	return UserTokenVaultRecord{
		RuntimeID:  runtimeID,
		Version:    1,
		KDF:        "pbkdf2-sha256",
		Iterations: 100000,
		Salt:       make([]byte, 16),
		Nonce:      make([]byte, 12),
		Ciphertext: []byte("ciphertext-only"),
		UserPoolID: DefaultAgentIdentityUserPoolID,
		Issuer:     "https://issuer.example",
		ClientID:   "client-1",
		Subject:    "user-1",
		MPAID:      "mi-1",
		SpaceID:    "space-1",
		ExpiresAt:  "2026-10-29T12:00:00Z",
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

func TestUserTokenVaultRecordRoundTripAndReplacement(t *testing.T) {
	store := openTest(t)
	ctx := context.Background()
	if _, err := store.LoadUserTokenVault(ctx, "runtime-1"); !errors.Is(err, ErrNoVault) {
		t.Fatalf("expected missing user token vault, got %v", err)
	}
	if err := store.SaveUserTokenVault(ctx, testUserTokenVaultRecord("runtime-1")); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadUserTokenVault(ctx, "runtime-1")
	if err != nil || loaded.Subject != "user-1" || string(loaded.Ciphertext) != "ciphertext-only" {
		t.Fatalf("bad user token vault load: %+v %v", loaded, err)
	}
	replacement := testUserTokenVaultRecord("runtime-1")
	replacement.Subject = "user-2"
	if err := store.SaveUserTokenVault(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadUserTokenVault(ctx, "runtime-1")
	if err != nil || loaded.Subject != "user-2" {
		t.Fatalf("user token vault replacement failed: %+v %v", loaded, err)
	}
	if err := store.DeleteUserTokenVault(ctx, "runtime-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadUserTokenVault(ctx, "runtime-1"); !errors.Is(err, ErrNoVault) {
		t.Fatalf("user token vault survived delete: %v", err)
	}
}

func TestAgentIdentityUserPoolRoundTrip(t *testing.T) {
	store := openTest(t)
	ctx := context.Background()
	if got, err := store.LoadAgentIdentityUserPool(ctx); err != nil || got != "" {
		t.Fatalf("unexpected initial user pool: %q %v", got, err)
	}
	if err := store.SaveAgentIdentityUserPool(ctx, DefaultAgentIdentityUserPoolID); err != nil {
		t.Fatal(err)
	}
	if got, err := store.LoadAgentIdentityUserPool(ctx); err != nil || got != DefaultAgentIdentityUserPoolID {
		t.Fatalf("user pool did not persist: %q %v", got, err)
	}
	if err := store.SaveAgentIdentityUserPool(ctx, "not-a-user-pool"); err == nil {
		t.Fatal("invalid user pool accepted")
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
	if _, err := raw.Exec("DROP TABLE user_token_vault"); err != nil {
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
	if _, err := reopened.LoadUserTokenVault(context.Background(), "runtime-1"); !errors.Is(err, ErrNoVault) {
		t.Fatalf("migrated user token vault table not empty: %v", err)
	}
}

func TestUserTokenVaultTableMigratesFromVersionThree(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openmpa.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("ALTER TABLE user_token_vault DROP COLUMN user_pool_id"); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 3"); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	record := testUserTokenVaultRecord("runtime-1")
	if err := reopened.SaveUserTokenVault(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.LoadUserTokenVault(context.Background(), "runtime-1")
	if err != nil || loaded.UserPoolID != DefaultAgentIdentityUserPoolID {
		t.Fatalf("migrated user pool column unavailable: %+v %v", loaded, err)
	}
}

func TestUserTokenVaultTableMigratesFromVersionTwo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openmpa.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("DROP TABLE user_token_vault"); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 2"); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var version int
	if err := reopened.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("schema version = %d, err=%v", version, err)
	}
	if _, err := reopened.LoadUserTokenVault(context.Background(), "runtime-1"); !errors.Is(err, ErrNoVault) {
		t.Fatalf("migrated user token vault table not empty: %v", err)
	}
}

func TestThemePreferencePersists(t *testing.T) {
	store := openTest(t)
	snapshot, err := store.Snapshot(context.Background())
	if err != nil || snapshot.Theme != DefaultTheme {
		t.Fatalf("default theme = %+v, err=%v", snapshot, err)
	}
	if err := store.SetTheme(context.Background(), "dark"); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Snapshot(context.Background())
	if err != nil || snapshot.Theme != "dark" {
		t.Fatalf("saved theme = %+v, err=%v", snapshot, err)
	}
	if err := store.SetTheme(context.Background(), "sepia"); err == nil {
		t.Fatal("invalid theme accepted")
	}
}
