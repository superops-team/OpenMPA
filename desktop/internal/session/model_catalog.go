package session

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

const modelSelectionURI = "urn:veadk:mpa:model-selection:v1"

// ModelCatalog contains only models advertised by the currently bound Runtime.
// It never exposes the Agent Card's endpoints, credentials or unrelated metadata.
type ModelCatalog struct {
	Models       []string `json:"models"`
	DefaultModel string   `json:"defaultModel"`
	Supported    bool     `json:"supported"`
}
type ModelCatalogResult struct {
	OK    bool           `json:"ok"`
	Value *ModelCatalog  `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}

// Models follows the same model-selection extension consumed by AgentKit Studio.
// A legacy Runtime without this capability has no client-invented fallback list.
func (s *Service) Models(ctx context.Context, r dto.ScopeRequest) ModelCatalogResult {
	if s.conn == nil || !s.conn.Current(r.ConnectionEpoch) {
		return ModelCatalogResult{Error: dto.Failure("staleConnection", "transport", "reload", 0)}
	}
	var card struct {
		Capabilities struct {
			Extensions []struct {
				URI    string          `json:"uri"`
				Params json.RawMessage `json:"params"`
			} `json:"extensions"`
		} `json:"capabilities"`
	}
	if err := s.conn.JSON(ctx, r.ConnectionEpoch, http.MethodGet, "/.well-known/agent-card.json", nil, &card); err != nil {
		return ModelCatalogResult{Error: err}
	}
	if !s.conn.Current(r.ConnectionEpoch) {
		return ModelCatalogResult{Error: dto.Failure("staleConnection", "transport", "reload", 0)}
	}
	out := ModelCatalog{Models: []string{}}
	for _, extension := range card.Capabilities.Extensions {
		if extension.URI != modelSelectionURI {
			continue
		}
		var config struct {
			Models       []string `json:"models"`
			DefaultModel string   `json:"defaultModel"`
		}
		if json.Unmarshal(extension.Params, &config) != nil || len(config.Models) > 1000 {
			return invalidModelCatalog()
		}
		seen := map[string]bool{}
		for _, id := range config.Models {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if !validModelID(id) {
				return invalidModelCatalog()
			}
			if !seen[id] {
				seen[id] = true
				out.Models = append(out.Models, id)
			}
		}
		out.DefaultModel = strings.TrimSpace(config.DefaultModel)
		if out.DefaultModel != "" {
			if !validModelID(out.DefaultModel) {
				return invalidModelCatalog()
			}
			if !seen[out.DefaultModel] {
				out.Models = append([]string{out.DefaultModel}, out.Models...)
			}
		}
		out.Supported = len(out.Models) > 0
		break
	}
	return ModelCatalogResult{OK: true, Value: &out}
}
func validModelID(id string) bool {
	return utf8.ValidString(id) && len(id) <= 256 && !strings.ContainsAny(id, "\r\n\x00")
}
func invalidModelCatalog() ModelCatalogResult {
	return ModelCatalogResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "readOnly", 0)}
}
