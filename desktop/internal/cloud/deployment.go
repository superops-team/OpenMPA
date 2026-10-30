package cloud

import (
	"context"
	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"strconv"
)

type DeploymentProvider struct{ Client *Client }

func (p DeploymentProvider) PlatformEpoch() uint64 { return p.Client.Epoch() }
func (p DeploymentProvider) GetRuntime(ctx context.Context, region, project, id string) (deployment.RuntimeSnapshot, *dto.SafeError) {
	r, e := p.Client.GetRuntime(ctx, dto.RuntimeRequest{Region: region, ProjectName: project, RuntimeID: id})
	return deployment.RuntimeSnapshot{RuntimeID: r.RuntimeID, Version: strconv.Itoa(r.CurrentVersionNumber), Image: r.ArtifactURL, Status: r.Status, ToolID: r.ToolID}, e
}
func (p DeploymentProvider) GetTool(ctx context.Context, region, project, id string) (deployment.ToolSnapshot, *dto.SafeError) {
	var out struct {
		ToolID      string `json:"ToolId"`
		ImageURL    string `json:"ImageUrl"`
		Status      string `json:"Status"`
		UpdatedAt   string `json:"UpdatedAt"`
		ProjectName string `json:"ProjectName"`
	}
	e := p.Client.Action(ctx, region, "GetTool", struct {
		ToolID string `json:"ToolId"`
	}{id}, &out)
	if e == nil && (out.ToolID != id || (out.ProjectName != "" && out.ProjectName != project)) {
		e = dto.Failure("scopeMismatch", "platform", "never", 200)
	}
	return deployment.ToolSnapshot{ToolID: out.ToolID, Image: out.ImageURL, Status: out.Status, UpdatedAt: out.UpdatedAt}, e
}
func (p DeploymentProvider) UpdateRuntime(ctx context.Context, region string, input deployment.UpdateRuntimeInput) *dto.SafeError {
	return p.Client.Action(ctx, region, "UpdateRuntime", input, nil)
}
func (p DeploymentProvider) UpdateTool(ctx context.Context, region string, input deployment.UpdateToolInput) *dto.SafeError {
	return p.Client.Action(ctx, region, "UpdateTool", input, nil)
}
