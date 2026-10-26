package artifact

import (
	"context"
	"encoding/json"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadBoundHTTPAuthAndScope(t *testing.T) {
	var header http.Header
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/runtime/api/v1/sessions/session-1" {
			_, _ = w.Write([]byte(`{"id":"session-1"}`))
			return
		}
		header = r.Header.Clone()
		requestedPath = r.URL.Query().Get("path")
		if r.URL.Path != "/runtime/api/v1/sessions/session-1/files/download" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Length", "8")
		_, _ = w.Write([]byte("complete"))
	}))
	defer server.Close()
	c := connection.New()
	epoch, e := c.Bind(connection.Binding{Endpoint: server.URL + "/runtime", RuntimeID: "runtime-1", Scope: "verified-owner-scope", RuntimeKey: "fixture-runtime-key"}, true)
	if e != nil {
		t.Fatal(e)
	}
	if e := c.SetBusinessAuth(dto.IdentityRequest{ConnectionEpoch: epoch, AuthMode: "explicitTest", TestSubject: "fixture-owner", VerifiedTestDeployment: true}); e != nil {
		t.Fatal(e)
	}
	epoch = c.State().ConnectionEpoch
	d := &fixtureDialog{target: filepath.Join(t.TempDir(), "saved.md"), confirm: true}
	s := NewWithDependencies(c, d, OSFiles{})
	r := request()
	r.ConnectionEpoch = epoch
	r.RemotePath = "/sandbox/报告 +#?&%..md"
	result := s.download(context.Background(), r, successfulProgress)
	if !result.OK || requestedPath != r.RemotePath || header.Get("Authorization") != "Bearer fixture-runtime-key" || header.Get("X-User-Id") != "fixture-owner" || header.Get("X-MPA-Channel-Key") != "" {
		t.Fatal("bound auth, scope or encoded path was not preserved")
	}
	wire, _ := json.Marshal(result)
	for _, secret := range []string{"fixture-runtime-key", "fixture-owner", "verified-owner-scope", d.target} {
		if strings.Contains(string(wire), secret) {
			t.Fatal("private transport value leaked")
		}
	}
}

func TestDownloadRealHTTPInterruptAndRedirect(t *testing.T) {
	for _, kind := range []string{"interrupt", "redirect", "denied", "stalled"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/sessions/session-1" && kind != "denied" {
					_, _ = w.Write([]byte(`{"id":"session-1"}`))
					return
				}
				switch kind {
				case "redirect":
					http.Redirect(w, r, "http://evil.invalid/token", 302)
				case "denied":
					w.WriteHeader(403)
					_, _ = w.Write([]byte(`{"message":"secret runtime key","code":"denied"}`))
				case "interrupt":
					w.Header().Set("Content-Length", "100")
					_, _ = w.Write([]byte("partial"))
				case "stalled":
					w.Header().Set("Content-Length", "100")
					_, _ = w.Write([]byte("partial"))
					w.(http.Flusher).Flush()
					<-r.Context().Done()
				}
			}))
			defer server.Close()
			c := connection.New()
			epoch, e := c.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope", RuntimeKey: "fixture-key"}, true)
			if e != nil {
				t.Fatal(e)
			}
			d := &fixtureDialog{target: filepath.Join(t.TempDir(), "saved.md"), confirm: true}
			original(t, d)
			s := NewWithDependencies(c, d, OSFiles{})
			r := request()
			r.ConnectionEpoch = epoch
			deadline := time.AfterFunc(3*time.Second, cancel)
			defer deadline.Stop()
			result := s.download(ctx, r, func(p DownloadProgress) error {
				if kind == "stalled" && p.BytesWritten > 0 {
					cancel()
				}
				return nil
			})
			if result.OK || result.Error == nil {
				t.Fatalf("%+v", result)
			}
			if kind == "stalled" && result.Error.Code != "cancelled" {
				t.Fatalf("%+v", result)
			}
			assertOriginal(t, d)
			wire, _ := json.Marshal(result)
			if strings.Contains(string(wire), "secret") || strings.Contains(string(wire), "evil") {
				t.Fatal("unsafe upstream details leaked")
			}
		})
	}
}

func TestDownloadEpochSwitchWhileHTTPReadIsBlocked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/sessions/session-1" {
			_, _ = w.Write([]byte(`{"id":"session-1"}`))
			return
		}
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	c := connection.New()
	epoch, e := c.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope", RuntimeKey: "fixture-key"}, true)
	if e != nil {
		t.Fatal(e)
	}
	d := &fixtureDialog{target: filepath.Join(t.TempDir(), "saved.md"), confirm: true}
	original(t, d)
	s := NewWithDependencies(c, d, OSFiles{})
	r := request()
	r.ConnectionEpoch = epoch
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var switched *time.Timer
	result := s.download(ctx, r, func(p DownloadProgress) error {
		if p.BytesWritten > 0 && switched == nil {
			switched = time.AfterFunc(20*time.Millisecond, c.Disconnect)
		}
		return nil
	})
	if switched != nil {
		switched.Stop()
	}
	if result.OK || result.Error.Code != "staleConnection" {
		t.Fatalf("%+v", result)
	}
	assertOriginal(t, d)
}
