package usage

import (
	"context"
	"math"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

const agentsMDPath = "/api/v1/studio/agents-md"
const resourceTopologyURI = "urn:veadk:mpa:resource-topology:v1"
const resourceInstructionLimit = 40000

type AgentsMDDocument struct {
	Content  string `json:"content"`
	Revision int64  `json:"revision"`
}

type agentsMDWire struct {
	Content  *string `json:"content"`
	Revision *int64  `json:"revision"`
}

func (v agentsMDWire) valid() bool {
	return v.Content != nil && resourceInstructionsValid(*v.Content) && v.Revision != nil && *v.Revision >= 0
}

func (s *Service) ReadAgentsMD(ctx context.Context, epoch uint64) (AgentsMDDocument, *dto.SafeError) {
	var wire agentsMDWire
	if err := s.request(ctx, epoch, http.MethodGet, agentsMDPath, nil, &wire); err != nil {
		return AgentsMDDocument{}, resourceRequestError(err, false)
	}
	if !wire.valid() {
		return AgentsMDDocument{}, resourceResponseError(false)
	}
	if err := s.resourceEpochError(epoch); err != nil {
		return AgentsMDDocument{}, err
	}
	return AgentsMDDocument{Content: *wire.Content, Revision: *wire.Revision}, nil
}

func (s *Service) SaveAgentsMD(ctx context.Context, epoch uint64, document AgentsMDDocument) (AgentsMDDocument, *dto.SafeError) {
	if !resourceInstructionsValid(document.Content) || document.Revision < 0 || document.Revision == math.MaxInt64 {
		return AgentsMDDocument{}, resourceInputError()
	}
	var wire agentsMDWire
	if err := s.request(ctx, epoch, http.MethodPost, agentsMDPath, document, &wire); err != nil {
		return AgentsMDDocument{}, resourceRequestError(err, true)
	}
	if !wire.valid() || *wire.Content != document.Content || *wire.Revision < document.Revision {
		return AgentsMDDocument{}, resourceResponseError(true)
	}
	if err := s.resourceEpochError(epoch); err != nil {
		return AgentsMDDocument{}, err
	}
	return AgentsMDDocument{Content: *wire.Content, Revision: *wire.Revision}, nil
}

type MCPTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// MCPServer excludes gateway URLs, credentials and tool input schemas.
type MCPServer struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Alias       string    `json:"alias"`
	Description string    `json:"description"`
	Tools       []MCPTool `json:"tools"`
	ToolsKnown  bool      `json:"toolsKnown"`
	Source      string    `json:"source"`
	Address     string    `json:"address"`
}

type MCPDirectory struct {
	SessionID string      `json:"sessionId"`
	Servers   []MCPServer `json:"servers"`
}

func (s *Service) ListMCPServers(ctx context.Context, epoch uint64, sessionID string) (MCPDirectory, *dto.SafeError) {
	if !resourceIdentifierValid(sessionID) {
		return MCPDirectory{}, resourceInputError()
	}
	var wire struct {
		SessionID string `json:"sessionId"`
		Servers   []struct {
			ID          string    `json:"id"`
			Name        string    `json:"name"`
			Alias       string    `json:"alias"`
			Description string    `json:"description"`
			Tools       []MCPTool `json:"tools"`
		} `json:"mcpServers"`
	}
	path := "/api/v1/sessions/" + url.PathEscape(sessionID) + "/mcp-servers"
	if err := s.request(ctx, epoch, http.MethodGet, path, nil, &wire); err != nil {
		return MCPDirectory{}, resourceRequestError(err, false)
	}
	if wire.SessionID != sessionID || wire.Servers == nil {
		return MCPDirectory{}, resourceResponseError(false)
	}
	out := MCPDirectory{SessionID: sessionID, Servers: make([]MCPServer, 0, len(wire.Servers))}
	seen := make(map[string]bool)
	for _, item := range wire.Servers {
		if !resourceIdentifierValid(item.ID) || seen[item.ID] {
			return MCPDirectory{}, resourceResponseError(false)
		}
		seen[item.ID] = true
		toolNames := make(map[string]bool)
		for _, tool := range item.Tools {
			if strings.TrimSpace(tool.Name) == "" || toolNames[tool.Name] {
				return MCPDirectory{}, resourceResponseError(false)
			}
			toolNames[tool.Name] = true
		}
		out.Servers = append(out.Servers, MCPServer{
			ID: item.ID, Name: item.Name, Alias: item.Alias, Description: item.Description,
			Tools: item.Tools, ToolsKnown: item.Tools != nil,
		})
	}
	if err := s.resourceEpochError(epoch); err != nil {
		return MCPDirectory{}, err
	}
	return out, nil
}

type SkillSpace struct {
	Description string `json:"description"`
	ProjectName string `json:"projectName"`
	Region      string `json:"region"`
	ID          string `json:"id"`
	Name        string `json:"name"`
}

type SkillMounts struct {
	Spaces []SkillSpace `json:"spaces"`
}

type resourceTopologyNode struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	ResourceID string `json:"resourceId"`
	Status     string `json:"status"`
}

func (s *Service) DiscoverSkillSpaces(ctx context.Context, epoch uint64) (SkillMounts, *dto.SafeError) {
	var wire struct {
		Name         string `json:"name"`
		Capabilities struct {
			Extensions []struct {
				URI    string `json:"uri"`
				Params struct {
					Nodes []resourceTopologyNode `json:"nodes"`
					Edges []struct {
						Source   string `json:"source"`
						Target   string `json:"target"`
						Relation string `json:"relation"`
					} `json:"edges"`
				} `json:"params"`
			} `json:"extensions"`
		} `json:"capabilities"`
	}
	if err := s.request(ctx, epoch, http.MethodGet, "/.well-known/agent-card.json", nil, &wire); err != nil {
		return SkillMounts{}, resourceRequestError(err, false)
	}
	if strings.TrimSpace(wire.Name) == "" {
		return SkillMounts{}, resourceResponseError(false)
	}
	out := SkillMounts{Spaces: []SkillSpace{}}
	found := false
	for _, extension := range wire.Capabilities.Extensions {
		if extension.URI != resourceTopologyURI {
			continue
		}
		if found || extension.Params.Nodes == nil || extension.Params.Edges == nil {
			return SkillMounts{}, resourceResponseError(false)
		}
		found = true
		nodes := make(map[string]resourceTopologyNode)
		for _, node := range extension.Params.Nodes {
			if node.ID == "" || nodes[node.ID].ID != "" {
				return SkillMounts{}, resourceResponseError(false)
			}
			nodes[node.ID] = node
		}
		seen := make(map[string]bool)
		for _, edge := range extension.Params.Edges {
			if nodes[edge.Source].Kind != "agent" || edge.Relation != "mounts" {
				continue
			}
			node, ok := nodes[edge.Target]
			if !ok {
				return SkillMounts{}, resourceResponseError(false)
			}
			if node.Kind != "skill-space" || node.Status != "configured" {
				continue
			}
			if !resourceIdentifierValid(node.ResourceID) {
				return SkillMounts{}, resourceResponseError(false)
			}
			if !seen[node.ResourceID] {
				out.Spaces = append(out.Spaces, SkillSpace{ID: node.ResourceID, Name: node.Name, Region: s.resourceRegion(epoch)})
				seen[node.ResourceID] = true
			}
		}
	}
	if !found {
		return SkillMounts{}, dto.Failure("capabilityUnavailable", "runtimeIngress", "readOnly", 200)
	}
	if err := s.resourceEpochError(epoch); err != nil {
		return SkillMounts{}, err
	}
	return out, nil
}

type Skill struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Status      string `json:"status"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SkillPage struct {
	SpaceID         string  `json:"spaceId"`
	Region          string  `json:"region"`
	Items           []Skill `json:"items"`
	TotalCount      int     `json:"totalCount"`
	PageNumber      int     `json:"pageNumber"`
	PageSize        int     `json:"pageSize"`
	HasMore         bool    `json:"hasMore"`
	ServerPaginated bool    `json:"serverPaginated"`
	ConnectionEpoch uint64  `json:"connectionEpoch"`
	PlatformEpoch   uint64  `json:"platformEpoch"`
}

// ListSkills pages the complete Items response locally. ListSkillsBySpaceId has
// no PageNumber, PageSize or TotalCount contract; those belong to a different API.
func (s *Service) ListSkills(ctx context.Context, epoch uint64, region, spaceID string, pageNumber, pageSize int) (SkillPage, *dto.SafeError) {
	if !cloud.ValidRegion(region) || !resourceIdentifierValid(spaceID) ||
		pageNumber < 1 || pageNumber > 10000 || pageSize < 1 || pageSize > 100 {
		return SkillPage{}, resourceInputError()
	}
	if s.cloud == nil {
		return SkillPage{}, dto.Failure("authRequired", "platform", "reauthenticate", 0)
	}
	platform := s.cloud.State()
	if !platform.SignedIn {
		return SkillPage{}, dto.Failure("authRequired", "platform", "reauthenticate", 0)
	}
	scope, err := s.conn.ScopeContext(epoch)
	if err != nil {
		return SkillPage{}, err
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(scope, cancel)
	defer func() { stop(); cancel() }()
	mounts, err := s.DiscoverSkillSpaces(ctx, epoch)
	if err != nil {
		return SkillPage{}, err
	}
	mounted := false
	for _, space := range mounts.Spaces {
		mounted = mounted || space.ID == spaceID
	}
	if !mounted {
		return SkillPage{}, dto.Failure("resourceNotVisible", "runtimeIngress", "reload", 0)
	}
	if s.cloud.Epoch() != platform.Epoch {
		return SkillPage{}, dto.Failure("staleConnection", "platform", "reload", 0)
	}
	// This is the ID-only runtime consumer contract in veadk/skills/utils.py,
	// not Studio's managed-skill relation API or its SDK name-based fallback.
	input := struct {
		SkillSpaceID string            `json:"SkillSpaceId"`
		InnerTags    map[string]string `json:"InnerTags"`
	}{spaceID, map[string]string{"source": "sandbox"}}
	var wire struct {
		Items []struct {
			Name        *string `json:"Name"`
			Description *string `json:"Description"`
		} `json:"Items"`
	}
	err = s.cloud.Action(ctx, region, "ListSkillsBySpaceId", input, &wire)
	if stale := s.resourceEpochError(epoch); stale != nil {
		return SkillPage{}, stale
	}
	if s.cloud.Epoch() != platform.Epoch {
		return SkillPage{}, dto.Failure("staleConnection", "platform", "reload", 0)
	}
	if err != nil {
		return SkillPage{}, err
	}
	if wire.Items == nil {
		return SkillPage{}, dto.Failure("invalidResponse", "platform", "readOnly", 200)
	}
	items := make([]Skill, 0, len(wire.Items))
	seen := make(map[string]bool)
	for _, item := range wire.Items {
		if item.Name == nil || strings.TrimSpace(*item.Name) == "" || seen[*item.Name] {
			return SkillPage{}, dto.Failure("invalidResponse", "platform", "readOnly", 200)
		}
		seen[*item.Name] = true
		description := ""
		if item.Description != nil {
			description = *item.Description
		}
		items = append(items, Skill{Name: *item.Name, Description: description})
	}
	start := min((pageNumber-1)*pageSize, len(items))
	end := min(start+pageSize, len(items))
	return SkillPage{
		SpaceID: spaceID, Region: region, Items: items[start:end], TotalCount: len(items),
		PageNumber: pageNumber, PageSize: pageSize, HasMore: end < len(items),
		ConnectionEpoch: epoch, PlatformEpoch: platform.Epoch,
	}, nil
}

func resourceInstructionsValid(value string) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= resourceInstructionLimit
}

func resourceIdentifierValid(value string) bool {
	if !utf8.ValidString(value) || strings.TrimSpace(value) != value || value == "" ||
		utf8.RuneCountInString(value) > 512 || value == "." || value == ".." || strings.ContainsAny(value, "/\\%?#") {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func resourceInputError() *dto.SafeError {
	return dto.Failure("invalidInput", "desktop", "never", 0)
}

func resourceResponseError(write bool) *dto.SafeError {
	if write {
		return dto.Failure("outcomeUnknown", "runtimeIngress", "verifyOnly", 200)
	}
	return dto.Failure("invalidResponse", "runtimeIngress", "readOnly", 200)
}

func resourceRequestError(err *dto.SafeError, write bool) *dto.SafeError {
	if err == nil {
		return nil
	}
	if err.HTTPStatus == 404 || err.HTTPStatus == 405 {
		return dto.Failure("capabilityUnavailable", "runtimeIngress", "readOnly", err.HTTPStatus)
	}
	if write && (err.Code == "invalidResponse" || err.HTTPStatus >= 500) {
		return dto.Failure("outcomeUnknown", err.Plane, "verifyOnly", err.HTTPStatus)
	}
	return err
}

func (s *Service) resourceEpochError(epoch uint64) *dto.SafeError {
	if !s.conn.Current(epoch) {
		return dto.Failure("staleConnection", "transport", "reload", 0)
	}
	return nil
}

func (s *Service) resourceRegion(epoch uint64) string {
	if s.target != nil {
		target := s.target()
		if target.ConnectionEpoch == epoch {
			return target.Region
		}
	}
	return ""
}
