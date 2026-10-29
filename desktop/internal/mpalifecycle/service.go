package mpalifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type Service struct {
	cloud         *cloud.Client
	notifyDeleted func(uint64, string, string)
	monitors      map[string]bool
	directory     func() (string, error)
	archive       func() (string, error)
	mu            sync.Mutex
	process       *engineProcess
	epoch         uint64
	closed        chan struct{}
	once          sync.Once
	workers       sync.WaitGroup
}

func New(platform *cloud.Client, directory, archive func() (string, error), onDeleted func(uint64, string, string)) *Service {
	s := &Service{cloud: platform, directory: directory, archive: archive, closed: make(chan struct{}), notifyDeleted: onDeleted, monitors: map[string]bool{}}
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.closed:
				return
			case <-ticker.C:
				s.mu.Lock()
				old := s.process
				if old != nil && s.cloud.Epoch() != s.epoch {
					s.process = nil
				} else {
					old = nil
				}
				s.mu.Unlock()
				if old != nil {
					old.close()
				}
			}
		}
	}()
	return s
}
func Shutdown(s *Service) {
	s.once.Do(func() {
		close(s.closed)
		s.mu.Lock()
		process := s.process
		s.process = nil
		s.mu.Unlock()
		if process != nil {
			process.close()
		}
		s.workers.Wait()
	})
}
func (s *Service) engine(ctx context.Context, epoch uint64) (*engineProcess, *dto.SafeError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.closed:
		return nil, engineFailure()
	default:
	}
	credential, state, ok := s.cloud.Credential()
	if !ok || state.Principal == nil {
		return nil, dto.Failure("authRequired", "platform", "reauthenticate", 0)
	}
	if state.Epoch != epoch {
		return nil, dto.Failure("staleConnection", "platform", "reload", 0)
	}
	if credential.ExpiresAt != "" {
		expiry, err := time.Parse(time.RFC3339, credential.ExpiresAt)
		if err != nil || !expiry.After(time.Now()) {
			return nil, dto.Failure("authRequired", "platform", "reauthenticate", 0)
		}
	}
	if s.process != nil && s.epoch == epoch {
		select {
		case <-s.process.done:
			s.process = nil
		default:
			return s.process, nil
		}
	}
	if s.process != nil {
		s.process.close()
		s.process = nil
	}
	root, err := s.directory()
	if err != nil {
		return nil, engineFailure()
	}
	archive, err := s.archive()
	if err != nil {
		return nil, engineFailure()
	}
	python, script, err := prepareArchive(archive, filepath.Join(root, "engines"))
	if err != nil {
		return nil, engineFailure()
	}
	scope := sha256.Sum256([]byte(state.Principal.Scope))
	owner := "openmpa-" + hex.EncodeToString(scope[:])
	accountPath := filepath.Join(root, "accounts", hex.EncodeToString(scope[:]))
	if err = os.MkdirAll(accountPath, 0700); err != nil {
		return nil, engineFailure()
	}
	process, failure := launchEngine(ctx, python, script, map[string]any{"account": state.Principal.AccountID, "owner": owner, "directory": accountPath, "credentials": []string{credential.AccessKey, credential.SecretKey, credential.Token}})
	if failure != nil {
		return nil, failure
	}
	if s.cloud.Epoch() != epoch {
		process.close()
		return nil, dto.Failure("staleConnection", "platform", "reload", 0)
	}
	s.process, s.epoch = process, epoch
	return process, nil
}
func (s *Service) call(ctx context.Context, epoch uint64, region, operation string, body, output any) *dto.SafeError {
	if !cloud.ValidRegion(region) {
		return dto.Failure("invalidInput", "desktop", "never", 0)
	}
	raw, err := json.Marshal(body)
	if err != nil || len(raw) > 16384 {
		return dto.Failure("invalidInput", "desktop", "never", 0)
	}
	bootstrap, stop := context.WithTimeout(ctx, 5*time.Minute)
	process, failure := s.engine(bootstrap, epoch)
	stop()
	if failure != nil {
		return failure
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	failure = process.call(ctx, operation, body, output)
	if s.cloud.Epoch() != epoch {
		return dto.Failure("staleConnection", "platform", "reload", 0)
	}
	return failure
}
func only[T any](value *T, err *dto.SafeError) *T {
	if err != nil {
		return nil
	}
	return value
}

// ListTOSBuckets is a read-only native SDK call and does not start the Python engine.
func (s *Service) ListTOSBuckets(ctx context.Context, r Scope) TOSBucketListResult {
	buckets, failure := s.cloud.ListMPATOSBuckets(ctx, r.PlatformEpoch, r.Region)
	if failure != nil {
		return TOSBucketListResult{Error: failure}
	}
	return TOSBucketListResult{OK: true, Value: &TOSBucketList{PlatformEpoch: r.PlatformEpoch, Region: r.Region, Buckets: buckets}}
}
func (s *Service) Config(ctx context.Context, r Scope) ConfigResult {
	var out Config
	err := s.call(ctx, r.PlatformEpoch, r.Region, "config", map[string]any{"region": r.Region}, &out)
	return ConfigResult{err == nil, only(&out, err), err}
}
func (s *Service) ListCreations(ctx context.Context, r Scope) CreationListResult {
	var out CreationList
	err := s.call(ctx, r.PlatformEpoch, r.Region, "listCreations", map[string]any{"region": r.Region}, &out)
	return CreationListResult{err == nil, only(&out, err), err}
}
func (s *Service) CheckName(ctx context.Context, r NameRequest) NameResult {
	var out struct {
		Available bool `json:"available"`
	}
	err := s.call(ctx, r.PlatformEpoch, r.Region, "checkName", map[string]any{"region": r.Region, "requestId": r.RequestID, "name": r.Name}, &out)
	return NameResult{err == nil, only(&out, err), err}
}
func (s *Service) Create(ctx context.Context, r CreateRequest) CreationResult {
	var out CreationTask
	err := s.call(ctx, r.PlatformEpoch, r.Input.Region, "create", r.Input, &out)
	return CreationResult{err == nil, only(&out, err), err}
}
func (s *Service) ReadCreation(ctx context.Context, r TaskRequest) CreationResult {
	var out CreationTask
	err := s.call(ctx, r.PlatformEpoch, r.Region, "readCreation", map[string]any{"region": r.Region, "taskId": r.TaskID}, &out)
	return CreationResult{err == nil, only(&out, err), err}
}
func (s *Service) CancelCreation(ctx context.Context, r TaskRequest) CreationResult {
	var out CreationTask
	err := s.call(ctx, r.PlatformEpoch, r.Region, "cancelCreation", map[string]any{"region": r.Region, "taskId": r.TaskID}, &out)
	return CreationResult{err == nil, only(&out, err), err}
}
func (s *Service) PreviewDelete(ctx context.Context, r PreviewRequest) PreviewResult {
	var out DeletePreview
	err := s.call(ctx, r.PlatformEpoch, r.Region, "previewDelete", map[string]any{"region": r.Region, "runtimeId": r.RuntimeID}, &out)
	return PreviewResult{err == nil, only(&out, err), err}
}

var fingerprintPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (s *Service) Delete(ctx context.Context, r DeleteRequest) DeletionResult {
	if !r.Confirm || !fingerprintPattern.MatchString(r.Fingerprint) || strings.TrimSpace(r.RuntimeID) == "" {
		return DeletionResult{Error: dto.Failure("invalidInput", "desktop", "never", 0)}
	}
	var out DeletionTask
	err := s.call(ctx, r.PlatformEpoch, r.Region, "delete", map[string]any{"region": r.Region, "runtimeId": r.RuntimeID, "operationId": r.OperationID, "fingerprint": r.Fingerprint, "confirm": true}, &out)
	if err == nil && out.State == "running" {
		s.monitorDeletion(r.PlatformEpoch, r.Region, out.OperationID)
	}
	if err == nil && out.State == "succeeded" && s.notifyDeleted != nil {
		s.notifyDeleted(r.PlatformEpoch, r.Region, out.RuntimeID)
	}
	return DeletionResult{err == nil, only(&out, err), err}
}
func (s *Service) ReadDeletion(ctx context.Context, r DeletionRequest) DeletionResult {
	var out DeletionTask
	err := s.call(ctx, r.PlatformEpoch, r.Region, "readDeletion", map[string]any{"region": r.Region, "operationId": r.OperationID}, &out)
	if err == nil && out.State == "succeeded" && s.notifyDeleted != nil {
		s.notifyDeleted(r.PlatformEpoch, r.Region, out.RuntimeID)
	}
	return DeletionResult{err == nil, only(&out, err), err}
}
func (s *Service) ListDeletions(ctx context.Context, r Scope) DeletionListResult {
	var out DeletionList
	err := s.call(ctx, r.PlatformEpoch, r.Region, "listDeletions", map[string]any{"region": r.Region}, &out)
	return DeletionListResult{err == nil, only(&out, err), err}
}

// Accepted deletion continues independently of any WebView or dialog. Only
// read-only status checks run here; an uncertain write is never resubmitted.
func (s *Service) monitorDeletion(epoch uint64, region, id string) {
	key := fmt.Sprintf("%d:%s", epoch, id)
	s.mu.Lock()
	select {
	case <-s.closed:
		s.mu.Unlock()
		return
	default:
	}
	if s.monitors[key] {
		s.mu.Unlock()
		return
	}
	s.monitors[key] = true
	s.workers.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.workers.Done()
		defer func() { s.mu.Lock(); delete(s.monitors, key); s.mu.Unlock() }()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		deadline := time.NewTimer(5 * time.Minute)
		defer deadline.Stop()
		for {
			select {
			case <-s.closed:
				return
			case <-deadline.C:
				return
			case <-ticker.C:
				if s.cloud.Epoch() != epoch {
					return
				}
				value := s.ReadDeletion(context.Background(), DeletionRequest{PlatformEpoch: epoch, Region: region, OperationID: id})
				if value.OK && value.Value.State != "running" {
					return
				}
			}
		}
	}()
}
