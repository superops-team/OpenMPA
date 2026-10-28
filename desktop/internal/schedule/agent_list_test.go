package schedule

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/connection"
)

func TestListForAgentReadsEveryPageBeforeFiltering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != taskPath {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		switch r.URL.Query().Get("offset") {
		case "0":
			fmt.Fprint(w, `{"items":[{"id":"t1","name":"One","agentId":"agent-a","prompt":"p","enabled":true,"schedule":{"type":"Once","timezone":"Asia/Shanghai"},"delivery":{"channel":"Web","receiveIdType":"chat_id","bestEffort":true},"jitterSeconds":0,"timeoutSeconds":300,"version":1,"createdAt":"2026-10-27T00:00:00Z","updatedAt":"2026-10-27T00:00:00Z"},{"id":"t2","name":"Two","agentId":"agent-b","prompt":"p","enabled":true,"schedule":{"type":"Once","timezone":"Asia/Shanghai"},"delivery":{"channel":"Web","receiveIdType":"chat_id","bestEffort":true},"jitterSeconds":0,"timeoutSeconds":300,"version":1,"createdAt":"2026-10-27T00:00:00Z","updatedAt":"2026-10-27T00:00:00Z"}],"limit":200,"offset":0,"nextOffset":2,"hasMore":true}`)
		case "2":
			fmt.Fprint(w, `{"items":[{"id":"t3","name":"Three","agentId":"agent-a","prompt":"p","enabled":true,"schedule":{"type":"Once","timezone":"Asia/Shanghai"},"delivery":{"channel":"Web","receiveIdType":"chat_id","bestEffort":true},"jitterSeconds":0,"timeoutSeconds":300,"version":1,"createdAt":"2026-10-27T00:00:00Z","updatedAt":"2026-10-27T00:00:00Z"}],"limit":200,"offset":2,"hasMore":false}`)
		default:
			t.Fatalf("unexpected offset %s", r.URL.Query().Get("offset"))
		}
	}))
	defer server.Close()
	c := connection.New()
	epoch, e := c.Bind(connection.Binding{Endpoint: server.URL}, true)
	if e != nil {
		t.Fatal(e)
	}
	service := New(c)
	include := true
	out := service.ListForAgent(AgentListRequest{ConnectionEpoch: epoch, AgentID: "agent-a", IncludeDisabled: &include})
	if !out.OK || out.Value == nil {
		t.Fatalf("list failed: %+v", out)
	}
	if len(out.Value.Items) != 2 || out.Value.Items[0].ID != "t1" || out.Value.Items[1].ID != "t3" {
		t.Fatalf("agent filter missed a page: %+v", out.Value.Items)
	}
	if out.Value.CountAvailable || out.Value.HasMore != nil || out.Value.NextOffset != nil {
		t.Fatalf("filtered page claimed an authoritative total: %+v", out.Value)
	}
	invalid := service.ListForAgent(AgentListRequest{ConnectionEpoch: epoch, AgentID: ""})
	if invalid.OK {
		t.Fatal("empty assistant accepted")
	}
}
