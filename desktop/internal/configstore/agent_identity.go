package configstore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

func NormalizeAgentIdentityUserPoolID(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if !validAgentIdentityUserPoolID(normalized) {
		return "", errors.New("invalid Agent Identity user pool")
	}
	return normalized, nil
}

func validAgentIdentityUserPoolID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
			continue
		}
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func (s *Store) SaveAgentIdentityUserPool(ctx context.Context, userPoolID string) error {
	normalized, err := NormalizeAgentIdentityUserPoolID(userPoolID)
	if err != nil {
		return err
	}
	return s.put(ctx, AgentIdentityUserPoolKey, normalized)
}

func (s *Store) LoadAgentIdentityUserPool(ctx context.Context) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", AgentIdentityUserPoolKey).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return NormalizeAgentIdentityUserPoolID(value)
}
