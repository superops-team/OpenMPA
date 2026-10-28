package usage

import "github.com/superops-team/OpenMPA/desktop/internal/dto"

type AgentsMDSaveRequest struct {
	ConnectionEpoch uint64           `json:"connectionEpoch"`
	Document        AgentsMDDocument `json:"document"`
}
type SkillListRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	Region          string `json:"region"`
	SpaceID         string `json:"spaceId"`
	PageNumber      int    `json:"pageNumber"`
	PageSize        int    `json:"pageSize"`
}
type MainProfileReadRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	MPAInstanceID   string `json:"mpaInstanceId"`
}
type MainProfileSaveRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SnapshotID      string `json:"snapshotId"`
	Instructions    string `json:"instructions"`
}
type MainProfileView struct {
	Status          MainProfileStatus  `json:"status"`
	Record          *MainProfileRecord `json:"record,omitempty"`
	ConnectionEpoch uint64             `json:"connectionEpoch"`
	SnapshotID      string             `json:"snapshotId,omitempty"`
}
type CollaborationPageRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	Page            int    `json:"page"`
}
type CollaborationAgentsRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SpaceID         string `json:"spaceId"`
	Page            int    `json:"page"`
}
type CollaborationBindingSaveRequest struct {
	ConnectionEpoch uint64  `json:"connectionEpoch"`
	SpaceID         *string `json:"spaceId"`
	Revision        int64   `json:"revision"`
}
type CollaborationAliasSaveRequest struct {
	ConnectionEpoch uint64  `json:"connectionEpoch"`
	SpaceID         string  `json:"spaceId"`
	AgentID         string  `json:"agentId"`
	Alias           *string `json:"alias"`
	Revision        int64   `json:"revision"`
}
type CollaborationMentionRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SpaceID         string `json:"spaceId"`
	BindingRevision int64  `json:"bindingRevision"`
	AgentID         string `json:"agentId"`
	AliasRevision   int64  `json:"aliasRevision"`
	Page            int    `json:"page"`
	Task            string `json:"task"`
}
type AgentsMDResult struct {
	OK    bool              `json:"ok"`
	Value *AgentsMDDocument `json:"value,omitempty"`
	Error *dto.SafeError    `json:"error,omitempty"`
}
type MCPDirectoryResult struct {
	OK    bool           `json:"ok"`
	Value *MCPDirectory  `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type SkillMountsResult struct {
	OK    bool           `json:"ok"`
	Value *SkillMounts   `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type SkillPageResult struct {
	OK    bool           `json:"ok"`
	Value *SkillPage     `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type ResourceIdentity struct {
	MPAInstanceID string `json:"mpaInstanceId"`
}
type ResourceIdentityResult struct {
	OK    bool              `json:"ok"`
	Value *ResourceIdentity `json:"value,omitempty"`
	Error *dto.SafeError    `json:"error,omitempty"`
}
type MainProfileResult struct {
	OK     bool              `json:"ok"`
	Status MainProfileStatus `json:"status"`
	Value  *MainProfileView  `json:"value,omitempty"`
	Error  *dto.SafeError    `json:"error,omitempty"`
}
type CollaborationSpacePageResult struct {
	OK    bool                    `json:"ok"`
	Value *CollaborationSpacePage `json:"value,omitempty"`
	Error *dto.SafeError          `json:"error,omitempty"`
}
type CollaborationAgentPageResult struct {
	OK    bool                    `json:"ok"`
	Value *CollaborationAgentPage `json:"value,omitempty"`
	Error *dto.SafeError          `json:"error,omitempty"`
}
type CollaborationBindingResult struct {
	OK    bool                  `json:"ok"`
	Value *CollaborationBinding `json:"value,omitempty"`
	Error *dto.SafeError        `json:"error,omitempty"`
}
type CollaborationAliasResult struct {
	OK    bool                `json:"ok"`
	Value *CollaborationAlias `json:"value,omitempty"`
	Error *dto.SafeError      `json:"error,omitempty"`
}
type MentionText struct {
	Text string `json:"text"`
}
type MentionResult struct {
	OK    bool           `json:"ok"`
	Value *MentionText   `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
