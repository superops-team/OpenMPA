package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/connection"
)

func collaborationTestService(t *testing.T, handler http.HandlerFunc) (*Service, uint64) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-MPA-Studio-Key") != "studio-test-key" || r.Header.Get("Authorization") != "Bearer studio-test-key" {
			t.Error("request did not use the bound Runtime credentials")
		}
		if !strings.HasPrefix(r.URL.Path, collaborationRegistry+"/") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	conn := connection.New()
	epoch, err := conn.Bind(connection.Binding{Endpoint: server.URL, RuntimeID: "r-self", Scope: "test", RuntimeKey: "studio-test-key"}, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Disconnect)
	return New(conn, nil), epoch
}

func TestCollaborationRegistryPagesAndCardProjection(t *testing.T) {
	s, epoch := collaborationTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Query().Get("page") != "2" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		switch r.URL.Path {
		case collaborationRegistry + "/spaces":
			io.WriteString(w, `{"items":[{"Id":"as-team","Name":"Team","IsDefault":true,"ProjectName":"default"}],"page":2,"page_size":50,"total":51}`)
		case collaborationRegistry + "/agents":
			if r.URL.Query().Get("space_id") != "as-team" {
				t.Error("space scope missing")
			}
			io.WriteString(w, `{"items":[{"Id":"a-writer","Name":"Writer","Description":"Writes reports","Status":"running","Alias":"Report Writer","AliasRevision":4,"AliasAvailable":true,"RuntimeConfig":{"RuntimeId":"r-writer","Secret":"hidden"},"AgentCard":{"name":"Card Writer","description":"Card details","version":"1","protocolVersion":"0.3","url":"https://secret.invalid","security":[{"bearer":["hidden"]}],"skills":[{"inputSchema":"hidden"}]}}],"page":2,"page_size":50,"total":51}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	spaces, err := s.CollaborationSpaces(context.Background(), epoch, 2)
	if err != nil || spaces.Total != 51 || spaces.Page != 2 || spaces.PageSize != 50 || spaces.Items[0].ID != "as-team" {
		t.Fatalf("spaces: %+v, %v", spaces, err)
	}
	agents, err := s.CollaborationAgents(context.Background(), epoch, "as-team", 2)
	if err != nil || agents.Total != 51 || agents.Page != 2 || agents.PageSize != 50 {
		t.Fatalf("agents: %+v, %v", agents, err)
	}
	agent := agents.Items[0]
	if agent.AliasRevision != 4 || !agent.AliasAvailable || agent.RuntimeID != "r-writer" ||
		agent.Description != "Writes reports" || agent.Card == nil || agent.Card.Name != "Card Writer" {
		t.Fatalf("member: %+v", agent)
	}
	projected, _ := json.Marshal(agent)
	if strings.Contains(string(projected), "hidden") || strings.Contains(string(projected), "secret.invalid") || strings.Contains(string(projected), "inputSchema") {
		t.Fatal("credential-bearing card fields crossed the service boundary")
	}
}

func TestCollaborationBindingAndAliasCASBodies(t *testing.T) {
	var writes atomic.Int32
	s, epoch := collaborationTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if r.URL.Path != collaborationRegistry+"/binding" {
				t.Error(r.URL.Path)
			}
			io.WriteString(w, `{"space_id":null,"region":"cn-beijing","revision":0}`)
			return
		}
		writes.Add(1)
		if r.Method != http.MethodPut {
			t.Error("writes must use PUT")
		}
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid request JSON")
		}
		if r.URL.Path == collaborationRegistry+"/binding" {
			if len(body) != 2 {
				t.Errorf("binding body fields: %v", body)
			}
			var revision int64
			json.Unmarshal(body["revision"], &revision)
			if revision == 0 && string(body["space_id"]) != `"as-team"` {
				t.Error("bind scope changed")
			}
			if revision == 1 && string(body["space_id"]) != "null" {
				t.Error("unbind must send explicit null")
			}
			fmt.Fprintf(w, `{"space_id":%s,"space_name":"Team","region":"cn-beijing","revision":%d}`, body["space_id"], revision+1)
		} else if r.URL.Path == collaborationRegistry+"/aliases/a-writer" {
			if len(body) != 3 || string(body["space_id"]) != `"as-team"` {
				t.Errorf("alias body fields: %v", body)
			}
			var revision int64
			json.Unmarshal(body["revision"], &revision)
			if revision == 4 && string(body["alias"]) != `"Report Writer"` {
				t.Error("alias not normalized")
			}
			if revision == 5 && string(body["alias"]) != "null" {
				t.Error("clear alias must send null")
			}
			fmt.Fprintf(w, `{"alias":%s,"revision":%d}`, body["alias"], revision+1)
		} else {
			t.Error(r.URL.Path)
		}
	})
	ctx := context.Background()
	binding, err := s.CollaborationBinding(ctx, epoch)
	if err != nil || binding.SpaceID != nil || binding.Revision != 0 {
		t.Fatalf("initial binding: %+v %v", binding, err)
	}
	id := "as-team"
	binding, err = s.SetCollaborationBinding(ctx, epoch, &id, binding.Revision)
	if err != nil || binding.SpaceID == nil || *binding.SpaceID != id || binding.Revision != 1 {
		t.Fatalf("bind: %+v %v", binding, err)
	}
	binding, err = s.SetCollaborationBinding(ctx, epoch, nil, binding.Revision)
	if err != nil || binding.SpaceID != nil || binding.Revision != 2 {
		t.Fatalf("unbind: %+v %v", binding, err)
	}
	alias := " \uff32eport Writer "
	saved, err := s.SetCollaborationAlias(ctx, epoch, id, "a-writer", &alias, 4)
	if err != nil || saved.Revision != 5 || saved.Alias == nil || *saved.Alias != "Report Writer" {
		t.Fatalf("alias: %+v %v", saved, err)
	}
	saved, err = s.SetCollaborationAlias(ctx, epoch, id, "a-writer", nil, 5)
	if err != nil || saved.Alias != nil || saved.Revision != 6 || writes.Load() != 4 {
		t.Fatalf("clear: %+v %v; writes=%d", saved, err, writes.Load())
	}
}

func TestCollaborationMemberCompatibilityAndRequiredAliasState(t *testing.T) {
	for _, tc := range []struct {
		fields string
		valid  bool
		card   bool
	}{
		{``, true, false},
		{`,"Alias":null,"AliasRevision":0,"AliasAvailable":true`, true, false},
		{`,"AliasRevision":0,"AliasAvailable":true`, false, false},
		{`,"Alias":null,"AliasAvailable":true`, false, false},
		{`,"Alias":123,"AliasRevision":0,"AliasAvailable":true`, false, false},
		{`,"Alias":null,"AliasRevision":-1,"AliasAvailable":true`, false, false},
		{`,"AgentCard":"{\"name\":\"Card\",\"url\":\"https://secret.invalid\"}"`, true, true},
		{`,"AgentCard":"not-json"`, true, false},
	} {
		t.Run(tc.fields, func(t *testing.T) {
			s, epoch := collaborationTestService(t, func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"items":[{"Id":"a-writer","Name":"Writer","Status":"running"%s}],"page":1,"page_size":50,"total":1}`, tc.fields)
			})
			value, err := s.CollaborationAgents(context.Background(), epoch, "as-team", 1)
			if !tc.valid {
				if value != nil || err == nil || err.Code != "invalidResponse" {
					t.Fatalf("partial alias state accepted: %+v %v", value, err)
				}
				return
			}
			if err != nil || value == nil || (value.Items[0].Card != nil) != tc.card {
				t.Fatalf("member compatibility: %+v %v", value, err)
			}
		})
	}
}

func TestCollaborationResponseErrorsNeverBecomeEmptySuccess(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		code   string
	}{
		{401, `{"detail":"secret"}`, "authRequired"},
		{403, `{}`, "denied"},
		{404, `{}`, "capabilityUnavailable"},
		{405, `{}`, "capabilityUnavailable"},
		{409, `{}`, "conflict"},
		{412, `{}`, "conflict"},
		{200, `{}`, "invalidResponse"},
		{200, `null`, "invalidResponse"},
		{200, `{"items":null,"total":0,"page":1,"page_size":50}`, "invalidResponse"},
		{200, `{"items":[],"total":0,"page":2,"page_size":50}`, "invalidResponse"},
		{200, `{"items":[],"total":0,"page":1,"page_size":0}`, "invalidResponse"},
		{200, `{"items":[],"total":0,"page":1,"page_size":50} {}`, "invalidResponse"},
	} {
		t.Run(fmt.Sprintf("%d/%s", tc.status, tc.body), func(t *testing.T) {
			s, epoch := collaborationTestService(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			})
			value, err := s.CollaborationSpaces(context.Background(), epoch, 1)
			if value != nil || err == nil || err.Code != tc.code || err.HTTPStatus != tc.status || strings.Contains(err.Message, "secret") {
				t.Fatalf("value=%+v error=%+v", value, err)
			}
		})
	}
}

func TestCollaborationWritesRequireVerifiableReceipts(t *testing.T) {
	for _, body := range []string{`{`, `{}`, `null`, `{"space_id":null,"region":"cn-beijing","revision":8}`, `{"space_id":"as-other","region":"cn-beijing","revision":5}`} {
		t.Run(body, func(t *testing.T) {
			var calls atomic.Int32
			s, epoch := collaborationTestService(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				io.WriteString(w, body)
			})
			id := "as-team"
			value, err := s.SetCollaborationBinding(context.Background(), epoch, &id, 4)
			if value != nil || err == nil || err.Code != "outcomeUnknown" || err.RetryDisposition != "verifyOnly" || calls.Load() != 1 {
				t.Fatalf("value=%+v error=%+v calls=%d", value, err, calls.Load())
			}
			alias := "Writer"
			saved, err := s.SetCollaborationAlias(context.Background(), epoch, id, "a-writer", &alias, 4)
			if saved != nil || err == nil || err.Code != "outcomeUnknown" || calls.Load() != 2 {
				t.Fatalf("alias=%+v error=%+v calls=%d", saved, err, calls.Load())
			}
		})
	}
}

func TestCollaborationWriteConflictAndLostResponseAreNotRetried(t *testing.T) {
	for _, status := range []int{409, 412, 0} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			s, epoch := collaborationTestService(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if status != 0 {
					w.WriteHeader(status)
					io.WriteString(w, `{"detail":"revision_conflict"}`)
					return
				}
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				conn.Close()
			})
			value, err := s.SetCollaborationBinding(context.Background(), epoch, nil, 3)
			if value != nil || err == nil || calls.Load() != 1 {
				t.Fatalf("value=%+v error=%+v calls=%d", value, err, calls.Load())
			}
			if status == 0 && (err.Code != "outcomeUnknown" || err.RetryDisposition != "verifyOnly") {
				t.Fatal(err)
			}
			if status != 0 && (err.HTTPStatus != status || err.RetryDisposition != "reload") {
				t.Fatal(err)
			}
		})
	}
}

func TestCollaborationInputValidationDoesNotSendRequests(t *testing.T) {
	var calls atomic.Int32
	s, epoch := collaborationTestService(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1) })
	ctx := context.Background()
	for _, page := range []int{0, -1, 10001} {
		if _, err := s.CollaborationSpaces(ctx, epoch, page); err == nil {
			t.Error("invalid page accepted")
		}
	}
	for _, id := range []string{"as-", "as-team/../binding", "as-team?x=y", "https://example.com", "as-%2e%2e"} {
		if _, err := s.CollaborationAgents(ctx, epoch, id, 1); err == nil {
			t.Error("invalid scope accepted")
		}
		if _, err := s.SetCollaborationBinding(ctx, epoch, &id, 0); err == nil {
			t.Error("invalid binding accepted")
		}
	}
	for _, alias := range []string{" ", "a\nb", "a\u200bb", `a"b`, "@other", strings.Repeat("a", 81)} {
		if _, err := s.SetCollaborationAlias(ctx, epoch, "as-team", "a-writer", &alias, 0); err == nil {
			t.Errorf("invalid alias %q accepted", alias)
		}
	}
	if _, err := s.SetCollaborationAlias(ctx, epoch, "as-team", "a-writer/../binding", nil, 0); err == nil {
		t.Error("invalid agent accepted")
	}
	if _, err := s.SetCollaborationBinding(ctx, epoch, nil, -1); err == nil {
		t.Error("negative revision accepted")
	}
	if calls.Load() != 0 {
		t.Fatalf("sent %d invalid requests", calls.Load())
	}
}

func TestCollaborationHTTP500WritesAreAmbiguousWithoutReplay(t *testing.T) {
	for _, operation := range []string{"binding", "alias"} {
		t.Run(operation, func(t *testing.T) {
			var calls atomic.Int32
			s, epoch := collaborationTestService(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
				io.WriteString(w, `{"detail":"storage error after commit"}`)
			})
			if operation == "binding" {
				value, err := s.SetCollaborationBinding(context.Background(), epoch, nil, 3)
				if value != nil || err == nil || err.Code != "outcomeUnknown" || err.RetryDisposition != "verifyOnly" || err.HTTPStatus != 500 {
					t.Fatalf("binding: %+v %+v", value, err)
				}
			} else {
				value, err := s.SetCollaborationAlias(context.Background(), epoch, "as-team", "a-writer", nil, 4)
				if value != nil || err == nil || err.Code != "outcomeUnknown" || err.RetryDisposition != "verifyOnly" || err.HTTPStatus != 500 {
					t.Fatalf("alias: %+v %+v", value, err)
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("ambiguous write replayed %d times", calls.Load())
			}
		})
	}
}

func TestCollaborationEpochChangeRejectsResult(t *testing.T) {
	var s *Service
	s, epoch := collaborationTestService(t, func(w http.ResponseWriter, r *http.Request) {
		s.conn.Disconnect()
		io.WriteString(w, `{"space_id":null,"region":"cn-beijing","revision":0}`)
	})
	value, err := s.CollaborationBinding(context.Background(), epoch)
	if value != nil || err == nil || err.Code != "staleConnection" {
		t.Fatalf("stale result: %+v %v", value, err)
	}
}

func TestCollaborationMentionMatchesRuntimeGrammar(t *testing.T) {
	for _, tc := range []struct {
		name, alias, task, want string
	}{
		{"Writer", "", "Summarize", ""},
		{"Display Writer", "Writer", "Summarize", "@Writer Summarize"},
		{"Display name", "Report Writer", "Summarize", `@"Report Writer" Summarize`},
		{"Writer", "\u5199\u4f5c", "Summarize", "@\u5199\u4f5c Summarize"},
		{"Writer:one", "Writer one", "Summarize", `@"Writer one" Summarize`},
		{"Writer", "Writer", "first\nsecond", "@Writer first\nsecond"},
		{`Bad"name`, "", "task", ""},
		{"Bad\nname", "", "task", ""},
		{"Bad\u200bname", "", "task", ""},
		{strings.Repeat("a", 81), "", "task", ""},
		{"Writer", "", "@other task", ""},
		{"Writer", "", " ", ""},
	} {
		t.Run(tc.name+"/"+tc.alias+"/"+tc.task, func(t *testing.T) {
			agent := CollaborationAgent{ID: "a-writer", Name: tc.name, Status: "running", AliasAvailable: true}
			if tc.alias != "" {
				agent.Alias = &tc.alias
			}
			got, err := CollaborationMention(agent, tc.task)
			if got != tc.want || (err != nil) != (tc.want == "") {
				t.Fatalf("mention=%q error=%v want=%q", got, err, tc.want)
			}
		})
	}
	agent := CollaborationAgent{ID: "a-writer", Name: "Display", Status: "running", Card: &CollaborationCard{Name: "Card name"}}
	if got, err := CollaborationMention(agent, "task"); err == nil || err.Code != "collaborationAliasRequired" || got != "" {
		t.Fatalf("unverified Card name became a target: %q %v", got, err)
	}
	alias := "Selected member"
	agent.Alias, agent.AliasAvailable = &alias, true
	if got, err := CollaborationMention(agent, "task"); err != nil || got != `@"Selected member" task` {
		t.Fatalf("saved alias not used: %q %v", got, err)
	}
	agent.Status = "stopped"
	if _, err := CollaborationMention(agent, "task"); err == nil {
		t.Fatal("stopped member accepted")
	}
}
