package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func TestModelCatalogReadsLiveMetadataShapeAndCoalesces(t *testing.T) {
	fixture, err := os.ReadFile("testdata/ark-responses-metadata.json")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	c := testCloud(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("Action") != "" && r.URL.Query().Get("Version") != ArkAPIVersion {
			t.Error("incorrect Ark API version")
		}
		switch r.URL.Query().Get("Action") {
		case "ListModelChargeItems":
			once.Do(func() { close(started) })
			<-release
			io.WriteString(w, `{"Result":{"TotalCount":2,"PageNumber":1,"PageSize":100,"Items":[{"FoundationModelName":"kimi-k3-1-pro","State":"Available","SubServices":[{"SubService":"base","Status":"Available"}]},{"FoundationModelName":"unopened","State":"Unavailable"}]}}`)
		case "ListFoundationModels":
			io.WriteString(w, `{"Result":{"Items":[{"Name":"kimi-k3-1-pro","PrimaryVersion":"261024"}]}}`)
		case "ListModelMetaDatas":
			var request struct {
				FoundationModelName, FoundationModelVersion string
				Keys                                        []string
				PageNum, PageSize                           int
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil || request.FoundationModelName != "kimi-k3-1-pro" || (request.FoundationModelVersion != "261024" && request.FoundationModelVersion != "260901") || !reflect.DeepEqual(request.Keys, []string{"api.responses.support"}) || request.PageNum != 1 {
				t.Error("metadata query did not target exact model/version/key")
			}
			io.WriteString(w, `{"Result":`)
			if request.FoundationModelVersion == "261024" {
				w.Write(fixture)
			} else {
				value, _ := json.Marshal(map[string]any{"ModelMetaDatas": []any{map[string]any{"FoundationModelName": request.FoundationModelName, "FoundationModelVersion": request.FoundationModelVersion, "Key": "api.responses.support", "Data": []string{"true"}}}, "Total": 1})
				w.Write(value)
			}
			io.WriteString(w, `}`)
		case "ListApiKeys":
			io.WriteString(w, `{"Result":{"Items":[{"Id":1,"Name":"default"}],"TotalCount":1}}`)
		case "GetRawApiKey":
			io.WriteString(w, `{"Result":{"ApiKey":"TEST_KEY"}}`)
		case "ListModelActivations":
			io.WriteString(w, `{"Result":{"Items":[{"FoundationModelName":"kimi-k3-1-pro","State":"Available"}],"TotalCount":1}}`)
		case "":
			io.WriteString(w, `{"data":[{"id":"kimi-k3-1-pro-261024","name":"kimi-k3-1-pro","domain":"LLM"},{"id":"kimi-k3-1-pro-260901","name":"kimi-k3-1-pro","domain":"LLM"}]}`)
		default:
			t.Errorf("unexpected action %s", r.URL.Query().Get("Action"))
			w.WriteHeader(400)
		}
	})
	c.credential = Credentials{AccessKey: "ak", SecretKey: "sk"}
	c.transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "ark.cn-beijing.volces.com" {
			local, _ := url.Parse(c.endpoint("ark", arkControlRegion))
			copy := request.Clone(request.Context())
			copy.URL.Scheme, copy.URL.Host = local.Scheme, local.Host
			return http.DefaultTransport.RoundTrip(copy)
		}
		return http.DefaultTransport.RoundTrip(request)
	})
	c.state = dto.PlatformState{SignedIn: true, Epoch: 1, Principal: &dto.Principal{Scope: "account/user"}}
	results := make(chan dto.ModelCatalog, 2)
	for range 2 {
		go func() {
			catalog, failure := c.ModelCatalog(context.Background())
			if failure != nil {
				t.Error(failure)
			}
			results <- catalog
		}()
	}
	<-started
	close(release)
	for range 2 {
		if result := <-results; len(result.Models) != 2 || result.Models[0].ID != "kimi-k3-1-pro-261024" || result.Models[1].ID != "kimi-k3-1-pro-260901" {
			t.Errorf("unexpected catalog: %+v", result)
		}
	}
	if calls.Load() != 8 {
		t.Fatalf("coalesced load made %d requests", calls.Load())
	}
	first, _ := c.ModelCatalog(context.Background())
	first.Models[0].ID = "mutated"
	cached, _ := c.ModelCatalog(context.Background())
	if cached.Models[0].ID != "kimi-k3-1-pro-261024" || calls.Load() != 8 {
		t.Fatal("cache alias or cache miss")
	}
}

func TestResponsesSupportRequiresMatchingVersionAndExplicitData(t *testing.T) {
	model := arkFoundationModel{Name: "kimi-k3-1-pro", PrimaryVersion: "261024"}
	row := arkModelMetadata{FoundationModelName: model.Name, FoundationModelVersion: model.PrimaryVersion, Key: "api.responses.support", Data: []string{"true"}}
	for _, tc := range []struct {
		name             string
		edit             func(*arkModelMetadata)
		supported, known bool
	}{
		{"explicit true", func(*arkModelMetadata) {}, true, true},
		{"explicit false", func(r *arkModelMetadata) { r.Data = []string{"false"} }, false, true},
		{"wrong version", func(r *arkModelMetadata) { r.FoundationModelVersion = "old" }, false, false},
		{"wrong model", func(r *arkModelMetadata) { r.FoundationModelName = "another" }, false, false},
		{"tool api description", func(r *arkModelMetadata) { r.Key = "web_browsing.support_apis"; r.Data = []string{"Responses API"} }, false, false},
		{"no value", func(r *arkModelMetadata) { r.Data = nil }, false, false},
		{"unknown value", func(r *arkModelMetadata) { r.Data = []string{"yes"} }, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := row
			tc.edit(&value)
			supported, known := responsesSupport(model, []arkModelMetadata{value})
			if supported != tc.supported || known != tc.known {
				t.Fatalf("got %v/%v", supported, known)
			}
		})
	}
}

func TestModelCatalogCachesEmptyButRejectsStaleAccount(t *testing.T) {
	var calls atomic.Int32
	c := testCloud(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.WriteString(w, `{"Result":{"Items":[],"TotalCount":0}}`)
	})
	c.credential = Credentials{AccessKey: "ak", SecretKey: "sk"}
	c.state = dto.PlatformState{SignedIn: true, Epoch: 1}
	for range 2 {
		result, err := c.ModelCatalog(context.Background())
		if err != nil || len(result.Models) != 0 {
			t.Fatal(result, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("empty catalog was not cached")
	}
	c.SignOut()
	if _, err := c.ModelCatalog(context.Background()); err == nil {
		t.Fatal("signed-out account got cached catalog")
	}
}

func TestCancelledCatalogWaitDoesNotPoisonSharedLoad(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	c := testCloud(t, func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		io.WriteString(w, `{"Result":{"Items":[],"TotalCount":0}}`)
	})
	c.credential = Credentials{AccessKey: "ak", SecretKey: "sk"}
	c.state = dto.PlatformState{SignedIn: true, Epoch: 1}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *dto.SafeError, 1)
	go func() { _, err := c.ModelCatalog(ctx); done <- err }()
	<-started
	cancel()
	if err := <-done; err == nil || err.Code != "cancelled" {
		t.Fatal("wait was not cancelled")
	}
	close(release)
	deadline, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if _, err := c.ModelCatalog(deadline); err != nil {
		t.Fatal("shared request failed", err)
	}
}

func TestAbsentResponsesMetadataIsNotAProtocolMatch(t *testing.T) {
	c := testCloud(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"ResponseMetadata":{"Action":"ListModelMetaDatas"},"Result":null}`)
	})
	supported, err := c.modelSupportsResponses(context.Background(), Credentials{}, arkFoundationModel{Name: "hitem3d-2-0", PrimaryVersion: "251223"})
	if err != nil || supported {
		t.Fatalf("absent capability must not match: %v %+v", supported, err)
	}
}

func TestFoundationModelsRejectRepeatedLastPage(t *testing.T) {
	var calls atomic.Int32
	c := testCloud(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.WriteString(w, `{"Result":{"Items":[{"Name":"model","PrimaryVersion":"v1"}],"TotalCount":2}}`)
	})
	_, err := c.foundationModels(context.Background(), Credentials{}, arkControlRegion, []string{"model"})
	if err == nil || err.Code != "invalidResponse" || calls.Load() != 2 {
		t.Fatalf("repeated final page accepted: %v requests %d", err, calls.Load())
	}
}
