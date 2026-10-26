package deployment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"regexp"
	"strings"
	"sync"
	"time"
)

type reviewed struct {
	target Target
	images Images
	at     time.Time
}
type tracked struct {
	target      Target
	operation   Operation
	fingerprint string
	release     bool
	before      Images
}
type Service struct {
	mu             sync.Mutex
	provider       Provider
	target         func() Target
	reviews        map[string]reviewed
	operations     map[string]*tracked
	ledgerPath     string
	ledgerError    bool
	ledgerLoaded   bool
	ledgerResolver func() (string, error)
	ledgerHash     [32]byte
	ledgerExists   bool
}

func New(provider Provider, target func() Target, ledgerPath ...string) *Service {
	s := &Service{provider: provider, target: target, reviews: map[string]reviewed{}, operations: map[string]*tracked{}}
	if len(ledgerPath) > 0 && ledgerPath[0] != "" {
		s.ledgerPath = ledgerPath[0]
	}
	return s
}

// NewWithLedger defers app-path resolution and all filesystem access until an
// actual operation. Binding generation can safely construct this service.
func NewWithLedger(provider Provider, target func() Target, path func() (string, error)) *Service {
	s := New(provider, target)
	s.ledgerResolver = path
	return s
}
func (s *Service) ensureLedger() {
	if s.ledgerLoaded {
		return
	}
	s.ledgerLoaded = true
	if s.ledgerResolver != nil {
		path, e := s.ledgerResolver()
		if e != nil || path == "" {
			s.ledgerError = true
			return
		}
		s.ledgerPath = path
	}
	if s.ledgerPath != "" && s.loadLedger() != nil {
		s.ledgerError = true
		s.operations = map[string]*tracked{}
	}
}

var safeIdentifier = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
var repositoryPart = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|[-]+)[a-z0-9]+)*$`)
var registry = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?(?::[0-9]{1,5})?$`)
var tag = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
var digest = regexp.MustCompile(`^[a-f0-9]{64}$`)

// validImage accepts an OCI reference, never a URL or shell expression.
func validImage(v string) bool {
	if len(v) == 0 || len(v) > 1024 || strings.ContainsAny(v, " \t\r\n?&#;%\\\"'`$(){}[]!|<>=") {
		return false
	}
	name := v
	if strings.Contains(v, "@") {
		parts := strings.Split(v, "@sha256:")
		if len(parts) != 2 || !digest.MatchString(parts[1]) {
			return false
		}
		name = parts[0]
	} else {
		colon := strings.LastIndex(v, ":")
		if colon <= strings.LastIndex(v, "/") || colon < 1 || !tag.MatchString(v[colon+1:]) {
			return false
		}
		name = v[:colon]
	}
	parts := strings.Split(name, "/")
	if len(parts) > 1 && (strings.ContainsAny(parts[0], ".:") || parts[0] == "localhost") {
		if !registry.MatchString(parts[0]) {
			return false
		}
		parts = parts[1:]
	}
	for _, p := range parts {
		if !repositoryPart.MatchString(p) {
			return false
		}
	}
	return len(parts) > 0
}

func failure(code string, status int) *dto.SafeError {
	return dto.Failure(code, "platform", "never", status)
}
func safeError(e *dto.SafeError) *dto.SafeError {
	if e == nil {
		return nil
	}
	code := e.Code
	switch code {
	case "authRequired", "denied", "outcomeUnknown", "staleConnection", "conflict", "invalidInput", "notConnected", "resourceNotVisible", "unavailable", "connectionFailure", "invalidResponse", "requestFailed":
	default:
		code = "requestFailed"
	}
	r := dto.Failure(code, "platform", "never", e.HTTPStatus)
	if code == "outcomeUnknown" {
		r.RetryDisposition = "verifyOnly"
	}
	if code == "conflict" || code == "staleConnection" {
		r.RetryDisposition = "reload"
	}
	if safeIdentifier.MatchString(e.RequestID) {
		r.RequestID = e.RequestID
	}
	if e.CurrentState != nil {
		cs := &dto.CurrentState{Generation: e.CurrentState.Generation}
		if safeIdentifier.MatchString(e.CurrentState.Status) {
			cs.Status = e.CurrentState.Status
		}
		if safeIdentifier.MatchString(e.CurrentState.ExecutionConfigVersion) {
			cs.ExecutionConfigVersion = e.CurrentState.ExecutionConfigVersion
		}
		r.CurrentState = cs
	}
	return r
}
func (s *Service) current(t Target) bool {
	return s.target != nil && s.provider != nil && s.target() == t && s.provider.PlatformEpoch() == t.PlatformEpoch
}
func (s *Service) resolve(epoch uint64) (Target, *dto.SafeError) {
	if s.target == nil || s.provider == nil {
		return Target{}, failure("notConnected", 0)
	}
	t := s.target()
	if t.ConnectionEpoch != epoch || t.PlatformEpoch != s.provider.PlatformEpoch() {
		return Target{}, failure("staleConnection", 0)
	}
	if !safeIdentifier.MatchString(t.RuntimeID) || t.Region == "" || t.ProjectName == "" {
		return Target{}, failure("notConnected", 0)
	}
	return t, nil
}
func (s *Service) read(ctx context.Context, t Target) (Images, *dto.SafeError) {
	r, e := s.provider.GetRuntime(ctx, t.Region, t.ProjectName, t.RuntimeID)
	if e != nil {
		return Images{}, safeError(e)
	}
	if !s.current(t) {
		return Images{}, failure("staleConnection", 0)
	}
	if r.RuntimeID != t.RuntimeID || !safeIdentifier.MatchString(r.ToolID) {
		return Images{}, failure("conflict", 409)
	}
	if t.ToolID != "" && t.ToolID != r.ToolID {
		return Images{}, failure("conflict", 409)
	}
	w, e := s.provider.GetTool(ctx, t.Region, t.ProjectName, r.ToolID)
	if e != nil {
		return Images{}, safeError(e)
	}
	if !s.current(t) {
		return Images{}, failure("staleConnection", 0)
	}
	if w.ToolID != r.ToolID {
		return Images{}, failure("conflict", 409)
	}
	_, timestampError := time.Parse(time.RFC3339Nano, w.UpdatedAt)
	if !validImage(r.Image) || !validImage(w.Image) || !safeIdentifier.MatchString(r.Version) || !safeIdentifier.MatchString(r.Status) || !safeIdentifier.MatchString(w.Status) || timestampError != nil {
		return Images{}, failure("invalidResponse", 0)
	}
	i := Images{ConnectionEpoch: t.ConnectionEpoch, RuntimeID: r.RuntimeID, RuntimeVersion: r.Version, RuntimeImage: r.Image, RuntimeStatus: r.Status, ToolID: w.ToolID, WorkerImage: w.Image, WorkerStatus: w.Status, WorkerUpdatedAt: w.UpdatedAt}
	b, _ := json.Marshal(struct {
		Target Target
		Images Images
	}{t, i})
	h := sha256.Sum256(b)
	i.ReviewFingerprint = hex.EncodeToString(h[:])
	return i, nil
}
func (s *Service) ReadImages(req ScopeRequest) ImagesResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLedger()
	t, e := s.resolve(req.ConnectionEpoch)
	if e != nil {
		return ImagesResult{Error: e}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	i, e := s.read(ctx, t)
	if e != nil {
		return ImagesResult{Error: e}
	}
	now := time.Now()
	for k, r := range s.reviews {
		if now.Sub(r.at) > 10*time.Minute {
			delete(s.reviews, k)
		}
	}
	if _, exists := s.reviews[i.ReviewFingerprint]; !exists && len(s.reviews) >= 256 {
		return ImagesResult{Error: failure("unavailable", 0)}
	}
	s.reviews[i.ReviewFingerprint] = reviewed{t, i, now}
	return ImagesResult{OK: true, Value: &i}
}
func (s *Service) UpdateRuntimeImage(req RuntimeImageRequest) OperationResult {
	return s.update(req.ConnectionEpoch, req.OperationID, req.TargetImage, req.ReviewFingerprint, "runtime", req.ReleaseEnable)
}
func (s *Service) UpdateWorkerImage(req WorkerImageRequest) OperationResult {
	return s.update(req.ConnectionEpoch, req.OperationID, req.TargetImage, req.ReviewFingerprint, "worker", false)
}
func receipt(op Operation) OperationResult {
	copy := op
	return OperationResult{OK: true, Value: &copy}
}

// The preflight detects changes already observed by GET. AgentKit has no verified
// expectedVersion/CAS contract, so another actor can still race GET and Update.
func (s *Service) update(epoch uint64, id, image, fingerprint, kind string, release bool) OperationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLedger()
	t, e := s.resolve(epoch)
	if e != nil {
		return OperationResult{Error: e}
	}
	if !safeIdentifier.MatchString(id) || !validImage(image) {
		return OperationResult{Error: failure("invalidInput", 400)}
	}
	if s.ledgerError || (s.ledgerPath != "" && t.Scope == "") {
		return OperationResult{Error: failure("unavailable", 0)}
	}
	key := operationKey(t, id)
	if old, ok := s.operations[key]; ok {
		if !sameResource(old.target, t) || old.operation.Kind != kind || old.operation.TargetImage != image || old.fingerprint != fingerprint || old.release != release {
			return OperationResult{Error: failure("conflict", 409)}
		}
		copy := old.operation
		copy.ConnectionEpoch = t.ConnectionEpoch
		return receipt(copy)
	}
	for _, old := range s.operations {
		if sameResource(old.target, t) && old.operation.Kind == kind && (old.operation.State == "uncertain" || old.operation.State == "accepted" || (old.operation.State == "pending" && !old.operation.RuntimeReady)) {
			return OperationResult{Error: failure("conflict", 409)}
		}
	}
	r, ok := s.reviews[fingerprint]
	if !ok || r.target != t || time.Since(r.at) > 10*time.Minute {
		return OperationResult{Error: failure("conflict", 409)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	i, e := s.read(ctx, t)
	if e != nil {
		return OperationResult{Error: e}
	}
	if i.ReviewFingerprint != fingerprint {
		return OperationResult{Error: &dto.SafeError{Code: "conflict", Message: dto.SafeMessage("conflict"), Plane: "platform", HTTPStatus: 409, OriginConfidence: "unknown", RetryDisposition: "reload", CurrentState: &dto.CurrentState{ExecutionConfigVersion: i.RuntimeVersion, Status: i.RuntimeStatus}}}
	}
	previous := i.RuntimeImage
	if kind == "worker" {
		previous = i.WorkerImage
	}
	if image == previous {
		return OperationResult{Error: failure("invalidInput", 400)}
	}
	if len(s.operations) >= 1024 {
		return OperationResult{Error: failure("unavailable", 0)}
	}
	op := &tracked{target: t, fingerprint: fingerprint, release: release, before: i, operation: Operation{ConnectionEpoch: epoch, OperationID: id, Kind: kind, RuntimeID: t.RuntimeID, ToolID: i.ToolID, PreviousImage: previous, PreviousVersion: i.RuntimeVersion, TargetImage: image, State: "uncertain"}}
	s.operations[key] = op // Reserve before sending; repeated calls never resend this operation.
	if s.saveLedger() != nil {
		delete(s.operations, key)
		return OperationResult{Error: failure("unavailable", 0)}
	}
	if !s.current(t) {
		delete(s.operations, key)
		s.saveLedger()
		return OperationResult{Error: failure("staleConnection", 0)}
	}
	if kind == "runtime" {
		e = s.provider.UpdateRuntime(ctx, t.Region, UpdateRuntimeInput{RuntimeID: t.RuntimeID, ArtifactURL: image, ReleaseEnable: release})
	} else {
		e = s.provider.UpdateTool(ctx, t.Region, UpdateToolInput{ToolID: i.ToolID, ImageURL: image})
	}
	if e != nil {
		e = safeError(e)
		if e.Code == "denied" || e.Code == "authRequired" || e.Code == "invalidInput" || e.Code == "conflict" || e.Code == "resourceNotVisible" {
			op.operation.State = "failed"
			op.operation.Message = e.Message
			s.saveLedger()
			return OperationResult{Error: e}
		}
		op.operation.Message = "The update outcome is unknown. Read this operation to check configuration; do not submit it again."
		s.saveLedger()
		return receipt(op.operation)
	}
	op.operation.Accepted = true
	op.operation.State = "accepted"
	op.operation.Message = "The platform accepted the image update. Read back its configuration to verify progress."
	if s.saveLedger() != nil {
		op.operation.State = "uncertain"
		op.operation.Message = "The image update was submitted, but its receipt could not be saved. Preserve the original operation and read its configuration."
	}
	if !s.current(t) {
		op.operation.State = "uncertain"
		op.operation.Message = "The connection changed after submission. The original operation is preserved."
		return OperationResult{Error: failure("staleConnection", 0)}
	}
	return receipt(op.operation)
}

func (s *Service) ReadUpdate(req ReadUpdateRequest) OperationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLedger()
	if s.ledgerError {
		return OperationResult{Error: failure("unavailable", 0)}
	}
	t, e := s.resolve(req.ConnectionEpoch)
	if e != nil {
		return OperationResult{Error: e}
	}
	op, ok := s.operations[operationKey(t, req.OperationID)]
	if !ok {
		return OperationResult{Error: failure("resourceNotVisible", 404)}
	}
	if !sameResource(op.target, t) {
		return OperationResult{Error: failure("staleConnection", 0)}
	}
	op.operation.ConnectionEpoch = t.ConnectionEpoch
	if op.operation.State == "failed" {
		return receipt(op.operation)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	op.operation.RuntimeReady = false
	op.operation.WorkerConfigUpdated = false
	defer s.saveLedger()
	i, e := s.read(ctx, t)
	if e != nil {
		op.operation.State = "uncertain"
		op.operation.Message = "Configuration could not be read. Preserve this operation and check again."
		return receipt(op.operation)
	}
	if i.ToolID != op.before.ToolID {
		op.operation.State = "conflict"
		op.operation.Message = "The Runtime now refers to another Worker. Review the current deployment."
		return receipt(op.operation)
	}
	if op.operation.Kind == "worker" {
		if i.WorkerImage == op.operation.TargetImage {
			op.operation.WorkerConfigUpdated = true
			op.operation.State = "pending"
			op.operation.Message = "The Worker image configuration is updated; the platform is still updating. Existing sandboxes have not been verified."
			if strings.EqualFold(i.WorkerStatus, "Ready") {
				op.operation.State = "ready"
				op.operation.Message = "The Worker image configuration is updated and the platform reports Ready. Existing sandboxes have not been verified with this image."
			}
			if strings.EqualFold(i.WorkerStatus, "Failed") || strings.EqualFold(i.WorkerStatus, "Error") {
				op.operation.State = "failed"
				op.operation.Message = "The Worker reports an unsuccessful image update. Existing sandboxes have not been verified."
			}
		} else if i.WorkerImage != op.operation.PreviousImage {
			op.operation.State = "conflict"
			op.operation.Message = "The Worker image changed to another target. Review the current deployment."
		} else {
			op.operation.State = "pending"
			if !op.operation.Accepted {
				op.operation.State = "uncertain"
			}
			op.operation.Message = "The Worker image has not changed yet. Existing sandboxes have not been verified."
		}
	} else {
		op.operation.RuntimeReady = i.RuntimeImage == op.operation.TargetImage && i.RuntimeVersion != op.before.RuntimeVersion && strings.EqualFold(i.RuntimeStatus, "Ready")
		if strings.EqualFold(i.RuntimeStatus, "Failed") || strings.EqualFold(i.RuntimeStatus, "Error") {
			op.operation.State = "failed"
			op.operation.Message = "The Runtime reports an unsuccessful deployment."
		} else if i.RuntimeImage != op.operation.TargetImage && (i.RuntimeImage != op.operation.PreviousImage || i.RuntimeVersion != op.before.RuntimeVersion) {
			op.operation.State = "conflict"
			op.operation.Message = "The Runtime configuration changed to another version. Review the current deployment."
		} else {
			op.operation.State = "pending"
			if !op.operation.Accepted && !op.operation.RuntimeReady {
				op.operation.State = "uncertain"
			}
			op.operation.Message = "The Runtime update is pending."
			if op.operation.RuntimeReady {
				op.operation.Message = "The target Runtime image and a new Ready version are visible. Business health has not been verified."
			}
		}
	}
	return receipt(op.operation)
}
