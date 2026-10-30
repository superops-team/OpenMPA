// Package deployment exposes fixed, typed image operations. Cloud credentials and
// complete cloud responses never enter these DTOs.
package deployment

import (
	"context"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

// Snapshot types are the deliberately narrow projection performed by the cloud adapter.
type RuntimeSnapshot struct{ RuntimeID, Version, Image, Status, ToolID string }
type ToolSnapshot struct{ ToolID, Image, Status, UpdatedAt string }
type UpdateRuntimeInput struct {
	RuntimeID     string `json:"RuntimeId"`
	ArtifactURL   string `json:"ArtifactUrl"`
	ReleaseEnable bool   `json:"ReleaseEnable"`
}
type UpdateToolInput struct {
	ToolID   string `json:"ToolId"`
	ImageURL string `json:"ImageUrl"`
}
type Provider interface {
	PlatformEpoch() uint64
	GetRuntime(context.Context, string, string, string) (RuntimeSnapshot, *dto.SafeError)
	GetTool(context.Context, string, string, string) (ToolSnapshot, *dto.SafeError)
	UpdateRuntime(context.Context, string, UpdateRuntimeInput) *dto.SafeError
	UpdateTool(context.Context, string, UpdateToolInput) *dto.SafeError
}

// Target comes from the authoritative platform/connection binding, never renderer IDs.
type Target struct {
	Scope, Region, ProjectName, RuntimeID string
	CredentialProfileID, ToolID           string
	ConnectionEpoch, PlatformEpoch        uint64
}
type ScopeRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
}
type RuntimeImageRequest struct {
	ConnectionEpoch   uint64 `json:"connectionEpoch"`
	OperationID       string `json:"operationId"`
	TargetImage       string `json:"targetImage"`
	ReviewFingerprint string `json:"reviewFingerprint"`
	ReleaseEnable     bool   `json:"releaseEnable"`
}
type WorkerImageRequest struct {
	ConnectionEpoch   uint64 `json:"connectionEpoch"`
	OperationID       string `json:"operationId"`
	TargetImage       string `json:"targetImage"`
	ReviewFingerprint string `json:"reviewFingerprint"`
}
type ReadUpdateRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	OperationID     string `json:"operationId"`
}
type Images struct {
	ConnectionEpoch   uint64 `json:"connectionEpoch"`
	RuntimeID         string `json:"runtimeId"`
	RuntimeVersion    string `json:"runtimeVersion"`
	RuntimeImage      string `json:"runtimeImage"`
	RuntimeStatus     string `json:"runtimeStatus"`
	ToolID            string `json:"toolId"`
	WorkerImage       string `json:"workerImage"`
	WorkerStatus      string `json:"workerStatus"`
	WorkerUpdatedAt   string `json:"workerUpdatedAt"`
	ReviewFingerprint string `json:"reviewFingerprint"`
}
type Operation struct {
	ConnectionEpoch           uint64 `json:"connectionEpoch"`
	OperationID               string `json:"operationId"`
	Kind                      string `json:"kind"`
	RuntimeID                 string `json:"runtimeId"`
	ToolID                    string `json:"toolId"`
	PreviousImage             string `json:"previousImage"`
	PreviousVersion           string `json:"previousVersion"`
	TargetImage               string `json:"targetImage"`
	Accepted                  bool   `json:"accepted"`
	State                     string `json:"state"`
	RuntimeReady              bool   `json:"runtimeReady"`
	BusinessHealthVerified    bool   `json:"businessHealthVerified"`
	WorkerConfigUpdated       bool   `json:"workerConfigUpdated"`
	ExistingSandboxesVerified bool   `json:"existingSandboxesVerified"`
	Message                   string `json:"message"`
}
type ImagesResult struct {
	OK    bool           `json:"ok"`
	Value *Images        `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type OperationResult struct {
	OK    bool           `json:"ok"`
	Value *Operation     `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type OperationList struct {
	ConnectionEpoch uint64      `json:"connectionEpoch"`
	Operations      []Operation `json:"operations"`
}
type OperationListResult struct {
	OK    bool           `json:"ok"`
	Value *OperationList `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
