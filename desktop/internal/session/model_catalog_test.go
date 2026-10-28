package session

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func TestModelsReadCurrentRuntimeCapabilityOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/.well-known/agent-card.json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"capabilities":{"extensions":[{"uri":"urn:veadk:mpa:model-selection:v1","params":{"models":["new-runtime-model","new-runtime-model","another-model"],"defaultModel":"runtime-default"}}]}}`))
	}))
	defer server.Close()
	c := connection.New()
	epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL}, true)
	s := New(c, nil)
	result := s.Models(context.Background(), dto.ScopeRequest{ConnectionEpoch: epoch})
	if !result.OK || result.Value == nil || !result.Value.Supported || result.Value.DefaultModel != "runtime-default" || !reflect.DeepEqual(result.Value.Models, []string{"runtime-default", "new-runtime-model", "another-model"}) {
		t.Fatalf("unexpected catalog: %+v", result)
	}
	c.Disconnect()
	if stale := s.Models(context.Background(), dto.ScopeRequest{ConnectionEpoch: epoch}); stale.OK {
		t.Fatal("old connection catalog remained usable")
	}
}

type catalogRoundTrip func(*http.Request) (*http.Response, error)

func (f catalogRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestModelsMatchStudioAccountIntersectionAndDefaultFallback(t *testing.T) {
	for _, tc := range []struct {
		name, mode string
		want       []string
		stale      bool
	}{
		{"available only", "normal", []string{"runtime-default", "deepseek-version"}, false},
		{"empty account catalog retains default", "empty", []string{"runtime-default"}, false},
		{"catalog unavailable uses Runtime fallback", "denied", []string{"qwen-version", "deepseek-version", "runtime-default", "key-denied"}, false},
		{"account change rejects response", "account-change", nil, true},
		{"Runtime change rejects response", "runtime-change", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MODEL_AGENT_API_KEY_NAME", "")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"capabilities":{"extensions":[{"uri":"urn:veadk:mpa:model-selection:v1","params":{"models":["qwen-version","deepseek-version","runtime-default","key-denied","qwen-version"],"defaultModel":"runtime-default"}}]}}`)
			}))
			defer server.Close()
			conn := connection.New()
			epoch, _ := conn.Bind(connection.Binding{Endpoint: server.URL}, true)
			var platform *cloud.Client
			platform = cloud.NewWithTransport(catalogRoundTrip(func(r *http.Request) (*http.Response, error) {
				var payload string
				switch r.URL.Query().Get("Action") {
				case "GetCallerIdentity":
					payload = `{"Result":{"AccountId":"account","UserId":"user","IdentityType":"User"}}`
				case "ListApiKeys":
					if tc.mode == "denied" {
						payload = `{"ResponseMetadata":{"Error":{"Code":"LackPolicy"}}}`
					} else {
						payload = `{"Result":{"Items":[{"Id":1,"Name":"default","AccessControlInfo":{"AllowAll":false,"AccessRules":{"model":{"Effect":"deny","EffectToAll":false,"Ids":["key-denied"]}}}}],"TotalCount":1}}`
					}
				case "GetRawApiKey":
					payload = `{"Result":{"ApiKey":"NEVER_PROJECT_THIS_KEY"}}`
				case "ListModelActivations":
					payload = `{"Result":{"Items":[{"FoundationModelName":"deepseek","State":"Available"},{"FoundationModelName":"qwen","State":"Unavailable"},{"FoundationModelName":"denied","State":"Available"},{"FoundationModelName":"account-only","State":"Available"}],"TotalCount":4}}`
				case "":
					payload = `{"data":[{"id":"deepseek-version","name":"deepseek","domain":"LLM"},{"id":"qwen-version","name":"qwen","domain":"LLM"},{"id":"key-denied","name":"denied","domain":"LLM"},{"id":"not-in-runtime","name":"account-only","domain":"LLM"}]}`
					if tc.mode == "empty" {
						payload = `{"data":[]}`
					}
					if tc.mode == "account-change" {
						platform.SignOut()
					}
					if tc.mode == "runtime-change" {
						conn.Disconnect()
					}
				default:
					t.Errorf("unexpected request: %s", r.URL)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
			}))
			if _, err := platform.SignIn(context.Background(), dto.SignInRequest{AccessKey: "TEST_ACCESS", SecretKey: "TEST_SECRET", Region: "cn-beijing"}); err != nil {
				t.Fatal(err)
			}
			service := NewWithCache(conn, nil, nil, platform)
			result := service.Models(context.Background(), dto.ScopeRequest{ConnectionEpoch: epoch})
			if tc.stale {
				if result.OK || result.Error == nil || result.Error.Code != "staleConnection" {
					t.Fatalf("accepted stale catalog: %+v", result)
				}
				return
			}
			if !result.OK || result.Value == nil || !reflect.DeepEqual(result.Value.Models, tc.want) || result.Value.DefaultModel != "runtime-default" {
				t.Fatalf("catalog: %+v, want %v", result, tc.want)
			}
			data, _ := json.Marshal(result)
			if strings.Contains(string(data), "NEVER_PROJECT") || strings.Contains(string(data), "TEST_SECRET") {
				t.Fatal("credentials exposed in catalog")
			}
		})
	}
}

func TestModelsNeverInventFallbackCatalog(t *testing.T) {
	for _, body := range []string{`{"capabilities":{}}`, `{"capabilities":{"extensions":[]}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		c := connection.New()
		epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL}, true)
		result := New(c, nil).Models(context.Background(), dto.ScopeRequest{ConnectionEpoch: epoch})
		server.Close()
		if !result.OK || result.Value == nil || result.Value.Supported || len(result.Value.Models) != 0 {
			t.Fatalf("invented models: %+v", result)
		}
	}
}

func TestModelsRejectMalformedCapability(t *testing.T) {
	for _, body := range []string{`{"capabilities":{"extensions":[{"uri":"urn:veadk:mpa:model-selection:v1","params":{"models":"invalid","defaultModel":"a"}}]}}`, `{"capabilities":{"extensions":[{"uri":"urn:veadk:mpa:model-selection:v1","params":{"models":["a\nsecret"],"defaultModel":"a"}}]}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		c := connection.New()
		epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL}, true)
		result := New(c, nil).Models(context.Background(), dto.ScopeRequest{ConnectionEpoch: epoch})
		server.Close()
		if result.OK || result.Error == nil || result.Error.Code != "invalidResponse" {
			t.Fatalf("accepted invalid catalog: %+v", result)
		}
	}
}
