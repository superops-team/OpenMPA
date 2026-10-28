package channels

import (
	"net/http"
	"testing"
)

func TestDiagnosticsAcceptsRuntimeObservationTimestamps(t *testing.T) {
	for _, tc := range []struct{ name, inbound, delivered, expected string }{
		{"unix-seconds", `1700000000.25`, `1700000001.5`, "2023-11-14T22:13:20.25Z"},
		{"rfc3339", `"2023-11-14T22:13:20.25Z"`, `"2023-11-14T22:13:21Z"`, "2023-11-14T22:13:20.25Z"},
		{"not-observed", `null`, `null`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, epoch := harness(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("appId") != "app-1" {
					t.Fatal("diagnostics lost exact account")
				}
				reply(w, `{"channel":"feishu","appId":"app-1","configured":true,"gatewayConfigured":true,"routeConfigured":true,"deliveryHealth":"delivered","lastInboundAt":`+tc.inbound+`,"lastDeliveryAt":`+tc.delivered+`}`)
			})
			out := svc.Diagnostics(AccountRequest{epoch, "feishu", "app-1"})
			if !out.OK || out.Value.LastInboundAt != tc.expected || out.Value.DeliveryHealth != "delivered" {
				t.Fatalf("timestamp rejected: %+v", out)
			}
		})
	}
}
func TestDiagnosticsStillRejectsAnotherBotWithNumericTimestamps(t *testing.T) {
	svc, epoch := harness(t, func(w http.ResponseWriter, r *http.Request) {
		reply(w, `{"channel":"feishu","appId":"other-bot","lastInboundAt":1700000000.25}`)
	})
	if svc.Diagnostics(AccountRequest{epoch, "feishu", "app-1"}).OK {
		t.Fatal("another Bot was accepted")
	}
}

func TestDiagnosticsRejectsInvalidObservationTypes(t *testing.T) {
	for _, raw := range []string{`true`, `{}`, `[]`, `"not-a-time"`, `1e100`} {
		svc, epoch := harness(t, func(w http.ResponseWriter, r *http.Request) {
			reply(w, `{"channel":"feishu","appId":"app-1","lastInboundAt":`+raw+`}`)
		})
		if svc.Diagnostics(AccountRequest{epoch, "feishu", "app-1"}).OK {
			t.Fatalf("invalid observation accepted: %s", raw)
		}
	}
}
