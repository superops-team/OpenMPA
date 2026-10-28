package usage

import (
	"context"
	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

// Cloud operations use the host's bound target, never a renderer-supplied region or Runtime.
func (s *Service) cloudTarget(epoch uint64) (deployment.Target, *dto.SafeError) {
	if s.conn == nil || !s.conn.Current(epoch) {
		return deployment.Target{}, dto.Failure("staleConnection", "transport", "reload", 0)
	}
	if s.cloud == nil || !s.cloud.State().SignedIn {
		return deployment.Target{}, dto.Failure("authRequired", "platform", "reauthenticate", 0)
	}
	if s.target == nil {
		return deployment.Target{}, dto.Failure("capabilityUnavailable", "platform", "readOnly", 0)
	}
	target := s.target()
	if target.ConnectionEpoch != epoch || target.PlatformEpoch != s.cloud.Epoch() || target.RuntimeID != s.conn.State().RuntimeID || !cloud.ValidRegion(target.Region) {
		return target, dto.Failure("staleConnection", "platform", "reload", 0)
	}
	return target, nil
}

func (s *Service) cloudAction(ctx context.Context, epoch uint64, action string, input, output any, write bool) *dto.SafeError {
	target, err := s.cloudTarget(epoch)
	if err != nil {
		return err
	}
	scope, err := s.conn.ScopeContext(epoch)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(scope, cancel)
	defer func() { stop(); cancel() }()
	err = s.cloud.Action(ctx, target.Region, action, input, output)
	if stale := s.resourceEpochError(epoch); stale != nil {
		return stale
	}
	if s.cloud.Epoch() != target.PlatformEpoch {
		return dto.Failure("staleConnection", "platform", "reload", 0)
	}
	if err != nil && write && (err.Code == "invalidResponse" || err.Code == "missingResult" || err.Code == "connectionFailure" || err.HTTPStatus >= 500) {
		return dto.Failure("outcomeUnknown", "platform", "verifyOnly", err.HTTPStatus)
	}
	return err
}
