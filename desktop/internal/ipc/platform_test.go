package ipc

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/configstore"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type platformRoundTrip struct {
	listCalls    atomic.Int32
	runtimeCalls atomic.Int32
}

func (p *platformRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	action := request.URL.Query().Get("Action")
	switch action {
	case "GetCallerIdentity":
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"Result":{"AccountId":"account-one","IdentityType":"User","UserId":"user-one"}}`))}, nil
	case "ListRuntimes":
		p.listCalls.Add(1)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"Result":{"AgentKitRuntimes":[{"RuntimeId":"runtime-a","Name":"Runtime A","ProjectName":"default","Status":"Ready","CurrentVersionNumber":3,"Endpoint":"https://runtime.example","ToolId":"tool-a","Tags":[{"Key":"veadk:agent-type","Value":"mpa"}],"AuthorizerConfiguration":{"KeyAuth":{"ApiKey":"runtime-key"}}}]}}`))}, nil
	case "GetRuntime":
		p.runtimeCalls.Add(1)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"Result":{"RuntimeId":"runtime-a","Name":"Runtime A","ProjectName":"default","Status":"Ready","CurrentVersionNumber":3,"Endpoint":"https://runtime.example","ToolId":"tool-a","AuthorizerConfiguration":{"KeyAuth":{"ApiKey":"runtime-key"}}}}`))}, nil
	default:
		return &http.Response{StatusCode: 400, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"ResponseMetadata":{"Error":{"Code":"InvalidActionOrVersion"}}}`))}, nil
	}
}

func TestPlatformCachesRuntimeListAndRestoresPreferredRuntime(t *testing.T) {
	transport := &platformRoundTrip{}
	store, err := configstore.Open(filepath.Join(t.TempDir(), "openmpa.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	p := NewPlatformWithStore(cloud.NewWithTransport(transport), connection.New(), nil, store)
	ctx := context.Background()
	if out := p.SignIn(ctx, dto.SignInRequest{Region: "cn-beijing", AccessKey: "AK", SecretKey: "SK"}); !out.OK {
		t.Fatalf("sign in failed %+v", out)
	}
	list := p.ListRuntimes(ctx, dto.RuntimeListRequest{Region: "cn-beijing", ProjectName: "default", ForceRefresh: true})
	if !list.OK || len(list.Value.Runtimes) != 1 || transport.listCalls.Load() != 1 {
		t.Fatalf("remote list failed %+v calls=%d", list, transport.listCalls.Load())
	}
	cached := p.ListRuntimes(ctx, dto.RuntimeListRequest{Region: "cn-beijing", ProjectName: "default"})
	if !cached.OK || len(cached.Value.Runtimes) != 1 || transport.listCalls.Load() != 1 {
		t.Fatalf("cached list missed %+v calls=%d", cached, transport.listCalls.Load())
	}
	bound := p.BindRuntime(ctx, dto.RuntimeRequest{Region: "cn-beijing", ProjectName: "default", RuntimeID: "runtime-a"})
	if !bound.OK || bound.Value.RuntimeID != "runtime-a" {
		t.Fatalf("bind failed %+v", bound)
	}
	if out := p.SignOut(); !out.OK {
		t.Fatalf("sign out failed %+v", out)
	}
	if p.Connection.State().Connected {
		t.Fatal("sign out did not disconnect active runtime")
	}
	if out := p.SignIn(ctx, dto.SignInRequest{Region: "cn-beijing", AccessKey: "AK", SecretKey: "SK"}); !out.OK {
		t.Fatalf("second sign in failed %+v", out)
	}
	state := p.Connection.State()
	if !state.Connected || state.RuntimeID != "runtime-a" {
		t.Fatalf("preferred Runtime not restored: %+v", state)
	}
}
