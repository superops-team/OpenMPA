package cloud

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSkillStorageUsesFixedRegionalHostAndPrivateBucket(t *testing.T) {
	c := New()
	c.credential = Credentials{AccessKey: "test-ak", SecretKey: "do-not-expose", Token: "session-token"}
	c.state = dto.PlatformState{SignedIn: true, Epoch: 4, Principal: &dto.Principal{AccountID: "123", Scope: "account/user"}}
	calls := 0
	c.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if !(strings.HasSuffix(r.URL.Host, ".tos-cn-beijing.volces.com") || r.URL.Host == "tos-cn-beijing.volces.com") || r.URL.Scheme != "https" || r.Header.Get("Authorization") == "" {
			t.Errorf("unsafe or unsigned storage request: %s", r.URL.Host)
		}
		if r.Header.Get("x-tos-acl") == "public-read" {
			t.Error("public upload")
		}
		if r.Method == http.MethodPut {
			raw, _ := io.ReadAll(r.Body)
			if string(raw) != "archive" {
				t.Error("wrong package")
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"X-Tos-Bucket-Region": []string{"cn-beijing"}}, Body: io.NopCloser(strings.NewReader(func() string {
			if r.Method == http.MethodGet {
				owner := sha256.Sum256([]byte("123/default"))
				return fmt.Sprintf(`{"Buckets":[{"Name":"openmpa-skills-%x-cn-beijing","Location":"cn-beijing"}]}`, owner[:8])
			}
			return ""
		}())), Request: r}, nil
	})
	out, err := c.PutSkillArchive(context.Background(), "cn-beijing", "default", []byte("archive"))
	if err != nil || calls != 2 || out.Bucket == "" || !strings.Contains(out.URL, "/agentkit/skills/") {
		t.Fatalf("storage: %+v %v, calls=%d", out, err, calls)
	}
}
func TestSkillStorageDoesNotExposeProviderErrors(t *testing.T) {
	c := New()
	c.credential = Credentials{AccessKey: "ak", SecretKey: "sk"}
	c.state = dto.PlatformState{SignedIn: true, Epoch: 1, Principal: &dto.Principal{AccountID: "123"}}
	c.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"Code":"AccessDenied","Message":"do-not-expose"}`)), Request: r}, nil
	})
	_, err := c.PutSkillArchive(context.Background(), "cn-beijing", "default", []byte("archive"))
	if err == nil || err.Code != "denied" || strings.Contains(err.Message, "do-not-expose") {
		t.Fatalf("unsafe storage failure: %v", err)
	}
}
