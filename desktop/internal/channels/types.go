// Package channels exposes fixed, provider-specific Runtime channel actions.
// Credentials are input-only; output types never contain provider credentials,
// raw errors, arbitrary metadata, or Gateway URLs.
package channels

import "github.com/superops-team/OpenMPA/desktop/internal/dto"

type ScopeRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
}
type AccountRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	Channel         string `json:"channel"`
	AppID           string `json:"appId"`
}
type BindingCreateRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	OperationID     string `json:"operationId"`
	Channel         string `json:"channel"`
	Mode            string `json:"mode"`
	Domain          string `json:"domain,omitempty"`
	BindingType     string `json:"bindingType,omitempty"`
	AppID           string `json:"appId,omitempty"`
	AppSecret       string `json:"appSecret,omitempty"`
	ClientID        string `json:"clientId,omitempty"`
	ClientSecret    string `json:"clientSecret,omitempty"`
	BotID           string `json:"botId,omitempty"`
	Secret          string `json:"secret,omitempty"`
}
type BindingRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	OperationID     string `json:"operationId"`
	Channel         string `json:"channel"`
	BindingID       string `json:"bindingId"`
	// ResumeConfirmed means the user explicitly continues a previously started
	// operation after restarting. The server still verifies the binding actor.
	ResumeConfirmed bool `json:"resumeConfirmed,omitempty"`
	GatewayVerified bool `json:"gatewayVerified,omitempty"`
}
type ConfigRequest struct {
	ConnectionEpoch   uint64 `json:"connectionEpoch"`
	Channel           string `json:"channel"`
	AppID             string `json:"appId"`
	AppSecret         string `json:"appSecret,omitempty"`
	ClientSecret      string `json:"clientSecret,omitempty"`
	Secret            string `json:"secret,omitempty"`
	Domain            string `json:"domain,omitempty"`
	BindingType       string `json:"bindingType,omitempty"`
	GroupSessionScope string `json:"groupSessionScope,omitempty"`
}
type EnabledRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	Channel         string `json:"channel"`
	AppID           string `json:"appId"`
	Enabled         bool   `json:"enabled"`
}
type ProfileSaveRequest struct {
	ConnectionEpoch  uint64    `json:"connectionEpoch"`
	Channel          string    `json:"channel"`
	AppID            string    `json:"appId"`
	AgentsMD         *string   `json:"agentsMd"`
	Model            *string   `json:"model"`
	Skills           *[]string `json:"skills"`
	MCP              *[]string `json:"mcp"`
	ExpectedRevision int       `json:"expectedRevision"`
}
type UpgradeRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	Channel         string `json:"channel"`
	AppID           string `json:"appId"`
	SessionID       string `json:"sessionId"`
}
type PermissionRequest struct {
	ConnectionEpoch   uint64 `json:"connectionEpoch"`
	Channel           string `json:"channel"`
	AppID             string `json:"appId"`
	ChatID            string `json:"chatId"`
	GroupSessionScope string `json:"groupSessionScope,omitempty"`
}
type PrivateMutationRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	Channel         string `json:"channel"`
	AppID           string `json:"appId"`
	ExpectedVersion string `json:"expectedVersion"`
}

type Capabilities struct {
	Version                   string   `json:"version"`
	Channels                  []string `json:"channels"`
	CredentialBindingChannels []string `json:"credentialBindingChannels"`
	MultiBotChannels          []string `json:"multiBotChannels"`
	GroupPermissionChannels   []string `json:"groupPermissionChannels"`
	FeishuDomains             []string `json:"feishuDomains"`
	BotBindingTypes           []string `json:"botBindingTypes"`
	ServerSideBinding         bool     `json:"serverSideBinding"`
	BindingReady              bool     `json:"bindingReady"`
	GroupPermissions          bool     `json:"groupPermissions"`
	Diagnostics               bool     `json:"diagnostics"`
	BotProfiles               bool     `json:"botProfiles"`
	PrivateBotUsers           bool     `json:"privateBotUsers"`
	AccountScopedPermissions  bool     `json:"accountScopedPermissions"`
	AuthMode                  string   `json:"authMode"`
	// Capabilities describe routes, not individual user or Gateway authorization.
	ManagementScope                 string `json:"managementScope"`
	OwnerActionsRequireIdentity     bool   `json:"ownerActionsRequireIdentity"`
	OwnerIdentityAvailable          bool   `json:"ownerIdentityAvailable"`
	UpgradeHasCAS                   bool   `json:"upgradeHasCas"`
	UpgradeAdvancesExecutionVersion bool   `json:"upgradeAdvancesExecutionVersion"`
	ConfigHasCAS                    bool   `json:"configHasCas"`
}
type AccountConfig struct {
	Domain      string `json:"domain,omitempty"`
	OpenID      string `json:"openId,omitempty"`
	TenantBrand string `json:"tenantBrand,omitempty"`
	RobotName   string `json:"robotName,omitempty"`
}
type Account struct {
	Channel           string        `json:"channel"`
	AppID             string        `json:"appId"`
	AppName           string        `json:"appName,omitempty"`
	Enabled           bool          `json:"enabled"`
	BindingType       string        `json:"bindingType,omitempty"`
	OwnerUserID       string        `json:"ownerUserId,omitempty"`
	BoundByUserID     string        `json:"boundByUserId,omitempty"`
	BoundByAuthMode   string        `json:"boundByAuthMode,omitempty"`
	IMGatewayUUID     string        `json:"imGatewayUuid,omitempty"`
	GroupSessionScope string        `json:"groupSessionScope,omitempty"`
	Config            AccountConfig `json:"config"`
	CreatedAt         string        `json:"createdAt,omitempty"`
	UpdatedAt         string        `json:"updatedAt,omitempty"`
}
type AccountList struct {
	Channels        []Account `json:"channels"`
	ManagementScope string    `json:"managementScope"`
}
type Binding struct {
	OperationID      string `json:"operationId"`
	ID               string `json:"id,omitempty"`
	Channel          string `json:"channel"`
	Domain           string `json:"domain,omitempty"`
	BindingType      string `json:"bindingType,omitempty"`
	Status           string `json:"status"`
	State            string `json:"state"`
	AppID            string `json:"appId,omitempty"`
	QRCodeImage      string `json:"qrCodeImage,omitempty"`
	LoginURL         string `json:"loginUrl,omitempty"`
	ExpiresAt        string `json:"expiresAt,omitempty"`
	PollAfterSeconds int    `json:"pollAfterSeconds,omitempty"`
	ErrorCode        string `json:"errorCode,omitempty"`
}
type Profile struct {
	AppID        string    `json:"appId"`
	Revision     int       `json:"revision"`
	AgentsMD     *string   `json:"agentsMd"`
	Model        *string   `json:"model"`
	Skills       *[]string `json:"skills"`
	MCP          *[]string `json:"mcp"`
	ConfigDigest string    `json:"configDigest,omitempty"`
	OwnerUserID  string    `json:"ownerUserId,omitempty"`
}
type ProfileUpgrade struct {
	SessionID                string  `json:"sessionId"`
	Profile                  Profile `json:"profile"`
	HasCAS                   bool    `json:"hasCas"`
	AdvancesExecutionVersion bool    `json:"advancesExecutionVersion"`
}
type Permission struct {
	Channel           string `json:"channel"`
	AppID             string `json:"appId"`
	ChatID            string `json:"chatId"`
	ChatName          string `json:"chatName,omitempty"`
	Avatar            string `json:"avatar,omitempty"`
	ChatMode          string `json:"chatMode,omitempty"`
	GroupSessionScope string `json:"groupSessionScope,omitempty"`
	CreatedAt         string `json:"createdAt,omitempty"`
	UpdatedAt         string `json:"updatedAt,omitempty"`
}
type PermissionList struct {
	Permissions []Permission `json:"permissions"`
}
type Diagnostics struct {
	Channel                 string   `json:"channel"`
	AppID                   string   `json:"appId"`
	Configured              bool     `json:"configured"`
	Enabled                 bool     `json:"enabled"`
	GatewayConfigured       bool     `json:"gatewayConfigured"`
	RouteConfigured         bool     `json:"routeConfigured"`
	MissingConfiguration    []string `json:"missingConfiguration"`
	GroupSessionScope       string   `json:"groupSessionScope,omitempty"`
	GroupTrigger            string   `json:"groupTrigger,omitempty"`
	GroupPermissionRequired bool     `json:"groupPermissionRequired"`
	DeliveryHealth          string   `json:"deliveryHealth"`
	LastInboundAt           string   `json:"lastInboundAt,omitempty"`
	LastDeliveryAt          string   `json:"lastDeliveryAt,omitempty"`
}
type PrivateSlot struct {
	Version       string `json:"version"`
	ChatID        string `json:"chatId,omitempty"`
	SenderID      string `json:"senderId,omitempty"`
	SenderIDType  string `json:"senderIdType,omitempty"`
	RuntimeUserID string `json:"runtimeUserId,omitempty"`
}
type PrivateAuthorization struct {
	Status string `json:"status"`
}
type PrivateUser struct {
	AppID         string               `json:"appId"`
	Slot          *PrivateSlot         `json:"slot"`
	Authorization PrivateAuthorization `json:"authorization"`
}
type RemovalReceipt struct {
	Channel        string `json:"channel"`
	AppID          string `json:"appId"`
	Removed        bool   `json:"removed"`
	HistoryDeleted bool   `json:"historyDeleted"`
}
type MutationReceipt struct {
	Success bool `json:"success"`
}
type PrivateMutation struct {
	AppID   string      `json:"appId"`
	Revoked bool        `json:"revoked"`
	Reset   bool        `json:"reset"`
	Current PrivateUser `json:"current"`
}

type CapabilitiesResult struct {
	OK    bool           `json:"ok"`
	Value *Capabilities  `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type ListResult struct {
	OK    bool           `json:"ok"`
	Value *AccountList   `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type BindingResult struct {
	OK    bool           `json:"ok"`
	Value *Binding       `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type AccountResult struct {
	OK    bool           `json:"ok"`
	Value *Account       `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type ProfileResult struct {
	OK    bool           `json:"ok"`
	Value *Profile       `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type UpgradeResult struct {
	OK    bool            `json:"ok"`
	Value *ProfileUpgrade `json:"value,omitempty"`
	Error *dto.SafeError  `json:"error,omitempty"`
}
type PermissionsResult struct {
	OK    bool            `json:"ok"`
	Value *PermissionList `json:"value,omitempty"`
	Error *dto.SafeError  `json:"error,omitempty"`
}
type PermissionResult struct {
	OK    bool           `json:"ok"`
	Value *Permission    `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type MutationResult struct {
	OK    bool             `json:"ok"`
	Value *MutationReceipt `json:"value,omitempty"`
	Error *dto.SafeError   `json:"error,omitempty"`
}
type DiagnosticsResult struct {
	OK    bool           `json:"ok"`
	Value *Diagnostics   `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type RemovalResult struct {
	OK    bool            `json:"ok"`
	Value *RemovalReceipt `json:"value,omitempty"`
	Error *dto.SafeError  `json:"error,omitempty"`
}
type PrivateUserResult struct {
	OK    bool           `json:"ok"`
	Value *PrivateUser   `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type PrivateMutationResult struct {
	OK    bool             `json:"ok"`
	Value *PrivateMutation `json:"value,omitempty"`
	Error *dto.SafeError   `json:"error,omitempty"`
}
