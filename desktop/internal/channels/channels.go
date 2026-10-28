package channels

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

const base = "/api/v1/channels"

type channelProvider interface {
	Do(context.Context, uint64, string, string, interface{}, interface{}, string) *dto.SafeError
	HasChannelOwnerIdentity() bool
}
type operation struct {
	Epoch                                   uint64
	Channel, BindingID, Domain, BindingType string
	Uncertain                               bool
	InFlight                                bool
}
type slotKey struct {
	Epoch uint64
	AppID string
}
type Channels struct {
	provider   channelProvider
	mu         sync.Mutex
	operations map[string]operation
	slots      map[slotKey]string
}

func New(conn *connection.Client) *Channels {
	return &Channels{provider: conn, operations: map[string]operation{}, slots: map[slotKey]string{}}
}

func invalid() *dto.SafeError     { return dto.Failure("invalidInput", "mpa", "never", 0) }
func unsupported() *dto.SafeError { return dto.Failure("channelActionUnsupported", "mpa", "never", 0) }
func ownerRequired() *dto.SafeError {
	e := dto.Failure("ownerIdentityRequired", "mpa", "never", 0)
	e.Message = "A verified channel owner identity is required for this action."
	return e
}
func (s *Channels) owner() *dto.SafeError {
	if !s.provider.HasChannelOwnerIdentity() {
		return ownerRequired()
	}
	return nil
}
func validID(v string, max int) bool {
	if v == "" || utf8.RuneCountInString(v) > max || strings.Contains(v, "..") || strings.TrimSpace(v) != v {
		return false
	}
	for _, r := range v {
		if r < 33 || r == 127 || strings.ContainsRune("/\\?#%", r) {
			return false
		}
	}
	return true
}
func validChannel(v string) bool         { return v == "feishu" || v == "dingtalk" || v == "wecom" }
func accountValid(r AccountRequest) bool { return validChannel(r.Channel) && validID(r.AppID, 256) }
func scopeValid(v string) bool {
	return v == "" || v == "group" || v == "group_sender" || v == "group_topic" || v == "group_topic_sender"
}
func accountPath(appID string) string { return base + "/feishu/accounts/" + url.PathEscape(appID) }

// SafeError is re-projected here as channel errors must never carry a session's
// arbitrary currentState, raw provider detail, or supplied message.
func safeError(e *dto.SafeError, mutating bool) *dto.SafeError {
	if e == nil {
		return nil
	}
	code, retry := e.Code, e.RetryDisposition
	switch code {
	case "staleConnection", "notConnected", "authRequired", "denied", "resourceNotVisible", "invalidInput", "conflict", "unavailable", "outcomeUnknown", "connectionFailure", "invalidResponse", "requestFamilyUnsupported", "authModeUnsupported", "ownerIdentityRequired":
	default:
		code = "requestFailed"
		retry = "never"
	}
	if mutating && (code == "connectionFailure" || code == "outcomeUnknown" || code == "invalidResponse" || e.HTTPStatus >= 500) {
		code = "outcomeUnknown"
		retry = "verifyOnly"
	}
	return dto.Failure(code, "runtimeIngress", retry, e.HTTPStatus)
}
func (s *Channels) call(epoch uint64, method, path string, input, output interface{}, mutating bool) *dto.SafeError {
	return safeError(s.provider.Do(context.Background(), epoch, method, path, input, output, "channel"), mutating)
}

func (s *Channels) Capabilities(r ScopeRequest) CapabilitiesResult {
	var v Capabilities
	if e := s.call(r.ConnectionEpoch, http.MethodGet, base+"/capabilities", nil, &v, false); e != nil {
		return CapabilitiesResult{Error: e}
	}
	v.Channels = allowed(v.Channels, "feishu", "dingtalk", "wecom")
	v.CredentialBindingChannels = allowed(v.CredentialBindingChannels, "feishu", "dingtalk", "wecom")
	v.MultiBotChannels = allowed(v.MultiBotChannels, "feishu")
	v.GroupPermissionChannels = allowed(v.GroupPermissionChannels, "feishu")
	v.FeishuDomains = allowed(v.FeishuDomains, "feishu", "lark")
	v.BotBindingTypes = allowed(v.BotBindingTypes, "shared", "private")
	v.AuthMode = enum(v.AuthMode, "runtime_key", "jwt", "legacy", "local")
	v.Version = token(v.Version, 32)
	v.ManagementScope = "runtimeAdmin"
	v.OwnerActionsRequireIdentity = true
	v.OwnerIdentityAvailable = s.provider.HasChannelOwnerIdentity()
	v.UpgradeHasCAS = false
	v.UpgradeAdvancesExecutionVersion = false
	v.ConfigHasCAS = false
	return CapabilitiesResult{OK: true, Value: &v}
}
func (s *Channels) List(r ScopeRequest) ListResult {
	var wire struct {
		Channels *[]json.RawMessage `json:"channels"`
	}
	if e := s.call(r.ConnectionEpoch, http.MethodGet, base, nil, &wire, false); e != nil {
		return ListResult{Error: e}
	}
	bad := func() ListResult {
		return ListResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 200)}
	}
	if wire.Channels == nil {
		return bad()
	}
	clean := make([]Account, 0, len(*wire.Channels))
	seen := map[string]bool{}
	for _, raw := range *wire.Channels {
		var a Account
		if json.Unmarshal(raw, &a) != nil || a.Channel == "" {
			return bad()
		}
		// Other channel types do not invalidate the supported multi-Bot directory.
		if !validChannel(a.Channel) {
			continue
		}
		if a.AppID == "" {
			var legacy struct {
				Config struct {
					AppID    string `json:"appId"`
					ClientID string `json:"clientId"`
					BotID    string `json:"botId"`
				} `json:"config"`
			}
			if json.Unmarshal(raw, &legacy) != nil {
				return bad()
			}
			switch a.Channel {
			case "feishu":
				a.AppID = legacy.Config.AppID
			case "dingtalk":
				a.AppID = legacy.Config.ClientID
			case "wecom":
				a.AppID = legacy.Config.BotID
			}
		}
		// Historical non-Feishu records can omit their provider identifier. They are
		// visible but cannot be passed to account-scoped mutations without an ID.
		if (a.AppID != "" && !validID(a.AppID, 256)) || (a.Channel == "feishu" && a.AppID == "") {
			return bad()
		}
		key := a.Channel + ":" + a.AppID
		if seen[key] {
			return bad()
		}
		seen[key] = true
		clean = append(clean, cleanAccount(a))
	}
	return ListResult{OK: true, Value: &AccountList{Channels: clean, ManagementScope: "runtimeAdmin"}}
}

type feishuConfig struct {
	AppID       string `json:"appId"`
	AppSecret   string `json:"appSecret"`
	Domain      string `json:"domain"`
	BindingType string `json:"bindingType"`
}
type dingtalkConfig struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}
type wecomConfig struct {
	BotID  string `json:"botId"`
	Secret string `json:"secret"`
}
type qrConfig struct {
	Domain      string `json:"domain"`
	BindingType string `json:"bindingType"`
}

func bindingOptions(domain, bindingType string) (string, string, bool) {
	if domain == "" {
		domain = "feishu"
	}
	if bindingType == "" {
		bindingType = "shared"
	}
	return domain, bindingType, (domain == "feishu" || domain == "lark") && (bindingType == "shared" || bindingType == "private")
}
func secretValid(v string) bool {
	return strings.TrimSpace(v) != "" && len(v) <= 16384 && !strings.ContainsAny(v, "\r\n\x00")
}
func createPayload(r BindingCreateRequest) (string, interface{}, *dto.SafeError) {
	if !validChannel(r.Channel) || !validID(r.OperationID, 128) {
		return "", nil, invalid()
	}
	if r.Mode != "qr" && r.Mode != "manual" {
		return "", nil, invalid()
	}
	switch r.Channel {
	case "feishu":
		domain, kind, ok := bindingOptions(r.Domain, r.BindingType)
		if !ok {
			return "", nil, invalid()
		}
		if r.ClientID != "" || r.ClientSecret != "" || r.BotID != "" || r.Secret != "" {
			return "", nil, invalid()
		}
		if r.Mode == "qr" {
			if r.AppID != "" || r.AppSecret != "" {
				return "", nil, invalid()
			}
			return base + "/feishu/bindings", qrConfig{domain, kind}, nil
		}
		if !validID(r.AppID, 256) || !secretValid(r.AppSecret) {
			return "", nil, invalid()
		}
		return base + "/feishu/bindings/manual", feishuConfig{r.AppID, r.AppSecret, domain, kind}, nil
	case "dingtalk":
		if r.Domain != "" || r.BindingType != "" || r.AppID != "" || r.AppSecret != "" || r.BotID != "" || r.Secret != "" {
			return "", nil, invalid()
		}
		if r.Mode == "qr" {
			if r.ClientID != "" || r.ClientSecret != "" {
				return "", nil, invalid()
			}
			return base + "/dingtalk/bindings", nil, nil
		}
		if !validID(r.ClientID, 256) || !secretValid(r.ClientSecret) {
			return "", nil, invalid()
		}
		return base + "/dingtalk/bindings/manual", dingtalkConfig{r.ClientID, r.ClientSecret}, nil
	case "wecom":
		if r.Mode != "manual" {
			return "", nil, unsupported()
		}
		if r.Domain != "" || r.BindingType != "" || r.AppID != "" || r.AppSecret != "" || r.ClientID != "" || r.ClientSecret != "" {
			return "", nil, invalid()
		}
		if !validID(r.BotID, 256) || !secretValid(r.Secret) {
			return "", nil, invalid()
		}
		return base + "/wecom/bindings", wecomConfig{r.BotID, r.Secret}, nil
	}
	return "", nil, unsupported()
}
func (s *Channels) CreateBinding(r BindingCreateRequest) BindingResult {
	path, payload, e := createPayload(r)
	if e != nil {
		return BindingResult{Error: e}
	}
	if e = s.owner(); e != nil {
		return BindingResult{Error: e}
	}
	domain, kind, _ := bindingOptions(r.Domain, r.BindingType)
	op := operation{Epoch: r.ConnectionEpoch, Channel: r.Channel, Domain: domain, BindingType: kind}
	s.mu.Lock()
	if _, exists := s.operations[r.OperationID]; exists {
		s.mu.Unlock()
		return BindingResult{Error: dto.Failure("operationAlreadyStarted", "mpa", "verifyOnly", 0)}
	}
	if len(s.operations) >= 2048 {
		s.mu.Unlock()
		return BindingResult{Error: dto.Failure("operationLimit", "mpa", "never", 0)}
	}
	s.operations[r.OperationID] = op
	s.mu.Unlock()
	var wire bindingWire
	e = s.call(r.ConnectionEpoch, http.MethodPost, path, payload, &wire, true)
	if e != nil {
		if e.HTTPStatus == 409 {
			e = dto.Failure("registrationUncertain", "runtimeIngress", "verifyOnly", 409)
		}
		op.Uncertain = e.Code == "registrationUncertain" || e.Code == "outcomeUnknown"
		s.mu.Lock()
		s.operations[r.OperationID] = op
		s.mu.Unlock()
		return BindingResult{Error: e}
	}
	v := projectBinding(wire, r.OperationID, r.Channel, domain, kind)
	if (r.Mode == "qr" && !validID(v.ID, 256)) || (v.AppID != "" && !validID(v.AppID, 256)) {
		return BindingResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 200)}
	}
	if containsInputSecret(v, r.AppSecret, r.ClientSecret, r.Secret) {
		return BindingResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 200)}
	}
	op.BindingID = v.ID
	op.Uncertain = v.State == "unknown"
	s.mu.Lock()
	s.operations[r.OperationID] = op
	s.mu.Unlock()
	return BindingResult{OK: true, Value: &v}
}
func (s *Channels) operationFor(r BindingRequest, allowResume bool) (operation, *dto.SafeError) {
	if !validID(r.OperationID, 128) || !validID(r.BindingID, 256) || !(r.Channel == "feishu" || r.Channel == "dingtalk") {
		return operation{}, invalid()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	op, exists := s.operations[r.OperationID]
	if !exists {
		if !r.ResumeConfirmed || !allowResume {
			return operation{}, dto.Failure("bindingOperationRequired", "mpa", "never", 0)
		}
		if len(s.operations) >= 2048 {
			return operation{}, dto.Failure("operationLimit", "mpa", "never", 0)
		}
		op = operation{Epoch: r.ConnectionEpoch, Channel: r.Channel, BindingID: r.BindingID}
		s.operations[r.OperationID] = op
	}
	if op.Epoch != r.ConnectionEpoch {
		return operation{}, dto.Failure("staleConnection", "transport", "reload", 0)
	}
	if op.Channel != r.Channel || op.BindingID != r.BindingID {
		return operation{}, dto.Failure("bindingOperationMismatch", "mpa", "never", 0)
	}
	if op.InFlight {
		return operation{}, dto.Failure("bindingOperationBusy", "mpa", "verifyOnly", 0)
	}
	op.InFlight = true
	s.operations[r.OperationID] = op
	return op, nil
}
func (s *Channels) ReadBinding(r BindingRequest) BindingResult  { return s.pollBinding(r, false) }
func (s *Channels) RetryBinding(r BindingRequest) BindingResult { return s.pollBinding(r, true) }
func (s *Channels) pollBinding(r BindingRequest, retry bool) BindingResult {
	if e := s.owner(); e != nil {
		return BindingResult{Error: e}
	}
	op, e := s.operationFor(r, !retry)
	if e != nil {
		return BindingResult{Error: e}
	}
	defer func() {
		s.mu.Lock()
		latest := s.operations[r.OperationID]
		latest.InFlight = false
		s.operations[r.OperationID] = latest
		s.mu.Unlock()
	}()
	if retry && op.Uncertain && !r.GatewayVerified {
		return BindingResult{Error: dto.Failure("registrationUncertain", "mpa", "verifyOnly", 409)}
	}
	method, path := http.MethodGet, base+"/"+r.Channel+"/bindings/"+url.PathEscape(r.BindingID)
	if retry {
		method = http.MethodPost
		path += "/retry"
	}
	var wire bindingWire
	e = s.call(r.ConnectionEpoch, method, path, nil, &wire, true)
	if e != nil {
		if e.HTTPStatus == 409 {
			e = dto.Failure("registrationUncertain", "runtimeIngress", "verifyOnly", 409)
		}
		if e.Code == "outcomeUnknown" || e.Code == "registrationUncertain" {
			op.Uncertain = true
			s.mu.Lock()
			s.operations[r.OperationID] = op
			s.mu.Unlock()
		}
		return BindingResult{Error: e}
	}
	if wire.ID != r.BindingID {
		op.Uncertain = true
		s.mu.Lock()
		s.operations[r.OperationID] = op
		s.mu.Unlock()
		return BindingResult{Error: dto.Failure("outcomeUnknown", "runtimeIngress", "verifyOnly", 200)}
	}
	v := projectBinding(wire, r.OperationID, r.Channel, op.Domain, op.BindingType)
	op.Uncertain = v.State == "unknown"
	s.mu.Lock()
	s.operations[r.OperationID] = op
	s.mu.Unlock()
	return BindingResult{OK: true, Value: &v}
}

func (s *Channels) existing(r AccountRequest, requireSingle bool) (*Account, *dto.SafeError) {
	if !accountValid(r) {
		return nil, invalid()
	}
	result := s.List(ScopeRequest{r.ConnectionEpoch})
	if !result.OK {
		return nil, result.Error
	}
	var found *Account
	count := 0
	for _, a := range result.Value.Channels {
		if a.Channel == r.Channel {
			count++
		}
		if a.Channel == r.Channel && a.AppID == r.AppID {
			copy := a
			found = &copy
		}
	}
	if found == nil {
		return nil, dto.Failure("resourceNotVisible", "runtimeIngress", "never", 404)
	}
	if requireSingle && r.Channel != "feishu" && count != 1 {
		return nil, dto.Failure("singleAccountRequired", "mpa", "never", 0)
	}
	return found, nil
}

type upsertPayload struct {
	Channel           string          `json:"channel"`
	Feishu            *feishuConfig   `json:"feishu,omitempty"`
	Dingtalk          *dingtalkConfig `json:"dingtalk,omitempty"`
	Wecom             *wecomConfig    `json:"wecom,omitempty"`
	GroupSessionScope string          `json:"groupSessionScope,omitempty"`
}

func (s *Channels) SaveConfig(r ConfigRequest) AccountResult {
	account := AccountRequest{r.ConnectionEpoch, r.Channel, r.AppID}
	if !accountValid(account) || !scopeValid(r.GroupSessionScope) {
		return AccountResult{Error: invalid()}
	}
	old, e := s.existing(account, true)
	if e != nil {
		return AccountResult{Error: e}
	}
	payload := upsertPayload{Channel: r.Channel}
	switch r.Channel {
	case "feishu":
		if r.Domain == "" {
			r.Domain = old.Config.Domain
		}
		if r.BindingType == "" {
			r.BindingType = old.BindingType
		}
		domain, kind, ok := bindingOptions(r.Domain, r.BindingType)
		if !ok || !secretValid(r.AppSecret) || r.ClientSecret != "" || r.Secret != "" {
			return AccountResult{Error: invalid()}
		}
		if kind == "private" {
			if e := s.owner(); e != nil {
				return AccountResult{Error: e}
			}
		}
		payload.Feishu = &feishuConfig{r.AppID, r.AppSecret, domain, kind}
		payload.GroupSessionScope = r.GroupSessionScope
	case "dingtalk":
		if r.GroupSessionScope != "" || r.Domain != "" || r.BindingType != "" || r.AppSecret != "" || r.Secret != "" || !secretValid(r.ClientSecret) {
			return AccountResult{Error: invalid()}
		}
		payload.Dingtalk = &dingtalkConfig{r.AppID, r.ClientSecret}
	case "wecom":
		if r.GroupSessionScope != "" || r.Domain != "" || r.BindingType != "" || r.AppSecret != "" || r.ClientSecret != "" || !secretValid(r.Secret) {
			return AccountResult{Error: invalid()}
		}
		payload.Wecom = &wecomConfig{r.AppID, r.Secret}
	}
	var wire struct {
		Success bool    `json:"success"`
		Record  Account `json:"record"`
	}
	if e := s.call(r.ConnectionEpoch, http.MethodPost, base+"/upsert", payload, &wire, true); e != nil {
		return AccountResult{Error: e}
	}
	if !wire.Success || wire.Record.Channel != r.Channel || wire.Record.AppID != r.AppID {
		return AccountResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 200)}
	}
	current, e := s.existing(account, true)
	if e != nil {
		return AccountResult{Error: e}
	}
	if r.Channel == "feishu" && (current.Config.Domain != payload.Feishu.Domain || current.BindingType != payload.Feishu.BindingType || (r.GroupSessionScope != "" && current.GroupSessionScope != r.GroupSessionScope)) {
		return AccountResult{Error: dto.Failure("outcomeUnknown", "runtimeIngress", "verifyOnly", 200)}
	}
	if containsInputSecret(current, r.AppSecret, r.ClientSecret, r.Secret) {
		return AccountResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 200)}
	}
	return AccountResult{OK: true, Value: current}
}
func (s *Channels) SetEnabled(r EnabledRequest) AccountResult {
	if r.Channel != "feishu" {
		return AccountResult{Error: unsupported()}
	}
	if !validID(r.AppID, 256) {
		return AccountResult{Error: invalid()}
	}
	var wire Account
	if e := s.call(r.ConnectionEpoch, http.MethodPatch, accountPath(r.AppID), struct {
		Enabled bool `json:"enabled"`
	}{r.Enabled}, &wire, true); e != nil {
		return AccountResult{Error: e}
	}
	if wire.AppID != r.AppID || wire.Channel != "feishu" || wire.Enabled != r.Enabled {
		return AccountResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 200)}
	}
	v := cleanAccount(wire)
	return AccountResult{OK: true, Value: &v}
}

func (s *Channels) Diagnostics(r AccountRequest) DiagnosticsResult {
	if !accountValid(r) {
		return DiagnosticsResult{Error: invalid()}
	}
	path := base + "/" + r.Channel + "/diagnostics"
	if r.Channel == "feishu" {
		path += "?" + url.Values{"appId": {r.AppID}}.Encode()
	} else {
		if _, e := s.existing(r, true); e != nil {
			return DiagnosticsResult{Error: e}
		}
	}
	var wire struct {
		Diagnostics
		LastInboundAt  json.RawMessage `json:"lastInboundAt"`
		LastDeliveryAt json.RawMessage `json:"lastDeliveryAt"`
	}
	if e := s.call(r.ConnectionEpoch, http.MethodGet, path, nil, &wire, false); e != nil {
		return DiagnosticsResult{Error: e}
	}
	v := wire.Diagnostics
	inbound, inboundOK := diagnosticTime(wire.LastInboundAt)
	delivery, deliveryOK := diagnosticTime(wire.LastDeliveryAt)
	if !inboundOK || !deliveryOK || v.Channel != r.Channel || v.AppID != r.AppID {
		return DiagnosticsResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 200)}
	}
	v.MissingConfiguration = allowed(v.MissingConfiguration, "private_endpoint", "runtime_api_key", "apig_instance_id")
	v.DeliveryHealth = enum(v.DeliveryHealth, "not_observed", "retrying", "delivered")
	if v.DeliveryHealth == "" {
		v.DeliveryHealth = "not_observed"
	}
	v.GroupSessionScope = enum(v.GroupSessionScope, "group", "group_sender", "group_topic", "group_topic_sender")
	v.GroupTrigger = enum(v.GroupTrigger, "mention", "platform_default")
	v.LastInboundAt = inbound
	v.LastDeliveryAt = delivery
	return DiagnosticsResult{OK: true, Value: &v}
}
func (s *Channels) Remove(r AccountRequest) RemovalResult {
	if _, e := s.existing(r, false); e != nil {
		return RemovalResult{Error: e}
	}
	var wire MutationReceipt
	path := base + "?" + url.Values{"channel": {r.Channel}, "appId": {r.AppID}}.Encode()
	if e := s.call(r.ConnectionEpoch, http.MethodDelete, path, nil, &wire, true); e != nil {
		return RemovalResult{Error: e}
	}
	if !wire.Success {
		return RemovalResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 200)}
	}
	current := s.List(ScopeRequest{r.ConnectionEpoch})
	if !current.OK {
		return RemovalResult{Error: current.Error}
	}
	for _, a := range current.Value.Channels {
		if a.Channel == r.Channel && a.AppID == r.AppID {
			return RemovalResult{Error: dto.Failure("outcomeUnknown", "runtimeIngress", "verifyOnly", 200)}
		}
	}
	v := RemovalReceipt{Channel: r.Channel, AppID: r.AppID, Removed: true, HistoryDeleted: false}
	return RemovalResult{OK: true, Value: &v}
}
