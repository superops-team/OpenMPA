package knowledge

import (
	"encoding/json"
	"errors"
	"time"
)

// This wire type is private and never crosses IPC. Pydantic's item schema always
// supplies these fields; missing flags or counters cannot become a READY item.
type itemWire struct{ value Item }

func (w *itemWire) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return errors.New("invalid item")
	}
	for _, key := range []string{"importId", "knowledgeId", "version", "sourceType", "displayName", "status", "chunkCount", "active", "retryable"} {
		field, ok := fields[key]
		if !ok || string(field) == "null" {
			return errors.New("missing item field")
		}
	}
	var v Item
	if e := json.Unmarshal(data, &v); e != nil {
		return e
	}
	w.value = v
	return nil
}

type listWire struct {
	Items []itemWire `json:"items"`
}

type contentWire struct{ value ContentPage }

func (w *contentWire) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return errors.New("invalid content")
	}
	for _, key := range []string{"knowledgeId", "version", "content", "visibleBytes", "completeness", "citation"} {
		field, ok := fields[key]
		if !ok || string(field) == "null" {
			return errors.New("missing content field")
		}
	}
	var v ContentPage
	if e := json.Unmarshal(data, &v); e != nil {
		return e
	}
	w.value = v
	return nil
}

func cleanDate(v **string) {
	if *v != nil {
		if _, e := time.Parse(time.RFC3339Nano, **v); e != nil {
			*v = nil
		}
	}
}
