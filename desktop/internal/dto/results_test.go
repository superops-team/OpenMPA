package dto

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"strings"
	"testing"
)

func TestEmptyResultPreservesSuccessfulValueAcrossJSONVersions(t *testing.T) {
	result := EmptyResult{OK: true, Value: &Empty{}}
	for name, marshal := range map[string]func(interface{}) ([]byte, error){"v1": json.Marshal, "v2": func(v interface{}) ([]byte, error) { return jsonv2.Marshal(v) }} {
		t.Run(name, func(t *testing.T) {
			b, e := marshal(result)
			if e != nil || !strings.Contains(string(b), `"value":{}`) || strings.Contains(string(b), `"error"`) {
				t.Fatalf("success envelope %s %v", b, e)
			}
		})
	}
}
func TestEmptyResultFailureOmitsValueAcrossJSONVersions(t *testing.T) {
	result := EmptyResult{Error: Failure("denied", "mpa", "never", 403)}
	for name, marshal := range map[string]func(interface{}) ([]byte, error){"v1": json.Marshal, "v2": func(v interface{}) ([]byte, error) { return jsonv2.Marshal(v) }} {
		t.Run(name, func(t *testing.T) {
			b, e := marshal(result)
			if e != nil || strings.Contains(string(b), `"value"`) || !strings.Contains(string(b), `"ok":false`) || !strings.Contains(string(b), `"error"`) {
				t.Fatalf("failure envelope %s %v", b, e)
			}
		})
	}
}
