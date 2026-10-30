package usage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

const mainProfileTestRecord = `{"mpaInstanceId":"mpa-instance","revision":7,"etag":"\"7\"","digest":"sha256:old",
	"profile":{"agentsMd":"old","model":{"id":"current-model"},"skills":[{"name":"review","version":"1.2.3","content":"pinned instructions"}]},
	"sourceProfileId":"source-profile","sourceProfileDigest":"sha256:source","updatedBy":"owner","createdAt":"2026-10-27T00:00:00Z"}`

func TestMainProfileInstructionEditPreservesSnapshot(t *testing.T) {
	writes := 0
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents/mpa-instance/main-profile" || r.Header.Get("X-MPA-Studio-Key") != "" {
			t.Error("wrong Main profile route or credential family")
		}
		if r.Method == http.MethodGet {
			w.Header().Set("ETag", `"7"`)
			io.WriteString(w, mainProfileTestRecord)
			return
		}
		writes++
		if r.Method != http.MethodPut || r.Header.Get("If-Match") != `"7"` ||
			r.Header.Get("If-None-Match") != "" || r.Header.Get("Idempotency-Key") == "" {
			t.Error("missing update-only CAS or idempotency header")
		}
		var body struct {
			Profile             MainProfile `json:"profile"`
			SourceProfileID     string      `json:"sourceProfileId"`
			SourceProfileDigest string      `json:"sourceProfileDigest"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Profile.AgentsMD != "new" ||
			body.Profile.Model == nil || body.Profile.Model.ID != "current-model" ||
			len(body.Profile.Skills) != 1 || body.Profile.Skills[0].Content != "pinned instructions" ||
			body.SourceProfileID != "source-profile" || body.SourceProfileDigest != "sha256:source" {
			t.Errorf("profile settings lost: %+v", body)
		}
		w.Header().Set("ETag", `"8"`)
		saved := strings.ReplaceAll(mainProfileTestRecord, `"revision":7`, `"revision":8`)
		saved = strings.ReplaceAll(saved, `\"7\"`, `\"8\"`)
		saved = strings.ReplaceAll(saved, `"agentsMd":"old"`, `"agentsMd":"new"`)
		io.WriteString(w, saved)
	})
	current, err := service.ReadMainProfile(context.Background(), epoch, "mpa-instance")
	if err != nil || current.Status != MainProfileConfigured || current.Record == nil {
		t.Fatalf("read: %+v %v", current, err)
	}
	// The UI's editable projection cannot alter the preserved model or skills.
	current.Record.Profile.Model.ID = "unintended-model"
	current.Record.Profile.Skills[0].Content = "unintended-instructions"
	out, err := service.SaveMainProfileInstructions(context.Background(), epoch, current, "new")
	if err != nil || out.Record == nil || out.Record.Revision != 8 || writes != 1 {
		t.Fatalf("save: %+v %v", out, err)
	}
}

func TestMainProfile404NeverInitializes(t *testing.T) {
	for _, body := range []string{`{"detail":"Not Found"}`, `{"detail":{"code":"main_profile_not_found"}}`} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet {
					t.Error("unexpected initialization")
				}
				w.WriteHeader(404)
				io.WriteString(w, body)
			})
			current, err := service.ReadMainProfile(context.Background(), epoch, "mpa-instance")
			if err == nil || current.Status != MainProfileUnsupportedOrUnconfigured || current.Record != nil {
				t.Fatalf("404 status: %+v %v", current, err)
			}
			_, err = service.SaveMainProfileInstructions(context.Background(), epoch, current, "new")
			if err == nil || calls != 1 {
				t.Fatal("404 allowed a write")
			}
		})
	}
}

func TestMainProfileReadErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, etag, code string
		status                 int
		state                  MainProfileStatus
	}{
		{"method", `{}`, "", "requestFailed", 405, MainProfileUnsupported},
		{"auth", `{}`, "", "authRequired", 401, MainProfileUnavailable},
		{"denied", `{}`, "", "denied", 403, MainProfileUnavailable},
		{"unavailable", `{"detail":{"code":"main_profile_runtime_unconfigured"}}`, "", "unavailable", 503, MainProfileUnavailable},
		{"empty", `{}`, "", "invalidResponse", 200, MainProfileUnavailable},
		{"null", `null`, "", "invalidResponse", 200, MainProfileUnavailable},
		{"bad-etag", mainProfileTestRecord, `"8"`, "invalidResponse", 200, MainProfileUnavailable},
		{"wrong-instance", strings.Replace(mainProfileTestRecord, "mpa-instance", "r-runtime", 1), "", "invalidResponse", 200, MainProfileUnavailable},
		{"missing-version", strings.Replace(mainProfileTestRecord, `"revision":7,`, "", 1), "", "invalidResponse", 200, MainProfileUnavailable},
		{"missing-profile", `{"mpaInstanceId":"mpa-instance","revision":7,"etag":"\"7\"","digest":"x"}`, "", "invalidResponse", 200, MainProfileUnavailable},
		{"unknown-profile-setting", strings.Replace(mainProfileTestRecord, `"agentsMd":"old"`, `"agentsMd":"old","futureSetting":true`, 1), "", "invalidResponse", 200, MainProfileUnavailable},
		{"unknown-model-setting", strings.Replace(mainProfileTestRecord, `"id":"current-model"`, `"id":"current-model","temperature":0.5`, 1), "", "invalidResponse", 200, MainProfileUnavailable},
		{"floating-skill", strings.Replace(mainProfileTestRecord, `"version":"1.2.3"`, `"version":"latest"`, 1), "", "invalidResponse", 200, MainProfileUnavailable},
		{"null-model", strings.Replace(mainProfileTestRecord, `"model":{"id":"current-model"}`, `"model":null`, 1), `"7"`, "", 200, MainProfileConfigured},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
				if tc.etag != "" {
					w.Header().Set("ETag", tc.etag)
				}
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			})
			out, err := service.ReadMainProfile(context.Background(), epoch, "mpa-instance")
			if out.Status != tc.state {
				t.Fatalf("wrong status: %+v %v", out, err)
			}
			if tc.code == "" {
				if err != nil || out.Record == nil || out.Record.Profile.Model != nil {
					t.Fatalf("valid null model rejected: %+v %v", out, err)
				}
			} else if err == nil || err.Code != tc.code || out.Record != nil {
				t.Fatalf("incorrect read error: %+v %v", out, err)
			}
		})
	}
}

func TestMainProfileWriteFailureRequiresReadback(t *testing.T) {
	for _, tc := range []struct {
		name, body, code, retry string
		status                  int
	}{
		{"conflict", `{}`, "conflict", "reload", 412},
		{"idempotency", `{}`, "conflict", "reload", 409},
		{"auth", `{}`, "authRequired", "never", 401},
		{"server-error", `{}`, "outcomeUnknown", "verifyOnly", 500},
		{"malformed", `{`, "outcomeUnknown", "verifyOnly", 200},
		{"empty", `{}`, "outcomeUnknown", "verifyOnly", 200},
		{"trailing", mainProfileTestRecord + `{}`, "outcomeUnknown", "verifyOnly", 200},
		{"wrong-instructions", mainProfileTestRecord, "outcomeUnknown", "verifyOnly", 200},
		{"oversized", strings.Repeat(" ", 4<<20) + mainProfileTestRecord, "outcomeUnknown", "verifyOnly", 200},
		{"lost-response", "", "outcomeUnknown", "verifyOnly", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var writes atomic.Int32
			service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					io.WriteString(w, mainProfileTestRecord)
					return
				}
				writes.Add(1)
				if tc.status == 0 {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					conn.Close()
					return
				}
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			})
			current, err := service.ReadMainProfile(context.Background(), epoch, "mpa-instance")
			if err != nil {
				t.Fatal(err)
			}
			out, err := service.SaveMainProfileInstructions(context.Background(), epoch, current, "new")
			if err == nil || err.Code != tc.code || err.RetryDisposition != tc.retry || out.Record != nil || writes.Load() != 1 {
				t.Fatalf("wrong write result: %+v %v, writes=%d", out, err, writes.Load())
			}
			if current.Record.Profile.AgentsMD != "old" {
				t.Fatal("failure mutated saved state")
			}
			_, err = service.SaveMainProfileInstructions(context.Background(), epoch, current, "new")
			if err == nil || err.Code != "conflict" || writes.Load() != 1 {
				t.Fatal("uncertain snapshot permitted another write")
			}
		})
	}
}

func TestMainProfileSaveValidationAndIsolation(t *testing.T) {
	writes := 0
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes++
		}
		io.WriteString(w, mainProfileTestRecord)
	})
	current, err := service.ReadMainProfile(context.Background(), epoch, "mpa-instance")
	if err != nil {
		t.Fatal(err)
	}
	for _, instructions := range []string{strings.Repeat("\U0001f4dd", 40000), string([]byte{0xff})} {
		if _, err := service.SaveMainProfileInstructions(context.Background(), epoch, current, instructions); err == nil || err.Code != "invalidInput" {
			t.Fatalf("combined instruction budget or invalid UTF-8 accepted: %v", err)
		}
	}
	for _, forged := range []MainProfileState{{}, {Status: MainProfileConfigured, Record: current.Record}} {
		if _, err := service.SaveMainProfileInstructions(context.Background(), epoch, forged, "new"); err == nil {
			t.Fatal("unread profile permitted a write")
		}
	}
	other := New(service.conn, nil)
	if _, err := other.SaveMainProfileInstructions(context.Background(), epoch, current, "new"); err == nil {
		t.Fatal("another Service accepted the snapshot")
	}
	if err := service.conn.SetIdentity(epoch, "new-subject"); err != nil {
		t.Fatal(err)
	}
	newEpoch := service.conn.State().ConnectionEpoch
	for _, suppliedEpoch := range []uint64{epoch, newEpoch} {
		if _, err := service.SaveMainProfileInstructions(context.Background(), suppliedEpoch, current, "new"); err == nil || err.Code != "staleConnection" {
			t.Fatal("stale snapshot accepted")
		}
	}
	if writes != 0 {
		t.Fatalf("invalid saves reached Runtime: %d", writes)
	}
}

func TestMainProfileNoopAndConcurrentSave(t *testing.T) {
	var writes atomic.Int32
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			writes.Add(1)
		}
		w.Header().Set("ETag", `"7"`)
		io.WriteString(w, mainProfileTestRecord)
	})
	current, err := service.ReadMainProfile(context.Background(), epoch, "mpa-instance")
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	results := make(chan *dto.SafeError, 2)
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := service.SaveMainProfileInstructions(context.Background(), epoch, current, "old")
			results <- err
		}()
	}
	group.Wait()
	close(results)
	succeeded, conflicted := 0, 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if err.Code == "conflict" {
			conflicted++
		} else {
			t.Fatal(err)
		}
	}
	if succeeded != 1 || conflicted != 1 || writes.Load() != 1 {
		t.Fatalf("same snapshot raced: success=%d conflicts=%d writes=%d", succeeded, conflicted, writes.Load())
	}
}

func TestMainProfileDefaultModelAndInstructionBoundary(t *testing.T) {
	for _, defaultModel := range []bool{false, true} {
		t.Run(map[bool]string{false: "pinned", true: "default"}[defaultModel], func(t *testing.T) {
			var original MainProfileRecord
			if err := json.Unmarshal([]byte(mainProfileTestRecord), &original); err != nil {
				t.Fatal(err)
			}
			if defaultModel {
				original.Profile.Model = nil
				original.Profile.Skills = []MainProfileSkill{}
			}
			limit := resourceInstructionLimit
			for _, skill := range original.Profile.Skills {
				limit -= len(skill.Content)
			}
			instructions := strings.Repeat("\U0001f4dd", limit)
			service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
				record := original
				if r.Method == http.MethodPut {
					var body struct {
						Profile MainProfile `json:"profile"`
					}
					if json.NewDecoder(r.Body).Decode(&body) != nil || body.Profile.AgentsMD != instructions {
						t.Error("instruction body not retained")
					}
					if defaultModel && (body.Profile.Model != nil || body.Profile.Skills == nil || len(body.Profile.Skills) != 0) {
						t.Error("default model or empty skills lost")
					}
					record.Profile = body.Profile
					record.Revision = 8
					record.ETag = `"8"`
					record.Digest = "sha256:new"
				}
				json.NewEncoder(w).Encode(record)
			})
			current, err := service.ReadMainProfile(context.Background(), epoch, "mpa-instance")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.SaveMainProfileInstructions(context.Background(), epoch, current, instructions+"x"); err == nil || err.Code != "invalidInput" {
				t.Fatal("combined instruction limit exceeded")
			}
			saved, err := service.SaveMainProfileInstructions(context.Background(), epoch, current, instructions)
			if err != nil || saved.Record == nil || saved.Record.Profile.AgentsMD != instructions {
				t.Fatalf("exact Unicode boundary rejected: %+v %v", saved, err)
			}
		})
	}
}

func TestMainProfileIdentityChangesDuringResponse(t *testing.T) {
	for _, duringWrite := range []bool{false, true} {
		t.Run(map[bool]string{false: "read", true: "write"}[duringWrite], func(t *testing.T) {
			var service *Service
			service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
				if (r.Method == http.MethodPut) == duringWrite {
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
					service.conn.Disconnect()
				}
				io.WriteString(w, mainProfileTestRecord)
			})
			current, err := service.ReadMainProfile(context.Background(), epoch, "mpa-instance")
			if duringWrite {
				if err != nil {
					t.Fatal(err)
				}
				current, err = service.SaveMainProfileInstructions(context.Background(), epoch, current, "new")
			}
			if err == nil || err.Code != "staleConnection" || current.Record != nil {
				t.Fatalf("old-identity profile exposed: %+v %v", current, err)
			}
		})
	}
}

func TestMainProfileRejectsChangedSettingsInSaveResponse(t *testing.T) {
	for _, change := range []struct{ old, new string }{
		{`"current-model"`, `"other-model"`},
		{`"pinned instructions"`, `"other instructions"`},
		{`"source-profile"`, `"other-source"`},
	} {
		t.Run(change.old, func(t *testing.T) {
			service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
				raw := mainProfileTestRecord
				if r.Method == http.MethodPut {
					raw = strings.Replace(raw, `"revision":7`, `"revision":8`, 1)
					raw = strings.Replace(raw, `\"7\"`, `\"8\"`, 1)
					raw = strings.Replace(raw, `"agentsMd":"old"`, `"agentsMd":"new"`, 1)
					raw = strings.Replace(raw, change.old, change.new, 1)
				}
				io.WriteString(w, raw)
			})
			current, err := service.ReadMainProfile(context.Background(), epoch, "mpa-instance")
			if err != nil {
				t.Fatal(err)
			}
			out, err := service.SaveMainProfileInstructions(context.Background(), epoch, current, "new")
			if err == nil || err.Code != "outcomeUnknown" || out.Record != nil {
				t.Fatalf("changed settings reported as saved: %+v %v", out, err)
			}
		})
	}
}
