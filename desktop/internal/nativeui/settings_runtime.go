package nativeui

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/ipc"
	"github.com/superops-team/OpenMPA/desktop/internal/localization"
)

func (p *Controller) loadDirectory(restore bool) {
	p.stopSettingsTimer()
	r := dto.RuntimeListRequest{Region: p.settings.region, ProjectName: p.settings.project}
	previousDirectory := p.settings.directoryRegion + "\x00" + p.settings.directoryProject
	nextDirectory := r.Region + "\x00" + r.ProjectName
	preferred := p.settings.selected
	if preferred == "" {
		preferred = p.settings.preferredRuntime
	}
	platform := p.services.Platform.Status().Value
	if p.settings.preferredPrincipalScope != "" && (platform == nil || platform.Principal == nil || platform.Principal.Scope != p.settings.preferredPrincipalScope) {
		preferred = ""
	}
	if conn := p.services.Connection.Status().Value; conn != nil && conn.Connected {
		preferred = conn.RuntimeID
	}
	target := ipc.Target(p.services.Platform)
	resetConnection := target.RuntimeID != "" && (target.Region != r.Region || target.ProjectName != r.ProjectName)
	if !resetConnection {
		resetConnection = previousDirectory != "\x00" && previousDirectory != nextDirectory
	}
	if resetConnection {
		p.resetRuntimeProjection()
	}
	if previousDirectory != nextDirectory {
		p.settings.runtimes = nil
		p.settings.selected = ""
		p.settings.runtimeChoice = ""
	}
	p.settings.directoryLoaded = false
	p.settings.directoryGeneration++
	p.async(func(ctx context.Context) (func(), *dto.SafeError) {
		if p.services.Config != nil {
			if err := p.services.Config.SetDirectory(ctx, r.Region, r.ProjectName); err != nil {
				return nil, dto.Failure("localConfigUnavailable", "desktop", "never", 0)
			}
		}
		out := p.services.Platform.ListRuntimes(ctx, r)
		return func() {
			if p.settings.region != r.Region || p.settings.project != r.ProjectName {
				return
			}
			if out.OK && out.Value != nil {
				p.settings.runtimes = out.Value.Runtimes
				p.settings.directoryRegion = r.Region
				p.settings.directoryProject = r.ProjectName
				p.settings.directoryLoaded = out.Value.Complete
				p.settings.runtimeChoice = ""
				p.settings.selected = ""
				for _, runtime := range out.Value.Runtimes {
					if runtime.RuntimeID == preferred && runtime.Region == r.Region && runtime.ProjectName == r.ProjectName {
						p.settings.selected = preferred
						p.settings.runtimeChoice = runtimeChoiceLabel(runtime)
						break
					}
				}
				if restore {
					if runtime, ok := p.settings.restorableRuntime(*p.services.Platform.Status().Value); ok {
						p.bindRuntime(runtime, true)
					}
				}
			}
		}, out.Error
	})
}

func (p *Controller) bindRuntime(runtime dto.RuntimeSummary, restored bool) {
	request := dto.RuntimeRequest{Region: runtime.Region, ProjectName: runtime.ProjectName, RuntimeID: runtime.RuntimeID}
	p.identityAsync(func(ctx context.Context) (func(), *dto.SafeError) {
		out := p.services.Platform.BindRuntime(ctx, request)
		platform := p.services.Platform.Status().Value
		if out.OK && p.services.Config != nil {
			if platform == nil || !platform.SignedIn || platform.Principal == nil || platform.Principal.Scope == "" {
				return nil, dto.Failure("staleConnection", "platform", "reload", 0)
			}
			if err := p.services.Config.SetRuntime(ctx, platform.Principal.Scope, request.Region, request.ProjectName, request.RuntimeID); err != nil {
				return nil, dto.Failure("localConfigUnavailable", "desktop", "never", 0)
			}
		}
		return func() { p.finishRuntimeBinding(runtime, restored, platform, out.OK) }, out.Error
	})
}

func (p *Controller) finishRuntimeBinding(runtime dto.RuntimeSummary, restored bool, platform *dto.PlatformState, ok bool) {
	if !ok {
		// MyGo Combobox only reports Changed when its selected value changes.
		// Clear the failed candidate and rebuild the field so selecting the same
		// sole Runtime is an explicit retry instead of a no-op.
		p.settings.selected = ""
		p.settings.runtimeChoice = ""
		p.settings.directoryGeneration++
		return
	}
	p.settings.selected = runtime.RuntimeID
	p.settings.runtimeChoice = runtimeChoiceLabel(runtime)
	p.settings.preferredRuntime = runtime.RuntimeID
	if platform != nil && platform.Principal != nil {
		p.settings.preferredPrincipalScope = platform.Principal.Scope
	}
	if restored {
		p.message = "Last Runtime connection restored."
	} else {
		p.message = "Runtime connected. You can start conversations now; an explicit business identity is optional."
	}
}

func (p *Controller) renderRuntime(c *ui.Context) {
	s := &p.settings
	conn := p.services.Connection.Status().Value
	platform := p.services.Platform.Status().Value
	if conn != nil && conn.Connected {
		p.section(c, "Current Runtime", func() {
			label := conn.RuntimeID
			for _, runtime := range s.runtimes {
				if runtime.RuntimeID == conn.RuntimeID {
					label = runtimeChoiceLabel(runtime)
					break
				}
			}
			p.note(c, localization.Text("connectedRuntime", map[string]any{"runtime": label, "host": conn.EndpointHost}))
			if conn.HasBusinessIdentity {
				p.note(c, "Session identity: explicit business identity")
			} else {
				p.note(c, "Session identity: Runtime key default · optional override in Authentication")
			}
		})
	} else {
		p.section(c, "Runtime for this account", func() {
			account := localization.Text("currentAccount", nil)
			if platform != nil && platform.Principal != nil {
				account = platform.Principal.AccountID
			}
			p.note(c, localization.Text("runtimeNotSelectedForAccount", map[string]any{"account": account}))
		})
	}
	sectionTitle := "Select Runtime"
	if conn != nil && conn.Connected {
		sectionTitle = "Switch Runtime"
	}
	p.section(c, sectionTitle, func() {
		p.note(c, "The directory loads automatically. Select a Runtime to switch immediately.")
		ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Grow(1).Children(func() { p.renderRegionSelect(c, "Region") })
			ui.Column(c).Grow(1).Children(func() { p.renderProjectField(c) })
		})
		validDirectory := s.region == s.directoryRegion && s.project == s.directoryProject
		if s.directoryLoaded && validDirectory {
			p.note(c, localization.Text("runtimeCount", map[string]any{"count": len(s.runtimes)}))
		}
		options := make([]string, 0, len(s.runtimes))
		for _, runtime := range s.runtimes {
			options = append(options, runtimeChoiceLabel(runtime))
		}
		ui.Column(c.Key("Runtime selector field")).Gap(5).Children(func() {
			ui.Text(c, localization.UI("Select Runtime")).Label(localization.UI("Select Runtime") + " caption").FontSize(12).TextColor(c.Theme().TextMuted)
			combo := ui.Combobox(c.Key(fmt.Sprintf("runtime-combobox:%d", s.directoryGeneration)), &s.runtimeChoice, options).
				Label(localization.UI("Find by name or ID")).FillWidth().Disabled(p.busy || p.confirm || !s.directoryLoaded || len(options) == 0)
			if combo.Changed() {
				if runtime, ok := s.chooseRuntimeLabel(s.runtimeChoice); ok && (conn == nil || !conn.Connected || conn.RuntimeID != runtime.RuntimeID) {
					p.bindRuntime(runtime, false)
				}
			}
		})
		if s.directoryLoaded && validDirectory && len(s.runtimes) == 0 {
			p.note(c, "No MPA Runtimes are visible in this account, Region and Project.")
		}
	})
}
func (p *Controller) renderAuthentication(c *ui.Context) {
	s := &p.settings
	conn := p.services.Connection.Status().Value
	if !conn.Connected {
		p.note(c, "Choose and connect a Runtime first.")
		return
	}
	p.section(c, "Business identity", func() {
		p.note(c, "Runtime "+conn.RuntimeID)
		p.field(c, "Business user ID", &s.userID, false)
		p.field(c, "Legacy JWT", &s.jwt, true)
		ui.Checkbox(c, &s.verifiedTest, localization.Text("verifiedTest", nil)).Disabled(p.busy)
		p.button(c, "Save business identity", func() {
			mode := "legacyJwt"
			if s.verifiedTest {
				mode = "explicitTest"
			}
			r := dto.IdentityRequest{ConnectionEpoch: p.epoch(), AuthMode: mode, LegacyJWT: s.jwt, TestSubject: s.userID, UserID: s.userID, VerifiedTestDeployment: s.verifiedTest}
			s.jwt = ""
			p.identityAsync(func(ctx context.Context) (func(), *dto.SafeError) {
				out := p.services.Connection.SetBusinessIdentity(ctx, r)
				return func() {
					if out.OK {
						p.settings.userID = r.UserID
						p.settings.verifiedTest = r.VerifiedTestDeployment
						p.message = "Business identity saved."
					}
				}, out.Error
			})
		})
		p.button(c, "Test connection", func() {
			r := dto.ScopeRequest{ConnectionEpoch: p.epoch()}
			p.async(func(ctx context.Context) (func(), *dto.SafeError) {
				out := p.services.Connection.Test(ctx, r)
				return func() {
					if out.OK && out.Value != nil {
						p.settings.capabilities = out.Value.Capabilities
					}
				}, out.Error
			})
		})
		for _, probe := range s.capabilities {
			p.note(c, probe.Name+": "+probe.State)
		}
	})
	p.section(c, "Channel administrator", func() {
		ui.Select(c.Key("Channel authentication mode"), &s.authMode, []string{"runtimeKey", "legacyJwt", "none"}).Label(localization.Text("channelAuthenticationMode", nil)).Disabled(p.busy)
		if s.authMode != "none" {
			p.field(c, "Channel credential", &s.channelKey, true)
		}
		p.button(c, "Save channel authentication", func() {
			r := dto.ChannelAuthRequest{ConnectionEpoch: p.epoch(), AuthMode: s.authMode}
			if s.authMode == "legacyJwt" {
				r.LegacyJWT = s.channelKey
			} else {
				r.ChannelKey = s.channelKey
			}
			s.channelKey = ""
			p.identityAsync(func(ctx context.Context) (func(), *dto.SafeError) {
				out := p.services.Connection.ConfigureChannelAuth(ctx, r)
				return func() {
					if out.OK {
						p.settings.authMode = r.AuthMode
						p.message = "Channel authentication saved."
					}
				}, out.Error
			})
		})
	})
}
