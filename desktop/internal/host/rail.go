package host

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"strings"
	"sync"
	"time"
)

type RailAction struct {
	Action string `json:"action"`
	ID     string `json:"id,omitempty"`
}
type NavigationItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
type ShellState struct {
	ConnectionEpoch uint64           `json:"connectionEpoch"`
	View            string           `json:"view"`
	Collapsed       bool             `json:"collapsed"`
	Pane            bool             `json:"pane"`
	Location        string           `json:"location"`
	PageTitle       string           `json:"pageTitle"`
	SelectedSession string           `json:"selectedSession"`
	SelectedSpace   string           `json:"selectedSpace"`
	Sessions        []NavigationItem `json:"sessions"`
	Spaces          []NavigationItem `json:"spaces"`
	Revision        uint64           `json:"revision"`
}

var NavigationRequested = mygo.NewEvent[RailAction]("NavigationRequested")

type nativeRail struct {
	mu          sync.Mutex
	state       ShellState
	request     func(string, string)
	window      *mygo.Window
	page        *mygo.WebView
	lastAction  string
	actionCount int
	content     func(*ui.Context, ShellState) bool
	pane        func(*ui.Context, ShellState)
	query       string
	overlay     func(*ui.Context, ShellState)
	handle      func(string, string) bool
}

func (r *nativeRail) snapshot() ShellState { r.mu.Lock(); defer r.mu.Unlock(); return r.state }
func (r *nativeRail) action(action, id string) {
	r.mu.Lock()
	r.lastAction = action
	r.actionCount++
	r.mu.Unlock()
	if r.handle != nil && r.handle(action, id) {
		return
	}
	if r.request != nil {
		r.request(action, id)
	}
}

// InspectNavigation reports safe shell evidence to a native harness.
func InspectNavigation(h *Host) (ShellState, string, int) {
	h.mu.Lock()
	r := h.rail
	h.mu.Unlock()
	if r == nil {
		return ShellState{}, "", 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state, r.lastAction, r.actionCount
}

var railIcons = map[string]*ui.SVG{
	"toggleSidebar":   ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M9 3v18"/></svg>`)),
	"newConversation": ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M12 5v14M5 12h14"/></svg>`)),
	"pages":           ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M3 7V5a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/></svg>`)),
	"tasks":           ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg>`)),
	"settings":        ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M4 7h6m4 0h6M4 17h10m4 0h2"/><circle cx="12" cy="7" r="2"/><circle cx="16" cy="17" r="2"/></svg>`)),
	"artifacts":       ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><rect x="3" y="3" width="18" height="14" rx="2"/><path d="M12 17v4M8 21h8"/></svg>`)),
	"selectSpace":     ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M3 7V5a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/></svg>`)),
	"selectSession":   ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M21 11a9 9 0 0 1-9 9c-2 0-3-.3-4-1L3 21l1-5c-1-1-1-3-1-5a9 9 0 0 1 18 0Z"/></svg>`)),
	"knowledge":       ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M12 5v16M3 4l9 2 9-2v16l-9 2-9-2Z"/></svg>`)),
	"channels":        ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="m12 3 10 5-10 5L2 8Z M2 12l10 5 10-5M2 16l10 5 10-5"/></svg>`)),
}
var shellTheme = ui.LightTheme()

func buttonFeedback(b ui.Element) {
	if b.Pressed() {
		b.Background(ui.RGB(215, 215, 215))
	}
	b.Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
}

func iconButton(c *ui.Context, action, label string, selected bool, fn func()) ui.Element {
	b := ui.Button(c.Key(action), "").Label(label).Tooltip(label).Size(32, 32).Padding(0).Radius(5).BorderWidth(0).Background(ui.Transparent).Selected(selected)
	if selected || b.Hovered() {
		b.Background(ui.RGB(228, 228, 228))
	}
	buttonFeedback(b)
	b.Children(func() { ui.Icon(c, railIcons[action]).FontSize(18).TextColor(ui.RGB(85, 85, 85)) })
	if b.Clicked() {
		fn()
	}
	return b
}
func (r *nativeRail) controls(c *ui.Context) {
	state := r.snapshot()
	ui.Column(c).Fill().Padding(12, 6).Gap(10).AlignItems(ui.Center).Background(ui.RGB(245, 245, 245)).BorderWidth(0, 1, 0, 0).BorderColor(ui.RGB(228, 228, 228)).Children(func() {
		home := ui.Button(c.Key("home"), "").Label("OpenMPA home").Tooltip("OpenMPA home").Size(36, 36).Padding(0).Radius(6).BorderWidth(0).Background(ui.Transparent)
		if home.Hovered() {
			home.Background(ui.RGB(228, 228, 228))
		}
		buttonFeedback(home)
		home.Children(func() { ui.Icon(c, nativeBrand).FontSize(28).TextColor(ui.RGB(48, 56, 50)) })
		if home.Clicked() {
			r.action("home", "")
		}
		for _, item := range []struct {
			action, label string
			selected      bool
		}{
			{"toggleSidebar", "Toggle sidebar", !state.Collapsed}, {"newConversation", "New conversation", false},
			{"pages", "Open Spaces", state.View == "pages"}, {"tasks", "Open scheduled activity", state.View == "tasks"},
		} {
			item := item
			iconButton(c, item.action, item.label, item.selected, func() { r.action(item.action, "") })
		}
		ui.Spacer(c)
		iconButton(c, "settings", "Open settings", state.View == "settings", func() { r.action("settings", "") })
	})
}
func (r *nativeRail) titlebar(c *ui.Context) {
	state := r.snapshot()
	insets := c.TitleBar()
	ui.Row(c).Key("titlebar").Label("OpenMPA title bar").Height(44).Shrink(0).Gap(10).AlignItems(ui.Center).
		Padding(0, insets.Right+14, 0, insets.Left+10).DragWindow().Background(ui.RGB(245, 245, 245)).
		BorderWidth(0, 0, 1, 0).BorderColor(ui.RGB(228, 228, 228)).Children(func() {
		ui.Text(c, "OpenMPA").FontSize(13).Bold().TextColor(ui.RGB(48, 48, 48)).SingleLine().Shrink(0)
		iconButton(c, "toggleSidebar", "Toggle navigation", false, func() { r.action("toggleSidebar", "") })
		// Divider defaults to Stretch, so a short titlebar rule must opt into centering.
		ui.Divider(c).Key("titlebar-divider").Label("Title bar separator").Width(1).Height(16).AlignSelf(ui.Center).Background(ui.RGB(222, 222, 222)).Margin(0, 6)
		ui.Row(c).Key("location").Label("Current location").Grow(1).MinWidth(0).Gap(12).Children(func() {
			location := state.Location
			if location == "" {
				location = "Your workspace"
			}
			title := state.PageTitle
			if title == "" {
				title = "chat"
			}
			ui.Text(c, location).Tooltip(location).SingleLine().FontSize(13).TextColor(ui.RGB(105, 105, 105)).MinWidth(0).Shrink(1)
			ui.Text(c, "/").FontSize(13).TextColor(ui.RGB(150, 150, 150)).Shrink(0)
			ui.Text(c, title).Tooltip(title).SingleLine().FontSize(13).TextColor(ui.RGB(75, 75, 75)).MinWidth(0).Shrink(1)
		})
		iconButton(c, "artifacts", "Show capabilities and artifacts", state.Pane, func() { r.action("artifacts", "") })
	})
}
func (r *nativeRail) sidebar(c *ui.Context) {
	state := r.snapshot()
	row := func(action, id, title, subtitle string, selected bool) {
		b := ui.Button(c.Key(action+":"+id), "").Label(title).Tooltip(title).FillWidth().MinHeight(36).Padding(7, 9).Radius(6).BorderWidth(0).Background(ui.Transparent).Selected(selected)
		if selected {
			b.Background(ui.RGB(235, 237, 246))
		} else if b.Hovered() {
			b.Background(ui.RGB(239, 239, 239))
		}
		buttonFeedback(b)
		b.Children(func() {
			if action == "selectSession" && subtitle == "" {
				ui.Image(c, avatarFor(id)).Size(28, 28).Shrink(0)
			} else if icon := railIcons[action]; icon != nil {
				ui.Icon(c, icon).FontSize(17).TextColor(ui.RGB(112, 112, 112))
			}
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, title).FontSize(13).TextColor(ui.RGB(80, 80, 80)).SingleLine()
				if subtitle != "" {
					ui.Text(c, subtitle).FontSize(11).TextColor(ui.RGB(120, 120, 120)).SingleLine()
				}
			})
		})
		if b.Clicked() {
			r.action(action, id)
		}
	}
	heading := func(label, action, actionLabel string) {
		ui.Row(c).Margin(10, 4, 4).Children(func() {
			ui.Text(c, label).FontSize(11).Bold().TextColor(ui.RGB(115, 115, 115)).Grow(1)
			if action != "" {
				iconButton(c, "newConversation", actionLabel, false, func() { r.action(action, "") }).Size(24, 24)
			}
		})
	}
	ui.Column(c).Key("sidebar").Label("Workspace navigation").Fill().Padding(12, 10).Gap(4).Background(ui.RGB(249, 249, 249)).BorderWidth(0, 1, 0, 0).BorderColor(ui.RGB(228, 228, 228)).Children(func() {
		row("newConversation", "", "New conversation", "", false)
		ui.TextInput(c.Key("navigation-search"), &r.query).Label("Find conversations").Placeholder("Search assistants and Spaces").FillWidth()
		matches := func(item NavigationItem) bool {
			return strings.Contains(strings.ToLower(item.ID+" "+item.Title), strings.ToLower(strings.TrimSpace(r.query)))
		}

		type navRow struct {
			heading, action, id, title, subtitle, actionLabel string
			selected                                          bool
		}
		rows := []navRow{{heading: "ASSISTANTS", action: "newConversation", actionLabel: "Create assistant"}}
		if len(state.Sessions) == 0 {
			rows = append(rows, navRow{title: "Choose your Runtime in Settings."})
		}
		for _, item := range state.Sessions {
			if !matches(item) {
				continue
			}
			rows = append(rows, navRow{action: "selectSession", id: item.ID, title: item.Title, selected: state.View == "chat" && state.SelectedSession == item.ID})
		}
		rows = append(rows, navRow{heading: "SPACES", action: "createSpace", actionLabel: "Create Space"})
		for _, item := range state.Spaces {
			if !matches(item) {
				continue
			}
			rows = append(rows, navRow{action: "selectSpace", id: item.ID, title: item.Title, selected: state.View == "pages" && state.SelectedSpace == item.ID})
		}
		rows = append(rows, navRow{heading: "RECENT CHATS", action: "refreshSessions", actionLabel: "Refresh conversations"})
		for _, item := range state.Sessions {
			if !matches(item) {
				continue
			}
			rows = append(rows, navRow{action: "selectSession", id: item.ID, title: item.Title, subtitle: "Session", selected: state.View == "chat" && state.SelectedSession == item.ID})
		}
		ui.List(c.Key("navigation-list"), nil, len(rows), func(i int) {
			item := rows[i]
			if item.heading != "" {
				heading(item.heading, item.action, item.actionLabel)
			} else if item.action == "" {
				ui.Text(c, item.title).Padding(8).FontSize(12).TextColor(ui.RGB(120, 120, 120))
			} else {
				row(item.action, item.id, item.title, item.subtitle, item.selected)
			}
		}).Grow(1).MinHeight(0).Gap(3)

		ui.Divider(c).Margin(8, 0)
		for _, item := range []struct{ action, label string }{{"tasks", "Scheduled & activity"}, {"knowledge", "Knowledge"}, {"channels", "Channels & templates"}, {"settings", "Settings & setup"}} {
			row(item.action, "", item.label, "", state.View == item.action)
		}
	})
}
func (r *nativeRail) view(c *ui.Context) {
	c.SetTheme(shellTheme)
	ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
		r.titlebar(c)
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Width(48).Shrink(0).Children(func() { r.controls(c) })
			if !r.snapshot().Collapsed {
				ui.Column(c).Width(240).Shrink(0).Children(func() { r.sidebar(c) })
			}
			if r.page != nil {
				ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
					if r.content == nil || !r.content(c, r.snapshot()) {
						ui.WebView(c, r.page).Grow(1).MinWidth(0)
					}
				})
			}
			if r.pane != nil && r.snapshot().Pane {
				r.pane(c, r.snapshot())
			}
		})
		if r.overlay != nil {
			r.overlay(c, r.snapshot())
		}
	})
}

// SetNativeContent composes native domain pages beside the retained chat/editor.
// Call during window assembly on the UI thread.
func SetNativeContent(h *Host, render func(*ui.Context, ShellState) bool) {
	h.mu.Lock()
	r := h.rail
	h.mu.Unlock()
	if r != nil {
		r.content = render
		r.window.Invalidate()
	}
}

func NotifyConnection(h *Host) {
	h.mu.Lock()
	r := h.rail
	h.mu.Unlock()
	if r != nil {
		r.action("connectionChanged", "")
	}
}
func NewDesktopWindow(h *Host, options mygo.WindowOptions) (*mygo.Window, *mygo.Page, error) {
	r := &nativeRail{state: ShellState{View: "chat"}}
	url := options.URL
	pageOptions := options.Page
	options.URL = ""
	options.Content = ui.View(r.view)
	w := mygo.NewWindow(options)
	r.window = w
	var err error
	r.page, err = w.NewWebView(mygo.WebViewOptions{URL: url, Page: pageOptions})
	if err != nil {
		w.Destroy()
		return nil, nil, err
	}
	r.request = func(action, id string) { NavigationRequested.Emit(w, RailAction{Action: action, ID: id}) }
	h.mu.Lock()
	h.rail = r
	h.mu.Unlock()
	w.Invalidate()
	return w, r.page.Page(), nil
}
func validShellState(state ShellState) bool {
	switch state.View {
	case "chat", "pages", "tasks", "channels", "settings", "knowledge":
	default:
		return false
	}
	if len(state.Location) > 512 || len(state.PageTitle) > 512 || len(state.Sessions) > 1000 || len(state.Spaces) > 1000 {
		return false
	}
	for _, items := range [][]NavigationItem{state.Sessions, state.Spaces} {
		seen := map[string]bool{}
		for _, item := range items {
			if item.ID == "" || len(item.ID) > 256 || strings.ContainsRune(item.ID, 0) || len(item.Title) > 512 || seen[item.ID] {
				return false
			}
			seen[item.ID] = true
		}
	}
	return true
}
func (h *Host) SetShellState(state ShellState) dto.EmptyResult {
	if !validShellState(state) {
		return dto.EmptyResult{Error: dto.Failure("invalidInput", "transport", "never", 0)}
	}
	state.Sessions = append([]NavigationItem(nil), state.Sessions...)
	state.Spaces = append([]NavigationItem(nil), state.Spaces...)
	h.mu.Lock()
	r := h.rail
	h.mu.Unlock()
	if r != nil {
		r.mu.Lock()
		if state.Revision >= r.state.Revision {
			r.state = state
		}
		r.mu.Unlock()
		r.window.Invalidate()
	}
	return dto.EmptyResult{OK: true, Value: &dto.Empty{}}
}

func SetNativePane(h *Host, render func(*ui.Context, ShellState)) {
	h.mu.Lock()
	r := h.rail
	h.mu.Unlock()
	if r != nil {
		r.pane = render
		r.window.Invalidate()
	}
}
func ToggleArtifacts(h *Host) {
	h.mu.Lock()
	r := h.rail
	h.mu.Unlock()
	if r != nil {
		r.action("artifacts", "")
	}
}

func SetNativeOverlay(h *Host, render func(*ui.Context, ShellState)) {
	h.mu.Lock()
	r := h.rail
	h.mu.Unlock()
	if r != nil {
		r.overlay = render
		r.window.Invalidate()
	}
}
func SetNativeActionHandler(h *Host, fn func(string, string) bool) {
	h.mu.Lock()
	r := h.rail
	h.mu.Unlock()
	if r != nil {
		r.handle = fn
	}
}
func (h *Host) RequestConversation() dto.EmptyResult {
	h.mu.Lock()
	r := h.rail
	h.mu.Unlock()
	if r == nil || r.handle == nil {
		return dto.EmptyResult{Error: dto.Failure("nativeDialogUnavailable", "desktop", "never", 0)}
	}
	r.window.Update(func() { r.action("newConversation", "") })
	return dto.EmptyResult{OK: true, Value: &dto.Empty{}}
}
func NotifyCreated(h *Host, action, id string) {
	if action != "sessionCreated" && action != "spaceCreated" && action != "sessionChanged" && action != "sessionDeleted" {
		return
	}
	h.mu.Lock()
	r := h.rail
	h.mu.Unlock()
	if r != nil {
		NavigationRequested.Emit(r.window, RailAction{Action: action, ID: id})
	}
}

func (h *Host) RequestSessionAction(action string) dto.EmptyResult {
	if action != "configureSession" && action != "renameSession" && action != "deleteSession" {
		return dto.EmptyResult{Error: dto.Failure("invalidInput", "desktop", "never", 0)}
	}
	h.mu.Lock()
	r := h.rail
	h.mu.Unlock()
	if r == nil || r.handle == nil {
		return dto.EmptyResult{Error: dto.Failure("nativeDialogUnavailable", "desktop", "never", 0)}
	}
	r.window.Update(func() { r.action(action, "") })
	return dto.EmptyResult{OK: true, Value: &dto.Empty{}}
}
