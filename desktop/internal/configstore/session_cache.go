package configstore

import (
	"context"
	"errors"
	"strings"
	"time"
)

const sessionListCacheTTL = 24 * time.Hour

func validCacheScope(scope string) bool {
	return scope != "" && len(scope) <= 512 && !strings.ContainsAny(scope, "\x00\r\n")
}

func (s *Store) ensureSessionCache(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS session_list_cache (
        scope TEXT PRIMARY KEY NOT NULL CHECK(length(scope) BETWEEN 1 AND 512),
        payload TEXT NOT NULL CHECK(length(payload) <= 1048576),
        updated_at INTEGER NOT NULL
    ) STRICT`)
	return err
}

func (s *Store) SaveSessionListCache(ctx context.Context, scope string, payload []byte) error {
	if !validCacheScope(scope) || len(payload) == 0 || len(payload) > 1<<20 {
		return errors.New("invalid session list cache")
	}
	if err := s.ensureSessionCache(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO session_list_cache(scope, payload, updated_at)
        VALUES(?, ?, ?)
        ON CONFLICT(scope) DO UPDATE SET payload=excluded.payload, updated_at=excluded.updated_at`,
		scope, string(payload), time.Now().UnixMilli())
	return err
}

func (s *Store) LoadSessionListCache(ctx context.Context, scope string, maxAge time.Duration) ([]byte, bool, error) {
	if !validCacheScope(scope) {
		return nil, false, errors.New("invalid session list cache")
	}
	if err := s.ensureSessionCache(ctx); err != nil {
		return nil, false, err
	}
	var payload string
	var updatedAt int64
	err := s.db.QueryRowContext(ctx, "SELECT payload, updated_at FROM session_list_cache WHERE scope = ?", scope).Scan(&payload, &updatedAt)
	if err != nil {
		return nil, false, err
	}
	if maxAge > 0 && time.Since(time.UnixMilli(updatedAt)) > maxAge {
		return nil, false, nil
	}
	return []byte(payload), true, nil
}

func SessionListCacheTTL() time.Duration { return sessionListCacheTTL }
