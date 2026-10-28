package usage

import (
	"context"
	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"strings"
)

// WorkerSkills reads managed SkillSpace relations first. An empty or missing
// relation directory also checks the Runtime consumer directory by bound space.
func (s *Service) WorkerSkills(ctx context.Context, epoch uint64, region, spaceID string, page, pageSize int) (SkillPage, *dto.SafeError) {
	empty := SkillPage{}
	if !cloud.ValidRegion(region) || !resourceIdentifierValid(spaceID) || page < 1 || page > 10000 || pageSize < 1 || pageSize > 100 {
		return empty, resourceInputError()
	}
	if s.cloud == nil || !s.cloud.State().SignedIn {
		return empty, dto.Failure("authRequired", "platform", "reauthenticate", 0)
	}
	platformEpoch := s.cloud.Epoch()
	if s.target != nil {
		target, err := s.cloudTarget(epoch)
		if err != nil {
			return empty, err
		}
		if target.Region != region {
			return empty, resourceInputError()
		}
	}
	mounts, err := s.DiscoverSkillSpaces(ctx, epoch)
	if err != nil {
		return empty, err
	}
	mounted := false
	for _, space := range mounts.Spaces {
		mounted = mounted || space.ID == spaceID
	}
	if !mounted {
		return empty, dto.Failure("resourceNotVisible", "runtimeIngress", "reload", 0)
	}
	scope, err := s.conn.ScopeContext(epoch)
	if err != nil {
		return empty, err
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(scope, cancel)
	defer func() { stop(); cancel() }()
	action := func(name string, input, output any) *dto.SafeError {
		if s.cloud.Epoch() != platformEpoch {
			return dto.Failure("staleConnection", "platform", "reload", 0)
		}
		err := s.cloud.Action(ctx, region, name, input, output)
		if stale := s.resourceEpochError(epoch); stale != nil {
			return stale
		}
		if s.cloud.Epoch() != platformEpoch {
			return dto.Failure("staleConnection", "platform", "reload", 0)
		}
		return err
	}
	var relations struct {
		Items []struct {
			Name        string `json:"SkillName"`
			ID          string `json:"SkillId"`
			Version     string `json:"Version"`
			Status      string `json:"SkillStatus"`
			Description string `json:"SkillDescription"`
		} `json:"Items"`
		Total *int `json:"TotalCount"`
	}
	err = action("ListSkillsBySkillSpace", map[string]any{"SkillSpaceId": spaceID, "PageNumber": page, "PageSize": pageSize}, &relations)
	if err == nil {
		if relations.Items == nil || relations.Total == nil || *relations.Total < 0 || len(relations.Items) > pageSize {
			return empty, resourceResponseError(false)
		}
		items := []Skill{}
		seen := map[string]bool{}
		for _, item := range relations.Items {
			if strings.TrimSpace(item.Name) == "" || seen[item.Name] {
				return empty, resourceResponseError(false)
			}
			seen[item.Name] = true
			items = append(items, Skill{Name: item.Name, Description: item.Description, ID: item.ID, Version: item.Version, Status: item.Status})
		}
		if *relations.Total > 0 || len(items) > 0 {
			return SkillPage{SpaceID: spaceID, Region: region, Items: items, TotalCount: *relations.Total, PageNumber: page, PageSize: pageSize, HasMore: page*pageSize < *relations.Total, ServerPaginated: true, ConnectionEpoch: epoch, PlatformEpoch: platformEpoch}, nil
		}
	}
	if err != nil && err.HTTPStatus != 404 && err.Code != "resourceNotVisible" {
		return empty, err
	}
	var space struct {
		Name string `json:"Name"`
		ID   string `json:"Id"`
	}
	if err = action("GetSkillSpace", map[string]string{"Id": spaceID}, &space); err != nil {
		return empty, err
	}
	if space.ID != spaceID || strings.TrimSpace(space.Name) == "" {
		return empty, resourceResponseError(false)
	}
	var legacy struct {
		Items []struct {
			Name        string `json:"Name"`
			Description string `json:"Description"`
		} `json:"Items"`
	}
	if err = action("ListSkillsBySpaceId", map[string]string{"SkillSpaceId": spaceID, "SkillSpaceName": space.Name}, &legacy); err != nil {
		return empty, err
	}
	if legacy.Items == nil {
		return empty, resourceResponseError(false)
	}
	items := []Skill{}
	seen := map[string]bool{}
	for _, basic := range legacy.Items {
		if strings.TrimSpace(basic.Name) == "" || seen[basic.Name] {
			return empty, resourceResponseError(false)
		}
		seen[basic.Name] = true
		var info struct {
			Name        string `json:"SkillName"`
			ID          string `json:"SkillId"`
			Version     string `json:"Version"`
			Status      string `json:"SkillStatus"`
			Description string `json:"Description"`
		}
		if err = action("GetSkillInfo", map[string]string{"SkillName": basic.Name, "SkillSpaceName": space.Name, "SkillSpaceId": spaceID}, &info); err != nil {
			if err.HTTPStatus == 404 || err.Code == "resourceNotVisible" {
				continue
			}
			return empty, err
		}
		if info.Name == "" {
			info.Name = basic.Name
		}
		if info.Description == "" {
			info.Description = basic.Description
		}
		items = append(items, Skill{Name: info.Name, Description: info.Description, ID: info.ID, Version: info.Version, Status: info.Status})
	}
	start := min((page-1)*pageSize, len(items))
	end := min(start+pageSize, len(items))
	return SkillPage{SpaceID: spaceID, Region: region, Items: items[start:end], TotalCount: len(items), PageNumber: page, PageSize: pageSize, HasMore: end < len(items), ConnectionEpoch: epoch, PlatformEpoch: platformEpoch}, nil
}
