package session

import (
	"context"
	"encoding/json"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

// Read projects authoritative Runtime history and reconciles durable pending
// receipts without changing the public Session service boundary.
func (s *Service) Read(ctx context.Context, r dto.SessionRequest) dto.SessionHistoryResult {
	out := dto.SessionHistory{Events: []dto.ClientEvent{}, Complete: true}
	var e *dto.SafeError
	if s.traceSession(r.SessionID) {
		e = s.readAdmin(ctx, r, &out)
	} else {
		e = s.readNormal(ctx, r, &out)
		if e != nil && e.HTTPStatus == 404 {
			e = s.readAdmin(ctx, r, &out)
		}
	}
	if e == nil {
		if list, err := s.pending.List(s.conn.State().Scope); err == nil {
			for _, p := range list {
				if p.TraceEnabled {
					s.markTraceSession(p.SessionID)
				}
				if p.SessionID == r.SessionID {
					out.ActiveRun = &dto.ActiveRun{OperationID: p.OperationID, InvocationID: p.InvocationID, State: p.State, Authority: "localReceipt", RecoveryDisposition: p.disposition()}
					break
				}
			}
		}
	}
	return dto.SessionHistoryResult{OK: e == nil, Value: only(&out, e), Error: e}
}

func (s *Service) readAdmin(ctx context.Context, r dto.SessionRequest, out *dto.SessionHistory) *dto.SafeError {
	var adminHistory struct {
		SessionID string            `json:"sessionId"`
		Events    []json.RawMessage `json:"events"`
	}
	e := s.conn.JSON(ctx, r.ConnectionEpoch, "GET", adminPath(r.SessionID, "/events"), nil, &adminHistory)
	if e != nil {
		return e
	}
	title := s.traceTitle(r.SessionID)
	if title == "" {
		title = r.SessionID
	}
	out.Summary = dto.SessionSummary{ID: r.SessionID, SessionID: r.SessionID, Title: title, Status: "idle", TraceEnabled: true}
	s.markTraceSession(r.SessionID)
	for _, raw := range adminHistory.Events {
		events, err := Project(raw, "message", dto.SubscribeRequest{ConnectionEpoch: r.ConnectionEpoch, SessionID: r.SessionID})
		if err != nil {
			return dto.Failure("invalidEvent", "runtimeIngress", "history", 0)
		}
		out.Events = append(out.Events, events...)
	}
	s.reconcileTerminalPendingFromHistory(r.SessionID, out.Events)
	return nil
}

func (s *Service) readNormal(ctx context.Context, r dto.SessionRequest, out *dto.SessionHistory) *dto.SafeError {
	e := s.conn.JSON(ctx, r.ConnectionEpoch, "GET", path(r.SessionID, ""), nil, &out.Summary)
	if e != nil {
		return e
	}
	var history struct {
		Events []json.RawMessage `json:"events"`
	}
	e = s.conn.JSON(ctx, r.ConnectionEpoch, "GET", path(r.SessionID, "/events"), nil, &history)
	if e != nil {
		return e
	}
	for _, raw := range history.Events {
		events, err := Project(raw, "message", dto.SubscribeRequest{ConnectionEpoch: r.ConnectionEpoch, SessionID: r.SessionID})
		if err != nil {
			return dto.Failure("invalidEvent", "runtimeIngress", "history", 0)
		}
		out.Events = append(out.Events, events...)
	}
	s.reconcileTerminalPendingFromHistory(r.SessionID, out.Events)
	return nil
}
