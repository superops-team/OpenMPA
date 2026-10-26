package nativeui

import (
	"context"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/host"
	"github.com/superops-team/OpenMPA/desktop/internal/localization"
	"github.com/superops-team/OpenMPA/desktop/internal/workspace"
	"strings"
)

type assistantIntent struct {
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
}
type promptState struct {
	sessionID  string
	open       bool
	kind, name string
	assistant  *assistantIntent
}

func (p *Controller) HandleAction(action, id string) bool {
	if p.host != nil {
		state, _, _ := host.InspectNavigation(p.host)
		p.observe(state)
	}
	if action == "renameSession" || action == "deleteSession" || action == "configureSession" {
		if p.busy {
			return true
		}
		return p.sessionAction(action)
	}
	if action != "newConversation" && action != "createSpace" {
		return false
	}
	if action == "newConversation" {
		state := p.services.Connection.Status().Value
		if state == nil || !state.Connected {
			p.prompt = promptState{}
			p.err = nil
			p.settings.tab = "Runtime"
			if p.window != nil {
				p.window.Invalidate()
			}
			// Let the WebView navigation owner switch the shell to Settings.
			return false
		}
	}
	if p.busy {
		p.message = "Finish the current operation before creating another item."
		return true
	}
	p.prompt = promptState{open: true, kind: action}
	p.err = nil
	if action == "newConversation" {
		scope := p.services.Connection.Status().Value.Scope + "/native-assistant"
		p.async(func(context.Context) (func(), *dto.SafeError) {
			intent, e := p.intents.load(scope)
			if e != nil {
				return nil, dto.Failure("pendingPersistenceFailed", "desktop", "never", 0)
			}
			return func() {
				if intent.Assistant != nil {
					p.prompt.assistant = intent.Assistant
					p.prompt.name = intent.Assistant.Name
				}
			}, nil
		})
	}
	return true
}
func (p *Controller) RenderOverlay(c *ui.Context, state host.ShellState) {
	ui.Modal(c.Key("native-create-dialog"), &p.prompt.open, func() {
		ui.Column(c).Width(420).Gap(14).Children(func() {
			title := "New Space"
			label := "Space name"
			if p.prompt.kind == "renameSession" {
				title = "Rename conversation"
				label = "Conversation title"
			}
			if p.prompt.kind == "newConversation" {
				title = "New assistant"
				label = "Assistant name"
			}
			ui.Text(c, localization.UI(title)).FontSize(18).Bold()
			p.field(c, label, &p.prompt.name, false)
			if p.prompt.assistant != nil {
				p.note(c, "Recover the original assistant: "+p.prompt.assistant.SessionID)
			}
			if p.err != nil {
				ui.Text(c, localization.Error(p.err.Code, p.err.Message)+statusSuffix(p.err)).FontSize(12).TextColor(ui.RGB(175, 60, 50))
			}
			ui.Row(c).Gap(10).Children(func() {
				ui.Button(c, localization.Text("cancel", nil)).Label(localization.Text("cancel", nil)).Disabled(p.busy).OnClick(func() { p.prompt.open = false })
				button := "Create"
				if p.prompt.kind == "renameSession" {
					button = "Save title"
				}
				p.button(c, button, p.createPromptItem)
			})
		})
	})
	p.renderSessionDialog(c)
	if p.err != nil && !p.prompt.open && !p.sessionDialog.open && !state.Pane && state.View != "settings" && state.View != "channels" && state.View != "tasks" && state.View != "knowledge" {
		ui.Column(c.Key("native-operation-feedback")).Absolute().Bottom(20).Right(20).Width(400).Padding(14).Gap(10).Background(ui.RGB(255, 255, 255)).Border(1, ui.RGB(225, 225, 225)).Radius(8).Label("Native operation status").Children(func() {
			ui.Text(c, localization.Error(p.err.Code, p.err.Message)+statusSuffix(p.err)).FontSize(12)
			ui.Button(c, localization.Text("dismiss", nil)).OnClick(func() { p.err = nil })
		})
	}
	switch ui.AlertDialog(c, &p.confirm, p.confirmTitle, p.confirmMessage, "Cancel", "Continue") {
	case 1:
		fn := p.confirmed
		p.confirmed = nil
		if fn != nil {
			fn()
		}
	case ui.AlertDismissed:
		p.confirmed = nil
	}
	if !p.confirm {
		p.confirmed = nil
	}
}
func (p *Controller) createPromptItem() {
	name := strings.TrimSpace(p.prompt.name)
	if name == "" || len(name) > 200 {
		p.err = dto.Failure("invalidInput", "desktop", "never", 0)
		return
	}
	if p.prompt.kind == "renameSession" {
		r := dto.RenameRequest{ConnectionEpoch: p.epoch(), SessionID: p.prompt.sessionID, Title: name}
		p.async(func(ctx context.Context) (func(), *dto.SafeError) {
			out := p.services.Session.Rename(ctx, r)
			return func() {
				if out.OK {
					p.prompt.open = false
					host.NotifyCreated(p.host, "sessionChanged", r.SessionID)
				}
			}, out.Error
		})
		return
	}
	if p.prompt.kind == "createSpace" {
		r := workspace.CreateSpaceRequest{Name: name}
		p.async(func(ctx context.Context) (func(), *dto.SafeError) {
			out := p.services.Workspace.CreateSpace(ctx, r)
			return func() {
				if out.OK && out.Value != nil {
					p.prompt.open = false
					host.NotifyCreated(p.host, "spaceCreated", out.Value.ID)
				}
			}, workspaceError(out.Error)
		})
		return
	}
	state := p.services.Connection.Status().Value
	if err := assistantCreationPrecondition(*state); err != nil {
		p.err = err
		return
	}
	intent := p.prompt.assistant
	if intent == nil {
		intent = &assistantIntent{SessionID: uuid.NewString(), Name: name}
		p.prompt.assistant = intent
	} else if name != intent.Name {
		p.err = dto.Failure("conflict", "desktop", "reload", 0)
		return
	}
	record := *intent
	scope := state.Scope + "/native-assistant"
	epoch := state.ConnectionEpoch
	p.async(func(ctx context.Context) (func(), *dto.SafeError) {
		if e := p.intents.saveAssistant(scope, &record); e != nil {
			return nil, dto.Failure("pendingPersistenceFailed", "desktop", "never", 0)
		}
		read := p.services.Session.Read(ctx, dto.SessionRequest{ConnectionEpoch: epoch, SessionID: record.SessionID})
		if !read.OK {
			if read.Error == nil || read.Error.HTTPStatus != 404 {
				return nil, read.Error
			}
			created := p.services.Session.Create(ctx, dto.CreateRequest{ConnectionEpoch: epoch, SessionID: record.SessionID})
			if !created.OK {
				return nil, created.Error
			}
			if created.Value == nil || created.Value.ID != record.SessionID {
				return nil, dto.Failure("invalidResponse", "desktop", "verifyOnly", 0)
			}
		}
		named := p.services.Session.Rename(ctx, dto.RenameRequest{ConnectionEpoch: epoch, SessionID: record.SessionID, Title: record.Name})
		if !named.OK {
			return nil, named.Error
		}
		if e := p.intents.saveAssistant(scope, nil); e != nil {
			return nil, dto.Failure("pendingPersistenceFailed", "desktop", "never", 0)
		}
		return func() {
			p.prompt.open = false
			p.prompt.assistant = nil
			p.createdSessionID = record.SessionID
			host.NotifyCreated(p.host, "sessionCreated", record.SessionID)
		}, nil
	})
}

// assistantCreationPrecondition mirrors AgentKit key-auth: a bound Runtime
// key is a complete Session identity. An explicit business identity is an
// optional subject override and must not be mistaken for connectivity.
func assistantCreationPrecondition(state dto.ConnectionState) *dto.SafeError {
	if !state.Connected {
		return dto.Failure("notConnected", "desktop", "never", 0)
	}
	if !state.HasCredential {
		return dto.Failure("authRequired", "desktop", "never", 0)
	}
	return nil
}

func workspaceError(e *workspace.ServiceError) *dto.SafeError {
	if e == nil {
		return nil
	}
	return &dto.SafeError{Code: e.Code, Message: e.Message, Plane: "desktopWorkspace", HTTPStatus: e.Status, RetryDisposition: "never", OriginConfidence: "verified"}
}
