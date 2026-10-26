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
	ID           string `json:"id"`
	InvocationID string `json:"invocationId"`
	Author       string `json:"author"`
	Partial      bool   `json:"partial"`
	Terminal     bool   `json:"terminal"`
	ErrorCode    string `json:"errorCode"`
	Content      struct {
		Parts []struct {
			Text         string `json:"text"`
			Thought      bool   `json:"thought"`
			FunctionCall *struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"functionCall"`
			FunctionResponse *struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"functionResponse"`
		} `json:"parts"`
	} `json:"content"`
}

func Project(data []byte, kind string, r dto.SubscribeRequest) ([]dto.ClientEvent, error) {
	var wire adkEvent
	if json.Unmarshal(data, &wire) != nil {
		return nil, fmt.Errorf("invalid event")
	}
	base := dto.ClientEvent{SchemaVersion: 1, ConnectionEpoch: r.ConnectionEpoch, SubscriptionEpoch: r.SubscriptionEpoch, SessionID: r.SessionID, InvocationID: wire.InvocationID, EventID: wire.ID, Author: wire.Author, Kind: kind, Completeness: "complete", Partial: wire.Partial, Terminal: wire.Terminal}
	if base.InvocationID == "" {
		base.InvocationID = r.InvocationID
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
		if wire.InvocationID != r.InvocationID {
			base.Terminal = false
		}
		return []dto.ClientEvent{base}, nil
	}
	if kind == "error" || wire.ErrorCode != "" {
		if wire.InvocationID != r.InvocationID {
			base.Terminal = false
		}
		base.Kind = "error"
		base.EventID = ""
		base.Error = dto.Failure("streamError", "runtimeIngress", "history", 0)
		return []dto.ClientEvent{base}, nil
	}
	out := []dto.ClientEvent{}
	for index, p := range wire.Content.Parts {
		base.BlockID = wire.ID + "/part/" + strconv.Itoa(index)
		if p.Thought {
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
			e.Kind = "toolCall"
			e.ToolID = p.FunctionCall.ID
			e.ToolName = p.FunctionCall.Name
			out = append(out, e)
		}
		if p.FunctionResponse != nil {
			e := base
			e.Kind = "toolResult"
			e.ToolID = p.FunctionResponse.ID
			e.ToolName = p.FunctionResponse.Name
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		base.Kind = "unknown"
		out = append(out, base)
	}
	return out, nil
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
