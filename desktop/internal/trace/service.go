// Package trace reads the Runtime-owned Turn trace for one invocation.
package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
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

const traceProjectionVersion = "worker-diagnostics-v2"

type ReadRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SessionID       string `json:"sessionId"`
	InvocationID    string `json:"invocationId"`
	ForceRefresh    bool   `json:"forceRefresh,omitempty"`
}

type ReadResult struct {
	OK    bool           `json:"ok"`
	Value *Trace         `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}

type PrefetchResult struct {
	OK    bool           `json:"ok"`
	Value *dto.Empty     `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}

type Service struct {
	conn        *connection.Client
	cache       *Cache
	prefetchMu  sync.Mutex
	prefetching map[ReadRequest]bool
}

func New(conn *connection.Client) *Service { return &Service{conn: conn} }
func NewWithCache(conn *connection.Client, cache *Cache) *Service {
	return &Service{conn: conn, cache: cache}
}

func fail(code, retry string) *dto.SafeError {
	return dto.Failure(code, "trace", retry, 0)
}

func validID(value string) bool {
	return value != "" && len(value) <= 4096 && !strings.ContainsAny(value, "\r\n\x00")
}

func markTraceProjectionVersion(trace *Trace) {
	if trace == nil {
		return
	}
	if trace.Correlation == nil {
		trace.Correlation = map[string]any{}
	}
	trace.Correlation["openmpaTraceProjectionVersion"] = traceProjectionVersion
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
	if cached, ok := s.cache.LoadTrace(r.SessionID, r.InvocationID); ok {
		if !r.ForceRefresh {
			if traceStillActive(cached) || cached.TraceVersion == "local-sse-v1" {
				_ = s.Prefetch(ctx, r)
			}
			return ReadResult{OK: true, Value: &cached}
		}
	}
	return s.readRemote(ctx, r)
}

func traceStillActive(trace Trace) bool {
	return trace.Coverage["active"] == true || trace.Run["status"] == "running"
}

func traceRequiresPrefetch(trace Trace) bool {
	if traceStillActive(trace) {
		return true
	}
	if complete, exists := trace.Coverage["aggregateComplete"]; exists && complete != true {
		return true
	}
	worker, _ := trace.Coverage["workerTrajectory"].(map[string]any)
	return worker["status"] == "unavailable"
}

func (s *Service) readRemote(ctx context.Context, r ReadRequest) ReadResult {
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
	// Current Runtime traces already include worker trajectory and its coverage.
	// Only older responses need the compatibility metadata requests.
	if _, ownsWorkerTrajectory := out.Coverage["workerTrajectory"]; !ownsWorkerTrajectory {
		s.enrich(ctx, r, &out)
	}
	if e := s.guard(r.ConnectionEpoch); e != nil {
		return ReadResult{Error: e}
	}
	out, _ = s.cache.MergeTrace(r.SessionID, r.InvocationID, out)
	return ReadResult{OK: true, Value: &out}
}

func (s *Service) Prefetch(ctx context.Context, r ReadRequest) PrefetchResult {
	if !validID(r.SessionID) || !validID(r.InvocationID) {
		return PrefetchResult{Error: fail("invalidInput", "never")}
	}
	if e := s.guard(r.ConnectionEpoch); e != nil {
		return PrefetchResult{Error: e}
	}
	if s.cache.sessionDeleted(r.SessionID) {
		return PrefetchResult{OK: true, Value: &dto.Empty{}}
	}
	if cached, ok := s.cache.LoadTrace(r.SessionID, r.InvocationID); ok && !traceRequiresPrefetch(cached) && !r.ForceRefresh {
		return PrefetchResult{OK: true, Value: &dto.Empty{}}
	}
	s.prefetchMu.Lock()
	if s.prefetching == nil {
		s.prefetching = map[ReadRequest]bool{}
	}
	if s.prefetching[r] {
		s.prefetchMu.Unlock()
		return PrefetchResult{OK: true, Value: &dto.Empty{}}
	}
	s.prefetching[r] = true
	s.prefetchMu.Unlock()
	go func() {
		defer func() { s.prefetchMu.Lock(); delete(s.prefetching, r); s.prefetchMu.Unlock() }()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()
		for failures := 0; ; {
			if s.guard(r.ConnectionEpoch) != nil || s.cache.sessionDeleted(r.SessionID) {
				return
			}
			result := s.readRemote(ctx, r)
			if result.OK {
				failures = 0
				if !traceStillActive(*result.Value) {
					return
				}
			} else {
				failures++
				if failures >= 3 {
					return
				}
			}
			timer := time.NewTimer(5 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	return PrefetchResult{OK: true, Value: &dto.Empty{}}
}

func (s *Service) enrich(ctx context.Context, r ReadRequest, out *Trace) {
	if out == nil {
		return
	}
	maxSequence := 0
	rootID := ""
	startedAt := ""
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
		if step.ParentStepID == "" && step.Kind != "run.total" {
			step.ParentStepID = rootID
		}
		out.Steps = append(out.Steps, step)
	}
	s.enrichLifecycle(ctx, r, seenKinds, appendStep)
	s.enrichWorker(ctx, r, startedAt, rootID, appendStep)
	s.enrichWorkerDiagnostics(ctx, r, startedAt, rootID, appendStep)
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
	for index, value := range bindings {
		latest, _ := value.(map[string]any)
		// This endpoint lists the entire session. A later turn's worker must not
		// appear in an earlier turn's trace.
		if stringValue(latest["invocationId"]) != r.InvocationID {
			continue
		}
		details := safeMap(latest)
		if summary, ok := worker["workerSessionSummary"].(map[string]any); ok {
			details["workerSessionSummary"] = summary
		}
		details["projectionSource"] = "runtime-console.worker"
		appendStep(Step{
			StepID:       fmt.Sprintf("worker:binding:%d", index),
			ParentStepID: rootID,
			Kind:         "worker.binding",
			Title:        "Worker binding",
			Status:       workerBindingStatus(latest),
			StartedAt:    stringValue(latest["createdAt"]),
			Category:     "worker",
			Correlation:  workerCorrelation(latest),
			Details:      details,
		})
	}
}

func (s *Service) enrichWorkerDiagnostics(ctx context.Context, r ReadRequest, startedAt, rootID string, appendStep func(Step)) {
	path := "/api/v1/runtime-console/sessions/" + url.PathEscape(r.SessionID) + "/diagnostics"
	var diagnostics map[string]any
	if e := s.conn.JSON(ctx, r.ConnectionEpoch, http.MethodGet, path, nil, &diagnostics); e != nil {
		return
	}
	_ = s.cache.SaveDiagnostics(r.SessionID, r.InvocationID, diagnostics)
	for _, rawItem := range anySlice(firstValue(diagnostics, "items", "Items")) {
		item := mapValue(rawItem)
		if len(item) == 0 {
			continue
		}
		sourceName := strings.ToLower(firstNonEmpty(stringValue(item["source"]), stringValue(item["Source"])))
		invocationID := firstNonEmpty(stringValue(item["invocationId"]), stringValue(item["InvocationId"]), stringValue(item["invocationID"]), stringValue(item["InvocationID"]))
		if sourceName != "worker_live" || invocationID != r.InvocationID {
			continue
		}
		workerSessionID := firstNonEmpty(stringValue(item["workerSessionId"]), stringValue(item["workerSessionID"]), stringValue(item["WorkerSessionID"]))
		workerTurnID := firstNonEmpty(stringValue(item["workerTurnId"]), stringValue(item["workerTurnID"]), stringValue(item["WorkerTurnID"]))
		for _, sourceValue := range diagnosticTrajectorySources(item) {
			source := mapValue(sourceValue)
			if strings.EqualFold(firstNonEmpty(stringValue(source["status"]), stringValue(source["Status"])), "unavailable") {
				continue
			}
			for index, event := range collectWorkerTrajectoryEvents(source) {
				appendStep(workerTrajectoryStep(event, index+1, workerSessionID, workerTurnID, startedAt, rootID))
			}
		}
	}
}

func diagnosticTrajectorySources(item map[string]any) []any {
	sources := anySlice(firstValue(item, "sources", "Sources"))
	if len(sources) > 0 {
		out := []any{}
		for _, sourceValue := range sources {
			source := mapValue(sourceValue)
			sourceName := strings.ToLower(firstNonEmpty(stringValue(source["source"]), stringValue(source["Source"])))
			if sourceName == "trajectory" || sourceName == "worker_trajectory" || sourceName == "" {
				out = append(out, sourceValue)
			}
		}
		return out
	}
	return []any{item}
}

func collectWorkerTrajectoryEvents(value any) []map[string]any {
	out := []map[string]any{}
	seen := map[string]bool{}
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case []any:
			for _, item := range typed {
				visit(item)
			}
		case map[string]any:
			if looksLikeWorkerEvent(typed) {
				key := workerEventKey(typed, len(out)+1)
				if !seen[key] {
					seen[key] = true
					out = append(out, typed)
				}
			}
			for _, key := range []string{"events", "Events", "steps", "Steps", "trajectory", "Trajectory", "evidence", "Evidence", "data", "Data", "result", "Result", "items", "Items"} {
				if child, ok := typed[key]; ok {
					visit(child)
				}
			}
		}
	}
	visit(value)
	return out
}

func looksLikeWorkerEvent(value map[string]any) bool {
	if len(value) == 0 {
		return false
	}
	eventType := firstNonEmpty(stringValue(value["type"]), stringValue(value["eventType"]), stringValue(value["kind"]), stringValue(value["Kind"]))
	if strings.Contains(strings.ToLower(eventType), "worker") || strings.Contains(strings.ToLower(eventType), "codex") {
		return true
	}
	if eventType != "" && (value["payload"] != nil || value["details"] != nil || value["durationMs"] != nil || value["startedAt"] != nil) {
		return true
	}
	if stringValue(value["stepId"]) != "" && stringValue(value["kind"]) != "" {
		return true
	}
	return false
}

func workerEventKey(value map[string]any, fallback int) string {
	payload := mapValue(value["payload"])
	if len(payload) == 0 {
		payload = mapValue(value["details"])
	}
	return firstNonEmpty(
		stringValue(value["eventId"]),
		stringValue(value["id"]),
		stringValue(value["stepId"]),
		stringValue(value["sourceEventId"]),
		stringValue(payload["eventId"]),
		stringValue(payload["id"]),
		fmt.Sprintf("%d", fallback),
	)
}

func workerTrajectoryStep(event map[string]any, sequence int, workerSessionID, workerTurnID, startedAt, rootID string) Step {
	payload := safeMap(mapValue(event["payload"]))
	if details := mapValue(event["details"]); len(details) > 0 {
		if len(payload) == 0 {
			payload = safeMap(details)
		} else {
			for key, value := range details {
				if payload[key] == nil {
					payload[key] = value
				}
			}
		}
	}
	for _, key := range []string{"title", "status", "startedAt", "endedAt", "durationMs", "timestamp", "toolName", "name", "toolType", "commandId", "callId", "exitCode", "modelId", "requestId", "inputTokens", "outputTokens", "totalTokens", "firstContentFrameMs"} {
		if payload[key] == nil && event[key] != nil {
			payload[key] = event[key]
		}
	}
	sourceType := firstNonEmpty(stringValue(event["type"]), stringValue(event["eventType"]), stringValue(event["kind"]), stringValue(event["Kind"]), stringValue(payload["type"]), stringValue(payload["kind"]))
	eventID := firstNonEmpty(stringValue(event["eventId"]), stringValue(event["id"]), stringValue(event["stepId"]), stringValue(event["sourceEventId"]), stringValue(payload["eventId"]), stringValue(payload["id"]))
	if eventID == "" {
		eventID = fmt.Sprintf("%d", sequence)
	}
	status := normalizeStatus(firstNonEmpty(stringValue(payload["status"]), stringValue(event["status"])))
	if status == "not_observed" {
		status = workerEventStatus(sourceType)
	}
	workerSessionID = firstNonEmpty(workerSessionID, stringValue(payload["sessionId"]), stringValue(event["sessionId"]))
	workerTurnID = firstNonEmpty(workerTurnID, stringValue(payload["turnId"]), stringValue(event["turnId"]))
	return Step{
		StepID:        "worker:trajectory:" + eventID,
		ParentStepID:  rootID,
		Kind:          "worker." + firstNonEmpty(sourceType, "event"),
		Title:         firstNonEmpty(stringValue(event["title"]), workerEventTitle(sourceType, payload)),
		Status:        status,
		StartedAt:     firstNonEmpty(timeValue(payload["timestamp"]), timeValue(event["timestamp"]), stringValue(payload["startedAt"]), stringValue(event["startedAt"]), stringValue(payload["createdAt"]), stringValue(event["createdAt"])),
		EndedAt:       firstNonEmpty(stringValue(payload["endedAt"]), stringValue(event["endedAt"])),
		DurationMS:    numberPtr(firstNumberValue(payload["durationMs"], event["durationMs"])),
		SourceEventID: eventID,
		Category:      "worker",
		Correlation:   workerTrajectoryCorrelation(workerSessionID, workerTurnID, payload),
		Details:       workerTrajectoryDetails(sourceType, eventID, workerSessionID, workerTurnID, payload),
	}
}

func firstValue(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return nil
}

func workerEventStatus(sourceType string) string {
	switch strings.ToLower(sourceType) {
	case "codex.tool.call", "codex.turn.started", "codex.turn.progress":
		return "running"
	case "codex.turn.failed", "codex.tool.error", "codex.tool.denied":
		return "failed"
	case "codex.turn.cancelled", "codex.turn.interrupted":
		return "cancelled"
	case "codex.tool.result", "codex.usage.updated", "codex.turn.completed":
		return "success"
	default:
		return "not_observed"
	}
}

func workerEventTitle(sourceType string, payload map[string]any) string {
	lowered := strings.ToLower(sourceType)
	if strings.Contains(lowered, "tool") {
		return firstNonEmpty(stringValue(payload["toolName"]), stringValue(payload["name"]), stringValue(payload["toolType"]), "Worker tool")
	}
	if strings.Contains(lowered, "usage") {
		return "Worker model usage"
	}
	if strings.Contains(lowered, "message_delta") {
		return "Worker response output"
	}
	if strings.Contains(lowered, "turn.completed") {
		return "Worker turn completed"
	}
	if strings.Contains(lowered, "turn") {
		return "Worker turn"
	}
	return firstNonEmpty(sourceType, "Worker event")
}

func workerTrajectoryCorrelation(workerSessionID, workerTurnID string, payload map[string]any) map[string]any {
	worker := map[string]any{}
	if workerSessionID != "" {
		worker["sessionId"] = workerSessionID
	}
	if workerTurnID != "" {
		worker["turnId"] = workerTurnID
	}
	if commandID := firstNonEmpty(stringValue(payload["commandId"]), stringValue(payload["callId"])); commandID != "" {
		worker["commandId"] = commandID
	}
	if len(worker) == 0 {
		return nil
	}
	return map[string]any{"worker": worker}
}

func workerTrajectoryDetails(sourceType, eventID, workerSessionID, workerTurnID string, payload map[string]any) map[string]any {
	details := map[string]any{
		"projectionSource": "runtime-console.diagnostics.worker_live.trajectory",
		"sourceType":       sourceType,
		"sourceEventId":    eventID,
	}
	for _, item := range []struct {
		key   string
		value any
	}{
		{"workerSessionId", workerSessionID},
		{"workerTurnId", workerTurnID},
		{"toolName", firstNonEmpty(stringValue(payload["toolName"]), stringValue(payload["name"]), stringValue(payload["toolType"]))},
		{"commandId", firstNonEmpty(stringValue(payload["commandId"]), stringValue(payload["callId"]))},
		{"modelId", firstNonEmpty(stringValue(payload["modelId"]), stringValue(mapValue(payload["model"])["modelId"]))},
		{"requestId", firstNonEmpty(stringValue(payload["requestId"]), stringValue(payload["upstreamRequestId"]))},
		{"exitCode", numberValue(firstNumberValue(payload["exitCode"]))},
		{"durationMs", numberValue(firstNumberValue(payload["durationMs"]))},
		{"firstContentFrameMs", numberValue(firstNumberValue(payload["firstContentFrameMs"], payload["first_content_frame_ms"]))},
		{"inputTokens", numberValue(firstNumberValue(payload["inputTokens"], payload["input_tokens"]))},
		{"outputTokens", numberValue(firstNumberValue(payload["outputTokens"], payload["output_tokens"]))},
		{"totalTokens", numberValue(firstNumberValue(payload["totalTokens"], payload["total_tokens"]))},
		{"cachedTokens", numberValue(firstNumberValue(payload["cachedTokens"], payload["cached_tokens"]))},
	} {
		if item.value != nil && item.value != "" {
			details[item.key] = item.value
		}
	}
	return details
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func mapValue(value any) map[string]any {
	if v, ok := value.(map[string]any); ok {
		return v
	}
	return map[string]any{}
}

func anySlice(value any) []any {
	if out, ok := value.([]any); ok {
		return out
	}
	return nil
}

func firstNumberValue(values ...any) *float64 {
	for _, value := range values {
		switch v := value.(type) {
		case float64:
			return &v
		case float32:
			out := float64(v)
			return &out
		case int:
			out := float64(v)
			return &out
		case int64:
			out := float64(v)
			return &out
		case json.Number:
			if out, err := v.Float64(); err == nil {
				return &out
			}
		}
	}
	return nil
}

func numberPtr(value *float64) *float64 { return value }

func numberValue(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func timeValue(value any) string {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			return v
		}
	case float64:
		if v > 1_000_000_000_000 {
			return time.UnixMilli(int64(v)).UTC().Format(time.RFC3339Nano)
		}
	case int64:
		if v > 1_000_000_000_000 {
			return time.UnixMilli(v).UTC().Format(time.RFC3339Nano)
		}
	case int:
		if v > 1_000_000_000_000 {
			return time.UnixMilli(int64(v)).UTC().Format(time.RFC3339Nano)
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
