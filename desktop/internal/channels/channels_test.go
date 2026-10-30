package channels

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

// The fixture explicitly models an already verified owner proxy. Production
// owner provenance is supplied solely by connection.Client, never by IPC input.
type ownedFixtureClient struct{ *connection.Client }

func (ownedFixtureClient) HasChannelOwnerIdentity() bool { return true }

func harness(t *testing.T, h http.HandlerFunc) (*Channels, uint64) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-MPA-Channel-Key") != "channel-test-key" {
			t.Errorf("wrong channel auth family: %v", r.Header)
		}
		if r.Header.Get("Authorization") != "Bearer ingress-test-key" {
			t.Errorf("missing separate ingress auth")
		}
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}))
	t.Cleanup(server.Close)
	conn := connection.New()
	epoch, err := conn.Bind(connection.Binding{Endpoint: server.URL, RuntimeKey: "ingress-test-key"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.ConfigureChannelAuth(dto.ChannelAuthRequest{ConnectionEpoch: epoch, AuthMode: "runtimeKey", ChannelKey: "channel-test-key"}); err != nil {
		t.Fatal(err)
	}
	svc := New(conn)
	svc.provider = ownedFixtureClient{conn}
	return svc, conn.State().ConnectionEpoch
}
func reply(w http.ResponseWriter, body string) { _, _ = io.WriteString(w, body) }

const record = `{"channel":"feishu","appId":"app-1","enabled":true,"bindingType":"private","config":{"domain":"lark","appSecret":"NEVER-IPC","gatewayBaseUrl":"https://example.org?token=NEVER-IPC"},"ownerUserId":"owner","appSecret":"NEVER-IPC"}`

func TestListWhiteListAndCapabilities(t *testing.T) {
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/channels":
			reply(w, `{"channels":[`+record+`]}`)
		case "/api/v1/channels/capabilities":
			reply(w, `{"version":"1.3","channels":["feishu","wecom","dingtalk","evil"],"multiBotChannels":["feishu","wecom"],"groupPermissionChannels":["feishu","dingtalk"],"botProfiles":true,"bindingReady":true,"bindingError":"NEVER-IPC","authMode":"runtime_key"}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	out := svc.List(ScopeRequest{e})
	if !out.OK || len(out.Value.Channels) != 1 {
		t.Fatalf("%+v", out)
	}
	encoded, _ := json.Marshal(out)
	if strings.Contains(string(encoded), "NEVER-IPC") || strings.Contains(string(encoded), "gatewayBaseUrl") {
		t.Fatalf("secret response: %s", encoded)
	}
	if out.Value.ManagementScope != "runtimeAdmin" {
		t.Fatal("admin scope was hidden")
	}
	cap := svc.Capabilities(ScopeRequest{e})
	if !cap.OK || len(cap.Value.MultiBotChannels) != 1 || cap.Value.MultiBotChannels[0] != "feishu" || len(cap.Value.GroupPermissionChannels) != 1 || cap.Value.UpgradeHasCAS || cap.Value.UpgradeAdvancesExecutionVersion {
		t.Fatalf("unsupported capability: %+v", cap)
	}
}

func TestProviderBindingPathsAndSecretInput(t *testing.T) {
	cases := []struct{ channel, mode, path, secretField string }{
		{"feishu", "manual", "/feishu/bindings/manual", "appSecret"},
		{"dingtalk", "manual", "/dingtalk/bindings/manual", "clientSecret"},
		{"wecom", "manual", "/wecom/bindings", "secret"},
		{"feishu", "qr", "/feishu/bindings", ""},
		{"dingtalk", "qr", "/dingtalk/bindings", ""},
	}
	for _, tc := range cases {
		t.Run(tc.channel+tc.mode, func(t *testing.T) {
			calls := 0
			svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "POST" || r.URL.Path != "/api/v1/channels"+tc.path {
					t.Fatalf("wrong path %s %s", r.Method, r.URL.Path)
				}
				b, _ := io.ReadAll(r.Body)
				var body map[string]interface{}
				_ = json.Unmarshal(b, &body)
				if tc.secretField != "" && body[tc.secretField] != "TOP-SECRET" {
					t.Fatalf("credential not sent: %s", b)
				}
				if _, ok := body["ownerUserId"]; ok {
					t.Fatal("client supplied owner")
				}
				reply(w, `{"id":"binding-1","status":"PENDING","appId":"app-1","appSecret":"TOP-SECRET","clientSecret":"TOP-SECRET","secret":"TOP-SECRET","loginUrl":"https://auth.example/authorize?token=TOP-SECRET#frag","lastErrorMessage":"TOP-SECRET"}`)
			})
			req := BindingCreateRequest{ConnectionEpoch: e, OperationID: "op-1", Channel: tc.channel, Mode: tc.mode}
			switch tc.secretField {
			case "appSecret":
				req.AppID = "app-1"
				req.AppSecret = "TOP-SECRET"
			case "clientSecret":
				req.ClientID = "app-1"
				req.ClientSecret = "TOP-SECRET"
			case "secret":
				req.BotID = "app-1"
				req.Secret = "TOP-SECRET"
			}
			out := svc.CreateBinding(req)
			if !out.OK || out.Value.ID != "binding-1" {
				t.Fatalf("%+v", out)
			}
			data, _ := json.Marshal(out)
			if strings.Contains(string(data), "TOP-SECRET") || strings.Contains(string(data), "?token") {
				t.Fatalf("output exposed secret: %s", data)
			}
			_ = svc.CreateBinding(req)
			if calls != 1 {
				t.Fatal("duplicated registration")
			}
		})
	}
}

func TestBindingIntentAndFullRetryPaths(t *testing.T) {
	for _, provider := range []string{"feishu", "dingtalk"} {
		t.Run(provider, func(t *testing.T) {
			calls := []string{}
			svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, r.Method+" "+r.URL.Path)
				reply(w, `{"id":"b-1","status":"SCANNED"}`)
			})
			r := BindingRequest{ConnectionEpoch: e, OperationID: "op-1", Channel: provider, BindingID: "b-1"}
			if svc.ReadBinding(r).OK || len(calls) != 0 {
				t.Fatal("poll without operation intent")
			}
			r.ResumeConfirmed = true
			if !svc.ReadBinding(r).OK {
				t.Fatal("explicit prior-operation recovery rejected")
			}
			r.ResumeConfirmed = false
			if !svc.RetryBinding(r).OK {
				t.Fatal("explicit retry rejected")
			}
			prefix := "/api/v1/channels/" + provider + "/bindings/b-1"
			if len(calls) != 2 || calls[0] != "GET "+prefix || calls[1] != "POST "+prefix+"/retry" {
				t.Fatalf("%v", calls)
			}
		})
	}
}

func TestUncertainRegistrationCannotBlindRetry(t *testing.T) {
	calls := 0
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		reply(w, `{"id":"b-1","status":"FAILED","lastErrorCode":"REGISTRATION_UNCERTAIN","lastErrorMessage":"secret"}`)
	})
	r := BindingRequest{ConnectionEpoch: e, OperationID: "op-1", Channel: "feishu", BindingID: "b-1", ResumeConfirmed: true}
	out := svc.ReadBinding(r)
	if !out.OK || out.Value.State != "unknown" || out.Value.ErrorCode != "REGISTRATION_UNCERTAIN" {
		t.Fatalf("%+v", out)
	}
	if svc.RetryBinding(r).OK || calls != 1 {
		t.Fatal("blind retry of uncertain registration")
	}
	r.GatewayVerified = true
	_ = svc.RetryBinding(r)
	if calls != 2 {
		t.Fatal("confirmed gateway verification must allow explicit retry")
	}
}

func TestProfileFullNullableCASAndUnicode(t *testing.T) {
	calls := 0
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "PUT" || r.URL.Path != "/api/v1/channels/feishu/accounts/app-1/profile" {
			t.Fatal(r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		var wire map[string]interface{}
		_ = json.Unmarshal(b, &wire)
		if len(wire) != 5 || wire["agentsMd"] != nil || wire["model"] != nil || wire["mcp"] != nil || wire["expectedRevision"] != float64(7) {
			t.Fatalf("incomplete nullable CAS payload: %s", b)
		}
		v, ok := wire["skills"].([]interface{})
		if !ok || len(v) != 0 {
			t.Fatalf("[] lost: %s", b)
		}
		w.WriteHeader(409)
		reply(w, `{"detail":"TOP-SECRET"}`)
	})
	empty := []string{}
	req := ProfileSaveRequest{ConnectionEpoch: e, Channel: "feishu", AppID: "app-1", Skills: &empty, ExpectedRevision: 7}
	out := svc.SaveProfile(req)
	if out.OK || out.Error.HTTPStatus != 409 || calls != 1 {
		t.Fatalf("%+v", out)
	}
	data, _ := json.Marshal(out)
	if strings.Contains(string(data), "TOP-SECRET") {
		t.Fatal("raw CAS error")
	}
	bad := "https://model.example/token"
	req.Model = &bad
	if svc.SaveProfile(req).OK || calls != 1 {
		t.Fatal("URL model submitted")
	}
	text := strings.Repeat("😀", 32001)
	req.Model = nil
	req.AgentsMD = &text
	if svc.SaveProfile(req).OK || calls != 1 {
		t.Fatal("Python character limit ignored")
	}
}

func TestForbiddenProviderActionsAndIdentifiers(t *testing.T) {
	calls := 0
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) { calls++; reply(w, `{}`) })
	if svc.CreateBinding(BindingCreateRequest{ConnectionEpoch: e, OperationID: "op", Channel: "wecom", Mode: "qr"}).OK {
		t.Fatal("invented wecom QR")
	}
	if svc.SetEnabled(EnabledRequest{ConnectionEpoch: e, Channel: "dingtalk", AppID: "id", Enabled: true}).OK {
		t.Fatal("invented enabled")
	}
	if svc.Permissions(AccountRequest{e, "dingtalk", "id"}).OK {
		t.Fatal("invented group permissions")
	}
	if svc.ReadProfile(AccountRequest{e, "wecom", "id"}).OK {
		t.Fatal("invented profile")
	}
	if svc.Diagnostics(AccountRequest{e, "feishu", ""}).OK {
		t.Fatal("ambiguous diagnostics")
	}
	if svc.Remove(AccountRequest{e, "feishu", "../other"}).OK {
		t.Fatal("unsafe exact account")
	}
	if calls != 0 {
		t.Fatal("invalid action reached provider")
	}
}

func TestPrivateSlotOriginalVersionAndConflict(t *testing.T) {
	calls := []string{}
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		if r.Method == "GET" {
			reply(w, `{"appId":"app-1","slot":{"version":"v-original","chat_id":"chat","encrypted_state":"NEVER-IPC"},"authorization":{"status":"authorized","verification_url":"https://auth?token=NEVER-IPC"}}`)
			return
		}
		if r.URL.Query().Get("expected_version") != "v-original" {
			t.Fatal("rewrote original version")
		}
		w.WriteHeader(409)
		reply(w, `{"detail":"private_bot_slot_changed TOP-SECRET"}`)
	})
	request := PrivateMutationRequest{ConnectionEpoch: e, Channel: "feishu", AppID: "app-1", ExpectedVersion: "v-original"}
	if svc.ResetPrivateUser(request).OK || len(calls) != 0 {
		t.Fatal("mutation before reading user slot")
	}
	user := svc.PrivateUser(AccountRequest{e, "feishu", "app-1"})
	if !user.OK || user.Value.Slot.Version != "v-original" {
		t.Fatalf("%+v", user)
	}
	encoded, _ := json.Marshal(user)
	if strings.Contains(string(encoded), "NEVER-IPC") {
		t.Fatal("private auth material leaked")
	}
	out := svc.ResetPrivateUser(request)
	if out.OK || out.Error.HTTPStatus != 409 {
		t.Fatalf("%+v", out)
	}
	if len(calls) != 3 || !strings.Contains(calls[2], "expected_version=v-original") {
		t.Fatalf("%v", calls)
	}
	// A conflict does not fetch a new version or repeat the DELETE.
}

func TestRemoveExactAccountAndReadBack(t *testing.T) {
	removed := false
	calls := []string{}
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		if r.Method == "DELETE" {
			if r.URL.Query().Get("channel") != "feishu" || r.URL.Query().Get("appId") != "app-1" {
				t.Fatal("wrong deletion target")
			}
			removed = true
			reply(w, `{"success":true,"purged":true}`)
			return
		}
		if removed {
			reply(w, `{"channels":[]}`)
		} else {
			reply(w, `{"channels":[`+record+`]}`)
		}
	})
	out := svc.Remove(AccountRequest{e, "feishu", "app-1"})
	if !out.OK || !out.Value.Removed || out.Value.HistoryDeleted {
		t.Fatalf("%+v", out)
	}
	if len(calls) != 3 || calls[0] != "GET /api/v1/channels" || !strings.HasPrefix(calls[1], "DELETE /api/v1/channels?") || calls[2] != "GET /api/v1/channels" {
		t.Fatal(calls)
	}
}

func TestHTTPFailuresAreSafeAndNotRetried(t *testing.T) {
	for _, status := range []int{404, 409} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(status)
				reply(w, `{"detail":"secret=https://x?token=TOP-SECRET","appSecret":"TOP-SECRET"}`)
			})
			out := svc.ReadProfile(AccountRequest{e, "feishu", "app-1"})
			if out.OK || out.Error.HTTPStatus != status || calls != 1 {
				t.Fatalf("%+v", out)
			}
			b, _ := json.Marshal(out)
			if strings.Contains(string(b), "TOP-SECRET") {
				t.Fatalf("%s", b)
			}
		})
	}
}

func TestQRCodeIsGeneratedLocallyAsAuthorizationMaterial(t *testing.T) {
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
		reply(w, `{"id":"qr-binding","status":"PENDING","loginUrl":"https://accounts.feishu.cn/oauth/v1/app/registration?device_code=AUTH-MATERIAL","qrCodeImage":"https://api.qrserver.com/v1/create-qr-code/?data=AUTH-MATERIAL"}`)
	})
	out := svc.CreateBinding(BindingCreateRequest{ConnectionEpoch: e, OperationID: "qr-operation", Channel: "feishu", Mode: "qr"})
	if !out.OK {
		t.Fatalf("%+v", out)
	}
	if !strings.HasPrefix(out.Value.QRCodeImage, "data:image/png;base64,") {
		t.Fatal("provider URL would leak auth material or stripped query would break QR")
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(out.Value.QRCodeImage, "data:image/png;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(b))
	if err != nil || config.Width != 256 || config.Height != 256 {
		t.Fatalf("invalid local PNG: %+v %v", config, err)
	}
	encoded, _ := json.Marshal(out)
	if strings.Contains(string(encoded), "AUTH-MATERIAL") || strings.Contains(string(encoded), "qrserver.com") || strings.Contains(out.Value.LoginURL, "?") {
		t.Fatal("raw authorization URL in generic IPC fields")
	}
}

func TestBindingRetryRequiresRecoveredReadAndTerminalHidesQR(t *testing.T) {
	calls := 0
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		reply(w, `{"id":"b-1","status":"BOUND","appId":"app-1","loginUrl":"https://accounts.feishu.cn/oauth?token=AUTH-MATERIAL","qrCodeImage":"data:image/png;base64,YQ=="}`)
	})
	r := BindingRequest{ConnectionEpoch: e, OperationID: "recovered", Channel: "feishu", BindingID: "b-1", ResumeConfirmed: true}
	if svc.RetryBinding(r).OK || calls != 0 {
		t.Fatal("recovery retried before inspecting uncertain server state")
	}
	out := svc.ReadBinding(r)
	if !out.OK || out.Value.QRCodeImage != "" || out.Value.LoginURL != "" {
		t.Fatalf("terminal QR auth material retained: %+v", out)
	}
}

func TestMalformedBindingResponseIsUncertainAndDoesNotAllowBlindRetry(t *testing.T) {
	calls := 0
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) { calls++; reply(w, `not json with secret=TOP-SECRET`) })
	req := BindingRequest{ConnectionEpoch: e, OperationID: "op", Channel: "feishu", BindingID: "b-1", ResumeConfirmed: true}
	out := svc.ReadBinding(req)
	if out.OK || out.Error.Code != "outcomeUnknown" || out.Error.RetryDisposition != "verifyOnly" {
		t.Fatalf("malformed polling result may hide completed registration: %+v", out)
	}
	if svc.RetryBinding(req).OK || calls != 1 {
		t.Fatal("blind retry after malformed registration outcome")
	}
}

func TestConcurrentBindingReadsDoNotAdvanceRegistrationTwice(t *testing.T) {
	var calls int32
	started, release := make(chan struct{}), make(chan struct{})
	svc, e := harness(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(started)
			<-release
		}
		reply(w, `{"id":"b-1","status":"SCANNED"}`)
	})
	req := BindingRequest{ConnectionEpoch: e, OperationID: "op", Channel: "feishu", BindingID: "b-1", ResumeConfirmed: true}
	done := make(chan BindingResult, 1)
	go func() { done <- svc.ReadBinding(req) }()
	<-started
	second := svc.ReadBinding(req)
	close(release)
	first := <-done
	if second.OK || second.Error.Code != "bindingOperationBusy" || !first.OK || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("concurrent state advancing requests first=%+v second=%+v calls=%d", first, second, calls)
	}
}

func TestFeishuLauncherQRIsGeneratedLocally(t *testing.T) {
	for _, host := range []string{"open.feishu.cn", "open.larksuite.com"} {
		t.Run(host, func(t *testing.T) {
			svc, epoch := harness(t, func(w http.ResponseWriter, r *http.Request) {
				reply(w, `{"id":"launcher-qr","status":"PENDING","loginUrl":"https://`+host+`/page/launcher?user_code=TEST-AUTH-CODE","qrCodeImage":"https://api.qrserver.com/?data=TEST-AUTH-CODE"}`)
			})
			out := svc.CreateBinding(BindingCreateRequest{ConnectionEpoch: epoch, OperationID: "launcher", Channel: "feishu", Mode: "qr"})
			if !out.OK || out.Value.QRCodeImage == "" || out.Value.ErrorCode != "" {
				t.Fatalf("launcher QR unavailable: %+v", out)
			}
			raw, _ := json.Marshal(out)
			if strings.Contains(string(raw), "TEST-AUTH-CODE") || strings.Contains(string(raw), "qrserver.com") {
				t.Fatal("authorization URL leaked")
			}
		})
	}
}
func TestMultiBotDirectorySurvivesLegacyOtherProvider(t *testing.T) {
	svc, epoch := harness(t, func(w http.ResponseWriter, r *http.Request) {
		reply(w, `{"channels":[{"channel":"dingtalk","appId":null,"enabled":true,"config":{"clientId":"ding-old","clientSecret":"NEVER-IPC"}},`+record+`,{"channel":"feishu","appId":"app-2","enabled":true,"config":{"domain":"feishu"}}]}`)
	})
	out := svc.List(ScopeRequest{epoch})
	if !out.OK || len(out.Value.Channels) != 3 || out.Value.Channels[0].AppID != "ding-old" || out.Value.Channels[1].AppID != "app-1" || out.Value.Channels[2].AppID != "app-2" {
		t.Fatalf("multi bot directory: %+v", out)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "NEVER-IPC") {
		t.Fatal("secret exposed")
	}
}
func TestMissingAccountDirectoryIsNotAnEmptySuccess(t *testing.T) {
	svc, epoch := harness(t, func(w http.ResponseWriter, r *http.Request) { reply(w, `{}`) })
	if svc.List(ScopeRequest{epoch}).OK {
		t.Fatal("missing directory became empty success")
	}
}
