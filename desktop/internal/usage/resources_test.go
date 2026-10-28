package usage

import (
	"context"
	"encoding/json"
	"io"
	"math"
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

func resourceTestService(t *testing.T, handler http.HandlerFunc) (*Service, uint64) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	conn := connection.New()
	epoch, err := conn.Bind(connection.Binding{Endpoint: server.URL, RuntimeID: "r-runtime", RuntimeKey: "test-runtime-key"}, true)
	if err != nil {
		t.Fatal(err)
	}
	return New(conn, nil), epoch
}

func TestResourcesAgentsMDCAS(t *testing.T) {
	calls := 0
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v1/studio/agents-md" || r.Header.Get("X-MPA-Studio-Key") != "test-runtime-key" {
			t.Error("wrong Studio route or credential")
		}
		if r.Method == http.MethodGet {
			io.WriteString(w, `{"content":"old","revision":7}`)
			return
		}
		var body map[string]any
		if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&body) != nil ||
			len(body) != 2 || body["content"] != "new" || body["revision"] != float64(7) {
			t.Errorf("wrong CAS request: %+v", body)
		}
		io.WriteString(w, `{"content":"new","revision":8}`)
	})
	current, err := service.ReadAgentsMD(context.Background(), epoch)
	if err != nil || current.Content != "old" || current.Revision != 7 {
		t.Fatalf("read: %+v %v", current, err)
	}
	current.Content = "new"
	saved, err := service.SaveAgentsMD(context.Background(), epoch, current)
	if err != nil || saved.Content != "new" || saved.Revision != 8 || calls != 2 {
		t.Fatalf("save: %+v %v, calls=%d", saved, err, calls)
	}
}

func TestResourcesMCPProjection(t *testing.T) {
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sessions/session-1/mcp-servers" || r.Method != http.MethodGet {
			t.Error("wrong MCP route")
		}
		io.WriteString(w, `{"sessionId":"session-1","mcpServers":[
			{"id":"one","name":"Search","alias":"lookup","description":"Find documents","gatewayUrl":"https://private.invalid","headers":{"Authorization":"private"},"tools":[{"name":"query","description":"Search documents","inputSchema":{"secret":"private"}}]},
			{"id":"two","name":"Unknown"},
			{"id":"three","name":"Empty","tools":[]}
		]}`)
	})
	out, err := service.ListMCPServers(context.Background(), epoch, "session-1")
	if err != nil || out.SessionID != "session-1" || len(out.Servers) != 3 {
		t.Fatalf("MCP: %+v %v", out, err)
	}
	if !out.Servers[0].ToolsKnown || out.Servers[1].ToolsKnown || !out.Servers[2].ToolsKnown ||
		out.Servers[0].Tools[0].Description != "Search documents" || out.Servers[0].Alias != "lookup" {
		t.Fatalf("tool metadata: %+v", out.Servers)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "inputSchema") {
		t.Fatal("private MCP fields projected")
	}
}

const resourceTestCard = `{"name":"main","capabilities":{"extensions":[
	{"uri":"urn:veadk:mpa:resource-topology:v1","params":{
		"nodes":[
			{"id":"agent:main","kind":"agent","name":"main"},
			{"id":"agent:other","kind":"agent","name":"other"},
			{"id":"skill-space:mounted","kind":"skill-space","name":"Skill Space","status":"configured","resourceId":"mounted"},
			{"id":"skill-space:unmounted","kind":"skill-space","name":"Other","status":"configured","resourceId":"unmounted"}],
		"edges":[
			{"source":"agent:main","target":"skill-space:mounted","relation":"mounts"},
			{"source":"agent:other","target":"skill-space:unmounted","relation":"calls"}]
	}}]}}`

type resourceRoundTripFunc func(*http.Request) (*http.Response, error)

func (f resourceRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func resourceTestCloud(t *testing.T, action func(*http.Request) string) *cloud.Client {
	t.Helper()
	client := cloud.NewWithTransport(resourceRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		raw := `{"Result":{"AccountId":"test-account","IdentityType":"User","UserId":"test-user"}}`
		if r.URL.Query().Get("Action") != "GetCallerIdentity" {
			raw = action(r)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(raw))}, nil
	}))
	_, err := client.SignIn(context.Background(), dto.SignInRequest{Region: "cn-beijing", AccessKey: "test-access", SecretKey: "test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestResourcesMountedSkillsAndLocalPaging(t *testing.T) {
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/agent-card.json" {
			t.Error("wrong Card route")
		}
		io.WriteString(w, resourceTestCard)
	})
	calls := 0
	service.cloud = resourceTestCloud(t, func(r *http.Request) string {
		calls++
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("invalid cloud body")
		}
		if r.URL.Query().Get("Action") != "ListSkillsBySpaceId" || r.URL.Query().Get("Version") != cloud.APIVersion ||
			r.URL.Host != "agentkit.cn-beijing.volcengineapi.com" || len(body) != 2 ||
			string(body["SkillSpaceId"]) != `"mounted"` || string(body["InnerTags"]) != `{"source":"sandbox"}` {
			t.Errorf("wrong fixed cloud contract: %s %+v", r.URL, body)
		}
		if !strings.Contains(r.Header.Get("Authorization"), "/cn-beijing/agentkit/request") {
			t.Error("unsigned cloud request")
		}
		return `{"Result":{"Items":[{"Name":"first","Description":"one","BucketName":"private"},{"Name":"second","Description":"two"},{"Name":"third","Description":"three"}]}}`
	})
	mounts, err := service.DiscoverSkillSpaces(context.Background(), epoch)
	if err != nil || len(mounts.Spaces) != 1 || mounts.Spaces[0].ID != "mounted" {
		t.Fatalf("mounts: %+v %v", mounts, err)
	}
	page, err := service.ListSkills(context.Background(), epoch, "cn-beijing", "mounted", 2, 2)
	if err != nil || page.TotalCount != 3 || page.PageNumber != 2 || page.PageSize != 2 ||
		len(page.Items) != 1 || page.Items[0].Name != "third" || page.HasMore || page.ServerPaginated {
		t.Fatalf("page: %+v %v", page, err)
	}
	if page.ConnectionEpoch != epoch || page.PlatformEpoch != service.cloud.Epoch() {
		t.Fatal("missing identity stamps")
	}
	if _, err = service.ListSkills(context.Background(), epoch, "cn-beijing", "unmounted", 1, 2); err == nil || calls != 1 {
		t.Fatal("unmounted Skill Space reached cloud")
	}
}

func TestResourcesAgentsMDInputBoundaries(t *testing.T) {
	calls := 0
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var document AgentsMDDocument
		if json.NewDecoder(r.Body).Decode(&document) != nil {
			t.Error("bad document")
		}
		document.Revision++
		json.NewEncoder(w).Encode(document)
	})
	for _, content := range []string{"", strings.Repeat("\U0001f4dd", 40000)} {
		if _, err := service.SaveAgentsMD(context.Background(), epoch, AgentsMDDocument{Content: content}); err != nil {
			t.Fatal(err)
		}
	}
	for _, document := range []AgentsMDDocument{
		{Content: strings.Repeat("a", 40001)}, {Content: string([]byte{0xff})},
		{Revision: -1}, {Revision: math.MaxInt64},
	} {
		if _, err := service.SaveAgentsMD(context.Background(), epoch, document); err == nil || err.Code != "invalidInput" {
			t.Fatalf("invalid input accepted: %v", err)
		}
	}
	if calls != 2 {
		t.Fatal("invalid document reached Runtime")
	}
}

func TestResourcesAgentsMDFailureContracts(t *testing.T) {
	for _, tc := range []struct {
		name, body, code, retry string
		status                  int
	}{
		{"conflict", `{}`, "conflict", "reload", 409},
		{"precondition", `{}`, "conflict", "reload", 412},
		{"unauthorized", `{"detail":"private"}`, "authRequired", "never", 401},
		{"denied", `{}`, "denied", "never", 403},
		{"unsupported", `{}`, "capabilityUnavailable", "readOnly", 404},
		{"method", `{}`, "capabilityUnavailable", "readOnly", 405},
		{"server-error", `{}`, "outcomeUnknown", "verifyOnly", 500},
		{"missing-content", `{"revision":2}`, "outcomeUnknown", "verifyOnly", 200},
		{"missing-revision", `{"content":"draft"}`, "outcomeUnknown", "verifyOnly", 200},
		{"null-content", `{"content":null,"revision":2}`, "outcomeUnknown", "verifyOnly", 200},
		{"older-revision", `{"content":"draft","revision":0}`, "outcomeUnknown", "verifyOnly", 200},
		{"wrong-content", `{"content":"different","revision":2}`, "outcomeUnknown", "verifyOnly", 200},
		{"trailing-json", `{"content":"draft","revision":2}{}`, "outcomeUnknown", "verifyOnly", 200},
		{"null", `null`, "outcomeUnknown", "verifyOnly", 200},
		{"too-large", `{"content":"` + strings.Repeat("x", 4<<20) + `","revision":2}`, "outcomeUnknown", "verifyOnly", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			})
			draft := AgentsMDDocument{Content: "draft", Revision: 1}
			out, err := service.SaveAgentsMD(context.Background(), epoch, draft)
			if err == nil || err.Code != tc.code || err.RetryDisposition != tc.retry || out != (AgentsMDDocument{}) || calls != 1 {
				t.Fatalf("write error: %+v %+v, calls=%d", out, err, calls)
			}
			if draft.Content != "draft" || draft.Revision != 1 || strings.Contains(err.Message, "private") {
				t.Fatal("draft changed or provider detail exposed")
			}
		})
	}
}

func TestResourcesAgentsMDLostResponse(t *testing.T) {
	var calls atomic.Int32
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		conn.Close()
	})
	_, err := service.SaveAgentsMD(context.Background(), epoch, AgentsMDDocument{Content: "draft", Revision: 1})
	if err == nil || err.Code != "outcomeUnknown" || err.RetryDisposition != "verifyOnly" || calls.Load() != 1 {
		t.Fatalf("lost response retried or hidden: %+v, calls=%d", err, calls.Load())
	}
}

func TestResourcesRejectMalformedReads(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		read       func(*Service, uint64) *dto.SafeError
	}{
		{"document-empty", `{}`, func(s *Service, e uint64) *dto.SafeError {
			_, err := s.ReadAgentsMD(context.Background(), e)
			return err
		}},
		{"document-negative", `{"content":"","revision":-1}`, func(s *Service, e uint64) *dto.SafeError {
			_, err := s.ReadAgentsMD(context.Background(), e)
			return err
		}},
		{"mcp-wrong-session", `{"sessionId":"other","mcpServers":[]}`, func(s *Service, e uint64) *dto.SafeError {
			_, err := s.ListMCPServers(context.Background(), e, "s")
			return err
		}},
		{"mcp-missing-servers", `{"sessionId":"s"}`, func(s *Service, e uint64) *dto.SafeError {
			_, err := s.ListMCPServers(context.Background(), e, "s")
			return err
		}},
		{"mcp-null-servers", `{"sessionId":"s","mcpServers":null}`, func(s *Service, e uint64) *dto.SafeError {
			_, err := s.ListMCPServers(context.Background(), e, "s")
			return err
		}},
		{"mcp-missing-id", `{"sessionId":"s","mcpServers":[{"name":"no id"}]}`, func(s *Service, e uint64) *dto.SafeError {
			_, err := s.ListMCPServers(context.Background(), e, "s")
			return err
		}},
		{"mcp-null-tool", `{"sessionId":"s","mcpServers":[{"id":"m","tools":[null]}]}`, func(s *Service, e uint64) *dto.SafeError {
			_, err := s.ListMCPServers(context.Background(), e, "s")
			return err
		}},
		{"mcp-duplicate", `{"sessionId":"s","mcpServers":[{"id":"m"},{"id":"m"}]}`, func(s *Service, e uint64) *dto.SafeError {
			_, err := s.ListMCPServers(context.Background(), e, "s")
			return err
		}},
		{"card-null", `null`, func(s *Service, e uint64) *dto.SafeError {
			_, err := s.DiscoverSkillSpaces(context.Background(), e)
			return err
		}},
		{"card-malformed", `{"name":"main","capabilities":{"extensions":[{"uri":"urn:veadk:mpa:resource-topology:v1","params":{}}]}}`, func(s *Service, e uint64) *dto.SafeError {
			_, err := s.DiscoverSkillSpaces(context.Background(), e)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, tc.body)
			})
			if err := tc.read(service, epoch); err == nil || err.Code != "invalidResponse" {
				t.Fatalf("malformed read accepted: %+v", err)
			}
		})
	}
}

func TestResourcesDiscoveryEmptyAndUnsupported(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		code       string
	}{
		{"missing-extension", `{"name":"main","capabilities":{}}`, 200, "capabilityUnavailable"},
		{"missing-endpoint", `{}`, 404, "capabilityUnavailable"},
		{"unauthorized", `{}`, 401, "authRequired"},
		{"denied", `{}`, 403, "denied"},
		{"unmounted", strings.ReplaceAll(resourceTestCard, `"relation":"mounts"`, `"relation":"calls"`), 200, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			})
			out, err := service.DiscoverSkillSpaces(context.Background(), epoch)
			if tc.code == "" {
				if err != nil || out.Spaces == nil || len(out.Spaces) != 0 {
					t.Fatalf("valid empty mounts: %+v %v", out, err)
				}
			} else if err == nil || err.Code != tc.code || out.Spaces != nil {
				t.Fatalf("unsupported discovery became empty success: %+v %v", out, err)
			}
		})
	}
}

func TestResourcesRejectUnsafeIdentifiersAndPaging(t *testing.T) {
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid input reached Runtime")
	})
	for _, id := range []string{"", "..", "../other", "a/b", `a\b`, "%2e%2e", "a?x", "a#x", "a\r\nb", strings.Repeat("x", 513)} {
		if _, err := service.ListMCPServers(context.Background(), epoch, id); err == nil || err.Code != "invalidInput" {
			t.Errorf("unsafe session accepted: %q", id)
		}
		if _, err := service.ReadMainProfile(context.Background(), epoch, id); err == nil || err.Code != "invalidInput" {
			t.Errorf("unsafe instance accepted: %q", id)
		}
	}
	for _, page := range [][2]int{{0, 1}, {1, 0}, {-1, 1}, {10001, 1}, {1, 101}, {math.MaxInt, 100}} {
		if _, err := service.ListSkills(context.Background(), epoch, "cn-beijing", "mounted", page[0], page[1]); err == nil || err.Code != "invalidInput" {
			t.Errorf("invalid page accepted: %+v", page)
		}
	}
}

func TestResourcesSkillCloudFailures(t *testing.T) {
	for _, body := range []string{
		`{"Result":{}}`, `{"Result":{"Items":null}}`,
		`{"Result":{"Items":[{"SkillName":"wrong-api","SkillDescription":"wrong"}]}}`,
		`{"Result":{"Items":[{"Name":"x","Description":123}]}}`,
		`{"Result":{"Items":[{"Name":"x","Description":""},{"Name":"x","Description":""}]}}`,
		`{"ResponseMetadata":{"Error":{"Code":"LackPolicy","Message":"private"}}}`,
	} {
		t.Run(body, func(t *testing.T) {
			service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, resourceTestCard) })
			service.cloud = resourceTestCloud(t, func(r *http.Request) string { return body })
			page, err := service.ListSkills(context.Background(), epoch, "cn-beijing", "mounted", 1, 100)
			if err == nil || page.Items != nil || strings.Contains(err.Message, "private") {
				t.Fatalf("bad cloud response accepted: %+v %v", page, err)
			}
		})
	}
}

func TestResourcesAgentsMDNoOpKeepsRevision(t *testing.T) {
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":"unchanged","revision":7}`)
	})
	document, err := service.SaveAgentsMD(context.Background(), epoch, AgentsMDDocument{Content: "unchanged", Revision: 7})
	if err != nil || document.Content != "unchanged" || document.Revision != 7 {
		t.Fatalf("Runtime no-op receipt rejected: %+v %v", document, err)
	}
}

func TestResourcesSkillDescriptionsAreOptional(t *testing.T) {
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, resourceTestCard) })
	service.cloud = resourceTestCloud(t, func(r *http.Request) string {
		return `{"Result":{"Items":[{"Name":"missing"},{"Name":"null","Description":null},{"Name":"described","Description":"Useful"}]}}`
	})
	page, err := service.ListSkills(context.Background(), epoch, "cn-beijing", "mounted", 1, 100)
	if err != nil || len(page.Items) != 3 || page.Items[0].Description != "" || page.Items[1].Description != "" || page.Items[2].Description != "Useful" {
		t.Fatalf("optional descriptions rejected: %+v %v", page, err)
	}
}

func TestResourcesSkillEpochFencing(t *testing.T) {
	for _, boundary := range []string{"discovery-platform", "cloud-platform", "cloud-runtime"} {
		t.Run(boundary, func(t *testing.T) {
			var service *Service
			service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
				if boundary == "discovery-platform" {
					service.cloud.SignOut()
				}
				io.WriteString(w, resourceTestCard)
			})
			calls := 0
			service.cloud = resourceTestCloud(t, func(r *http.Request) string {
				calls++
				if boundary == "cloud-platform" {
					service.cloud.SignOut()
				}
				if boundary == "cloud-runtime" {
					service.conn.Disconnect()
				}
				return `{"Result":{"Items":[]}}`
			})
			page, err := service.ListSkills(context.Background(), epoch, "cn-beijing", "mounted", 1, 10)
			if err == nil || err.Code != "staleConnection" || page.Items != nil || (boundary == "discovery-platform" && calls != 0) {
				t.Fatalf("stale cloud result accepted: %+v %v, calls=%d", page, err, calls)
			}
		})
	}
}

func TestResourcesReadCancellationAndStaleEpoch(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(map[bool]string{true: "identity", false: "cancel"}[stale], func(t *testing.T) {
			entered := make(chan struct{})
			service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
				close(entered)
				<-r.Context().Done()
			})
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			done := make(chan *dto.SafeError, 1)
			go func() { _, err := service.ReadAgentsMD(ctx, epoch); done <- err }()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("request did not start")
			}
			if stale {
				service.conn.Disconnect()
			} else {
				cancel()
			}
			err := <-done
			if err == nil || (stale && err.Code != "staleConnection") || (!stale && err.Code != "connectionFailure") {
				t.Fatalf("cancellation not surfaced: %v", err)
			}
		})
	}
}
