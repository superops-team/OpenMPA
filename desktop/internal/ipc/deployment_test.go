package ipc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type deploymentTransport struct {
	calls         int
	account, user string
	badEndpoint   bool
	missingTool   bool
}

func (f *deploymentTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.calls++
	result := map[string]interface{}{"AccountId": f.account, "UserId": f.user, "IdentityType": "User"}
	if r.URL.Query().Get("Action") == "GetRuntime" {
		var req struct {
			RuntimeID string `json:"RuntimeId"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		endpoint := "https://runtime.invalid/root"
		if f.badEndpoint {
			endpoint = "https://" + "user:secret@" + "runtime.invalid"
		}
		result = map[string]interface{}{"RuntimeId": req.RuntimeID, "ToolId": "tool-" + req.RuntimeID, "ProjectName": "default", "Endpoint": endpoint, "AuthorizerConfiguration": map[string]interface{}{"KeyAuth": map[string]interface{}{"ApiKey": "DO_NOT_PROJECT"}}}
		if f.missingTool {
			delete(result, "ToolId")
		}
	}
	b, _ := json.Marshal(map[string]interface{}{"Result": result})
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(b)))}, nil
}
func deploymentPlatform(t *testing.T) (*Platform, *deploymentTransport) {
	t.Helper()
	f := &deploymentTransport{account: "account-one", user: "user-one"}
	p := NewPlatform(cloud.NewWithTransport(f), connection.New())
	r := p.SignIn(context.Background(), dto.SignInRequest{Region: "cn-beijing", AccessKey: "fixture-ak", SecretKey: "fixture-sk"})
	if !r.OK {
		t.Fatal(r.Error)
	}
	return p, f
}
func bindDeployment(t *testing.T, p *Platform, id string) {
	t.Helper()
	r := p.BindRuntime(context.Background(), dto.RuntimeRequest{RuntimeID: id, Region: "cn-beijing", ProjectName: "default"})
	if !r.OK {
		t.Fatal(r.Error)
	}
}
func TestDeploymentTargetProjectsBoundToolAndCloudProfileOffline(t *testing.T) {
	p, f := deploymentPlatform(t)
	bindDeployment(t, p, "r-one")
	calls := f.calls
	target := DeploymentTarget(p)
	if target.Scope != "account-one/User/user-one" || target.CredentialProfileID != "user-one" || target.ToolID != "tool-r-one" || target.RuntimeID != "r-one" || target.Region != "cn-beijing" || target.ProjectName != "default" || target.ConnectionEpoch != p.Connection.State().ConnectionEpoch || target.PlatformEpoch != p.Cloud.Epoch() {
		t.Fatalf("target %+v", target)
	}
	if f.calls != calls {
		t.Fatal("deployment target contacted cloud")
	}
	b, _ := json.Marshal(target)
	if strings.Contains(string(b), "DO_NOT_PROJECT") {
		t.Fatal("target leaked gateway credential")
	}
	old := target.ConnectionEpoch
	p.Connection.SetIdentity(old, "fixture-business-user")
	next := DeploymentTarget(p)
	if next.ConnectionEpoch == old || next.ToolID != target.ToolID || next.Scope != target.Scope {
		t.Fatalf("business epoch projection %+v", next)
	}
}
func TestDeploymentTargetChangesOnlyAfterSuccessfulRuntimeBinding(t *testing.T) {
	p, f := deploymentPlatform(t)
	bindDeployment(t, p, "r-one")
	before := DeploymentTarget(p)
	f.badEndpoint = true
	r := p.BindRuntime(context.Background(), dto.RuntimeRequest{RuntimeID: "r-two", Region: "cn-beijing", ProjectName: "default"})
	if r.OK {
		t.Fatal("invalid endpoint bound")
	}
	if DeploymentTarget(p) != before {
		t.Fatal("failed binding changed deployment target")
	}
	f.badEndpoint = false
	bindDeployment(t, p, "r-two")
	after := DeploymentTarget(p)
	if after.RuntimeID != "r-two" || after.ToolID != "tool-r-two" || after.ConnectionEpoch == before.ConnectionEpoch {
		t.Fatalf("runtime switch %+v", after)
	}
}
func TestDeploymentTargetClearedOnSignOutAndAccountChange(t *testing.T) {
	p, f := deploymentPlatform(t)
	bindDeployment(t, p, "r-one")
	p.SignOut()
	target := DeploymentTarget(p)
	if target.RuntimeID != "" || target.ToolID != "" || target.Scope != "" || Target(p).RuntimeID != "" {
		t.Fatalf("signed out target %+v", target)
	}
	f.account = "account-two"
	f.user = "user-two"
	r := p.SignIn(context.Background(), dto.SignInRequest{Region: "cn-beijing", AccessKey: "fixture-ak", SecretKey: "fixture-sk"})
	if !r.OK {
		t.Fatal(r.Error)
	}
	if DeploymentTarget(p).RuntimeID != "" {
		t.Fatal("new account inherited old binding")
	}
	bindDeployment(t, p, "r-two")
	if DeploymentTarget(p).Scope != "account-two/User/user-two" {
		t.Fatal("new scope not projected")
	}
}
func TestDeploymentTargetRejectsDisconnectedOrReplacedCloudBinding(t *testing.T) {
	p, f := deploymentPlatform(t)
	bindDeployment(t, p, "r-one")
	p.Connection.Disconnect()
	if DeploymentTarget(p).RuntimeID != "" {
		t.Fatal("disconnected runtime available for deployment")
	}
	bindDeployment(t, p, "r-one")
	f.account = "account-other"
	f.user = "user-other"
	p.Cloud.SignIn(context.Background(), dto.SignInRequest{Region: "cn-beijing", AccessKey: "fixture-ak", SecretKey: "fixture-sk"})
	if DeploymentTarget(p).RuntimeID != "" {
		t.Fatal("replaced cloud authority inherited runtime binding")
	}
}
func TestDeploymentTargetRequiresOfficialBoundToolID(t *testing.T) {
	p, f := deploymentPlatform(t)
	f.missingTool = true
	bindDeployment(t, p, "r-one")
	if DeploymentTarget(p).RuntimeID != "" {
		t.Fatal("deployment enabled without official ToolId")
	}
}
