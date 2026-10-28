package connection

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChannelUsesHostVerifiedRuntimeCredentialAndOwner(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-MPA-Channel-Key") != "runtime-key" || r.Header.Get("X-User-Id") != "account/user" {
			t.Errorf("channel identity not inherited from verified cloud binding")
		}
		io.WriteString(w, `{}`)
	}))
	defer server.Close()
	c := New()
	epoch, _ := c.Bind(Binding{Endpoint: server.URL, RuntimeKey: "runtime-key", ChannelOwnerID: "account/user"}, true)
	if !c.HasChannelOwnerIdentity() {
		t.Fatal("verified channel owner missing")
	}
	var out map[string]any
	if err := c.Do(context.Background(), epoch, http.MethodGet, "/api/v1/channels/capabilities", nil, &out, "channel"); err != nil {
		t.Fatal(err)
	}
}
