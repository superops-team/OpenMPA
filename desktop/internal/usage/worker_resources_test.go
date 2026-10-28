package usage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
)

func TestWorkerMCPDoesNotRequireOrReadSession(t *testing.T) {
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/debug/mcp-resources/current" {
			t.Fatalf("unexpected session or other request: %s", r.URL.Path)
		}
		io.WriteString(w, `{"resources":[{"id":"agentkit:search","name":"search","tools":[{"name":"lookup","description":"Find docs","inputSchema":{"secret":"do-not-copy"}}],"headers":{"Authorization":"do-not-copy"}}],"total":1}`)
	})
	service.cloud = resourceTestCloud(t, func(r *http.Request) string {
		if r.URL.Query().Get("Action") != "GetRuntime" {
			t.Errorf("unexpected action")
		}
		return `{"Result":{"RuntimeId":"r-runtime","Envs":[{"Key":"MCP_SERVERS_JSON","Value":"[{\"name\":\"external\",\"url\":\"https://example.invalid/mcp?token=do-not-copy\",\"headers\":{\"Authorization\":\"do-not-copy\"}}]"}]}}`
	})
	service.target = func() deployment.Target {
		return deployment.Target{RuntimeID: "r-runtime", Region: "cn-beijing", ConnectionEpoch: epoch, PlatformEpoch: service.cloud.Epoch()}
	}
	out, err := service.WorkerMCPServers(context.Background(), epoch)
	if err != nil || len(out.Servers) != 2 {
		t.Fatalf("worker MCP: %+v %v", out, err)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "do-not-copy") || strings.Contains(string(raw), "inputSchema") || strings.Contains(string(raw), "Authorization") {
		t.Fatal("credential or schema escaped projection")
	}
	if !out.Servers[1].ToolsKnown || out.Servers[1].Tools[0].Name != "lookup" {
		t.Fatalf("missing tool metadata: %+v", out)
	}
}

func TestWorkerMCPMalformedConfigurationIsNotAnEmptyDirectory(t *testing.T) {
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"resources":[],"total":0}`) })
	service.cloud = resourceTestCloud(t, func(r *http.Request) string {
		return `{"Result":{"RuntimeId":"r-runtime","Envs":[{"Key":"AGENTKIT_MCP_SERVERS","Value":"bad-json"}]}}`
	})
	service.target = func() deployment.Target {
		return deployment.Target{RuntimeID: "r-runtime", Region: "cn-beijing", ConnectionEpoch: epoch, PlatformEpoch: service.cloud.Epoch()}
	}
	_, err := service.WorkerMCPServers(context.Background(), epoch)
	if err == nil || err.Code != "invalidResponse" {
		t.Fatalf("invalid config was hidden: %v", err)
	}
}

func TestWorkerMCPFollowsCurrentRuntimeToolAndSupportsLegacyWithoutDiscovery(t *testing.T) {
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404); io.WriteString(w, `{}`) })
	service.cloud = resourceTestCloud(t, func(r *http.Request) string {
		if r.URL.Query().Get("Action") == "GetRuntime" {
			return `{"Result":{"RuntimeId":"r-runtime","ToolId":"tool-current"}}`
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Query().Get("Action") != "GetTool" || body["ToolId"] != "tool-current" {
			t.Errorf("read stale or unrelated Worker Tool: %+v", body)
		}
		return `{"Result":{"ToolId":"tool-current","Envs":[{"Key":"MCP_SERVERS_JSON","Value":"[{\"name\":\"worker-search\",\"url\":\"https://example.invalid/mcp\"}]"}]}}`
	})
	service.target = func() deployment.Target {
		return deployment.Target{RuntimeID: "r-runtime", ToolID: "tool-stale", Region: "cn-beijing", ConnectionEpoch: epoch, PlatformEpoch: service.cloud.Epoch()}
	}
	out, err := service.WorkerMCPServers(context.Background(), epoch)
	if err != nil || len(out.Servers) != 1 || out.Servers[0].Name != "worker-search" || out.Servers[0].ToolsKnown {
		t.Fatalf("legacy Worker configuration: %+v %v", out, err)
	}
}
