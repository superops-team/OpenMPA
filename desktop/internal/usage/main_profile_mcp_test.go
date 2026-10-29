package usage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestMainProfileCompleteEditWithMCP(t *testing.T) {
	original := strings.Replace(mainProfileTestRecord, `"skills":[`, `"mcpSources":[{"resourceId":"agentkit:search","enabledTools":null}],"mcpServers":{"private":{"url":"https://example.test/mcp","headers":{"Authorization":"**********"},"enabledTools":[]}},"skills":[`, 1)
	writes := 0
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			io.WriteString(w, original)
			return
		}
		writes++
		if r.Header.Get("If-Match") != `"7"` || r.Header.Get("Idempotency-Key") == "" {
			t.Error("missing CAS or idempotency")
		}
		var body struct {
			Profile         MainProfile `json:"profile"`
			SourceProfileID string      `json:"sourceProfileId"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("invalid profile body")
		}
		if body.Profile.Model.ID != "main-model" || body.Profile.AgentsMD != "new main" || len(body.Profile.Skills) != 2 || body.SourceProfileID != "source-profile" || body.Profile.MCPServers["private"].Headers["Authorization"] != "Bearer test-only" || body.Profile.MCPSources[0].EnabledTools == nil || len(*body.Profile.MCPSources[0].EnabledTools) != 1 {
			t.Error("complete configuration was not preserved")
		}
		var record MainProfileRecord
		json.Unmarshal([]byte(original), &record)
		record.Profile = publicMainProfile(body.Profile)
		record.Revision = 8
		record.ETag = `"8"`
		json.NewEncoder(w).Encode(record)
	})
	state, err := service.ReadMainProfile(context.Background(), epoch, "mpa-instance")
	if err != nil {
		t.Fatal(err)
	}
	draft := cloneMainProfileRecord(*state.Record).Profile
	draft.AgentsMD = "new main"
	draft.Model = &MainProfileModel{ID: "main-model"}
	draft.Skills = append(draft.Skills, MainProfileSkill{Name: "new-skill", Version: "2.0.0", Content: "instructions"})
	tools := []string{"search"}
	draft.MCPSources[0].EnabledTools = &tools
	server := draft.MCPServers["private"]
	server.Headers["Authorization"] = "Bearer test-only"
	draft.MCPServers["private"] = server
	out, err := service.SaveMainProfile(context.Background(), epoch, state, draft)
	if err != nil || out.Record == nil || writes != 1 {
		t.Fatalf("save failed: %v", err)
	}
	if out.Record.Profile.MCPServers["private"].Headers["Authorization"] != mainProfileMaskedHeader || out.Record.Profile.MCPServers["private"].EnabledTools == nil || len(*out.Record.Profile.MCPServers["private"].EnabledTools) != 0 {
		t.Fatal("credential masking or disabled selection lost")
	}
	if state.snapshot.record.Profile.MCPSources[0].EnabledTools != nil {
		t.Fatal("editable projection mutated snapshot")
	}
}
func TestMainProfileInstructionEditRetainsMCPReferences(t *testing.T) {
	original := strings.Replace(mainProfileTestRecord, `"skills":[`, `"mcpSources":[{"resourceId":"agentkit:search","enabledTools":[]}],"skills":[`, 1)
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			io.WriteString(w, original)
			return
		}
		var body struct {
			Profile MainProfile `json:"profile"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if len(body.Profile.MCPSources) != 1 || body.Profile.MCPSources[0].EnabledTools == nil || len(*body.Profile.MCPSources[0].EnabledTools) != 0 {
			t.Error("disabled MCP source dropped")
		}
		saved := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(original, `"agentsMd":"old"`, `"agentsMd":"new"`), `"revision":7`, `"revision":8`), `\"7\"`, `\"8\"`)
		io.WriteString(w, saved)
	})
	state, err := service.ReadMainProfile(context.Background(), epoch, "mpa-instance")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveMainProfileInstructions(context.Background(), epoch, state, "new"); err != nil {
		t.Fatal(err)
	}
}
func TestMainProfileMCPRejectsInvalidWrites(t *testing.T) {
	base := MainProfile{Skills: []MainProfileSkill{}, MCPServers: map[string]MainProfileMCPServer{"server": {URL: "https://example.test/mcp", Headers: map[string]string{"Authorization": "test-only"}}}}
	cases := map[string]func(*MainProfile){
		"http": func(p *MainProfile) {
			s := p.MCPServers["server"]
			s.URL = "http://example.test/mcp"
			p.MCPServers["server"] = s
		},
		"reserved-header": func(p *MainProfile) { p.MCPServers["server"].Headers["X-Ve-TIP-Token"] = "test" },
		"masked":          func(p *MainProfile) { p.MCPServers["server"].Headers["Authorization"] = mainProfileMaskedHeader },
		"encrypted":       func(p *MainProfile) { p.MCPServers["server"].Headers["Authorization"] = "fernet:v1:sealed" },
		"duplicate-source": func(p *MainProfile) {
			p.MCPSources = []MainProfileMCPSource{{ResourceID: "agentkit:x"}, {ResourceID: "agentkit:x"}}
		},
		"reserved-source": func(p *MainProfile) { p.MCPSources = []MainProfileMCPSource{{ResourceID: "profile:x"}} },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Error("invalid profile sent upstream")
				}
				io.WriteString(w, mainProfileTestRecord)
			})
			state, err := service.ReadMainProfile(context.Background(), epoch, "mpa-instance")
			if err != nil {
				t.Fatal(err)
			}
			profile := cloneMainProfileRecord(MainProfileRecord{Profile: base}).Profile
			change(&profile)
			_, err = service.SaveMainProfile(context.Background(), epoch, state, profile)
			if err == nil || err.Code != "invalidInput" {
				t.Fatal("invalid MCP write accepted")
			}
		})
	}
	// Null means all tools; [] means disabled. They must remain different on the wire.
	all := MainProfileMCPSource{ResourceID: "all"}
	empty := []string{}
	disabled := MainProfileMCPSource{ResourceID: "disabled", EnabledTools: &empty}
	if reflect.DeepEqual(all.EnabledTools, disabled.EnabledTools) {
		t.Fatal("tool selection semantics collapsed")
	}
}

func TestMainProfileExplicitCreateUsesAbsentCAS(t *testing.T) {
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.Header.Get("If-None-Match") != "*" || r.Header.Get("If-Match") != "" || r.Header.Get("Idempotency-Key") == "" {
			t.Fatal("initialization is not create-only")
		}
		var body struct {
			Profile MainProfile `json:"profile"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		var record MainProfileRecord
		json.Unmarshal([]byte(mainProfileTestRecord), &record)
		record.Profile = body.Profile
		record.SourceProfileID = nil
		record.SourceProfileDigest = nil
		record.Revision = 1
		record.ETag = `"1"`
		json.NewEncoder(w).Encode(record)
	})
	state, err := service.CreateMainProfile(context.Background(), epoch, "mpa-instance", MainProfile{Skills: []MainProfileSkill{}})
	if err != nil || state.Record == nil || state.Record.Revision != 1 {
		t.Fatalf("create failed: %v", err)
	}
}
func TestMainProfileCredentialNoopAcceptsUnchangedRevision(t *testing.T) {
	original := strings.Replace(mainProfileTestRecord, `"skills":[`, `"mcpServers":{"server":{"url":"https://example.test/mcp","headers":{"Authorization":"**********"},"enabledTools":null}},"skills":[`, 1)
	service, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, original) })
	state, err := service.ReadMainProfile(context.Background(), epoch, "mpa-instance")
	if err != nil {
		t.Fatal(err)
	}
	draft := cloneMainProfileRecord(*state.Record).Profile
	draft.MCPServers["server"].Headers["Authorization"] = "test-only-unchanged"
	out, err := service.SaveMainProfile(context.Background(), epoch, state, draft)
	if err != nil || out.Record.Revision != 7 {
		t.Fatalf("credential no-op rejected: %v", err)
	}
}
