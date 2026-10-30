package usage

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"golang.org/x/text/unicode/norm"
)

const collaborationRegistry = "/api/v1/studio/collaborators/registry"

var collaborationSpaceID = regexp.MustCompile(`^as-[A-Za-z0-9_-]{1,100}$`)
var collaborationRuntimeID = regexp.MustCompile(`^r-[A-Za-z0-9_-]{1,100}$`)
var collaborationAgentID = regexp.MustCompile(`^a-[A-Za-z0-9_-]{1,100}$`)

type CollaborationSpace struct {
	ID            string `json:"Id"`
	Name          string `json:"Name"`
	IsDefault     bool   `json:"IsDefault"`
	IntentEnabled bool   `json:"IntentEnabled"`
	ProjectName   string `json:"ProjectName"`
}

type CollaborationSpacePage struct {
	Items    []CollaborationSpace `json:"items"`
	Total    int                  `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

// CollaborationCard deliberately excludes endpoints, credentials and tool schemas.
type CollaborationCard struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	Version         string `json:"version"`
	ProtocolVersion string `json:"protocolVersion"`
}

type CollaborationAgent struct {
	ID             string             `json:"Id"`
	Name           string             `json:"Name"`
	Description    string             `json:"Description"`
	Status         string             `json:"Status"`
	Alias          *string            `json:"Alias"`
	AliasRevision  int64              `json:"AliasRevision"`
	AliasAvailable bool               `json:"AliasAvailable"`
	RuntimeID      string             `json:"runtimeId"`
	Card           *CollaborationCard `json:"card,omitempty"`
}

type CollaborationAgentPage struct {
	Items    []CollaborationAgent `json:"items"`
	Total    int                  `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

type CollaborationBinding struct {
	SpaceID   *string `json:"space_id"`
	SpaceName *string `json:"space_name"`
	Region    string  `json:"region"`
	Revision  int64   `json:"revision"`
}

type CollaborationAlias struct {
	Alias    *string `json:"alias"`
	Revision int64   `json:"revision"`
}

type collaborationPageWire[T any] struct {
	Items    []T  `json:"items"`
	Total    *int `json:"total"`
	Page     *int `json:"page"`
	PageSize *int `json:"page_size"`
}

func (p collaborationPageWire[T]) valid(page int) bool {
	return p.Items != nil && p.Total != nil && *p.Total >= len(p.Items) &&
		p.Page != nil && *p.Page == page && p.PageSize != nil &&
		*p.PageSize > 0 && *p.PageSize <= 50 && len(p.Items) <= *p.PageSize
}

func collaborationInputError() *dto.SafeError {
	return dto.Failure("invalidInput", "desktop", "never", 0)
}

func collaborationResponseError(write bool) *dto.SafeError {
	if write {
		return dto.Failure("outcomeUnknown", "runtimeIngress", "verifyOnly", http.StatusOK)
	}
	return dto.Failure("invalidResponse", "runtimeIngress", "readOnly", http.StatusOK)
}

func collaborationRequestError(err *dto.SafeError, write bool) *dto.SafeError {
	if err == nil {
		return nil
	}
	if err.HTTPStatus == 404 || err.HTTPStatus == 405 {
		return dto.Failure("capabilityUnavailable", "runtimeIngress", "readOnly", err.HTTPStatus)
	}
	if write && err.Code == "invalidResponse" {
		return dto.Failure("outcomeUnknown", err.Plane, "verifyOnly", err.HTTPStatus)
	}
	return err
}

func (s *Service) CollaborationSpaces(ctx context.Context, epoch uint64, page int) (*CollaborationSpacePage, *dto.SafeError) {
	if page < 1 || page > 10000 {
		return nil, collaborationInputError()
	}
	var out collaborationPageWire[struct {
		ID            *string `json:"Id"`
		Name          *string `json:"Name"`
		IsDefault     bool    `json:"IsDefault"`
		IntentEnabled bool    `json:"IntentEnabled"`
		ProjectName   string  `json:"ProjectName"`
	}]
	if err := s.request(ctx, epoch, http.MethodGet, collaborationRegistry+"/spaces?page="+strconv.Itoa(page), nil, &out); err != nil {
		return nil, collaborationRequestError(err, false)
	}
	if !out.valid(page) {
		return nil, collaborationResponseError(false)
	}
	result := &CollaborationSpacePage{Items: []CollaborationSpace{}, Total: *out.Total, Page: *out.Page, PageSize: *out.PageSize}
	seen := make(map[string]bool)
	for _, item := range out.Items {
		if item.ID == nil || !collaborationSpaceID.MatchString(*item.ID) || item.Name == nil || seen[*item.ID] {
			return nil, collaborationResponseError(false)
		}
		seen[*item.ID] = true
		result.Items = append(result.Items, CollaborationSpace{ID: *item.ID, Name: *item.Name, IsDefault: item.IsDefault, IntentEnabled: item.IntentEnabled, ProjectName: item.ProjectName})
	}
	return result, nil
}

func (s *Service) CollaborationAgents(ctx context.Context, epoch uint64, spaceID string, page int) (*CollaborationAgentPage, *dto.SafeError) {
	if !collaborationSpaceID.MatchString(spaceID) || page < 1 || page > 10000 {
		return nil, collaborationInputError()
	}
	var out collaborationPageWire[struct {
		ID             *string         `json:"Id"`
		Name           *string         `json:"Name"`
		Description    string          `json:"Description"`
		Status         *string         `json:"Status"`
		Alias          json.RawMessage `json:"Alias"`
		AliasRevision  *int64          `json:"AliasRevision"`
		AliasAvailable bool            `json:"AliasAvailable"`
		AgentCard      json.RawMessage `json:"AgentCard"`
		RuntimeConfig  struct {
			RuntimeID string `json:"RuntimeId"`
		} `json:"RuntimeConfig"`
	}]
	query := url.Values{"space_id": {spaceID}, "page": {strconv.Itoa(page)}}
	if err := s.request(ctx, epoch, http.MethodGet, collaborationRegistry+"/agents?"+query.Encode(), nil, &out); err != nil {
		return nil, collaborationRequestError(err, false)
	}
	if !out.valid(page) {
		return nil, collaborationResponseError(false)
	}
	result := &CollaborationAgentPage{Items: []CollaborationAgent{}, Total: *out.Total, Page: *out.Page, PageSize: *out.PageSize}
	seen := make(map[string]bool)
	for _, item := range out.Items {
		var alias *string
		if item.ID == nil || !collaborationAgentID.MatchString(*item.ID) || item.Name == nil || item.Status == nil ||
			seen[*item.ID] || (item.AliasAvailable && (item.AliasRevision == nil || len(item.Alias) == 0)) ||
			(item.AliasRevision != nil && *item.AliasRevision < 0) ||
			(len(item.Alias) > 0 && json.Unmarshal(item.Alias, &alias) != nil) {
			return nil, collaborationResponseError(false)
		}
		seen[*item.ID] = true
		agent := CollaborationAgent{ID: *item.ID, Name: *item.Name, Description: item.Description, Status: *item.Status,
			Alias: alias, AliasAvailable: item.AliasAvailable, RuntimeID: item.RuntimeConfig.RuntimeID, Card: collaborationCard(item.AgentCard)}
		if item.AliasRevision != nil {
			agent.AliasRevision = *item.AliasRevision
		}
		result.Items = append(result.Items, agent)
	}
	return result, nil
}

func collaborationCard(raw json.RawMessage) *CollaborationCard {
	if len(raw) == 0 || len(raw) > 128*1024 {
		return nil
	}
	var encoded string
	if json.Unmarshal(raw, &encoded) == nil {
		raw = json.RawMessage(encoded)
	}
	var card CollaborationCard
	if json.Unmarshal(raw, &card) != nil || card.Name == "" {
		return nil
	}
	return &card
}

type collaborationBindingWire struct {
	SpaceID   json.RawMessage `json:"space_id"`
	SpaceName *string         `json:"space_name"`
	Region    string          `json:"region"`
	Revision  *int64          `json:"revision"`
}

func (v collaborationBindingWire) binding() *CollaborationBinding {
	var spaceID *string
	if len(v.SpaceID) == 0 || json.Unmarshal(v.SpaceID, &spaceID) != nil ||
		(spaceID != nil && !collaborationSpaceID.MatchString(*spaceID)) ||
		v.Revision == nil || *v.Revision < 0 || v.Region == "" {
		return nil
	}
	return &CollaborationBinding{SpaceID: spaceID, SpaceName: v.SpaceName, Region: v.Region, Revision: *v.Revision}
}

func (s *Service) CollaborationBinding(ctx context.Context, epoch uint64) (*CollaborationBinding, *dto.SafeError) {
	var out collaborationBindingWire
	if err := s.request(ctx, epoch, http.MethodGet, collaborationRegistry+"/binding", nil, &out); err != nil {
		return nil, collaborationRequestError(err, false)
	}
	if value := out.binding(); value != nil {
		return value, nil
	}
	return nil, collaborationResponseError(false)
}

func (s *Service) SetCollaborationBinding(ctx context.Context, epoch uint64, spaceID *string, revision int64) (*CollaborationBinding, *dto.SafeError) {
	if (spaceID != nil && !collaborationSpaceID.MatchString(*spaceID)) || revision < 0 || revision == math.MaxInt64 {
		return nil, collaborationInputError()
	}
	body := struct {
		SpaceID  *string `json:"space_id"`
		Revision int64   `json:"revision"`
	}{spaceID, revision}
	var out collaborationBindingWire
	if err := s.request(ctx, epoch, http.MethodPut, collaborationRegistry+"/binding", body, &out); err != nil {
		return nil, collaborationRequestError(err, true)
	}
	value := out.binding()
	if value == nil || value.Revision != revision+1 || !collaborationSameString(value.SpaceID, spaceID) {
		return nil, collaborationResponseError(true)
	}
	return value, nil
}

func collaborationSameString(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func collaborationNormalizeAlias(alias *string) (*string, bool) {
	if alias == nil || *alias == "" {
		return nil, true
	}
	if !utf8.ValidString(*alias) || utf8.RuneCountInString(*alias) > 80 {
		return nil, false
	}
	for _, r := range *alias {
		if unicode.Is(unicode.C, r) {
			return nil, false
		}
	}
	value := strings.TrimSpace(norm.NFKC.String(*alias))
	if value == "" || utf8.RuneCountInString(value) > 80 {
		return nil, false
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && !strings.ContainsRune(" ._-", r) {
			return nil, false
		}
	}
	return &value, true
}

func (s *Service) SetCollaborationAlias(ctx context.Context, epoch uint64, spaceID, agentID string, alias *string, revision int64) (*CollaborationAlias, *dto.SafeError) {
	value, valid := collaborationNormalizeAlias(alias)
	if !valid || !collaborationSpaceID.MatchString(spaceID) || !collaborationAgentID.MatchString(agentID) || revision < 0 || revision == math.MaxInt64 {
		return nil, collaborationInputError()
	}
	body := struct {
		SpaceID  string  `json:"space_id"`
		Alias    *string `json:"alias"`
		Revision int64   `json:"revision"`
	}{spaceID, value, revision}
	var out struct {
		Alias    json.RawMessage `json:"alias"`
		Revision *int64          `json:"revision"`
	}
	if err := s.request(ctx, epoch, http.MethodPut, collaborationRegistry+"/aliases/"+agentID, body, &out); err != nil {
		return nil, collaborationRequestError(err, true)
	}
	var saved *string
	if len(out.Alias) == 0 || json.Unmarshal(out.Alias, &saved) != nil || out.Revision == nil ||
		*out.Revision != revision+1 || !collaborationSameString(saved, value) {
		return nil, collaborationResponseError(true)
	}
	return &CollaborationAlias{Alias: saved, Revision: *out.Revision}, nil
}

// CollaborationMention matches a2a_mentions.parse_mention; execution remains a normal Session Run.
func CollaborationMention(agent CollaborationAgent, task string) (string, *dto.SafeError) {
	// Registry display names are not Runtime addresses. A saved business alias
	// identifies this member and takes precedence over every remote Card name.
	if !agent.AliasAvailable || agent.Alias == nil || *agent.Alias == "" {
		return "", dto.Failure("collaborationAliasRequired", "desktop", "never", 0)
	}
	name := *agent.Alias
	name, task = strings.TrimSpace(name), strings.TrimSpace(task)
	if agent.Status != "running" || !collaborationAgentID.MatchString(agent.ID) ||
		!utf8.ValidString(name) || name == "" || utf8.RuneCountInString(name) > 80 ||
		strings.ContainsRune(name, '"') || task == "" || strings.HasPrefix(task, "@") {
		return "", collaborationInputError()
	}
	quoted := false
	for _, r := range name {
		if unicode.Is(unicode.C, r) {
			return "", collaborationInputError()
		}
		if unicode.IsSpace(r) || strings.ContainsRune("@`,\uff0c:\uff1a", r) {
			quoted = true
		}
	}
	if quoted {
		name = `"` + name + `"`
	}
	return "@" + name + " " + task, nil
}
