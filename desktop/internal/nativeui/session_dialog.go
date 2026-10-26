package nativeui

import (
	"context"
	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/host"
	"strings"
)

type sessionDialogState struct {
	open                                      bool
	id, title, modelMode, model, mcpMode, mcp string
	config                                    *dto.ExecutionConfig
}

func (p *Controller) sessionAction(action string) bool {
	id := p.shell.SelectedSession
	title := ""
	for _, v := range p.currentSessions() {
		if v.ID == id {
			title = v.Title
		}
	}
	if id == "" || title == "" {
		p.err = dto.Failure("staleConnection", "desktop", "reload", 0)
		return true
	}
	r := dto.SessionRequest{ConnectionEpoch: p.epoch(), SessionID: id}
	switch action {
	case "renameSession":
		p.prompt = promptState{open: true, kind: action, name: title, sessionID: id}
	case "deleteSession":
		p.ask("Delete conversation", title+" · "+id, func() {
			p.async(func(ctx context.Context) (func(), *dto.SafeError) {
				out := p.services.Session.Delete(ctx, r)
				return func() {
					if out.OK {
						host.NotifyCreated(p.host, "sessionDeleted", id)
					}
				}, out.Error
			})
		})
	case "configureSession":
		p.sessionDialog = sessionDialogState{open: true, id: id, title: title}
		p.async(func(ctx context.Context) (func(), *dto.SafeError) {
			out := p.services.Session.ExecutionConfig(ctx, r)
			return func() {
				if out.OK && out.Value != nil {
					v := out.Value
					p.sessionDialog.config = v
					p.sessionDialog.model = v.Model
					p.sessionDialog.modelMode = v.ModelMode
					p.sessionDialog.mcpMode = v.MCPMode
					p.sessionDialog.mcp = strings.Join(v.MCPServers, ", ")
				} else if out.Error != nil && out.Error.HTTPStatus == 404 {
					p.err = &dto.SafeError{Code: "configurationUnavailable", Message: "This conversation uses Runtime defaults and has no editable execution configuration.", Plane: "mpa", RetryDisposition: "never", HTTPStatus: 404}
				}
			}, out.Error
		})
	default:
		return false
	}
	return true
}
func (p *Controller) renderSessionDialog(c *ui.Context) {
	d := &p.sessionDialog
	ui.Modal(c.Key("native-session-configuration"), &d.open, func() {
		ui.Column(c).Width(480).Gap(14).Children(func() {
			ui.Text(c, "Conversation configuration").FontSize(18).Bold()
			p.note(c, d.title+" · "+d.id)
			p.note(c, "Changes apply to future turns; accepted turns keep their original configuration.")
			if p.err != nil {
				ui.Text(c, p.err.Message+statusSuffix(p.err)).FontSize(12).TextColor(ui.RGB(175, 60, 50))
			}
			if d.config != nil {
				ui.Select(c.Key("Model configuration mode"), &d.modelMode, []string{"inherit", "replace", "clear"}).Label("Model configuration mode").Disabled(p.busy)
				if d.modelMode == "replace" {
					p.field(c, "Session model ID", &d.model, false)
				}
				ui.Select(c.Key("MCP configuration mode"), &d.mcpMode, []string{"inherit", "replace", "clear"}).Label("MCP configuration mode").Disabled(p.busy)
				if d.mcpMode == "replace" {
					p.field(c, "Session MCP IDs (comma separated)", &d.mcp, false)
				}
				p.button(c, "Save conversation configuration", func() {
					r := dto.ExecutionConfigUpdateRequest{ConnectionEpoch: p.epoch(), SessionID: d.id, ETag: d.config.ETag, ModelMode: d.modelMode, Model: d.model, MCPMode: d.mcpMode, MCPServers: *refs(d.mcp)}
					p.ask("Update conversation configuration", d.title+" · "+d.id, func() {
						p.async(func(ctx context.Context) (func(), *dto.SafeError) {
							out := p.services.Session.UpdateExecutionConfig(ctx, r)
							return func() {
								if out.OK {
									p.sessionDialog.open = false
									host.NotifyCreated(p.host, "sessionChanged", r.SessionID)
								}
							}, out.Error
						})
					})
				})
			}
			ui.Button(c, "Close configuration").Disabled(p.busy).OnClick(func() { d.open = false; p.err = nil })
		})
	})
}
