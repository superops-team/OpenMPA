package usage

import (
	"context"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func only[T any](value *T, err *dto.SafeError) *T {
	if err != nil {
		return nil
	}
	return value
}

func deadline(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 30*time.Second)
}

func (b *Bridge) ReadAgentsMD(ctx context.Context, r dto.ScopeRequest) AgentsMDResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.ReadAgentsMD(ctx, r.ConnectionEpoch)
	return AgentsMDResult{err == nil, only(&out, err), err}
}
func (b *Bridge) SaveAgentsMD(ctx context.Context, r AgentsMDSaveRequest) AgentsMDResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.SaveAgentsMD(ctx, r.ConnectionEpoch, r.Document)
	return AgentsMDResult{err == nil, only(&out, err), err}
}
func (b *Bridge) ListMCPServers(ctx context.Context, r dto.SessionRequest) MCPDirectoryResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.ListMCPServers(ctx, r.ConnectionEpoch, r.SessionID)
	return MCPDirectoryResult{err == nil, only(&out, err), err}
}
func (b *Bridge) WorkerMCPServers(ctx context.Context, r dto.ScopeRequest) MCPDirectoryResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.WorkerMCPServers(ctx, r.ConnectionEpoch)
	return MCPDirectoryResult{err == nil, only(&out, err), err}
}
func (b *Bridge) DiscoverSkillSpaces(ctx context.Context, r dto.ScopeRequest) SkillMountsResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.DiscoverSkillSpaces(ctx, r.ConnectionEpoch)
	return SkillMountsResult{err == nil, only(&out, err), err}
}
func (b *Bridge) ListSkills(ctx context.Context, r SkillListRequest) SkillPageResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.ListSkills(ctx, r.ConnectionEpoch, r.Region, r.SpaceID, r.PageNumber, r.PageSize)
	return SkillPageResult{err == nil, only(&out, err), err}
}
func (b *Bridge) BoundSkillSpaces(ctx context.Context, r dto.ScopeRequest) SkillMountsResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.BoundSkillSpaces(ctx, r.ConnectionEpoch)
	return SkillMountsResult{err == nil, only(&out, err), err}
}
func (b *Bridge) ListWorkerSkills(ctx context.Context, r SkillListRequest) SkillPageResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.WorkerSkills(ctx, r.ConnectionEpoch, r.Region, r.SpaceID, r.PageNumber, r.PageSize)
	return SkillPageResult{err == nil, only(&out, err), err}
}
func (b *Bridge) ResourceInstanceID(ctx context.Context, r dto.ScopeRequest) ResourceIdentityResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	id, err := b.core.ResourceInstanceID(ctx, r.ConnectionEpoch)
	return ResourceIdentityResult{err == nil, only(&ResourceIdentity{id}, err), err}
}
func (b *Bridge) profileResult(ctx context.Context, scope string, state MainProfileState, err *dto.SafeError) MainProfileResult {
	status := state.Status
	if status == "" {
		status = MainProfileUnavailable
	}
	if err != nil {
		return MainProfileResult{Status: status, Error: err}
	}
	if state.snapshot == nil || state.snapshot.scope != scope || b.core.conn.State().Scope != scope {
		return MainProfileResult{Status: MainProfileUnavailable, Error: dto.Failure("staleConnection", "transport", "reload", 0)}
	}
	token, err := b.retain(ctx, state)
	if err != nil {
		return MainProfileResult{Status: MainProfileUnavailable, Error: err}
	}
	view := &MainProfileView{state.Status, state.Record, state.ConnectionEpoch, token}
	return MainProfileResult{OK: true, Status: state.Status, Value: view}
}
func (b *Bridge) ReadMainProfile(ctx context.Context, r MainProfileReadRequest) MainProfileResult {
	scope := b.core.conn.State().Scope
	requestCtx, cancel := deadline(ctx)
	defer cancel()
	id := r.MPAInstanceID
	if id == "" {
		var err *dto.SafeError
		id, err = b.core.ResourceInstanceID(requestCtx, r.ConnectionEpoch)
		if err != nil {
			return MainProfileResult{Status: MainProfileUnavailable, Error: err}
		}
	}
	state, err := b.core.ReadMainProfile(requestCtx, r.ConnectionEpoch, id)
	return b.profileResult(ctx, scope, state, err)
}
func (b *Bridge) SaveMainProfileInstructions(ctx context.Context, r MainProfileSaveRequest) MainProfileResult {
	scope := b.core.conn.State().Scope
	state, err := b.claim(ctx, r.ConnectionEpoch, r.SnapshotID)
	if err != nil {
		return MainProfileResult{Status: MainProfileUnavailable, Error: err}
	}
	requestCtx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.SaveMainProfileInstructions(requestCtx, r.ConnectionEpoch, state, r.Instructions)
	return b.profileResult(ctx, scope, out, err)
}
func (b *Bridge) CreateMainProfile(ctx context.Context, r MainProfileCreateRequest) MainProfileResult {
	scope := b.core.conn.State().Scope
	requestCtx, cancel := deadline(ctx)
	defer cancel()
	id, err := b.core.ResourceInstanceID(requestCtx, r.ConnectionEpoch)
	if err != nil {
		return MainProfileResult{Status: MainProfileUnavailable, Error: err}
	}
	out, err := b.core.CreateMainProfile(requestCtx, r.ConnectionEpoch, id, r.Profile)
	return b.profileResult(ctx, scope, out, err)
}
func (b *Bridge) SaveMainProfile(ctx context.Context, r MainProfileUpdateRequest) MainProfileResult {
	scope := b.core.conn.State().Scope
	state, err := b.claim(ctx, r.ConnectionEpoch, r.SnapshotID)
	if err != nil {
		return MainProfileResult{Status: MainProfileUnavailable, Error: err}
	}
	requestCtx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.SaveMainProfile(requestCtx, r.ConnectionEpoch, state, r.Profile)
	return b.profileResult(ctx, scope, out, err)
}
func (b *Bridge) CollaborationSpaces(ctx context.Context, r CollaborationPageRequest) CollaborationSpacePageResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.CollaborationSpaces(ctx, r.ConnectionEpoch, r.Page)
	return CollaborationSpacePageResult{err == nil, only(out, err), err}
}
func (b *Bridge) CollaborationAgents(ctx context.Context, r CollaborationAgentsRequest) CollaborationAgentPageResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.CollaborationAgents(ctx, r.ConnectionEpoch, r.SpaceID, r.Page)
	return CollaborationAgentPageResult{err == nil, only(out, err), err}
}
func (b *Bridge) CollaborationBinding(ctx context.Context, r dto.ScopeRequest) CollaborationBindingResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.CollaborationBinding(ctx, r.ConnectionEpoch)
	return CollaborationBindingResult{err == nil, only(out, err), err}
}
func (b *Bridge) SetCollaborationBinding(ctx context.Context, r CollaborationBindingSaveRequest) CollaborationBindingResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.SetCollaborationBinding(ctx, r.ConnectionEpoch, r.SpaceID, r.Revision)
	return CollaborationBindingResult{err == nil, only(out, err), err}
}
func (b *Bridge) SetCollaborationAlias(ctx context.Context, r CollaborationAliasSaveRequest) CollaborationAliasResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.SetCollaborationAlias(ctx, r.ConnectionEpoch, r.SpaceID, r.AgentID, r.Alias, r.Revision)
	return CollaborationAliasResult{err == nil, only(out, err), err}
}
