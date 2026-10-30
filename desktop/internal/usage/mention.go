package usage

import (
	"context"
	"unicode/utf8"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

// CollaborationMention rechecks persisted metadata and returns text, never a run.
func (b *Bridge) CollaborationMention(ctx context.Context, r CollaborationMentionRequest) MentionResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	text, err := b.checkedMention(ctx, r)
	return MentionResult{err == nil, only(&MentionText{text}, err), err}
}

func (b *Bridge) checkedMention(ctx context.Context, r CollaborationMentionRequest) (string, *dto.SafeError) {
	scope := b.core.conn.State().Scope
	if !utf8.ValidString(r.Task) || len(r.Task) > 64<<10 {
		return "", resourceInputError()
	}
	before, err := b.core.CollaborationBinding(ctx, r.ConnectionEpoch)
	if err != nil {
		return "", err
	}
	if before.SpaceID == nil || *before.SpaceID != r.SpaceID || before.Revision != r.BindingRevision {
		return "", dto.Failure("conflict", "runtimeIngress", "reload", 0)
	}
	members, err := b.core.CollaborationAgents(ctx, r.ConnectionEpoch, r.SpaceID, r.Page)
	if err != nil {
		return "", err
	}
	after, err := b.core.CollaborationBinding(ctx, r.ConnectionEpoch)
	if err != nil {
		return "", err
	}
	if before.Revision != after.Revision || before.Region != after.Region || !collaborationSameString(before.SpaceID, after.SpaceID) {
		return "", dto.Failure("conflict", "runtimeIngress", "reload", 0)
	}
	for _, member := range members.Items {
		if member.ID != r.AgentID {
			continue
		}
		if member.AliasRevision != r.AliasRevision || member.RuntimeID == b.core.conn.State().RuntimeID {
			return "", dto.Failure("conflict", "runtimeIngress", "reload", 0)
		}
		if !b.core.conn.Current(r.ConnectionEpoch) || b.core.conn.State().Scope != scope {
			return "", dto.Failure("staleConnection", "transport", "reload", 0)
		}
		return CollaborationMention(member, r.Task)
	}
	return "", dto.Failure("resourceNotVisible", "runtimeIngress", "reload", 0)
}
