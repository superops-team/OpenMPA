package dto

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"
)

// UnmarshalJSON accepts server revision numbers while keeping a stable string UI field.
func (s *CurrentState) UnmarshalJSON(data []byte) error {
	if !uniqueStateFields(data) {
		return errors.New("invalid state object")
	}
	var wire struct {
		MemoryRef              string          `json:"memoryRef"`
		OperationRef           string          `json:"operationRef"`
		OperationState         string          `json:"operationState"`
		ProviderState          string          `json:"providerState"`
		ExecutionConfigVersion json.RawMessage `json:"executionConfigVersion"`
		Status                 string          `json:"status"`
		Generation             int             `json:"generation"`
		Version                int             `json:"version"`
		ID                     string          `json:"id"`
		Enabled                *bool           `json:"enabled"`
	}
	if e := json.Unmarshal(data, &wire); e != nil {
		return e
	}
	if (wire.MemoryRef != "" && !memoryReference.MatchString(wire.MemoryRef)) || (wire.OperationRef != "" && !memoryOperationReference.MatchString(wire.OperationRef)) || !validMemoryState(wire.OperationState) || !validProviderState(wire.ProviderState) {
		return errors.New("invalid memory state")
	}
	if (wire.OperationRef != "" || wire.OperationState != "") && (wire.MemoryRef == "" || wire.OperationRef == "" || wire.OperationState == "" || wire.ProviderState == "") {
		return errors.New("incomplete memory operation state")
	}
	out := CurrentState{MemoryRef: wire.MemoryRef, OperationRef: wire.OperationRef, OperationState: wire.OperationState, ProviderState: wire.ProviderState, Status: wire.Status, Generation: wire.Generation, Version: wire.Version, ID: wire.ID, Enabled: wire.Enabled}
	if len(wire.ExecutionConfigVersion) > 0 && string(wire.ExecutionConfigVersion) != "null" {
		var text string
		if e := json.Unmarshal(wire.ExecutionConfigVersion, &text); e == nil {
			out.ExecutionConfigVersion = text
		} else {
			var number int
			if e = json.Unmarshal(wire.ExecutionConfigVersion, &number); e != nil {
				return e
			}
			out.ExecutionConfigVersion = strconv.Itoa(number)
		}
	}
	*s = out
	return nil
}

var memoryReference = regexp.MustCompile(`^(m_[a-f0-9]{32}|r_[a-f0-9]{64})$`)
var memoryOperationReference = regexp.MustCompile(`^mop_[a-f0-9]{64}$`)

func validMemoryState(s string) bool {
	switch s {
	case "", "pending", "accepted", "committed", "failed", "deleted", "unknown":
		return true
	}
	return false
}
func validProviderState(s string) bool {
	switch s {
	case "", "unknown", "pending", "confirmed", "available", "unavailable", "degraded", "failed", "deleted":
		return true
	}
	return false
}

func uniqueStateFields(data []byte) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	token, e := d.Token()
	if e != nil || token != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for d.More() {
		token, e = d.Token()
		if e != nil {
			return false
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return false
		}
		seen[key] = true
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return false
		}
	}
	token, e = d.Token()
	return e == nil && token == json.Delim('}') && d.Decode(new(interface{})) == io.EOF
}
