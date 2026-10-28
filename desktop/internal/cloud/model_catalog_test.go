package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func TestModelCatalogJoinsActivationVersionsAndKeyPermissions(t *testing.T) {
	t.Setenv("MODEL_AGENT_API_KEY_NAME", "")
	c := New()
	c.credential = Credentials{AccessKey: "TEST_ACCESS", SecretKey: "TEST_SECRET", Token: "TEST_STS"}
	c.state = dto.PlatformState{SignedIn: true, Epoch: 7}
	calls := []string{}
	c.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		action := r.URL.Query().Get("Action")
		var body map[string]any
		if r.Body != nil {
			json.NewDecoder(r.Body).Decode(&body)
		}
		var payload string
		if r.URL.Host == "ark.cn-beijing.volces.com" {
			if r.Method != "GET" || r.URL.Path != "/api/v3/models" || r.Header.Get("Authorization") != "Bearer PRIVATE_RAW_KEY" || r.Header.Get("X-Security-Token") != "" {
				t.Error("wrong data-plane credentials or route")
			}
			calls = append(calls, "models")
			payload = `{"data":[
			 {"id":"deepseek-version","name":"deepseek","domain":"LLM","task_type":["TextGeneration"]},
			 {"id":"deepseek-denied","name":"deepseek","domain":"LLM"},
			 {"id":"qwen-version","name":"qwen","domain":"LLM"},
			 {"id":"retired","name":"deepseek","domain":"LLM","status":"Shutdown"},
			 {"id":"embed","name":"deepseek","domain":"LLM","task_type":["Embedding"]},
			 {"id":"video","name":"deepseek","domain":"VideoGeneration"},
			 {"id":"vision-version","name":"vision","domain":"VLM","task_type":["VLM Agent"]}
			]}`
		} else {
			calls = append(calls, action)
			if r.URL.Host != "open.volcengineapi.com" || r.URL.Query().Get("Version") != "2024-01-01" || !strings.Contains(r.Header.Get("Authorization"), "/cn-beijing/ark/request") || r.Header.Get("X-Security-Token") != "TEST_STS" {
				t.Error("wrong Ark control-plane signature")
			}
			switch action {
			case "ListApiKeys":
				if body["ProjectName"] != "default" {
					t.Error("wrong key project")
				}
				payload = `{"Result":{"Items":[{"Id":9,"Name":"Z-last"},{"Id":1,"Name":"A-disabled","Status":"Restricted"},{"Id":2,"Name":"B-first","AccessControlInfo":{"AllowAll":false,"AccessRules":{"model":{"Effect":"allow","EffectToAll":false,"Ids":["deepseek-version","vision"]}}}}],"TotalCount":3}}`
			case "GetRawApiKey":
				if body["Id"] != float64(2) {
					t.Errorf("wrong default key: %+v", body)
				}
				payload = `{"Result":{"ApiKey":"PRIVATE_RAW_KEY"}}`
			case "ListModelActivations":
				if body["PageNumber"] == float64(1) {
					payload = `{"Result":{"Items":[{"FoundationModelName":"deepseek","State":"Available"}],"TotalCount":3}}`
				} else {
					payload = `{"Result":{"Items":[{"FoundationModelName":"qwen","State":"Unavailable"},{"FoundationModelName":"vision","State":"Available"}],"TotalCount":3}}`
				}
			default:
				t.Fatalf("unexpected action: %s", action)
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	})
	out, err := c.AvailableModelIDs(context.Background())
	if err != nil || !reflect.DeepEqual(out, map[string]bool{"deepseek-version": true, "vision-version": true}) {
		t.Fatalf("catalog: %+v %v", out, err)
	}
	if !reflect.DeepEqual(calls, []string{"ListApiKeys", "GetRawApiKey", "models", "ListModelActivations", "ListModelActivations"}) {
		t.Fatalf("unexpected calls: %v", calls)
	}
}

func TestModelAPIKeyPermissionsMatchStudio(t *testing.T) {
	for _, tc := range []struct {
		access string
		want   bool
	}{
		{`{}`, true}, {`{"AllowAll":true}`, true}, {`{"AllowAll":false}`, false},
		{`{"AllowAll":false,"AccessRules":{"model":{"Effect":"allow","EffectToAll":false,"Ids":["model-version"]}}}`, true},
		{`{"AllowAll":false,"AccessRules":{"model":{"Effect":"allow","EffectToAll":false,"Ids":["model-name"]}}}`, true},
		{`{"AllowAll":false,"AccessRules":{"model":{"Effect":"deny","EffectToAll":false,"Ids":["model-name"]}}}`, false},
		{`{"AllowAll":false,"AccessRules":{"model":{"Effect":"deny","EffectToAll":false,"Ids":["other"]}}}`, true},
		{`{"AllowAll":false,"AccessRules":{"model":{"Effect":"deny","EffectToAll":true}}}`, false},
	} {
		var key modelAPIKey
		if err := json.Unmarshal([]byte(`{"AccessControlInfo":`+tc.access+`}`), &key); err != nil {
			t.Fatal(err)
		}
		if got := key.permits("model-version", "model-name"); got != tc.want {
			t.Errorf("%s: got %v", tc.access, got)
		}
	}
}

func TestModelCatalogRejectsAccountChangeBeforeResolvingKey(t *testing.T) {
	c := New()
	c.credential = Credentials{AccessKey: "ak", SecretKey: "sk"}
	c.state = dto.PlatformState{SignedIn: true, Epoch: 1}
	calls := 0
	c.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		c.SignOut()
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"Result":{"Items":[{"Id":1,"Name":"key"}],"TotalCount":1}}`))}, nil
	})
	_, err := c.AvailableModelIDs(context.Background())
	if err == nil || err.Code != "staleConnection" || calls != 1 {
		t.Fatalf("account isolation: %v, calls=%d", err, calls)
	}
}

func TestModelCatalogNeverFollowsDataPlaneRedirectWithRawKey(t *testing.T) {
	c := New()
	calls := 0
	c.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://untrusted.invalid/models"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	_, err := c.arkModelVersions(context.Background(), "PRIVATE_RAW_KEY")
	if err == nil || calls != 1 || strings.Contains(err.Message, "PRIVATE_RAW_KEY") {
		t.Fatalf("redirect: %v, calls=%d", err, calls)
	}
}
