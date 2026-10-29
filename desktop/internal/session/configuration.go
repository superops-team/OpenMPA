package session

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type configChange struct {
	Category string `json:"category"`
	Mode     string `json:"mode"`
	Value    any    `json:"value,omitempty"`
}

func validConfigMode(mode string) bool {
	return mode == "inherit" || mode == "replace" || mode == "clear"
}
func validResourceID(id string) bool {
	return len(id) > 0 && len(id) <= 256 && !strings.ContainsAny(id, "\r\n\x00")
}

func (s *Service) ModelCatalog(ctx context.Context, r dto.SessionRequest) dto.ModelCatalogResult {
	result := s.Models(ctx, dto.ScopeRequest{ConnectionEpoch: r.ConnectionEpoch})
	if !result.OK {
		return dto.ModelCatalogResult{Error: result.Error}
	}
	catalog := dto.ModelCatalog{Models: []dto.ModelCatalogItem{}}
	for _, id := range result.Value.Models {
		catalog.Models = append(catalog.Models, dto.ModelCatalogItem{ID: id})
	}
	return dto.ModelCatalogResult{OK: true, Value: &catalog}
}

func (s *Service) UpdateExecutionConfig(ctx context.Context, r dto.ExecutionConfigUpdateRequest) dto.ExecutionConfigResult {
	if r.SessionID == "" || r.ETag == "" || len(r.ETag) > 256 || strings.ContainsAny(r.ETag, "\r\n") || !validConfigMode(r.ModelMode) || !validConfigMode(r.MCPMode) || len(r.MCPServers) > 200 {
		return dto.ExecutionConfigResult{Error: dto.Failure("invalidInput", "mpa", "never", 0)}
	}
	changes := []configChange{{Category: "model", Mode: r.ModelMode}, {Category: "mcpServers", Mode: r.MCPMode}}
	if r.ModelMode == "replace" {
		if !validResourceID(r.Model) {
			return dto.ExecutionConfigResult{Error: dto.Failure("invalidInput", "mpa", "never", 0)}
		}
		changes[0].Value = map[string]string{"id": r.Model}
	}
	if r.MCPMode == "replace" {
		refs := []map[string]string{}
		seen := map[string]bool{}
		for _, id := range r.MCPServers {
			if !validResourceID(id) || seen[id] {
				return dto.ExecutionConfigResult{Error: dto.Failure("invalidInput", "mpa", "never", 0)}
			}
			seen[id] = true
			refs = append(refs, map[string]string{"id": id})
		}
		changes[1].Value = refs
	}
	body, _ := json.Marshal(struct {
		Changes []configChange `json:"changes"`
	}{changes})
	resp, e := s.conn.Request(ctx, r.ConnectionEpoch, "PATCH", path(r.SessionID, "/execution-config"), body, http.Header{"If-Match": {r.ETag}})
	if e != nil {
		return dto.ExecutionConfigResult{Error: e}
	}
	return decodeExecutionConfig(resp)
}

// EnsureExecutionConfig repairs a legacy normal Session only after the user
// explicitly chooses a model. It does not submit a Turn or change an active
// Turn's immutable execution snapshot.
func (s *Service) EnsureExecutionConfig(ctx context.Context, r dto.SessionRequest) dto.ExecutionConfigResult {
	current := s.ExecutionConfig(ctx, r)
	if current.Error != nil && current.Error.Code == "diagnosticModelUnsupported" {
		return current
	}
	if current.OK || current.Error == nil || current.Error.HTTPStatus != http.StatusNotFound {
		return current
	}
	var overview struct {
		Agent struct {
			ID string `json:"id"`
		} `json:"agent"`
	}
	if e := s.conn.JSON(ctx, r.ConnectionEpoch, http.MethodGet, "/api/v1/runtime-console/overview", nil, &overview); e != nil {
		return dto.ExecutionConfigResult{Error: e}
	}
	if !validResourceID(overview.Agent.ID) {
		return dto.ExecutionConfigResult{Error: dto.Failure("executionConfigUnavailable", "mpa", "never", 0)}
	}
	var created struct {
		SessionID string `json:"sessionId"`
	}
	input := dto.CreateRequest{ConnectionEpoch: r.ConnectionEpoch, SessionID: r.SessionID, MPAInstanceID: overview.Agent.ID}
	if e := s.conn.JSON(ctx, r.ConnectionEpoch, http.MethodPost, "/api/v1/sessions", input, &created); e != nil {
		return dto.ExecutionConfigResult{Error: e}
	}
	if created.SessionID != r.SessionID {
		return dto.ExecutionConfigResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 0)}
	}
	return s.ExecutionConfig(ctx, r)
}

func decodeExecutionConfig(resp *http.Response) dto.ExecutionConfigResult {
	defer resp.Body.Close()
	var wire struct {
		Revision        int    `json:"revision"`
		ProfileRevision int    `json:"profileRevision"`
		ETag            string `json:"etag"`
		EffectiveRefs   struct {
			Model struct {
				ID string `json:"id"`
			} `json:"model"`
			MCPServers []struct {
				ID string `json:"id"`
			} `json:"mcpServers"`
		} `json:"effectiveRefs"`
		Overrides map[string]struct {
			Mode string `json:"mode"`
		} `json:"overrides"`
		InvalidRefs []json.RawMessage `json:"invalidRefs"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if decoder.Decode(&wire) != nil || decoder.Decode(new(any)) != io.EOF || wire.Revision < 1 || wire.ProfileRevision < 0 {
		return dto.ExecutionConfigResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "readOnly", resp.StatusCode)}
	}
	if tag := resp.Header.Get("ETag"); tag != "" {
		wire.ETag = tag
	}
	if wire.ETag == "" || strings.ContainsAny(wire.ETag, "\r\n") {
		return dto.ExecutionConfigResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "readOnly", resp.StatusCode)}
	}
	out := dto.ExecutionConfig{ExecutionConfigVersion: strconv.Itoa(wire.Revision), ETag: wire.ETag, ProfileBound: wire.ProfileRevision > 0, Model: wire.EffectiveRefs.Model.ID, ModelMode: "inherit", MCPMode: "inherit", MCPServers: []string{}, InvalidRefsCount: len(wire.InvalidRefs)}
	for _, ref := range wire.EffectiveRefs.MCPServers {
		if validResourceID(ref.ID) {
			out.MCPServers = append(out.MCPServers, ref.ID)
		} else {
			return dto.ExecutionConfigResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "readOnly", resp.StatusCode)}
		}
	}
	if change, ok := wire.Overrides["model"]; ok {
		if !validConfigMode(change.Mode) {
			return dto.ExecutionConfigResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "readOnly", resp.StatusCode)}
		}
		out.ModelMode = change.Mode
	}
	if change, ok := wire.Overrides["mcpServers"]; ok {
		if !validConfigMode(change.Mode) {
			return dto.ExecutionConfigResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "readOnly", resp.StatusCode)}
		}
		out.MCPMode = change.Mode
	}
	return dto.ExecutionConfigResult{OK: true, Value: &out}
}
