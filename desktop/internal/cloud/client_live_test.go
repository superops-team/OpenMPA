package cloud

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

// TestLiveSkillCatalog is an opt-in, read-only probe for the Runtime's Skill
// Space and the AgentKit skill catalog actions. It prints only counts and
// whether the allowlisted space identifier is present.
func TestLiveSkillCatalog(t *testing.T) {
	if os.Getenv("OPENMPA_LIVE_SKILL_TEST") != "2112682748/r-yexsqsrpxcxfapor9fvl" {
		t.Skip("set the explicit authorized Runtime to run the live skill probe")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client := New()
	if _, err := client.SignIn(ctx, dto.SignInRequest{
		Region:        "cn-beijing",
		AccessKey:     os.Getenv("VOLCENGINE_ACCESS_KEY"),
		SecretKey:     os.Getenv("VOLCENGINE_SECRET_KEY"),
		SecurityToken: os.Getenv("VOLCENGINE_SECURITY_TOKEN"),
	}); err != nil {
		t.Fatalf("sign in failed: code=%s status=%d", err.Code, err.HTTPStatus)
	}
	cred, _, err := client.snapshot()
	if err != nil {
		t.Fatalf("credential snapshot failed: code=%s", err.Code)
	}
	var runtime struct {
		RuntimeID string `json:"RuntimeId"`
		Envs      []struct {
			Key   string `json:"Key"`
			Value string `json:"Value"`
		} `json:"Envs"`
	}
	if callErr := client.call(ctx, cred, "agentkit", "cn-beijing", "GetRuntime", struct {
		RuntimeID string `json:"RuntimeId"`
	}{"r-yexsqsrpxcxfapor9fvl"}, &runtime); callErr != nil {
		t.Fatalf("GetRuntime failed: code=%s status=%d", callErr.Code, callErr.HTTPStatus)
	}
	spaceID := ""
	for _, env := range runtime.Envs {
		if env.Key == "SKILL_SPACE_ID" {
			spaceID = strings.TrimSpace(env.Value)
		}
	}
	t.Logf("runtime has SKILL_SPACE_ID=%t prefixOK=%t legacy=%t skillhub=%t", spaceID != "", strings.HasPrefix(spaceID, "ss-") || strings.HasPrefix(spaceID, "sp-"), strings.HasPrefix(spaceID, "ss-"), strings.HasPrefix(spaceID, "sp-"))

	var spaces struct {
		Items []struct {
			ID          string `json:"Id"`
			Name        string `json:"Name"`
			ProjectName string `json:"ProjectName"`
		} `json:"Items"`
		TotalCount int `json:"TotalCount"`
	}
	if callErr := client.call(ctx, cred, "agentkit", "cn-beijing", "ListSkillSpaces", map[string]any{
		"ProjectName": "default",
		"PageNumber":  1,
		"PageSize":    100,
	}, &spaces); callErr != nil {
		t.Fatalf("ListSkillSpaces failed: code=%s status=%d", callErr.Code, callErr.HTTPStatus)
	}
	t.Logf("skill spaces: items=%d total=%d", len(spaces.Items), spaces.TotalCount)
	if spaceID == "" {
		return
	}
	var skills struct {
		Items []struct {
			SkillID     string `json:"SkillId"`
			Name        string `json:"Name"`
			Description string `json:"Description"`
			Version     string `json:"Version"`
		} `json:"Items"`
		TotalCount int `json:"TotalCount"`
	}
	if callErr := client.call(ctx, cred, "agentkit", "cn-beijing", "ListSkillsBySpaceId", map[string]any{
		"SkillSpaceId": spaceID,
		"PageNumber":   1,
		"PageSize":     100,
	}, &skills); callErr != nil {
		t.Fatalf("ListSkillsBySpaceId failed: code=%s status=%d", callErr.Code, callErr.HTTPStatus)
	}
	t.Logf("skills in bound space: items=%d total=%d", len(skills.Items), skills.TotalCount)

	var bySkillSpace struct {
		Items []struct {
			SkillID string `json:"SkillId"`
			Name    string `json:"Name"`
		} `json:"Items"`
		TotalCount int `json:"TotalCount"`
	}
	if callErr := client.call(ctx, cred, "agentkit", "cn-beijing", "ListSkillsBySkillSpace", map[string]any{
		"SkillSpaceId": spaceID,
		"PageNumber":   1,
		"PageSize":     100,
	}, &bySkillSpace); callErr != nil {
		t.Logf("ListSkillsBySkillSpace failed: code=%s status=%d", callErr.Code, callErr.HTTPStatus)
	} else {
		t.Logf("skills by SkillSpace action: items=%d total=%d", len(bySkillSpace.Items), bySkillSpace.TotalCount)
	}
	for index, space := range spaces.Items {
		if index >= 3 || space.ID == spaceID {
			continue
		}
		var raw json.RawMessage
		if callErr := client.call(ctx, cred, "agentkit", "cn-beijing", "ListSkillsBySpaceId", map[string]any{
			"SkillSpaceId": space.ID,
			"PageNumber":   1,
			"PageSize":     5,
		}, &raw); callErr != nil {
			t.Logf("sample space %d: failed code=%s", index, callErr.Code)
			continue
		}
		var envelope map[string]json.RawMessage
		_ = json.Unmarshal(raw, &envelope)
		t.Logf("sample space %d: topKeys=%v", index, mapKeys(envelope))
		if result, ok := envelope["Result"]; ok {
			var resultMap map[string]json.RawMessage
			_ = json.Unmarshal(result, &resultMap)
			t.Logf("sample space %d: resultKeys=%v", index, mapKeys(resultMap))
			if items, ok := resultMap["Items"]; ok {
				var list []map[string]json.RawMessage
				_ = json.Unmarshal(items, &list)
				if len(list) > 0 {
					t.Logf("sample space %d: itemKeys=%v", index, mapKeys(list[0]))
				}
			}
		}
	}
}

func mapKeys(value map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// TestLiveAgentKitRegionCatalog is an opt-in, read-only proof that every
// shipped Region reaches the account's real ListRuntimes control plane. It
// never prints credentials, response bodies or Runtime identifiers.
func TestLiveAgentKitRegionCatalog(t *testing.T) {
	if os.Getenv("OPENMPA_LIVE_REGION_TEST") != "2112682748" {
		t.Skip("set the explicit authorized account to run the live Region proof")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client := New()
	state, err := client.SignIn(ctx, dto.SignInRequest{
		Region:        "cn-beijing",
		AccessKey:     os.Getenv("VOLCENGINE_ACCESS_KEY"),
		SecretKey:     os.Getenv("VOLCENGINE_SECRET_KEY"),
		SecurityToken: os.Getenv("VOLCENGINE_SECURITY_TOKEN"),
	})
	if err != nil {
		t.Fatalf("AgentKit identity failed: code=%s status=%d", err.Code, err.HTTPStatus)
	}
	if state.Principal == nil || state.Principal.AccountID != "2112682748" {
		t.Fatal("unexpected AgentKit account")
	}
	for _, region := range Regions() {
		result, listErr := client.List(ctx, dto.RuntimeListRequest{Region: region.Region, ProjectName: "default"})
		if listErr != nil {
			t.Errorf("Region %s failed: code=%s status=%d", region.Region, listErr.Code, listErr.HTTPStatus)
			continue
		}
		if !result.Complete {
			t.Errorf("Region %s returned an incomplete Runtime directory", region.Region)
		}
	}
}
