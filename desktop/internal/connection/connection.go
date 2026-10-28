package connection

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Binding struct {
	Endpoint, RuntimeID, Scope, RuntimeKey  string
	RuntimeName, WorkerToolID, RuntimeImage string
}
type Client struct {
	mu                                                       sync.RWMutex
	binding                                                  Binding
	epoch                                                    uint64
	userID, legacyJWT, channelKey, channelJWT, businessScope string
	verifiedTest                                             bool
	transport                                                http.RoundTripper
	epochCtx                                                 context.Context
	cancel                                                   context.CancelFunc
}

func New() *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{transport: http.DefaultTransport, epochCtx: ctx, cancel: cancel}
}
func (c *Client) advanceEpochLocked() {
	if c.cancel != nil {
		c.cancel()
	}
	c.epochCtx, c.cancel = context.WithCancel(context.Background())
	c.epoch++
}
func (c *Client) Bind(b Binding, local bool) (uint64, *dto.SafeError) {
	u, e := url.Parse(b.Endpoint)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(local && u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
		return 0, dto.Failure("invalidEndpoint", "platform", "never", 0)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.advanceEpochLocked()
	c.binding = b
	c.userID = ""
	c.legacyJWT = ""
	c.channelKey = ""
	c.channelJWT = ""
	c.businessScope = ""
	c.verifiedTest = false
	return c.epoch, nil
}
func (c *Client) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.advanceEpochLocked()
	c.binding = Binding{}
	c.userID = ""
	c.legacyJWT = ""
	c.channelKey = ""
	c.channelJWT = ""
	c.businessScope = ""
	c.verifiedTest = false
}
func (c *Client) SetIdentity(epoch uint64, id string) *dto.SafeError {
	c.mu.Lock()
	defer c.mu.Unlock()
	if epoch != c.epoch {
		return dto.Failure("staleConnection", "transport", "reload", 0)
	}
	if len(id) > 256 || strings.ContainsAny(id, "\r\n") {
		return dto.Failure("invalidInput", "mpa", "never", 0)
	}
	c.advanceEpochLocked()
	c.userID = id
	return nil
}
func (c *Client) Current(epoch uint64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.epoch == epoch && c.binding.Endpoint != ""
}
func (c *Client) WithCurrent(epoch uint64, commit func() *dto.SafeError) *dto.SafeError {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.epoch != epoch || c.binding.Endpoint == "" {
		return dto.Failure("staleConnection", "transport", "reload", 0)
	}
	return commit()
}
func (c *Client) State() dto.ConnectionState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	u, _ := url.Parse(c.binding.Endpoint)
	out := dto.ConnectionState{Connected: c.binding.Endpoint != "", ConnectionEpoch: c.epoch, Scope: c.binding.Scope + c.businessScope, RuntimeID: c.binding.RuntimeID, RuntimeName: c.binding.RuntimeName, WorkerToolID: c.binding.WorkerToolID, RuntimeImage: c.binding.RuntimeImage, HasCredential: c.binding.RuntimeKey != "", HasBusinessIdentity: c.userID != "" || c.legacyJWT != ""}
	if u != nil {
		out.EndpointHost = u.Host
		out.EndpointPath = u.Path
	}
	return out
}
func (c *Client) Request(ctx context.Context, epoch uint64, method, path string, body []byte, headers http.Header) (*http.Response, *dto.SafeError) {
	c.mu.RLock()
	b, id, jwt, current, epochCtx := c.binding, c.userID, c.legacyJWT, c.epoch, c.epochCtx
	c.mu.RUnlock()
	if epoch != current {
		return nil, dto.Failure("staleConnection", "transport", "reload", 0)
	}
	if b.Endpoint == "" {
		return nil, dto.Failure("notConnected", "transport", "never", 0)
	}
	parsed, e := url.ParseRequestURI(path)
	if e != nil || parsed.IsAbs() || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\r\n") || hasTraversal(parsed.Path) {
		return nil, dto.Failure("invalidInput", "transport", "never", 0)
	}
	u, e := url.Parse(strings.TrimRight(b.Endpoint, "/") + path)
	if e != nil {
		return nil, dto.Failure("invalidInput", "transport", "never", 0)
	}
	requestCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(epochCtx, cancel)
	transferred := false
	defer func() {
		if !transferred {
			stop()
			cancel()
		}
	}()
	req, e := http.NewRequestWithContext(requestCtx, method, u.String(), bytes.NewReader(body))
	if e != nil {
		return nil, dto.Failure("invalidInput", "transport", "never", 0)
	}
	// The standard transport treats Idempotency-Key plus GetBody as replayable.
	// Business writes recover explicitly; a lost response must stay unknown.
	if method != "GET" {
		req.GetBody = nil
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if b.RuntimeKey != "" {
		req.Header.Set("Authorization", "Bearer "+b.RuntimeKey)
	}
	if id != "" {
		req.Header.Set("X-User-Id", id)
	}
	if jwt != "" {
		req.Header.Set("X-Jwt-Token", jwt)
	}
	for _, k := range []string{"Idempotency-Key", "If-Match", "X-MPA-Channel-Key", "X-Jwt-Token", "Content-Type"} {
		if v := headers.Get(k); v != "" {
			req.Header.Set(k, v)
		}
	}
	client := http.Client{Transport: c.transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(req)
	if e != nil {
		if !c.Current(epoch) {
			return nil, dto.Failure("staleConnection", "transport", "reload", 0)
		}
		code, retry := "connectionFailure", "readOnly"
		if method != "GET" {
			code, retry = "outcomeUnknown", "verifyOnly"
		}
		return nil, dto.Failure(code, "transport", retry, 0)
	}
	if !c.Current(epoch) {
		resp.Body.Close()
		return nil, dto.Failure("staleConnection", "transport", "reload", 0)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return nil, ErrorFromResponse(resp.StatusCode, raw)
	}
	resp.Body = &epochBody{ReadCloser: resp.Body, stop: stop, cancel: cancel}
	transferred = true
	return resp, nil
}

type epochBody struct {
	io.ReadCloser
	stop   func() bool
	cancel context.CancelFunc
	once   sync.Once
}

func (b *epochBody) Close() error {
	b.once.Do(func() { b.stop(); b.cancel() })
	return b.ReadCloser.Close()
}
func hasTraversal(path string) bool {
	for _, part := range strings.Split(path, "/") {
		if part == ".." || part == "." {
			return true
		}
	}
	return false
}
func ErrorFromResponse(status int, data []byte) *dto.SafeError {
	code, retry := "requestFailed", "never"
	switch status {
	case 401:
		code = "authRequired"
	case 403:
		code = "denied"
	case 404:
		code = "resourceNotVisible"
	case 409, 412, 428:
		code, retry = "conflict", "reload"
	case 410:
		code, retry = "cursorExpired", "history"
	case 400, 422:
		code = "invalidInput"
	case 503:
		code, retry = "unavailable", "readOnly"
	}
	out := dto.Failure(code, "runtimeIngress", retry, status)
	var wire struct {
		Code         string          `json:"code"`
		RequestID    string          `json:"requestId"`
		CurrentState json.RawMessage `json:"currentState"`
		Detail       json.RawMessage `json:"detail"`
		Details      struct {
			CurrentState json.RawMessage `json:"currentState"`
		} `json:"details"`
	}
	if json.Unmarshal(data, &wire) != nil {
		return out
	}
	if knownCode(wire.Code) {
		out.Code = wire.Code
		out.Message = dto.SafeMessage(wire.Code)
	}
	out.CurrentState = decodeCurrentState(wire.CurrentState)
	if out.CurrentState == nil {
		out.CurrentState = decodeCurrentState(wire.Details.CurrentState)
	}
	if len(wire.RequestID) < 128 && !strings.ContainsAny(wire.RequestID, "\r\n /?=") {
		out.RequestID = wire.RequestID
	}
	var detailCode string
	if json.Unmarshal(wire.Detail, &detailCode) == nil && knownCode(detailCode) {
		out.Code = detailCode
		out.Message = dto.SafeMessage(detailCode)
	}
	var detail struct {
		Code         string          `json:"code"`
		CurrentState json.RawMessage `json:"currentState"`
	}
	if json.Unmarshal(wire.Detail, &detail) == nil {
		if knownCode(detail.Code) {
			out.Code = detail.Code
			out.Message = dto.SafeMessage(detail.Code)
		}
		if state := decodeCurrentState(detail.CurrentState); state != nil {
			out.CurrentState = state
		}
	}
	return out
}
func knownCode(s string) bool {
	switch s {
	case "voiceHistoryInvalidInput", "voiceHistoryCursorInvalid", "voiceHistoryCursorExpired", "voiceHistoryKeyUnavailable", "voiceHistorySchemaUnavailable":
		return true
	case "human_frame_stale", "human_frame_not_found", "human_frame_expired", "human_file_exists", "human_file_digest_conflict", "human_file_not_found", "human_version_conflict", "human_lease_conflict", "human_lease_expired", "human_browser_generation_conflict", "human_audit_cursor_expired", "human_artifact_digest_mismatch", "human_cancel_pending", "human_provider_outcome_unknown", "human_preview_busy", "human_preview_invalid", "human_preview_too_large", "human_output_not_ready", "human_provider_unavailable", "human_worker_binding_unavailable", "human_execution_fenced", "human_artifact_not_found", "human_recovery_digest_mismatch", "challenge_conflict", "challenge_expired", "attachment_not_ready", "attachment_digest_mismatch":
		return true
	case "voiceASRKeyMissing", "voiceASRResourceMissing", "voiceTTSKeyMissing", "voiceTTSSpeakerMissing", "voiceAdmissionUnavailable", "voiceAuthorizationUnavailable", "voiceBackpressure", "voiceConfigurationChanged", "voiceFinalIncomplete", "voiceFrameTooLarge", "voicePlaybackAckInvalid", "voicePrincipalExpired", "voiceProtocolInvalid", "voiceProviderUnavailable", "voiceScopeDenied", "voiceSequenceConflict", "voiceSequenceGap", "voiceServiceUnavailable", "voiceSessionChanged", "voiceSetupRequired", "voiceFinalConflict", "voiceAuthUnsupported", "voicePaidRequestsDisabled", "voiceSecretReferenceInvalid", "voiceInvalidInput", "voiceResourceUnsupported", "voiceCallNotFound", "voiceDurableStoreRequired", "voiceIdempotencyConflict", "voiceInputBusy", "voiceLeaseExpired", "voiceSchemaUnsupported", "voiceUtteranceNotFound", "staleVoiceGeneration":
		return true
	case "memory_not_found", "session_not_found", "memory_readonly", "idempotency_conflict", "revision_conflict", "provider_revision_conflict", "memory_operation_pending", "memory_index_pending", "memory_delete_unconfirmed", "memory_provider_unavailable", "memory_freshness_degraded", "ScheduledTaskNotFound", "ScheduledTaskVersionConflict", "IdempotencyConflict", "ScheduledTaskQuotaExceeded", "SchedulerUnavailable", "InvalidSchedule", "InvalidDeliveryTarget", "session_busy", "idempotency_mismatch", "execution_config_changed", "cursor_expired", "invalidInput", "conflict", "denied", "authRequired", "IDEMPOTENCY_CONFLICT", "NOT_FOUND", "SESSION_DELETED", "KNOWLEDGE_UNAVAILABLE", "FILE_REJECTED", "KNOWLEDGE_INGEST_FAILED", "SOURCE_MISSING":
		return true
	}
	return false
}
func (c *Client) JSON(ctx context.Context, epoch uint64, method, path string, input, output interface{}) *dto.SafeError {
	return c.Do(ctx, epoch, method, path, input, output, "session")
}
func (c *Client) Do(ctx context.Context, epoch uint64, method, path string, input, output interface{}, family string) *dto.SafeError {
	if family != "session" && family != "schedule" && family != "channel" {
		return dto.Failure("requestFamilyUnsupported", "transport", "never", 0)
	}
	var body []byte
	if input != nil {
		var e error
		body, e = json.Marshal(input)
		if e != nil {
			return dto.Failure("invalidInput", "transport", "never", 0)
		}
	}
	headers := http.Header{}
	if family == "channel" {
		c.mu.RLock()
		key, jwt := c.channelKey, c.channelJWT
		c.mu.RUnlock()
		if key == "" && jwt == "" {
			return dto.Failure("authRequired", "mpa", "never", 0)
		}
		if key != "" {
			headers.Set("X-MPA-Channel-Key", key)
		}
		if jwt != "" {
			headers.Set("X-Jwt-Token", jwt)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, e := c.Request(ctx, epoch, method, path, body, headers)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 32<<20))
	if output != nil && (decoder.Decode(output) != nil || decoder.Decode(new(interface{})) != io.EOF) {
		return dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", resp.StatusCode)
	}
	if !c.Current(epoch) {
		return dto.Failure("staleConnection", "transport", "reload", 0)
	}
	return nil
}
func (c *Client) SetBusinessAuth(r dto.IdentityRequest) *dto.SafeError {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.epoch != r.ConnectionEpoch {
		return dto.Failure("staleConnection", "transport", "reload", 0)
	}
	if len(r.LegacyJWT) > 16384 || len(r.TestSubject) > 256 || strings.ContainsAny(r.LegacyJWT+r.TestSubject, "\r\n") {
		return dto.Failure("invalidInput", "mpa", "never", 0)
	}
	switch r.AuthMode {
	case "none":
		c.userID = ""
		c.legacyJWT = ""
		c.businessScope = ""
		c.verifiedTest = false
	case "legacyJwt":
		if r.LegacyJWT == "" {
			return dto.Failure("invalidInput", "mpa", "never", 0)
		}
		c.userID = ""
		c.legacyJWT = r.LegacyJWT
		c.businessScope = "/business/profile-" + newScopeID()
		c.verifiedTest = false
	case "explicitTest":
		if !r.VerifiedTestDeployment || r.TestSubject == "" {
			return dto.Failure("invalidInput", "mpa", "never", 0)
		}
		c.userID = r.TestSubject
		c.legacyJWT = ""
		c.businessScope = "/business/test/" + r.TestSubject
		c.verifiedTest = true
	default:
		return dto.Failure("authModeUnsupported", "mpa", "never", 0)
	}
	c.advanceEpochLocked()
	return nil
}
func (c *Client) ConfigureChannelAuth(r dto.ChannelAuthRequest) *dto.SafeError {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.epoch != r.ConnectionEpoch {
		return dto.Failure("staleConnection", "transport", "reload", 0)
	}
	if len(r.ChannelKey+r.LegacyJWT) > 16384 || strings.ContainsAny(r.ChannelKey+r.LegacyJWT, "\r\n") {
		return dto.Failure("invalidInput", "mpa", "never", 0)
	}
	switch r.AuthMode {
	case "none":
		c.channelKey = ""
		c.channelJWT = ""
	case "runtimeKey":
		if r.ChannelKey == "" {
			return dto.Failure("invalidInput", "mpa", "never", 0)
		}
		c.channelKey = r.ChannelKey
		c.channelJWT = ""
	case "legacyJwt":
		if r.LegacyJWT == "" {
			return dto.Failure("invalidInput", "mpa", "never", 0)
		}
		c.channelJWT = r.LegacyJWT
		c.channelKey = ""
	default:
		return dto.Failure("authModeUnsupported", "mpa", "never", 0)
	}
	c.advanceEpochLocked()
	return nil
}
func (c *Client) HasChannelOwnerIdentity() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.channelJWT != "" || (c.verifiedTest && c.userID != "" && c.channelKey != "")
}
func (c *Client) SetBusinessScope(epoch uint64, scope string) *dto.SafeError {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.epoch != epoch {
		return dto.Failure("staleConnection", "transport", "reload", 0)
	}
	if scope == "" || len(scope) > 512 || !strings.HasPrefix(scope, "/business/") {
		return dto.Failure("invalidInput", "transport", "never", 0)
	}
	c.businessScope = scope
	return nil
}
func newScopeID() string { b := make([]byte, 16); rand.Read(b); return hex.EncodeToString(b) }

func (c *Client) ScopeContext(epoch uint64) (context.Context, *dto.SafeError) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if epoch != c.epoch || c.binding.Endpoint == "" {
		return nil, dto.Failure("staleConnection", "transport", "reload", 0)
	}
	return c.epochCtx, nil
}

func decodeCurrentState(raw json.RawMessage) *dto.CurrentState {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var state dto.CurrentState
	if json.Unmarshal(raw, &state) != nil {
		return nil
	}
	return &state
}
