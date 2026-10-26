package ipc

import (
	"context"
	"errors"
	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/credentials"
	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"strings"
	"sync"
	"time"
)

type Platform struct {
	Cloud           *cloud.Client
	Connection      *connection.Client
	mu              sync.RWMutex
	target          dto.RuntimeRequest
	toolID          string
	boundScope      string
	boundCloudEpoch uint64
	operationMu     sync.Mutex
	persistence     *loginPersistence
}

func NewPlatform(c *cloud.Client, conn *connection.Client) *Platform {
	return &Platform{Cloud: c, Connection: conn}
}
func (p *Platform) Status() dto.PlatformStateResult {
	out := p.Cloud.State()
	if p.persistence != nil {
		out = p.persistence.decorate(out)
	}
	return dto.PlatformStateResult{OK: true, Value: &out}
}
func (p *Platform) SignIn(ctx context.Context, r dto.SignInRequest) dto.PlatformStateResult {
	p.operationMu.Lock()
	defer p.operationMu.Unlock()
	out, e := p.Cloud.SignIn(ctx, r)
	if e == nil {
		p.Connection.Disconnect()
		clearTarget(p)
		if p.persistence != nil {
			e = p.persistence.signIn(ctx, out, r)
			if e != nil {
				p.Cloud.SignOut()
				p.Connection.Disconnect()
			} else {
				out = p.persistence.decorate(out)
			}
		}
	}
	return dto.PlatformStateResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (p *Platform) ListRuntimes(ctx context.Context, r dto.RuntimeListRequest) dto.RuntimeListResult {
	out, e := p.Cloud.List(ctx, r)
	return dto.RuntimeListResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (p *Platform) Runtime(ctx context.Context, r dto.RuntimeRequest) dto.RuntimeSummaryResult {
	raw, e := p.Cloud.GetRuntime(ctx, r)
	out := raw.Summary(r.Region)
	return dto.RuntimeSummaryResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (p *Platform) BindRuntime(ctx context.Context, r dto.RuntimeRequest) dto.ConnectionStateResult {
	p.operationMu.Lock()
	defer p.operationMu.Unlock()
	return p.bindRuntime(ctx, r)
}
func (p *Platform) bindRuntime(ctx context.Context, r dto.RuntimeRequest) dto.ConnectionStateResult {
	raw, e := p.Cloud.GetRuntime(ctx, r)
	if e != nil {
		return dto.ConnectionStateResult{Error: e}
	}
	endpoint := raw.Endpoint
	if endpoint == "" {
		if len(raw.NetworkConfigurations) == 1 {
			endpoint = raw.NetworkConfigurations[0].Endpoint
		} else {
			return dto.ConnectionStateResult{Error: dto.Failure("endpointNeedsSelection", "platform", "never", 0)}
		}
	}
	mode := strings.ToLower(raw.AuthorizerConfiguration.Type)
	key := raw.AuthorizerConfiguration.KeyAuth.APIKey
	if mode != "keyauth" && mode != "key_auth" && mode != "key" && key == "" {
		return dto.ConnectionStateResult{Error: dto.Failure("authorizerUnsupported", "platform", "never", 0)}
	}
	if key == "" {
		return dto.ConnectionStateResult{Error: dto.Failure("runtimeKeyRequired", "platform", "never", 0)}
	}
	cloudState := p.Cloud.State()
	if cloudState.Principal == nil {
		return dto.ConnectionStateResult{Error: dto.Failure("staleConnection", "platform", "reload", 0)}
	}
	scope := cloudState.Principal.Scope + "/" + r.Region + "/" + r.ProjectName + "/" + r.RuntimeID
	e = p.Cloud.WithEpoch(cloudState.Epoch, func() *dto.SafeError {
		_, bindingError := p.Connection.Bind(connection.Binding{Endpoint: endpoint, RuntimeID: r.RuntimeID, Scope: scope, RuntimeKey: key}, false)
		if bindingError == nil {
			p.mu.Lock()
			p.target = r
			p.toolID = raw.ToolID
			p.boundScope = cloudState.Principal.Scope
			p.boundCloudEpoch = cloudState.Epoch
			p.mu.Unlock()
		}
		return bindingError
	})
	if e == nil {
		if p.persistence != nil {
			e = p.persistence.bind(r)
		}
	}
	out := p.Connection.State()
	return dto.ConnectionStateResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (p *Platform) Versions(ctx context.Context, r dto.RuntimeRequest) dto.VersionListResult {
	out, e := p.Cloud.Versions(ctx, r)
	return dto.VersionListResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (p *Platform) SignOut() dto.EmptyResult {
	p.operationMu.Lock()
	defer p.operationMu.Unlock()
	var e *dto.SafeError
	if p.persistence != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		e = p.persistence.signOut(ctx)
	}
	p.Cloud.SignOut()
	p.Connection.Disconnect()
	clearTarget(p)
	if e != nil {
		return dto.EmptyResult{Error: e}
	}
	return dto.EmptyResult{OK: true, Value: &dto.Empty{}}
}

func Target(p *Platform) dto.RuntimeRequest { p.mu.RLock(); defer p.mu.RUnlock(); return p.target }
func clearTarget(p *Platform) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.target = dto.RuntimeRequest{}
	p.toolID = ""
	p.boundScope = ""
	p.boundCloudEpoch = 0
}

// DeploymentTarget is a Go assembly helper, never an IPC method. It projects
// only the successfully bound Runtime and its official ToolId under the current
// cloud principal; no network request, credential access or file operation occurs.
func DeploymentTarget(p *Platform) deployment.Target {
	if p == nil || p.Cloud == nil || p.Connection == nil {
		return deployment.Target{}
	}
	p.operationMu.Lock()
	defer p.operationMu.Unlock()
	cloudState := p.Cloud.State()
	connState := p.Connection.State()
	empty := deployment.Target{ConnectionEpoch: connState.ConnectionEpoch, PlatformEpoch: cloudState.Epoch}
	if !cloudState.SignedIn || cloudState.Principal == nil || !connState.Connected {
		return empty
	}
	p.mu.RLock()
	r, tool, scope, epoch := p.target, p.toolID, p.boundScope, p.boundCloudEpoch
	p.mu.RUnlock()
	if r.RuntimeID == "" || tool == "" || r.RuntimeID != connState.RuntimeID || scope == "" || scope != cloudState.Principal.Scope || epoch != cloudState.Epoch {
		return empty
	}
	profile := cloudState.Principal.CloudPrincipalID
	if profile == "" && p.persistence != nil {
		profile = p.persistence.decorate(cloudState).ProfileID
	}
	if profile == "" {
		return empty
	}
	return deployment.Target{Scope: scope, CredentialProfileID: profile, ToolID: tool, Region: r.Region, ProjectName: r.ProjectName, RuntimeID: r.RuntimeID, ConnectionEpoch: connState.ConnectionEpoch, PlatformEpoch: cloudState.Epoch}
}
func only[T any](v *T, e *dto.SafeError) *T {
	if e != nil {
		return nil
	}
	return v
}

type Connection struct {
	Client   *connection.Client
	Platform *Platform
}

func (c *Connection) Status() dto.ConnectionStateResult {
	out := c.Client.State()
	return dto.ConnectionStateResult{OK: true, Value: &out}
}
func (c *Connection) SetBusinessIdentity(ctx context.Context, r dto.IdentityRequest) dto.ConnectionStateResult {
	if c.Platform != nil {
		c.Platform.operationMu.Lock()
		defer c.Platform.operationMu.Unlock()
	}
	operationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	e := c.Client.SetBusinessAuth(r)
	if e == nil && c.Platform != nil && c.Platform.persistence != nil {
		scope, saveError := c.Platform.persistence.saveAuth(operationCtx, Target(c.Platform), &r, nil)
		e = saveError
		if e == nil && r.AuthMode == "legacyJwt" && scope != "" {
			c.Client.SetBusinessScope(c.Client.State().ConnectionEpoch, scope)
		}
	}
	if e != nil {
		c.Client.Disconnect()
	}
	out := c.Client.State()
	return dto.ConnectionStateResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (c *Connection) Disconnect() dto.EmptyResult {
	c.Client.Disconnect()
	return dto.EmptyResult{OK: true, Value: &dto.Empty{}}
}
func (c *Connection) Test(ctx context.Context, r dto.ScopeRequest) dto.ConnectionTestResult {
	out := dto.ConnectionTest{State: c.Client.State(), Capabilities: []dto.Capability{}}
	for _, probe := range []struct{ name, path string }{{"health", "/ping"}, {"sessions", "/api/v1/sessions?include_a2a=false"}, {"agent", "/api/v1/agents"}} {
		resp, e := c.Client.Request(ctx, r.ConnectionEpoch, "GET", probe.path, nil, nil)
		if resp != nil {
			resp.Body.Close()
		}
		state := "supported"
		if e != nil {
			state = "unknown"
			if e.HTTPStatus == 401 || e.HTTPStatus == 403 {
				state = "denied"
			}
			if e.HTTPStatus == 503 {
				state = "unavailable"
			}
		}
		out.Capabilities = append(out.Capabilities, dto.Capability{Name: probe.name, State: state, Source: probe.path, Error: e})
	}
	if !c.Client.Current(r.ConnectionEpoch) {
		return dto.ConnectionTestResult{Error: dto.Failure("staleConnection", "transport", "reload", 0)}
	}
	return dto.ConnectionTestResult{OK: true, Value: &out}
}

func (c *Connection) ConfigureChannelAuth(ctx context.Context, r dto.ChannelAuthRequest) dto.ConnectionStateResult {
	if c.Platform != nil {
		c.Platform.operationMu.Lock()
		defer c.Platform.operationMu.Unlock()
	}
	operationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	e := c.Client.ConfigureChannelAuth(r)
	if e == nil && c.Platform != nil && c.Platform.persistence != nil {
		_, e = c.Platform.persistence.saveAuth(operationCtx, Target(c.Platform), nil, &r)
	}
	if e != nil {
		c.Client.Disconnect()
	}
	out := c.Client.State()
	return dto.ConnectionStateResult{OK: e == nil, Value: only(&out, e), Error: e}
}

// SavedProfiles exposes safe references only; credentials remain in the system store.
func (p *Platform) SavedProfiles() dto.SavedProfilesResult {
	if p.persistence == nil {
		return dto.SavedProfilesResult{OK: true, Value: &dto.SavedProfiles{Profiles: []dto.SavedProfile{}}}
	}
	out, e := p.persistence.list()
	return dto.SavedProfilesResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func NewPlatformWithPersistence(c *cloud.Client, conn *connection.Client, store *credentials.Store, path func() (string, error)) *Platform {
	p := NewPlatform(c, conn)
	p.persistence = newLoginPersistence(store, path)
	return p
}
func (p *Platform) Restore(ctx context.Context, r dto.RestoreRequest) dto.PlatformStateResult {
	p.operationMu.Lock()
	defer p.operationMu.Unlock()
	if p.persistence == nil {
		return dto.PlatformStateResult{Error: dto.Failure("savedProfileMissing", "transport", "never", 0)}
	}
	record, e := p.persistence.lookup(r.ProfileID)
	if e != nil {
		return dto.PlatformStateResult{Error: e}
	}
	bundle, err := p.persistence.store.Load(ctx, record.Profile)
	if err != nil {
		return dto.PlatformStateResult{Error: secretFailure(err)}
	}
	request := dto.SignInRequest{AccessKey: slotValue(bundle, credentials.CloudAK), SecretKey: slotValue(bundle, credentials.CloudSK), SecurityToken: slotValue(bundle, credentials.SecurityToken), ExpiresAt: record.ExpiresAt, Region: record.Summary.Region}
	state, e := p.Cloud.SignIn(ctx, request)
	if e != nil {
		return dto.PlatformStateResult{Error: e}
	}
	clearTarget(p)
	if state.Principal == nil || state.Principal.Scope != record.Profile.Scope {
		p.Cloud.SignOut()
		p.Connection.Disconnect()
		return dto.PlatformStateResult{Error: dto.Failure("scopeMismatch", "platform", "never", 0)}
	}
	p.Connection.Disconnect()
	p.persistence.activate(r.ProfileID)
	for _, binding := range record.Bindings {
		if binding.Runtime.RuntimeID != record.SelectedRuntime || binding.Runtime.Region != record.Summary.Region || binding.Runtime.ProjectName != record.Summary.ProjectName {
			continue
		}
		result := p.bindRuntime(ctx, binding.Runtime)
		if !result.OK {
			return dto.PlatformStateResult{Error: result.Error}
		}
		b, loadError := p.persistence.store.Load(ctx, binding.Profile)
		if loadError != nil && !errors.Is(loadError, credentials.ErrNotFound) {
			return dto.PlatformStateResult{Error: secretFailure(loadError)}
		}
		epoch := p.Connection.State().ConnectionEpoch
		if binding.BusinessMode != "" {
			e = p.Connection.SetBusinessAuth(dto.IdentityRequest{ConnectionEpoch: epoch, AuthMode: binding.BusinessMode, LegacyJWT: slotValue(b, credentials.ApplicationJWT), TestSubject: binding.TestSubject, VerifiedTestDeployment: binding.VerifiedTest})
			if e != nil {
				return dto.PlatformStateResult{Error: e}
			}
			if binding.BusinessMode == "legacyJwt" {
				p.Connection.SetBusinessScope(p.Connection.State().ConnectionEpoch, binding.BusinessScope)
			}
		}
		epoch = p.Connection.State().ConnectionEpoch
		if binding.ChannelMode != "" {
			e = p.Connection.ConfigureChannelAuth(dto.ChannelAuthRequest{ConnectionEpoch: epoch, AuthMode: binding.ChannelMode, ChannelKey: slotValue(b, credentials.ChannelManagement), LegacyJWT: slotValue(b, credentials.ChannelManagementJWT)})
			if e != nil {
				return dto.PlatformStateResult{Error: e}
			}
		}
		break
	}
	state = p.persistence.decorate(p.Cloud.State())
	return dto.PlatformStateResult{OK: true, Value: &state}
}
