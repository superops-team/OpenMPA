package nativeui

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
	"github.com/superops-team/OpenMPA/desktop/internal/channels"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"strings"
	"time"
)

type channelState struct {
	tab, selected, channel, mode, appID, secret, domain, bindingType, groupScope string
	capabilities                                                                 *channels.Capabilities
	accounts                                                                     []channels.Account
	loaded                                                                       bool
	binding                                                                      *channels.Binding
	qr                                                                           *ui.Bitmap
	recoveryID, recoveryOperation                                                string
	diagnostics                                                                  *channels.Diagnostics
	permissions                                                                  []channels.Permission
	chatID                                                                       string
	profile                                                                      *channels.Profile
	agentsMD, model, skills, mcp                                                 string
	inheritPrompt, inheritModel, inheritSkills, inheritMCP                       bool
	upgradeSession                                                               string
	private                                                                      *channels.PrivateUser
}

func newChannelState() channelState {
	return channelState{tab: "Accounts", channel: "feishu", mode: "qr", domain: "feishu", bindingType: "shared", groupScope: "group"}
}
func (s *channelState) account() (channels.Account, bool) {
	for _, a := range s.accounts {
		if a.Channel+":"+a.AppID == s.selected {
			return a, true
		}
	}
	return channels.Account{}, false
}
func (s *channelState) bindingRequest(epoch uint64, op string) channels.BindingCreateRequest {
	r := channels.BindingCreateRequest{ConnectionEpoch: epoch, OperationID: op, Channel: s.channel, Mode: s.mode}
	switch s.channel {
	case "feishu":
		r.Domain = s.domain
		r.BindingType = s.bindingType
		if s.mode == "manual" {
			r.AppID = s.appID
			r.AppSecret = s.secret
		}
	case "dingtalk":
		if s.mode == "manual" {
			r.ClientID = s.appID
			r.ClientSecret = s.secret
		}
	case "wecom":
		r.Mode = "manual"
		r.BotID = s.appID
		r.Secret = s.secret
	}
	return r
}
func (p *Controller) loadChannels() {
	r := channels.ScopeRequest{ConnectionEpoch: p.epoch()}
	p.async(func(context.Context) (func(), *dto.SafeError) {
		caps := p.services.Channels.Capabilities(r)
		if !caps.OK {
			return nil, caps.Error
		}
		out := p.services.Channels.List(r)
		return func() {
			p.channels.capabilities = caps.Value
			if out.OK && out.Value != nil {
				p.channels.accounts = out.Value.Channels
				p.channels.loaded = true
			}
		}, out.Error
	})
}
func (p *Controller) multiline(c *ui.Context, label string, value *string) {
	ui.Column(c.Key(label + " field")).Gap(5).Children(func() {
		ui.Text(c, label).Label(label + " caption").FontSize(12).TextColor(c.Theme().TextMuted)
		ui.TextArea(c.Key(label), value).Label(label).Lines(3, 8).FillWidth().Disabled(p.busy)
	})
}
func (p *Controller) renderChannels(c *ui.Context) {
	s := &p.channels
	if !p.services.Connection.Status().Value.Connected {
		p.note(c, "Connect a Runtime to manage channels.")
		return
	}
	ui.Row(c).Gap(8).Children(func() {
		for _, tab := range []string{"Accounts", "Add channel", "Permissions", "Bot profile", "Private user"} {
			tab := tab
			p.button(c, tab, func() { s.tab = tab; p.err = nil; p.message = "" })
		}
	})
	p.button(c, "Refresh channels", p.loadChannels)
	if s.capabilities != nil {
		p.note(c, "Management scope: "+s.capabilities.ManagementScope)
	}
	switch s.tab {
	case "Add channel":
		p.renderBinding(c)
	case "Permissions":
		p.renderPermissions(c)
	case "Bot profile":
		p.renderChannelProfile(c)
	case "Private user":
		p.renderPrivateUser(c)
	default:
		p.section(c, "Channel accounts", func() {
			if s.loaded && len(s.accounts) == 0 {
				p.note(c, "No channel accounts configured.")
			}
			p.rows(c, "channel-directory", len(s.accounts), 220, func(i int) {
				a := s.accounts[i]

				ui.Row(c.Key(a.Channel + ":" + a.AppID)).Gap(12).Children(func() {
					ui.Column(c).Grow(1).Children(func() {
						ui.Text(c, a.AppName+" · "+a.AppID).FontSize(14)
						p.note(c, fmt.Sprintf("%s · Enabled: %t · Owner: %s", a.Channel, a.Enabled, a.OwnerUserID))
					})
					p.button(c, "Manage "+a.Channel+":"+a.AppID, func() {
						s.selected = a.Channel + ":" + a.AppID
						s.secret = ""
						s.diagnostics = nil
						s.profile = nil
						s.private = nil
						s.permissions = nil
						s.groupScope = a.GroupSessionScope
						if s.groupScope == "" {
							s.groupScope = "group"
						}
					})
				})
			})
		})
		a, ok := s.account()
		if !ok {
			return
		}
		r := channels.AccountRequest{ConnectionEpoch: p.epoch(), Channel: a.Channel, AppID: a.AppID}
		p.section(c, "Configuration · "+a.AppID, func() {
			p.field(c, "New channel credential", &s.secret, true)
			if a.Channel == "feishu" {
				ui.Select(c.Key("Account group session scope"), &s.groupScope, []string{"group", "group_sender", "group_topic", "group_topic_sender"}).Label("Account group session scope").Disabled(p.busy)
			}
			p.button(c, "Save channel configuration", func() {
				request := channels.ConfigRequest{ConnectionEpoch: r.ConnectionEpoch, Channel: a.Channel, AppID: a.AppID}
				switch a.Channel {
				case "feishu":
					request.AppSecret = s.secret
					request.Domain = a.Config.Domain
					request.BindingType = a.BindingType
					request.GroupSessionScope = s.groupScope
				case "dingtalk":
					request.ClientSecret = s.secret
				case "wecom":
					request.Secret = s.secret
				}
				s.secret = ""
				p.ask("Update channel configuration", a.Channel+" · "+a.AppID, func() {
					p.async(func(context.Context) (func(), *dto.SafeError) {
						out := p.services.Channels.SaveConfig(request)
						return func() {
							if out.Value != nil {
								p.replaceAccount(*out.Value)
							}
						}, out.Error
					})
				})
			})
			if a.Channel == "feishu" {
				label := "Enable account"
				if a.Enabled {
					label = "Disable account"
				}
				p.button(c, label, func() {
					request := channels.EnabledRequest{ConnectionEpoch: r.ConnectionEpoch, Channel: a.Channel, AppID: a.AppID, Enabled: !a.Enabled}
					p.async(func(context.Context) (func(), *dto.SafeError) {
						out := p.services.Channels.SetEnabled(request)
						return func() {
							if out.Value != nil {
								p.replaceAccount(*out.Value)
							}
						}, out.Error
					})
				})
			}
			p.button(c, "Read diagnostics", func() {
				p.async(func(context.Context) (func(), *dto.SafeError) {
					out := p.services.Channels.Diagnostics(r)
					return func() {
						if out.OK {
							p.channels.diagnostics = out.Value
						}
					}, out.Error
				})
			})
			if s.diagnostics != nil {
				d := s.diagnostics
				p.note(c, fmt.Sprintf("Configured: %t · Gateway: %t · Route: %t · Delivery: %s", d.Configured, d.GatewayConfigured, d.RouteConfigured, d.DeliveryHealth))
				p.note(c, "Missing configuration: "+strings.Join(d.MissingConfiguration, ", "))
			}
			p.button(c, "Remove channel account", func() {
				p.ask("Remove channel account", a.Channel+" · "+a.AppID+"\nConversation history is retained.", func() {
					p.async(func(context.Context) (func(), *dto.SafeError) {
						out := p.services.Channels.Remove(r)
						return func() {
							if out.OK {
								items := p.channels.accounts[:0]
								for _, v := range p.channels.accounts {
									if v.Channel != a.Channel || v.AppID != a.AppID {
										items = append(items, v)
									}
								}
								p.channels.accounts = items
								p.channels.selected = ""
								p.message = "Channel account removed."
							}
						}, out.Error
					})
				})
			})
		})
	}
}
func (p *Controller) replaceAccount(a channels.Account) {
	for i, v := range p.channels.accounts {
		if v.Channel == a.Channel && v.AppID == a.AppID {
			p.channels.accounts[i] = a
			return
		}
	}
	p.channels.accounts = append(p.channels.accounts, a)
}
func (p *Controller) acceptBinding(b *channels.Binding) {
	if p.bindingTimer != nil {
		p.bindingTimer.Stop()
		p.bindingTimer = nil
	}
	p.channels.binding = b
	p.channels.qr = nil
	if b == nil {
		return
	}
	parts := strings.SplitN(b.QRCodeImage, ",", 2)
	if len(parts) == 2 && len(parts[1]) < 2*1024*1024 {
		if data, e := base64.StdEncoding.DecodeString(parts[1]); e == nil {
			p.channels.qr, _ = ui.DecodeBitmap(data)
		}
	}
	if p.window != nil && b.ID != "" && (b.State == "waiting" || b.State == "pending" || b.State == "registering") {
		generation := p.generation
		request := channels.BindingRequest{ConnectionEpoch: p.epoch(), OperationID: b.OperationID, Channel: b.Channel, BindingID: b.ID}
		delay := b.PollAfterSeconds
		if delay < 2 {
			delay = 2
		}
		if delay > 30 {
			delay = 30
		}
		p.bindingTimer = time.AfterFunc(time.Duration(delay)*time.Second, func() {
			p.window.Update(func() {
				if p.closed || p.generation != generation || p.channels.binding == nil || p.channels.binding.ID != request.BindingID {
					return
				}
				if p.busy {
					p.acceptBinding(p.channels.binding)
					return
				}
				p.async(func(context.Context) (func(), *dto.SafeError) {
					out := p.services.Channels.ReadBinding(request)
					return func() {
						if out.Value != nil {
							p.acceptBinding(out.Value)
							if out.Value.State == "registered" {
								p.loadChannels()
							}
						}
					}, out.Error
				})
			})
		})
	}
}

func (p *Controller) renderBinding(c *ui.Context) {
	s := &p.channels
	p.section(c, "Add a channel", func() {
		if s.capabilities == nil {
			p.note(c, "Refresh channels to read supported binding methods.")
			return
		}
		ui.Select(c.Key("Channel type"), &s.channel, s.capabilities.Channels).Label("Channel type").Disabled(p.busy).OnChange(func() {
			s.secret = ""
			s.appID = ""
			s.binding = nil
			s.qr = nil
			if s.channel == "wecom" {
				s.mode = "manual"
			}
		})
		if s.channel != "wecom" {
			ui.Select(c.Key("Binding method"), &s.mode, []string{"qr", "manual"}).Label("Binding method").Disabled(p.busy)
		}
		if s.channel == "feishu" {
			ui.Select(c.Key("Feishu domain"), &s.domain, s.capabilities.FeishuDomains).Label("Feishu domain").Disabled(p.busy)
			ui.Select(c.Key("Bot binding type"), &s.bindingType, s.capabilities.BotBindingTypes).Label("Bot binding type").Disabled(p.busy)
		}
		if s.mode == "manual" || s.channel == "wecom" {
			p.field(c, "Application or bot ID", &s.appID, false)
			p.field(c, "Application secret", &s.secret, true)
		}
		ui.Button(c, "Create channel binding").Disabled(p.busy || s.binding != nil).OnClick(func() {
			r := s.bindingRequest(p.epoch(), uuid.NewString())
			s.secret = ""
			p.ask("Create channel binding", r.Channel+" · "+r.Mode, func() {
				p.channels.binding = &channels.Binding{OperationID: r.OperationID, Channel: r.Channel, State: "submitted"}
				p.async(func(context.Context) (func(), *dto.SafeError) {
					out := p.services.Channels.CreateBinding(r)
					return func() {
						if out.Value != nil {
							p.acceptBinding(out.Value)
						}
					}, out.Error
				})
			})
		})
		if s.binding != nil {
			b := *s.binding
			p.note(c, b.OperationID+" · "+b.State+" · "+b.ErrorCode)
			if s.qr != nil {
				ui.Image(c, s.qr).Size(200, 200)
			}
			p.note(c, "Binding "+b.ID+" · expires "+b.ExpiresAt)
			p.button(c, "Read binding state", func() {
				r := channels.BindingRequest{ConnectionEpoch: p.epoch(), OperationID: b.OperationID, Channel: b.Channel, BindingID: b.ID}
				p.async(func(context.Context) (func(), *dto.SafeError) {
					out := p.services.Channels.ReadBinding(r)
					return func() {
						if out.Value != nil {
							p.acceptBinding(out.Value)
						}
					}, out.Error
				})
			})
			p.button(c, "Retry channel registration", func() {
				r := channels.BindingRequest{ConnectionEpoch: p.epoch(), OperationID: b.OperationID, Channel: b.Channel, BindingID: b.ID}
				p.ask("Retry registration", b.Channel+" · "+b.ID, func() {
					p.async(func(context.Context) (func(), *dto.SafeError) {
						out := p.services.Channels.RetryBinding(r)
						return func() {
							if out.Value != nil {
								p.acceptBinding(out.Value)
							}
						}, out.Error
					})
				})
			})
			if b.State == "registered" || b.State == "failed" || b.State == "expired" {
				p.button(c, "New binding", func() { s.binding = nil; s.qr = nil })
			}
		}
	})
	p.section(c, "Recover an existing binding", func() {
		p.field(c, "Binding operation ID", &s.recoveryOperation, false)
		p.field(c, "Binding ID", &s.recoveryID, false)
		p.button(c, "Recover binding", func() {
			r := channels.BindingRequest{ConnectionEpoch: p.epoch(), Channel: s.channel, OperationID: s.recoveryOperation, BindingID: s.recoveryID, ResumeConfirmed: true}
			p.ask("Recover binding", r.Channel+" · "+r.BindingID, func() {
				p.async(func(context.Context) (func(), *dto.SafeError) {
					out := p.services.Channels.ReadBinding(r)
					return func() {
						if out.Value != nil {
							p.acceptBinding(out.Value)
						}
					}, out.Error
				})
			})
		})
	})
}
func (p *Controller) renderPermissions(c *ui.Context) {
	s := &p.channels
	a, ok := s.account()
	if !ok {
		p.note(c, "Choose an account on the Accounts tab.")
		return
	}
	if a.Channel != "feishu" {
		p.note(c, "Group permissions are unavailable for this channel.")
		return
	}
	r := channels.AccountRequest{ConnectionEpoch: p.epoch(), Channel: a.Channel, AppID: a.AppID}
	p.section(c, "Group permissions · "+a.AppID, func() {
		p.button(c, "Read group permissions", func() {
			p.async(func(context.Context) (func(), *dto.SafeError) {
				out := p.services.Channels.Permissions(r)
				return func() {
					if out.OK && out.Value != nil {
						p.channels.permissions = out.Value.Permissions
					}
				}, out.Error
			})
		})
		p.field(c, "Chat ID", &s.chatID, false)
		ui.Select(c.Key("Group session scope"), &s.groupScope, []string{"group", "group_sender", "group_topic", "group_topic_sender"}).Label("Group session scope").Disabled(p.busy)
		p.button(c, "Save group permission", func() {
			request := channels.PermissionRequest{ConnectionEpoch: r.ConnectionEpoch, Channel: r.Channel, AppID: r.AppID, ChatID: s.chatID, GroupSessionScope: s.groupScope}
			p.async(func(context.Context) (func(), *dto.SafeError) {
				out := p.services.Channels.SavePermission(request)
				return func() {
					if out.OK && out.Value != nil {
						found := false
						for i, v := range p.channels.permissions {
							if v.ChatID == out.Value.ChatID {
								p.channels.permissions[i] = *out.Value
								found = true
								break
							}
						}
						if !found {
							p.channels.permissions = append(p.channels.permissions, *out.Value)
						}
						p.message = "Permission saved. Refresh to read the current list."
					}
				}, out.Error
			})
		})
		for _, v := range s.permissions {
			v := v
			ui.Row(c.Key(v.ChatID)).Gap(8).Children(func() {
				ui.Text(c, v.ChatName+" · "+v.ChatID+" · "+v.GroupSessionScope).Grow(1)
				p.button(c, "Delete permission "+v.ChatID, func() {
					request := channels.PermissionRequest{ConnectionEpoch: r.ConnectionEpoch, Channel: r.Channel, AppID: r.AppID, ChatID: v.ChatID}
					p.ask("Delete group permission", a.AppID+" · "+v.ChatID, func() {
						p.async(func(context.Context) (func(), *dto.SafeError) {
							out := p.services.Channels.DeletePermission(request)
							return func() {
								if out.OK {
									items := p.channels.permissions[:0]
									for _, item := range p.channels.permissions {
										if item.ChatID != v.ChatID {
											items = append(items, item)
										}
									}
									p.channels.permissions = items
								}
							}, out.Error
						})
					})
				})
			})
		}
	})
}
func refs(v string) *[]string {
	out := []string{}
	for _, ref := range strings.Split(v, ",") {
		if ref = strings.TrimSpace(ref); ref != "" {
			out = append(out, ref)
		}
	}
	return &out
}
