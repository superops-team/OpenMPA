package connection

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStudioCredentialIsOwnedByConnectionAndRouteScoped(t *testing.T) {
	for _, tc := range []struct {
		path   string
		studio bool
	}{
		{"/api/v1/studio/agents-md", true},
		{"/api/v1/studio/agent-info", true},
		{"/api/v1/studio/collaborators/registry/spaces?page=2", true},
		{"/api/v1/studio/collaborators/registry/agents?space_id=as-demo", true},
		{"/api/v1/studio/collaborators/registry/binding", true},
		{"/api/v1/studio/collaborators/registry/aliases/a-demo", true},
		{"/api/v1/sessions", false},
		{"/api/v1/studio/unknown", false},
		{"/api/v1/studio/collaborators/registry/aliases/a-demo/extra", false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expected := ""
				if tc.studio {
					expected = "runtime-key"
				}
				if r.Header.Get("X-MPA-Studio-Key") != expected || r.Header.Get("Authorization") != "Bearer runtime-key" {
					t.Error("credential route or ownership mismatch")
				}
				w.Write([]byte(`{}`))
			}))
			defer server.Close()
			c := New()
			epoch, _ := c.Bind(Binding{Endpoint: server.URL + "/runtime/prefix", RuntimeKey: "runtime-key"}, true)
			headers := http.Header{}
			headers.Set("X-MPA-Studio-Key", "untrusted")
			headers.Set("Authorization", "Bearer untrusted")
			resp, err := c.Request(context.Background(), epoch, "GET", tc.path, nil, headers)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
		})
	}
}

func TestStudioTraversalIsRejectedBeforeRequest(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer server.Close()
	c := New()
	epoch, _ := c.Bind(Binding{Endpoint: server.URL, RuntimeKey: "key"}, true)
	for _, path := range []string{"/api/v1/studio/../sessions", "/api/v1/studio/%2e%2e/sessions"} {
		if _, err := c.Request(context.Background(), epoch, "GET", path, nil, nil); err == nil {
			t.Fatal("traversal accepted")
		}
	}
	if hit {
		t.Fatal("invalid path reached transport")
	}
}
