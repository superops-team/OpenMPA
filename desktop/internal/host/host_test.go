package host

import (
	"github.com/egoist/mygo"
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
	h.runOnMain = func(fn func()) { fn() }
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

func TestRequestCloseAllowsSafeCloseWithoutPendingConfirmation(t *testing.T) {
	h := New()
	quitCount := 0
	emitted := 0
	h.quit = func() { quitCount++ }
	h.runOnMain = func(fn func()) { fn() }
	h.emitClose = func(*mygo.Window, CloseRequest) { emitted++ }
	SetCanClose(h, func() bool { return true })
	RequestClose(h, nil)
	if quitCount != 1 {
		t.Fatalf("safe close did not quit, count=%d", quitCount)
	}
	if h.CloseState().Value.Pending {
		t.Fatal("safe close left a pending request")
	}
	if emitted != 0 {
		t.Fatalf("safe close emitted confirmation request, count=%d", emitted)
	}
}

func TestRequestCloseStillRequestsConfirmationWhenUnsafe(t *testing.T) {
	h := New()
	quitCount := 0
	var request CloseRequest
	h.quit = func() { quitCount++ }
	h.runOnMain = func(fn func()) { fn() }
	h.emitClose = func(_ *mygo.Window, r CloseRequest) { request = r }
	SetCanClose(h, func() bool { return false })
	RequestClose(h, nil)
	if quitCount != 0 {
		t.Fatalf("unsafe close quit without confirmation, count=%d", quitCount)
	}
	if !h.CloseState().Value.Pending {
		t.Fatal("unsafe close did not create a pending request")
	}
	if request.RequestID == 0 {
		t.Fatal("unsafe close did not emit a close request")
	}
}
