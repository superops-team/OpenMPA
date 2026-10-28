package usage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestWorkerSkillsReadManagedRelationsAndConfiguredWorkerMounts(t *testing.T) {
	// Card name need not equal any agent node, and Worker mounts are included.
	card := strings.ReplaceAll(resourceTestCard, `"name":"main","capabilities"`, `"name":"public-card-name","capabilities"`)
	card = strings.ReplaceAll(card, `"source":"agent:other","target":"skill-space:unmounted","relation":"calls"`, `"source":"agent:other","target":"skill-space:unmounted","relation":"mounts"`)
	s, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, card) })
	s.cloud = resourceTestCloud(t, func(r *http.Request) string {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Query().Get("Action") != "ListSkillsBySkillSpace" || body["SkillSpaceId"] != "unmounted" || body["PageNumber"] != float64(2) || body["PageSize"] != float64(1) {
			t.Errorf("wrong managed relation request: %+v", body)
		}
		return `{"Result":{"Items":[{"SkillName":"worker-skill","SkillDescription":"configured in Studio"}],"TotalCount":3}}`
	})
	mounts, err := s.DiscoverSkillSpaces(context.Background(), epoch)
	if err != nil || len(mounts.Spaces) != 2 {
		t.Fatalf("worker mounts: %+v %v", mounts, err)
	}
	page, err := s.WorkerSkills(context.Background(), epoch, "cn-beijing", "unmounted", 2, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].Name != "worker-skill" || !page.ServerPaginated || !page.HasMore {
		t.Fatalf("managed skills: %+v %v", page, err)
	}
}
func TestWorkerSkillPermissionsDoNotTriggerLegacyFallback(t *testing.T) {
	s, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, resourceTestCard) })
	calls := 0
	s.cloud = resourceTestCloud(t, func(r *http.Request) string { calls++; return `{"ResponseMetadata":{"Error":{"Code":"LackPolicy"}}}` })
	_, err := s.WorkerSkills(context.Background(), epoch, "cn-beijing", "mounted", 1, 10)
	if err == nil || err.Code != "denied" || calls != 1 {
		t.Fatalf("permission failure hidden: %v, calls=%d", err, calls)
	}
}
