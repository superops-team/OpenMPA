package schedule

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func connectedHTTP(t *testing.T, server *httptest.Server, mode string) (*Service, uint64) {
	t.Helper()
	c := connection.New()
	epoch, e := c.Bind(connection.Binding{Endpoint: server.URL + "/base", RuntimeID: "test-runtime", Scope: "local-space-does-not-grant-identity", RuntimeKey: "runtime-key-test"}, true)
	if e != nil {
		t.Fatal(e)
	}
	if mode != "" {
		req := dto.IdentityRequest{ConnectionEpoch: epoch, AuthMode: mode}
		if mode == "explicitTest" {
			req.TestSubject = "explicit-scheduled-user"
			req.VerifiedTestDeployment = true
		} else if mode == "legacyJwt" {
			req.LegacyJWT = "legacy-task-jwt-test"
		}
		if e := c.SetBusinessAuth(req); e != nil {
			t.Fatal(e)
		}
		epoch = c.State().ConnectionEpoch
	}
	return New(c), epoch
}

func TestHTTPFixedFamilyIdentityAndBasePath(t *testing.T) {
	for _, mode := range []string{"explicitTest", "legacyJwt", ""} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/base/api/v1/esa-cron-tasks" || r.Method != "GET" {
					t.Error("wrong scheduled path or method")
				}
				if r.Header.Get("Authorization") != "Bearer runtime-key-test" {
					t.Error("Runtime ingress key missing")
				}
				if r.Header.Get("X-MPA-Channel-Key") != "" || r.Header.Get("X-Space-Id") != "" || r.Header.Get("X-Mpa-Id") != "" {
					t.Error("unselected identity or channel credential escaped")
				}
				switch mode {
				case "explicitTest":
					if r.Header.Get("X-User-Id") != "explicit-scheduled-user" || r.Header.Get("X-Jwt-Token") != "" {
						t.Error("explicit test identity used wrong slot")
					}
				case "legacyJwt":
					if r.Header.Get("X-Jwt-Token") != "legacy-task-jwt-test" || r.Header.Get("X-User-Id") != "" {
						t.Error("legacy task identity used wrong slot")
					}
				default:
					if r.Header.Get("X-User-Id") != "" || r.Header.Get("X-Jwt-Token") != "" {
						t.Error("identity was inferred from local scope or cloud account")
					}
					w.WriteHeader(422)
					io.WriteString(w, `{"detail":"x-user-id header is required"}`)
					return
				}
				io.WriteString(w, `{"items":[],"total":0,"limit":20,"offset":0,"hasMore":false,"overview":{"executionCount":0}}`)
			}))
			defer server.Close()
			s, epoch := connectedHTTP(t, server, mode)
			got := s.List(ListRequest{ConnectionEpoch: epoch})
			if mode == "" {
				if got.OK || got.Error == nil || got.Error.HTTPStatus != 422 {
					t.Fatalf("missing task identity accepted: %+v", got)
				}
			} else if !got.OK || !got.Value.CountAvailable {
				t.Fatalf("authorized list failed: %+v", got)
			}
		})
	}
}

func TestHTTPServerErrorCodesAndCurrentState(t *testing.T) {
	cases := []struct {
		code   string
		status int
	}{
		{"ScheduledTaskNotFound", 404}, {"ScheduledTaskVersionConflict", 409}, {"IdempotencyConflict", 409}, {"ScheduledTaskQuotaExceeded", 429}, {"SchedulerUnavailable", 503}, {"InvalidSchedule", 400}, {"InvalidDeliveryTarget", 400},
	}
	for _, test := range cases {
		t.Run(test.code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				json.NewEncoder(w).Encode(struct {
					Detail string `json:"detail"`
				}{test.code})
			}))
			defer server.Close()
			s, epoch := connectedHTTP(t, server, "explicitTest")
			got := s.Update(UpdateRequest{ConnectionEpoch: epoch, TaskID: "task-1", ExpectedVersion: 3, Enabled: boolPtr(false)})
			if got.OK || got.Value != nil || got.Error == nil || got.Error.HTTPStatus != test.status || got.Error.Code != test.code {
				t.Fatalf("server code was discarded: %+v", got.Error)
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		io.WriteString(w, `{"code":"ScheduledTaskVersionConflict","message":"Bearer unsafe secret","requestId":"request-1","details":{"currentState":{"status":"Running","generation":4}}}`)
	}))
	defer server.Close()
	s, epoch := connectedHTTP(t, server, "explicitTest")
	got := s.Update(UpdateRequest{ConnectionEpoch: epoch, TaskID: "task-1", ExpectedVersion: 3, Enabled: boolPtr(false)})
	if got.Error == nil || got.Error.CurrentState == nil || got.Error.CurrentState.Status != "Running" || got.Error.RequestID != "request-1" || got.Error.Message == "Bearer unsafe secret" {
		t.Fatalf("safe structured error contract failed: %+v", got.Error)
	}
}

func TestHTTPUnknownCreateExplicitSameKeyRecovery(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/base/api/v1/esa-cron-tasks" {
			t.Error("wrong create route")
		}
		var input createPayload
		if e := json.NewDecoder(r.Body).Decode(&input); e != nil {
			t.Error(e)
		}
		if input.ClientToken != "durable-token" || input.Schedule.Type != "Once" || input.Schedule.RunAt != "2026-10-25T19:00:00+08:00" {
			t.Error("recovery request changed")
		}
		if calls.Add(1) == 1 {
			conn, _, e := w.(http.Hijacker).Hijack()
			if e != nil {
				t.Error(e)
				return
			}
			conn.Close()
			return
		}
		io.WriteString(w, taskWire)
	}))
	defer server.Close()
	s, epoch := connectedHTTP(t, server, "explicitTest")
	request := CreateRequest{ConnectionEpoch: epoch, ClientToken: "durable-token", Name: "Check", Prompt: "Check status", Schedule: Config{Type: "Once", RunAt: "2026-10-25T19:00:00+08:00"}}
	unknown := s.Create(request)
	if unknown.OK || unknown.Error == nil || unknown.Error.Code != "outcomeUnknown" || unknown.ClientToken != "durable-token" || calls.Load() != 1 {
		t.Fatalf("transport uncertainty retried or token lost: %+v", unknown)
	}
	recovered := s.Create(request)
	if !recovered.OK || recovered.ClientToken != "durable-token" || recovered.Value.Task.ID != "task-1" || calls.Load() != 2 {
		t.Fatalf("same-key recovery failed: %+v", recovered)
	}
}
