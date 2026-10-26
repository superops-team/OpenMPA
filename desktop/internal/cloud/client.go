// Package cloud owns platform credentials and the fixed AgentKit/STS actions.
package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/volcengine/volc-sdk-golang/base"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const APIVersion = "2025-10-30"

type Credentials struct{ AccessKey, SecretKey, Token, ExpiresAt string }
type Client struct {
	mu         sync.RWMutex
	credential Credentials
	state      dto.PlatformState
	transport  http.RoundTripper
	endpoint   func(string, string) string
	profiles   profileStore
}

func New() *Client {
	return &Client{transport: http.DefaultTransport, endpoint: func(service, region string) string {
		if service == "sts" {
			return "https://sts.volcengineapi.com"
		}
		endpoint, _ := AgentKitEndpoint(region)
		return endpoint
	}}
}
func (c *Client) State() dto.PlatformState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s := c.state
	if s.Principal != nil {
		p := *s.Principal
		s.Principal = &p
	}
	return s
}
func (c *Client) Epoch() uint64 { return c.State().Epoch }

// Credential returns a copy of the in-memory credential for the encrypted
// vault. It is never an IPC value. The boolean is false unless signed in with
// a complete AK/SK pair.
func (c *Client) Credential() (Credentials, dto.PlatformState, bool) {
	c.mu.RLock()
	cred, state := c.credential, c.state
	c.mu.RUnlock()
	if state.Principal != nil {
		p := *state.Principal
		state.Principal = &p
	}
	if !state.SignedIn || cred.AccessKey == "" || cred.SecretKey == "" {
		return Credentials{}, state, false
	}
	return cred, state, true
}

func (c *Client) SignOut() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.credential = Credentials{}
	c.state = dto.PlatformState{Epoch: c.state.Epoch + 1}
}

var supportedRegions = []dto.RegionSummary{
	{Region: "cn-beijing", DisplayName: "华北2（北京）", EndpointHost: "agentkit.cn-beijing.volcengineapi.com"},
	{Region: "cn-shanghai", DisplayName: "华东2（上海）", EndpointHost: "agentkit.cn-shanghai.volcengineapi.com"},
}

func Regions() []dto.RegionSummary { return append([]dto.RegionSummary(nil), supportedRegions...) }
func AgentKitEndpoint(region string) (string, bool) {
	for _, item := range supportedRegions {
		if item.Region == region {
			return "https://" + item.EndpointHost, true
		}
	}
	return "", false
}
func ValidRegion(region string) bool { _, ok := AgentKitEndpoint(region); return ok }
func (c *Client) SignIn(ctx context.Context, r dto.SignInRequest) (dto.PlatformState, *dto.SafeError) {
	if !ValidRegion(r.Region) || r.AccessKey == "" || r.SecretKey == "" || len(r.AccessKey) > 2048 || len(r.SecretKey) > 2048 || len(r.SecurityToken) > 16384 {
		return dto.PlatformState{}, dto.Failure("invalidInput", "platform", "never", 0)
	}
	if r.SecurityToken != "" && r.ExpiresAt != "" {
		expiry, e := time.Parse(time.RFC3339, r.ExpiresAt)
		if e != nil || !expiry.After(time.Now()) {
			return dto.PlatformState{}, dto.Failure("authRequired", "platform", "reauthenticate", 0)
		}
	}
	cred := Credentials{r.AccessKey, r.SecretKey, r.SecurityToken, r.ExpiresAt}
	var identity struct {
		AccountID    identifier `json:"AccountId"`
		IdentityType string     `json:"IdentityType"`
		UserID       identifier `json:"UserId"`
		Trn          string     `json:"Trn"`
	}
	expected := c.Epoch()
	if e := c.call(ctx, cred, "sts", r.Region, "GetCallerIdentity", nil, &identity); e != nil {
		return dto.PlatformState{}, e
	}
	if identity.AccountID == "" {
		return dto.PlatformState{}, dto.Failure("invalidResponse", "platform", "never", 200)
	}
	principalID := string(identity.UserID)
	if principalID == "" {
		var e error
		principalID, e = c.profiles.ID(string(identity.AccountID), identity.IdentityType, r.AccessKey)
		if e != nil {
			return dto.PlatformState{}, dto.Failure("profilePersistenceFailed", "platform", "never", 0)
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Epoch != expected {
		return dto.PlatformState{}, dto.Failure("staleConnection", "platform", "reload", 0)
	}
	c.credential = cred
	c.state = dto.PlatformState{SignedIn: true, HasCredential: true, ExpiresAt: r.ExpiresAt, Region: r.Region, Epoch: expected + 1, Principal: &dto.Principal{AccountID: string(identity.AccountID), IdentityType: identity.IdentityType, CloudPrincipalID: principalID, Scope: string(identity.AccountID) + "/" + identity.IdentityType + "/" + principalID}}
	return c.state, nil
}
func (c *Client) snapshot() (Credentials, uint64, *dto.SafeError) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.state.SignedIn {
		return Credentials{}, 0, dto.Failure("authRequired", "platform", "reauthenticate", 0)
	}
	if c.credential.ExpiresAt != "" {
		t, e := time.Parse(time.RFC3339, c.credential.ExpiresAt)
		if e != nil || !t.After(time.Now()) {
			return Credentials{}, 0, dto.Failure("authRequired", "platform", "reauthenticate", 0)
		}
	}
	return c.credential, c.state.Epoch, nil
}
func (c *Client) Action(ctx context.Context, region, action string, input, output interface{}) *dto.SafeError {
	if !ValidRegion(region) {
		return dto.Failure("invalidInput", "platform", "never", 0)
	}
	switch action {
	case "ListRuntimes", "GetRuntime", "ListRuntimeVersions", "GetRuntimeVersion", "GetTool", "UpdateRuntime", "UpdateTool":
	default:
		return dto.Failure("actionUnsupported", "platform", "never", 0)
	}
	cred, epoch, e := c.snapshot()
	if e != nil {
		return e
	}
	if e = c.call(ctx, cred, "agentkit", region, action, input, output); e != nil {
		return e
	}
	if c.Epoch() != epoch {
		return dto.Failure("staleConnection", "platform", "reload", 0)
	}
	return nil
}
func (c *Client) call(ctx context.Context, cred Credentials, service, region, action string, input, output interface{}) *dto.SafeError {
	var data []byte
	if input != nil {
		var e error
		data, e = json.Marshal(input)
		if e != nil {
			return dto.Failure("invalidInput", "platform", "never", 0)
		}
	}
	version := APIVersion
	if service == "sts" {
		version = "2018-01-01"
	}
	u, _ := url.Parse(c.endpoint(service, region))
	q := u.Query()
	q.Set("Action", action)
	q.Set("Version", version)
	u.RawQuery = q.Encode()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(data))
	if e != nil {
		return dto.Failure("invalidInput", "platform", "never", 0)
	}
	req.Header.Set("Content-Type", "application/json")
	signer := base.Credentials{AccessKeyID: cred.AccessKey, SecretAccessKey: cred.SecretKey, SessionToken: cred.Token, Service: service, Region: region}
	signer.Sign(req)
	client := http.Client{Transport: c.transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(req)
	if e != nil {
		code, retry := "connectionFailure", "readOnly"
		if action == "UpdateRuntime" || action == "UpdateTool" {
			code, retry = "outcomeUnknown", "verifyOnly"
		}
		return dto.Failure(code, "platform", retry, 0)
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if e != nil {
		return dto.Failure("invalidResponse", "platform", "readOnly", resp.StatusCode)
	}
	var envelope struct {
		Metadata struct {
			RequestID string `json:"RequestId"`
			Error     *struct {
				Code string `json:"Code"`
			} `json:"Error"`
		} `json:"ResponseMetadata"`
		Result json.RawMessage `json:"Result"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return dto.Failure("invalidResponse", "platform", "readOnly", resp.StatusCode)
	}
	if envelope.Metadata.Error != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code := "platformRequestFailed"
		if envelope.Metadata.Error != nil {
			switch envelope.Metadata.Error.Code {
			case "LackPolicy", "LackProjectPolicy", "AccessDenied":
				code = "denied"
			case "InvalidSecretToken", "InvalidAccessKeyId", "SignatureDoesNotMatch", "InvalidTimestamp":
				code = "authRequired"
			case "NotFound", "ResourceNotFound":
				code = "resourceNotVisible"
			case "FlowLimitExceeded":
				code = "rateLimited"
			case "InvalidActionOrVersion":
				code = "protocolUnsupported"
			}
		}
		err := dto.Failure(code, "platform", "readOnly", resp.StatusCode)
		err.OriginConfidence = "verified"
		if len(envelope.Metadata.RequestID) < 128 && !strings.ContainsAny(envelope.Metadata.RequestID, "\r\n /?=") {
			err.RequestID = envelope.Metadata.RequestID
		}
		return err
	}
	if len(envelope.Result) == 0 || bytes.Equal(envelope.Result, []byte("null")) {
		return dto.Failure("missingResult", "platform", "never", resp.StatusCode)
	}
	if output != nil && json.Unmarshal(envelope.Result, output) != nil {
		return dto.Failure("invalidResponse", "platform", "never", resp.StatusCode)
	}
	return nil
}

// RuntimeRecord stays in Go: credentials and configuration are never IPC values.
type RuntimeRecord struct {
	RuntimeID            string `json:"RuntimeId"`
	Name                 string `json:"Name"`
	Status               string `json:"Status"`
	ProjectName          string `json:"ProjectName"`
	CurrentVersionNumber int    `json:"CurrentVersionNumber"`
	GatewayMode          string `json:"GatewayMode"`
	Endpoint             string `json:"Endpoint"`
	ArtifactURL          string `json:"ArtifactUrl"`
	ToolID               string `json:"ToolId"`
	Description          string `json:"Description"`
	CPUMilli             int    `json:"CpuMilli"`
	MemoryMB             int    `json:"MemoryMb"`
	MinInstance          int    `json:"MinInstance"`
	MaxInstance          int    `json:"MaxInstance"`
	MaxConcurrency       int    `json:"MaxConcurrency"`
	Tags                 []struct {
		Key   string `json:"Key"`
		Value string `json:"Value"`
	} `json:"Tags"`
	AuthorizerConfiguration struct {
		Type    string `json:"Type"`
		KeyAuth struct {
			APIKey         string `json:"ApiKey"`
			APIKeyName     string `json:"ApiKeyName"`
			APIKeyLocation string `json:"ApiKeyLocation"`
		} `json:"KeyAuth"`
	} `json:"AuthorizerConfiguration"`
	NetworkConfigurations []struct {
		NetworkType string `json:"NetworkType"`
		Endpoint    string `json:"Endpoint"`
	} `json:"NetworkConfigurations"`
}

func (r RuntimeRecord) Summary(region string) dto.RuntimeSummary {
	u, _ := url.Parse(r.Endpoint)
	s := dto.RuntimeSummary{RuntimeID: r.RuntimeID, Name: r.Name, Status: r.Status, Region: region, ProjectName: r.ProjectName, CurrentVersionNumber: r.CurrentVersionNumber, GatewayMode: r.GatewayMode, AuthorizerMode: r.AuthorizerConfiguration.Type, HasRuntimeKey: r.AuthorizerConfiguration.KeyAuth.APIKey != ""}
	if s.AuthorizerMode == "" && r.AuthorizerConfiguration.KeyAuth.APIKey != "" {
		s.AuthorizerMode = "keyAuth"
	}
	if u != nil {
		s.EndpointHost = u.Host
		s.EndpointPath = u.Path
	}
	for _, network := range r.NetworkConfigurations {
		if network.Endpoint == r.Endpoint {
			s.NetworkType = network.NetworkType
		}
	}
	return s
}
func (c *Client) GetRuntime(ctx context.Context, r dto.RuntimeRequest) (RuntimeRecord, *dto.SafeError) {
	var out RuntimeRecord
	e := c.Action(ctx, r.Region, "GetRuntime", struct {
		RuntimeID string `json:"RuntimeId"`
	}{r.RuntimeID}, &out)
	if e == nil && (out.RuntimeID != r.RuntimeID || (r.ProjectName != "" && out.ProjectName != r.ProjectName)) {
		e = dto.Failure("invalidResponse", "platform", "never", 200)
	}
	return out, e
}
func (c *Client) List(ctx context.Context, r dto.RuntimeListRequest) (dto.RuntimeList, *dto.SafeError) {
	out := dto.RuntimeList{Runtimes: []dto.RuntimeSummary{}}
	token := ""
	seen := map[string]bool{}
	for n := 0; n < 100; n++ {
		var page struct {
			Runtimes  []RuntimeRecord `json:"AgentKitRuntimes"`
			NextToken string          `json:"NextToken"`
		}
		input := struct {
			ProjectName string `json:"ProjectName,omitempty"`
			MaxResults  int    `json:"MaxResults"`
			NextToken   string `json:"NextToken,omitempty"`
			TagFilters  []struct {
				Key    string   `json:"Key"`
				Values []string `json:"Values"`
			} `json:"TagFilters"`
		}{ProjectName: r.ProjectName, MaxResults: 100, NextToken: token}
		input.TagFilters = append(input.TagFilters, struct {
			Key    string   `json:"Key"`
			Values []string `json:"Values"`
		}{"veadk:agent-type", []string{"mpa"}})
		if e := c.Action(ctx, r.Region, "ListRuntimes", input, &page); e != nil {
			return dto.RuntimeList{}, e
		}
		for _, item := range page.Runtimes {
			mpa := false
			for _, tag := range item.Tags {
				if tag.Key == "veadk:agent-type" && tag.Value == "mpa" {
					mpa = true
				}
			}
			if mpa && (r.ProjectName == "" || item.ProjectName == r.ProjectName) {
				out.Runtimes = append(out.Runtimes, item.Summary(r.Region))
			}
		}
		token = page.NextToken
		if token == "" {
			out.Complete = true
			return out, nil
		}
		if seen[token] {
			out.NextToken = token
			return out, nil
		}
		seen[token] = true
	}
	out.NextToken = token
	return out, nil
}
func (c *Client) Versions(ctx context.Context, r dto.RuntimeRequest) (dto.VersionList, *dto.SafeError) {
	out := dto.VersionList{Versions: []dto.RuntimeVersion{}}
	token := ""
	seen := map[string]bool{}
	for n := 0; n < 100; n++ {
		var page struct {
			Versions []struct {
				VersionNumber int    `json:"VersionNumber"`
				Status        string `json:"Status"`
			} `json:"AgentKitRuntimeVersions"`
			NextToken string `json:"NextToken"`
		}
		input := struct {
			RuntimeID  string `json:"RuntimeId"`
			MaxResults int    `json:"MaxResults"`
			NextToken  string `json:"NextToken,omitempty"`
		}{r.RuntimeID, 100, token}
		if e := c.Action(ctx, r.Region, "ListRuntimeVersions", input, &page); e != nil {
			return out, e
		}
		for _, v := range page.Versions {
			out.Versions = append(out.Versions, dto.RuntimeVersion{VersionNumber: v.VersionNumber, Status: v.Status})
		}
		token = page.NextToken
		if token == "" {
			out.Complete = true
			return out, nil
		}
		if seen[token] {
			return out, nil
		}
		seen[token] = true
	}
	return out, nil
}

func (c *Client) WithEpoch(epoch uint64, commit func() *dto.SafeError) *dto.SafeError {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.state.SignedIn || c.state.Epoch != epoch {
		return dto.Failure("staleConnection", "platform", "reload", 0)
	}
	return commit()
}

// NewWithTransport injects an HTTP transport for contract tests without changing the fixed hosts.
func NewWithTransport(transport http.RoundTripper) *Client {
	c := New()
	if transport != nil {
		c.transport = transport
	}
	return c
}
