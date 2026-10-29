package session

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/trace"
)

func TestLiveExecutionArchive(t *testing.T) {
	if os.Getenv("OPENMPA_LIVE_ARCHIVE") != "1" {
		t.Skip("explicit temporary Runtime regression only")
	}
	root := os.Getenv("OPENMPA_LIVE_ARCHIVE_ROOT")
	if !filepath.IsAbs(root) {
		t.Fatal("absolute artifact root required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	platform := cloud.New()
	state, failure := platform.SignIn(ctx, dto.SignInRequest{AccessKey: os.Getenv("VOLCENGINE_ACCESS_KEY"), SecretKey: os.Getenv("VOLCENGINE_SECRET_KEY"), Region: "cn-beijing"})
	if failure != nil {
		t.Fatal(failure.Code)
	}
	if state.Principal == nil || state.Principal.AccountID != os.Getenv("OPENMPA_LIVE_ACCOUNT") {
		t.Fatal("account mismatch")
	}
	runtime, failure := platform.GetRuntime(ctx, dto.RuntimeRequest{RuntimeID: os.Getenv("OPENMPA_LIVE_RUNTIME"), Region: "cn-beijing", ProjectName: "default"})
	if failure != nil {
		t.Fatal(failure.Code)
	}
	conn := connection.New()
	epoch, failure := conn.Bind(connection.Binding{RuntimeID: runtime.RuntimeID, Endpoint: runtime.Endpoint, RuntimeKey: runtime.AuthorizerConfiguration.KeyAuth.APIKey, Scope: state.Principal.Scope}, false)
	if failure != nil {
		t.Fatal(failure.Code)
	}
	defer conn.Disconnect()
	service := New(conn, NewPendingStore(func() (string, error) { return filepath.Join(root, "pending"), nil }))
	SetCloud(service, platform)
	cache, err := trace.NewCache(filepath.Join(root, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	SetEventHook(service, func(event dto.ClientEvent) {
		if err := cache.AppendEvent(event); err != nil {
			t.Error(err)
		}
	})
	SetDeleteHook(service, func(id string) {
		if err := cache.DeleteSession(id); err != nil {
			t.Error(err)
		}
	})
	uuidBytes := make([]byte, 16)
	if _, err := rand.Read(uuidBytes); err != nil {
		t.Fatal(err)
	}
	uuidBytes[6] = (uuidBytes[6] & 0x0f) | 0x40
	uuidBytes[8] = (uuidBytes[8] & 0x3f) | 0x80
	id := fmt.Sprintf("%x-%x-%x-%x-%x", uuidBytes[:4], uuidBytes[4:6], uuidBytes[6:8], uuidBytes[8:10], uuidBytes[10:])
	created := service.Create(ctx, dto.CreateRequest{ConnectionEpoch: epoch, SessionID: id})
	if !created.OK {
		t.Fatal("create", created.Error.Code, created.Error.HTTPStatus)
	}
	t.Log("temporary session created", id)
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		result := service.Delete(cleanup, dto.SessionRequest{ConnectionEpoch: epoch, SessionID: id})
		if !result.OK {
			t.Error("temporary session cleanup failed", id, result.Error.Code)
		} else {
			t.Log("temporary session deleted", id)
		}
	}()
	config := service.EnsureExecutionConfig(ctx, dto.SessionRequest{ConnectionEpoch: epoch, SessionID: id})
	if !config.OK {
		t.Fatal("configuration", config.Error.Code)
	}
	content := "This is a desktop execution-history regression check. Use the command tool to execute only: printf 'openmpa-archive-check\\n'. Do not read or write files, use network, install anything, or contact anyone. Then reply with the printed marker."
	receipt := service.Submit(ctx, dto.SubmitRequest{ConnectionEpoch: epoch, SessionID: id, AssistantID: id, OperationID: id, IdempotencyKey: id, Content: content, ExecutionConfigVersion: config.Value.ExecutionConfigVersion})
	if !receipt.OK {
		t.Fatal("submit", receipt.Error.Code, receipt.Error.HTTPStatus)
	}
	kinds := map[string]int{}
	stream := service.stream(ctx, dto.SubscribeRequest{ConnectionEpoch: epoch, SubscriptionEpoch: 1, SessionID: id, InvocationID: receipt.Value.InvocationID}, func(event dto.ClientEvent) error {
		kinds[event.Kind]++
		if event.Kind == "error" {
			t.Log("stream error", event.Error)
		}
		return nil
	})
	t.Log("stream settled", stream.OK, "event kinds", kinds)
	if !stream.OK {
		t.Fatal("stream", stream.Error.Code)
	}
	archived, ok := cache.LoadTrace(id, receipt.Value.InvocationID)
	if !ok {
		t.Fatal("no local trace")
	}
	data, _ := json.MarshalIndent(archived, "", "  ")
	if err := os.WriteFile(filepath.Join(root, "archived-trace.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	toolCount := 0
	for _, step := range archived.Steps {
		if strings.Contains(step.Kind, "tool") || step.Category == "tool" {
			toolCount++
		}
	}
	t.Log("archived steps", len(archived.Steps), "tools", toolCount, "status", archived.Run["status"])
	if toolCount == 0 {
		t.Fatal("no tool execution observed in live verification")
	}
	remote := trace.NewWithCache(conn, cache).Read(ctx, trace.ReadRequest{ConnectionEpoch: epoch, SessionID: id, InvocationID: receipt.Value.InvocationID, ForceRefresh: true})
	if !remote.OK {
		t.Fatal("remote trace", remote.Error.Code)
	}
	data, _ = json.MarshalIndent(remote.Value, "", "  ")
	if err := os.WriteFile(filepath.Join(root, "merged-trace.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("merged steps", len(remote.Value.Steps), "coverage", remote.Value.Coverage)
}
