// Package trace reads the Runtime-owned Turn trace for one invocation.
package trace

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type Step struct {
	StepID        string         `json:"stepId"`
	ParentStepID  string         `json:"parentStepId,omitempty"`
	Sequence      int            `json:"sequence"`
	Kind          string         `json:"kind"`
	Title         string         `json:"title"`
	Status        string         `json:"status"`
	StartedAt     string         `json:"startedAt"`
	EndedAt       string         `json:"endedAt,omitempty"`
	DurationMS    *float64       `json:"durationMs,omitempty"`
	SourceEventID string         `json:"sourceEventId,omitempty"`
	Category      string         `json:"category,omitempty"`
	Timing        map[string]any `json:"timing,omitempty"`
	Correlation   map[string]any `json:"correlation,omitempty"`
	Details       map[string]any `json:"details,omitempty"`
}

type Trace struct {
	TraceVersion string         `json:"traceVersion"`
	SessionID    string         `json:"sessionId"`
	InvocationID string         `json:"invocationId"`
	Correlation  map[string]any `json:"correlation,omitempty"`
	Run          map[string]any `json:"run,omitempty"`
	Coverage     map[string]any `json:"coverage,omitempty"`
	Steps        []Step         `json:"steps"`
}

type ReadRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SessionID       string `json:"sessionId"`
	InvocationID    string `json:"invocationId"`
}

type ReadResult struct {
	OK    bool           `json:"ok"`
	Value *Trace         `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}

type Service struct {
	conn *connection.Client
}

func New(conn *connection.Client) *Service { return &Service{conn: conn} }

func fail(code, retry string) *dto.SafeError {
	return dto.Failure(code, "trace", retry, 0)
}

func validID(value string) bool {
	return value != "" && len(value) <= 4096 && !strings.ContainsAny(value, "\r\n\x00")
}

func (s *Service) guard(epoch uint64) *dto.SafeError {
	if s.conn == nil || !s.conn.State().Connected {
		return fail("notConnected", "never")
	}
	if !s.conn.Current(epoch) {
		return fail("staleConnection", "reload")
	}
	return nil
}

func (s *Service) Read(ctx context.Context, r ReadRequest) ReadResult {
	if !validID(r.SessionID) || !validID(r.InvocationID) {
		return ReadResult{Error: fail("invalidInput", "never")}
	}
	if e := s.guard(r.ConnectionEpoch); e != nil {
		return ReadResult{Error: e}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	path := "/api/v1/runtime-console/sessions/" + url.PathEscape(r.SessionID) + "/trace?" + url.Values{"invocationId": {r.InvocationID}}.Encode()
	var out Trace
	if e := s.conn.JSON(ctx, r.ConnectionEpoch, http.MethodGet, path, nil, &out); e != nil {
		return ReadResult{Error: e}
	}
	if out.SessionID != r.SessionID || out.InvocationID != r.InvocationID || len(out.Steps) > 1000 {
		return ReadResult{Error: fail("invalidResponse", "verifyOnly")}
	}
	s.enrich(ctx, r, &out)
	return ReadResult{OK: true, Value: &out}
}

func (s *Service) enrich(ctx context.Context, r ReadRequest, out *Trace) {
	if out == nil {
		return
	}
	maxSequence := 0
	rootID := ""
	startedAt := time.Now().UTC().Format(time.RFC3339)
	seenKinds := map[string]bool{}
	for _, step := range out.Steps {
		if step.Sequence > maxSequence {
			maxSequence = step.Sequence
		}
		if step.Kind == "run.total" && rootID == "" {
			rootID = step.StepID
			if step.StartedAt != "" {
				startedAt = step.StartedAt
			}
		}
		if step.StartedAt != "" && startedAt == "" {
			startedAt = step.StartedAt
		}
		seenKinds[step.Kind] = true
	}
	appendStep := func(step Step) {
		if len(out.Steps) >= 1000 {
			return
		}
		maxSequence++
		if step.Sequence <= 0 {
			step.Sequence = maxSequence
		}
		if step.StartedAt == "" {
			step.StartedAt = startedAt
		}
		if step.ParentStepID == "" && step.Kind != "run.total" {
			step.ParentStepID = rootID
		}
		out.Steps = append(out.Steps, step)
	}
	s.enrichLifecycle(ctx, r, seenKinds, appendStep)
	s.enrichWorker(ctx, r, startedAt, rootID, appendStep)
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "started", "pending", "in_progress":
		return "running"
	case "completed", "done", "ok":
		return "success"
	case "":
		return "not_observed"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func safeMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func (s *Service) enrichLifecycle(ctx context.Context, r ReadRequest, seenKinds map[string]bool, appendStep func(Step)) {
	path := "/api/v1/runtime-console/sessions/" + url.PathEscape(r.SessionID) + "/lifecycle?" + url.Values{"invocationId": {r.InvocationID}}.Encode()
	var lifecycle struct {
		Stages []struct {
			Key        string         `json:"key"`
			Title      string         `json:"title"`
			Status     string         `json:"status"`
			StartedAt  string         `json:"startedAt,omitempty"`
			EndedAt    string         `json:"endedAt,omitempty"`
			DurationMS *float64       `json:"durationMs,omitempty"`
			Details    map[string]any `json:"details,omitempty"`
		} `json:"stages"`
	}
	if e := s.conn.JSON(ctx, r.ConnectionEpoch, http.MethodGet, path, nil, &lifecycle); e != nil {
		return
	}
	for index, stage := range lifecycle.Stages {
		if stage.Key == "" || seenKinds[stage.Key] {
			continue
		}
		details := safeMap(stage.Details)
		details["projectionSource"] = "runtime-console.lifecycle"
		appendStep(Step{
			StepID:     fmt.Sprintf("lifecycle:%s:%d", stage.Key, index+1),
			Kind:       stage.Key,
			Title:      firstNonEmpty(stage.Title, stage.Key),
			Status:     normalizeStatus(stage.Status),
			StartedAt:  stage.StartedAt,
			EndedAt:    stage.EndedAt,
			DurationMS: stage.DurationMS,
			Category:   traceCategory(stage.Key),
			Details:    details,
		})
		seenKinds[stage.Key] = true
	}
}

func (s *Service) enrichWorker(ctx context.Context, r ReadRequest, startedAt, rootID string, appendStep func(Step)) {
	path := "/api/v1/runtime-console/sessions/" + url.PathEscape(r.SessionID) + "/worker"
	var worker map[string]any
	if e := s.conn.JSON(ctx, r.ConnectionEpoch, http.MethodGet, path, nil, &worker); e != nil {
		return
	}
	bindings, _ := worker["bindings"].([]any)
	if len(bindings) == 0 {
		return
	}
	latest, _ := bindings[len(bindings)-1].(map[string]any)
	details := safeMap(latest)
	if summary, ok := worker["workerSessionSummary"].(map[string]any); ok {
		details["workerSessionSummary"] = summary
	}
	details["projectionSource"] = "runtime-console.worker"
	appendStep(Step{
		StepID:       "worker:binding:latest",
		ParentStepID: rootID,
		Kind:         "worker.binding",
		Title:        "Worker binding",
		Status:       workerBindingStatus(latest),
		StartedAt:    startedAt,
		Category:     "worker",
		Correlation:  workerCorrelation(latest),
		Details:      details,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func traceCategory(kind string) string {
	if strings.HasPrefix(kind, "worker") || kind == "event.project.persist" {
		return "worker"
	}
	if strings.HasPrefix(kind, "sandbox") {
		return "sandbox"
	}
	if strings.HasPrefix(kind, "model.") || strings.HasPrefix(kind, "top_loop.") {
		return "model"
	}
	if strings.HasPrefix(kind, "request.") {
		return "request"
	}
	if strings.HasPrefix(kind, "before_model") || strings.HasPrefix(kind, "tooling.") {
		return "preparation"
	}
	if strings.HasPrefix(kind, "terminal.") {
		return "terminal"
	}
	if kind == "tool.call" {
		return "tool"
	}
	return "runtime"
}

func workerBindingStatus(binding map[string]any) string {
	for _, key := range []string{"status", "lastRunStatus", "bindingStatus"} {
		if value, ok := binding[key].(string); ok && value != "" {
			switch strings.ToLower(value) {
			case "completed":
				return "success"
			case "staging":
				return "running"
			default:
				return normalizeStatus(value)
			}
		}
	}
	return "not_observed"
}

func workerCorrelation(binding map[string]any) map[string]any {
	out := map[string]any{}
	for source, target := range map[string]string{
		"subagentSessionId":      "sessionId",
		"activeSubagentRunId":    "turnId",
		"subagentProfileId":      "profileId",
		"subagentProfileVersion": "profileVersion",
	} {
		if value := binding[source]; value != nil && value != "" {
			out[target] = value
		}
	}
	if metadata, ok := binding["metadata"].(map[string]any); ok {
		for source, target := range map[string]string{
			"workerTurnId": "turnId",
			"workerRunId":  "runId",
		} {
			if value := metadata[source]; value != nil && value != "" {
				out[target] = value
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return map[string]any{"worker": out}
}
