package nativeui

import (
	"context"
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/runtimeconfig"
	"strconv"
	"strings"
)

type settingsState struct {
	tab, ak, sk, securityToken, region, project, search, selected string
	remember, verifiedTest                                        bool
	directoryRegion, directoryProject                             string
	directoryLoaded                                               bool
	runtimes                                                      []dto.RuntimeSummary
	profiles                                                      []dto.SavedProfile
	userID, jwt, authMode, channelKey                             string
	capabilities                                                  []dto.Capability
	images                                                        *deployment.Images
	runtimeImage, workerImage                                     string
	operations                                                    []deployment.Operation
	config                                                        *runtimeconfig.Snapshot
	description, cpu, memory, min, max, concurrency               string
	configOperation                                               *runtimeconfig.Operation
	resourceReceiptID                                             string
}

func (s *settingsState) selectedRuntime() (dto.RuntimeSummary, bool) {
	if s.region != s.directoryRegion || s.project != s.directoryProject {
		return dto.RuntimeSummary{}, false
	}
	for _, r := range s.runtimes {
		if r.RuntimeID == s.selected && r.Region == s.region && r.ProjectName == s.project && runtimeMatches(r, s.search) {
			return r, true
		}
	}
	return dto.RuntimeSummary{}, false
}
func runtimeMatches(r dto.RuntimeSummary, search string) bool {
	return strings.Contains(strings.ToLower(r.Name+" "+r.RuntimeID), strings.ToLower(strings.TrimSpace(search)))
}
func (s *settingsState) configValues() (runtimeconfig.Values, *dto.SafeError) {
	values := []int{}
	for _, v := range []string{s.cpu, s.memory, s.min, s.max, s.concurrency} {
		n, e := strconv.Atoi(strings.TrimSpace(v))
		if e != nil {
			return runtimeconfig.Values{}, dto.Failure("invalidInput", "platform", "never", 0)
		}
		values = append(values, n)
	}
	v := runtimeconfig.Values{Description: s.description, CPUMilli: values[0], MemoryMB: values[1], MinInstance: values[2], MaxInstance: values[3], MaxConcurrency: values[4]}
	if len(v.Description) > 4096 || v.CPUMilli < 100 || v.CPUMilli > 64000 || v.MemoryMB < 128 || v.MemoryMB > 262144 || v.MinInstance < 0 || v.MaxInstance < 1 || v.MaxInstance > 1000 || v.MinInstance > v.MaxInstance || v.MaxConcurrency < 1 || v.MaxConcurrency > 100000 {
		return v, dto.Failure("invalidInput", "platform", "never", 0)
	}
	return v, nil
}
func (p *Controller) renderSettings(c *ui.Context) {
	s := &p.settings
	ui.Row(c).Gap(8).Children(func() {
		for _, tab := range []string{"Account", "Runtime", "Authentication", "Deployment"} {
			tab := tab
			p.button(c, tab, func() { s.tab = tab; p.err = nil; p.message = "" })
		}
	})
	switch s.tab {
	case "Runtime":
		p.renderRuntime(c)
	case "Authentication":
		p.renderAuthentication(c)
	case "Deployment":
		p.renderDeployment(c)
	default:
		p.renderAccount(c)
	}
}
func (p *Controller) renderAccount(c *ui.Context) {
	s := &p.settings
	platform := p.services.Platform.Status().Value
	p.section(c, "Volcengine account", func() {
		if platform.SignedIn && platform.Principal != nil {
			p.note(c, "Account "+platform.Principal.AccountID+" · "+platform.Principal.IdentityType)
			if platform.Remembered {
				p.note(c, "Credentials saved in macOS Keychain")
			}
			p.button(c, "Sign out", func() {
				p.identityAsync(func(context.Context) (func(), *dto.SafeError) {
					r := p.services.Platform.SignOut()
					return nil, r.Error
				})
			})
		} else {
			p.field(c, "Access key", &s.ak, true)
			p.field(c, "Secret key", &s.sk, true)
			p.field(c, "Security token (optional)", &s.securityToken, true)
			p.field(c, "Login region", &s.region, false)
			ui.Checkbox(c, &s.remember, "Remember in macOS Keychain").Disabled(p.busy)
			p.button(c, "Sign in", func() {
				r := dto.SignInRequest{Remember: s.remember, AccessKey: s.ak, SecretKey: s.sk, SecurityToken: s.securityToken, Region: s.region}
				s.ak = ""
				s.sk = ""
				s.securityToken = ""
				p.identityAsync(func(ctx context.Context) (func(), *dto.SafeError) {
					out := p.services.Platform.SignIn(ctx, r)
					return func() {
						if out.OK {
							p.settings.tab = "Runtime"
							p.loadDirectory()
						}
					}, out.Error
				})
			})
		}
	})
	p.section(c, "Saved accounts", func() {
		p.button(c, "Refresh saved accounts", func() {
			p.async(func(context.Context) (func(), *dto.SafeError) {
				r := p.services.Platform.SavedProfiles()
				return func() {
					if r.OK && r.Value != nil {
						p.settings.profiles = r.Value.Profiles
					}
				}, r.Error
			})
		})
		for _, profile := range s.profiles {
			profile := profile
			ui.Row(c.Key(profile.ProfileID)).Gap(10).Children(func() {
				ui.Text(c, profile.Label+" · "+profile.AccountID).Grow(1)
				p.button(c, "Restore "+profile.AccountID, func() {
					p.identityAsync(func(ctx context.Context) (func(), *dto.SafeError) {
						r := p.services.Platform.Restore(ctx, dto.RestoreRequest{ProfileID: profile.ProfileID})
						return func() {
							if r.OK {
								p.settings.region = profile.Region
								p.settings.project = profile.ProjectName
								p.settings.tab = "Runtime"
								p.loadDirectory()
							}
						}, r.Error
					})
				})
			})
		}
	})
}
func (p *Controller) loadDirectory() {
	r := dto.RuntimeListRequest{Region: p.settings.region, ProjectName: p.settings.project}
	p.settings.runtimes = nil
	p.settings.selected = ""
	p.settings.directoryLoaded = false
	p.async(func(ctx context.Context) (func(), *dto.SafeError) {
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
			}
		}, out.Error
	})
}
func (p *Controller) renderRuntime(c *ui.Context) {
	s := &p.settings
	conn := p.services.Connection.Status().Value
	p.section(c, "Choose an MPA Runtime", func() {
		ui.Row(c).Gap(14).Children(func() {
			ui.Column(c).Grow(1).Children(func() { p.field(c, "Region", &s.region, false) })
			ui.Column(c).Grow(1).Children(func() { p.field(c, "Project", &s.project, false) })
		})
		p.button(c, "Refresh MPA Runtimes", p.loadDirectory)
		p.field(c, "Find by name or ID", &s.search, false)
		validDirectory := s.region == s.directoryRegion && s.project == s.directoryProject
		if s.directoryLoaded && validDirectory {
			p.note(c, fmt.Sprintf("%d MPA Runtimes in this account and project", len(s.runtimes)))
		}
		if validDirectory {
			filtered := []dto.RuntimeSummary{}
			for _, r := range s.runtimes {
				if runtimeMatches(r, s.search) {
					filtered = append(filtered, r)
				}
			}
			p.rows(c, "runtime-directory", len(filtered), 230, func(i int) {
				r := filtered[i]

				ui.Row(c.Key(r.RuntimeID)).Gap(12).Children(func() {
					title := r.Name + " · " + r.RuntimeID + " · " + r.Status
					if s.selected == r.RuntimeID {
						title = "✓ " + title
					}
					ui.Text(c, title).Grow(1).FontSize(13)
					p.button(c, "Choose "+r.RuntimeID, func() { s.selected = r.RuntimeID })
				})
			})
		}
		r, valid := s.selectedRuntime()
		ui.Button(c, "Connect selected Runtime").Disabled(p.busy || !valid || !s.directoryLoaded).OnClick(func() {
			request := dto.RuntimeRequest{Region: r.Region, ProjectName: r.ProjectName, RuntimeID: r.RuntimeID}
			p.ask("Switch Runtime", "Connect to "+r.Name+" ("+r.RuntimeID+")?", func() {
				p.identityAsync(func(ctx context.Context) (func(), *dto.SafeError) {
					out := p.services.Platform.BindRuntime(ctx, request)
					return func() {
						if out.OK {
							p.message = "Runtime connected. Configure its business identity to start conversations."
						}
					}, out.Error
				})
			})
		})
	})
	if conn.Connected {
		p.note(c, "Connected Runtime: "+conn.RuntimeID+" · "+conn.EndpointHost)
	}
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
		ui.Checkbox(c, &s.verifiedTest, "Verified test deployment with JWT disabled").Disabled(p.busy)
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
		ui.Select(c.Key("Channel authentication mode"), &s.authMode, []string{"runtimeKey", "legacyJwt", "none"}).Label("Channel authentication mode").Disabled(p.busy)
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
func (p *Controller) renderDeployment(c *ui.Context) {
	s := &p.settings
	if !p.services.Connection.Status().Value.Connected {
		p.note(c, "Choose and connect a Runtime first.")
		return
	}
	p.section(c, "Runtime resources", func() {
		p.button(c, "Read current resources", func() {
			r := runtimeconfig.ScopeRequest{ConnectionEpoch: p.epoch()}
			p.async(func(context.Context) (func(), *dto.SafeError) {
				out := p.services.RuntimeConfig.Read(r)
				return func() {
					if out.OK && out.Value != nil {
						v := out.Value
						p.settings.config = v
						p.settings.description = v.Values.Description
						p.settings.cpu = strconv.Itoa(v.Values.CPUMilli)
						p.settings.memory = strconv.Itoa(v.Values.MemoryMB)
						p.settings.min = strconv.Itoa(v.Values.MinInstance)
						p.settings.max = strconv.Itoa(v.Values.MaxInstance)
						p.settings.concurrency = strconv.Itoa(v.Values.MaxConcurrency)
					}
				}, out.Error
			})
		})
		if s.config != nil {
			p.note(c, fmt.Sprintf("%s · version %d · %s", s.config.RuntimeID, s.config.Version, s.config.Status))
			p.field(c, "Description", &s.description, false)
			ui.Row(c).Gap(14).Children(func() {
				ui.Column(c).Grow(1).Children(func() { p.field(c, "CPU (milli cores)", &s.cpu, false) })
				ui.Column(c).Grow(1).Children(func() { p.field(c, "Memory (MB)", &s.memory, false) })
			})
			ui.Row(c).Gap(14).Children(func() {
				ui.Column(c).Grow(1).Children(func() { p.field(c, "Minimum instances", &s.min, false) })
				ui.Column(c).Grow(1).Children(func() { p.field(c, "Maximum instances", &s.max, false) })
				ui.Column(c).Grow(1).Children(func() { p.field(c, "Maximum concurrency", &s.concurrency, false) })
			})
			p.button(c, "Update Runtime resources", func() {
				v, e := s.configValues()
				if e != nil {
					p.err = e
					return
				}
				r := runtimeconfig.UpdateRequest{ConnectionEpoch: p.epoch(), OperationID: uuid.NewString(), Fingerprint: s.config.Fingerprint, Values: v, Confirm: true}
				p.ask("Update Runtime resources", fmt.Sprintf("%s · CPU %d · Memory %d MB · Instances %d–%d · Concurrency %d", s.config.RuntimeID, v.CPUMilli, v.MemoryMB, v.MinInstance, v.MaxInstance, v.MaxConcurrency), func() {
					p.settings.configOperation = &runtimeconfig.Operation{OperationID: r.OperationID, RuntimeID: p.services.Connection.Status().Value.RuntimeID, Target: r.Values, State: "submitted"}
					p.async(func(context.Context) (func(), *dto.SafeError) {
						out := p.services.RuntimeConfig.Update(r)
						return func() {
							p.settings.config = nil
							if out.Value != nil {
								p.settings.configOperation = out.Value
							}
						}, out.Error
					})
				})
			})
		}
		p.field(c, "Resource update operation ID", &s.resourceReceiptID, false)
		p.button(c, "Recover resource update receipt", func() {
			r := runtimeconfig.ReadRequest{ScopeRequest: runtimeconfig.ScopeRequest{ConnectionEpoch: p.epoch()}, OperationID: s.resourceReceiptID}
			p.async(func(context.Context) (func(), *dto.SafeError) {
				out := p.services.RuntimeConfig.ReadUpdate(r)
				return func() {
					if out.Value != nil {
						p.settings.configOperation = out.Value
					}
				}, out.Error
			})
		})
		if s.configOperation != nil {
			op := *s.configOperation
			p.note(c, op.OperationID+" · "+op.State)
			p.button(c, "Read resource update receipt", func() {
				r := runtimeconfig.ReadRequest{ScopeRequest: runtimeconfig.ScopeRequest{ConnectionEpoch: p.epoch()}, OperationID: op.OperationID}
				p.async(func(context.Context) (func(), *dto.SafeError) {
					out := p.services.RuntimeConfig.ReadUpdate(r)
					return func() {
						if out.Value != nil {
							p.settings.configOperation = out.Value
						}
					}, out.Error
				})
			})
		}
	})
	p.section(c, "Runtime and Worker Tool images", func() {
		p.button(c, "Read current images", func() {
			r := deployment.ScopeRequest{ConnectionEpoch: p.epoch()}
			p.async(func(context.Context) (func(), *dto.SafeError) {
				out := p.services.Deployment.ReadImages(r)
				return func() {
					if out.OK && out.Value != nil {
						p.settings.images = out.Value
						p.settings.runtimeImage = out.Value.RuntimeImage
						p.settings.workerImage = out.Value.WorkerImage
					}
				}, out.Error
			})
		})
		if s.images != nil {
			p.note(c, "Runtime "+s.images.RuntimeID+" · version "+s.images.RuntimeVersion+" · "+s.images.RuntimeStatus)
			p.note(c, "Worker Tool "+s.images.ToolID+" · "+s.images.WorkerStatus)
			p.field(c, "Runtime image", &s.runtimeImage, false)
			p.button(c, "Update Runtime image", func() { p.updateImage(false) })
			p.field(c, "Worker image", &s.workerImage, false)
			p.button(c, "Update Worker Tool image", func() { p.updateImage(true) })
		}
		p.button(c, "Refresh saved image receipts", func() {
			r := deployment.ScopeRequest{ConnectionEpoch: p.epoch()}
			p.async(func(context.Context) (func(), *dto.SafeError) {
				out := p.services.Deployment.ListUpdates(r)
				return func() {
					if out.OK && out.Value != nil {
						p.settings.operations = out.Value.Operations
					}
				}, out.Error
			})
		})
		for _, op := range s.operations {
			op := op
			ui.Column(c.Key(op.OperationID)).Gap(5).Children(func() {
				p.note(c, op.Kind+" · "+op.OperationID+" · "+op.State)
				p.note(c, op.Message)
				p.button(c, "Read receipt "+op.OperationID, func() {
					r := deployment.ReadUpdateRequest{ConnectionEpoch: p.epoch(), OperationID: op.OperationID}
					p.async(func(context.Context) (func(), *dto.SafeError) {
						out := p.services.Deployment.ReadUpdate(r)
						return func() {
							if out.Value != nil {
								p.rememberImageOperation(*out.Value)
							}
						}, out.Error
					})
				})
			})
		}
	})
}
func (p *Controller) rememberImageOperation(op deployment.Operation) {
	for i, v := range p.settings.operations {
		if v.OperationID == op.OperationID {
			p.settings.operations[i] = op
			return
		}
	}
	p.settings.operations = append(p.settings.operations, op)
}
func (p *Controller) updateImage(worker bool) {
	s := &p.settings
	if s.images == nil {
		return
	}
	images := *s.images
	epoch := p.epoch()
	opID := uuid.NewString()
	target := s.runtimeImage
	kind := "Runtime"
	resource := images.RuntimeID
	if worker {
		target = s.workerImage
		kind = "Worker Tool"
		resource = images.ToolID
	}
	p.ask("Update "+kind+" image", resource+"\n"+target, func() {
		p.rememberImageOperation(deployment.Operation{OperationID: opID, Kind: kind, RuntimeID: images.RuntimeID, ToolID: images.ToolID, TargetImage: target, State: "submitted"})
		p.async(func(context.Context) (func(), *dto.SafeError) {
			var out deployment.OperationResult
			if worker {
				out = p.services.Deployment.UpdateWorkerImage(deployment.WorkerImageRequest{ConnectionEpoch: epoch, OperationID: opID, TargetImage: target, ReviewFingerprint: images.ReviewFingerprint})
			} else {
				out = p.services.Deployment.UpdateRuntimeImage(deployment.RuntimeImageRequest{ConnectionEpoch: epoch, OperationID: opID, TargetImage: target, ReviewFingerprint: images.ReviewFingerprint, ReleaseEnable: true})
			}
			return func() {
				p.settings.images = nil
				if out.Value != nil {
					p.rememberImageOperation(*out.Value)
				}
			}, out.Error
		})
	})
}
