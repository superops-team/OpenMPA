package usage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
)

func TestDeleteSkillValidatesMountedMembershipBeforeDeleting(t *testing.T) {
	for _, tc := range []struct {
		name, space, skill, receipt, want string
		writes                            int
	}{
		{"success", "mounted", "skill-owned", `{"Result":{}}`, "", 1},
		{"unmounted", "unmounted", "skill-owned", "", "resourceNotVisible", 0},
		{"unrelated skill", "mounted", "skill-other", "", "resourceNotVisible", 0},
		{"missing id", "mounted", "", "", "invalidInput", 0},
		{"lost receipt", "mounted", "skill-owned", `{`, "outcomeUnknown", 1},
		{"denied", "mounted", "skill-owned", `{"ResponseMetadata":{"Error":{"Code":"LackPolicy"}}}`, "denied", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, resourceTestCard) })
			writes := 0
			s.cloud = resourceTestCloud(t, func(r *http.Request) string {
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				switch r.URL.Query().Get("Action") {
				case "ListSkillsBySkillSpace":
					if body["SkillSpaceId"] != "mounted" {
						t.Error("read an unmounted space")
					}
					return `{"Result":{"Items":[{"SkillId":"skill-owned","SkillName":"review-code"}],"TotalCount":1}}`
				case "DeleteSkill":
					writes++
					if len(body) != 1 || body["Id"] != "skill-owned" || r.Method != http.MethodPost {
						t.Errorf("incorrect deletion: %+v", body)
					}
					return tc.receipt
				default:
					t.Errorf("unexpected action: %s", r.URL.Query().Get("Action"))
					return `{}`
				}
			})
			s.target = func() deployment.Target {
				return deployment.Target{RuntimeID: "r-runtime", Region: "cn-beijing", ConnectionEpoch: epoch, PlatformEpoch: s.cloud.Epoch()}
			}
			err := s.DeleteSkill(context.Background(), SkillDeleteRequest{epoch, tc.space, tc.skill})
			if (err == nil && tc.want != "") || (err != nil && err.Code != tc.want) || writes != tc.writes {
				t.Fatalf("delete: %v, writes=%d; want %s, %d", err, writes, tc.want, tc.writes)
			}
		})
	}
}

func TestDeleteSkillRejectsAccountChangesDuringMembershipRead(t *testing.T) {
	s, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, resourceTestCard) })
	writes := 0
	s.cloud = resourceTestCloud(t, func(r *http.Request) string {
		if r.URL.Query().Get("Action") == "DeleteSkill" {
			writes++
		}
		s.cloud.SignOut()
		return `{"Result":{"Items":[{"SkillId":"skill-owned","SkillName":"review-code"}],"TotalCount":1}}`
	})
	s.target = func() deployment.Target {
		return deployment.Target{RuntimeID: "r-runtime", Region: "cn-beijing", ConnectionEpoch: epoch, PlatformEpoch: s.cloud.Epoch()}
	}
	err := s.DeleteSkill(context.Background(), SkillDeleteRequest{epoch, "mounted", "skill-owned"})
	if err == nil || err.Code != "staleConnection" || writes != 0 {
		t.Fatalf("account changed: %v, writes=%d", err, writes)
	}
}
