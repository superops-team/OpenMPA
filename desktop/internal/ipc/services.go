package ipc

import (
	"context"
	"errors"
	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/configstore"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/vault"
	"strings"
	"sync"
	"time"
)

type Platform struct {
	Cloud           *cloud.Client
	Connection      *connection.Client
	Vault           *vault.Service
	Settings        *configstore.Store
	mu              sync.RWMutex
	target          dto.RuntimeRequest
	toolID          string
	boundScope      string
	boundCloudEpoch uint64
	operationMu     sync.Mutex
}

func NewPlatform(c *cloud.Client, conn *connection.Client) *Platform {
	return &Platform{Cloud: c, Connection: conn}
}

func NewPlatformWithVault(c *cloud.Client, conn *connection.Client, v *vault.Service) *Platform {
	p := NewPlatform(c, conn)
	p.Vault = v
	return p
}

func NewPlatformWithStore(c *cloud.Client, conn *connection.Client, v *vault.Service, store *configstore.Store) *Platform {
	p := NewPlatformWithVault(c, conn, v)
	p.Settings = store
	return p
}
func (p *Platform) Status() dto.PlatformStateResult {
	out := p.Cloud.State()
	return dto.PlatformStateResult{OK: true, Value: &out}
}
func (p *Platform) Regions() dto.RegionListResult {
	out := dto.RegionList{Regions: cloud.AllRegions()}
	return dto.RegionListResult{OK: true, Value: &out}
}

// VaultStatus is a local read; it never returns the passphrase or any secret.
func (p *Platform) VaultStatus() dto.VaultStatusResult {
	out := dto.VaultStatus{}
	if p.Vault == nil {
		return dto.VaultStatusResult{OK: true, Value: &out}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	status, err := p.Vault.Status(ctx)
	if err != nil {
		return dto.VaultStatusResult{Error: dto.Failure("vaultUnavailable", "desktop", "readOnly", 0)}
	}
	out.Exists = status.Exists
	out.AccountID = status.AccountID
	out.Region = status.Region
	if status.UpdatedAt > 0 {
		out.UpdatedAt = time.UnixMilli(status.UpdatedAt).UTC().Format(time.RFC3339)
	}
	return dto.VaultStatusResult{OK: true, Value: &out}
}

// SaveVault encrypts the current in-memory credential. It requires a signed-in
// session because only the cloud client can prove the account and Region.
func (p *Platform) SaveVault(ctx context.Context, r dto.VaultPassphraseRequest) dto.VaultStatusResult {
	if p.Vault == nil {
		return dto.VaultStatusResult{Error: dto.Failure("vaultUnavailable", "desktop", "never", 0)}
	}
	credential, state, ok := p.Cloud.Credential()
	if !ok || state.Principal == nil || state.Region == "" {
		return dto.VaultStatusResult{Error: dto.Failure("vaultNotSignedIn", "desktop", "never", 0)}
	}
	passphrase := []byte(r.Passphrase)
	r.Passphrase = ""
	defer clear(passphrase)
	payload := vault.Payload{Provider: state.Provider, AccessKey: credential.AccessKey, SecretKey: credential.SecretKey, SecurityToken: credential.Token, ExpiresAt: credential.ExpiresAt}
	err := p.Vault.Save(ctx, passphrase, payload, state.Principal.AccountID, state.Region)
	payload = vault.Payload{}
	if err != nil {
		if errors.Is(err, vault.ErrInvalid) {
			return dto.VaultStatusResult{Error: dto.Failure("invalidInput", "desktop", "never", 0)}
		}
		return dto.VaultStatusResult{Error: dto.Failure("vaultUnavailable", "desktop", "readOnly", 0)}
	}
	return p.VaultStatus()
}

// UnlockVault decrypts the saved credential and signs in. A wrong passphrase
// and a damaged record deliberately share one safe error.
func (p *Platform) UnlockVault(ctx context.Context, r dto.VaultPassphraseRequest) dto.PlatformStateResult {
	if p.Vault == nil {
		return dto.PlatformStateResult{Error: dto.Failure("vaultUnavailable", "desktop", "never", 0)}
	}
	p.operationMu.Lock()
	defer p.operationMu.Unlock()
	passphrase := []byte(r.Passphrase)
	r.Passphrase = ""
	defer clear(passphrase)
	payload, record, err := p.Vault.Unlock(ctx, passphrase)
	if err != nil {
		switch {
		case errors.Is(err, vault.ErrMissing):
			return dto.PlatformStateResult{Error: dto.Failure("vaultMissing", "desktop", "never", 0)}
		case errors.Is(err, vault.ErrWrongPassphrase):
			return dto.PlatformStateResult{Error: dto.Failure("vaultWrongPassphrase", "desktop", "never", 0)}
		default:
			return dto.PlatformStateResult{Error: dto.Failure("vaultUnavailable", "desktop", "readOnly", 0)}
		}
	}
	state, signInError := p.Cloud.SignIn(ctx, dto.SignInRequest{
		Provider:      payload.Provider,
		Region:        record.Region,
		AccessKey:     payload.AccessKey,
		SecretKey:     payload.SecretKey,
		SecurityToken: payload.SecurityToken,
		ExpiresAt:     payload.ExpiresAt,
	})
	payload = vault.Payload{}
	if signInError != nil {
		return dto.PlatformStateResult{Error: signInError}
	}
	if state.Principal == nil || state.Principal.AccountID != record.AccountID {
		p.Cloud.SignOut()
		p.Connection.Disconnect()
		clearTarget(p)
		return dto.PlatformStateResult{Error: dto.Failure("vaultAccountMismatch", "desktop", "never", 0)}
	}
	p.Connection.Disconnect()
	clearTarget(p)
	p.restorePreferredRuntime(ctx, state)
	return dto.PlatformStateResult{OK: true, Value: &state}
}

func (p *Platform) ForgetVault() dto.EmptyResult {
	if p.Vault == nil {
		return dto.EmptyResult{Error: dto.Failure("vaultUnavailable", "desktop", "never", 0)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := p.Vault.Forget(ctx); err != nil {
		return dto.EmptyResult{Error: dto.Failure("vaultUnavailable", "desktop", "readOnly", 0)}
	}
	return dto.EmptyResult{OK: true, Value: &dto.Empty{}}
}

func (p *Platform) SignIn(ctx context.Context, r dto.SignInRequest) dto.PlatformStateResult {
	p.operationMu.Lock()
	defer p.operationMu.Unlock()
	out, e := p.Cloud.SignIn(ctx, r)
	if e == nil {
		p.Connection.Disconnect()
		clearTarget(p)
		p.restorePreferredRuntime(ctx, out)
	}
	return dto.PlatformStateResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (p *Platform) ListRuntimes(ctx context.Context, r dto.RuntimeListRequest) dto.RuntimeListResult {
	state := p.Cloud.State()
	if state.Principal == nil {
		return dto.RuntimeListResult{Error: dto.Failure("authRequired", "platform", "reauthenticate", 0)}
	}
	if !r.ForceRefresh && p.Settings != nil {
		if cached, ok, err := p.Settings.LoadRuntimeListCache(ctx, state.Principal.Scope, r, configstore.RuntimeListCacheTTL()); err == nil && ok {
			return dto.RuntimeListResult{OK: true, Value: &cached}
		}
	}
	out, e := p.Cloud.List(ctx, r)
	if e == nil && p.Settings != nil {
		_ = p.Settings.SetDirectory(ctx, r.Region, r.ProjectName)
		if out.Complete {
			_ = p.Settings.SaveRuntimeListCache(ctx, state.Principal.Scope, r, out)
		}
	} else if e != nil && p.Settings != nil {
		if cached, ok, err := p.Settings.LoadRuntimeListCache(ctx, state.Principal.Scope, r, 7*24*time.Hour); err == nil && ok {
			return dto.RuntimeListResult{OK: true, Value: &cached}
		}
	}
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
		_, bindingError := p.Connection.Bind(connection.Binding{Endpoint: endpoint, RuntimeID: r.RuntimeID, Scope: scope, RuntimeKey: key, RuntimeName: raw.Name, WorkerToolID: raw.ToolID, RuntimeImage: raw.ArtifactURL, ChannelOwnerID: cloudState.Principal.Scope}, false)
		if bindingError == nil {
			p.mu.Lock()
			p.target = r
			p.toolID = raw.ToolID
			p.boundScope = cloudState.Principal.Scope
			p.boundCloudEpoch = cloudState.Epoch
			p.mu.Unlock()
			if p.Settings != nil {
				_ = p.Settings.SetRuntime(ctx, cloudState.Principal.Scope, r.Region, r.ProjectName, r.RuntimeID)
			}
		}
		return bindingError
	})
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
	p.Cloud.SignOut()
	p.Connection.Disconnect()
	clearTarget(p)
	return dto.EmptyResult{OK: true, Value: &dto.Empty{}}
}

func (p *Platform) restorePreferredRuntime(ctx context.Context, state dto.PlatformState) {
	if p.Settings == nil || state.Principal == nil {
		return
	}
	snapshot, err := p.Settings.Snapshot(ctx)
	if err != nil || snapshot.Runtime == "" || snapshot.PrincipalScope != state.Principal.Scope {
		return
	}
	_ = p.bindRuntime(ctx, dto.RuntimeRequest{
		Region:      snapshot.Region,
		ProjectName: snapshot.Project,
		RuntimeID:   snapshot.Runtime,
	})
}

// ResetRuntime drops the current Runtime projection without signing the cloud
// account out. Region or project changes must not retain a binding from the
// previous Runtime directory.
func (p *Platform) ResetRuntime() dto.EmptyResult {
	p.operationMu.Lock()
	defer p.operationMu.Unlock()
	p.Connection.Disconnect()
	clearTarget(p)
	if p.Settings != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = p.Settings.ClearRuntime(ctx)
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
	e := c.Client.SetBusinessAuth(r)
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
	e := c.Client.ConfigureChannelAuth(r)
	if e != nil {
		c.Client.Disconnect()
	}
	out := c.Client.State()
	return dto.ConnectionStateResult{OK: e == nil, Value: only(&out, e), Error: e}
}

// ClearDeletedRuntime invalidates only the verified target of a completed
// deletion. It cannot disconnect a newer account, region or Runtime selection.
// This is a Go-only hook; it is not part of the WebView binding.
func ClearDeletedRuntime(p *Platform, epoch uint64, region, runtimeID string) bool {
	p.operationMu.Lock()
	defer p.operationMu.Unlock()
	cleared := false
	_ = p.Cloud.WithEpoch(epoch, func() *dto.SafeError {
		p.mu.RLock()
		matches := p.boundCloudEpoch == epoch && p.target.Region == region && p.target.RuntimeID == runtimeID
		p.mu.RUnlock()
		if !matches {
			return nil
		}
		p.Connection.Disconnect()
		clearTarget(p)
		if p.Settings != nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = p.Settings.ClearRuntime(ctx)
		}
		cleared = true
		return nil
	})
	return cleared
}
