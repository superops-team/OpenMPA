package nativeui

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/configstore"
	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/host"
	"github.com/superops-team/OpenMPA/desktop/internal/ipc"
	"github.com/superops-team/OpenMPA/desktop/internal/localization"
	"github.com/superops-team/OpenMPA/desktop/internal/runtimeconfig"
)

type settingsState struct {
	tab, ak, sk, securityToken, region, project, selected string
	verifiedTest, showCredentials                         bool
	directoryRegion, directoryProject                     string
	directoryLoaded                                       bool
	directoryGeneration                                   uint64
	runtimes                                              []dto.RuntimeSummary
	runtimeChoice                                         string
	userID, jwt, authMode, channelKey                     string
	capabilities                                          []dto.Capability
	images                                                *deployment.Images
	runtimeImage, workerImage                             string
	operations                                            []deployment.Operation
	config                                                *runtimeconfig.Snapshot
	description, cpu, memory, min, max, concurrency       string
	configOperation                                       *runtimeconfig.Operation
	resourceReceiptID                                     string
	language                                              string
	preferredRuntime, preferredPrincipalScope             string
	vaultPassphrase, vaultConfirm                         string
	showVault                                             bool
	vault                                                 dto.VaultStatus
}

func supportedRegion(platform *ipc.Platform, region string) bool {
	if platform == nil {
		return region == configstore.DefaultRegion
	}
	result := platform.Regions()
	if !result.OK || result.Value == nil {
		return region == configstore.DefaultRegion
	}
	for _, item := range result.Value.Regions {
		if item.Region == region {
			return true
		}
	}
	return false
}

func (p *Controller) regionCatalog() []dto.RegionSummary {
	result := p.services.Platform.Regions()
	if result.OK && result.Value != nil && len(result.Value.Regions) > 0 {
		return result.Value.Regions
	}
	return []dto.RegionSummary{{Region: configstore.DefaultRegion, DisplayName: "华北2（北京）", EndpointHost: "agentkit.cn-beijing.volcengineapi.com"}}
}

func (p *Controller) regionOptions() []string {
	regions := p.regionCatalog()
	options := make([]string, 0, len(regions))
	for _, item := range regions {
		options = append(options, item.Region)
	}
	return options
}

func (p *Controller) renderRegionSelect(c *ui.Context, label string) {
	s := &p.settings
	ui.Column(c.Key(label + " field")).Gap(5).Children(func() {
		ui.Text(c, localization.UI(label)).Label(localization.UI(label) + " caption").FontSize(12).TextColor(c.Theme().TextMuted)
		ui.Select(c.Key(label), &s.region, p.regionOptions()).Label(localization.UI(label)).FillWidth().Disabled(p.busy).OnChange(func() {
			p.changeRegion(s.region)
		})
		for _, item := range p.regionCatalog() {
			if item.Region == s.region {
				p.note(c, item.DisplayName+" · "+item.EndpointHost)
				break
			}
		}
	})
}

func (p *Controller) resetRuntimeProjection() {
	p.services.Platform.ResetRuntime()
	p.connectionEpoch = p.epoch()
	if platform := p.services.Platform.Status().Value; platform != nil {
		p.platformEpoch = platform.Epoch
	}
	if p.host != nil {
		host.NotifyConnection(p.host)
	}
}

func (p *Controller) changeRegion(region string) {
	s := &p.settings
	p.stopSettingsTimer()
	if !supportedRegion(p.services.Platform, region) {
		s.region = configstore.DefaultRegion
		p.err = dto.Failure("invalidInput", "platform", "never", 0)
		return
	}
	s.region = region
	s.runtimes = nil
	s.selected = ""
	s.runtimeChoice = ""
	s.preferredRuntime = ""
	s.preferredPrincipalScope = ""
	s.directoryRegion = ""
	s.directoryProject = ""
	s.directoryLoaded = false
	s.directoryGeneration++
	p.resetRuntimeProjection()
	if p.services.Config == nil {
		if platform := p.services.Platform.Status().Value; platform != nil && platform.SignedIn {
			p.loadDirectory(true)
		}
		return
	}
	project := s.project
	p.async(func(ctx context.Context) (func(), *dto.SafeError) {
		if err := p.services.Config.SetDirectory(ctx, region, project); err != nil {
			return nil, dto.Failure("localConfigUnavailable", "desktop", "never", 0)
		}
		return func() {
			if platform := p.services.Platform.Status().Value; platform != nil && platform.SignedIn {
				p.loadDirectory(true)
			}
		}, nil
	})
}

func (p *Controller) renderProjectField(c *ui.Context) {
	s := &p.settings
	ui.Column(c.Key("Project field")).Gap(5).Children(func() {
		ui.Text(c, localization.UI("Project")).Label(localization.UI("Project") + " caption").FontSize(12).TextColor(c.Theme().TextMuted)
		input := ui.TextInput(c.Key("Project"), &s.project).Label(localization.UI("Project")).FillWidth().Disabled(p.busy)
		if input.Changed() {
			p.changeProject()
			p.scheduleDirectoryLoad()
		}
		if input.Submitted() {
			p.stopSettingsTimer()
			p.loadDirectory(true)
		}
	})
}

func (p *Controller) stopSettingsTimer() {
	if p.settingsTimer != nil {
		p.settingsTimer.Stop()
		p.settingsTimer = nil
	}
}

func (p *Controller) scheduleDirectoryLoad() {
	p.stopSettingsTimer()
	platform := p.services.Platform.Status().Value
	if p.window == nil || platform == nil || !platform.SignedIn {
		return
	}
	generation := p.generation
	region, project := p.settings.region, p.settings.project
	p.settingsTimer = time.AfterFunc(400*time.Millisecond, func() {
		p.window.Update(func() {
			if p.closed || p.generation != generation || p.settings.region != region || p.settings.project != project {
				return
			}
			p.settingsTimer = nil
			if !p.busy {
				p.loadDirectory(true)
			}
		})
	})
}

func (p *Controller) changeProject() {
	s := &p.settings
	conn := p.services.Connection.Status().Value
	target := ipc.Target(p.services.Platform)
	hasRuntimeState := (conn != nil && conn.Connected) || target.RuntimeID != "" || s.preferredRuntime != "" || s.selected != "" || s.directoryLoaded || len(s.runtimes) > 0
	if !hasRuntimeState {
		return
	}
	p.resetRuntimeProjection()
	s.runtimes = nil
	s.selected = ""
	s.runtimeChoice = ""
	s.preferredRuntime = ""
	s.preferredPrincipalScope = ""
	s.directoryRegion = ""
	s.directoryProject = ""
	s.directoryLoaded = false
	s.directoryGeneration++
	if p.services.Config != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		if err := p.services.Config.ClearRuntime(ctx); err != nil {
			p.err = dto.Failure("localConfigUnavailable", "desktop", "never", 0)
		}
	}
}

func (s *settingsState) selectedRuntime() (dto.RuntimeSummary, bool) {
	if !s.directoryLoaded || s.region != s.directoryRegion || s.project != s.directoryProject {
		return dto.RuntimeSummary{}, false
	}
	for _, r := range s.runtimes {
		if r.RuntimeID == s.selected && r.Region == s.region && r.ProjectName == s.project {
			return r, true
		}
	}
	return dto.RuntimeSummary{}, false
}

func (s *settingsState) restorableRuntime(platform dto.PlatformState) (dto.RuntimeSummary, bool) {
	if !platform.SignedIn || platform.Principal == nil || platform.Region != s.region || platform.Principal.Scope == "" || s.preferredRuntime == "" || !s.directoryLoaded || s.region != s.directoryRegion || s.project != s.directoryProject {
		return dto.RuntimeSummary{}, false
	}
	if s.preferredPrincipalScope != "" && platform.Principal.Scope != s.preferredPrincipalScope {
		return dto.RuntimeSummary{}, false
	}
	for _, runtime := range s.runtimes {
		if runtime.RuntimeID == s.preferredRuntime && runtime.Region == s.region && runtime.ProjectName == s.project {
			return runtime, true
		}
	}
	return dto.RuntimeSummary{}, false
}

func runtimeChoiceLabel(runtime dto.RuntimeSummary) string {
	name := strings.TrimSpace(runtime.Name)
	if name == "" || name == runtime.RuntimeID {
		return runtime.RuntimeID
	}
	return name + " · " + runtime.RuntimeID
}

func (s *settingsState) chooseRuntimeLabel(choice string) (dto.RuntimeSummary, bool) {
	for _, runtime := range s.runtimes {
		if runtimeChoiceLabel(runtime) == choice && runtime.Region == s.region && runtime.ProjectName == s.project {
			s.selected = runtime.RuntimeID
			s.runtimeChoice = choice
			return runtime, true
		}
	}
	return dto.RuntimeSummary{}, false
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
	tabs := []string{"Account", "Runtime", "Authentication", "Deployment", "Language"}
	labels := make([]string, len(tabs))
	selected := 0
	for i, tab := range tabs {
		labels[i] = localization.UI(tab)
		if s.tab == tab {
			selected = i
		}
	}
	if ui.Tabs(c.Key("settings-tabs"), &selected, labels...).Label(localization.Text("settingsSections", nil)).Changed() {
		s.tab = tabs[selected]
		p.err = nil
		p.message = ""
		p.confirm = false
		p.confirmed = nil
	}
	switch s.tab {
	case "Runtime":
		p.renderRuntime(c)
	case "Authentication":
		p.renderAuthentication(c)
	case "Deployment":
		p.renderDeployment(c)
	case "Language":
		p.renderLanguage(c)
	default:
		p.renderAccount(c)
	}
}

func (p *Controller) renderLanguage(c *ui.Context) {
	s := &p.settings
	p.section(c, "Display language", func() {
		p.note(c, "OpenMPA uses one language across native UI, system menus and content pages.")
		for _, option := range []struct{ mode, label string }{{localization.System, "Follow system"}, {localization.Chinese, "中文"}, {localization.English, "English"}} {
			option := option
			label := option.label
			if s.language == option.mode {
				label = "✓ " + label
			}
			p.button(c, label, func() {
				if p.services.Localization == nil {
					p.err = dto.Failure("displayLanguageUnavailable", "desktop", "never", 0)
					return
				}
				p.async(func(context.Context) (func(), *dto.SafeError) {
					result := p.services.Localization.Set(localization.LanguageRequest{Mode: option.mode})
					return func() {
						if result.OK && result.Value != nil {
							s.language = result.Value.Mode
							p.message = "Display language updated."
						}
					}, result.Error
				})
			})
		}
	})
}
