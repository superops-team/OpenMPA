package host

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/design"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/localization"
	"strings"
	"sync"
	"time"
)

type RailAction struct {
	Action string `json:"action"`
	ID     string `json:"id,omitempty"`
}
type ThemeValue struct {
	Theme string `json:"theme"`
}

type ThemeResult struct {
	OK    bool           `json:"ok"`
	Value *ThemeValue    `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}

type NavigationItem struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	AgentID string `json:"agentId,omitempty"`
}
type ShellState struct {
	ConnectionEpoch   uint64           `json:"connectionEpoch"`
	View              string           `json:"view"`
	Collapsed         bool             `json:"collapsed"`
	TraceOpen         bool             `json:"traceOpen,omitempty"`
	Location          string           `json:"location"`
	PageTitle         string           `json:"pageTitle"`
	SelectedSession   string           `json:"selectedSession"`
	SelectedSpace     string           `json:"selectedSpace"`
	SelectedAssistant string           `json:"selectedAssistant,omitempty"`
	SettingsSection   string           `json:"settingsSection,omitempty"`
	Theme             string           `json:"theme,omitempty"`
	Sessions          []NavigationItem `json:"sessions"`
	Spaces            []NavigationItem `json:"spaces"`
	Assistants        []NavigationItem `json:"assistants,omitempty"`
	Revision          uint64           `json:"revision"`
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
	query       string
}

func (r *nativeRail) snapshot() ShellState { r.mu.Lock(); defer r.mu.Unlock(); return r.state }
func (r *nativeRail) action(action, id string) {
	r.mu.Lock()
	r.lastAction = action
	r.actionCount++
	r.mu.Unlock()
	if r.request != nil {
		r.request(action, id)
	}
}

// InspectNavigation reports safe shell evidence to tests.
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
	"home":            nativeBrand,
	"toggleSidebar":   ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M9 3v18"/></svg>`)),
	"newConversation": ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M12 5v14M5 12h14"/></svg>`)),
	"pages":           ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M3 7V5a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/></svg>`)),
	"tasks":           ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg>`)),
	"settings":        ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M4 7h6m4 0h6M4 17h10m4 0h2"/><circle cx="12" cy="7" r="2"/><circle cx="16" cy="17" r="2"/></svg>`)),
	"trace":           ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M4 6h16M4 12h10M4 18h7"/><circle cx="18" cy="12" r="2"/><circle cx="15" cy="18" r="2"/></svg>`)),
	"selectSpace":     ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M3 7V5a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/></svg>`)),
	"selectSession":   ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M21 11a9 9 0 0 1-9 9c-2 0-3-.3-4-1L3 21l1-5c-1-1-1-3-1-5a9 9 0 0 1 18 0Z"/></svg>`)),
	"sessionActions":  ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="1.7"/><circle cx="12" cy="12" r="1.7"/><circle cx="19" cy="12" r="1.7"/></svg>`)),
	"knowledge":       ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M12 5v16M3 4l9 2 9-2v16l-9 2-9-2Z"/></svg>`)),
	"channels":        ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="m12 3 10 5-10 5L2 8Z M2 12l10 5 10-5M2 16l10 5 10-5"/></svg>`)),
	"resources":       ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><path d="M14 17h7M17.5 14v7"/></svg>`)),
	"runtimes":        ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><rect x="3" y="3" width="18" height="7" rx="2"/><rect x="3" y="14" width="18" height="7" rx="2"/><path d="M7 6.5h.01M7 17.5h.01"/></svg>`)),
	"collaboration":   ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><circle cx="8" cy="7" r="3"/><path d="M2 21v-3a6 6 0 0 1 12 0v3M16 4a3 3 0 0 1 0 6M17 15a5 5 0 0 1 5 5"/></svg>`)),
}

var settingsSections = []struct{ id, label string }{
	{"Account", "Account"},
	{"Runtime", "Runtime"},
	{"Assistant", "Assistant"},
	{"Authentication", "Authentication"},
	{"Deployment", "Deployment"},
	{"Theme", "Theme"},
	{"Language", "Language"},
}

func buttonFeedback(c *ui.Context, b ui.Element) {
	if b.Pressed() {
		b.Background(design.Colors(c.Theme().Dark).SurfacePressed)
	}
	if !c.Preferences().ReduceMotion {
		b.Transition(ui.ElementTransition{Colors: true, Duration: design.PressDurationMs * time.Millisecond})
	}
}

func iconButton(c *ui.Context, action, label string, selected bool, fn func()) ui.Element {
	shown := localization.UI(label)
	b := ui.Button(c.Key(action), "").Label(shown).Tooltip(shown).Size(design.ControlLg, design.ControlLg).Padding(0).Radius(design.RadiusMd).BorderWidth(0).Background(ui.Transparent).Selected(selected)
	if selected || b.Hovered() {
		b.Background(c.Theme().Border)
	}
	buttonFeedback(c, b)
	b.Children(func() { ui.Icon(c, railIcons[action]).FontSize(design.IconLg).TextColor(c.Theme().TextMuted) })
	if b.Clicked() {
		fn()
	}
	return b
}
func (r *nativeRail) controls(c *ui.Context) {
	state := r.snapshot()
	ui.Column(c).Fill().Padding(design.Space3, design.Space2).Gap(design.Space2).AlignItems(ui.Center).Background(design.Colors(c.Theme().Dark).Sidebar).BorderWidth(0, 1, 0, 0).BorderColor(c.Theme().Border).Children(func() {
		homeLabel := localization.Text("openMpaHome", nil)
		home := ui.Button(c.Key("home"), "").Label(homeLabel).Tooltip(homeLabel).Size(design.ControlInput, design.ControlInput).Padding(0).Radius(design.RadiusMd).BorderWidth(0).Background(ui.Transparent)
		if home.Hovered() {
			home.Background(c.Theme().Border)
		}
		buttonFeedback(c, home)
		home.Children(func() {
			ui.Icon(c, nativeBrand).FontSize(design.IconBrand).TextColor(design.Colors(c.Theme().Dark).Brand)
		})
		if home.Clicked() {
			r.action("home", "")
		}
		for _, item := range []struct {
			action, label string
			selected      bool
		}{
			{"toggleSidebar", "Toggle sidebar", !state.Collapsed},
			{"runtimes", localization.Text("manageRuntimes", nil), state.View == "runtimes"},
			{"newConversation", "New conversation", false},
			{"tasks", "Open scheduled activity", state.View == "tasks"},
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
	ui.Row(c).Key("titlebar").Label(localization.Text("titleBar", nil)).Height(design.LayoutTitlebarHeight).Shrink(0).Gap(design.Space2).AlignItems(ui.Center).
		Padding(0, insets.Right+design.Space4, 0, insets.Left+design.Space3).DragWindow().Background(design.Colors(c.Theme().Dark).Header).
		BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
		ui.Text(c, "OpenMPA").FontSize(c.Theme().FontSize).Bold().TextColor(c.Theme().Text).SingleLine().Shrink(0)
		iconButton(c, "toggleSidebar", "Toggle navigation", false, func() { r.action("toggleSidebar", "") })
		// Divider defaults to Stretch, so a short titlebar rule must opt into centering.
		ui.Divider(c).Key("titlebar-divider").Label(localization.Text("titleBarSeparator", nil)).Width(1).Height(design.Space4).AlignSelf(ui.Center).Background(c.Theme().Border).Margin(0, design.Space2)
		ui.Row(c).Key("location").Label(localization.Text("currentLocation", nil)).Grow(1).MinWidth(0).Gap(design.Space3).Children(func() {
			location := state.Location
			if location == "" {
				location = localization.Text("yourWorkspace", nil)
			}
			title := state.PageTitle
			if title == "" {
				title = localization.Text("chat", nil)
			}
			ui.Text(c, location).Tooltip(location).SingleLine().FontSize(c.Theme().FontSize).TextColor(c.Theme().TextMuted).MinWidth(0).Shrink(1)
			ui.Text(c, "/").FontSize(c.Theme().FontSize).TextColor(c.Theme().TextMuted).Shrink(0)
			ui.Text(c, title).Tooltip(title).SingleLine().FontSize(c.Theme().FontSize).TextColor(c.Theme().Text).MinWidth(0).Shrink(1)
		})
	})
}
func (r *nativeRail) sidebar(c *ui.Context) {
	state := r.snapshot()
	row := func(action, id, title, subtitle string, selected bool) {
		shown := localization.UI(title)
		b := ui.Button(c.Key(action+":"+id), "").Label(shown).Tooltip(shown).FillWidth().MinHeight(design.ControlInput).Padding(design.Space2).Radius(design.RadiusMd).BorderWidth(0).Background(ui.Transparent).Selected(selected)
		if selected {
			b.Background(design.Colors(c.Theme().Dark).CardSelected)
		} else if b.Hovered() {
			b.Background(design.Colors(c.Theme().Dark).SurfaceHover)
		}
		buttonFeedback(c, b)
		b.Children(func() {
			if action == "selectSession" && subtitle == "" {
				ui.Image(c, avatarFor(id)).Size(28, 28).Shrink(0)
			} else if icon := railIcons[action]; icon != nil {
				ui.Icon(c, icon).FontSize(design.IconMd).TextColor(c.Theme().TextMuted)
			}
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, shown).FontSize(c.Theme().FontSize).TextColor(c.Theme().Text).SingleLine()
				if subtitle != "" {
					ui.Text(c, localization.UI(subtitle)).FontSize(design.TypographyUiSm).TextColor(c.Theme().TextMuted).SingleLine()
				}
			})
			if action == "selectSession" {
				menuColor := c.Theme().TextMuted
				if !b.Hovered() && !b.Focused() {
					menuColor = ui.Transparent
				}
				menu := ui.Button(c.Key("session-actions:"+id), "").Label(localization.Text("assistantActions", nil)).Tooltip(localization.Text("assistantActions", nil)).Size(design.ControlMd, design.ControlMd).Padding(0).Radius(design.RadiusMd).BorderWidth(0).Background(ui.Transparent)
				if menu.Hovered() || menu.Pressed() || menu.Focused() {
					menu.Background(c.Theme().Border)
					menuColor = c.Theme().Text
				}
				buttonFeedback(c, menu)
				menu.Children(func() {
					ui.Icon(c, railIcons["sessionActions"]).FontSize(design.IconMd).TextColor(menuColor)
				})
				menu.Menu(func(m *ui.Menu) {
					if m.Item(localization.Text("editAssistant", nil)).Chosen() {
						r.action("editSession", id)
					}
					if m.Item(localization.Text("delete", nil)).Chosen() {
						r.action("deleteSession", id)
					}
				})
			}
		})
		if b.Clicked() {
			r.action(action, id)
		}
	}
	heading := func(label, action, actionLabel string) {
		ui.Row(c).Margin(10, 4, 4).Children(func() {
			ui.Text(c, localization.UI(label)).FontSize(design.TypographyUiSm).Bold().TextColor(c.Theme().TextMuted).Grow(1)
			if action != "" {
				iconButton(c, "newConversation", actionLabel, false, func() { r.action(action, "") }).Size(design.ControlSm, design.ControlSm)
			}
		})
	}
	ui.Column(c).Key("sidebar").Label(localization.Text("workspaceNavigation", nil)).Fill().Padding(design.Space3, design.Space2).Gap(design.Space1).Background(design.Colors(c.Theme().Dark).Sidebar).BorderWidth(0, 1, 0, 0).BorderColor(c.Theme().Border).Children(func() {
		if state.View == "settings" {
			heading("SETTINGS", "", "")
			for _, section := range settingsSections {
				selected := state.SettingsSection == section.id || (state.SettingsSection == "" && section.id == "Account")
				row("selectSettingsSection", section.id, section.label, "", selected)
			}
			return
		}
		assistantLabels := []string{localization.UI("Select Runtime")}
		selectedLabel := assistantLabels[0]
		for _, item := range state.Assistants {
			label := item.Title
			if label == "" {
				label = item.ID
			}
			if label != item.ID {
				label += " (" + item.ID + ")"
			}
			assistantLabels = append(assistantLabels, label)
			if item.ID == state.SelectedAssistant {
				selectedLabel = label
			}
		}
		ui.Combobox(c.Key("assistant-select"), &selectedLabel, assistantLabels).Label(localization.UI("Runtime")).FillWidth().OnChange(func() {
			for i, label := range assistantLabels[1:] {
				if label == selectedLabel {
					r.action("selectAssistant", state.Assistants[i].ID)
					return
				}
			}
		})
		if len(state.Assistants) == 0 {
			ui.Text(c, localization.UI("Choose your Runtime in Settings.")).Padding(6).FontSize(design.TypographyUiSm).TextColor(c.Theme().TextMuted)
		}
		ui.TextInput(c.Key("navigation-search"), &r.query).Label(localization.Text("findConversations", nil)).Placeholder(localization.Text("searchConversations", nil)).FillWidth()
		matches := func(item NavigationItem) bool {
			return strings.Contains(strings.ToLower(item.ID+" "+item.Title), strings.ToLower(strings.TrimSpace(r.query)))
		}

		type navRow struct {
			heading, action, id, title, subtitle, actionLabel string
			selected                                          bool
		}
		rows := []navRow{{heading: "ASSISTANTS", action: "newConversation", actionLabel: "New assistant"}}
		if len(state.Sessions) == 0 {
			rows = append(rows, navRow{title: "No assistants yet."})
		}
		for _, item := range state.Sessions {
			if !matches(item) {
				continue
			}
			rows = append(rows, navRow{action: "selectSession", id: item.ID, title: item.Title, selected: state.View == "chat" && state.SelectedSession == item.ID})
		}
		ui.List(c.Key("navigation-list"), nil, len(rows), func(i int) {
			item := rows[i]
			if item.heading != "" {
				heading(item.heading, item.action, item.actionLabel)
			} else if item.action == "" {
				ui.Text(c, localization.UI(item.title)).Padding(8).FontSize(design.TypographyUiSm).TextColor(c.Theme().TextMuted)
			} else {
				row(item.action, item.id, item.title, item.subtitle, item.selected)
			}
		}).Grow(1).MinHeight(0).Gap(3)

		ui.Divider(c).Margin(8, 0)
		for _, item := range []struct{ action, label string }{{"tasks", "Scheduled & activity"}, {"resources", "Skills, MCP & instructions"}, {"collaboration", "A2A collaboration"}, {"knowledge", "Knowledge"}, {"channels", "Channels & templates"}} {
			row(item.action, "", item.label, "", state.View == item.action)
		}
	})
}
func (r *nativeRail) view(c *ui.Context) {
	dark := c.Theme().Dark
	switch r.snapshot().Theme {
	case "dark":
		dark = true
	case "light":
		dark = false
	}
	c.SetTheme(design.Theme(dark, c.Preferences()))
	ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
		r.titlebar(c)
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Width(design.LayoutRailWidth).Shrink(0).Children(func() { r.controls(c) })
			viewport := ui.Column(c).Key("sidebar-viewport").Shrink(0).MinWidth(0).Clip()
			target := float32(design.LayoutSidebarWidth)
			if r.snapshot().Collapsed {
				target = 0
			}
			width := viewport.AnimateWith("width", target, time.Duration(design.SidebarDurationMs)*time.Millisecond, ui.EaseOut)
			viewport.Width(width).Children(func() {
				if width > 0 {
					ui.Column(c).Key("sidebar-content").Width(design.LayoutSidebarWidth).Shrink(0).FillHeight().Translate(width-design.LayoutSidebarWidth, 0).Children(func() { r.sidebar(c) })
				}
			})
			ui.Column(c).Key("workspace-content").Label("Workspace content").Grow(1).MinWidth(0).MinHeight(0).Children(func() {
				if r.page != nil {
					ui.WebView(c, r.page).Key("workspace-webview").Grow(1).MinWidth(0)
				}
			})
		})
	})
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
	case "chat", "tasks", "channels", "settings", "knowledge", "resources", "collaboration", "runtimes":
	default:
		return false
	}
	if len(state.Location) > 512 || len(state.PageTitle) > 512 || len(state.SelectedAssistant) > 256 || len(state.SettingsSection) > 64 || len(state.Theme) > 16 || len(state.Sessions) > 1000 || len(state.Spaces) > 1000 || len(state.Assistants) > 1000 {
		return false
	}
	for _, items := range [][]NavigationItem{state.Sessions, state.Spaces, state.Assistants} {
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
	state.Assistants = append([]NavigationItem(nil), state.Assistants...)
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

// Theme returns the persisted display theme for the WebView to apply on
// mount. The change event remains for live updates.
func (h *Host) Theme() ThemeResult {
	if h == nil {
		return ThemeResult{Error: dto.Failure("notReady", "desktop", "reload", 0)}
	}
	h.mu.Lock()
	r := h.rail
	h.mu.Unlock()
	if r == nil {
		return ThemeResult{Error: dto.Failure("notReady", "desktop", "reload", 0)}
	}
	r.mu.Lock()
	theme := r.state.Theme
	r.mu.Unlock()
	if theme == "" {
		theme = "system"
	}
	return ThemeResult{OK: true, Value: &ThemeValue{Theme: theme}}
}
