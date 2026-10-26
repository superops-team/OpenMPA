package channels

import (
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func validateProfile(v Profile) bool {
	if v.AgentsMD != nil && utf8.RuneCountInString(*v.AgentsMD) > 32000 {
		return false
	}
	if v.Model != nil && (utf8.RuneCountInString(*v.Model) > 256 || strings.TrimSpace(*v.Model) == "" || strings.ContainsAny(*v.Model, "/:")) {
		return false
	}
	for _, refs := range []*[]string{v.Skills, v.MCP} {
		if refs != nil {
			if len(*refs) > 200 {
				return false
			}
			for _, ref := range *refs {
				if strings.TrimSpace(ref) == "" || utf8.RuneCountInString(ref) > 256 {
					return false
				}
			}
		}
	}
	return v.Revision >= 0
}
func normalizeRefs(refs *[]string) *[]string {
	if refs == nil {
		return nil
	}
	clean := make([]string, 0, len(*refs))
	seen := map[string]bool{}
	for _, ref := range *refs {
		ref = strings.TrimSpace(ref)
		if !seen[ref] {
			clean = append(clean, ref)
			seen[ref] = true
		}
	}
	return &clean
}
func cleanProfile(v Profile, appID string) (Profile, *dto.SafeError) {
	if v.AppID == "" {
		v.AppID = appID
	}
	if v.AppID != appID || !validateProfile(v) {
		return Profile{}, dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 200)
	}
	if v.Model != nil {
		trim := strings.TrimSpace(*v.Model)
		v.Model = &trim
	}
	v.Skills = normalizeRefs(v.Skills)
	v.MCP = normalizeRefs(v.MCP)
	v.ConfigDigest = digest(v.ConfigDigest)
	v.OwnerUserID = token(v.OwnerUserID, 256)
	return v, nil
}
func (s *Channels) ReadProfile(r AccountRequest) ProfileResult {
	if r.Channel != "feishu" {
		return ProfileResult{Error: unsupported()}
	}
	if !validID(r.AppID, 256) {
		return ProfileResult{Error: invalid()}
	}
	if e := s.owner(); e != nil {
		return ProfileResult{Error: e}
	}
	var wire Profile
	if e := s.call(r.ConnectionEpoch, http.MethodGet, accountPath(r.AppID)+"/profile", nil, &wire, false); e != nil {
		return ProfileResult{Error: e}
	}
	v, e := cleanProfile(wire, r.AppID)
	if e != nil {
		return ProfileResult{Error: e}
	}
	return ProfileResult{OK: true, Value: &v}
}

type profilePayload struct {
	AgentsMD         *string   `json:"agentsMd"`
	Model            *string   `json:"model"`
	Skills           *[]string `json:"skills"`
	MCP              *[]string `json:"mcp"`
	ExpectedRevision int       `json:"expectedRevision"`
}

func (s *Channels) SaveProfile(r ProfileSaveRequest) ProfileResult {
	if r.Channel != "feishu" {
		return ProfileResult{Error: unsupported()}
	}
	v := Profile{AppID: r.AppID, Revision: r.ExpectedRevision, AgentsMD: r.AgentsMD, Model: r.Model, Skills: r.Skills, MCP: r.MCP}
	if !validID(r.AppID, 256) || !validateProfile(v) {
		return ProfileResult{Error: invalid()}
	}
	if e := s.owner(); e != nil {
		return ProfileResult{Error: e}
	}
	payload := profilePayload{AgentsMD: r.AgentsMD, Model: r.Model, Skills: normalizeRefs(r.Skills), MCP: normalizeRefs(r.MCP), ExpectedRevision: r.ExpectedRevision}
	var wire Profile
	if e := s.call(r.ConnectionEpoch, http.MethodPut, accountPath(r.AppID)+"/profile", payload, &wire, true); e != nil {
		return ProfileResult{Error: e}
	}
	current, e := cleanProfile(wire, r.AppID)
	if e != nil {
		return ProfileResult{Error: e}
	}
	if current.Revision != r.ExpectedRevision+1 {
		return ProfileResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 200)}
	}
	return ProfileResult{OK: true, Value: &current}
}
func (s *Channels) UpgradeProfile(r UpgradeRequest) UpgradeResult {
	if r.Channel != "feishu" {
		return UpgradeResult{Error: unsupported()}
	}
	if !validID(r.AppID, 256) || !validID(r.SessionID, 256) {
		return UpgradeResult{Error: invalid()}
	}
	if e := s.owner(); e != nil {
		return UpgradeResult{Error: e}
	}
	// This route has no CAS or idempotency input and changes only the Bot snapshot.
	// An unknown outcome must be inspected through the existing session contract.
	path := accountPath(r.AppID) + "/sessions/" + url.PathEscape(r.SessionID) + "/profile-upgrade"
	var wire Profile
	if e := s.call(r.ConnectionEpoch, http.MethodPost, path, nil, &wire, true); e != nil {
		return UpgradeResult{Error: e}
	}
	profile, e := cleanProfile(wire, r.AppID)
	if e != nil {
		return UpgradeResult{Error: e}
	}
	v := ProfileUpgrade{SessionID: r.SessionID, Profile: profile, HasCAS: false, AdvancesExecutionVersion: false}
	return UpgradeResult{OK: true, Value: &v}
}

func (s *Channels) Permissions(r AccountRequest) PermissionsResult {
	if r.Channel != "feishu" {
		return PermissionsResult{Error: unsupported()}
	}
	if !validID(r.AppID, 256) {
		return PermissionsResult{Error: invalid()}
	}
	var v PermissionList
	path := base + "/chat-permissions?" + url.Values{"channel": {"feishu"}, "appId": {r.AppID}}.Encode()
	if e := s.call(r.ConnectionEpoch, http.MethodGet, path, nil, &v, false); e != nil {
		return PermissionsResult{Error: e}
	}
	if v.Permissions == nil {
		v.Permissions = []Permission{}
	}
	for i, p := range v.Permissions {
		if p.Channel != "feishu" || p.AppID != r.AppID || !validID(p.ChatID, 512) {
			return PermissionsResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 200)}
		}
		v.Permissions[i] = cleanPermission(p)
	}
	return PermissionsResult{OK: true, Value: &v}
}
func (s *Channels) SavePermission(r PermissionRequest) PermissionResult {
	if r.Channel != "feishu" {
		return PermissionResult{Error: unsupported()}
	}
	if !validID(r.AppID, 256) || !validID(r.ChatID, 512) || !scopeValid(r.GroupSessionScope) {
		return PermissionResult{Error: invalid()}
	}
	payload := struct {
		Channel           string `json:"channel"`
		AppID             string `json:"appId"`
		ChatID            string `json:"chatId"`
		GroupSessionScope string `json:"groupSessionScope,omitempty"`
	}{r.Channel, r.AppID, r.ChatID, r.GroupSessionScope}
	var wire Permission
	if e := s.call(r.ConnectionEpoch, http.MethodPost, base+"/chat-permissions", payload, &wire, true); e != nil {
		return PermissionResult{Error: e}
	}
	if wire.Channel != "feishu" || wire.AppID != r.AppID || wire.ChatID != r.ChatID {
		return PermissionResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 200)}
	}
	v := cleanPermission(wire)
	return PermissionResult{OK: true, Value: &v}
}
func (s *Channels) DeletePermission(r PermissionRequest) MutationResult {
	if r.Channel != "feishu" {
		return MutationResult{Error: unsupported()}
	}
	if !validID(r.AppID, 256) || !validID(r.ChatID, 512) {
		return MutationResult{Error: invalid()}
	}
	path := base + "/chat-permissions?" + url.Values{"channel": {"feishu"}, "appId": {r.AppID}, "chat_id": {r.ChatID}}.Encode()
	var v MutationReceipt
	if e := s.call(r.ConnectionEpoch, http.MethodDelete, path, nil, &v, true); e != nil {
		return MutationResult{Error: e}
	}
	if !v.Success {
		return MutationResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 200)}
	}
	return MutationResult{OK: true, Value: &v}
}

type privateUserWire struct {
	AppID string `json:"appId"`
	Slot  *struct {
		Version       string `json:"version"`
		ChatID        string `json:"chat_id"`
		SenderID      string `json:"sender_id"`
		SenderIDType  string `json:"sender_id_type"`
		RuntimeUserID string `json:"runtime_user_id"`
	} `json:"slot"`
	Authorization PrivateAuthorization `json:"authorization"`
}

func (s *Channels) PrivateUser(r AccountRequest) PrivateUserResult {
	if r.Channel != "feishu" {
		return PrivateUserResult{Error: unsupported()}
	}
	if !validID(r.AppID, 256) {
		return PrivateUserResult{Error: invalid()}
	}
	if e := s.owner(); e != nil {
		return PrivateUserResult{Error: e}
	}
	var wire privateUserWire
	if e := s.call(r.ConnectionEpoch, http.MethodGet, accountPath(r.AppID)+"/private-user", nil, &wire, false); e != nil {
		return PrivateUserResult{Error: e}
	}
	if wire.AppID != r.AppID {
		return PrivateUserResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 200)}
	}
	v := PrivateUser{AppID: r.AppID, Authorization: PrivateAuthorization{Status: enum(wire.Authorization.Status, "not_authorized", "ready", "revoked", "pending", "expired", "starting", "authorization_required", "identity_mismatch", "refreshing", "authorized")}}
	if v.Authorization.Status == "" {
		v.Authorization.Status = "unknown"
	}
	if wire.Slot != nil {
		if !validID(wire.Slot.Version, 64) {
			return PrivateUserResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 200)}
		}
		v.Slot = &PrivateSlot{Version: wire.Slot.Version, ChatID: token(wire.Slot.ChatID, 512), SenderID: token(wire.Slot.SenderID, 256), SenderIDType: token(wire.Slot.SenderIDType, 32), RuntimeUserID: token(wire.Slot.RuntimeUserID, 256)}
	}
	s.mu.Lock()
	if v.Slot != nil {
		s.slots[slotKey{r.ConnectionEpoch, r.AppID}] = v.Slot.Version
	} else {
		delete(s.slots, slotKey{r.ConnectionEpoch, r.AppID})
	}
	s.mu.Unlock()
	return PrivateUserResult{OK: true, Value: &v}
}
func (s *Channels) RevokePrivateAuthorization(r PrivateMutationRequest) PrivateMutationResult {
	return s.mutatePrivate(r, false)
}
func (s *Channels) ResetPrivateUser(r PrivateMutationRequest) PrivateMutationResult {
	return s.mutatePrivate(r, true)
}
func (s *Channels) mutatePrivate(r PrivateMutationRequest, reset bool) PrivateMutationResult {
	if r.Channel != "feishu" {
		return PrivateMutationResult{Error: unsupported()}
	}
	if !validID(r.AppID, 256) || !validID(r.ExpectedVersion, 64) {
		return PrivateMutationResult{Error: invalid()}
	}
	if e := s.owner(); e != nil {
		return PrivateMutationResult{Error: e}
	}
	s.mu.Lock()
	observed := s.slots[slotKey{r.ConnectionEpoch, r.AppID}]
	s.mu.Unlock()
	if observed == "" {
		return PrivateMutationResult{Error: dto.Failure("privateSlotReadRequired", "mpa", "reload", 0)}
	}
	if observed != r.ExpectedVersion {
		return PrivateMutationResult{Error: dto.Failure("conflict", "mpa", "reload", 409)}
	}
	// Re-read without changing the expected_version that the user confirmed.
	current := s.PrivateUser(AccountRequest{r.ConnectionEpoch, r.Channel, r.AppID})
	if !current.OK {
		return PrivateMutationResult{Error: current.Error}
	}
	if current.Value.Slot == nil || current.Value.Slot.Version != r.ExpectedVersion {
		return PrivateMutationResult{Error: dto.Failure("conflict", "mpa", "reload", 409)}
	}
	path := accountPath(r.AppID) + "/private-user"
	if !reset {
		path += "/authorization"
	}
	path += "?" + url.Values{"expected_version": {r.ExpectedVersion}}.Encode()
	var wire struct {
		AppID   string `json:"appId"`
		Revoked bool   `json:"revoked"`
		Reset   bool   `json:"reset"`
	}
	if e := s.call(r.ConnectionEpoch, http.MethodDelete, path, nil, &wire, true); e != nil {
		return PrivateMutationResult{Error: e}
	}
	if wire.AppID != r.AppID || (reset && !wire.Reset) || (!reset && !wire.Revoked) {
		return PrivateMutationResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 200)}
	}
	result := s.PrivateUser(AccountRequest{r.ConnectionEpoch, r.Channel, r.AppID})
	if !result.OK {
		return PrivateMutationResult{Error: result.Error}
	}
	v := PrivateMutation{AppID: r.AppID, Reset: wire.Reset, Revoked: wire.Revoked, Current: *result.Value}
	return PrivateMutationResult{OK: true, Value: &v}
}
