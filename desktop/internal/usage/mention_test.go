package usage

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestBridgeCheckedMentionRequiresCoherentBindingAndAlias(t *testing.T) {
	for _, mode := range []string{"valid", "binding", "changed-binding", "alias", "missing", "self", "scope"} {
		t.Run(mode, func(t *testing.T) {
			var bindingReads atomic.Int32
			var core *Service
			var epoch uint64
			core, epoch = collaborationTestService(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Error("mention must not write or execute")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				switch r.URL.Path {
				case collaborationRegistry + "/binding":
					read := bindingReads.Add(1)
					revision := 1
					if mode == "changed-binding" && read == 2 {
						revision = 2
					}
					if mode == "scope" && read == 2 {
						if err := core.conn.SetBusinessScope(epoch, "/business/new"); err != nil {
							t.Error(err)
						}
					}
					fmt.Fprintf(w, `{"space_id":"as-team","space_name":"Team","region":"cn-beijing","revision":%d}`, revision)
				case collaborationRegistry + "/agents":
					id, runtime := "a-writer", "r-peer"
					if mode == "missing" {
						id = "a-other"
					}
					if mode == "self" {
						runtime = "r-self"
					}
					fmt.Fprintf(w, `{"items":[{"Id":%q,"Name":"Writer","Status":"running","Alias":"writer","AliasRevision":4,"AliasAvailable":true,"RuntimeConfig":{"RuntimeId":%q}}],"page":1,"page_size":50,"total":1}`, id, runtime)
				default:
					t.Errorf("unexpected route: %s", r.URL.Path)
				}
			})
			request := CollaborationMentionRequest{ConnectionEpoch: epoch, SpaceID: "as-team", BindingRevision: 1, AgentID: "a-writer", AliasRevision: 4, Page: 1, Task: "review"}
			if mode == "binding" {
				request.BindingRevision = 2
			}
			if mode == "alias" {
				request.AliasRevision = 3
			}
			result := testBridge(core).CollaborationMention(context.Background(), request)
			if mode == "valid" {
				if !result.OK || result.Value.Text != "@writer review" || bindingReads.Load() != 2 {
					t.Fatalf("mention: %+v", result)
				}
			} else if result.OK || result.Value != nil || result.Error == nil {
				t.Fatalf("stale mention accepted: %+v", result)
			}
		})
	}
}
