package cloud

import (
	"context"
	"encoding/json"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestAgentKitRegionCatalogRoutesHostAndSignature(t *testing.T) {
	regions := Regions()
	if len(regions) != 2 || regions[0].Region != "cn-beijing" || regions[1].Region != "cn-shanghai" {
		t.Fatalf("unexpected AgentKit regions: %+v", regions)
	}
	for _, item := range regions {
		endpoint, ok := AgentKitEndpoint(item.Region)
		if !ok || endpoint != "https://"+item.EndpointHost || !ValidRegion(item.Region) {
			t.Fatalf("bad endpoint for %+v", item)
		}
	}
	if _, ok := AgentKitEndpoint("cn-guangzhou"); ok {
		t.Fatal("unsupported AgentKit region accepted")
	}

	c := New()
	c.credential = Credentials{AccessKey: "ak", SecretKey: "sk"}
	c.state = dto.PlatformState{SignedIn: true}
	c.transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "agentkit.cn-shanghai.volcengineapi.com" || !strings.Contains(request.Header.Get("Authorization"), "/cn-shanghai/agentkit/request") {
			t.Errorf("Shanghai routing/signature mismatch: %s", request.URL.Host)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"Result":{"AgentKitRuntimes":[]}}`))}, nil
	})
	if _, err := c.List(context.Background(), dto.RuntimeListRequest{Region: "cn-shanghai", ProjectName: "default"}); err != nil {
		t.Fatal(err)
	}
}

func testCloud(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	c := New()
	c.endpoint = func(service, region string) string { return s.URL }
	return c
}
func TestOfficialSignerAndSecretProjection(t *testing.T) {
	c := testCloud(t, func(w http.ResponseWriter, r *http.Request) {
		a := r.URL.Query().Get("Action")
		wantVersion := APIVersion
		wantService := "agentkit"
		if a == "GetCallerIdentity" {
			wantVersion = "2018-01-01"
			wantService = "sts"
		}
		if r.Method != "POST" || r.URL.Query().Get("Version") != wantVersion {
			t.Error("fixed platform wire mismatch")
		}
		auth := r.Header.Get("Authorization")
		if !strings.Contains(auth, "/cn-beijing/"+wantService+"/request") || r.Header.Get("X-Security-Token") != "SECRET_STS" || len(r.Header.Get("X-Content-Sha256")) != 64 || r.Header.Get("X-Date") == "" {
			t.Error("official signer wire missing")
		}
		if a == "GetCallerIdentity" {
			w.Write([]byte(`{"Result":{"AccountId":"account","IdentityType":"User","UserId":"user"}}`))
		} else {
			w.Write([]byte(`{"Result":{"RuntimeId":"r1","Name":"MPA","Endpoint":"https://example.com/runtime/r1","ArtifactUrl":"private-image","ToolId":"t1","AuthorizerConfiguration":{"KeyAuth":{"ApiKey":"SECRET_RUNTIME"}},"Envs":[{"Key":"SECRET","Value":"SECRET_ENV"}]}}`))
		}
	})
	state, e := c.SignIn(context.Background(), dto.SignInRequest{AccessKey: "AK_TEST", SecretKey: "SECRET_SK", SecurityToken: "SECRET_STS", Region: "cn-beijing"})
	if e != nil {
		t.Fatal(e)
	}
	raw, e := c.GetRuntime(context.Background(), dto.RuntimeRequest{Region: "cn-beijing", RuntimeID: "r1"})
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(struct {
		State   dto.PlatformState
		Runtime dto.RuntimeSummary
	}{state, raw.Summary("cn-beijing")})
	for _, secret := range []string{"AK_TEST", "SECRET_SK", "SECRET_STS", "SECRET_RUNTIME", "SECRET_ENV", "private-image"} {
		if strings.Contains(string(b), secret) {
			t.Errorf("secret projected: %s", secret)
		}
	}
}
func TestPlatformHTTP200ErrorIsNotSuccess(t *testing.T) {
	c := testCloud(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ResponseMetadata":{"Error":{"Code":"LackPolicy","Message":"SECRET_HTML"}},"Result":{}}`))
	})
	_, e := c.SignIn(context.Background(), dto.SignInRequest{AccessKey: "ak", SecretKey: "sk", Region: "cn-beijing"})
	if e == nil || e.Code != "denied" || e.Plane != "platform" || strings.Contains(e.Message, "SECRET") {
		t.Fatalf("bad platform error: %+v", e)
	}
}
func TestListPaginationExplicitMPATags(t *testing.T) {
	calls := 0
	c := testCloud(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var input struct {
			NextToken  string `json:"NextToken"`
			TagFilters []struct {
				Key    string   `json:"Key"`
				Values []string `json:"Values"`
			} `json:"TagFilters"`
		}
		json.NewDecoder(r.Body).Decode(&input)
		if len(input.TagFilters) != 1 || input.TagFilters[0].Key != "veadk:agent-type" {
			t.Error("MPA filter absent")
		}
		if calls == 1 {
			w.Write([]byte(`{"Result":{"AgentKitRuntimes":[{"RuntimeId":"r1","ProjectName":"default","Tags":[{"Key":"veadk:agent-type","Value":"mpa"}]}],"NextToken":"next"}}`))
		} else {
			if input.NextToken != "next" {
				t.Error("cursor changed")
			}
			w.Write([]byte(`{"Result":{"AgentKitRuntimes":[{"RuntimeId":"other","ProjectName":"default","Tags":[{"Key":"veadk:agent-type","Value":"other"}]}]}}`))
		}
	})
	c.credential = Credentials{AccessKey: "ak", SecretKey: "sk"}
	c.state = dto.PlatformState{SignedIn: true}
	out, e := c.List(context.Background(), dto.RuntimeListRequest{Region: "cn-beijing", ProjectName: "default"})
	if e != nil || !out.Complete || len(out.Runtimes) != 1 || calls != 2 {
		t.Fatalf("list %+v %v", out, e)
	}
}
func TestNumericAccountIdentityAndStableProfileAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Result":{"AccountId":1234567890,"IdentityType":"Account","UserId":null}}`))
	}))
	defer server.Close()
	var scope string
	for i := 0; i < 2; i++ {
		c := NewWithProfileStore(func() (string, error) { return dir + "/profiles.json", nil })
		c.endpoint = func(string, string) string { return server.URL }
		state, e := c.SignIn(context.Background(), dto.SignInRequest{AccessKey: "ak", SecretKey: "sk", Region: "cn-beijing"})
		if e != nil {
			t.Fatal(e)
		}
		if state.Principal.AccountID != "1234567890" {
			t.Fatal("numeric identity lost")
		}
		if i == 0 {
			scope = state.Principal.Scope
		} else if scope != state.Principal.Scope {
			t.Fatal("namespace changed across restart")
		}
	}
}
