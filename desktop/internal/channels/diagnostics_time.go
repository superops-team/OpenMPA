package channels

import (
	"bytes"
	"encoding/json"
	"math"
	"time"
)

// Observation times are Unix seconds in Runtime and RFC3339 in older clients.
// Keep the IPC projection a normalized string rather than exposing mixed types.
func diagnosticTime(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", true
	}
	var value string
	if raw[0] == '"' {
		if json.Unmarshal(raw, &value) != nil {
			return "", false
		}
		if value == "" {
			return "", true
		}
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return "", false
		}
		return parsed.UTC().Format(time.RFC3339Nano), true
	}
	var seconds float64
	if json.Unmarshal(raw, &seconds) != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < -62135596800 || seconds >= 253402300800 {
		return "", false
	}
	whole, fraction := math.Modf(seconds)
	parsed := time.Unix(int64(whole), int64(math.Round(fraction*1e9))).UTC()
	if parsed.Year() < 1 || parsed.Year() > 9999 {
		return "", false
	}
	return parsed.Format(time.RFC3339Nano), true
}
