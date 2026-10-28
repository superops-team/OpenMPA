package runtimeterminal

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	myterminal "github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type TargetFunc func() dto.RuntimeRequest

type Service struct {
	Cloud      *cloud.Client
	Connection *connection.Client
	Target     TargetFunc

	mu           sync.Mutex
	windows      map[string]*terminalWindow
	dialer       websocketDialer
	pollInterval time.Duration
	waitTimeout  time.Duration
}

type IPC struct {
	service *Service
}

type OpenRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SessionID       string `json:"sessionId"`
	Title           string `json:"title,omitempty"`
}

type OpenReceipt struct {
	SessionID string `json:"sessionId"`
	RuntimeID string `json:"runtimeId"`
}

type OpenResult struct {
	OK    bool           `json:"ok"`
	Value *OpenReceipt   `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}

func New(cloudClient *cloud.Client, conn *connection.Client, target TargetFunc) *Service {
	return &Service{Cloud: cloudClient, Connection: conn, Target: target, windows: map[string]*terminalWindow{}, pollInterval: 3 * time.Second, waitTimeout: 120 * time.Second}
}

func NewIPC(service *Service) *IPC { return &IPC{service: service} }

func (i *IPC) Open(ctx context.Context, r OpenRequest) OpenResult {
	if i == nil || i.service == nil {
		return OpenResult{Error: dto.Failure("terminalUnavailable", "desktop", "reload", 0)}
	}
	return i.service.Open(ctx, r)
}

func (s *Service) Open(ctx context.Context, r OpenRequest) OpenResult {
	if s == nil || s.Cloud == nil || s.Connection == nil || s.Target == nil {
		return OpenResult{Error: dto.Failure("terminalUnavailable", "desktop", "reload", 0)}
	}
	if r.SessionID == "" || len(r.SessionID) > 256 || strings.ContainsAny(r.SessionID, "\r\n\x00") || len(r.Title) > 512 || strings.ContainsRune(r.Title, 0) {
		return OpenResult{Error: dto.Failure("invalidInput", "desktop", "never", 0)}
	}
	state := s.Connection.State()
	if !state.Connected || state.ConnectionEpoch != r.ConnectionEpoch {
		return OpenResult{Error: dto.Failure("notConnected", "transport", "reload", 0)}
	}
	target := s.Target()
	if target.RuntimeID == "" || target.RuntimeID != state.RuntimeID || target.Region == "" {
		return OpenResult{Error: dto.Failure("notConnected", "transport", "reload", 0)}
	}
	key := terminalKey(r.ConnectionEpoch, r.SessionID)
	s.mu.Lock()
	if existing := s.windows[key]; existing != nil {
		s.mu.Unlock()
		existing.focus()
		return OpenResult{OK: true, Value: &OpenReceipt{SessionID: r.SessionID, RuntimeID: target.RuntimeID}}
	}
	win := &terminalWindow{
		service: s,
		key:     key,
		request: r,
		target:  target,
		runtime: firstNonEmpty(state.RuntimeName, state.RuntimeID),
		state:   "connecting",
		message: "Resolving Runtime instance...",
	}
	win.ctx, win.cancel = context.WithCancel(context.Background())
	s.windows[key] = win
	s.mu.Unlock()
	if err := win.createWindow(); err != nil {
		s.remove(key)
		return OpenResult{Error: dto.Failure("terminalWindowUnavailable", "desktop", "reload", 0)}
	}
	go win.start(false)
	return OpenResult{OK: true, Value: &OpenReceipt{SessionID: r.SessionID, RuntimeID: target.RuntimeID}}
}

func (s *Service) CloseAll() {
	s.mu.Lock()
	windows := make([]*terminalWindow, 0, len(s.windows))
	for _, win := range s.windows {
		windows = append(windows, win)
	}
	s.windows = map[string]*terminalWindow{}
	s.mu.Unlock()
	for _, win := range windows {
		win.close()
	}
}

func (s *Service) remove(key string) {
	s.mu.Lock()
	delete(s.windows, key)
	s.mu.Unlock()
}

func terminalKey(epoch uint64, sessionID string) string {
	return strings.Join([]string{time.Unix(0, int64(epoch)).Format("150405.000000000"), sessionID}, ":")
}

type terminalWindow struct {
	service *Service
	key     string
	request OpenRequest
	target  dto.RuntimeRequest
	runtime string
	ctx     context.Context
	cancel  context.CancelFunc

	mu       sync.Mutex
	window   *mygo.Window
	term     *myterminal.Terminal
	conn     io.Closer
	state    string
	message  string
	instance string
	starting bool
}

func (w *terminalWindow) createWindow() error {
	created := runOnMainValue(func() error {
		title := "Runtime Terminal"
		if w.request.Title != "" {
			title += " - " + w.request.Title
		}
		win := mygo.NewWindow(mygo.WindowOptions{Title: title, Width: 920, Height: 620, MinWidth: 720, MinHeight: 420, StateKey: "runtime-terminal", Content: ui.View(w.view)})
		w.mu.Lock()
		w.window = win
		w.mu.Unlock()
		win.OnClosed(func() {
			w.close()
			w.service.remove(w.key)
		})
		win.Focus()
		return nil
	})
	return created
}

func (w *terminalWindow) focus() {
	runOnMain(func() {
		w.mu.Lock()
		win := w.window
		w.mu.Unlock()
		if win != nil && !win.IsDestroyed() {
			win.Show()
			win.Focus()
		}
	})
}

func (w *terminalWindow) close() {
	w.mu.Lock()
	cancel := w.cancel
	term := w.term
	conn := w.conn
	w.term = nil
	w.conn = nil
	w.state = "closed"
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if term != nil {
		_ = term.Close()
	}
	if conn != nil {
		_ = conn.Close()
	}
}

func (w *terminalWindow) snapshot() (state, message, runtime, instance string, term *myterminal.Terminal, starting bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.state, w.message, w.runtime, w.instance, w.term, w.starting
}

func (w *terminalWindow) set(state, message string) {
	w.mu.Lock()
	w.state, w.message = state, message
	w.mu.Unlock()
	w.invalidate()
}

func (w *terminalWindow) invalidate() {
	w.mu.Lock()
	win := w.window
	w.mu.Unlock()
	if win == nil {
		return
	}
	runOnMain(func() {
		if win != nil && !win.IsDestroyed() {
			win.Invalidate()
		}
	})
}

func (w *terminalWindow) start(allowWarmup bool) {
	w.mu.Lock()
	if w.starting {
		w.mu.Unlock()
		return
	}
	w.starting = true
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.starting = false
		w.mu.Unlock()
		w.invalidate()
	}()
	w.set("connecting", "Resolving Runtime instance...")
	instance, err := w.resolveInstance(allowWarmup)
	if err != nil {
		w.set(err.state, err.message)
		return
	}
	w.mu.Lock()
	w.instance = instance.InstanceName
	w.mu.Unlock()
	w.set("connecting", "Opening terminal...")
	endpoint, e := w.service.Cloud.WebshellEndpoint(w.ctx, w.target.Region, functionIDFromInstance(instance.InstanceName), instance.InstanceName)
	if e != nil {
		w.set("failed", e.Message)
		return
	}
	conn, dialErr := dialWebshell(endpoint, w.service.dialer)
	if dialErr != nil {
		w.set("failed", "Could not attach to the Runtime terminal.")
		return
	}
	term, termErr := myterminal.New(myterminal.Options{
		Conn: conn,
		OnExit: func(int) {
			w.set("closed", "Terminal session closed.")
		},
	})
	if termErr != nil {
		_ = conn.Close()
		w.set("failed", "Native terminal support is unavailable on this machine.")
		return
	}
	w.mu.Lock()
	if w.term != nil {
		_ = w.term.Close()
	}
	if w.conn != nil {
		_ = w.conn.Close()
	}
	w.term = term
	w.conn = conn
	w.state = "attached"
	w.message = "Attached"
	w.mu.Unlock()
	w.invalidate()
}

type terminalStartError struct {
	state   string
	message string
}

func (w *terminalWindow) resolveInstance(allowWarmup bool) (cloud.RuntimeInstance, *terminalStartError) {
	instances, e := w.service.Cloud.RuntimeInstances(w.ctx, w.target)
	if e != nil {
		return cloud.RuntimeInstance{}, &terminalStartError{"failed", e.Message}
	}
	if instance, ok := attachableInstance(instances); ok {
		return instance, nil
	}
	if !allowWarmup {
		return cloud.RuntimeInstance{}, &terminalStartError{"warmupRequired", "No running Runtime instance is available."}
	}
	w.set("warming", "Starting Runtime instance...")
	if e := w.warmup(); e != nil {
		return cloud.RuntimeInstance{}, &terminalStartError{"failed", e.Message}
	}
	deadline := time.Now().Add(w.service.waitTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-w.ctx.Done():
			return cloud.RuntimeInstance{}, &terminalStartError{"closed", "Terminal session closed."}
		case <-time.After(w.service.pollInterval):
		}
		instances, e = w.service.Cloud.RuntimeInstances(w.ctx, w.target)
		if e != nil {
			return cloud.RuntimeInstance{}, &terminalStartError{"failed", e.Message}
		}
		if instance, ok := attachableInstance(instances); ok {
			return instance, nil
		}
	}
	return cloud.RuntimeInstance{}, &terminalStartError{"failed", "Timed out waiting for a Runtime instance to become ready."}
}

func (w *terminalWindow) warmup() *dto.SafeError {
	resp, e := w.service.Connection.Request(w.ctx, w.request.ConnectionEpoch, http.MethodPost, "/invoke", []byte(`{"prompt":"ping"}`), nil)
	if resp != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
	}
	return e
}

func attachableInstance(instances []cloud.RuntimeInstance) (cloud.RuntimeInstance, bool) {
	for _, instance := range instances {
		status := strings.ToLower(instance.InstanceStatus)
		if status == "running" || status == "ready" {
			return instance, true
		}
	}
	return cloud.RuntimeInstance{}, false
}

func functionIDFromInstance(instanceName string) string {
	name := strings.TrimPrefix(instanceName, "vefaas-")
	if before, _, ok := strings.Cut(name, "-"); ok {
		return before
	}
	return name
}

func (w *terminalWindow) view(c *ui.Context) {
	state, message, runtimeName, instance, term, starting := w.snapshot()
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Height(44).Shrink(0).Padding(0, 14).Gap(10).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).DragWindow().Children(func() {
			ui.Text(c, "Runtime Terminal").Bold().SingleLine()
			ui.Text(c, runtimeName).TextColor(t.TextMuted).SingleLine().Grow(1).MinWidth(0)
			label := state
			if instance != "" {
				label += " · " + instance
			}
			ui.Text(c, label).FontSize(12).TextColor(t.TextMuted).SingleLine()
		})
		if term != nil && state == "attached" {
			myterminal.View(c, term).Fill().AutoFocus()
			return
		}
		ui.Column(c).Fill().Padding(24).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Spacer(c)
			switch state {
			case "warming", "connecting":
				ui.Spinner(c).Size(24, 24)
			}
			ui.Text(c, message).Font("SF Mono, Menlo, monospace").FontSize(14).TextColor(t.Text).Label("Runtime terminal status")
			if state == "warmupRequired" {
				ui.Text(c, "Starting a Runtime instance is an external action. Confirm before OpenMPA sends the warm-up request.").TextColor(t.TextMuted).FontSize(12).MaxWidth(560)
				if ui.PrimaryButton(c, "Start And Connect").Disabled(starting).Clicked() {
					go w.start(true)
				}
			} else if state == "failed" || state == "closed" {
				if ui.Button(c, "Retry").Disabled(starting).Clicked() {
					go w.start(false)
				}
			}
			ui.Spacer(c)
		})
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func runOnMain(fn func()) {
	mygo.RunOnMain(fn)
}

func runOnMainValue[T any](fn func() T) T {
	var out T
	runOnMain(func() { out = fn() })
	return out
}
