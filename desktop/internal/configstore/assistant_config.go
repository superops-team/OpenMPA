package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

const (
	assistantConfigAgentsLimit  = 32000
	assistantConfigPayloadLimit = 1 << 20
)

type AssistantConfigRecord struct {
	Scope       string
	AssistantID string
	Config      dto.AssistantConfig
	UpdatedAt   int64
}

func validAssistantID(value string) bool {
	return value != "" && len(value) <= 256 && !strings.ContainsAny(value, "\x00\r\n/?#") && value != "." && value != ".."
}

func validateAssistantConfig(config dto.AssistantConfig) error {
	if !validAssistantID(config.AssistantID) {
		return errors.New("invalid assistant config")
	}
	if len([]rune(config.AgentsMD)) > assistantConfigAgentsLimit {
		return errors.New("invalid assistant config")
	}
	if len(config.Skills) > 64 {
		return errors.New("invalid assistant config")
	}
	seen := map[string]bool{}
	for _, skill := range config.Skills {
		if skill.ID == "" || len(skill.ID) > 256 || strings.ContainsAny(skill.ID, "\x00\r\n") {
			return errors.New("invalid assistant config")
		}
		if seen[skill.ID] {
			return errors.New("invalid assistant config")
		}
		seen[skill.ID] = true
		if len(skill.Name) > 256 || len(skill.Description) > 2048 || len(skill.Source) > 64 || len(skill.SpaceID) > 256 || len(skill.Content) > 262144 {
			return errors.New("invalid assistant config")
		}
		if skill.Source == "" || strings.ContainsAny(skill.Source, "\x00\r\n") || strings.ContainsAny(skill.SpaceID, "\x00\r\n") {
			return errors.New("invalid assistant config")
		}
	}
	return nil
}

func (s *Store) ensureAssistantConfig(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS assistant_configs (
        scope TEXT NOT NULL CHECK(length(scope) BETWEEN 1 AND 512),
        assistant_id TEXT NOT NULL CHECK(length(assistant_id) BETWEEN 1 AND 256),
        payload TEXT NOT NULL CHECK(length(payload) <= 1048576),
        updated_at INTEGER NOT NULL,
        PRIMARY KEY(scope, assistant_id)
    ) STRICT`)
	return err
}

func (s *Store) SaveAssistantConfig(ctx context.Context, scope string, config dto.AssistantConfig) (dto.AssistantConfig, error) {
	if !validCacheScope(scope) {
		return dto.AssistantConfig{}, errors.New("invalid assistant config")
	}
	if err := validateAssistantConfig(config); err != nil {
		return dto.AssistantConfig{}, err
	}
	config.UpdatedAt = ""
	payload, err := json.Marshal(config)
	if err != nil || len(payload) == 0 || len(payload) > assistantConfigPayloadLimit {
		return dto.AssistantConfig{}, errors.New("invalid assistant config")
	}
	if err := s.ensureAssistantConfig(ctx); err != nil {
		return dto.AssistantConfig{}, err
	}
	now := time.Now().UnixMilli()
	_, err = s.db.ExecContext(ctx, `INSERT INTO assistant_configs(scope, assistant_id, payload, updated_at)
        VALUES(?, ?, ?, ?)
        ON CONFLICT(scope, assistant_id) DO UPDATE SET payload=excluded.payload, updated_at=excluded.updated_at`,
		scope, config.AssistantID, string(payload), now)
	if err != nil {
		return dto.AssistantConfig{}, err
	}
	config.UpdatedAt = time.UnixMilli(now).UTC().Format(time.RFC3339)
	return config, nil
}

func (s *Store) LoadAssistantConfig(ctx context.Context, scope, assistantID string) (dto.AssistantConfig, bool, error) {
	if !validCacheScope(scope) || !validAssistantID(assistantID) {
		return dto.AssistantConfig{}, false, errors.New("invalid assistant config")
	}
	if err := s.ensureAssistantConfig(ctx); err != nil {
		return dto.AssistantConfig{}, false, err
	}
	var payload string
	var updatedAt int64
	err := s.db.QueryRowContext(ctx, "SELECT payload, updated_at FROM assistant_configs WHERE scope = ? AND assistant_id = ?", scope, assistantID).Scan(&payload, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.AssistantConfig{AssistantID: assistantID, Skills: []dto.AssistantSkill{}}, false, nil
		}
		return dto.AssistantConfig{}, false, err
	}
	var config dto.AssistantConfig
	if json.Unmarshal([]byte(payload), &config) != nil {
		return dto.AssistantConfig{}, false, errors.New("invalid assistant config")
	}
	if config.AssistantID == "" {
		config.AssistantID = assistantID
	}
	if config.Skills == nil {
		config.Skills = []dto.AssistantSkill{}
	}
	if err := validateAssistantConfig(config); err != nil {
		return dto.AssistantConfig{}, false, err
	}
	config.UpdatedAt = time.UnixMilli(updatedAt).UTC().Format(time.RFC3339)
	return config, true, nil
}
