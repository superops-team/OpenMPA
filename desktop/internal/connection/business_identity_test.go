package connection

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBusinessIdentityErrorsKeepOnlyExactKnownMessages(t *testing.T) {
	for _, tc := range []struct {
		body, code string
		status     int
	}{
		{`{"detail":"x-user-id header is required"}`, "businessIdentityRequired", 422},
		{`{"detail":"X-Jwt-Token header is required"}`, "businessJWTRequired", 401},
		{`{"detail":"SECRET x-user-id header is required"}`, "invalidInput", 422},
		{`{"detail":[{"input":"SECRET"}]}`, "invalidInput", 422},
		{`{"detail":"x-user-id header is required"}`, "denied", 403},
	} {
		err := ErrorFromResponse(tc.status, []byte(tc.body))
		raw, _ := json.Marshal(err)
		if err.Code != tc.code || err.HTTPStatus != tc.status || strings.Contains(string(raw), "SECRET") {
			t.Fatalf("unsafe or incorrect projection: %s", raw)
		}
	}
}
