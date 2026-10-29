package session

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveAssistantModelSwitch(t *testing.T) {
	if os.Getenv("OPENMPA_LIVE_MODEL_SWITCH") != "1" {
		t.Skip("explicit temporary assistant model regression")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	platform := cloud.New()
	state, err := platform.SignIn(ctx, dto.SignInRequest{AccessKey: os.Getenv("VOLCENGINE_ACCESS_KEY"), SecretKey: os.Getenv("VOLCENGINE_SECRET_KEY"), Region: "cn-beijing"})
	if err != nil {
		t.Fatal(err.Code)
	}
	if state.Principal == nil || state.Principal.AccountID != os.Getenv("OPENMPA_LIVE_ACCOUNT") {
		t.Fatal("account mismatch")
	}
	runtime, err := platform.GetRuntime(ctx, dto.RuntimeRequest{RuntimeID: os.Getenv("OPENMPA_LIVE_RUNTIME"), Region: "cn-beijing", ProjectName: "default"})
	if err != nil {
		t.Fatal(err.Code)
	}
	conn := connection.New()
	epoch, err := conn.Bind(connection.Binding{RuntimeID: runtime.RuntimeID, Endpoint: runtime.Endpoint, RuntimeKey: runtime.AuthorizerConfiguration.KeyAuth.APIKey, Scope: state.Principal.Scope}, false)
	if err != nil {
		t.Fatal(err.Code)
	}
	defer conn.Disconnect()
	service := New(conn, NewPendingStore(func() (string, error) { return t.TempDir(), nil }))
	SetCloud(service, platform)
	if os.Getenv("OPENMPA_LIVE_READ_EXISTING") == "1" {
		list := service.List(ctx, dto.ScopeRequest{ConnectionEpoch: epoch, ForceRefresh: true})
		if !list.OK {
			t.Fatal(list.Error.Code)
		}
		for _, item := range list.Value.Sessions {
			config := service.ExecutionConfig(ctx, dto.SessionRequest{ConnectionEpoch: epoch, SessionID: item.ID})
			if !config.OK {
				t.Log("existing config", item.ID, item.TraceEnabled, config.Error.Code, config.Error.HTTPStatus)
			} else {
				t.Log("existing config", item.ID, item.TraceEnabled, config.Value.Model, config.Value.ModelMode, config.Value.ExecutionConfigVersion)
			}
		}
		return
	}
	var api struct {
		Paths map[string]map[string]json.RawMessage
	}
	if err := conn.JSON(ctx, epoch, http.MethodGet, "/openapi.json", nil, &api); err != nil {
		t.Fatal(err.Code)
	}
	for method := range api.Paths["/api/v1/sessions/{session_id}/execution-config"] {
		t.Log("execution config method", method)
	}
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		t.Fatal(err)
	}
	bytes[6] = (bytes[6] & 15) | 64
	bytes[8] = (bytes[8] & 63) | 128
	id := fmt.Sprintf("%x-%x-%x-%x-%x", bytes[:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:])
	created := service.Create(ctx, dto.CreateRequest{ConnectionEpoch: epoch, SessionID: id})
	if !created.OK {
		t.Fatal("create", created.Error.Code, created.Error.HTTPStatus)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		result := service.Delete(cleanup, dto.SessionRequest{ConnectionEpoch: epoch, SessionID: id})
		if !result.OK {
			t.Error("cleanup", id, result.Error.Code)
		} else {
			t.Log("temporary assistant deleted", id)
		}
	}()
	config := service.ExecutionConfig(ctx, dto.SessionRequest{ConnectionEpoch: epoch, SessionID: id})
	if !config.OK {
		t.Fatal("read config", config.Error.Code, config.Error.HTTPStatus)
	}
	t.Log("initial config", config.Value.Model, config.Value.ModelMode, config.Value.ExecutionConfigVersion)
	catalog := service.ModelCatalog(ctx, dto.SessionRequest{ConnectionEpoch: epoch, SessionID: id})
	if !catalog.OK || len(catalog.Value.Models) < 2 {
		t.Fatal("catalog unavailable")
	}
	model := catalog.Value.Models[0].ID
	if model == config.Value.Model {
		model = catalog.Value.Models[1].ID
	}
	changed := service.UpdateExecutionConfig(ctx, dto.ExecutionConfigUpdateRequest{ConnectionEpoch: epoch, SessionID: id, ETag: config.Value.ETag, ModelMode: "replace", Model: model, MCPMode: config.Value.MCPMode, MCPServers: config.Value.MCPServers})
	if !changed.OK {
		t.Log("update failed", changed.Error.Code, changed.Error.HTTPStatus)
		t.Fatal("model selection failed")
	}
	reread := service.ExecutionConfig(ctx, dto.SessionRequest{ConnectionEpoch: epoch, SessionID: id})
	if !reread.OK || reread.Value.Model != model || reread.Value.ModelMode != "replace" {
		t.Fatal("selection not persisted")
	}
	root := os.Getenv("OPENMPA_LIVE_MODEL_OUTPUT")
	if filepath.IsAbs(root) {
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
		data, _ := json.MarshalIndent(map[string]any{"before": config.Value, "after": reread.Value}, "", "  ")
		if err := os.WriteFile(filepath.Join(root, "model-switch.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("model persisted", model, reread.Value.ExecutionConfigVersion)
}
