// Package dto defines the only values allowed to cross the desktop IPC boundary.
package dto

type CurrentState struct {
	MemoryRef              string `json:"memoryRef,omitempty"`
	OperationRef           string `json:"operationRef,omitempty"`
	OperationState         string `json:"operationState,omitempty"`
	ProviderState          string `json:"providerState,omitempty"`
	Version                int    `json:"version,omitempty"`
	ID                     string `json:"id,omitempty"`
	Enabled                *bool  `json:"enabled,omitempty"`
	ExecutionConfigVersion string `json:"executionConfigVersion,omitempty"`
	Status                 string `json:"status,omitempty"`
	Generation             int    `json:"generation,omitempty"`
}
type SafeError struct {
	Code             string        `json:"code"`
	Message          string        `json:"message"`
	HTTPStatus       int           `json:"httpStatus"`
	Plane            string        `json:"plane"`
	OriginConfidence string        `json:"originConfidence"`
	RetryDisposition string        `json:"retryDisposition"`
	RequestID        string        `json:"requestId,omitempty"`
	CurrentState     *CurrentState `json:"currentState,omitempty"`
}

func (e *SafeError) Error() string { return e.Code }
func Failure(code, plane, retry string, status int) *SafeError {
	return &SafeError{Code: code, Message: SafeMessage(code), HTTPStatus: status, Plane: plane, OriginConfidence: "unknown", RetryDisposition: retry}
}
func SafeMessage(code string) string {
	switch code {
	case "authRequired":
		return "Authentication is required."
	case "denied":
		return "This action is not permitted."
	case "outcomeUnknown":
		return "The server may have accepted this operation. Verify its state before submitting again."
	case "staleConnection":
		return "The connection changed. Reload this view."
	case "conflict":
		return "The resource changed. Reload and review the current state."
	case "invalidInput":
		return "The request contains invalid input."
	case "notConnected":
		return "Choose and connect a Runtime first."
	default:
		return "The requested operation could not be completed."
	}
}

type ScopeRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
}
type SignInRequest struct {
	Remember      bool   `json:"remember,omitempty"`
	AccessKey     string `json:"accessKey"`
	SecretKey     string `json:"secretKey"`
	SecurityToken string `json:"securityToken,omitempty"`
	ExpiresAt     string `json:"expiresAt,omitempty"`
	Region        string `json:"region"`
}
type Principal struct {
	AccountID        string `json:"accountId"`
	IdentityType     string `json:"identityType"`
	CloudPrincipalID string `json:"cloudPrincipalId"`
	Scope            string `json:"scope"`
}
type PlatformState struct {
	ProfileID     string     `json:"profileId,omitempty"`
	Remembered    bool       `json:"remembered"`
	SignedIn      bool       `json:"signedIn"`
	HasCredential bool       `json:"hasCredential"`
	ExpiresAt     string     `json:"expiresAt,omitempty"`
	Epoch         uint64     `json:"epoch"`
	Principal     *Principal `json:"principal,omitempty"`
}
type RuntimeRequest struct {
	Region      string `json:"region"`
	ProjectName string `json:"projectName"`
	RuntimeID   string `json:"runtimeId"`
}
type RuntimeListRequest struct {
	Region      string `json:"region"`
	ProjectName string `json:"projectName"`
}
type RuntimeSummary struct {
	RuntimeID            string `json:"runtimeId"`
	Name                 string `json:"name"`
	Status               string `json:"status"`
	Region               string `json:"region"`
	ProjectName          string `json:"projectName"`
	CurrentVersionNumber int    `json:"currentVersionNumber"`
	GatewayMode          string `json:"gatewayMode"`
	NetworkType          string `json:"networkType"`
	EndpointHost         string `json:"endpointHost,omitempty"`
	EndpointPath         string `json:"endpointPath,omitempty"`
	HasRuntimeKey        bool   `json:"hasRuntimeKey"`
	AuthorizerMode       string `json:"authorizerMode"`
}
type RuntimeList struct {
	Runtimes  []RuntimeSummary `json:"runtimes"`
	Complete  bool             `json:"complete"`
	NextToken string           `json:"nextToken,omitempty"`
}
type RuntimeVersion struct {
	VersionNumber int    `json:"versionNumber"`
	Status        string `json:"status"`
	Description   string `json:"description,omitempty"`
}
type VersionList struct {
	Versions []RuntimeVersion `json:"versions"`
	Complete bool             `json:"complete"`
}
type IdentityRequest struct {
	ConnectionEpoch        uint64 `json:"connectionEpoch"`
	UserID                 string `json:"userId,omitempty"`
	AuthMode               string `json:"authMode"`
	LegacyJWT              string `json:"legacyJwt,omitempty"`
	TestSubject            string `json:"testSubject,omitempty"`
	VerifiedTestDeployment bool   `json:"verifiedTestDeployment"`
}
type ConnectionState struct {
	Connected           bool   `json:"connected"`
	ConnectionEpoch     uint64 `json:"connectionEpoch"`
	Scope               string `json:"scope"`
	RuntimeID           string `json:"runtimeId"`
	EndpointHost        string `json:"endpointHost"`
	EndpointPath        string `json:"endpointPath"`
	HasCredential       bool   `json:"hasCredential"`
	HasBusinessIdentity bool   `json:"hasBusinessIdentity"`
}
type Capability struct {
	Name   string     `json:"name"`
	State  string     `json:"state"`
	Source string     `json:"source"`
	Error  *SafeError `json:"error,omitempty"`
}
type ConnectionTest struct {
	State        ConnectionState `json:"state"`
	Capabilities []Capability    `json:"capabilities"`
}
type SessionRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SessionID       string `json:"sessionId"`
}
type CreateRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SessionID       string `json:"sessionId,omitempty"`
}
type RenameRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SessionID       string `json:"sessionId"`
	Title           string `json:"title"`
}
type SessionSummary struct {
	ID             string  `json:"id"`
	SessionID      string  `json:"sessionId,omitempty"`
	Title          string  `json:"title"`
	Status         string  `json:"status"`
	LastUpdateTime float64 `json:"lastUpdateTime"`
	CreatedAt      string  `json:"createdAt,omitempty"`
}
type SessionList struct {
	Sessions []SessionSummary `json:"sessions"`
}
type SessionHistory struct {
	ActiveRun *ActiveRun     `json:"activeRun,omitempty"`
	Summary   SessionSummary `json:"summary"`
	Events    []ClientEvent  `json:"events"`
	Complete  bool           `json:"complete"`
}
type DeleteReceipt struct {
	SessionID    string `json:"sessionId"`
	Status       string `json:"status"`
	AccessState  string `json:"accessState,omitempty"`
	CleanupState string `json:"cleanupState,omitempty"`
}
type SubmitRequest struct {
	AttachmentRefs         []string `json:"attachmentRefs,omitempty"`
	ConnectionEpoch        uint64   `json:"connectionEpoch"`
	SessionID              string   `json:"sessionId"`
	OperationID            string   `json:"operationId"`
	IdempotencyKey         string   `json:"idempotencyKey"`
	Content                string   `json:"content"`
	ExecutionConfigVersion string   `json:"executionConfigVersion,omitempty"`
}
type RunReceipt struct {
	SessionID    string `json:"sessionId"`
	InvocationID string `json:"invocationId"`
	TurnID       string `json:"turnId,omitempty"`
	OperationID  string `json:"operationId,omitempty"`
	CreatedAt    string `json:"createdAt,omitempty"`
}
type SubscribeRequest struct {
	ConnectionEpoch   uint64 `json:"connectionEpoch"`
	SubscriptionEpoch uint64 `json:"subscriptionEpoch"`
	SessionID         string `json:"sessionId"`
	InvocationID      string `json:"invocationId"`
	LastEventID       string `json:"lastEventId,omitempty"`
}
type ClientEvent struct {
	SchemaVersion     int        `json:"schemaVersion"`
	ConnectionEpoch   uint64     `json:"connectionEpoch"`
	SubscriptionEpoch uint64     `json:"subscriptionEpoch"`
	SessionID         string     `json:"sessionId"`
	InvocationID      string     `json:"invocationId"`
	EventID           string     `json:"eventId,omitempty"`
	BlockID           string     `json:"blockId,omitempty"`
	Kind              string     `json:"kind"`
	Completeness      string     `json:"completeness"`
	Text              string     `json:"text,omitempty"`
	ToolID            string     `json:"toolId,omitempty"`
	ToolName          string     `json:"toolName,omitempty"`
	Author            string     `json:"author,omitempty"`
	Partial           bool       `json:"partial"`
	Terminal          bool       `json:"terminal"`
	Error             *SafeError `json:"error,omitempty"`
}
type AbortReceipt struct {
	SessionID string `json:"sessionId"`
	Aborted   bool   `json:"aborted"`
}
type ExecutionConfig struct {
	ExecutionConfigVersion string   `json:"executionConfigVersion"`
	Model                  string   `json:"model,omitempty"`
	ProfileBound           bool     `json:"profileBound"`
	ETag                   string   `json:"etag,omitempty"`
	ModelMode              string   `json:"modelMode"`
	MCPMode                string   `json:"mcpMode"`
	MCPServers             []string `json:"mcpServers"`
	InvalidRefsCount       int      `json:"invalidRefsCount"`
}
type ExecutionConfigUpdateRequest struct {
	ConnectionEpoch uint64   `json:"connectionEpoch"`
	SessionID       string   `json:"sessionId"`
	ETag            string   `json:"etag"`
	ModelMode       string   `json:"modelMode"`
	Model           string   `json:"model"`
	MCPMode         string   `json:"mcpMode"`
	MCPServers      []string `json:"mcpServers"`
}
type PendingSummary struct {
	AttachmentRefs         []string `json:"attachmentRefs"`
	InvocationID           string   `json:"invocationId,omitempty"`
	RecoveryDisposition    string   `json:"recoveryDisposition"`
	TerminalVerified       bool     `json:"terminalVerified"`
	SessionID              string   `json:"sessionId"`
	OperationID            string   `json:"operationId"`
	IdempotencyKey         string   `json:"idempotencyKey"`
	ExecutionConfigVersion string   `json:"executionConfigVersion,omitempty"`
	Content                string   `json:"content"`
	State                  string   `json:"state"`
}
type PendingList struct {
	Operations []PendingSummary `json:"operations"`
}
type Empty struct{}

type ChannelAuthRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	AuthMode        string `json:"authMode"`
	ChannelKey      string `json:"channelKey,omitempty"`
	LegacyJWT       string `json:"legacyJwt,omitempty"`
}

// BotProfile pointer lists preserve inheritance (null) and all available ([]).
type BotProfile struct {
	AgentsMD *string   `json:"agentsMd"`
	Model    *string   `json:"model"`
	Skills   *[]string `json:"skills"`
	MCP      *[]string `json:"mcp"`
}
type Bot struct {
	CanConfigure           bool       `json:"canConfigure"`
	ProfileVerified        bool       `json:"profileVerified"`
	Channel                string     `json:"channel,omitempty"`
	AppID                  string     `json:"appId,omitempty"`
	GatewayBotID           string     `json:"gatewayBotId,omitempty"`
	SlotVersion            *int       `json:"slotVersion,omitempty"`
	SessionID              string     `json:"sessionId"`
	DisplayName            string     `json:"displayName"`
	OwnerSubject           string     `json:"ownerSubject"`
	ExecutionConfigVersion int        `json:"executionConfigVersion"`
	ProfileRevision        int        `json:"profileRevision"`
	ConfigDigest           string     `json:"configDigest"`
	Profile                BotProfile `json:"profile"`
	ETag                   string     `json:"etag"`
	Source                 string     `json:"source"`
	MemoryScope            string     `json:"memoryScope"`
}
type BotList struct {
	Items   []Bot `json:"items"`
	Total   int   `json:"total"`
	Offset  int   `json:"offset"`
	Limit   int   `json:"limit"`
	HasMore bool  `json:"hasMore"`
}
type BotListRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	Offset          int    `json:"offset"`
	Limit           int    `json:"limit"`
}
type BotCreateRequest struct {
	ConnectionEpoch uint64     `json:"connectionEpoch"`
	SessionID       string     `json:"sessionId,omitempty"`
	OperationID     string     `json:"operationId"`
	IdempotencyKey  string     `json:"idempotencyKey"`
	DisplayName     string     `json:"displayName"`
	Profile         BotProfile `json:"profile"`
}
type BotUpdateRequest struct {
	ConnectionEpoch       uint64     `json:"connectionEpoch"`
	SessionID             string     `json:"sessionId"`
	OperationID           string     `json:"operationId"`
	IdempotencyKey        string     `json:"idempotencyKey"`
	ExpectedConfigVersion int        `json:"expectedConfigVersion"`
	DisplayName           *string    `json:"displayName,omitempty"`
	Profile               BotProfile `json:"profile"`
}
type BotResult struct {
	OK    bool       `json:"ok"`
	Value *Bot       `json:"value,omitempty"`
	Error *SafeError `json:"error,omitempty"`
}
type BotListResult struct {
	OK    bool       `json:"ok"`
	Value *BotList   `json:"value,omitempty"`
	Error *SafeError `json:"error,omitempty"`
}

type RestoreRequest struct {
	ProfileID string `json:"profileId"`
}
type SavedProfile struct {
	ProfileID   string `json:"profileId"`
	Label       string `json:"label"`
	AccountID   string `json:"accountId"`
	Scope       string `json:"scope"`
	Region      string `json:"region"`
	Remembered  bool   `json:"remembered"`
	RuntimeID   string `json:"runtimeId,omitempty"`
	ProjectName string `json:"projectName,omitempty"`
}
type SavedProfiles struct {
	Profiles []SavedProfile `json:"profiles"`
}
type SavedProfilesResult struct {
	OK    bool           `json:"ok"`
	Value *SavedProfiles `json:"value,omitempty"`
	Error *SafeError     `json:"error,omitempty"`
}

type ActiveRun struct {
	OperationID         string `json:"operationId"`
	InvocationID        string `json:"invocationId,omitempty"`
	State               string `json:"state"`
	Authority           string `json:"authority"`
	RecoveryDisposition string `json:"recoveryDisposition"`
}
type RecoverRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	OperationID     string `json:"operationId"`
	Mode            string `json:"mode"`
}
type ResolvePendingRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	OperationID     string `json:"operationId"`
	Mode            string `json:"mode"`
	Confirm         bool   `json:"confirm"`
}
