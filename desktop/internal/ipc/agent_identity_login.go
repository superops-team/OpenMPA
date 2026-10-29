package ipc

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/configstore"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/vault"
	"github.com/volcengine/volc-sdk-golang/base"
)

func (p *Platform) LoginAgentIdentity(ctx context.Context, r dto.AgentIdentityLoginRequest) dto.UserTokenVaultStatusResult {
	if p.Vault == nil {
		return dto.UserTokenVaultStatusResult{Error: dto.Failure("vaultUnavailable", "desktop", "never", 0)}
	}
	if p.Connection == nil || p.Cloud == nil || !validUserTokenVaultRuntimeID(r.RuntimeID) || r.ConnectionEpoch == 0 {
		return dto.UserTokenVaultStatusResult{Error: dto.Failure("invalidInput", "desktop", "never", 0)}
	}
	passphrase := []byte(r.Passphrase)
	r.Passphrase = ""
	defer clear(passphrase)
	userPoolID, failure := p.resolveAgentIdentityUserPool(ctx, r.UserPoolID)
	if failure != nil {
		return dto.UserTokenVaultStatusResult{Error: failure}
	}
	state := p.Connection.State()
	if !state.Connected || state.ConnectionEpoch != r.ConnectionEpoch || state.RuntimeID != r.RuntimeID {
		return dto.UserTokenVaultStatusResult{Error: dto.Failure("staleConnection", "transport", "reload", 0)}
	}
	cred, platform, ok := p.Cloud.Credential()
	if !ok {
		return dto.UserTokenVaultStatusResult{Error: dto.Failure("authRequired", "platform", "reauthenticate", 0)}
	}
	resolved, err := p.resolveOIDC(ctx, cred, firstNonEmpty(platform.Region, configstore.DefaultRegion), userPoolID)
	if err != nil {
		return dto.UserTokenVaultStatusResult{Error: dto.Failure("authRequired", "platform", "reauthenticate", 0)}
	}
	code, verifier, err := p.authorizeOIDC(ctx, resolved)
	if err != nil {
		return dto.UserTokenVaultStatusResult{Error: dto.Failure("authRequired", "desktop", "reauthenticate", 0)}
	}
	tokens, err := p.exchangeOIDCCode(ctx, resolved, code, verifier)
	if err != nil || tokens.IDToken == "" || tokens.RefreshToken == "" {
		return dto.UserTokenVaultStatusResult{Error: dto.Failure("authRequired", "desktop", "reauthenticate", 0)}
	}
	defer func() {
		tokens.IDToken = ""
		tokens.RefreshToken = ""
	}()
	body, _ := json.Marshal(struct {
		IDToken      string `json:"idToken"`
		RefreshToken string `json:"refreshToken"`
	}{IDToken: tokens.IDToken, RefreshToken: tokens.RefreshToken})
	resp, prewarmFailure := p.Connection.RequestRuntime(ctx, r.ConnectionEpoch, http.MethodPost, "/identity/sessions/put", body)
	if resp != nil {
		defer resp.Body.Close()
	}
	if prewarmFailure != nil {
		return dto.UserTokenVaultStatusResult{Error: prewarmFailure}
	}
	var prewarm dto.IdentityPrewarm
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&prewarm) != nil || prewarm.SenderID == "" {
		return dto.UserTokenVaultStatusResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", resp.StatusCode)}
	}
	expiresAt := time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	if tokens.ExpiresIn <= 0 {
		expiresAt = time.Now().Add(10 * time.Hour).UTC().Format(time.RFC3339)
	}
	token := vault.UserToken{
		RuntimeID:    r.RuntimeID,
		UserPoolID:   userPoolID,
		Issuer:       resolved.Issuer,
		ClientID:     resolved.ClientID,
		Subject:      prewarm.SenderID,
		MPAID:        r.RuntimeID,
		SpaceID:      userPoolID,
		IDToken:      tokens.IDToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    expiresAt,
	}
	defer func() {
		token.IDToken = ""
		token.RefreshToken = ""
	}()
	if err := p.Vault.SaveUserToken(ctx, passphrase, token); err != nil {
		return dto.UserTokenVaultStatusResult{Error: dto.Failure("invalidInput", "desktop", "never", 0)}
	}
	if err := p.Connection.SetBusinessAuth(dto.IdentityRequest{ConnectionEpoch: r.ConnectionEpoch, AuthMode: "legacyJwt", LegacyJWT: tokens.IDToken}); err != nil {
		return dto.UserTokenVaultStatusResult{Error: err}
	}
	return p.UserTokenVaultStatus(dto.UserTokenVaultRequest{RuntimeID: r.RuntimeID})
}

type oidcResolved struct {
	UserPoolID            string
	Issuer                string
	AuthorizationEndpoint string
	TokenEndpoint         string
	ClientID              string
	ClientSecret          string
	RedirectURI           string
}

type oidcTokens struct {
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

func (p *Platform) resolveOIDC(ctx context.Context, cred cloud.Credentials, region, userPoolID string) (oidcResolved, error) {
	poolResult, err := p.identityAction(ctx, cred, region, "GetUserPool", map[string]string{"UserPoolUid": userPoolID})
	if err != nil {
		return oidcResolved{}, err
	}
	pool := firstMap(poolResult, "UserPool", "Data")
	if len(pool) == 0 {
		pool = poolResult
	}
	issuer := firstNonEmpty(
		stringValue(pool, "IssuerUrl"),
		stringValue(pool, "IssuerURL"),
		stringValue(pool, "Issuer"),
		stringValue(poolResult, "IssuerUrl"),
		stringValue(poolResult, "IssuerURL"),
		stringValue(poolResult, "Issuer"),
	)
	discoveryURL := firstNonEmpty(
		stringValue(pool, "DiscoveryUrl"),
		stringValue(pool, "DiscoveryURL"),
		stringValue(pool, "OIDCDiscoveryUrl"),
		stringValue(poolResult, "DiscoveryUrl"),
		stringValue(poolResult, "DiscoveryURL"),
		stringValue(poolResult, "OIDCDiscoveryUrl"),
	)
	if discoveryURL == "" && issuer != "" {
		discoveryURL = strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	}
	discovery, err := p.readOIDCDiscovery(ctx, discoveryURL)
	if err != nil {
		return oidcResolved{}, err
	}
	list, err := p.identityAction(ctx, cred, region, "ListUserPoolClients", map[string]any{"UserPoolUid": userPoolID, "PageNumber": 1, "PageSize": 100})
	if err != nil {
		return oidcResolved{}, err
	}
	for _, item := range listItems(list, "Data", "UserPoolClients", "Clients", "Items") {
		uid := firstNonEmpty(
			stringValue(item, "Uid"),
			stringValue(item, "ClientUid"),
			stringValue(item, "ClientID"),
			stringValue(item, "ClientId"),
		)
		if uid == "" {
			continue
		}
		detail := item
		if candidate, err := p.identityAction(ctx, cred, region, "GetUserPoolClient", map[string]string{"UserPoolUid": userPoolID, "ClientUid": uid}); err == nil {
			if nested := firstMap(candidate, "UserPoolClient", "Client", "Data"); len(nested) > 0 {
				detail = nested
			} else {
				detail = candidate
			}
		}
		callback := chooseLocalCallback(firstStringSlice(detail,
			"AllowedCallbackUrls",
			"AllowedCallbackURLs",
			"CallbackUrls",
			"CallbackURLs",
			"RedirectUris",
			"RedirectURIs",
		), p.identityCallbackAddr)
		if callback == "" {
			continue
		}
		resolved := oidcResolved{
			UserPoolID: userPoolID,
			Issuer: firstNonEmpty(
				stringValue(discovery, "issuer"),
				issuer,
			),
			AuthorizationEndpoint: stringValue(discovery, "authorization_endpoint"),
			TokenEndpoint: firstNonEmpty(
				stringValue(discovery, "token_endpoint"),
				stringValue(pool, "TokenUrl"),
				stringValue(pool, "TokenURL"),
				stringValue(poolResult, "TokenUrl"),
				stringValue(poolResult, "TokenURL"),
			),
			ClientID: firstNonEmpty(
				stringValue(detail, "ClientID"),
				stringValue(detail, "ClientId"),
				stringValue(detail, "ClientUid"),
				uid,
			),
			ClientSecret: firstNonEmpty(
				stringValue(detail, "ClientSecret"),
				stringValue(detail, "Secret"),
			),
			RedirectURI: callback,
		}
		if resolved.Issuer != "" && resolved.AuthorizationEndpoint != "" && resolved.TokenEndpoint != "" && resolved.ClientID != "" {
			return resolved, nil
		}
	}
	return oidcResolved{}, errNoOIDCClient{}
}

type errNoOIDCClient struct{}

func (errNoOIDCClient) Error() string { return "no local OIDC client" }

func (p *Platform) identityAction(ctx context.Context, cred cloud.Credentials, region, action string, input any) (map[string]any, error) {
	payload, _ := json.Marshal(input)
	endpoint := firstNonEmpty(p.identityOpenAPIEndpoint, "https://open.volcengineapi.com")
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("Action", action)
	q.Set("Version", "2025-10-30")
	u.RawQuery = q.Encode()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	signer := base.Credentials{AccessKeyID: cred.AccessKey, SecretAccessKey: cred.SecretKey, SessionToken: cred.Token, Service: "id", Region: region}
	signer.Sign(req)
	resp, err := p.identityClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errNoOIDCClient{}
	}
	var envelope struct {
		ResponseMetadata struct {
			Error *struct {
				Code string `json:"Code"`
			} `json:"Error"`
		} `json:"ResponseMetadata"`
		Result json.RawMessage `json:"Result"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.ResponseMetadata.Error != nil {
		return nil, errNoOIDCClient{}
	}
	if json.Unmarshal(raw, &envelope) == nil && len(envelope.Result) > 0 {
		var result map[string]any
		if json.Unmarshal(envelope.Result, &result) == nil {
			return result, nil
		}
	}
	var result map[string]any
	if json.Unmarshal(raw, &result) != nil {
		return nil, errNoOIDCClient{}
	}
	return result, nil
}

func (p *Platform) authorizeOIDC(ctx context.Context, resolved oidcResolved) (string, string, error) {
	redirect, err := url.Parse(resolved.RedirectURI)
	if err != nil || redirect.Scheme != "http" || redirect.Host == "" {
		return "", "", errNoOIDCClient{}
	}
	callbackPath := redirect.EscapedPath()
	if callbackPath == "" {
		callbackPath = "/oauth/callback"
	}
	listener, err := net.Listen("tcp", firstNonEmpty(p.identityCallbackAddr, redirect.Host))
	if err != nil {
		return "", "", err
	}
	defer listener.Close()
	state, err := randomURLToken()
	if err != nil {
		return "", "", err
	}
	nonce, err := randomURLToken()
	if err != nil {
		return "", "", err
	}
	verifier, err := randomURLToken()
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	type callbackResult struct {
		code string
		err  error
	}
	done := make(chan callbackResult, 1)
	send := func(result callbackResult) {
		select {
		case done <- result:
		default:
		}
	}
	server := http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != callbackPath || r.URL.Query().Get("state") != state || r.URL.Query().Get("code") == "" {
			http.Error(w, "Invalid OpenMPA login callback", http.StatusBadRequest)
			send(callbackResult{err: errNoOIDCClient{}})
			return
		}
		_, _ = io.WriteString(w, "OpenMPA login complete. You can close this window.")
		send(callbackResult{code: r.URL.Query().Get("code")})
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Shutdown(context.Background())
	authURL, err := url.Parse(resolved.AuthorizationEndpoint)
	if err != nil {
		return "", "", err
	}
	values := authURL.Query()
	values.Set("response_type", "code")
	values.Set("client_id", resolved.ClientID)
	values.Set("redirect_uri", resolved.RedirectURI)
	values.Set("scope", "openid profile offline_access")
	values.Set("state", state)
	values.Set("nonce", nonce)
	values.Set("code_challenge", challenge)
	values.Set("code_challenge_method", "S256")
	authURL.RawQuery = values.Encode()
	if p.openExternal == nil || p.openExternal(authURL.String()) != nil {
		return "", "", errNoOIDCClient{}
	}
	select {
	case result := <-done:
		return result.code, verifier, result.err
	case <-ctx.Done():
		return "", "", ctx.Err()
	case <-time.After(5 * time.Minute):
		return "", "", errNoOIDCClient{}
	}
}

func (p *Platform) readOIDCDiscovery(ctx context.Context, discoveryURL string) (map[string]any, error) {
	if discoveryURL == "" {
		return nil, errNoOIDCClient{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.identityClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil || resp.StatusCode != http.StatusOK {
		return nil, errNoOIDCClient{}
	}
	return out, nil
}

func (p *Platform) exchangeOIDCCode(ctx context.Context, resolved oidcResolved, code, verifier string) (oidcTokens, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", resolved.RedirectURI)
	form.Set("code_verifier", verifier)
	form.Set("client_id", resolved.ClientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resolved.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return oidcTokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if resolved.ClientSecret != "" {
		req.SetBasicAuth(resolved.ClientID, resolved.ClientSecret)
	}
	resp, err := p.identityClient().Do(req)
	if err != nil {
		return oidcTokens{}, err
	}
	defer resp.Body.Close()
	var out oidcTokens
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil || resp.StatusCode != http.StatusOK {
		return oidcTokens{}, errNoOIDCClient{}
	}
	return out, nil
}

func (p *Platform) identityClient() *http.Client {
	if p.identityHTTPClient != nil {
		return p.identityHTTPClient
	}
	return &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func randomURLToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func chooseLocalCallback(callbacks []string, callbackAddr string) string {
	want := "http://" + firstNonEmpty(callbackAddr, "127.0.0.1:8766") + "/oauth/callback"
	for _, item := range callbacks {
		if item == want {
			return item
		}
	}
	return ""
}

func stringValue(m map[string]any, key string) string {
	if value, ok := m[key].(string); ok {
		return value
	}
	return ""
}

func firstStringSlice(m map[string]any, keys ...string) []string {
	for _, key := range keys {
		values := stringSlice(m, key)
		if len(values) > 0 {
			return values
		}
	}
	return nil
}

func stringSlice(m map[string]any, key string) []string {
	switch raw := m[key].(type) {
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if value, ok := item.(string); ok {
				out = append(out, value)
			}
		}
		return out
	case []string:
		return append([]string(nil), raw...)
	default:
		return nil
	}
}

func firstMap(m map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value, ok := m[key].(map[string]any); ok {
			return value
		}
	}
	return nil
}

func listItems(m map[string]any, keys ...string) []map[string]any {
	for _, key := range keys {
		if nested, ok := m[key].(map[string]any); ok {
			if out := listItems(nested, "Data", "List", "Items", "UserPoolClients", "Clients"); len(out) > 0 {
				return out
			}
		}
		raw, _ := m[key].([]any)
		if len(raw) == 0 {
			continue
		}
		out := make([]map[string]any, 0, len(raw))
		for _, item := range raw {
			if value, ok := item.(map[string]any); ok {
				out = append(out, value)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
