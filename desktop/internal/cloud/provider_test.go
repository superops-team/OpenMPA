package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func jsonResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestBytePlusIdentityRoutingAndProviderIsolation(t *testing.T) {
	c := New()
	var hosts []string
	c.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		hosts = append(hosts, r.URL.Host)
		action := r.URL.Query().Get("Action")
		service := "agentkit"
		if action == "GetCallerIdentity" {
			service = "sts"
		}
		region := "ap-southeast-1"
		if strings.Contains(r.URL.Host, "volcengine") {
			region = "cn-beijing"
		}
		if !strings.Contains(r.Header.Get("Authorization"), "/"+region+"/"+service+"/request") {
			t.Error("incorrect signature scope")
		}
		if action == "GetCallerIdentity" {
			return jsonResponse(`{"Result":{"AccountId":"123","UserId":"456","IdentityType":"User"}}`), nil
		}
		return jsonResponse(`{"Result":{"AgentKitRuntimes":[]}}`), nil
	})
	ctx := context.Background()
	domestic, failure := c.SignIn(ctx, dto.SignInRequest{Region: "cn-beijing", AccessKey: "ak", SecretKey: "sk"})
	if failure != nil || domestic.Principal.Scope != "123/User/456" {
		t.Fatalf("legacy scope changed: %+v %v", domestic, failure)
	}
	bp, failure := c.SignIn(ctx, dto.SignInRequest{Provider: "byteplus", Region: "ap-southeast-1", AccessKey: "bp-ak", SecretKey: "bp-sk"})
	if failure != nil || bp.Provider != "byteplus" || bp.Principal.Scope == domestic.Principal.Scope {
		t.Fatalf("provider identity not isolated: %+v %v", bp, failure)
	}
	if _, failure = c.List(ctx, dto.RuntimeListRequest{Region: "ap-southeast-1"}); failure != nil {
		t.Fatal(failure)
	}
	if strings.Join(hosts, ",") != "sts.volcengineapi.com,open.byteplusapi.com,agentkit.ap-southeast-1.byteplusapi.com" {
		t.Fatal(hosts)
	}
	before := len(hosts)
	if _, failure = c.List(ctx, dto.RuntimeListRequest{Region: "cn-beijing"}); failure == nil {
		t.Fatal("BP credentials accepted in domestic region")
	}
	for _, request := range []dto.SignInRequest{
		{Provider: "byteplus", Region: "cn-beijing", AccessKey: "ak", SecretKey: "sk"},
		{Region: "ap-southeast-1", AccessKey: "ak", SecretKey: "sk"},
		{Provider: "unknown", Region: "ap-southeast-1", AccessKey: "ak", SecretKey: "sk"},
	} {
		if _, failure = c.SignIn(ctx, request); failure == nil {
			t.Fatal("invalid provider/region accepted")
		}
	}
	if len(hosts) != before || c.State().Epoch != bp.Epoch {
		t.Fatal("invalid requests sent traffic or replaced account")
	}
}

func TestBytePlusModelCatalogUsesOverseasHostsAndAccountAvailability(t *testing.T) {
	c := New()
	c.credential = Credentials{AccessKey: "ak", SecretKey: "sk", Provider: "byteplus"}
	c.state = dto.PlatformState{SignedIn: true, Provider: "byteplus", Epoch: 1}
	c.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		action := r.URL.Query().Get("Action")
		if action == "" {
			if r.URL.Host != "ark.ap-southeast.bytepluses.com" || r.Header.Get("Authorization") != "Bearer private-key" {
				t.Error("wrong BP model data plane")
			}
			return jsonResponse(`{"data":[{"id":"seed-v1","name":"seed","domain":"LLM"},{"id":"unavailable-v1","name":"unavailable","domain":"LLM"}]}`), nil
		}
		if r.URL.Host != "open.byteplusapi.com" || !strings.Contains(r.Header.Get("Authorization"), "/ap-southeast-1/ark/request") {
			t.Error("wrong BP model control plane")
		}
		result := ""
		switch action {
		case "ListModelChargeItems":
			result = `{"Items":[{"FoundationModelName":"seed","State":"Available","SubServices":[{"SubService":"base","Status":"Available"}]}],"TotalCount":1}`
		case "ListFoundationModels":
			result = `{"Items":[{"Name":"seed","PrimaryVersion":"v1"}]}`
		case "ListApiKeys":
			result = `{"Items":[{"Id":1,"Name":"default"}],"TotalCount":1}`
		case "GetRawApiKey":
			result = `{"ApiKey":"private-key"}`
		case "ListModelActivations":
			result = `{"Items":[{"FoundationModelName":"seed","State":"Available"},{"FoundationModelName":"unavailable","State":"Unavailable"}],"TotalCount":2}`
		case "ListModelMetaDatas":
			result = `{"ModelMetaDatas":[{"FoundationModelName":"seed","FoundationModelVersion":"v1","Key":"api.responses.support","Data":["true"]}],"Total":1}`
		default:
			t.Fatalf("unexpected action %s", action)
		}
		return jsonResponse(`{"Result":` + result + `}`), nil
	})
	result, failure := c.ModelCatalog(context.Background())
	if failure != nil || len(result.Models) != 1 || result.Models[0].ID != "seed-v1" {
		t.Fatalf("catalog=%+v error=%v", result, failure)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "private-key") {
		t.Fatal("key leaked")
	}
}

func TestBytePlusStorageAndWebshellRoutes(t *testing.T) {
	c := New()
	c.credential = Credentials{AccessKey: "ak", SecretKey: "sk", Provider: "byteplus"}
	c.state = dto.PlatformState{SignedIn: true, Provider: "byteplus", Epoch: 1, Principal: &dto.Principal{AccountID: "123"}}
	c.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("Action") == "GenWebshellEndpoint" {
			if r.URL.Host != "vefaas.ap-southeast-1.byteplusapi.com" {
				t.Error(r.URL.Host)
			}
			return jsonResponse(`{"Result":{"Endpoint":"wss://terminal.example.invalid"}}`), nil
		}
		if r.URL.Host != "tos-ap-southeast-1.bytepluses.com" {
			t.Errorf("wrong TOS host: %s", r.URL.Host)
		}
		return jsonResponse(`{"Buckets":[{"Name":"agentkit-platform-bp","Location":"ap-southeast-1"},{"Name":"agentkit-platform-wrong-region","Location":"cn-beijing"}]}`), nil
	})
	buckets, failure := c.ListMPATOSBuckets(context.Background(), 1, "ap-southeast-1")
	if failure != nil || len(buckets) != 1 || buckets[0].Name != "agentkit-platform-bp" {
		t.Fatalf("buckets=%v failure=%v", buckets, failure)
	}
	if _, failure = c.WebshellEndpoint(context.Background(), "ap-southeast-1", "function", "instance"); failure != nil {
		t.Fatal(failure)
	}
	if _, failure = c.ListMPATOSBuckets(context.Background(), 1, "cn-beijing"); failure == nil {
		t.Fatal("cross-provider bucket request accepted")
	}
	if _, failure = c.PutSkillArchive(context.Background(), "cn-beijing", "default", []byte("archive")); failure == nil {
		t.Fatal("cross-provider skill write accepted")
	}
}
