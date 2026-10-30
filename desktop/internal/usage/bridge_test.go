package usage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func testBridge(core *Service) *Bridge {
	b := NewBridge(core)
	page := &mygo.Page{}
	b.page = func(context.Context) *mygo.Page { return page }
	return b
}

func TestBridgeHasOnlyConcreteBindableMethods(t *testing.T) {
	b := NewBridge(New(nil, nil))
	mygo.BindAs("StudioContractTest", b)
	typ := reflect.TypeOf(b)
	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i)
		if method.Type.NumIn() != 3 || method.Type.In(1) != reflect.TypeFor[context.Context]() || method.Type.NumOut() != 1 {
			t.Fatalf("invalid method %s", method.Name)
		}
	}
}

func TestBridgeJSONSnapshotIsOneUseAndPreservesPrivateRecord(t *testing.T) {
	var writes atomic.Int32
	core, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			writes.Add(1)
			var body struct {
				Profile MainProfile `json:"profile"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Profile.Model.ID != "current-model" || body.Profile.Skills[0].Content != "pinned instructions" {
				t.Error("private profile was replaced")
			}
		}
		io.WriteString(w, mainProfileTestRecord)
	})
	b := testBridge(core)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	read := b.ReadMainProfile(ctx, MainProfileReadRequest{epoch, "mpa-instance"})
	if !read.OK || read.Value.SnapshotID == "" {
		t.Fatalf("read: %+v", read)
	}
	read.Value.Record.Profile.Model.ID = "forged"
	raw, _ := json.Marshal(read.Value)
	if strings.Contains(string(raw), "attempted") || strings.Contains(string(raw), "owner") && !strings.Contains(string(raw), `"updatedBy":"owner"`) {
		t.Fatal("private authority leaked")
	}
	var roundtrip MainProfileView
	if json.Unmarshal(raw, &roundtrip) != nil {
		t.Fatal("invalid view JSON")
	}
	var wg sync.WaitGroup
	results := make(chan MainProfileResult, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- b.SaveMainProfileInstructions(ctx, MainProfileSaveRequest{epoch, roundtrip.SnapshotID, "old"})
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for result := range results {
		if result.OK {
			successes++
			if result.Value.SnapshotID == roundtrip.SnapshotID {
				t.Fatal("token was reused")
			}
		} else if result.Value != nil || result.Error.Code != "conflict" {
			t.Fatalf("wrong failure envelope: %+v", result)
		}
	}
	if successes != 1 || writes.Load() != 1 {
		t.Fatalf("replayed writes: successes %d writes %d", successes, writes.Load())
	}
}

func TestBridgeRejectsExpiredForeignCanceledAndWrongEpochSnapshots(t *testing.T) {
	for _, mode := range []string{"guessed", "foreign", "page", "lifetime", "canceled", "expired", "epoch", "scope", "invalid-content"} {
		t.Run(mode, func(t *testing.T) {
			var writes atomic.Int32
			core, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes.Add(1)
				}
				io.WriteString(w, mainProfileTestRecord)
			})
			b := testBridge(core)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			out := b.ReadMainProfile(ctx, MainProfileReadRequest{epoch, "mpa-instance"})
			if !out.OK {
				t.Fatal(out.Error)
			}
			token := out.Value.SnapshotID
			saveCtx, suppliedEpoch, text := ctx, epoch, "old"
			switch mode {
			case "guessed":
				token = strings.Repeat("a", 64)
			case "foreign":
				b = testBridge(core)
			case "page":
				b.page = func(context.Context) *mygo.Page { return &mygo.Page{} }
			case "lifetime":
				var lifetimeCancel context.CancelFunc
				saveCtx, lifetimeCancel = context.WithCancel(context.Background())
				defer lifetimeCancel()
			case "canceled":
				cancel()
			case "expired":
				b.now = func() time.Time { return time.Now().Add(snapshotTTL) }
			case "epoch":
				suppliedEpoch++
			case "scope":
				if err := core.conn.SetBusinessScope(epoch, "/business/new"); err != nil {
					t.Fatal(err)
				}
			case "invalid-content":
				text = string([]byte{0xff})
			}
			saved := b.SaveMainProfileInstructions(saveCtx, MainProfileSaveRequest{suppliedEpoch, token, text})
			if saved.OK || saved.Value != nil || saved.Error == nil || writes.Load() != 0 {
				t.Fatalf("unauthorized write: %+v", saved)
			}
			if mode == "invalid-content" && b.SaveMainProfileInstructions(ctx, MainProfileSaveRequest{epoch, token, "old"}).OK {
				t.Fatal("invalid attempt did not consume token")
			}
		})
	}
}

func TestBridgeSnapshotBoundAndFailureStatus(t *testing.T) {
	core, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/missing/") {
			w.WriteHeader(404)
			io.WriteString(w, `{}`)
			return
		}
		io.WriteString(w, mainProfileTestRecord)
	})
	b := testBridge(core)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < snapshotLimit+5; i++ {
		if out := b.ReadMainProfile(ctx, MainProfileReadRequest{epoch, "mpa-instance"}); !out.OK {
			t.Fatal(out.Error)
		}
	}
	if len(b.snapshots) != snapshotLimit {
		t.Fatal("unbounded snapshots")
	}
	out := b.ReadMainProfile(ctx, MainProfileReadRequest{epoch, "missing"})
	if out.OK || out.Status != MainProfileUnsupportedOrUnconfigured || out.Value != nil || out.Error.HTTPStatus != 404 {
		t.Fatalf("lost missing capability status: %+v", out)
	}
}

func TestBridgeIdentityUsesExistingRuntimeMetadataAndNeverPlatformID(t *testing.T) {
	for _, id := range []string{"mpa-current", "r-platform"} {
		t.Run(id, func(t *testing.T) {
			core, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/runtime-console/overview" {
					t.Error("unknown compatibility fallback")
				}
				json.NewEncoder(w).Encode(map[string]any{"agent": map[string]string{"id": id}})
			})
			out := testBridge(core).ResourceInstanceID(context.Background(), dto.ScopeRequest{ConnectionEpoch: epoch})
			if out.OK != (id == "mpa-current") {
				t.Fatalf("identity: %+v", out)
			}
		})
	}
}

func TestBridgeRejectsScopeChangesDuringProfileReadAndSave(t *testing.T) {
	for _, mode := range []string{"read", "fallback", "save"} {
		t.Run(mode, func(t *testing.T) {
			var core *Service
			var epoch uint64
			var writes atomic.Int32
			core, epoch = resourceTestService(t, func(w http.ResponseWriter, r *http.Request) {
				change := mode == "read" || mode == "fallback" && r.URL.Path == "/api/v1/runtime-console/overview" ||
					mode == "save" && r.Method == http.MethodPut
				if change {
					if err := core.conn.SetBusinessScope(epoch, "/business/new"); err != nil {
						t.Error(err)
					}
				}
				if r.Method == http.MethodPut {
					writes.Add(1)
				}
				if r.URL.Path == "/api/v1/runtime-console/overview" {
					io.WriteString(w, `{"agent":{"id":"mpa-instance"}}`)
					return
				}
				io.WriteString(w, mainProfileTestRecord)
			})
			b := testBridge(core)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			instanceID := "mpa-instance"
			if mode == "fallback" {
				instanceID = ""
			}
			out := b.ReadMainProfile(ctx, MainProfileReadRequest{epoch, instanceID})
			if mode == "save" {
				if !out.OK {
					t.Fatal(out.Error)
				}
				out = b.SaveMainProfileInstructions(ctx, MainProfileSaveRequest{epoch, out.Value.SnapshotID, "old"})
			}
			if out.OK || out.Value != nil || out.Error == nil || out.Error.Code != "staleConnection" || len(b.snapshots) != 0 {
				t.Fatalf("scope relabeled: %+v", out)
			}
			if writes.Load() != map[bool]int32{true: 1, false: 0}[mode == "save"] {
				t.Fatal("unexpected write")
			}
		})
	}
}
