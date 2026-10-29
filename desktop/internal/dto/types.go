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

// OperationDiagnostic contains only bounded, structured troubleshooting fields.
// Exception messages, response bodies and credentials must never be placed here.
type OperationDiagnostic struct {
	Stage         string `json:"stage"`
	Operation     string `json:"operation"`
	Category      string `json:"category"`
	Attempt       int    `json:"attempt"`
	Outcome       string `json:"outcome"`
	ErrorCode     string `json:"errorCode,omitempty"`
	RequestID     string `json:"requestId,omitempty"`
	HTTPStatus    int    `json:"httpStatus,omitempty"`
	ExceptionType string `json:"exceptionType,omitempty"`
	ResponseType  string `json:"responseType,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Source        string `json:"source,omitempty"`
	ResourceHost  string `json:"resourceHost,omitempty"`
}
type SafeError struct {
	Code             string               `json:"code"`
	Message          string               `json:"message"`
	HTTPStatus       int                  `json:"httpStatus"`
	Plane            string               `json:"plane"`
	OriginConfidence string               `json:"originConfidence"`
	RetryDisposition string               `json:"retryDisposition"`
	RequestID        string               `json:"requestId,omitempty"`
	CurrentState     *CurrentState        `json:"currentState,omitempty"`
	Diagnostic       *OperationDiagnostic `json:"diagnostic,omitempty"`
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
	case "businessIdentityRequired":
		return "This Runtime requires an explicit business user ID (x-user-id). Configure the identity required by this deployment in Settings > Authentication."
	case "businessJWTRequired":
		return "This Runtime requires a business JWT (X-Jwt-Token). Configure a valid JWT in Settings > Authentication."
	case "capabilityUnavailable", "agentProfileUnavailable":
		return "This Runtime does not provide this capability."
	case "collaborationAliasRequired":
		return "Save a unique member alias before using it in a conversation."
	case "conversationRequired":
		return "Choose a conversation first."
	case "resourceNotVisible":
		return "This resource is not available in the current scope."
	case "invalidResponse":
		return "The server returned an invalid response. Reload this view."
	case "notConnected":
		return "Choose and connect a Runtime first."
	case "localConfigUnavailable":
		return "OpenMPA could not save its local settings."
	case "vaultUnavailable":
		return "Saved credentials are unavailable on this build."
	case "vaultMissing":
		return "No saved credentials were found."
	case "vaultWrongPassphrase":
		return "The passphrase did not unlock the saved credentials."
	case "vaultNotSignedIn":
		return "Sign in before saving credentials."
	case "vaultAccountMismatch":
		return "The saved credentials belong to a different account."
	case "agentIdentityUserPoolRequired":
		return "Configure the Agent Identity user pool."
	case "agentIdentityUserPoolMismatch":
		return "Saved Runtime identity uses a different Agent Identity user pool."
	default:
		return "The requested operation could not be completed."
	}
}

type ScopeRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	ForceRefresh    bool   `json:"forceRefresh,omitempty"`
}
type SignInRequest struct {
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
	SignedIn      bool       `json:"signedIn"`
	HasCredential bool       `json:"hasCredential"`
	ExpiresAt     string     `json:"expiresAt,omitempty"`
	Region        string     `json:"region,omitempty"`
	Epoch         uint64     `json:"epoch"`
	Principal     *Principal `json:"principal,omitempty"`
}
type RegionSummary struct {
	Region       string `json:"region"`
	DisplayName  string `json:"displayName"`
	EndpointHost string `json:"endpointHost"`
}
type RegionList struct {
	Regions []RegionSummary `json:"regions"`
}
type VaultStatus struct {
	Exists    bool   `json:"exists"`
	AccountID string `json:"accountId,omitempty"`
	Region    string `json:"region,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}
type VaultPassphraseRequest struct {
	Passphrase string `json:"passphrase"`
}
type RuntimeRequest struct {
	Region      string `json:"region"`
	ProjectName string `json:"projectName"`
	RuntimeID   string `json:"runtimeId"`
}
type RuntimeListRequest struct {
	Region       string `json:"region"`
	ProjectName  string `json:"projectName"`
	ForceRefresh bool   `json:"forceRefresh,omitempty"`
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
	RuntimeName         string `json:"runtimeName,omitempty"`
	WorkerToolID        string `json:"workerToolId,omitempty"`
	RuntimeImage        string `json:"runtimeImage,omitempty"`
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
	MPAInstanceID   string `json:"mpaInstanceId,omitempty"`
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
	MPAInstanceID  string  `json:"mpaInstanceId,omitempty"`
	TraceEnabled   bool    `json:"traceEnabled,omitempty"`
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
	AssistantID            string   `json:"assistantId,omitempty"`
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
type SafeCommandParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type SafeCommandSummary struct {
	Binary         string                 `json:"binary,omitempty"`
	CommandPreview string                 `json:"commandPreview,omitempty"`
	ArgumentCount  *int                   `json:"argumentCount,omitempty"`
	Redacted       *bool                  `json:"redacted,omitempty"`
	Parameters     []SafeCommandParameter `json:"parameters,omitempty"`
}
type ClientEvent struct {
	SchemaVersion       int                 `json:"schemaVersion"`
	ConnectionEpoch     uint64              `json:"connectionEpoch"`
	SubscriptionEpoch   uint64              `json:"subscriptionEpoch"`
	SessionID           string              `json:"sessionId"`
	InvocationID        string              `json:"invocationId"`
	EventID             string              `json:"eventId,omitempty"`
	BlockID             string              `json:"blockId,omitempty"`
	PartIndex           *int                `json:"partIndex,omitempty"`
	Kind                string              `json:"kind"`
	SourceType          string              `json:"sourceType,omitempty"`
	Completeness        string              `json:"completeness"`
	Text                string              `json:"text,omitempty"`
	ReasoningVisibility string              `json:"reasoningVisibility,omitempty"`
	ToolID              string              `json:"toolId,omitempty"`
	ToolName            string              `json:"toolName,omitempty"`
	ToolStatus          string              `json:"toolStatus,omitempty"`
	ToolSubject         string              `json:"toolSubject,omitempty"`
	Author              string              `json:"author,omitempty"`
	StartedAt           string              `json:"startedAt,omitempty"`
	EndedAt             string              `json:"endedAt,omitempty"`
	DurationMS          *float64            `json:"durationMs,omitempty"`
	SafeCommandSummary  *SafeCommandSummary `json:"safeCommandSummary,omitempty"`
	OutputPreview       string              `json:"outputPreview,omitempty"`
	StderrPreview       string              `json:"stderrPreview,omitempty"`
	Partial             bool                `json:"partial"`
	Terminal            bool                `json:"terminal"`
	Error               *SafeError          `json:"error,omitempty"`
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
type ModelCatalogItem struct {
	ID string `json:"id"`
}
type ModelCatalog struct {
	Models []ModelCatalogItem `json:"models"`
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

type AssistantSkill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"`
	SpaceID     string `json:"spaceId,omitempty"`
	Content     string `json:"content,omitempty"`
}
type AssistantConfig struct {
	AssistantID string           `json:"assistantId"`
	AgentsMD    string           `json:"agentsMd"`
	Skills      []AssistantSkill `json:"skills"`
	UpdatedAt   string           `json:"updatedAt,omitempty"`
}
type AssistantConfigRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SessionID       string `json:"sessionId"`
	AssistantID     string `json:"assistantId,omitempty"`
}
type AssistantConfigSaveRequest struct {
	ConnectionEpoch uint64           `json:"connectionEpoch"`
	SessionID       string           `json:"sessionId"`
	AssistantID     string           `json:"assistantId,omitempty"`
	AgentsMD        string           `json:"agentsMd"`
	Skills          []AssistantSkill `json:"skills"`
}
type SkillCatalogRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	Query           string `json:"query,omitempty"`
}
type SkillCatalog struct {
	Skills []AssistantSkill `json:"skills"`
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
