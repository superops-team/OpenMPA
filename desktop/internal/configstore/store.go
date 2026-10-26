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
	LanguageKey       = "display.language"
	RegionKey         = "platform.region"
	ProjectKey        = "platform.project"
	RuntimeKey        = "platform.runtime_id"
	PrincipalScopeKey = "platform.principal_scope"
	RuntimeBindingKey = "platform.runtime_binding"

	DefaultLanguage = "system"
	DefaultRegion   = "cn-beijing"
	DefaultProject  = "default"

	SchemaVersion = 2
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

type Preferences struct {
	Language       string
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
	if _, err := tx.ExecContext(ctx, `CREATE TABLE credential_vault (
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
	if _, err := tx.ExecContext(ctx, "PRAGMA user_version = 2"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Path() string { return s.path }

func (s *Store) Snapshot(ctx context.Context) (Preferences, error) {
	out := Preferences{Language: DefaultLanguage, Region: DefaultRegion, Project: DefaultProject}
	rows, err := s.db.QueryContext(ctx, "SELECT key, value FROM settings WHERE key IN (?, ?, ?, ?, ?, ?)", LanguageKey, RegionKey, ProjectKey, RuntimeKey, PrincipalScopeKey, RuntimeBindingKey)
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
