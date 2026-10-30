package host

import (
	"github.com/egoist/mygo"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"net/url"
	"sync"
)

type CloseRequest struct {
	RequestID uint64 `json:"requestId"`
}
type CloseState struct {
	Pending   bool   `json:"pending"`
	RequestID uint64 `json:"requestId"`
}
type CloseResolution struct {
	RequestID uint64 `json:"requestId"`
	Allow     bool   `json:"allow"`
}
type CloseStateResult struct {
	OK    bool           `json:"ok"`
	Value *CloseState    `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}

var CloseRequested = mygo.NewEvent[CloseRequest]("CloseRequested")

type Host struct {
	mu        sync.Mutex
	pending   bool
	requestID uint64
	quit      func()
	canClose  func() bool
	runOnMain func(func())
	emitClose func(*mygo.Window, CloseRequest)
	rail      *nativeRail
}

func New() *Host {
	return &Host{
		runOnMain: mygo.RunOnMain,
		emitClose: func(w *mygo.Window, r CloseRequest) {
			CloseRequested.Emit(w, r)
		},
	}
}
func SetCanClose(h *Host, canClose func() bool) {
	h.mu.Lock()
	h.canClose = canClose
	h.mu.Unlock()
}
func Attach(h *Host, w *mygo.Window) {
	h.mu.Lock()
	h.quit = func() { w.Destroy(); mygo.App.Quit() }
	h.mu.Unlock()
	w.OnClose(func(e *mygo.CloseEvent) { e.PreventDefault(); RequestClose(h, w) })
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		h.mu.Lock()
		allowed := h.quit == nil
		h.mu.Unlock()
		if !allowed {
			e.PreventDefault()
			RequestClose(h, w)
		}
	})
}
func RequestClose(h *Host, w *mygo.Window) {
	h.mu.Lock()
	canClose := h.canClose
	quit := h.quit
	h.mu.Unlock()
	if canClose != nil && canClose() {
		h.mu.Lock()
		quit = h.quit
		h.quit = nil
		runOnMain := h.runOnMain
		h.mu.Unlock()
		if quit != nil {
			runOnMain(quit)
		}
		return
	}
	h.mu.Lock()
	if !h.pending {
		h.pending = true
		h.requestID++
	}
	id := h.requestID
	emitClose := h.emitClose
	h.mu.Unlock()
	emitClose(w, CloseRequest{RequestID: id})
}
func (h *Host) CloseState() CloseStateResult {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := CloseState{Pending: h.pending, RequestID: h.requestID}
	return CloseStateResult{OK: true, Value: &s}
}
func (h *Host) ResolveClose(r CloseResolution) dto.EmptyResult {
	h.mu.Lock()
	if !h.pending || r.RequestID != h.requestID {
		h.mu.Unlock()
		return dto.EmptyResult{Error: dto.Failure("staleCloseRequest", "transport", "never", 0)}
	}
	h.pending = false
	quit := h.quit
	runOnMain := h.runOnMain
	if r.Allow {
		h.quit = nil
	}
	h.mu.Unlock()
	if r.Allow && quit != nil {
		runOnMain(quit)
	}
	return dto.EmptyResult{OK: true, Value: &dto.Empty{}}
}

// RestrictNavigation keeps third party content outside the trusted IPC window.
func RestrictNavigation(w *mygo.Window) {
	RestrictPageNavigation(w.Page())
}
func RestrictPageNavigation(page *mygo.Page) {
	page.OnWillNavigate(func(e *mygo.NavigateEvent) {
		if !OwnNavigation(e.URL, mygo.App.IsPackaged()) {
			e.PreventDefault()
			if ExternalNavigation(e.URL) {
				go mygo.Shell.OpenExternal(e.URL)
			}
		}
	})
	page.SetWindowOpenHandler(func(r mygo.WindowOpenRequest) *mygo.WindowOptions {
		if ExternalNavigation(r.URL) {
			go mygo.Shell.OpenExternal(r.URL)
		}
		return nil
	})
}

func RestrictPermissions(w *mygo.Window) {
	RestrictPagePermissions(w.Page())
}
func RestrictPagePermissions(page *mygo.Page) {
	page.SetPermissionHandler(func(mygo.PermissionRequest) bool { return false })
}
func OwnNavigation(raw string, packaged bool) bool {
	u, e := url.Parse(raw)
	if e != nil || u.User != nil {
		return false
	}
	if u.Scheme == "mygo" && u.Host == "localhost" {
		return true
	}
	return !packaged && u.Scheme == "http" && u.Host == "127.0.0.1:5173"
}
func ExternalNavigation(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.User == nil && u.Host != "" && (u.Scheme == "https" || u.Scheme == "http")
}
