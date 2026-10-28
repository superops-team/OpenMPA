package usage

import (
	"context"
	"encoding/json"
	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
	"io"
	"net/http"
	"strings"
	"testing"
)

func managedService(t *testing.T, cloudReply func(*http.Request) string, bound bool) (*Service, uint64) {
	s, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if bound {
			io.WriteString(w, `{"space_id":"as-test","region":"cn-beijing","revision":1}`)
		} else {
			io.WriteString(w, `{"space_id":null,"region":"cn-beijing","revision":1}`)
		}
	})
	s.cloud = resourceTestCloud(t, cloudReply)
	s.target = func() deployment.Target {
		return deployment.Target{RuntimeID: "r-runtime", Region: "cn-beijing", ConnectionEpoch: epoch, PlatformEpoch: s.cloud.Epoch()}
	}
	return s, epoch
}
func TestA2ASpaceAndRegistrationMatchStudioContracts(t *testing.T) {
	calls := 0
	s, epoch := managedService(t, func(r *http.Request) string {
		calls++
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Query().Get("Action") {
		case "CreateA2aSpace":
			if body["Name"] != "Team" || body["ProjectName"] != "default" || body["IntentEnabled"] != true {
				t.Errorf("wrong space payload: %+v", body)
			}
			return `{"Result":{"Id":"as-test"}}`
		case "CreateA2aAgent":
			if body["A2aSpaceId"] != "as-test" || body["Source"] != "Standard" || body["SetDefaultVersion"] != true || body["StandardConfig"].(map[string]any)["ImportType"] != "json" {
				t.Errorf("wrong registration: %+v", body)
			}
			return `{"Result":{"Id":"a-test"}}`
		}
		t.Error("unexpected action")
		return `{}`
	}, false)
	if out, err := s.CreateCollaborationSpace(context.Background(), SpaceCreateRequest{epoch, "Team", "default", true}); err != nil || out.ID != "as-test" {
		t.Fatalf("space: %+v %v", out, err)
	}
	r := AgentRegisterRequest{ConnectionEpoch: epoch, SpaceID: "as-test", ProjectName: "default", Source: "Standard", ImportType: "json", AgentCard: `{"name":"Agent","url":"https://example.invalid/a2a","version":"1"}`}
	if out, err := s.RegisterCollaborationAgent(context.Background(), r); err != nil || out.ID != "a-test" {
		t.Fatalf("registration: %+v %v", out, err)
	}
	for _, card := range []string{`[]`, `{"name":"a","url":"http://private","version":"1"}`, `{"name":"a","url":"https://user:secret@example.invalid","version":"1"}`} {
		r.AgentCard = card
		if _, err := s.RegisterCollaborationAgent(context.Background(), r); err == nil {
			t.Fatal("invalid Card accepted")
		}
	}
	if calls != 2 {
		t.Fatal("invalid input reached cloud")
	}
}
func TestA2ADeletionValidatesMembershipAndSpaceState(t *testing.T) {
	for _, mode := range []string{"bound", "nonempty", "empty", "wrong-member"} {
		t.Run(mode, func(t *testing.T) {
			deleted := false
			s, epoch := managedService(t, func(r *http.Request) string {
				switch r.URL.Query().Get("Action") {
				case "GetA2aSpace":
					return `{"Result":{"Id":"as-test"}}`
				case "ListA2aAgents":
					if mode == "nonempty" {
						return `{"Result":{"Items":[{"Id":"a-x"}],"TotalCount":1}}`
					}
					return `{"Result":{"Items":[],"TotalCount":0}}`
				case "GetA2aAgent":
					return `{"Result":{"Id":"a-test","A2aSpaceId":"as-other","Source":"Runtime"}}`
				case "DeleteA2aSpace", "DeleteA2aAgent":
					deleted = true
					return `{"Result":{}}`
				}
				t.Error("unexpected action")
				return `{}`
			}, mode == "bound")
			var err error
			if mode == "wrong-member" {
				if e := s.DeleteCollaborationAgent(context.Background(), CollaborationMemberRequest{epoch, "as-test", "a-test"}); e != nil {
					err = e
				}
			} else {
				if e := s.DeleteCollaborationSpace(context.Background(), CollaborationSpaceRequest{epoch, "as-test"}); e != nil {
					err = e
				}
			}
			if mode == "empty" {
				if err != nil || !deleted {
					t.Fatalf("empty space not deleted: %v", err)
				}
			} else if err == nil || deleted {
				t.Fatal("unsafe delete reached cloud")
			}
		})
	}
}
func TestA2ADetailDropsCardCredentialsAndUsesPublishedSkills(t *testing.T) {
	s, epoch := managedService(t, func(r *http.Request) string {
		return `{"Result":{"Id":"a-test","A2aSpaceId":"as-test","Source":"Standard","Name":"Peer","AgentCard":{"url":"https://example.invalid/a2a?token=do-not-copy","authentication":{"token":"do-not-copy"},"skills":[{"id":"review","name":"Review","description":"Review code","tags":[],"examples":[]}]}}}`
	}, false)
	out, err := s.CollaborationAgentDetail(context.Background(), CollaborationMemberRequest{epoch, "as-test", "a-test"})
	if err != nil || len(out.Skills) != 1 {
		t.Fatalf("detail: %+v %v", out, err)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "do-not-copy") || strings.Contains(string(raw), "authentication") {
		t.Fatal("Card credentials escaped")
	}
}
func TestA2AMutationMalformedSuccessStaysUncertain(t *testing.T) {
	s, epoch := managedService(t, func(r *http.Request) string { return `{"Result":{}}` }, false)
	_, err := s.CreateCollaborationSpace(context.Background(), SpaceCreateRequest{epoch, "Team", "default", false})
	if err == nil || err.Code != "outcomeUnknown" {
		t.Fatalf("malformed success: %v", err)
	}
	s.conn.Disconnect()
	if _, err := s.CreateCollaborationSpace(context.Background(), SpaceCreateRequest{epoch, "Team", "default", false}); err == nil || err.Code != "staleConnection" {
		t.Fatal("stale mutation accepted")
	}
}
