package session

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/egoist/mygo"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Service struct {
	operationMu sync.Mutex
	conn        *connection.Client
	pending     *PendingStore
}

func New(c *connection.Client, p *PendingStore) *Service { return &Service{conn: c, pending: p} }
func path(id, suffix string) string                      { return "/api/v1/sessions/" + url.PathEscape(id) + suffix }
func (s *Service) List(ctx context.Context, r dto.ScopeRequest) dto.SessionListResult {
	out := dto.SessionList{Sessions: []dto.SessionSummary{}}
	e := s.conn.JSON(ctx, r.ConnectionEpoch, "GET", "/api/v1/sessions?include_a2a=false", nil, &out)
	return dto.SessionListResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func only[T any](v *T, e *dto.SafeError) *T {
	if e != nil {
		return nil
	}
	return v
}
func (s *Service) Create(ctx context.Context, r dto.CreateRequest) dto.SessionSummaryResult {
	var out dto.SessionSummary
	input := struct {
		SessionID string `json:"sessionId,omitempty"`
	}{r.SessionID}
	e := s.conn.JSON(ctx, r.ConnectionEpoch, "POST", "/api/v1/sessions", input, &out)
	if out.ID == "" {
		out.ID = out.SessionID
	}
	return dto.SessionSummaryResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (s *Service) Read(ctx context.Context, r dto.SessionRequest) dto.SessionHistoryResult {
	out := dto.SessionHistory{Events: []dto.ClientEvent{}, Complete: true}
	e := s.conn.JSON(ctx, r.ConnectionEpoch, "GET", path(r.SessionID, ""), nil, &out.Summary)
	if e == nil {
		var history struct {
			Events []json.RawMessage `json:"events"`
		}
		e = s.conn.JSON(ctx, r.ConnectionEpoch, "GET", path(r.SessionID, "/events"), nil, &history)
		if e == nil {
			for _, raw := range history.Events {
				events, err := Project(raw, "message", dto.SubscribeRequest{ConnectionEpoch: r.ConnectionEpoch, SessionID: r.SessionID})
				if err != nil {
					e = dto.Failure("invalidEvent", "runtimeIngress", "history", 0)
					break
				}
				out.Events = append(out.Events, events...)
			}
		}
	}
	if e == nil {
		if list, err := s.pending.List(s.conn.State().Scope); err == nil {
			for _, p := range list {
				if p.SessionID == r.SessionID {
					out.ActiveRun = &dto.ActiveRun{OperationID: p.OperationID, InvocationID: p.InvocationID, State: p.State, Authority: "localReceipt", RecoveryDisposition: p.disposition()}
					break
				}
			}
		}
	}
	return dto.SessionHistoryResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (s *Service) Rename(ctx context.Context, r dto.RenameRequest) dto.SessionSummaryResult {
	var out dto.SessionSummary
	e := s.conn.JSON(ctx, r.ConnectionEpoch, "PUT", path(r.SessionID, ""), struct {
		Title string `json:"title"`
	}{r.Title}, &out)
	if out.ID == "" {
		out.ID = out.SessionID
	}
	return dto.SessionSummaryResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (s *Service) Delete(ctx context.Context, r dto.SessionRequest) dto.DeleteReceiptResult {
	var out dto.DeleteReceipt
	e := s.conn.JSON(ctx, r.ConnectionEpoch, "DELETE", path(r.SessionID, ""), nil, &out)
	return dto.DeleteReceiptResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (s *Service) Submit(ctx context.Context, r dto.SubmitRequest) dto.RunReceiptResult {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if len(r.AttachmentRefs) != 0 || r.OperationID == "" || r.IdempotencyKey == "" || r.Content == "" || len(r.Content) > 4<<20 || strings.ContainsAny(r.IdempotencyKey, "\r\n") {
		return dto.RunReceiptResult{Error: dto.Failure("invalidInput", "mpa", "never", 0)}
	}
	input := struct {
		Content                string `json:"content"`
		ExecutionConfigVersion *int   `json:"executionConfigVersion,omitempty"`
	}{Content: r.Content}
	if r.ExecutionConfigVersion != "" {
		v, err := strconv.Atoi(r.ExecutionConfigVersion)
		if err != nil || v < 1 {
			return dto.RunReceiptResult{Error: dto.Failure("invalidInput", "mpa", "never", 0)}
		}
		input.ExecutionConfigVersion = &v
	}
	state := s.conn.State()
	if !s.conn.Current(r.ConnectionEpoch) {
		return dto.RunReceiptResult{Error: dto.Failure("staleConnection", "transport", "reload", 0)}
	}
	operations, pendingError := s.pending.List(state.Scope)
	if pendingError != nil {
		return dto.RunReceiptResult{Error: dto.Failure("pendingPersistenceFailed", "transport", "never", 0)}
	}
	for _, operation := range operations {
		if operation.OperationID == r.OperationID || operation.IdempotencyKey == r.IdempotencyKey {
			if operation.SessionID != r.SessionID || operation.Content != r.Content || operation.ConfigVersion != r.ExecutionConfigVersion || !equalAttachmentRefs(operation.AttachmentRefs, r.AttachmentRefs) {
				return dto.RunReceiptResult{Error: dto.Failure("idempotency_mismatch", "transport", "never", 409)}
			}
		}
		if operation.OperationID == r.OperationID || operation.SessionID == r.SessionID {
			return dto.RunReceiptResult{Error: dto.Failure("outcomeUnknown", "transport", "verifyOnly", 0)}
		}
	}
	p := Pending{AttachmentRefs: cloneAttachmentRefs(r.AttachmentRefs), Scope: state.Scope, SessionID: r.SessionID, OperationID: r.OperationID, IdempotencyKey: r.IdempotencyKey, Content: r.Content, ConfigVersion: r.ExecutionConfigVersion, DurableBot: s.durableBot(ctx, r.ConnectionEpoch, r.SessionID, r.ExecutionConfigVersion)}
	if e := s.pending.Put(p); e != nil {
		if errors.Is(e, ErrPendingExists) {
			return dto.RunReceiptResult{Error: dto.Failure("outcomeUnknown", "transport", "verifyOnly", 0)}
		}
		return dto.RunReceiptResult{Error: dto.Failure("pendingPersistenceFailed", "transport", "never", 0)}
	}

	data, _ := json.Marshal(input)
	headers := http.Header{}
	headers.Set("Idempotency-Key", r.IdempotencyKey)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	resp, e := s.conn.Request(ctx, r.ConnectionEpoch, "POST", path(r.SessionID, "/run"), data, headers)
	var out dto.RunReceipt
	if e == nil {
		defer resp.Body.Close()
		if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil || out.InvocationID == "" {
			e = dto.Failure("outcomeUnknown", "runtimeIngress", "verifyOnly", resp.StatusCode)
		}
	}
	if e == nil {
		if err := s.pending.Update(state.Scope, r.OperationID, func(p *Pending) error { p.State = "accepted"; p.InvocationID = out.InvocationID; return nil }); err != nil {
			e = dto.Failure("pendingPersistenceFailed", "transport", "verifyOnly", 0)
		}
	}

	return dto.RunReceiptResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (s *Service) Subscribe(ctx context.Context, r dto.SubscribeRequest, channel *mygo.Channel[dto.ClientEvent]) dto.EmptyResult {
	return s.stream(ctx, r, channel.Send)
}
func (s *Service) stream(ctx context.Context, r dto.SubscribeRequest, send func(dto.ClientEvent) error) dto.EmptyResult {
	input := struct {
		LastEventID  string `json:"lastEventId,omitempty"`
		InvocationID string `json:"invocationId"`
	}{r.LastEventID, r.InvocationID}
	body, _ := json.Marshal(input)
	resp, e := s.conn.Request(ctx, r.ConnectionEpoch, "POST", path(r.SessionID, "/sse"), body, nil)
	if e != nil {
		_ = send(dto.ClientEvent{SchemaVersion: 1, Kind: "error", ConnectionEpoch: r.ConnectionEpoch, SubscriptionEpoch: r.SubscriptionEpoch, SessionID: r.SessionID, InvocationID: r.InvocationID, Error: e})
		return dto.EmptyResult{Error: e}
	}
	defer resp.Body.Close()
	err := ParseSSE(resp.Body, r, func(event dto.ClientEvent) error {
		if !s.conn.Current(r.ConnectionEpoch) {
			return context.Canceled
		}
		if event.Terminal && (event.Kind == "done" || event.Kind == "error") && event.InvocationID == r.InvocationID {
			s.observeTerminal(ctx, r)
		}
		return send(event)
	})
	if err != nil && ctx.Err() == nil && s.conn.Current(r.ConnectionEpoch) {
		e = dto.Failure("streamInterrupted", "runtimeIngress", "history", 0)
		_ = send(dto.ClientEvent{SchemaVersion: 1, Kind: "error", ConnectionEpoch: r.ConnectionEpoch, SubscriptionEpoch: r.SubscriptionEpoch, SessionID: r.SessionID, InvocationID: r.InvocationID, Error: e})
		return dto.EmptyResult{Error: e}
	}
	return dto.EmptyResult{OK: true, Value: &dto.Empty{}}
}
func (s *Service) Abort(ctx context.Context, r dto.SessionRequest) dto.AbortReceiptResult {
	var out dto.AbortReceipt
	e := s.conn.JSON(ctx, r.ConnectionEpoch, "POST", path(r.SessionID, "/abort"), nil, &out)
	return dto.AbortReceiptResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (s *Service) ExecutionConfig(ctx context.Context, r dto.SessionRequest) dto.ExecutionConfigResult {
	resp, e := s.conn.Request(ctx, r.ConnectionEpoch, "GET", path(r.SessionID, "/execution-config"), nil, nil)
	if e != nil {
		return dto.ExecutionConfigResult{Error: e}
	}
	return decodeExecutionConfig(resp)
}
func (s *Service) Pending(ctx context.Context, r dto.ScopeRequest) dto.PendingListResult {
	if !s.conn.Current(r.ConnectionEpoch) {
		return dto.PendingListResult{Error: dto.Failure("staleConnection", "transport", "reload", 0)}
	}
	list, e := s.pending.List(s.conn.State().Scope)
	if e != nil {
		return dto.PendingListResult{Error: dto.Failure("pendingPersistenceFailed", "transport", "never", 0)}
	}
	out := dto.PendingList{Operations: []dto.PendingSummary{}}
	for _, p := range list {
		out.Operations = append(out.Operations, p.Summary())
	}
	return dto.PendingListResult{OK: true, Value: &out}
}
