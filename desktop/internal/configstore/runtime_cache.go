package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

const runtimeListCacheTTL = 24 * time.Hour

func runtimeListScope(principalScope, region, project string) (string, error) {
	if !validPrincipalScope(principalScope) || !validID(region, 64) || !validID(project, 128) {
		return "", errors.New("invalid Runtime list cache")
	}
	return principalScope + "/" + region + "/" + project, nil
}

func (s *Store) ensureRuntimeListCache(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS runtime_list_cache (
        scope TEXT PRIMARY KEY NOT NULL CHECK(length(scope) BETWEEN 1 AND 1024),
        payload TEXT NOT NULL CHECK(length(payload) <= 1048576),
        updated_at INTEGER NOT NULL
    ) STRICT`)
	return err
}

func (s *Store) SaveRuntimeListCache(ctx context.Context, principalScope string, request dto.RuntimeListRequest, list dto.RuntimeList) error {
	scope, err := runtimeListScope(principalScope, request.Region, request.ProjectName)
	if err != nil {
		return err
	}
	data, err := json.Marshal(list)
	if err != nil || len(data) == 0 || len(data) > 1<<20 {
		return errors.New("invalid Runtime list cache")
	}
	if err := s.ensureRuntimeListCache(ctx); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO runtime_list_cache(scope, payload, updated_at)
        VALUES(?, ?, ?)
        ON CONFLICT(scope) DO UPDATE SET payload=excluded.payload, updated_at=excluded.updated_at`,
		scope, string(data), time.Now().UnixMilli())
	return err
}

func (s *Store) LoadRuntimeListCache(ctx context.Context, principalScope string, request dto.RuntimeListRequest, maxAge time.Duration) (dto.RuntimeList, bool, error) {
	scope, err := runtimeListScope(principalScope, request.Region, request.ProjectName)
	if err != nil {
		return dto.RuntimeList{}, false, err
	}
	if err := s.ensureRuntimeListCache(ctx); err != nil {
		return dto.RuntimeList{}, false, err
	}
	var payload string
	var updatedAt int64
	err = s.db.QueryRowContext(ctx, "SELECT payload, updated_at FROM runtime_list_cache WHERE scope = ?", scope).Scan(&payload, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.RuntimeList{}, false, nil
		}
		return dto.RuntimeList{}, false, err
	}
	if maxAge > 0 && time.Since(time.UnixMilli(updatedAt)) > maxAge {
		return dto.RuntimeList{}, false, nil
	}
	var out dto.RuntimeList
	if json.Unmarshal([]byte(payload), &out) != nil {
		return dto.RuntimeList{}, false, errors.New("invalid Runtime list cache")
	}
	return out, true, nil
}

func RuntimeListCacheTTL() time.Duration { return runtimeListCacheTTL }
