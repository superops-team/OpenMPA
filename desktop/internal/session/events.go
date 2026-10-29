package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"io"
	"strconv"
	"strings"
)

var ErrStreamInterrupted = errors.New("stream ended before terminal event")

type adkEvent struct {
	ID                 string                  `json:"id"`
	EventID            string                  `json:"eventId"`
	InvocationID       string                  `json:"invocationId"`
	Author             string                  `json:"author"`
	Type               string                  `json:"type"`
	SourceType         string                  `json:"sourceType"`
	Partial            bool                    `json:"partial"`
	Terminal           bool                    `json:"terminal"`
	ErrorCode          string                  `json:"errorCode"`
	Name               string                  `json:"name"`
	ToolName           string                  `json:"toolName"`
	ToolType           string                  `json:"toolType"`
	ToolID             string                  `json:"toolId"`
	CommandID          string                  `json:"commandId"`
	CallID             string                  `json:"callId"`
	Status             string                  `json:"status"`
	StartedAt          string                  `json:"startedAt"`
	EndedAt            string                  `json:"endedAt"`
	DurationMS         *float64                `json:"durationMs"`
	SafeCommandSummary *dto.SafeCommandSummary `json:"safeCommandSummary"`
	OutputPreview      string                  `json:"outputPreview"`
	StderrPreview      string                  `json:"stderrPreview"`
	Text               string                  `json:"text"`
	Message            string                  `json:"message"`
	SafeText           string                  `json:"safeText"`
	Delta              string                  `json:"delta"`
	FinalMessage       string                  `json:"finalMessage"`
	Payload            *eventPayload           `json:"payload"`
	Tool               *eventTool              `json:"tool"`
	Content            struct {
		Parts []struct {
			Text         string `json:"text"`
			Thought      bool   `json:"thought"`
			FunctionCall *struct {
				ID   string          `json:"id"`
				Name string          `json:"name"`
				Args json.RawMessage `json:"args"`
			} `json:"functionCall"`
			FunctionResponse *struct {
				ID       string          `json:"id"`
				Name     string          `json:"name"`
				Response json.RawMessage `json:"response"`
			} `json:"functionResponse"`
		} `json:"parts"`
	} `json:"content"`
}
type eventTool struct {
	Name        string `json:"name"`
	ToolName    string `json:"toolName"`
	ToolType    string `json:"toolType"`
	Status      string `json:"status"`
	Preview     string `json:"preview"`
	SafePreview string `json:"safePreview"`
	CommandID   string `json:"commandId"`
	CallID      string `json:"callId"`
	ItemID      string `json:"itemId"`
}
type eventPayload struct {
	ID                 string                  `json:"id"`
	EventID            string                  `json:"eventId"`
	InvocationID       string                  `json:"invocationId"`
	TurnID             string                  `json:"turnId"`
	Name               string                  `json:"name"`
	ToolName           string                  `json:"toolName"`
	ToolType           string                  `json:"toolType"`
	ToolID             string                  `json:"toolId"`
	CommandID          string                  `json:"commandId"`
	CallID             string                  `json:"callId"`
	ItemID             string                  `json:"itemId"`
	Status             string                  `json:"status"`
	StartedAt          string                  `json:"startedAt"`
	EndedAt            string                  `json:"endedAt"`
	DurationMS         *float64                `json:"durationMs"`
	SafeCommandSummary *dto.SafeCommandSummary `json:"safeCommandSummary"`
	OutputPreview      string                  `json:"outputPreview"`
	StderrPreview      string                  `json:"stderrPreview"`
	SafePreview        string                  `json:"safePreview"`
	Output             string                  `json:"output"`
	Delta              string                  `json:"delta"`
	Message            string                  `json:"message"`
	SafeText           string                  `json:"safeText"`
	FinalMessage       string                  `json:"finalMessage"`
	ErrorMessage       string                  `json:"errorMessage"`
	IsTurnTerminal     bool                    `json:"isTurnTerminal"`
	InputTokens        *float64                `json:"inputTokens"`
	OutputTokens       *float64                `json:"outputTokens"`
	TotalTokens        *float64                `json:"totalTokens"`
	InputTokensSnake   *float64                `json:"input_tokens"`
	OutputTokensSnake  *float64                `json:"output_tokens"`
	TotalTokensSnake   *float64                `json:"total_tokens"`
	ModelID            string                  `json:"modelId"`
}
type eventMetadata struct {
	SourceType         string
	Text               string
	ToolID             string
	ToolName           string
	ToolStatus         string
	ToolSubject        string
	StartedAt          string
	EndedAt            string
	DurationMS         *float64
	SafeCommandSummary *dto.SafeCommandSummary
	OutputPreview      string
	StderrPreview      string
	Terminal           bool
}

func Project(data []byte, kind string, r dto.SubscribeRequest) ([]dto.ClientEvent, error) {
	var wire adkEvent
	if json.Unmarshal(data, &wire) != nil {
		return nil, fmt.Errorf("invalid event")
	}
	id := firstNonEmpty(wire.ID, wire.EventID)
	base := dto.ClientEvent{SchemaVersion: 2, ConnectionEpoch: r.ConnectionEpoch, SubscriptionEpoch: r.SubscriptionEpoch, SessionID: r.SessionID, InvocationID: firstNonEmpty(wire.InvocationID, payloadValue(wire.Payload, func(p *eventPayload) string { return p.InvocationID })), EventID: id, Author: wire.Author, Kind: kind, Completeness: "complete", Partial: wire.Partial, Terminal: wire.Terminal}
	if base.InvocationID == "" {
		base.InvocationID = r.InvocationID
	}
	metadata := wire.metadata()
	base.SourceType = boundedDisplayText(metadata.SourceType, 120)
	base.StartedAt = metadata.StartedAt
	base.EndedAt = metadata.EndedAt
	base.DurationMS = metadata.DurationMS
	base.SafeCommandSummary = metadata.SafeCommandSummary
	base.OutputPreview = metadata.OutputPreview
	base.StderrPreview = metadata.StderrPreview
	if metadata.Terminal {
		base.Terminal = true
	}
	if wire.Partial {
		base.Completeness = "partial"
	}
	if kind == "heartbeat" {
		base.EventID = ""
		return []dto.ClientEvent{base}, nil
	}
	if kind == "done" {
		base.EventID = ""
		if base.InvocationID != r.InvocationID {
			base.Terminal = false
		}
		return []dto.ClientEvent{base}, nil
	}
	if kind == "error" || wire.ErrorCode != "" {
		if base.InvocationID != r.InvocationID {
			base.Terminal = false
		}
		base.Kind = "error"
		base.EventID = ""
		base.Error = dto.Failure("streamError", "runtimeIngress", "history", 0)
		return []dto.ClientEvent{base}, nil
	}
	if structured := projectStructuredEvent(base, metadata); len(wire.Content.Parts) == 0 && len(structured) > 0 {
		return structured, nil
	}
	out := []dto.ClientEvent{}
	for index, p := range wire.Content.Parts {
		base.BlockID = wire.ID + "/part/" + strconv.Itoa(index)
		partIndex := index
		base.PartIndex = &partIndex
		if p.Thought {
			e := base
			e.Kind = "reasoning"
			e.ReasoningVisibility = "summary"
			e.Text = p.Text
			out = append(out, e)
			continue
		}
		if p.Text != "" {
			e := base
			e.Kind = "text"
			e.Text = p.Text
			out = append(out, e)
		}
		if p.FunctionCall != nil {
			e := base
			metadata := mergeMetadata(baseMetadata(base), metadataFromRaw(p.FunctionCall.Args))
			e.Kind = "toolCall"
			e.ToolID = p.FunctionCall.ID
			e.ToolName = p.FunctionCall.Name
			e.ToolStatus = "running"
			e.StartedAt = metadata.StartedAt
			e.EndedAt = metadata.EndedAt
			e.DurationMS = metadata.DurationMS
			e.SafeCommandSummary = metadata.SafeCommandSummary
			e.OutputPreview = metadata.OutputPreview
			e.StderrPreview = metadata.StderrPreview
			e.ToolSubject = firstNonEmpty(safeCommandSubject(metadata.SafeCommandSummary), safeToolSubject(p.FunctionCall.Name, p.FunctionCall.Args))
			out = append(out, e)
		}
		if p.FunctionResponse != nil {
			e := base
			metadata := mergeMetadata(baseMetadata(base), metadataFromRaw(p.FunctionResponse.Response))
			e.Kind = "toolResult"
			e.ToolID = p.FunctionResponse.ID
			e.ToolName = p.FunctionResponse.Name
			e.ToolStatus = normalizedToolStatus(metadata.ToolStatus, true)
			e.StartedAt = metadata.StartedAt
			e.EndedAt = metadata.EndedAt
			e.DurationMS = metadata.DurationMS
			e.SafeCommandSummary = metadata.SafeCommandSummary
			e.OutputPreview = metadata.OutputPreview
			e.StderrPreview = metadata.StderrPreview
			e.ToolSubject = firstNonEmpty(safeCommandSubject(metadata.SafeCommandSummary), metadata.ToolSubject)
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		base.Kind = "unknown"
		out = append(out, base)
	}
	return out, nil
}

func projectStructuredEvent(base dto.ClientEvent, metadata eventMetadata) []dto.ClientEvent {
	source := strings.ToLower(metadata.SourceType)
	if strings.HasPrefix(source, "codex.tool.") || metadata.ToolName != "" {
		e := base
		if strings.Contains(source, ".call") || strings.EqualFold(metadata.ToolStatus, "started") || strings.EqualFold(metadata.ToolStatus, "running") {
			e.Kind = "toolCall"
		} else {
			e.Kind = "toolResult"
		}
		e.ToolID = firstNonEmpty(metadata.ToolID, e.EventID)
		e.ToolName = firstNonEmpty(metadata.ToolName, "tool")
		e.ToolStatus = normalizedToolStatus(metadata.ToolStatus, e.Kind == "toolResult")
		e.ToolSubject = firstNonEmpty(metadata.ToolSubject, safeCommandSubject(metadata.SafeCommandSummary))
		return []dto.ClientEvent{e}
	}
	if strings.Contains(source, "usage") {
		e := base
		e.Kind = "text"
		e.Text = firstNonEmpty(metadata.Text, "[usage]")
		return []dto.ClientEvent{e}
	}
	if source == "codex.turn.completed" {
		e := base
		e.Kind = "done"
		e.Terminal = true
		return []dto.ClientEvent{e}
	}
	if metadata.Text != "" {
		e := base
		e.Kind = "text"
		e.Text = metadata.Text
		if strings.Contains(source, "message_delta") {
			e.Partial = true
			e.Completeness = "partial"
		}
		return []dto.ClientEvent{e}
	}
	return nil
}

func (e adkEvent) metadata() eventMetadata {
	payload := e.Payload
	source := firstNonEmpty(e.SourceType, e.Type)
	toolName := firstNonEmpty(
		payloadValue(payload, func(p *eventPayload) string { return p.ToolName }),
		payloadValue(payload, func(p *eventPayload) string { return p.Name }),
		payloadValue(payload, func(p *eventPayload) string { return p.ToolType }),
		toolValue(e.Tool, func(t *eventTool) string { return t.ToolName }),
		toolValue(e.Tool, func(t *eventTool) string { return t.Name }),
		toolValue(e.Tool, func(t *eventTool) string { return t.ToolType }),
		e.ToolName,
		e.Name,
		e.ToolType,
	)
	toolID := firstNonEmpty(
		payloadValue(payload, func(p *eventPayload) string { return p.CommandID }),
		payloadValue(payload, func(p *eventPayload) string { return p.CallID }),
		payloadValue(payload, func(p *eventPayload) string { return p.ItemID }),
		payloadValue(payload, func(p *eventPayload) string { return p.ToolID }),
		toolValue(e.Tool, func(t *eventTool) string { return t.CommandID }),
		toolValue(e.Tool, func(t *eventTool) string { return t.CallID }),
		toolValue(e.Tool, func(t *eventTool) string { return t.ItemID }),
		e.CommandID,
		e.CallID,
		e.ToolID,
	)
	status := firstNonEmpty(
		payloadValue(payload, func(p *eventPayload) string { return p.Status }),
		toolValue(e.Tool, func(t *eventTool) string { return t.Status }),
		e.Status,
	)
	duration := firstDuration(
		payloadDuration(payload, func(p *eventPayload) *float64 { return p.DurationMS }),
		e.DurationMS,
	)
	summary := firstSummary(
		payloadSummary(payload, func(p *eventPayload) *dto.SafeCommandSummary { return p.SafeCommandSummary }),
		e.SafeCommandSummary,
	)
	outputPreview := firstNonEmpty(
		payloadValue(payload, func(p *eventPayload) string { return p.OutputPreview }),
		payloadValue(payload, func(p *eventPayload) string { return p.Output }),
		payloadValue(payload, func(p *eventPayload) string { return p.SafePreview }),
		toolValue(e.Tool, func(t *eventTool) string { return t.Preview }),
		toolValue(e.Tool, func(t *eventTool) string { return t.SafePreview }),
		e.OutputPreview,
	)
	text := firstNonEmpty(
		payloadValue(payload, func(p *eventPayload) string { return p.FinalMessage }),
		payloadValue(payload, func(p *eventPayload) string { return p.Message }),
		payloadValue(payload, func(p *eventPayload) string { return p.SafeText }),
		payloadValue(payload, func(p *eventPayload) string { return p.Delta }),
		e.FinalMessage,
		e.Message,
		e.SafeText,
		e.Text,
	)
	if strings.Contains(strings.ToLower(source), "usage") && text == "" {
		text = usageText(payload)
	}
	return eventMetadata{
		SourceType:         source,
		Text:               boundedDisplayText(text, 12000),
		ToolID:             boundedDisplayText(toolID, 120),
		ToolName:           boundedDisplayText(toolName, 120),
		ToolStatus:         status,
		ToolSubject:        safeCommandSubject(summary),
		StartedAt:          firstNonEmpty(payloadValue(payload, func(p *eventPayload) string { return p.StartedAt }), e.StartedAt),
		EndedAt:            firstNonEmpty(payloadValue(payload, func(p *eventPayload) string { return p.EndedAt }), e.EndedAt),
		DurationMS:         duration,
		SafeCommandSummary: sanitizeSafeCommandSummary(summary),
		OutputPreview:      boundedPreviewText(outputPreview, 240),
		StderrPreview:      boundedPreviewText(firstNonEmpty(payloadValue(payload, func(p *eventPayload) string { return p.StderrPreview }), e.StderrPreview), 240),
		Terminal:           payloadTerminal(payload) || e.Terminal,
	}
}

func baseMetadata(e dto.ClientEvent) eventMetadata {
	return eventMetadata{
		StartedAt:          e.StartedAt,
		EndedAt:            e.EndedAt,
		DurationMS:         e.DurationMS,
		SafeCommandSummary: e.SafeCommandSummary,
		OutputPreview:      e.OutputPreview,
		StderrPreview:      e.StderrPreview,
	}
}

func metadataFromRaw(raw json.RawMessage) eventMetadata {
	if len(raw) == 0 {
		return eventMetadata{}
	}
	var payload eventPayload
	if json.Unmarshal(raw, &payload) != nil {
		return eventMetadata{}
	}
	summary := sanitizeSafeCommandSummary(payload.SafeCommandSummary)
	return eventMetadata{
		ToolStatus:         payload.Status,
		ToolSubject:        safeCommandSubject(summary),
		StartedAt:          payload.StartedAt,
		EndedAt:            payload.EndedAt,
		DurationMS:         payload.DurationMS,
		SafeCommandSummary: summary,
		OutputPreview:      boundedPreviewText(firstNonEmpty(payload.OutputPreview, payload.Output, payload.SafePreview), 240),
		StderrPreview:      boundedPreviewText(payload.StderrPreview, 240),
	}
}

func mergeMetadata(primary, secondary eventMetadata) eventMetadata {
	return eventMetadata{
		ToolStatus:         firstNonEmpty(secondary.ToolStatus, primary.ToolStatus),
		ToolSubject:        firstNonEmpty(secondary.ToolSubject, primary.ToolSubject),
		StartedAt:          firstNonEmpty(secondary.StartedAt, primary.StartedAt),
		EndedAt:            firstNonEmpty(secondary.EndedAt, primary.EndedAt),
		DurationMS:         firstDuration(secondary.DurationMS, primary.DurationMS),
		SafeCommandSummary: firstSummary(secondary.SafeCommandSummary, primary.SafeCommandSummary),
		OutputPreview:      firstNonEmpty(secondary.OutputPreview, primary.OutputPreview),
		StderrPreview:      firstNonEmpty(secondary.StderrPreview, primary.StderrPreview),
	}
}

func safeToolSubject(name string, raw json.RawMessage) string {
	var args map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &args) != nil {
		return ""
	}
	var keys []string
	switch strings.ToLower(name) {
	case "read", "read_file", "write", "write_file", "edit", "edit_file":
		keys = []string{"file_path", "path"}
	case "search", "grep", "find":
		keys = []string{"query", "pattern"}
	case "bash", "shell", "run", "run_command":
		keys = []string{"description"}
	default:
		return ""
	}
	for _, key := range keys {
		var value string
		if candidate, ok := args[key]; ok && json.Unmarshal(candidate, &value) == nil {
			if unsafeDisplaySubject(value) {
				return ""
			}
			return boundedDisplayText(value, 240)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func payloadValue(payload *eventPayload, read func(*eventPayload) string) string {
	if payload == nil {
		return ""
	}
	return read(payload)
}

func toolValue(tool *eventTool, read func(*eventTool) string) string {
	if tool == nil {
		return ""
	}
	return read(tool)
}

func payloadDuration(payload *eventPayload, read func(*eventPayload) *float64) *float64 {
	if payload == nil {
		return nil
	}
	return read(payload)
}

func payloadSummary(payload *eventPayload, read func(*eventPayload) *dto.SafeCommandSummary) *dto.SafeCommandSummary {
	if payload == nil {
		return nil
	}
	return read(payload)
}

func payloadTerminal(payload *eventPayload) bool {
	return payload != nil && payload.IsTurnTerminal
}

func firstDuration(values ...*float64) *float64 {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstSummary(values ...*dto.SafeCommandSummary) *dto.SafeCommandSummary {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func normalizedToolStatus(status string, result bool) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "preparing", "queued":
		return "preparing"
	case "started", "start", "running", "in_progress", "inprogress":
		return "running"
	case "completed", "complete", "success", "succeeded", "ok":
		return "success"
	case "failed", "failure", "error":
		return "failed"
	case "stopped", "stop", "canceled", "cancelled", "aborted":
		return "stopped"
	default:
		if result {
			return "success"
		}
		return "running"
	}
}

func safeCommandSubject(summary *dto.SafeCommandSummary) string {
	summary = sanitizeSafeCommandSummary(summary)
	if summary == nil || summary.CommandPreview == "" {
		return ""
	}
	return summary.CommandPreview
}

func sanitizeSafeCommandSummary(summary *dto.SafeCommandSummary) *dto.SafeCommandSummary {
	if summary == nil {
		return nil
	}
	out := *summary
	out.Binary = boundedDisplayText(out.Binary, 80)
	if unsafeDisplaySubject(out.Binary) {
		out.Binary = ""
	}
	out.CommandPreview = boundedDisplayText(out.CommandPreview, 360)
	if unsafeDisplaySubject(out.CommandPreview) {
		out.CommandPreview = ""
	}
	out.Parameters = nil
	for _, parameter := range summary.Parameters {
		if len(out.Parameters) >= 16 {
			break
		}
		name := boundedDisplayText(parameter.Name, 80)
		value := boundedDisplayText(parameter.Value, 240)
		if name == "" || unsafeDisplaySubject(name) || unsafeDisplaySubject(value) {
			continue
		}
		out.Parameters = append(out.Parameters, dto.SafeCommandParameter{Name: name, Value: value})
	}
	if out.Binary == "" && out.CommandPreview == "" && out.ArgumentCount == nil && out.Redacted == nil && len(out.Parameters) == 0 {
		return nil
	}
	return &out
}

func usageText(payload *eventPayload) string {
	if payload == nil {
		return "[usage]"
	}
	total := firstNumber(payload.TotalTokens, payload.TotalTokensSnake)
	input := firstNumber(payload.InputTokens, payload.InputTokensSnake)
	output := firstNumber(payload.OutputTokens, payload.OutputTokensSnake)
	if total == "" && input == "" && output == "" {
		return "[usage]"
	}
	model := boundedDisplayText(payload.ModelID, 120)
	if model != "" {
		model += " "
	}
	if total != "" {
		return "[usage] " + model + total + " tokens"
	}
	return "[usage] " + model + input + " input / " + output + " output tokens"
}

func firstNumber(values ...*float64) string {
	for _, value := range values {
		if value == nil {
			continue
		}
		return strconv.FormatFloat(*value, 'f', -1, 64)
	}
	return ""
}

func unsafeDisplaySubject(value string) bool {
	normalized := strings.ToLower(value)
	if strings.Contains(normalized, "://") {
		return true
	}
	for _, marker := range []string{
		"authorization", "bearer ", "token=", "secret=", "password=", "api_key", "apikey", "cookie=",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func boundedDisplayText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func boundedPreviewText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}
func ParseSSE(reader io.Reader, r dto.SubscribeRequest, send func(dto.ClientEvent) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	kind := "message"
	terminal := false
	data := []string{}
	size := 0
	dispatch := func() error {
		if len(data) == 0 {
			return nil
		}
		events, e := Project([]byte(strings.Join(data, "\n")), kind, r)
		if e != nil {
			return e
		}
		for _, event := range events {
			if (event.Kind == "done" || event.Kind == "error") && event.Terminal && event.InvocationID == r.InvocationID {
				terminal = true
			}
			if e = send(event); e != nil {
				return e
			}
		}
		return nil
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if e := dispatch(); e != nil {
				return e
			}
			if terminal {
				return nil
			}
			kind = "message"
			data = nil
			size = 0
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			kind = value
		case "data":
			size += len(value)
			if size > 4<<20 {
				return fmt.Errorf("event too large")
			}
			data = append(data, value)
		}
	}
	if e := scanner.Err(); e != nil {
		return e
	}
	// SSE dispatch requires a blank line. EOF must not publish an unfinished frame.

	if !terminal {
		return ErrStreamInterrupted
	}
	return nil
}
