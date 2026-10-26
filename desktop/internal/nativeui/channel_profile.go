package nativeui

import (
	"context"
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/channels"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"strings"
)

func (p *Controller) renderChannelProfile(c *ui.Context) {
	s := &p.channels
	a, ok := s.account()
	if !ok {
		p.note(c, "Choose an account on the Accounts tab.")
		return
	}
	r := channels.AccountRequest{ConnectionEpoch: p.epoch(), Channel: a.Channel, AppID: a.AppID}
	p.section(c, "Bot profile · "+a.AppID, func() {
		p.note(c, "This Runtime exposes profiles for authorized accounts. A template catalog is unavailable.")
		p.button(c, "Read bot profile", func() {
			p.async(func(context.Context) (func(), *dto.SafeError) {
				out := p.services.Channels.ReadProfile(r)
				return func() {
					if out.OK && out.Value != nil {
						v := out.Value
						p.channels.profile = v
						p.channels.inheritPrompt = v.AgentsMD == nil
						p.channels.inheritModel = v.Model == nil
						p.channels.inheritSkills = v.Skills == nil
						p.channels.inheritMCP = v.MCP == nil
						if v.AgentsMD != nil {
							p.channels.agentsMD = *v.AgentsMD
						}
						if v.Model != nil {
							p.channels.model = *v.Model
						}
						if v.Skills != nil {
							p.channels.skills = strings.Join(*v.Skills, ", ")
						}
						if v.MCP != nil {
							p.channels.mcp = strings.Join(*v.MCP, ", ")
						}
					}
				}, out.Error
			})
		})
		if s.profile == nil {
			return
		}
		p.note(c, fmt.Sprintf("Revision %d", s.profile.Revision))
		ui.Checkbox(c, &s.inheritPrompt, "Inherit assistant instructions").Disabled(p.busy)
		if !s.inheritPrompt {
			p.multiline(c, "Assistant instructions", &s.agentsMD)
		}
		ui.Checkbox(c, &s.inheritModel, "Inherit model").Disabled(p.busy)
		if !s.inheritModel {
			p.field(c, "Model ID", &s.model, false)
		}
		ui.Checkbox(c, &s.inheritSkills, "Inherit skills").Disabled(p.busy)
		if !s.inheritSkills {
			p.field(c, "Skills (comma separated)", &s.skills, false)
			p.note(c, "An empty list allows all available skills.")
		}
		ui.Checkbox(c, &s.inheritMCP, "Inherit MCP tools").Disabled(p.busy)
		if !s.inheritMCP {
			p.field(c, "MCP tools (comma separated)", &s.mcp, false)
			p.note(c, "An empty list allows all available MCP tools.")
		}
		p.button(c, "Save bot profile", func() {
			request := channels.ProfileSaveRequest{ConnectionEpoch: r.ConnectionEpoch, Channel: a.Channel, AppID: a.AppID, ExpectedRevision: s.profile.Revision}
			if !s.inheritPrompt {
				v := s.agentsMD
				request.AgentsMD = &v
			}
			if !s.inheritModel {
				v := s.model
				request.Model = &v
			}
			if !s.inheritSkills {
				request.Skills = refs(s.skills)
			}
			if !s.inheritMCP {
				request.MCP = refs(s.mcp)
			}
			p.ask("Save bot profile", a.AppID+fmt.Sprintf(" · revision %d", request.ExpectedRevision), func() {
				p.async(func(context.Context) (func(), *dto.SafeError) {
					out := p.services.Channels.SaveProfile(request)
					return func() {
						if out.Value != nil {
							p.channels.profile = out.Value
						}
					}, out.Error
				})
			})
		})
		p.field(c, "Session to apply profile", &s.upgradeSession, false)
		p.button(c, "Apply profile to Session", func() {
			request := channels.UpgradeRequest{ConnectionEpoch: r.ConnectionEpoch, Channel: a.Channel, AppID: a.AppID, SessionID: s.upgradeSession}
			member := false
			for _, v := range p.currentSessions() {
				if v.ID == request.SessionID {
					member = true
				}
			}
			if !member {
				p.err = dto.Failure("invalidInput", "mpa", "never", 0)
				return
			}
			p.ask("Apply bot profile", a.AppID+" → "+request.SessionID, func() {
				p.async(func(context.Context) (func(), *dto.SafeError) {
					out := p.services.Channels.UpgradeProfile(request)
					return func() {
						if out.OK {
							p.message = "Profile applied to Session."
						}
					}, out.Error
				})
			})
		})
	})
}
func (p *Controller) renderPrivateUser(c *ui.Context) {
	s := &p.channels
	a, ok := s.account()
	if !ok {
		p.note(c, "Choose an account on the Accounts tab.")
		return
	}
	r := channels.AccountRequest{ConnectionEpoch: p.epoch(), Channel: a.Channel, AppID: a.AppID}
	p.section(c, "Private bot user · "+a.AppID, func() {
		p.button(c, "Read private user", func() {
			p.async(func(context.Context) (func(), *dto.SafeError) {
				out := p.services.Channels.PrivateUser(r)
				return func() {
					if out.OK {
						p.channels.private = out.Value
					}
				}, out.Error
			})
		})
		if s.private == nil {
			return
		}
		p.note(c, "Authorization: "+s.private.Authorization.Status)
		if s.private.Slot == nil {
			return
		}
		slot := *s.private.Slot
		p.note(c, "User "+slot.RuntimeUserID+" · version "+slot.Version)
		for _, reset := range []bool{false, true} {
			reset := reset
			label := "Revoke private authorization"
			if reset {
				label = "Reset private user"
			}
			p.button(c, label, func() {
				request := channels.PrivateMutationRequest{ConnectionEpoch: r.ConnectionEpoch, Channel: a.Channel, AppID: a.AppID, ExpectedVersion: slot.Version}
				p.ask(label, a.AppID+" · "+slot.Version, func() {
					p.async(func(context.Context) (func(), *dto.SafeError) {
						var out channels.PrivateMutationResult
						if reset {
							out = p.services.Channels.ResetPrivateUser(request)
						} else {
							out = p.services.Channels.RevokePrivateAuthorization(request)
						}
						return func() {
							if out.Value != nil {
								p.channels.private = &out.Value.Current
							}
						}, out.Error
					})
				})
			})
		}
	})
}
