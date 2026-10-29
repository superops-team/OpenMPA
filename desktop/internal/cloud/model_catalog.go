package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

const arkCatalogRegion = "cn-beijing"

type modelAPIKey struct {
	ID     identifier `json:"Id"`
	Name   string     `json:"Name"`
	Status string     `json:"Status"`
	Access struct {
		AllowAll *bool `json:"AllowAll"`
		Rules    struct {
			Model *struct {
				Effect string   `json:"Effect"`
				All    *bool    `json:"EffectToAll"`
				IDs    []string `json:"Ids"`
			} `json:"model"`
		} `json:"AccessRules"`
	} `json:"AccessControlInfo"`
}

func (key modelAPIKey) permits(id, name string) bool {
	if key.Access.AllowAll == nil || *key.Access.AllowAll {
		return true
	}
	rule := key.Access.Rules.Model
	if rule == nil || rule.All == nil || (rule.Effect != "allow" && rule.Effect != "deny") {
		return false
	}
	matched := *rule.All
	for _, value := range rule.IDs {
		matched = matched || value == id || value == name
	}
	if rule.Effect == "deny" {
		return !matched
	}
	return matched
}

// AvailableModelIDs follows Studio's account activation, exact model-version and
// default Ark API Key rules. Credentials and raw keys remain in the host process.
// Reading a catalog never creates a key or activates a model.
func (c *Client) accountModelVersions(ctx context.Context) ([]arkModelVersion, *dto.SafeError) {
	cred, epoch, err := c.snapshot()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	action := func(name string, body, output any) *dto.SafeError {
		if c.Epoch() != epoch {
			return modelCatalogStale()
		}
		err := c.call(ctx, cred, "ark", arkCatalogRegion, name, body, output)
		if c.Epoch() != epoch {
			return modelCatalogStale()
		}
		return err
	}
	keys := []modelAPIKey{}
	seen := map[string]bool{}
	for page := 1; ; page++ {
		if page > 100 {
			return nil, modelCatalogInvalid()
		}
		var result struct {
			Items []modelAPIKey `json:"Items"`
			Total *int          `json:"TotalCount"`
		}
		if err := action("ListApiKeys", map[string]any{"ProjectName": "default", "PageNumber": page, "PageSize": 100}, &result); err != nil {
			return nil, err
		}
		added := 0
		for _, key := range result.Items {
			key.ID = identifier(strings.TrimSpace(string(key.ID)))
			key.Name = strings.TrimSpace(key.Name)
			if key.ID != "" && !seen[string(key.ID)] {
				seen[string(key.ID)] = true
				keys = append(keys, key)
				added++
			}
		}
		if (result.Total != nil && len(keys) >= *result.Total) || (result.Total == nil && len(result.Items) == 0) {
			break
		}
		if added == 0 {
			return nil, modelCatalogInvalid()
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := strings.ToLower(keys[i].Name), strings.ToLower(keys[j].Name)
		if a == b {
			return keys[i].ID < keys[j].ID
		}
		return a < b
	})
	var selected *modelAPIKey
	preferred := strings.TrimSpace(os.Getenv("MODEL_AGENT_API_KEY_NAME"))
	for i := range keys {
		if strings.TrimSpace(keys[i].Status) == "Restricted" {
			continue
		}
		if selected == nil {
			selected = &keys[i]
		}
		if preferred != "" && keys[i].Name == preferred {
			selected = &keys[i]
			break
		}
	}
	if selected == nil {
		return nil, dto.Failure("resourceNotVisible", "platform", "readOnly", 404)
	}
	var keyID any = string(selected.ID)
	if numeric, e := strconv.ParseUint(string(selected.ID), 10, 64); e == nil {
		keyID = numeric
	}
	var raw struct {
		Key string `json:"ApiKey"`
	}
	if err := action("GetRawApiKey", map[string]any{"Id": keyID, "ProjectName": "default"}, &raw); err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw.Key) == "" {
		return nil, modelCatalogInvalid()
	}
	models, err := c.arkModelVersions(ctx, strings.TrimSpace(raw.Key))
	if c.Epoch() != epoch {
		return nil, modelCatalogStale()
	}
	if err != nil {
		return nil, err
	}
	activated := map[string]bool{}
	activationCount := 0
	for page := 1; ; page++ {
		if page > 100 {
			return nil, modelCatalogInvalid()
		}
		var result struct {
			Items []struct {
				Name  string `json:"FoundationModelName"`
				State string `json:"State"`
			} `json:"Items"`
			Total *int `json:"TotalCount"`
		}
		if err := action("ListModelActivations", map[string]any{"PageNumber": page, "PageSize": 100, "WithPrice": false, "WithFreeUsage": false, "Filter": map[string]string{"FoundationModelDomain": "LLM"}}, &result); err != nil {
			return nil, err
		}
		previousCount := len(activated)
		for _, item := range result.Items {
			activated[strings.TrimSpace(item.Name)] = strings.TrimSpace(item.State) == "Available"
		}
		activationCount = len(activated)
		if (result.Total != nil && activationCount >= *result.Total) || (result.Total == nil && len(result.Items) < 100) {
			break
		}
		if activationCount == previousCount {
			return nil, modelCatalogInvalid()
		}
	}
	available := []arkModelVersion{}
	for _, model := range models {
		id, name := strings.TrimSpace(model.ID), strings.TrimSpace(model.Name)
		if id != "" && name != "" && activated[name] && model.supportsAgent() && selected.permits(id, name) {
			available = append(available, model)
		}
	}
	return available, nil
}

type arkModelVersion struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Domain string   `json:"domain"`
	Status string   `json:"status"`
	Tasks  []string `json:"task_type"`
}

func (m arkModelVersion) supportsAgent() bool {
	domain := strings.ToUpper(strings.TrimSpace(m.Domain))
	if strings.TrimSpace(m.Status) == "Shutdown" || (domain != "LLM" && domain != "VLM") {
		return false
	}
	if len(m.Tasks) == 0 {
		return domain == "LLM"
	}
	for _, task := range m.Tasks {
		switch strings.TrimSpace(task) {
		case "TextGeneration", "LLM Agent", "VLM Agent":
			return true
		}
	}
	return false
}
func (c *Client) arkModelVersions(ctx context.Context, key string) ([]arkModelVersion, *dto.SafeError) {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, "https://ark.cn-beijing.volces.com/api/v3/models", nil)
	if e != nil {
		return nil, modelCatalogInvalid()
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Transport: c.transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(req)
	if e != nil {
		return nil, dto.Failure("connectionFailure", "platform", "readOnly", 0)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, dto.Failure("platformRequestFailed", "platform", "readOnly", resp.StatusCode)
	}
	data, e := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	var result struct {
		Data []arkModelVersion `json:"data"`
	}
	if e != nil || len(data) > 8<<20 || json.Unmarshal(data, &result) != nil || result.Data == nil {
		return nil, modelCatalogInvalid()
	}
	return result.Data, nil
}
func modelCatalogInvalid() *dto.SafeError {
	return dto.Failure("invalidResponse", "platform", "readOnly", 0)
}
func modelCatalogStale() *dto.SafeError {
	return dto.Failure("staleConnection", "platform", "reload", 0)
}

// AvailableModelIDs is the Studio-compatible view of the cached eligibility
// catalog. Every returned ID has exact Responses support evidence.
func (c *Client) AvailableModelIDs(ctx context.Context) (map[string]bool, *dto.SafeError) {
	catalog, err := c.ModelCatalog(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, model := range catalog.Models {
		out[model.ID] = true
	}
	return out, nil
}

func (c *Client) accountModelIDs(ctx context.Context) (map[string]bool, *dto.SafeError) {
	models, err := c.accountModelVersions(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, model := range models {
		out[model.ID] = true
	}
	return out, nil
}
