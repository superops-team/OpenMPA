package cloud

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

const arkModelCatalogCacheTTL = 12 * time.Hour
const arkControlRegion = "cn-beijing"

// ModelCatalog returns account-enabled Ark foundation models that explicitly
// advertise OpenAI Responses API support. Credentials stay inside cloud.Client.
type modelCatalogRequest struct {
	done    chan struct{}
	catalog dto.ModelCatalog
	err     *dto.SafeError
}

func copyModelCatalog(value dto.ModelCatalog) dto.ModelCatalog {
	return dto.ModelCatalog{Models: append([]dto.ModelCatalogItem{}, value.Models...)}
}

func (c *Client) ModelCatalog(ctx context.Context) (dto.ModelCatalog, *dto.SafeError) {
	cred, state, e := c.snapshotState()
	if e != nil {
		return dto.ModelCatalog{}, e
	}
	key := modelRegion(cred.Provider) + "|" + strconv.FormatUint(state.Epoch, 10)
	if state.Principal != nil {
		key = state.Principal.Scope + "|" + key
	}
	c.modelMu.Lock()
	if c.modelKey == key && time.Since(c.modelCached) < arkModelCatalogCacheTTL {
		out := copyModelCatalog(c.modelCache)
		c.modelMu.Unlock()
		return out, nil
	}
	if c.modelPending == nil {
		c.modelPending = map[string]*modelCatalogRequest{}
	}
	pending := c.modelPending[key]
	if pending == nil {
		pending = &modelCatalogRequest{done: make(chan struct{})}
		c.modelPending[key] = pending
		go func() {
			// One caller closing a dialog must not cancel the shared catalog load.
			queryCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			catalog, failure := c.fetchModelCatalog(queryCtx, cred, state.Epoch)
			c.modelMu.Lock()
			defer c.modelMu.Unlock()
			if c.Epoch() != state.Epoch {
				failure = dto.Failure("staleConnection", "platform", "reload", 0)
			}
			if failure == nil {
				c.modelKey, c.modelCached, c.modelCache = key, time.Now(), copyModelCatalog(catalog)
			}
			pending.catalog, pending.err = catalog, failure
			delete(c.modelPending, key)
			close(pending.done)
		}()
	}
	c.modelMu.Unlock()
	select {
	case <-ctx.Done():
		return dto.ModelCatalog{}, dto.Failure("cancelled", "platform", "readOnly", 0)
	case <-pending.done:
		if c.Epoch() != state.Epoch {
			return dto.ModelCatalog{}, dto.Failure("staleConnection", "platform", "reload", 0)
		}
		return copyModelCatalog(pending.catalog), pending.err
	}
}

func (c *Client) fetchModelCatalog(ctx context.Context, cred Credentials, epoch uint64) (dto.ModelCatalog, *dto.SafeError) {
	enabled, err := c.enabledFoundationModels(ctx, cred, modelRegion(cred.Provider))
	if err != nil {
		return dto.ModelCatalog{}, err
	}
	if len(enabled) == 0 {
		return dto.ModelCatalog{Models: []dto.ModelCatalogItem{}}, nil
	}
	names := make([]string, 0, len(enabled))
	for name := range enabled {
		names = append(names, name)
	}
	sort.Strings(names)
	versions, err := c.foundationModels(ctx, cred, modelRegion(cred.Provider), names)
	if err != nil {
		return dto.ModelCatalog{}, err
	}
	primary := map[string]string{}
	for _, model := range versions {
		primary[model.Name] = model.PrimaryVersion
	}
	accountModels, availabilityError := c.accountModelVersionsSnapshot(ctx, cred, epoch)
	if availabilityError != nil {
		return dto.ModelCatalog{}, availabilityError
	}
	candidates := []arkFoundationModel{}
	for _, item := range accountModels {
		name, id := strings.TrimSpace(item.Name), strings.TrimSpace(item.ID)
		if !enabled[name] {
			continue
		}
		version := strings.TrimPrefix(id, name+"-")
		if id == name {
			version = primary[name]
		}
		if version == "" || (id != name && version == id) {
			continue
		}
		candidates = append(candidates, arkFoundationModel{Name: name, PrimaryVersion: version, ID: id})
	}
	supported := make([]bool, len(candidates))
	failures := make([]*dto.SafeError, len(candidates))
	groups := map[string][]int{}
	for index, model := range candidates {
		groups[model.Name+"\x00"+model.PrimaryVersion] = append(groups[model.Name+"\x00"+model.PrimaryVersion], index)
	}
	slots := make(chan struct{}, 4)
	var workers sync.WaitGroup
	for _, indexes := range groups {
		workers.Add(1)
		go func() {
			defer workers.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				for _, index := range indexes {
					failures[index] = dto.Failure("cancelled", "platform", "readOnly", 0)
				}
				return
			}
			yes, failure := c.modelSupportsResponses(ctx, cred, candidates[indexes[0]])
			for _, index := range indexes {
				supported[index], failures[index] = yes, failure
			}
		}()
	}
	workers.Wait()
	ids := []string{}
	for index, model := range candidates {
		if failures[index] != nil {
			return dto.ModelCatalog{}, failures[index]
		}
		if supported[index] {
			ids = append(ids, model.FullID())
		}
	}
	return dto.ModelCatalog{Models: modelCatalogItems(ids)}, nil
}

// Verified against the live Ark 2024-01-01 metadata API. No display text,
// capability labels, model family guesses or schema default values are used.
type arkModelMetadata struct {
	FoundationModelName    string
	FoundationModelVersion string
	Key                    string
	Data                   []string
}

func responsesSupport(model arkFoundationModel, rows []arkModelMetadata) (bool, bool) {
	var supported bool
	found := false
	for _, row := range rows {
		if row.Key != "api.responses.support" || row.FoundationModelName != model.Name || row.FoundationModelVersion != model.PrimaryVersion {
			continue
		}
		if len(row.Data) != 1 || (row.Data[0] != "true" && row.Data[0] != "false") {
			return false, false
		}
		value := row.Data[0] == "true"
		if found && value != supported {
			return false, false
		}
		supported, found = value, true
	}
	return supported, found
}

func (c *Client) modelSupportsResponses(ctx context.Context, cred Credentials, model arkFoundationModel) (bool, *dto.SafeError) {
	var out struct {
		ModelMetaDatas           []arkModelMetadata
		PageNum, PageSize, Total int
	}
	input := struct {
		FoundationModelName    string
		FoundationModelVersion string
		Keys                   []string
		PageNum, PageSize      int
	}{model.Name, model.PrimaryVersion, []string{"api.responses.support"}, 1, 1000}
	if e := c.call(ctx, cred, "ark", modelRegion(cred.Provider), "ListModelMetaDatas", input, &out); e != nil {
		// Ark returns Result:null for models without this metadata key (verified
		// for the enabled 3D models). No positive capability means not eligible.
		if e.Code == "missingResult" && e.HTTPStatus == 200 {
			return false, nil
		}
		return false, e
	}
	if out.Total > len(out.ModelMetaDatas) {
		return false, dto.Failure("invalidResponse", "platform", "readOnly", 0)
	}
	supported, known := responsesSupport(model, out.ModelMetaDatas)
	if len(out.ModelMetaDatas) == 0 {
		return false, nil
	}
	if !known {
		err := dto.Failure("modelProtocolMetadataUnavailable", "platform", "readOnly", 0)
		err.Message = "Ark did not return Responses API support metadata for " + model.FullID() + "."
		return false, err
	}
	return supported, nil
}

type arkChargeItem struct {
	FoundationModelName string `json:"FoundationModelName"`
	State               string `json:"State"`
	SubServices         []struct {
		SubService string `json:"SubService"`
		Status     string `json:"Status"`
	} `json:"SubServices"`
}

func (c *Client) enabledFoundationModels(ctx context.Context, cred Credentials, region string) (map[string]bool, *dto.SafeError) {
	enabled := map[string]bool{}
	for page := 1; page <= 100; page++ {
		var out struct {
			Items      []arkChargeItem `json:"Items"`
			PageNumber int             `json:"PageNumber"`
			PageSize   int             `json:"PageSize"`
			TotalCount *int            `json:"TotalCount"`
		}
		input := struct {
			PageNumber int `json:"PageNumber"`
			PageSize   int `json:"PageSize"`
		}{PageNumber: page, PageSize: 100}
		if e := c.call(ctx, cred, "ark", region, "ListModelChargeItems", input, &out); e != nil {
			return nil, e
		}
		for _, item := range out.Items {
			name := strings.TrimSpace(item.FoundationModelName)
			if name == "" || !strings.EqualFold(item.State, "Available") {
				continue
			}
			for _, service := range item.SubServices {
				if strings.EqualFold(service.SubService, "base") && strings.EqualFold(service.Status, "Available") {
					enabled[name] = true
					break
				}
			}
			if len(item.SubServices) == 0 {
				enabled[name] = true
			}
		}
		total := out.TotalCount
		size := out.PageSize
		if size <= 0 {
			size = len(out.Items)
		}
		if (total != nil && *total <= page*size) || (total == nil && len(out.Items) < 100) {
			return enabled, nil
		}
		if len(out.Items) == 0 {
			return nil, modelCatalogInvalid()
		}
	}
	return nil, modelCatalogInvalid()
}

type arkFoundationModel struct {
	ID             string
	Name           string
	PrimaryVersion string
}

func (m arkFoundationModel) FullID() string {
	if m.ID != "" {
		return m.ID
	}
	name := strings.TrimSpace(m.Name)
	version := strings.TrimSpace(m.PrimaryVersion)
	if name == "" || version == "" || strings.HasSuffix(name, "-"+version) {
		return name
	}
	return name + "-" + version
}

func (c *Client) foundationModels(ctx context.Context, cred Credentials, region string, names []string) ([]arkFoundationModel, *dto.SafeError) {
	models := []arkFoundationModel{}
	for start := 0; start < len(names); start += 100 {
		end := start + 100
		if end > len(names) {
			end = len(names)
		}
		received := 0
		seen := map[string]bool{}
		complete := false
		for page := 1; page <= 100; page++ {
			var out struct {
				Items      []map[string]any
				TotalCount *int
			}
			input := struct {
				PageNumber, PageSize int
				Filter               struct {
					Names []string `json:"Names,omitempty"`
				}
				SortBy, SortOrder string
			}{PageNumber: page, PageSize: 100, SortBy: "UpdateTime", SortOrder: "Desc"}
			input.Filter.Names = names[start:end]
			if e := c.call(ctx, cred, "ark", region, "ListFoundationModels", input, &out); e != nil {
				return nil, e
			}
			added := 0
			for _, raw := range out.Items {
				name := strings.TrimSpace(jsonString(raw["Name"]))
				version := strings.TrimSpace(jsonString(raw["PrimaryVersion"]))
				if name == "" {
					return nil, modelCatalogInvalid()
				}
				key := name + "\x00" + version
				if !seen[key] {
					seen[key] = true
					models = append(models, arkFoundationModel{Name: name, PrimaryVersion: version})
					added++
				}
			}
			received += added
			if (out.TotalCount != nil && received >= *out.TotalCount) || (out.TotalCount == nil && len(out.Items) < 100) {
				complete = true
				break
			}
			if added == 0 {
				return nil, modelCatalogInvalid()
			}
		}
		if !complete {
			return nil, modelCatalogInvalid()
		}
	}
	return models, nil
}

func jsonString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func modelCatalogItems(ids []string) []dto.ModelCatalogItem {
	out := []dto.ModelCatalogItem{}
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || strings.ContainsAny(id, "\r\n\x00") || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, dto.ModelCatalogItem{ID: id})
	}
	return out
}
