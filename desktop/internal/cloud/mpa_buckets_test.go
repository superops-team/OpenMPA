package cloud

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func bucketClient() *Client {
	c := New()
	c.credential = Credentials{AccessKey: "test-ak", SecretKey: "PRIVATE_SK", Token: "PRIVATE_STS"}
	c.state = dto.PlatformState{SignedIn: true, Epoch: 7, Principal: &dto.Principal{AccountID: "account", Scope: "account/user"}}
	return c
}
func bucketResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}
func TestMPABucketsUseSignedReadOnlyRegionAndStudioPrefix(t *testing.T) {
	c := bucketClient()
	calls := 0
	c.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Scheme != "https" || r.URL.Host != "tos-cn-beijing.volces.com" || r.URL.Path != "/" || r.Header.Get("Authorization") == "" || r.Header.Get("X-Tos-Security-Token") != "PRIVATE_STS" {
			t.Errorf("unexpected unsigned/non-read request: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("X-Tos-Project-Name") != "" {
			t.Error("Studio picker must include all account projects")
		}
		return bucketResponse(r, 200, `{"Owner":{"ID":"private-owner"},"Buckets":[
   {"Name":"agentkit-platform-z","Location":"cn-beijing","ExtranetEndpoint":"https://private.invalid"},
   {"Name":"agentkit-platform-a","Location":"cn-beijing"},
   {"Name":"other-bucket","Location":"cn-beijing"},
   {"Name":"agentkit-platform-shanghai","Location":"cn-shanghai"},
   {"Name":"agentkit-platform-unlocated"},
   {"Name":"agentkit-platform-a","Location":"cn-beijing"},
   {"Name":"agentkit-platform-INVALID","Location":"cn-beijing"}]}`), nil
	})
	values, failure := c.ListMPATOSBuckets(context.Background(), 7, "cn-beijing")
	want := []MPATOSBucket{{"agentkit-platform-a", "cn-beijing"}, {"agentkit-platform-unlocated", "cn-beijing"}, {"agentkit-platform-z", "cn-beijing"}}
	if failure != nil || !reflect.DeepEqual(values, want) || calls != 1 {
		t.Fatalf("wrong eligible buckets: %+v %+v calls %d", values, failure, calls)
	}
}
func TestMPABucketErrorsRemainErrorsAndNeverExposeProviderText(t *testing.T) {
	c := bucketClient()
	c.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return bucketResponse(r, 403, `{"Code":"AccessDenied","Message":"PRIVATE_SK private-owner provider details"}`), nil
	})
	values, failure := c.ListMPATOSBuckets(context.Background(), 7, "cn-shanghai")
	if values != nil || failure == nil || failure.Code != "denied" || strings.Contains(failure.Message, "PRIVATE") {
		t.Fatalf("unsafe error: %+v %+v", values, failure)
	}
	c.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { return bucketResponse(r, 200, `{"Buckets":[]}`), nil })
	values, failure = c.ListMPATOSBuckets(context.Background(), 7, "cn-shanghai")
	if failure != nil || values == nil || len(values) != 0 {
		t.Fatal("successful empty directory was not distinguished from failure")
	}
}
func TestMPABucketsRejectStaleEpochAndLateAccountReplies(t *testing.T) {
	c := bucketClient()
	calls := 0
	c.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		c.SignOut()
		return bucketResponse(r, 200, `{"Buckets":[{"Name":"agentkit-platform-old","Location":"cn-beijing"}]}`), nil
	})
	for _, r := range []struct {
		epoch  uint64
		region string
	}{{6, "cn-beijing"}, {7, "untrusted-region"}} {
		if _, failure := c.ListMPATOSBuckets(context.Background(), r.epoch, r.region); failure == nil {
			t.Fatal("invalid scope accepted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid scope made a network request")
	}
	values, failure := c.ListMPATOSBuckets(context.Background(), 7, "cn-beijing")
	if values != nil || failure == nil || failure.Code != "staleConnection" {
		t.Fatal("late account response was published")
	}
}
