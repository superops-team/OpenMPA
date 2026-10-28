package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func TestModelsReadCurrentRuntimeCapabilityOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/.well-known/agent-card.json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"capabilities":{"extensions":[{"uri":"urn:veadk:mpa:model-selection:v1","params":{"models":["new-runtime-model","new-runtime-model","another-model"],"defaultModel":"runtime-default"}}]}}`))
	}))
	defer server.Close()
	c := connection.New()
	epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL}, true)
	s := New(c, nil)
	result := s.Models(context.Background(), dto.ScopeRequest{ConnectionEpoch: epoch})
	if !result.OK || result.Value == nil || !result.Value.Supported || result.Value.DefaultModel != "runtime-default" || !reflect.DeepEqual(result.Value.Models, []string{"runtime-default", "new-runtime-model", "another-model"}) {
		t.Fatalf("unexpected catalog: %+v", result)
	}
	c.Disconnect()
	if stale := s.Models(context.Background(), dto.ScopeRequest{ConnectionEpoch: epoch}); stale.OK {
		t.Fatal("old connection catalog remained usable")
	}
}

func TestModelsNeverInventFallbackCatalog(t *testing.T) {
	for _, body := range []string{`{"capabilities":{}}`, `{"capabilities":{"extensions":[]}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		c := connection.New()
		epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL}, true)
		result := New(c, nil).Models(context.Background(), dto.ScopeRequest{ConnectionEpoch: epoch})
		server.Close()
		if !result.OK || result.Value == nil || result.Value.Supported || len(result.Value.Models) != 0 {
			t.Fatalf("invented models: %+v", result)
		}
	}
}

func TestModelsRejectMalformedCapability(t *testing.T) {
	for _, body := range []string{`{"capabilities":{"extensions":[{"uri":"urn:veadk:mpa:model-selection:v1","params":{"models":"invalid","defaultModel":"a"}}]}}`, `{"capabilities":{"extensions":[{"uri":"urn:veadk:mpa:model-selection:v1","params":{"models":["a\nsecret"],"defaultModel":"a"}}]}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		c := connection.New()
		epoch, _ := c.Bind(connection.Binding{Endpoint: server.URL}, true)
		result := New(c, nil).Models(context.Background(), dto.ScopeRequest{ConnectionEpoch: epoch})
		server.Close()
		if result.OK || result.Error == nil || result.Error.Code != "invalidResponse" {
			t.Fatalf("accepted invalid catalog: %+v", result)
		}
	}
}
