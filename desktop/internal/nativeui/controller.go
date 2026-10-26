// Package nativeui composes domain pages from the same typed Go services as IPC.
package nativeui

import (
	"context"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/artifact"
	"github.com/superops-team/OpenMPA/desktop/internal/channels"
	"github.com/superops-team/OpenMPA/desktop/internal/configstore"
	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/host"
	"github.com/superops-team/OpenMPA/desktop/internal/ipc"
	"github.com/superops-team/OpenMPA/desktop/internal/knowledge"
	"github.com/superops-team/OpenMPA/desktop/internal/localization"
	"github.com/superops-team/OpenMPA/desktop/internal/runtimeconfig"
	"github.com/superops-team/OpenMPA/desktop/internal/session"
	"github.com/superops-team/OpenMPA/desktop/internal/workspace"
	"strings"
	"time"
)

type Services struct {
	Workspace      *workspace.Service
	Artifact       *artifact.Service
	Session        *session.Service
	TaskIntentPath func() (string, error)
	Platform       *ipc.Platform
	Connection     *ipc.Connection
	Deployment     *deployment.Service
	RuntimeConfig  *runtimeconfig.Service
	Channels       *channels.Channels
	Schedule       ScheduleService
	Knowledge      *knowledge.Service
	Localization   *localization.Service
	Config         *configstore.Store
}
type Controller struct {
	services                       Services
	window                         *mygo.Window
	host                           *host.Host
	shell                          host.ShellState
	generation                     uint64
	connectionEpoch, platformEpoch uint64
	observed, closed               bool
	entered                        bool
	cancel                         context.CancelFunc
	bindingTimer                   *time.Timer
	settingsTimer                  *time.Timer
	busy                           bool
	message                        string
	err                            *dto.SafeError
	confirm                        bool
	confirmTitle, confirmMessage   string
	confirmed                      func()
	settings                       settingsState
	channels                       channelState
	schedules                      scheduleState
	knowledge                      knowledgeState
	artifacts                      artifactState
	prompt                         promptState
	sessionDialog                  sessionDialogState
	createdSessionID               string
	intents                        taskIntents
}

func Attach(h *host.Host, w *mygo.Window, s Services) *Controller {
	preferences := configstore.Preferences{Language: configstore.DefaultLanguage, Region: configstore.DefaultRegion, Project: configstore.DefaultProject}
	if s.Config != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if current, err := s.Config.Snapshot(ctx); err == nil {
			preferences = current
		}
		cancel()
	}
	if !supportedRegion(s.Platform, preferences.Region) {
		preferences.Region = configstore.DefaultRegion
		preferences.Runtime = ""
		preferences.PrincipalScope = ""
	}
	p := &Controller{services: s, window: w, host: h, settings: settingsState{region: preferences.Region, project: preferences.Project, selected: preferences.Runtime, preferredRuntime: preferences.Runtime, preferredPrincipalScope: preferences.PrincipalScope, authMode: "runtimeKey", tab: "Account", language: preferences.Language}}
	if s.Localization != nil && s.Localization.Status().Value != nil {
		p.settings.language = s.Localization.Status().Value.Mode
	}
	if s.Platform != nil {
		if status := s.Platform.VaultStatus(); status.OK && status.Value != nil {
			p.settings.vault = *status.Value
		}
	}
	p.intents.path = s.TaskIntentPath
	p.channels = newChannelState()
	p.schedules = scheduleState{}
	p.knowledge = knowledgeState{}
	p.artifacts = artifactState{}
	host.SetNativeContent(h, p.Render)
	host.SetNativePane(h, p.RenderPane)
	host.SetNativeOverlay(h, p.RenderOverlay)
	host.SetNativeActionHandler(h, p.HandleAction)
	w.OnClosed(func() { p.closed = true; p.invalidate() })
	return p
}
func (p *Controller) cancelWork() {
	p.generation++
	if p.bindingTimer != nil {
		p.bindingTimer.Stop()
		p.bindingTimer = nil
	}
	if p.settingsTimer != nil {
		p.settingsTimer.Stop()
		p.settingsTimer = nil
	}
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.busy = false
	p.err = nil
	p.message = ""
	p.confirm = false
	p.confirmed = nil
}
func (p *Controller) invalidate() {
	p.cancelWork()
	p.entered = false
	p.prompt = promptState{}
	p.sessionDialog = sessionDialogState{}
	s := p.settings
	p.channels = newChannelState()
	p.schedules = scheduleState{}
	p.knowledge = knowledgeState{}
	p.artifacts = artifactState{}
	language := s.language
	if p.services.Localization != nil && p.services.Localization.Status().Value != nil {
		language = p.services.Localization.Status().Value.Mode
	}
	p.settings = settingsState{region: s.region, project: s.project, preferredRuntime: s.preferredRuntime, preferredPrincipalScope: s.preferredPrincipalScope, tab: s.tab, authMode: "runtimeKey", language: language}
	if p.services.Connection != nil && p.services.Connection.Client != nil {
		conn := p.services.Connection.Status().Value
		if conn != nil && conn.Connected {
			p.settings.runtimes = s.runtimes
			p.settings.directoryRegion = s.directoryRegion
			p.settings.directoryProject = s.directoryProject
			p.settings.directoryLoaded = s.directoryLoaded
			p.settings.directoryGeneration = s.directoryGeneration
			for _, runtime := range s.runtimes {
				if runtime.RuntimeID == conn.RuntimeID && runtime.Region == s.region && runtime.ProjectName == s.project {
					p.settings.selected = conn.RuntimeID
					p.settings.runtimeChoice = runtimeChoiceLabel(runtime)
					break
				}
			}
		}
	}
	// Vault metadata is local and non-sensitive; reload it so signing out does
	// not hide the unlock path that the vault exists for.
	p.refreshVault()
}
func (p *Controller) observe(state host.ShellState) {
	epoch := p.epoch()
	platformEpoch := p.services.Platform.Status().Value.Epoch
	if p.observed && (epoch != p.connectionEpoch || platformEpoch != p.platformEpoch) {
		p.invalidate()
	} else if p.observed && p.shell.View != state.View {
		p.cancelWork()
		p.entered = false
		p.settings.ak = ""
		p.settings.sk = ""
		p.settings.securityToken = ""
		p.settings.jwt = ""
		p.settings.channelKey = ""
		p.settings.vaultPassphrase = ""
		p.settings.vaultConfirm = ""
		p.settings.showVault = false
		p.channels.secret = ""
		p.channels.qr = nil
		if p.channels.binding != nil {
			b := *p.channels.binding
			b.QRCodeImage = ""
			b.LoginURL = ""
			p.channels.binding = &b
		}
	} else if p.observed && (p.shell.Pane != state.Pane || (state.Pane && p.shell.SelectedSession != state.SelectedSession)) {
		p.cancelWork()
		p.artifacts = artifactState{}
	}

	p.observed = true
	p.shell = state
	p.connectionEpoch = epoch
	p.platformEpoch = platformEpoch
}
func (p *Controller) async(work func(context.Context) (func(), *dto.SafeError)) { p.start(false, work) }
func (p *Controller) identityAsync(work func(context.Context) (func(), *dto.SafeError)) {
	p.start(true, work)
}
func (p *Controller) start(identity bool, work func(context.Context) (func(), *dto.SafeError)) {
	if p.busy || p.closed {
		return
	}
	p.busy = true
	p.err = nil
	p.message = ""
	generation, epoch, platformEpoch := p.generation, p.epoch(), p.services.Platform.Status().Value.Epoch
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	p.cancel = cancel
	go func() {
		defer cancel()
		apply, err := work(ctx)
		p.window.Update(func() { p.complete(generation, epoch, platformEpoch, identity, apply, err) })
	}()
}

// complete runs only on the owning UI thread; stale work cannot change a new page.
func (p *Controller) complete(generation, epoch, platformEpoch uint64, identity bool, apply func(), err *dto.SafeError) {
	if p.closed {
		return
	}
	if p.generation != generation {
		if identity {
			host.NotifyConnection(p.host)
		}
		return
	}
	if !identity && (p.epoch() != epoch || p.services.Platform.Status().Value.Epoch != platformEpoch) {
		p.invalidate()
		return
	}
	identityChanged := identity && (p.epoch() != epoch || p.services.Platform.Status().Value.Epoch != platformEpoch)
	if identity && (err == nil || identityChanged) {
		p.invalidate()
		p.connectionEpoch = p.epoch()
		p.platformEpoch = p.services.Platform.Status().Value.Epoch
	}
	p.cancel = nil
	p.busy = false
	p.err = err
	if apply != nil {
		apply()
	}
	if identity {
		host.NotifyConnection(p.host)
	}
}

func (p *Controller) ask(title, message string, action func()) {
	if p.busy {
		return
	}
	p.confirmTitle = localization.UI(title)
	p.confirmMessage = localization.UI(message)
	p.confirmed = action
	p.confirm = true
}
func (p *Controller) button(c *ui.Context, label string, fn func()) {
	p.buttonWithKey(c, label, localization.UI(label), fn)
}
func (p *Controller) buttonWithKey(c *ui.Context, key, label string, fn func()) {
	ui.Button(c.Key(key), label).Disabled(p.busy).OnClick(func() {
		if p.busy || p.closed {
			return
		}
		fn()
		if p.window != nil {
			p.window.Invalidate()
		}
	})
}
func (p *Controller) field(c *ui.Context, label string, value *string, secret bool) {
	ui.Column(c.Key(label + " field")).Gap(5).Children(func() {
		ui.Text(c, localization.UI(label)).Label(localization.UI(label) + " caption").FontSize(12).TextColor(c.Theme().TextMuted)
		input := ui.TextInput(c.Key(label), value).Label(localization.UI(label)).FillWidth().Disabled(p.busy)
		if secret {
			input.Password()
		}
	})
}
func (p *Controller) section(c *ui.Context, title string, body func()) {
	ui.Column(c.Key(title)).Gap(12).Padding(18).Background(ui.RGB(255, 255, 255)).Border(1, ui.RGB(230, 230, 230)).Radius(8).Children(func() { ui.Text(c, localization.UI(title)).FontSize(15).Bold(); body() })
}
func (p *Controller) note(c *ui.Context, value string) {
	ui.Text(c, localization.UI(value)).FontSize(12).TextColor(c.Theme().TextMuted).Selectable()
}
func (p *Controller) epoch() uint64 { return p.services.Connection.Client.State().ConnectionEpoch }
func (p *Controller) Render(c *ui.Context, state host.ShellState) bool {
	p.observe(state)
	if state.View != "settings" && state.View != "channels" && state.View != "tasks" && state.View != "knowledge" {
		return false
	}
	if !p.entered && p.window != nil {
		p.entered = true
		generation := p.generation
		p.window.Update(func() {
			if p.closed || p.generation != generation {
				return
			}
			if p.busy {
				p.entered = false
				return
			}
			conn := p.services.Connection.Status().Value
			platform := p.services.Platform.Status().Value
			switch state.View {
			case "settings":
				validDirectory := p.settings.directoryLoaded && p.settings.directoryRegion == p.settings.region && p.settings.directoryProject == p.settings.project
				if platform != nil && platform.SignedIn && !validDirectory {
					p.loadDirectory(conn == nil || !conn.Connected)
				}
			case "channels":
				if conn != nil && conn.Connected {
					p.loadChannels()
				}
			case "tasks":
				if conn != nil && conn.Connected {
					p.loadTasks(0)
				}
			case "knowledge":
				if conn != nil && conn.Connected {
					p.knowledge.sessionID = state.SelectedSession
					if p.knowledgeMember() {
						p.loadKnowledge()
					}
				}
			}
		})
	}
	ui.Column(c).Fill().MinHeight(0).Background(ui.RGB(250, 250, 250)).Label("Native " + state.View).Children(func() {
		ui.Row(c).Padding(24, 28, 12).Children(func() {
			title := "Settings"
			if state.View == "channels" {
				title = "Channels"
			}
			if state.View == "tasks" {
				title = "Scheduled tasks"
			}
			if state.View == "knowledge" {
				title = "Knowledge"
			}
			ui.Text(c, localization.UI(title)).FontSize(24).Bold().Grow(1)
			if p.busy {
				ui.Text(c, localization.Text("working", nil)).FontSize(12)
			}
		})
		if p.err != nil {
			ui.Text(c, fmt.Sprintf("%s%s", localization.Error(p.err.Code, p.err.Message), statusSuffix(p.err))).Padding(8, 28).TextColor(ui.RGB(175, 60, 50)).Label("Native page error")
		}
		if p.message != "" {
			ui.Text(c, localization.UI(p.message)).Padding(8, 28).FontSize(12).Selectable()
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Padding(8, 28, 28).Gap(18).Children(func() {
			if state.View == "knowledge" {
				p.renderKnowledge(c)
			} else if state.View == "tasks" {
				p.renderSchedules(c)
			} else if state.View == "channels" {
				p.renderChannels(c)
			} else {
				p.renderSettings(c)
			}
		})
	})

	return true
}
func statusSuffix(err *dto.SafeError) string {
	if err.HTTPStatus > 0 {
		return fmt.Sprintf(" (HTTP %d)", err.HTTPStatus)
	}
	return ""
}
func required(value string) bool { return strings.TrimSpace(value) != "" }

func (p *Controller) rows(c *ui.Context, key string, count int, height float32, draw func(int)) {
	if count == 0 {
		return
	}
	ui.List(c.Key(key), nil, count, draw).Height(min(height, float32(count*56))).MinWidth(0).Gap(6)
}

func (p *Controller) currentSessions() []host.NavigationItem {
	if p.shell.ConnectionEpoch != p.epoch() {
		return nil
	}
	return p.shell.Sessions
}
