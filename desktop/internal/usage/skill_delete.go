package usage

import (
	"context"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type SkillDeleteRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SpaceID         string `json:"spaceId"`
	SkillID         string `json:"skillId"`
}

// DeleteSkill uses Studio's whole-skill deletion contract. Verify that the
// requested skill belongs to a space mounted by the current Runtime first.
func (s *Service) DeleteSkill(ctx context.Context, r SkillDeleteRequest) *dto.SafeError {
	if !resourceIdentifierValid(r.SpaceID) || !resourceIdentifierValid(r.SkillID) {
		return resourceInputError()
	}
	target, err := s.cloudTarget(r.ConnectionEpoch)
	if err != nil {
		return err
	}
	for page := 1; page <= 100; page++ {
		directory, err := s.WorkerSkills(ctx, r.ConnectionEpoch, target.Region, r.SpaceID, page, 100)
		if err != nil {
			return err
		}
		if directory.PlatformEpoch != target.PlatformEpoch || s.cloud.Epoch() != target.PlatformEpoch {
			return dto.Failure("staleConnection", "platform", "reload", 0)
		}
		for _, skill := range directory.Items {
			if skill.ID == r.SkillID {
				return s.cloudAction(ctx, r.ConnectionEpoch, "DeleteSkill", map[string]string{"Id": r.SkillID}, nil, true)
			}
		}
		if !directory.HasMore {
			return dto.Failure("resourceNotVisible", "platform", "reload", 0)
		}
	}
	return resourceResponseError(false)
}

func (b *Bridge) DeleteSkill(ctx context.Context, r SkillDeleteRequest) DeletedResourceResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	err := b.core.DeleteSkill(ctx, r)
	return DeletedResourceResult{err == nil, only(&DeletedResource{true}, err), err}
}
