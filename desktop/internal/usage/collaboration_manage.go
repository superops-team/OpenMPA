package usage

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type SpaceCreateRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	Name            string `json:"name"`
	ProjectName     string `json:"projectName"`
	IntentEnabled   bool   `json:"intentEnabled"`
}
type AgentRegisterRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SpaceID         string `json:"spaceId"`
	ProjectName     string `json:"projectName"`
	Source          string `json:"source"`
	RuntimeID       string `json:"runtimeId"`
	NetworkType     string `json:"networkType"`
	ImportType      string `json:"importType"`
	CardURL         string `json:"cardUrl"`
	AgentCard       string `json:"agentCard"`
}
type CollaborationMemberRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SpaceID         string `json:"spaceId"`
	AgentID         string `json:"agentId"`
}
type CollaborationSpaceRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SpaceID         string `json:"spaceId"`
}
type CollaborationRuntimeRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	RuntimeID       string `json:"runtimeId"`
}
type CreatedResource struct {
	ID string `json:"id"`
}
type CreatedResourceResult struct {
	OK    bool             `json:"ok"`
	Value *CreatedResource `json:"value,omitempty"`
	Error *dto.SafeError   `json:"error,omitempty"`
}
type DeletedResource struct {
	Deleted bool `json:"deleted"`
}
type DeletedResourceResult struct {
	OK    bool             `json:"ok"`
	Value *DeletedResource `json:"value,omitempty"`
	Error *dto.SafeError   `json:"error,omitempty"`
}
type CollaborationEndpoint struct {
	NetworkType string `json:"networkType"`
	URL         string `json:"url"`
}
type CollaborationRuntimeOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type CollaborationRuntimeList struct {
	Items     []CollaborationRuntimeOption `json:"items"`
	NextToken string                       `json:"nextToken"`
}
type CollaborationRuntimeListRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	NextToken       string `json:"nextToken"`
}
type CollaborationRuntimeListResult struct {
	OK    bool                      `json:"ok"`
	Value *CollaborationRuntimeList `json:"value,omitempty"`
	Error *dto.SafeError            `json:"error,omitempty"`
}
type CollaborationEndpoints struct {
	RuntimeID string                  `json:"runtimeId"`
	Endpoints []CollaborationEndpoint `json:"endpoints"`
}
type CollaborationEndpointsResult struct {
	OK    bool                    `json:"ok"`
	Value *CollaborationEndpoints `json:"value,omitempty"`
	Error *dto.SafeError          `json:"error,omitempty"`
}
type CollaborationSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Examples    []string `json:"examples"`
}
type CollaborationDetail struct {
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	Status          string               `json:"status"`
	Source          string               `json:"source"`
	DefaultVersion  string               `json:"defaultVersion"`
	ProjectName     string               `json:"projectName"`
	RuntimeID       string               `json:"runtimeId"`
	NetworkType     string               `json:"networkType"`
	ImportType      string               `json:"importType"`
	ServiceAddress  string               `json:"serviceAddress"`
	Skills          []CollaborationSkill `json:"skills"`
	SkillsTruncated bool                 `json:"skillsTruncated"`
}
type CollaborationDetailResult struct {
	OK    bool                 `json:"ok"`
	Value *CollaborationDetail `json:"value,omitempty"`
	Error *dto.SafeError       `json:"error,omitempty"`
}

func collaborationText(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || utf8.RuneCountInString(value) > 128 {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func registrationURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 4096 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	for _, r := range raw {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func (s *Service) CreateCollaborationSpace(ctx context.Context, r SpaceCreateRequest) (CreatedResource, *dto.SafeError) {
	if !collaborationText(r.Name) || !collaborationText(r.ProjectName) {
		return CreatedResource{}, collaborationInputError()
	}
	var out struct {
		ID string `json:"Id"`
	}
	err := s.cloudAction(ctx, r.ConnectionEpoch, "CreateA2aSpace", map[string]any{"Name": r.Name, "ProjectName": r.ProjectName, "IntentEnabled": r.IntentEnabled}, &out, true)
	if err != nil {
		return CreatedResource{}, err
	}
	if !collaborationSpaceID.MatchString(out.ID) {
		return CreatedResource{}, collaborationResponseError(true)
	}
	return CreatedResource{out.ID}, nil
}
func (s *Service) RegisterCollaborationAgent(ctx context.Context, r AgentRegisterRequest) (CreatedResource, *dto.SafeError) {
	if !collaborationSpaceID.MatchString(r.SpaceID) || !collaborationText(r.ProjectName) {
		return CreatedResource{}, collaborationInputError()
	}
	payload := map[string]any{"A2aSpaceId": r.SpaceID, "Source": r.Source, "SetDefaultVersion": true, "ProjectName": r.ProjectName}
	switch r.Source {
	case "Runtime":
		if !collaborationRuntimeID.MatchString(r.RuntimeID) || (r.NetworkType != "public" && r.NetworkType != "private") || r.ImportType != "" || r.AgentCard != "" || r.CardURL != "" {
			return CreatedResource{}, collaborationInputError()
		}
		endpoints, err := s.CollaborationRuntimeEndpoints(ctx, CollaborationRuntimeRequest{r.ConnectionEpoch, r.RuntimeID})
		if err != nil {
			return CreatedResource{}, err
		}
		found := false
		for _, ep := range endpoints.Endpoints {
			found = found || ep.NetworkType == r.NetworkType
		}
		if !found {
			return CreatedResource{}, collaborationInputError()
		}
		payload["RuntimeConfig"] = map[string]string{"RuntimeId": r.RuntimeID, "NetworkType": r.NetworkType}
	case "Standard":
		if r.RuntimeID != "" || r.NetworkType != "" {
			return CreatedResource{}, collaborationInputError()
		}
		switch r.ImportType {
		case "url":
			if !registrationURL(r.CardURL) || r.AgentCard != "" {
				return CreatedResource{}, collaborationInputError()
			}
			payload["StandardConfig"] = map[string]string{"ImportType": "url", "AgentCardUrl": r.CardURL}
		case "json":
			var card map[string]any
			if len(r.AgentCard) > 128*1024 || r.CardURL != "" || json.Unmarshal([]byte(r.AgentCard), &card) != nil || card == nil {
				return CreatedResource{}, collaborationInputError()
			}
			for _, key := range []string{"name", "url", "version"} {
				v, ok := card[key].(string)
				if !ok || strings.TrimSpace(v) == "" {
					return CreatedResource{}, collaborationInputError()
				}
			}
			if !registrationURL(card["url"].(string)) {
				return CreatedResource{}, collaborationInputError()
			}
			payload["AgentCard"] = r.AgentCard
			payload["StandardConfig"] = map[string]string{"ImportType": "json"}
		default:
			return CreatedResource{}, collaborationInputError()
		}
	default:
		return CreatedResource{}, collaborationInputError()
	}
	var out struct {
		ID string `json:"Id"`
	}
	err := s.cloudAction(ctx, r.ConnectionEpoch, "CreateA2aAgent", payload, &out, true)
	if err != nil {
		return CreatedResource{}, err
	}
	if !collaborationAgentID.MatchString(out.ID) {
		return CreatedResource{}, collaborationResponseError(true)
	}
	return CreatedResource{out.ID}, nil
}

type collaborationDetailWire struct {
	ID             string          `json:"Id"`
	SpaceID        string          `json:"A2aSpaceId"`
	Name           string          `json:"Name"`
	Status         string          `json:"Status"`
	Source         string          `json:"Source"`
	DefaultVersion string          `json:"DefaultVersion"`
	ProjectName    string          `json:"ProjectName"`
	Card           json.RawMessage `json:"AgentCard"`
	RuntimeConfig  struct {
		RuntimeID   string `json:"RuntimeId"`
		NetworkType string `json:"NetworkType"`
	} `json:"RuntimeConfig"`
	StandardConfig struct {
		ImportType string `json:"ImportType"`
	} `json:"StandardConfig"`
}

func (s *Service) CollaborationAgentDetail(ctx context.Context, r CollaborationMemberRequest) (CollaborationDetail, *dto.SafeError) {
	if !collaborationSpaceID.MatchString(r.SpaceID) || !collaborationAgentID.MatchString(r.AgentID) {
		return CollaborationDetail{}, collaborationInputError()
	}
	var wire collaborationDetailWire
	if err := s.cloudAction(ctx, r.ConnectionEpoch, "GetA2aAgent", map[string]string{"Id": r.AgentID, "SpaceId": r.SpaceID}, &wire, false); err != nil {
		return CollaborationDetail{}, err
	}
	if wire.ID != r.AgentID || wire.SpaceID != r.SpaceID || (wire.Source != "Runtime" && wire.Source != "Standard") {
		return CollaborationDetail{}, collaborationResponseError(false)
	}
	for _, value := range []string{wire.Name, wire.Status, wire.DefaultVersion, wire.ProjectName, wire.RuntimeConfig.RuntimeID, wire.RuntimeConfig.NetworkType, wire.StandardConfig.ImportType} {
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 512 {
			return CollaborationDetail{}, collaborationResponseError(false)
		}
	}
	out := CollaborationDetail{ID: wire.ID, Name: wire.Name, Status: wire.Status, Source: wire.Source, DefaultVersion: wire.DefaultVersion, ProjectName: wire.ProjectName, RuntimeID: wire.RuntimeConfig.RuntimeID, NetworkType: wire.RuntimeConfig.NetworkType, ImportType: wire.StandardConfig.ImportType}
	raw := wire.Card
	var encoded string
	if json.Unmarshal(raw, &encoded) == nil {
		raw = json.RawMessage(encoded)
	}
	var card struct {
		URL    string               `json:"url"`
		Skills []CollaborationSkill `json:"skills"`
	}
	if len(raw) <= 128*1024 && json.Unmarshal(raw, &card) == nil {
		out.ServiceAddress = safeDisplayAddress(card.URL)
		if len(card.Skills) > 100 {
			out.SkillsTruncated = true
			card.Skills = card.Skills[:100]
		}
		if validCollaborationSkills(card.Skills) {
			out.Skills = card.Skills
		}
	}
	return out, nil
}

func validCollaborationSkills(skills []CollaborationSkill) bool {
	for _, skill := range skills {
		if utf8.RuneCountInString(skill.ID) > 256 || utf8.RuneCountInString(skill.Name) > 512 || utf8.RuneCountInString(skill.Description) > 4096 || len(skill.Tags) > 20 || len(skill.Examples) > 20 {
			return false
		}
		for _, tag := range skill.Tags {
			if utf8.RuneCountInString(tag) > 128 {
				return false
			}
		}
		for _, example := range skill.Examples {
			if utf8.RuneCountInString(example) > 1024 {
				return false
			}
		}
	}
	return true
}
func (s *Service) DeleteCollaborationAgent(ctx context.Context, r CollaborationMemberRequest) *dto.SafeError {
	if _, err := s.CollaborationAgentDetail(ctx, r); err != nil {
		return err
	}
	return s.cloudAction(ctx, r.ConnectionEpoch, "DeleteA2aAgent", map[string]string{"Id": r.AgentID}, nil, true)
}
func (s *Service) DeleteCollaborationSpace(ctx context.Context, r CollaborationSpaceRequest) *dto.SafeError {
	if !collaborationSpaceID.MatchString(r.SpaceID) {
		return collaborationInputError()
	}
	binding, err := s.CollaborationBinding(ctx, r.ConnectionEpoch)
	if err != nil {
		return err
	}
	if binding.SpaceID != nil && *binding.SpaceID == r.SpaceID {
		return dto.Failure("conflict", "runtimeIngress", "reload", 409)
	}
	var space struct {
		ID string `json:"Id"`
	}
	if err = s.cloudAction(ctx, r.ConnectionEpoch, "GetA2aSpace", map[string]string{"Id": r.SpaceID}, &space, false); err != nil {
		return err
	}
	if space.ID != r.SpaceID {
		return collaborationResponseError(false)
	}
	var members struct {
		Items []json.RawMessage `json:"Items"`
		Total *int              `json:"TotalCount"`
	}
	if err = s.cloudAction(ctx, r.ConnectionEpoch, "ListA2aAgents", map[string]any{"SpaceId": r.SpaceID, "PageNumber": 1, "PageSize": 1}, &members, false); err != nil {
		return err
	}
	if members.Items == nil || members.Total == nil || *members.Total < 0 {
		return collaborationResponseError(false)
	}
	if len(members.Items) > 0 || *members.Total > 0 {
		return dto.Failure("conflict", "platform", "reload", 409)
	}
	return s.cloudAction(ctx, r.ConnectionEpoch, "DeleteA2aSpace", map[string]string{"Id": r.SpaceID}, nil, true)
}
func (s *Service) CollaborationRuntimes(ctx context.Context, r CollaborationRuntimeListRequest) (CollaborationRuntimeList, *dto.SafeError) {
	out := CollaborationRuntimeList{Items: []CollaborationRuntimeOption{}}
	if len(r.NextToken) > 4096 {
		return out, collaborationInputError()
	}
	var page struct {
		Items     []cloud.RuntimeRecord `json:"AgentKitRuntimes"`
		NextToken string                `json:"NextToken"`
	}
	if err := s.cloudAction(ctx, r.ConnectionEpoch, "ListRuntimes", map[string]any{"MaxResults": 100, "NextToken": r.NextToken}, &page, false); err != nil {
		return out, err
	}
	if page.Items == nil {
		return out, collaborationResponseError(false)
	}
	for _, item := range page.Items {
		if !resourceIdentifierValid(item.RuntimeID) {
			return out, collaborationResponseError(false)
		}
		out.Items = append(out.Items, CollaborationRuntimeOption{item.RuntimeID, item.Name})
	}
	out.NextToken = page.NextToken
	return out, nil
}
func (s *Service) CollaborationRuntimeEndpoints(ctx context.Context, r CollaborationRuntimeRequest) (CollaborationEndpoints, *dto.SafeError) {
	out := CollaborationEndpoints{RuntimeID: r.RuntimeID, Endpoints: []CollaborationEndpoint{}}
	if !collaborationRuntimeID.MatchString(r.RuntimeID) {
		return out, collaborationInputError()
	}
	var runtime cloud.RuntimeRecord
	if err := s.cloudAction(ctx, r.ConnectionEpoch, "GetRuntime", map[string]string{"RuntimeId": r.RuntimeID}, &runtime, false); err != nil {
		return out, err
	}
	if runtime.RuntimeID != r.RuntimeID {
		return out, collaborationResponseError(false)
	}
	seen := map[string]bool{}
	for _, ep := range runtime.NetworkConfigurations {
		address := safeDisplayAddress(ep.Endpoint)
		if address != "" && !seen[ep.NetworkType] && (ep.NetworkType == "public" || ep.NetworkType == "private") {
			out.Endpoints = append(out.Endpoints, CollaborationEndpoint{ep.NetworkType, address})
			seen[ep.NetworkType] = true
		}
	}
	return out, nil
}

func (b *Bridge) CreateCollaborationSpace(ctx context.Context, r SpaceCreateRequest) CreatedResourceResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.CreateCollaborationSpace(ctx, r)
	return CreatedResourceResult{err == nil, only(&out, err), err}
}
func (b *Bridge) RegisterCollaborationAgent(ctx context.Context, r AgentRegisterRequest) CreatedResourceResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.RegisterCollaborationAgent(ctx, r)
	return CreatedResourceResult{err == nil, only(&out, err), err}
}
func (b *Bridge) CollaborationAgentDetail(ctx context.Context, r CollaborationMemberRequest) CollaborationDetailResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.CollaborationAgentDetail(ctx, r)
	return CollaborationDetailResult{err == nil, only(&out, err), err}
}
func (b *Bridge) DeleteCollaborationAgent(ctx context.Context, r CollaborationMemberRequest) DeletedResourceResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	err := b.core.DeleteCollaborationAgent(ctx, r)
	return DeletedResourceResult{err == nil, only(&DeletedResource{true}, err), err}
}
func (b *Bridge) DeleteCollaborationSpace(ctx context.Context, r CollaborationSpaceRequest) DeletedResourceResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	err := b.core.DeleteCollaborationSpace(ctx, r)
	return DeletedResourceResult{err == nil, only(&DeletedResource{true}, err), err}
}
func (b *Bridge) CollaborationRuntimes(ctx context.Context, r CollaborationRuntimeListRequest) CollaborationRuntimeListResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.CollaborationRuntimes(ctx, r)
	return CollaborationRuntimeListResult{err == nil, only(&out, err), err}
}
func (b *Bridge) CollaborationRuntimeEndpoints(ctx context.Context, r CollaborationRuntimeRequest) CollaborationEndpointsResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.CollaborationRuntimeEndpoints(ctx, r)
	return CollaborationEndpointsResult{err == nil, only(&out, err), err}
}
