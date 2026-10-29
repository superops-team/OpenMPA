// Package configstore persists non-sensitive desktop settings in SQLite.
package configstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"github.com/egoist/mygo"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	LanguageKey              = "display.language"
	ThemeKey                 = "display.theme"
	RegionKey                = "platform.region"
	ProjectKey               = "platform.project"
	RuntimeKey               = "platform.runtime_id"
	PrincipalScopeKey        = "platform.principal_scope"
	RuntimeBindingKey        = "platform.runtime_binding"
	AgentIdentityUserPoolKey = "identity.user_pool_id"

	DefaultLanguage                = "system"
	DefaultTheme                   = "system"
	DefaultRegion                  = "cn-beijing"
	DefaultProject                 = "default"
	DefaultAgentIdentityUserPoolID = "3ea6c536-152c-435d-8040-9fcb3597476a"

	SchemaVersion = 4
)

// ErrNoVault reports that no encrypted credential record is stored.
var ErrNoVault = errors.New("no saved credentials")

// VaultRecord is ciphertext plus KDF parameters. It never contains plaintext
// credentials; callers own encryption and validation of the payload.
type VaultRecord struct {
	Version    int
	KDF        string
	Iterations int
	Salt       []byte
	Nonce      []byte
	Ciphertext []byte
	AccountID  string
	Region     string
	UpdatedAt  int64
}

type UserTokenVaultRecord struct {
	RuntimeID  string
	Version    int
	KDF        string
	Iterations int
	Salt       []byte
	Nonce      []byte
	Ciphertext []byte
	UserPoolID string
	Issuer     string
	ClientID   string
	Subject    string
	MPAID      string
	SpaceID    string
	ExpiresAt  string
	UpdatedAt  int64
}

type Preferences struct {
	Language       string
	Theme          string
	Region         string
	Project        string
	Runtime        string
	PrincipalScope string
}

type Store struct {
	db   *sql.DB
	path string
}

func Open(path string) (*Store, error) {
	if !filepath.IsAbs(path) || filepath.Base(path) != "openmpa.db" {
		return nil, errors.New("invalid config database path")
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("unsafe config database")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, path: path}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := store.configure(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) configure(ctx context.Context) error {
	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA synchronous = FULL",
		// Zero freed pages so a deleted vault record does not linger as
		// recoverable ciphertext in the database file.
		"PRAGMA secure_delete = ON",
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	var mode string
	if err := s.db.QueryRowContext(ctx, "PRAGMA journal_mode = WAL").Scan(&mode); err != nil || strings.ToLower(mode) != "wal" {
		return fmt.Errorf("sqlite WAL unavailable: %w", err)
	}
	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > SchemaVersion {
		return errors.New("config database is newer than this OpenMPA")
	}
	if version == SchemaVersion {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if version == 0 {
		for _, statement := range []string{
			`CREATE TABLE settings (
            key TEXT PRIMARY KEY NOT NULL CHECK(length(key) BETWEEN 1 AND 128),
            value TEXT NOT NULL CHECK(length(value) <= 4096),
            updated_at INTEGER NOT NULL
        ) STRICT`,
			`CREATE TABLE window_states (
            key TEXT PRIMARY KEY NOT NULL CHECK(length(key) BETWEEN 1 AND 128),
            x INTEGER NOT NULL, y INTEGER NOT NULL,
            width INTEGER NOT NULL CHECK(width > 0),
            height INTEGER NOT NULL CHECK(height > 0),
            maximized INTEGER NOT NULL CHECK(maximized IN (0, 1)),
            full_screen INTEGER NOT NULL CHECK(full_screen IN (0, 1)),
            updated_at INTEGER NOT NULL
        ) STRICT`,
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		now := time.Now().UnixMilli()
		for key, value := range map[string]string{LanguageKey: DefaultLanguage, RegionKey: DefaultRegion, ProjectKey: DefaultProject} {
			if _, err := tx.ExecContext(ctx, "INSERT INTO settings(key, value, updated_at) VALUES(?, ?, ?)", key, value, now); err != nil {
				return err
			}
		}
	}
	if version < 2 {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS credential_vault (
        id INTEGER PRIMARY KEY CHECK (id = 1),
        version INTEGER NOT NULL,
        kdf TEXT NOT NULL,
        iterations INTEGER NOT NULL,
        salt BLOB NOT NULL,
        nonce BLOB NOT NULL,
        ciphertext BLOB NOT NULL,
        account_id TEXT NOT NULL,
        region TEXT NOT NULL,
        updated_at INTEGER NOT NULL
    ) STRICT`); err != nil {
			return err
		}
	}
	if version < 3 {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS user_token_vault (
        runtime_id TEXT PRIMARY KEY NOT NULL CHECK(length(runtime_id) BETWEEN 1 AND 256),
        version INTEGER NOT NULL,
        kdf TEXT NOT NULL,
        iterations INTEGER NOT NULL,
        salt BLOB NOT NULL,
        nonce BLOB NOT NULL,
        ciphertext BLOB NOT NULL,
        user_pool_id TEXT NOT NULL,
        issuer TEXT NOT NULL,
        client_id TEXT NOT NULL,
        subject TEXT NOT NULL,
        mpa_id TEXT NOT NULL,
        space_id TEXT NOT NULL,
        expires_at TEXT NOT NULL,
        updated_at INTEGER NOT NULL
    ) STRICT`); err != nil {
			return err
		}
	}
	if version >= 3 && version < 4 {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE user_token_vault ADD COLUMN user_pool_id TEXT NOT NULL DEFAULT '`+DefaultAgentIdentityUserPoolID+`'`); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA user_version = 4"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Path() string { return s.path }

func (s *Store) Snapshot(ctx context.Context) (Preferences, error) {
	out := Preferences{Language: DefaultLanguage, Theme: DefaultTheme, Region: DefaultRegion, Project: DefaultProject}
	rows, err := s.db.QueryContext(ctx, "SELECT key, value FROM settings WHERE key IN (?, ?, ?, ?, ?, ?, ?)", LanguageKey, ThemeKey, RegionKey, ProjectKey, RuntimeKey, PrincipalScopeKey, RuntimeBindingKey)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	binding := ""
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return out, err
		}
		switch key {
		case LanguageKey:
			out.Language = value
		case ThemeKey:
			out.Theme = value
		case RegionKey:
			out.Region = value
		case ProjectKey:
			out.Project = value
		case RuntimeKey:
			out.Runtime = value
		case PrincipalScopeKey:
			out.PrincipalScope = value
		case RuntimeBindingKey:
			binding = value
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if binding == "" || binding != runtimeBindingFingerprint(out.PrincipalScope, out.Region, out.Project, out.Runtime) {
		out.PrincipalScope = ""
	}
	return out, nil
}

func (s *Store) SetLanguage(ctx context.Context, language string) error {
	if language != "system" && language != "zh-Hans" && language != "en" {
		return errors.New("invalid display language")
	}
	return s.put(ctx, LanguageKey, language)
}

func (s *Store) SetTheme(ctx context.Context, theme string) error {
	if theme != "system" && theme != "light" && theme != "dark" {
		return errors.New("invalid display theme")
	}
	return s.put(ctx, ThemeKey, theme)
}

func (s *Store) SetDirectory(ctx context.Context, region, project string) error {
	if !validID(region, 64) || !validID(project, 128) {
		return errors.New("invalid Runtime directory")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current := Preferences{Region: DefaultRegion, Project: DefaultProject}
	_ = tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", RegionKey).Scan(&current.Region)
	_ = tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", ProjectKey).Scan(&current.Project)
	if err := putTx(ctx, tx, RegionKey, region); err != nil {
		return err
	}
	if err := putTx(ctx, tx, ProjectKey, project); err != nil {
		return err
	}
	if current.Region != region || current.Project != project {
		if _, err := tx.ExecContext(ctx, "DELETE FROM settings WHERE key IN (?, ?, ?)", RuntimeKey, PrincipalScopeKey, RuntimeBindingKey); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetRuntime(ctx context.Context, principalScope, region, project, runtimeID string) error {
	if !validPrincipalScope(principalScope) || !validID(region, 64) || !validID(project, 128) || !validID(runtimeID, 256) {
		return errors.New("invalid Runtime selection")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for key, value := range map[string]string{PrincipalScopeKey: principalScope, RegionKey: region, ProjectKey: project, RuntimeKey: runtimeID, RuntimeBindingKey: runtimeBindingFingerprint(principalScope, region, project, runtimeID)} {
		if err := putTx(ctx, tx, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ClearRuntime removes the account-bound Runtime preference while preserving
// the last committed Region and Project. It is used as soon as a Project edit
// makes the existing connection scope stale; the new Project is committed only
// when its directory is refreshed successfully.
func (s *Store) ClearRuntime(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM settings WHERE key IN (?, ?, ?)", RuntimeKey, PrincipalScopeKey, RuntimeBindingKey)
	return err
}

func (s *Store) put(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings(key, value, updated_at) VALUES(?, ?, ?)
        ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, key, value, time.Now().UnixMilli())
	return err
}
func putTx(ctx context.Context, tx *sql.Tx, key, value string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value, updated_at) VALUES(?, ?, ?)
        ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, key, value, time.Now().UnixMilli())
	return err
}
func validID(value string, limit int) bool {
	return value != "" && len(value) <= limit && !strings.ContainsAny(value, "\x00\r\n/?#") && value != "." && value != ".."
}

func validPrincipalScope(value string) bool {
	parts := strings.Split(value, "/")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if !validID(part, 256) {
			return false
		}
	}
	return true
}

func runtimeBindingFingerprint(principalScope, region, project, runtimeID string) string {
	sum := sha256.Sum256([]byte(principalScope + "\x00" + region + "\x00" + project + "\x00" + runtimeID))
	return fmt.Sprintf("%x", sum)
}

func (s *Store) LoadWindowStates() (map[string]mygo.WindowState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, "SELECT key, x, y, width, height, maximized, full_screen FROM window_states")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]mygo.WindowState{}
	for rows.Next() {
		var key string
		var state mygo.WindowState
		if err := rows.Scan(&key, &state.X, &state.Y, &state.Width, &state.Height, &state.Maximized, &state.FullScreen); err != nil {
			return nil, err
		}
		out[key] = state
	}
	return out, rows.Err()
}

func (s *Store) SaveWindowStates(states map[string]mygo.WindowState) error {
	if len(states) > 100 {
		return errors.New("too many window states")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM window_states"); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for key, state := range states {
		if !validID(key, 128) || state.Width <= 0 || state.Height <= 0 || state.Width > 100000 || state.Height > 100000 {
			return errors.New("invalid window state")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO window_states(key, x, y, width, height, maximized, full_screen, updated_at)
            VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, key, state.X, state.Y, state.Width, state.Height, state.Maximized, state.FullScreen, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func validVaultRecord(r VaultRecord) bool {
	return r.Version >= 1 && r.Version <= 16 &&
		r.KDF != "" && len(r.KDF) <= 64 && !strings.ContainsAny(r.KDF, "\x00\r\n") &&
		r.Iterations >= 100000 && r.Iterations <= 2000000 &&
		len(r.Salt) == 16 && len(r.Nonce) == 12 &&
		len(r.Ciphertext) > 0 && len(r.Ciphertext) <= 8192 &&
		validID(r.AccountID, 128) && validID(r.Region, 64)
}

func validUserTokenVaultText(value string, limit int) bool {
	return value != "" && len(value) <= limit && !strings.ContainsAny(value, "\x00\r\n")
}

func validUserTokenVaultRecord(r UserTokenVaultRecord) bool {
	return validID(r.RuntimeID, 256) &&
		r.Version >= 1 && r.Version <= 16 &&
		r.KDF != "" && len(r.KDF) <= 64 && !strings.ContainsAny(r.KDF, "\x00\r\n") &&
		r.Iterations >= 100000 && r.Iterations <= 2000000 &&
		len(r.Salt) == 16 && len(r.Nonce) == 12 &&
		len(r.Ciphertext) > 0 && len(r.Ciphertext) <= 32768 &&
		validAgentIdentityUserPoolID(r.UserPoolID) &&
		validUserTokenVaultText(r.Issuer, 1024) &&
		validUserTokenVaultText(r.ClientID, 512) &&
		validUserTokenVaultText(r.Subject, 512) &&
		validUserTokenVaultText(r.MPAID, 256) &&
		validUserTokenVaultText(r.SpaceID, 256) &&
		validUserTokenVaultText(r.ExpiresAt, 64)
}

// SaveVault replaces the single encrypted credential record. The payload is
// already ciphertext; this store never sees plaintext credentials.
func (s *Store) SaveVault(ctx context.Context, r VaultRecord) error {
	if !validVaultRecord(r) {
		return errors.New("invalid credential vault record")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO credential_vault
        (id, version, kdf, iterations, salt, nonce, ciphertext, account_id, region, updated_at)
        VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(id) DO UPDATE SET
            version=excluded.version, kdf=excluded.kdf, iterations=excluded.iterations,
            salt=excluded.salt, nonce=excluded.nonce, ciphertext=excluded.ciphertext,
            account_id=excluded.account_id, region=excluded.region, updated_at=excluded.updated_at`,
		r.Version, r.KDF, r.Iterations, r.Salt, r.Nonce, r.Ciphertext, r.AccountID, r.Region, r.UpdatedAt)
	return err
}

// LoadVault returns the stored record or ErrNoVault.
func (s *Store) LoadVault(ctx context.Context) (VaultRecord, error) {
	var r VaultRecord
	err := s.db.QueryRowContext(ctx, `SELECT version, kdf, iterations, salt, nonce, ciphertext, account_id, region, updated_at
        FROM credential_vault WHERE id = 1`).Scan(
		&r.Version, &r.KDF, &r.Iterations, &r.Salt, &r.Nonce, &r.Ciphertext, &r.AccountID, &r.Region, &r.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return VaultRecord{}, ErrNoVault
	}
	if err != nil {
		return VaultRecord{}, err
	}
	if !validVaultRecord(r) {
		return VaultRecord{}, errors.New("invalid credential vault record")
	}
	return r, nil
}

func (s *Store) DeleteVault(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM credential_vault WHERE id = 1"); err != nil {
		return err
	}
	// Flush the WAL so the removed ciphertext does not stay in a side file.
	_, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

func (s *Store) SaveUserTokenVault(ctx context.Context, r UserTokenVaultRecord) error {
	if !validUserTokenVaultRecord(r) {
		return errors.New("invalid user token vault record")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO user_token_vault
        (runtime_id, version, kdf, iterations, salt, nonce, ciphertext, user_pool_id, issuer, client_id, subject, mpa_id, space_id, expires_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(runtime_id) DO UPDATE SET
            version=excluded.version, kdf=excluded.kdf, iterations=excluded.iterations,
            salt=excluded.salt, nonce=excluded.nonce, ciphertext=excluded.ciphertext,
            user_pool_id=excluded.user_pool_id, issuer=excluded.issuer, client_id=excluded.client_id, subject=excluded.subject,
            mpa_id=excluded.mpa_id, space_id=excluded.space_id, expires_at=excluded.expires_at,
            updated_at=excluded.updated_at`,
		r.RuntimeID, r.Version, r.KDF, r.Iterations, r.Salt, r.Nonce, r.Ciphertext,
		r.UserPoolID, r.Issuer, r.ClientID, r.Subject, r.MPAID, r.SpaceID, r.ExpiresAt, r.UpdatedAt)
	return err
}

func (s *Store) LoadUserTokenVault(ctx context.Context, runtimeID string) (UserTokenVaultRecord, error) {
	if !validID(runtimeID, 256) {
		return UserTokenVaultRecord{}, errors.New("invalid Runtime identity")
	}
	var r UserTokenVaultRecord
	err := s.db.QueryRowContext(ctx, `SELECT runtime_id, version, kdf, iterations, salt, nonce, ciphertext, user_pool_id, issuer, client_id, subject, mpa_id, space_id, expires_at, updated_at
        FROM user_token_vault WHERE runtime_id = ?`, runtimeID).Scan(
		&r.RuntimeID, &r.Version, &r.KDF, &r.Iterations, &r.Salt, &r.Nonce, &r.Ciphertext, &r.UserPoolID,
		&r.Issuer, &r.ClientID, &r.Subject, &r.MPAID, &r.SpaceID, &r.ExpiresAt, &r.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return UserTokenVaultRecord{}, ErrNoVault
	}
	if err != nil {
		return UserTokenVaultRecord{}, err
	}
	if !validUserTokenVaultRecord(r) {
		return UserTokenVaultRecord{}, errors.New("invalid user token vault record")
	}
	return r, nil
}

func (s *Store) DeleteUserTokenVault(ctx context.Context, runtimeID string) error {
	if !validID(runtimeID, 256) {
		return errors.New("invalid Runtime identity")
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM user_token_vault WHERE runtime_id = ?", runtimeID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}
