package usage

import (
	"context"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"strings"
)

func (s *Service) BoundSkillSpaces(ctx context.Context, epoch uint64) (SkillMounts, *dto.SafeError) {
	mounts, err := s.DiscoverSkillSpaces(ctx, epoch)
	if err != nil {
		return mounts, err
	}
	for i, space := range mounts.Spaces {
		var detail struct {
			ID          string `json:"Id"`
			Name        string `json:"Name"`
			Description string `json:"Description"`
			ProjectName string `json:"ProjectName"`
		}
		if err = s.cloudAction(ctx, epoch, "GetSkillSpace", map[string]string{"Id": space.ID}, &detail, false); err != nil {
			return SkillMounts{}, err
		}
		if detail.ID != space.ID || strings.TrimSpace(detail.Name) == "" {
			return SkillMounts{}, resourceResponseError(false)
		}
		mounts.Spaces[i].Name = detail.Name
		mounts.Spaces[i].Description = detail.Description
		mounts.Spaces[i].ProjectName = detail.ProjectName
	}
	return mounts, nil
}
func (s *Service) boundSkillSpace(ctx context.Context, epoch uint64, id string) (SkillSpace, *dto.SafeError) {
	if !resourceIdentifierValid(id) {
		return SkillSpace{}, resourceInputError()
	}
	mounts, err := s.BoundSkillSpaces(ctx, epoch)
	if err != nil {
		return SkillSpace{}, err
	}
	for _, space := range mounts.Spaces {
		if space.ID == id {
			return space, nil
		}
	}
	return SkillSpace{}, dto.Failure("resourceNotVisible", "runtimeIngress", "reload", 0)
}
