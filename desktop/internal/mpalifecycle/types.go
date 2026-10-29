// Package mpalifecycle adapts the bundled Studio lifecycle engine to desktop IPC.
package mpalifecycle

import (
	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type Scope struct {
	PlatformEpoch uint64 `json:"platformEpoch"`
	Region        string `json:"region"`
}
type CreationInput struct {
	RequestID             string `json:"requestId"`
	Name                  string `json:"name"`
	Description           string `json:"description"`
	MaxInstance           int    `json:"maxInstance"`
	Region                string `json:"region"`
	PGHost                string `json:"pgHost,omitempty"`
	PGPort                string `json:"pgPort,omitempty"`
	OpenVikingMode        string `json:"openvikingMode,omitempty"`
	OpenVikingURL         string `json:"openvikingUrl,omitempty"`
	OpenVikingResourceID  string `json:"openvikingResourceId,omitempty"`
	OpenVikingAPIKey      string `json:"openvikingApiKey,omitempty"`
	BusinessWorkspaceMode string `json:"businessWorkspaceMode,omitempty"`
	BusinessDatabaseName  string `json:"businessDatabaseName,omitempty"`
	TOSBucket             string `json:"tosBucket,omitempty"`
}
type CreateRequest struct {
	PlatformEpoch uint64        `json:"platformEpoch"`
	Input         CreationInput `json:"input"`
}
type NameRequest struct {
	PlatformEpoch uint64 `json:"platformEpoch"`
	Region        string `json:"region"`
	RequestID     string `json:"requestId"`
	Name          string `json:"name"`
}
type TaskRequest struct {
	PlatformEpoch uint64 `json:"platformEpoch"`
	Region        string `json:"region"`
	TaskID        string `json:"taskId"`
}
type PreviewRequest struct {
	PlatformEpoch uint64 `json:"platformEpoch"`
	Region        string `json:"region"`
	RuntimeID     string `json:"runtimeId"`
}
type DeleteRequest struct {
	PlatformEpoch uint64 `json:"platformEpoch"`
	Region        string `json:"region"`
	RuntimeID     string `json:"runtimeId"`
	OperationID   string `json:"operationId"`
	Fingerprint   string `json:"fingerprint"`
	Confirm       bool   `json:"confirm"`
}
type DeletionRequest struct {
	PlatformEpoch uint64 `json:"platformEpoch"`
	Region        string `json:"region"`
	OperationID   string `json:"operationId"`
}
type Config struct {
	Configured                bool   `json:"configured"`
	Region                    string `json:"region"`
	PostgresMode              string `json:"postgresMode,omitempty"`
	PGHost                    string `json:"pgHost,omitempty"`
	PGPort                    string `json:"pgPort,omitempty"`
	BusinessWorkspaceMode     string `json:"businessWorkspaceMode,omitempty"`
	BusinessDatabaseName      string `json:"businessDatabaseName,omitempty"`
	OpenVikingMode            string `json:"openvikingMode,omitempty"`
	PostgresMigrationRequired bool   `json:"postgresMigrationRequired,omitempty"`
	RuntimeImage              string `json:"runtimeImage,omitempty"`
	WorkerImage               string `json:"workerImage,omitempty"`
	Error                     string `json:"error,omitempty"`
}
type Diagnostic struct {
	Stage      string `json:"stage"`
	Operation  string `json:"operation"`
	Category   string `json:"category"`
	Attempt    int    `json:"attempt"`
	Outcome    string `json:"outcome"`
	ErrorCode  string `json:"errorCode,omitempty"`
	RequestID  string `json:"requestId,omitempty"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
}
type CreationTask struct {
	TaskID                string      `json:"taskId"`
	RequestID             string      `json:"requestId"`
	Region                string      `json:"region"`
	Name                  string      `json:"name"`
	Description           string      `json:"description"`
	MaxInstance           int         `json:"maxInstance"`
	State                 string      `json:"state"`
	Stage                 string      `json:"stage"`
	Error                 string      `json:"error,omitempty"`
	PGHost                string      `json:"pgHost,omitempty"`
	PGPort                string      `json:"pgPort,omitempty"`
	OpenVikingMode        string      `json:"openvikingMode,omitempty"`
	OpenVikingURL         string      `json:"openvikingUrl,omitempty"`
	OpenVikingResourceID  string      `json:"openvikingResourceId,omitempty"`
	BusinessWorkspaceMode string      `json:"businessWorkspaceMode,omitempty"`
	BusinessDatabaseName  string      `json:"businessDatabaseName,omitempty"`
	TOSBucket             string      `json:"tosBucket,omitempty"`
	LatestDiagnostic      *Diagnostic `json:"latestDiagnostic,omitempty"`
	Result                *struct {
		RuntimeID    string `json:"runtime_id"`
		SkillSpaceID string `json:"skill_space_id"`
		GatewayID    string `json:"gateway_id"`
	} `json:"result,omitempty"`
}
type CreationList struct {
	Tasks []CreationTask `json:"tasks"`
}
type PlanStage struct {
	Stage      string `json:"stage"`
	ResourceID string `json:"resourceId,omitempty"`
	Count      int    `json:"count"`
	Reason     string `json:"reason,omitempty"`
}
type DeletePreview struct {
	RuntimeID     string   `json:"runtimeId"`
	Region        string   `json:"region"`
	AgentID       string   `json:"agentId"`
	Name          string   `json:"name"`
	Fingerprint   string   `json:"fingerprint"`
	CanDelete     bool     `json:"canDelete"`
	Blockers      []string `json:"blockers"`
	SessionCounts struct {
		Total  int `json:"total"`
		Active int `json:"active"`
		Idle   int `json:"idle"`
	} `json:"sessionCounts"`
	CleanupPlan       []PlanStage `json:"cleanupPlan"`
	RetainedResources []PlanStage `json:"retainedResources"`
}
type DeletionTask struct {
	OperationID string  `json:"operationId"`
	RuntimeID   string  `json:"runtimeId"`
	Region      string  `json:"region"`
	Name        string  `json:"name"`
	State       string  `json:"state"`
	Stage       string  `json:"stage"`
	Error       string  `json:"error,omitempty"`
	UpdatedAt   float64 `json:"updatedAt"`
}
type DeletionList struct {
	Tasks []DeletionTask `json:"tasks"`
}
type ConfigResult struct {
	OK    bool           `json:"ok"`
	Value *Config        `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type NameResult struct {
	OK    bool `json:"ok"`
	Value *struct {
		Available bool `json:"available"`
	} `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type CreationResult struct {
	OK    bool           `json:"ok"`
	Value *CreationTask  `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type CreationListResult struct {
	OK    bool           `json:"ok"`
	Value *CreationList  `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type PreviewResult struct {
	OK    bool           `json:"ok"`
	Value *DeletePreview `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type DeletionResult struct {
	OK    bool           `json:"ok"`
	Value *DeletionTask  `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type DeletionListResult struct {
	OK    bool           `json:"ok"`
	Value *DeletionList  `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}

type TOSBucketList struct {
	PlatformEpoch uint64               `json:"platformEpoch"`
	Region        string               `json:"region"`
	Buckets       []cloud.MPATOSBucket `json:"buckets"`
}
type TOSBucketListResult struct {
	OK    bool           `json:"ok"`
	Value *TOSBucketList `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
