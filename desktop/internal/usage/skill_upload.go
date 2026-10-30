package usage

import (
	"context"
	"encoding/base64"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"strings"
	"sync"
	"time"
)

type SkillUploadRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	OperationID     string `json:"operationId"`
	SpaceID         string `json:"spaceId"`
	Filename        string `json:"filename"`
	ArchiveBase64   string `json:"archiveBase64"`
}
type SkillUploadReadRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	OperationID     string `json:"operationId"`
}
type SkillUploadView struct {
	OperationID string `json:"operationId"`
	SpaceID     string `json:"spaceId"`
	Name        string `json:"name"`
	SkillID     string `json:"skillId"`
	Version     string `json:"version"`
	Status      string `json:"status"`
}
type SkillUploadResult struct {
	OK    bool             `json:"ok"`
	Value *SkillUploadView `json:"value,omitempty"`
	Error *dto.SafeError   `json:"error,omitempty"`
}
type skillUploadOperation struct {
	View                 SkillUploadView
	Epoch, PlatformEpoch uint64
	Scope                string
	busy                 bool
	created              bool
	publishStarted       bool
}
type skillUploadState struct {
	mu         sync.Mutex
	operations map[string]*skillUploadOperation
}

func (s *Service) UploadSkill(ctx context.Context, r SkillUploadRequest) (SkillUploadView, *dto.SafeError) {
	empty := SkillUploadView{}
	if !resourceIdentifierValid(r.OperationID) || len(r.ArchiveBase64) > base64.StdEncoding.EncodedLen(maxSkillArchive) {
		return empty, resourceInputError()
	}
	data, e := base64.StdEncoding.DecodeString(r.ArchiveBase64)
	if e != nil {
		return empty, resourceInputError()
	}
	archive, e := validateSkillArchive(r.Filename, data)
	if e != nil {
		return empty, dto.Failure("skillArchiveInvalid", "desktop", "never", 0)
	}
	space, err := s.boundSkillSpace(ctx, r.ConnectionEpoch, r.SpaceID)
	if err != nil {
		return empty, err
	}
	target, err := s.cloudTarget(r.ConnectionEpoch)
	if err != nil {
		return empty, err
	}
	scope, scopeErr := s.conn.ScopeContext(r.ConnectionEpoch)
	if scopeErr != nil {
		return empty, scopeErr
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(scope, cancel)
	defer func() { stop(); cancel() }()

	state := s.skillUploads
	state.mu.Lock()
	if len(state.operations) >= 128 {
		state.mu.Unlock()
		return empty, dto.Failure("operationLimit", "desktop", "never", 0)
	}
	if _, exists := state.operations[r.OperationID]; exists {
		state.mu.Unlock()
		return empty, dto.Failure("conflict", "desktop", "reload", 409)
	}
	for _, op := range state.operations {
		if op.Scope == s.conn.State().Scope && op.View.SpaceID == r.SpaceID && op.View.Name == archive.Name && op.View.Status != "failed" {
			state.mu.Unlock()
			return empty, dto.Failure("conflict", "desktop", "reload", 409)
		}
	}
	op := &skillUploadOperation{View: SkillUploadView{OperationID: r.OperationID, SpaceID: r.SpaceID, Name: archive.Name, Status: "uploading"}, Epoch: r.ConnectionEpoch, PlatformEpoch: target.PlatformEpoch, Scope: s.conn.State().Scope, busy: true}
	state.operations[r.OperationID] = op
	state.mu.Unlock()
	finish := func(status string, err *dto.SafeError) (SkillUploadView, *dto.SafeError) {
		state.mu.Lock()
		defer state.mu.Unlock()
		op.View.Status = status
		op.busy = false
		return op.View, err
	}
	// Check every published page before a write; never overwrite an existing skill.
	for page := 1; page <= 100; page++ {
		directory, err := s.WorkerSkills(ctx, r.ConnectionEpoch, target.Region, r.SpaceID, page, 100)
		if err != nil {
			return finish("failed", err)
		}
		for _, skill := range directory.Items {
			if strings.EqualFold(skill.Name, archive.Name) {
				return finish("failed", dto.Failure("skillNameConflict", "platform", "never", 409))
			}
		}
		if !directory.HasMore {
			break
		}
		if page == 100 {
			return finish("failed", resourceResponseError(false))
		}
	}
	if s.putSkillArchive == nil {
		return finish("failed", dto.Failure("unavailable", "platform", "readOnly", 0))
	}
	object, err := s.putSkillArchive(ctx, target.Region, space.ProjectName, archive.Data)
	if err != nil {
		return finish("failed", err)
	}
	if err = s.resourceEpochError(r.ConnectionEpoch); err != nil {
		return finish("failed", err)
	}
	if s.cloud.Epoch() != op.PlatformEpoch {
		return finish("failed", dto.Failure("staleConnection", "platform", "reload", 0))
	}
	var created struct {
		ID string `json:"Id"`
	}
	err = s.cloudAction(ctx, r.ConnectionEpoch, "CreateSkill", map[string]any{"Name": archive.Name, "Description": archive.Description, "TosUrl": object.URL, "BucketName": object.Bucket, "SkillSpaces": []string{r.SpaceID}, "ProjectName": space.ProjectName}, &created, true)
	if err != nil {
		status := "failed"
		if err.Code == "outcomeUnknown" || err.Code == "staleConnection" {
			status = "unknown"
		}
		return finish(status, err)
	}
	if !resourceIdentifierValid(created.ID) {
		return finish("unknown", dto.Failure("outcomeUnknown", "platform", "verifyOnly", 200))
	}
	state.mu.Lock()
	op.View.SkillID = created.ID
	op.View.Status = "processing"
	op.created = true
	op.busy = false
	result := op.View
	state.mu.Unlock()
	return result, nil
}
func (s *Service) ReadSkillUpload(ctx context.Context, r SkillUploadReadRequest) (SkillUploadView, *dto.SafeError) {
	empty := SkillUploadView{}
	if _, err := s.cloudTarget(r.ConnectionEpoch); err != nil {
		return empty, err
	}
	state := s.skillUploads
	state.mu.Lock()
	op := state.operations[r.OperationID]
	if op == nil || op.Epoch != r.ConnectionEpoch || op.PlatformEpoch != s.cloud.Epoch() || op.Scope != s.conn.State().Scope {
		state.mu.Unlock()
		return empty, dto.Failure("staleConnection", "desktop", "reload", 0)
	}
	if op.busy || !op.created || op.View.Status == "published" || op.View.Status == "failed" {
		out := op.View
		state.mu.Unlock()
		return out, nil
	}
	op.busy = true
	snapshot := *op
	state.mu.Unlock()
	finish := func(status string, err *dto.SafeError) (SkillUploadView, *dto.SafeError) {
		state.mu.Lock()
		defer state.mu.Unlock()
		op.View.Status = status
		op.busy = false
		return op.View, err
	}
	if snapshot.publishStarted {
		// A lost publish receipt is resolved by reading the authoritative relation.
		target, err := s.cloudTarget(r.ConnectionEpoch)
		if err != nil {
			return finish("unknown", err)
		}
		for page := 1; page <= 100; page++ {
			directory, err := s.WorkerSkills(ctx, r.ConnectionEpoch, target.Region, op.View.SpaceID, page, 100)
			if err != nil {
				return finish("unknown", err)
			}
			for _, skill := range directory.Items {
				if skill.ID == snapshot.View.SkillID && skill.Version == snapshot.View.Version {
					return finish("published", nil)
				}
			}
			if !directory.HasMore {
				break
			}
		}
		return finish("unknown", nil)
	}
	var versions struct {
		Items []struct {
			Status  string `json:"Status"`
			Version string `json:"Version"`
		} `json:"Items"`
	}
	if err := s.cloudAction(ctx, r.ConnectionEpoch, "ListSkillVersions", map[string]any{"Id": snapshot.View.SkillID, "PageNumber": 1, "PageSize": 50}, &versions, false); err != nil {
		return finish("processing", err)
	}
	if len(versions.Items) == 0 {
		return finish("processing", nil)
	}
	version := ""
	for _, item := range versions.Items {
		if strings.EqualFold(item.Status, "running") && item.Version != "" {
			version = item.Version
			break
		}
	}
	if version == "" {
		for _, item := range versions.Items {
			if strings.EqualFold(item.Status, "failed") {
				return finish("failed", dto.Failure("skillProcessingFailed", "platform", "readOnly", 0))
			}
		}
		return finish("processing", nil)
	}
	if _, err := s.boundSkillSpace(ctx, r.ConnectionEpoch, snapshot.View.SpaceID); err != nil {
		return finish("processing", err)
	}
	state.mu.Lock()
	op.publishStarted = true
	op.View.Version = version
	state.mu.Unlock()
	err := s.cloudAction(ctx, r.ConnectionEpoch, "PublishSkillToSkillSpace", map[string]any{"SkillSpaces": []string{snapshot.View.SpaceID}, "Skills": []map[string]string{{"SkillId": snapshot.View.SkillID, "Version": version}}}, nil, true)
	if err != nil {
		if err.HTTPStatus == 400 || err.HTTPStatus == 401 || err.HTTPStatus == 403 || err.HTTPStatus == 422 {
			state.mu.Lock()
			op.publishStarted = false
			state.mu.Unlock()
			return finish("processing", err)
		}
		return finish("unknown", err)
	}
	return finish("published", nil)
}
func (b *Bridge) UploadSkill(ctx context.Context, r SkillUploadRequest) SkillUploadResult {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	out, err := b.core.UploadSkill(ctx, r)
	return SkillUploadResult{err == nil, only(&out, err), err}
}
func (b *Bridge) ReadSkillUpload(ctx context.Context, r SkillUploadReadRequest) SkillUploadResult {
	ctx, cancel := deadline(ctx)
	defer cancel()
	out, err := b.core.ReadSkillUpload(ctx, r)
	return SkillUploadResult{err == nil, only(&out, err), err}
}
