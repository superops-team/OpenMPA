package channels

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func TestSaveConfigKeepsAccountOptionsAndReadsBack(t *testing.T) {
	calls := []string{}
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == "GET" {
			reply(w, `{"channels":[`+record+`]}`)
			return
		}
		if r.Method != "POST" || r.URL.Path != "/api/v1/channels/upsert" {
			t.Fatal(r.Method, r.URL.Path)
		}
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body) != 2 || body["channel"] != "feishu" {
			t.Fatalf("invented CAS, metadata or owner: %+v", body)
		}
		config := body["feishu"].(map[string]interface{})
		if config["domain"] != "lark" || config["bindingType"] != "private" || config["appId"] != "app-1" || config["appSecret"] != "WRITE-ONLY" {
			t.Fatal(config)
		}
		reply(w, `{"success":true,"record":`+record+`}`)
	})
	out := svc.SaveConfig(ConfigRequest{ConnectionEpoch: e, Channel: "feishu", AppID: "app-1", AppSecret: "WRITE-ONLY"})
	if !out.OK || len(calls) != 3 || out.Value.Config.Domain != "lark" || out.Value.BindingType != "private" {
		t.Fatalf("%+v %v", out, calls)
	}
	encoded, _ := json.Marshal(out)
	if strings.Contains(string(encoded), "WRITE-ONLY") || strings.Contains(string(encoded), "NEVER-IPC") {
		t.Fatal("saved config leaked provider secrets")
	}
}

func TestSingleProviderConfigCannotAddSecondAccount(t *testing.T) {
	for _, provider := range []string{"dingtalk", "wecom"} {
		t.Run(provider, func(t *testing.T) {
			calls := 0
			svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" {
					t.Fatal("unexpected replacement")
				}
				reply(w, `{"channels":[{"channel":"`+provider+`","appId":"a"},{"channel":"`+provider+`","appId":"b"}]}`)
			})
			req := ConfigRequest{ConnectionEpoch: e, Channel: provider, AppID: "a", Secret: "WRITE-ONLY"}
			if provider == "dingtalk" {
				req.Secret = ""
				req.ClientSecret = "WRITE-ONLY"
			}
			out := svc.SaveConfig(req)
			if out.OK || out.Error.Code != "singleAccountRequired" || calls != 1 {
				t.Fatalf("%+v", out)
			}
		})
	}
}

func TestSetEnabledIsDedicatedFeishuPatch(t *testing.T) {
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" || r.URL.Path != "/api/v1/channels/feishu/accounts/app-1" {
			t.Fatal(r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		if string(b) != `{"enabled":false}` {
			t.Fatalf("enable must not upsert config: %s", b)
		}
		reply(w, `{"channel":"feishu","appId":"app-1","enabled":false,"appSecret":"WRITE-ONLY"}`)
	})
	out := svc.SetEnabled(EnabledRequest{ConnectionEpoch: e, Channel: "feishu", AppID: "app-1", Enabled: false})
	if !out.OK || out.Value.Enabled {
		t.Fatalf("%+v", out)
	}
}

func TestCompleteProfileSuccessAndUpgradeLimits(t *testing.T) {
	calls := 0
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.Method {
		case "PUT":
			if r.URL.Path != "/api/v1/channels/feishu/accounts/app-1/profile" {
				t.Fatal(r.URL.Path)
			}
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body) != 5 || body["expectedRevision"] != float64(3) || body["model"] != "test-model" || body["agentsMd"] != "😀" {
				t.Fatalf("invalid full profile: %+v", body)
			}
			refs := body["skills"].([]interface{})
			if len(refs) != 1 || refs[0] != "skill-a" {
				t.Fatal("server reference normalization lost")
			}
		case "GET":
			if r.URL.Path != "/api/v1/channels/feishu/accounts/app-1/profile" {
				t.Fatal(r.URL.Path)
			}
		case "POST":
			if r.URL.Path != "/api/v1/channels/feishu/accounts/app-1/sessions/session-1/profile-upgrade" {
				t.Fatal("invented upgrade route", r.URL.Path)
			}
			b, _ := io.ReadAll(r.Body)
			if len(b) != 0 || r.Header.Get("Idempotency-Key") != "" || r.Header.Get("If-Match") != "" {
				t.Fatal("invented upgrade CAS/idempotency")
			}
		}
		reply(w, `{"appId":"app-1","revision":4,"agentsMd":"😀","model":"test-model","skills":["skill-a"],"mcp":null,"role":"NEVER-IPC","toolPolicy":{"token":"NEVER-IPC"}}`)
	})
	md, model := "😀", "test-model"
	skills := []string{" skill-a ", "skill-a"}
	saved := svc.SaveProfile(ProfileSaveRequest{ConnectionEpoch: e, Channel: "feishu", AppID: "app-1", ExpectedRevision: 3, AgentsMD: &md, Model: &model, Skills: &skills})
	if !saved.OK || saved.Value.Revision != 4 || saved.Value.MCP != nil {
		t.Fatalf("%+v", saved)
	}
	read := svc.ReadProfile(AccountRequest{e, "feishu", "app-1"})
	if !read.OK {
		t.Fatalf("%+v", read)
	}
	upgraded := svc.UpgradeProfile(UpgradeRequest{ConnectionEpoch: e, Channel: "feishu", AppID: "app-1", SessionID: "session-1"})
	if !upgraded.OK || upgraded.Value.HasCAS || upgraded.Value.AdvancesExecutionVersion || calls != 3 {
		t.Fatalf("%+v", upgraded)
	}
	b, _ := json.Marshal(saved)
	if strings.Contains(string(b), "NEVER-IPC") || strings.Contains(string(b), "toolPolicy") {
		t.Fatal("invented profile permissions")
	}
}

func TestPermissionRoutesAreExactAndFeishuOnly(t *testing.T) {
	calls := 0
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v1/channels/chat-permissions" {
			t.Fatal(r.URL.Path)
		}
		p := `{"channel":"feishu","appId":"app-1","chatId":"chat-1","groupSessionScope":"group_topic_sender","avatar":"https://images.example/chat?token=NEVER-IPC","metadata":{"secret":"NEVER-IPC"}}`
		if r.Method == "POST" {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body) != 4 || body["channel"] != "feishu" || body["appId"] != "app-1" || body["chatId"] != "chat-1" || body["groupSessionScope"] != "group_topic_sender" {
				t.Fatal(body)
			}
			reply(w, p)
			return
		}
		if r.URL.Query().Get("channel") != "feishu" || r.URL.Query().Get("appId") != "app-1" {
			t.Fatal("permission was not scoped to exact account")
		}
		if r.Method == "DELETE" {
			if r.URL.Query().Get("chat_id") != "chat-1" {
				t.Fatal("incorrect provider delete query")
			}
			reply(w, `{"success":true}`)
		} else {
			reply(w, `{"permissions":[`+p+`]}`)
		}
	})
	if !svc.Permissions(AccountRequest{e, "feishu", "app-1"}).OK {
		t.Fatal("list failed")
	}
	req := PermissionRequest{ConnectionEpoch: e, Channel: "feishu", AppID: "app-1", ChatID: "chat-1", GroupSessionScope: "group_topic_sender"}
	out := svc.SavePermission(req)
	if !out.OK {
		t.Fatalf("%+v", out)
	}
	encoded, _ := json.Marshal(out)
	if strings.Contains(string(encoded), "NEVER-IPC") {
		t.Fatal("permission leaked URL query")
	}
	if !svc.DeletePermission(req).OK || calls != 3 {
		t.Fatal("delete failed")
	}
	req.Channel = "wecom"
	if svc.SavePermission(req).OK || svc.DeletePermission(req).OK || calls != 3 {
		t.Fatal("invented non-Feishu permissions")
	}
}

func TestDiagnosticsHasNoGatewaySecretsAndPreservesDeliveryEvidence(t *testing.T) {
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/channels/feishu/diagnostics" || r.URL.Query().Get("appId") != "app-1" {
			t.Fatal("ambiguous diagnostic")
		}
		reply(w, `{"channel":"feishu","appId":"app-1","configured":true,"gatewayConfigured":true,"routeConfigured":true,"deliveryHealth":"not_observed","bindingReady":true,"gatewayEndpoint":"https://gateway?token=NEVER-IPC","missingConfiguration":["runtime_api_key","NEVER-IPC"]}`)
	})
	out := svc.Diagnostics(AccountRequest{e, "feishu", "app-1"})
	if !out.OK || out.Value.DeliveryHealth != "not_observed" || len(out.Value.MissingConfiguration) != 1 {
		t.Fatalf("%+v", out)
	}
	encoded, _ := json.Marshal(out)
	if strings.Contains(string(encoded), "NEVER-IPC") || strings.Contains(string(encoded), "bindingReady") {
		t.Fatal("diagnostic included raw secrets or false readiness")
	}
}

func TestPrivateMutationSuccessReadsBackChangedSlot(t *testing.T) {
	for _, reset := range []bool{false, true} {
		name := "revoke"
		if reset {
			name = "reset"
		}
		t.Run(name, func(t *testing.T) {
			reads, deletes := 0, 0
			svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
				prefix := "/api/v1/channels/feishu/accounts/app-1/private-user"
				if r.Method == "GET" {
					reads++
					if r.URL.Path != prefix {
						t.Fatal(r.URL.Path)
					}
					if reset && deletes > 0 {
						reply(w, `{"appId":"app-1","slot":null,"authorization":{"status":"not_authorized"}}`)
						return
					}
					status := "ready"
					if deletes > 0 {
						status = "revoked"
					}
					reply(w, `{"appId":"app-1","slot":{"version":"slot-v1","chat_id":"chat","sender_id":"sender","runtime_user_id":"user"},"authorization":{"status":"`+status+`"}}`)
					return
				}
				deletes++
				if r.URL.Query().Get("expected_version") != "slot-v1" {
					t.Fatal("missing version")
				}
				if reset {
					if r.URL.Path != prefix {
						t.Fatal(r.URL.Path)
					}
					reply(w, `{"appId":"app-1","reset":true}`)
				} else {
					if r.URL.Path != prefix+"/authorization" {
						t.Fatal(r.URL.Path)
					}
					reply(w, `{"appId":"app-1","revoked":true}`)
				}
			})
			if !svc.PrivateUser(AccountRequest{e, "feishu", "app-1"}).OK {
				t.Fatal("read failed")
			}
			req := PrivateMutationRequest{ConnectionEpoch: e, Channel: "feishu", AppID: "app-1", ExpectedVersion: "slot-v1"}
			var out PrivateMutationResult
			if reset {
				out = svc.ResetPrivateUser(req)
			} else {
				out = svc.RevokePrivateAuthorization(req)
			}
			if !out.OK || reads != 3 || deletes != 1 || out.Value.Reset != reset || out.Value.Revoked == reset {
				t.Fatalf("%+v reads=%d deletes=%d", out, reads, deletes)
			}
			if reset && out.Value.Current.Slot != nil {
				t.Fatal("reset reused old private generation")
			}
		})
	}
}

func TestPrivateVersionChangedBeforeDeletePreservesConfirmation(t *testing.T) {
	reads := 0
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Fatal("mutated changed slot")
		}
		reads++
		version := "v1"
		if reads > 1 {
			version = "v2"
		}
		reply(w, `{"appId":"app-1","slot":{"version":"`+version+`"},"authorization":{"status":"ready"}}`)
	})
	_ = svc.PrivateUser(AccountRequest{e, "feishu", "app-1"})
	req := PrivateMutationRequest{ConnectionEpoch: e, Channel: "feishu", AppID: "app-1", ExpectedVersion: "v1"}
	out := svc.RevokePrivateAuthorization(req)
	if out.OK || out.Error.HTTPStatus != 409 || reads != 2 || req.ExpectedVersion != "v1" {
		t.Fatalf("%+v", out)
	}
}

func TestOwnerGateDoesNotReplaceRuntimeAdminAuthorization(t *testing.T) {
	calls := 0
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) { calls++; reply(w, `{"channels":[]}`) })
	svc.provider = svc.provider.(ownedFixtureClient).Client
	req := AccountRequest{e, "feishu", "app-1"}
	if svc.ReadProfile(req).Error.Code != "ownerIdentityRequired" || svc.PrivateUser(req).Error.Code != "ownerIdentityRequired" {
		t.Fatal("untrusted actor became owner")
	}
	if svc.CreateBinding(BindingCreateRequest{ConnectionEpoch: e, OperationID: "op", Channel: "feishu", Mode: "qr"}).Error.Code != "ownerIdentityRequired" || calls != 0 {
		t.Fatal("untrusted owner reached binding route")
	}
	if !svc.List(ScopeRequest{e}).OK || calls != 1 {
		t.Fatal("owner gate incorrectly narrowed Runtime admin list")
	}
}

func TestCreateConflictHasNoAutomaticNewOperation(t *testing.T) {
	calls := 0
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(409)
		reply(w, `{"detail":"Registration uncertain TOKEN-SECRET"}`)
	})
	req := BindingCreateRequest{ConnectionEpoch: e, OperationID: "op", Channel: "feishu", Mode: "manual", AppID: "app-1", AppSecret: "WRITE-ONLY"}
	out := svc.CreateBinding(req)
	if out.OK || out.Error.Code != "registrationUncertain" || out.Error.RetryDisposition != "verifyOnly" {
		t.Fatalf("%+v", out)
	}
	_ = svc.CreateBinding(req)
	if calls != 1 {
		t.Fatal("registration replay after uncertain")
	}
}

func TestVerifiedTestOwnerUsesActualConnectionProvenance(t *testing.T) {
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-User-Id") != "test-owner" {
			t.Fatal("owner was not resolved by explicit test connection contract")
		}
		reply(w, `{"appId":"app-1","revision":0,"agentsMd":null,"model":null,"skills":null,"mcp":null}`)
	})
	conn := svc.provider.(ownedFixtureClient).Client
	svc.provider = conn
	if conn.HasChannelOwnerIdentity() {
		t.Fatal("channel management key alone became owner identity")
	}
	if err := conn.SetBusinessAuth(dto.IdentityRequest{ConnectionEpoch: e, AuthMode: "explicitTest", TestSubject: "test-owner", VerifiedTestDeployment: true}); err != nil {
		t.Fatal(err)
	}
	if !conn.HasChannelOwnerIdentity() {
		t.Fatal("explicit verified test deployment owner was rejected")
	}
	out := svc.ReadProfile(AccountRequest{conn.State().ConnectionEpoch, "feishu", "app-1"})
	if !out.OK {
		t.Fatalf("%+v", out)
	}
}

func TestProfilePythonCharactersAcceptEmojiLimit(t *testing.T) {
	calls := 0
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body profilePayload
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.AgentsMD == nil || len([]rune(*body.AgentsMD)) != 32000 {
			t.Fatal("profile character limit used UTF-16 units")
		}
		reply(w, `{"appId":"app-1","revision":1,"agentsMd":null,"skills":null,"mcp":null,"model":null}`)
	})
	md := strings.Repeat("😀", 32000)
	out := svc.SaveProfile(ProfileSaveRequest{ConnectionEpoch: e, Channel: "feishu", AppID: "app-1", AgentsMD: &md})
	if !out.OK || calls != 1 {
		t.Fatalf("valid Python character length rejected: %+v", out)
	}
}
