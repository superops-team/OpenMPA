package usage

import (
	"context"
	"net/http"
	"strings"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

// ResourceInstanceID reads the current Runtime's identity, not its platform ID.
// This is the same verified metadata route used by Session.Create.
func (s *Service) ResourceInstanceID(ctx context.Context, epoch uint64) (string, *dto.SafeError) {
	var wire struct {
		Agent struct {
			ID string `json:"id"`
		} `json:"agent"`
	}
	if err := s.request(ctx, epoch, http.MethodGet, "/api/v1/runtime-console/overview", nil, &wire); err != nil {
		return "", err
	}
	if !resourceIdentifierValid(wire.Agent.ID) || strings.HasPrefix(strings.ToLower(wire.Agent.ID), "r-") {
		return "", resourceResponseError(false)
	}
	if err := s.resourceEpochError(epoch); err != nil {
		return "", err
	}
	return wire.Agent.ID, nil
}
