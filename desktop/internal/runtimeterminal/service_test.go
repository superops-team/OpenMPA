package runtimeterminal

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type terminalRoundTrip struct {
	instanceCalls atomic.Int32
}

func (t *terminalRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	switch request.URL.Query().Get("Action") {
	case "GetCallerIdentity":
		return jsonResponse(`{"Result":{"AccountId":"account","IdentityType":"User","UserId":"user"}}`), nil
	case "ListRuntimeInstances":
		call := t.instanceCalls.Add(1)
		if call == 1 {
			return jsonResponse(`{"Result":{"InstanceItems":[]}}`), nil
		}
		return jsonResponse(`{"Result":{"InstanceItems":[{"InstanceName":"vefaas-fnabc-worker","InstanceStatus":"Running"}]}}`), nil
	default:
		return jsonResponse(`{"ResponseMetadata":{"Error":{"Code":"InvalidActionOrVersion"}}}`), nil
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestResolveInstanceRequiresExplicitWarmupWhenNoRuntimeInstanceExists(t *testing.T) {
	rt := &terminalRoundTrip{}
	cloudClient := cloud.NewWithTransport(rt)
	if _, e := cloudClient.SignIn(context.Background(), dto.SignInRequest{AccessKey: "ak", SecretKey: "sk", Region: "cn-beijing"}); e != nil {
		t.Fatal(e)
	}
	conn := connection.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("warm-up must not run without confirmation: %s", r.URL.Path)
	}))
	defer server.Close()
	epoch, e := conn.Bind(connection.Binding{Endpoint: server.URL, RuntimeID: "r-1", RuntimeKey: "runtime-key"}, true)
	if e != nil {
		t.Fatal(e)
	}
	target := dto.RuntimeRequest{Region: "cn-beijing", ProjectName: "default", RuntimeID: "r-1"}
	svc := New(cloudClient, conn, func() dto.RuntimeRequest { return target })
	win := &terminalWindow{service: svc, request: OpenRequest{ConnectionEpoch: epoch, SessionID: "s"}, target: target, ctx: context.Background()}
	_, err := win.resolveInstance(false)
	if err == nil || err.state != "warmupRequired" {
		t.Fatalf("expected warmupRequired, got %+v", err)
	}
}

func TestResolveInstanceWarmupThenWaitsForAttachableInstance(t *testing.T) {
	rt := &terminalRoundTrip{}
	cloudClient := cloud.NewWithTransport(rt)
	if _, e := cloudClient.SignIn(context.Background(), dto.SignInRequest{AccessKey: "ak", SecretKey: "sk", Region: "cn-beijing"}); e != nil {
		t.Fatal(e)
	}
	var warmups atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/invoke" || r.Header.Get("Authorization") != "Bearer runtime-key" {
			t.Fatalf("bad warm-up request path=%s auth=%s", r.URL.Path, r.Header.Get("Authorization"))
		}
		warmups.Add(1)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	conn := connection.New()
	epoch, e := conn.Bind(connection.Binding{Endpoint: server.URL, RuntimeID: "r-1", RuntimeKey: "runtime-key"}, true)
	if e != nil {
		t.Fatal(e)
	}
	target := dto.RuntimeRequest{Region: "cn-beijing", ProjectName: "default", RuntimeID: "r-1"}
	svc := New(cloudClient, conn, func() dto.RuntimeRequest { return target })
	svc.pollInterval = time.Millisecond
	svc.waitTimeout = time.Second
	win := &terminalWindow{service: svc, request: OpenRequest{ConnectionEpoch: epoch, SessionID: "s"}, target: target, ctx: context.Background()}
	instance, err := win.resolveInstance(true)
	if err != nil || instance.InstanceName != "vefaas-fnabc-worker" || warmups.Load() != 1 {
		t.Fatalf("instance=%+v err=%+v warmups=%d", instance, err, warmups.Load())
	}
	if functionIDFromInstance(instance.InstanceName) != "fnabc" {
		t.Fatalf("bad function id %q", functionIDFromInstance(instance.InstanceName))
	}
}
