package host

import (
	"testing"
)

func TestNavigationScope(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "about:blank", "mygo://evil/desktop.html", "https://external.invalid", "http://127.0.0.1:5173.evil/", "javascript:alert(1)"} {
		if OwnNavigation(raw, false) {
			t.Errorf("trusted %s", raw)
		}
	}
	if !OwnNavigation("mygo://localhost/desktop.html", true) || !OwnNavigation("http://127.0.0.1:5173/desktop.html", false) || OwnNavigation("http://127.0.0.1:5173/desktop.html", true) {
		t.Fatal("own origin mismatch")
	}
}
func TestCloseResolutionRequiresCurrentRequest(t *testing.T) {
	h := New()
	if h.ResolveClose(CloseResolution{RequestID: 1, Allow: true}).OK {
		t.Fatal("unrequested close allowed")
	}
	h.pending = true
	h.requestID = 3
	if h.ResolveClose(CloseResolution{RequestID: 2, Allow: true}).OK {
		t.Fatal("stale close allowed")
	}
	if !h.ResolveClose(CloseResolution{RequestID: 3, Allow: false}).OK || h.CloseState().Value.Pending {
		t.Fatal("close denial not retained")
	}
}
