package usage

import (
	"context"
	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
	"io"
	"net/http"
	"testing"
)

func TestBoundSkillSpaceShowsCloudIdentity(t *testing.T) {
	s, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, resourceTestCard) })
	s.cloud = resourceTestCloud(t, func(r *http.Request) string {
		return `{"Result":{"Id":"mounted","Name":"my-worker-skills","Description":"Worker skills","ProjectName":"default"}}`
	})
	s.target = func() deployment.Target {
		return deployment.Target{RuntimeID: "r-runtime", Region: "cn-beijing", ConnectionEpoch: epoch, PlatformEpoch: s.cloud.Epoch()}
	}
	out, err := s.BoundSkillSpaces(context.Background(), epoch)
	if err != nil || len(out.Spaces) != 1 || out.Spaces[0].Name != "my-worker-skills" || out.Spaces[0].ID != "mounted" {
		t.Fatalf("space identity: %+v %v", out, err)
	}
}
func TestEmptyManagedRelationsStillReadRuntimeSkillDirectory(t *testing.T) {
	s, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, resourceTestCard) })
	s.cloud = resourceTestCloud(t, func(r *http.Request) string {
		switch r.URL.Query().Get("Action") {
		case "ListSkillsBySkillSpace":
			return `{"Result":{"Items":[],"TotalCount":0}}`
		case "GetSkillSpace":
			return `{"Result":{"Id":"mounted","Name":"worker-space"}}`
		case "ListSkillsBySpaceId":
			return `{"Result":{"Items":[{"Name":"runtime-skill","Description":"Available to Worker"}]}}`
		case "GetSkillInfo":
			return `{"Result":{"SkillName":"runtime-skill","Description":"Available to Worker"}}`
		}
		t.Error("unexpected action")
		return `{}`
	})
	out, err := s.WorkerSkills(context.Background(), epoch, "cn-beijing", "mounted", 1, 20)
	if err != nil || out.TotalCount != 1 || out.Items[0].Name != "runtime-skill" {
		t.Fatalf("runtime directory hidden: %+v %v", out, err)
	}
}
