package session

import (
	"context"
	"encoding/json"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"io"
	"net/http"
	"strconv"
	"time"
)

func (s *Service) durableBot(ctx context.Context, epoch uint64, id, version string) bool {
	v, e := strconv.Atoi(version)
	if e != nil || v < 1 {
		return false
	}
	out := s.ExecutionConfig(ctx, dto.SessionRequest{ConnectionEpoch: epoch, SessionID: id})
	return out.OK && out.Value.ProfileBound && out.Value.ETag != "" && out.Value.ExecutionConfigVersion == strconv.Itoa(v)
}
func (s *Service) pendingOperation(epoch uint64, id string) (Pending, *dto.SafeError) {
	if !s.conn.Current(epoch) {
		return Pending{}, dto.Failure("staleConnection", "transport", "reload", 0)
	}
	list, e := s.pending.List(s.conn.State().Scope)
	if e != nil {
		return Pending{}, dto.Failure("pendingPersistenceFailed", "transport", "never", 0)
	}
	for _, p := range list {
		if p.OperationID == id {
			return p, nil
		}
	}
	return Pending{}, dto.Failure("pendingNotFound", "transport", "never", 0)
}
func (s *Service) Recover(ctx context.Context, r dto.RecoverRequest) dto.RunReceiptResult {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	p, e := s.pendingOperation(r.ConnectionEpoch, r.OperationID)
	if e != nil {
		return dto.RunReceiptResult{Error: e}
	}
	if p.InvocationID != "" {
		if r.Mode != "subscribeAccepted" && r.Mode != "retryDurable" {
			return dto.RunReceiptResult{Error: dto.Failure("invalidInput", "transport", "never", 0)}
		}
		return dto.RunReceiptResult{OK: true, Value: &dto.RunReceipt{SessionID: p.SessionID, InvocationID: p.InvocationID, OperationID: p.OperationID}}
	}
	if r.Mode != "retryDurable" || len(p.AttachmentRefs) != 0 || !p.DurableBot || !s.durableBot(ctx, r.ConnectionEpoch, p.SessionID, p.ConfigVersion) {
		return dto.RunReceiptResult{Error: dto.Failure("outcomeUnknown", "transport", "verifyOnly", 0)}
	}
	version, err := strconv.Atoi(p.ConfigVersion)
	if err != nil {
		return dto.RunReceiptResult{Error: dto.Failure("invalidInput", "transport", "never", 0)}
	}
	data, _ := json.Marshal(struct {
		Content string `json:"content"`
		Version int    `json:"executionConfigVersion"`
	}{p.Content, version})
	headers := http.Header{}
	headers.Set("Idempotency-Key", p.IdempotencyKey)
	resp, e := s.conn.Request(ctx, r.ConnectionEpoch, "POST", path(p.SessionID, "/run"), data, headers)
	var out dto.RunReceipt
	if e == nil {
		defer resp.Body.Close()
		if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil || out.InvocationID == "" {
			e = dto.Failure("outcomeUnknown", "runtimeIngress", "verifyOnly", 0)
		}
	}
	if e == nil {
		if err = s.pending.Update(p.Scope, p.OperationID, func(p *Pending) error { p.State = "accepted"; p.InvocationID = out.InvocationID; return nil }); err != nil {
			e = dto.Failure("pendingPersistenceFailed", "transport", "verifyOnly", 0)
		}
	}
	return dto.RunReceiptResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (s *Service) ResolvePending(ctx context.Context, r dto.ResolvePendingRequest) dto.EmptyResult {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	p, e := s.pendingOperation(r.ConnectionEpoch, r.OperationID)
	if e != nil {
		return dto.EmptyResult{Error: e}
	}
	if !r.Confirm {
		return dto.EmptyResult{Error: dto.Failure("reviewRequired", "transport", "never", 0)}
	}
	switch r.Mode {
	case "verifiedTerminal":
		if !p.TerminalVerified {
			return dto.EmptyResult{Error: dto.Failure("terminalUnverified", "transport", "history", 0)}
		}
	case "userReviewed":
		if p.InvocationID != "" {
			return dto.EmptyResult{Error: dto.Failure("terminalUnverified", "transport", "history", 0)}
		}
	default:
		return dto.EmptyResult{Error: dto.Failure("invalidInput", "transport", "never", 0)}
	}
	if err := s.pending.Delete(p.Scope, p.OperationID); err != nil {
		return dto.EmptyResult{Error: dto.Failure("pendingPersistenceFailed", "transport", "never", 0)}
	}
	return dto.EmptyResult{OK: true, Value: &dto.Empty{}}
}
func (s *Service) observeTerminal(ctx context.Context, r dto.SubscribeRequest) {
	scope := s.conn.State().Scope
	list, e := s.pending.List(scope)
	if e != nil {
		return
	}
	for _, p := range list {
		if p.SessionID != r.SessionID || p.InvocationID != r.InvocationID || p.InvocationID == "" {
			continue
		}
		_ = s.pending.Update(scope, p.OperationID, func(p *Pending) error { p.State = "terminalObserved"; return nil })
		var history struct {
			Events []json.RawMessage `json:"events"`
		}
		if s.conn.JSON(ctx, r.ConnectionEpoch, "GET", path(r.SessionID, "/events"), nil, &history) != nil {
			return
		}
		verified := false
		for _, raw := range history.Events {
			var event adkEvent
			if json.Unmarshal(raw, &event) == nil && event.InvocationID == r.InvocationID && !event.Partial && event.ID != "" {
				verified = true
			}
		}
		if verified {
			_ = s.pending.Update(scope, p.OperationID, func(p *Pending) error { p.TerminalVerified = true; return nil })
		}
	}
}
