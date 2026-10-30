package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type archiveProjection struct {
	size     int64
	modified time.Time
	data     []byte
}

func executionEvent(event dto.ClientEvent) bool {
	if event.Author == "user" || event.Kind == "heartbeat" {
		return false
	}
	return event.Kind == "reasoning" || event.Kind == "toolCall" || event.Kind == "toolResult" || event.Terminal ||
		(strings.HasPrefix(event.SourceType, "codex.") && !strings.Contains(event.SourceType, "message_delta"))
}

// AppendEvent runs on the native SSE reader, before delivery to WebView. No
// application write buffer or remote request sits between receipt and storage.
func (c *Cache) AppendEvent(event dto.ClientEvent) error {
	if c == nil || !executionEvent(event) || !validID(event.SessionID) || !validID(event.InvocationID) {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.deleted[event.SessionID] {
		return nil
	}
	err := c.appendEventLocked(event)
	if err != nil {
		if c.archiveErrors == nil {
			c.archiveErrors = map[string]bool{}
		}
		c.archiveErrors[c.dir(event.SessionID, event.InvocationID)] = true
	}
	return err
}

func (c *Cache) appendEventLocked(event dto.ClientEvent) error {
	dir := c.dir(event.SessionID, event.InvocationID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	_, err = file.Write(append(data, '\n'))
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func eventIdentity(event dto.ClientEvent) string {
	if event.EventID == "" {
		return ""
	}
	return fmt.Sprintf("%s:%v:%s:%s", event.EventID, partNumber(event.PartIndex), event.Kind, event.ToolID)
}

func partNumber(part *int) int {
	if part == nil {
		return -1
	}
	return *part
}

func (c *Cache) archivedTrace(sessionID, invocationID string) (Trace, bool) {
	path := filepath.Join(c.dir(sessionID, invocationID), "events.jsonl")
	file, err := os.Open(path)
	if err != nil {
		return Trace{}, false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return Trace{}, false
	}
	c.projectionMu.Lock()
	memo, cached := c.projections[path]
	c.projectionMu.Unlock()
	if cached && memo.size == info.Size() && memo.modified.Equal(info.ModTime()) {
		var out Trace
		if json.Unmarshal(memo.data, &out) == nil {
			return out, true
		}
	}
	out := Trace{TraceVersion: "local-sse-v1", SessionID: sessionID, InvocationID: invocationID,
		Run: map[string]any{"status": "running"}, Coverage: map[string]any{"active": true, "complete": false, "aggregateComplete": "unknown", "scope": "local_sse_archive"}, Steps: []Step{}}
	seen := map[string]bool{}
	tools := map[string]int{}
	reasoning := -1
	count := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 8<<20)
	for scanner.Scan() {
		var event dto.ClientEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.SessionID != sessionID || event.InvocationID != invocationID {
			out.Coverage["localArchiveStatus"] = "incomplete"
			continue
		}
		id := eventIdentity(event)
		if id != "" && seen[id] {
			continue
		}
		if id != "" {
			seen[id] = true
		}
		count++
		if event.Terminal {
			out.Coverage["active"] = false
			out.Run["status"] = "success"
			if event.Kind == "error" {
				out.Run["status"] = "failed"
			}
			for index := range out.Steps {
				if out.Steps[index].Status == "running" {
					out.Steps[index].Status = "not_observed"
				}
			}
		}
		if event.Kind == "done" || event.Kind == "error" {
			continue
		}
		if id == "" {
			id = fmt.Sprintf("record-%d", count)
		}
		step := Step{StepID: "sse:" + id, Sequence: len(out.Steps) + 1, SourceEventID: event.EventID,
			Kind: firstNonEmpty(event.SourceType, event.Kind), StartedAt: event.StartedAt, EndedAt: event.EndedAt, DurationMS: event.DurationMS,
			Status: "not_observed", Category: "runtime", Details: map[string]any{"projectionSource": "local.sse", "partIndex": partNumber(event.PartIndex)}}
		if strings.HasPrefix(event.SourceType, "codex.") || strings.HasPrefix(event.SourceType, "worker.") {
			step.Category = "worker"
		}
		switch event.Kind {
		case "toolCall", "toolResult":
			reasoning = -1
			step.Title, step.Status = firstNonEmpty(event.ToolName, "Tool"), normalizeStatus(event.ToolStatus)
			if step.Category != "worker" {
				step.Category = "tool"
			}
			step.Correlation = map[string]any{"toolId": event.ToolID}
			step.Details["toolSubject"], step.Details["outputPreview"], step.Details["stderrPreview"] = event.ToolSubject, event.OutputPreview, event.StderrPreview
			if event.SafeCommandSummary != nil {
				step.Details["safeCommandSummary"] = event.SafeCommandSummary
			}
			if index, ok := tools[event.ToolID]; event.ToolID != "" && ok {
				previous := out.Steps[index]
				// Replayed call frames cannot regress a completed tool result.
				if event.Kind == "toolCall" && previous.Status != "running" && previous.Status != "not_observed" {
					continue
				}
				step.StepID, step.Sequence = previous.StepID, previous.Sequence
				step.StartedAt = firstNonEmpty(step.StartedAt, previous.StartedAt)
				if event.ToolSubject == "" {
					step.Details["toolSubject"] = previous.Details["toolSubject"]
				}
				out.Steps[index] = step
				continue
			}
			if event.ToolID != "" {
				tools[event.ToolID] = len(out.Steps)
			}
		case "reasoning":
			step.Title, step.Status = "Reasoning", "success"
			if event.Partial {
				step.Status = "running"
			}
			step.Details["text"] = event.Text
			if reasoning >= 0 && out.Steps[reasoning].Details["partIndex"] == partNumber(event.PartIndex) {
				previous := &out.Steps[reasoning]
				text := event.Text
				if event.Partial {
					text = stringValue(previous.Details["text"]) + text
				}
				// The complete source remains in events.jsonl; bound rendered detail.
				if len(text) > 1<<20 {
					text = text[:1<<20]
					previous.Details["contentTruncated"] = true
				}
				previous.Details["text"], previous.Status = text, step.Status
				if !event.Partial {
					previous.SourceEventID = event.EventID
				}
				if step.DurationMS != nil {
					previous.DurationMS = step.DurationMS
				}
				previous.EndedAt = firstNonEmpty(step.EndedAt, previous.EndedAt)
				continue
			}
			reasoning = len(out.Steps)
		default:
			reasoning = -1
			step.Title = firstNonEmpty(event.SourceType, event.Kind)
			step.Details["text"] = event.Text
		}
		if len(out.Steps) >= 1000 {
			out.Coverage["truncated"] = true
			// Preserve all raw events even when the display projection is bounded.
			reasoning = -1
			delete(tools, event.ToolID)
			continue
		}
		out.Steps = append(out.Steps, step)
	}
	if scanner.Err() != nil {
		out.Coverage["localArchiveStatus"] = "incomplete"
	}
	out.Coverage["localEventCount"] = count
	// Reopening a completed trace need not rescan every streamed chunk. Cache
	// serialized projections to avoid sharing mutable maps with callers, bounded
	// to eight entries of at most 2 MiB each. The full JSONL stays on disk.
	if count > 0 && scanner.Err() == nil {
		if data, err := json.Marshal(out); err == nil && len(data) <= 2<<20 {
			c.projectionMu.Lock()
			if len(c.projections) >= 8 || c.projections == nil {
				c.projections = map[string]archiveProjection{}
			}
			c.projections[path] = archiveProjection{size: info.Size(), modified: info.ModTime(), data: data}
			c.projectionMu.Unlock()
		}
	}
	return out, count > 0
}

func sameExecutionStep(a, b Step) bool {
	if a.StepID == b.StepID {
		return true
	}
	toolA, toolB := stringValue(a.Correlation["toolId"]), stringValue(b.Correlation["toolId"])
	if toolB == "" {
		toolB = stringValue(mapValue(b.Correlation["worker"])["commandId"])
	}
	if toolA != "" && toolA == toolB {
		return true
	}
	return a.SourceEventID != "" && a.SourceEventID == b.SourceEventID && a.Kind == b.Kind
}

func withLocalEvents(remote, local Trace) Trace {
	if remote.SessionID == "" {
		return local
	}
	for _, step := range local.Steps {
		found := false
		for index, existing := range remote.Steps {
			if sameExecutionStep(step, existing) {
				existing.Details = safeMap(existing.Details)
				for key, value := range step.Details {
					if key != "projectionSource" && existing.Details[key] == nil {
						existing.Details[key] = value
					}
				}
				if existing.DurationMS == nil {
					existing.DurationMS = step.DurationMS
				}
				existing.StartedAt = firstNonEmpty(existing.StartedAt, step.StartedAt)
				existing.EndedAt = firstNonEmpty(existing.EndedAt, step.EndedAt)
				remote.Steps[index] = existing
				found = true
				break
			}
		}
		if !found {
			step.Sequence = len(remote.Steps) + 1
			remote.Steps = append(remote.Steps, step)
		}
	}
	remote.Coverage = safeMap(remote.Coverage)
	remote.Coverage["localEventCount"] = local.Coverage["localEventCount"]
	if status := local.Coverage["localArchiveStatus"]; status != nil {
		remote.Coverage["localArchiveStatus"] = status
	}
	return remote
}
