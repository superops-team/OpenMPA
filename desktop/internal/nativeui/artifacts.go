package nativeui

import (
	"context"
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/artifact"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/host"
)

type artifactState struct {
	path         string
	bytes        int64
	capabilities []dto.Capability
	config       *dto.ExecutionConfig
}

func (p *Controller) RenderPane(c *ui.Context, state host.ShellState) {
	p.observe(state)
	ui.Column(c).Width(320).Shrink(0).FillHeight().MinHeight(0).Padding(18).Gap(12).Border(1, ui.RGB(230, 230, 230)).Background(ui.RGB(250, 250, 250)).Label("Native artifacts").Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "Capabilities & artifacts").FontSize(15).Bold().Grow(1)
			ui.Button(c, "Close").Label("Close results").OnClick(func() { host.ToggleArtifacts(p.host) })
		})
		ui.Scroll(c).Grow(1).MinHeight(0).Gap(12).Children(func() {
			conn := p.services.Connection.Status().Value
			p.note(c, "Runtime "+conn.RuntimeID)
			if !conn.Connected {
				p.note(c, "Choose and connect a Runtime first.")
				return
			}
			p.button(c, "Read Runtime capabilities", func() {
				r := dto.ScopeRequest{ConnectionEpoch: p.epoch()}
				p.async(func(ctx context.Context) (func(), *dto.SafeError) {
					out := p.services.Connection.Test(ctx, r)
					return func() {
						if out.OK && out.Value != nil {
							p.artifacts.capabilities = out.Value.Capabilities
						}
					}, out.Error
				})
			})
			for _, cap := range p.artifacts.capabilities {
				p.note(c, cap.Name+": "+cap.State)
			}
			member := false
			for _, v := range p.currentSessions() {
				if v.ID == state.SelectedSession {
					member = true
				}
			}
			if member && p.services.Session != nil {
				p.note(c, "Session "+state.SelectedSession)
				p.button(c, "Read Session configuration", func() {
					r := dto.SessionRequest{ConnectionEpoch: p.epoch(), SessionID: state.SelectedSession}
					p.async(func(ctx context.Context) (func(), *dto.SafeError) {
						out := p.services.Session.ExecutionConfig(ctx, r)
						return func() {
							if out.OK {
								p.artifacts.config = out.Value
							}
						}, out.Error
					})
				})
				if p.artifacts.config != nil {
					v := p.artifacts.config
					p.note(c, "Model: "+v.Model)
					p.note(c, fmt.Sprintf("Configuration %s · Profile bound: %t", v.ExecutionConfigVersion, v.ProfileBound))
					for _, server := range v.MCPServers {
						p.note(c, "MCP: "+server)
					}
				}
				p.field(c, "Artifact path", &p.artifacts.path, false)
				p.button(c, "Download artifact", func() {
					r := artifact.DownloadRequest{ConnectionEpoch: p.epoch(), SessionID: state.SelectedSession, RemotePath: p.artifacts.path}
					generation := p.generation
					p.async(func(ctx context.Context) (func(), *dto.SafeError) {
						out := artifact.DownloadFromWindow(p.services.Artifact, ctx, p.window, r, func(progress artifact.DownloadProgress) error {
							if ctx.Err() != nil {
								return ctx.Err()
							}
							p.window.Update(func() {
								if !p.closed && p.generation == generation {
									p.artifacts.bytes = progress.BytesWritten
								}
							})
							return nil
						})
						return func() {
							if out.OK && out.Value != nil {
								p.message = fmt.Sprintf("Saved %d bytes · %s", out.Value.BytesWritten, out.Value.Integrity)
							}
						}, out.Error
					})
				})
				if p.busy {
					ui.Button(c, "Cancel transfer").OnClick(func() {
						if p.cancel != nil {
							p.cancel()
						}
					})
				}
				if p.artifacts.bytes > 0 {
					p.note(c, fmt.Sprintf("%d bytes received", p.artifacts.bytes))
				}
			} else {
				p.note(c, "Select a conversation to download a Runtime artifact.")
			}
			if p.err != nil {
				ui.Text(c, p.err.Message+statusSuffix(p.err)).FontSize(12).TextColor(ui.RGB(175, 60, 50))
			}
			if p.message != "" {
				p.note(c, p.message)
			}
		})
	})
}
