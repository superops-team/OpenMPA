package nativeui

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/host"
	"github.com/superops-team/OpenMPA/desktop/internal/localization"
)

func TestRegionCatalogIsAClosedDropdownAndRegionChangeInvalidatesRuntime(t *testing.T) {
	p := testController(t)
	options := p.regionOptions()
	if len(options) != 2 || options[0] != "cn-beijing" || options[1] != "cn-shanghai" {
		t.Fatalf("unexpected Region dropdown: %v", options)
	}
	p.settings.directoryRegion = "cn-beijing"
	p.settings.directoryProject = "default"
	p.settings.directoryLoaded = true
	p.settings.selected = "r-old"
	p.settings.preferredRuntime = "r-old"
	p.settings.runtimes = []dto.RuntimeSummary{{RuntimeID: "r-old", Region: "cn-beijing", ProjectName: "default"}}
	if _, err := p.services.Connection.Client.Bind(connection.Binding{Endpoint: "https://runtime.example.com", RuntimeID: "r-old", Scope: "scope", RuntimeKey: "secret"}, false); err != nil {
		t.Fatal(err)
	}
	p.services.Config = nil // Persistence has configstore coverage; keep this UI transition synchronous.
	p.changeRegion("cn-shanghai")
	if p.services.Connection.Client.State().Connected || p.settings.selected != "" || p.settings.preferredRuntime != "" || p.settings.directoryLoaded || len(p.settings.runtimes) != 0 {
		t.Fatalf("old Runtime scope survived Region change: %+v", p.settings)
	}
	p.changeRegion("cn-guangzhou")
	if p.settings.region != "cn-beijing" || p.err == nil || p.err.Code != "invalidInput" {
		t.Fatalf("unsupported Region was accepted: %+v %+v", p.settings, p.err)
	}
}

func TestSelfInitiatedRegionResetDoesNotInvalidateItsOwnPersistenceWork(t *testing.T) {
	p := testController(t)
	p.services.Config = nil // Observe only the controller generation transition.
	p.observed = true
	p.shell = host.ShellState{View: "settings"}
	p.connectionEpoch = p.epoch()
	p.platformEpoch = p.services.Platform.Status().Value.Epoch
	generation := p.generation

	p.changeRegion("cn-shanghai")
	p.observe(host.ShellState{View: "settings"})

	if p.generation != generation {
		t.Fatal("self-initiated Runtime reset was mistaken for an external epoch change")
	}
}

func TestRuntimeDirectorySelectionRequiresLoadedScopeAndMembership(t *testing.T) {
	s := settingsState{region: "cn-beijing", project: "default", directoryRegion: "cn-beijing", directoryProject: "default", directoryLoaded: true, selected: "r-one", runtimes: []dto.RuntimeSummary{{RuntimeID: "r-one", Region: "cn-beijing", ProjectName: "default", Name: "Assistant"}}}
	if _, ok := s.selectedRuntime(); !ok {
		t.Fatal("loaded selection rejected")
	}
	s.region = "cn-shanghai"
	if _, ok := s.selectedRuntime(); ok {
		t.Fatal("selection leaked across regions")
	}
	s.region = "cn-beijing"
	s.selected = "r-unknown"
	if _, ok := s.selectedRuntime(); ok {
		t.Fatal("unlisted runtime selected")
	}
}

func TestRuntimeSelectionSurvivesConnectedEpochInvalidation(t *testing.T) {
	p := testController(t)
	p.settings.tab = "Runtime"
	p.settings.region = "cn-beijing"
	p.settings.project = "default"
	p.settings.directoryRegion = "cn-beijing"
	p.settings.directoryProject = "default"
	p.settings.directoryLoaded = true
	p.settings.selected = "r-one"
	p.settings.preferredRuntime = "r-one"
	p.settings.runtimes = []dto.RuntimeSummary{{RuntimeID: "r-one", Region: "cn-beijing", ProjectName: "default", Name: "Primary MPA"}}
	if _, err := p.services.Connection.Client.Bind(connection.Binding{Endpoint: "https://runtime.example.com", RuntimeID: "r-one", Scope: "scope", RuntimeKey: "fixture-key"}, false); err != nil {
		t.Fatal(err)
	}

	p.invalidate()

	if p.settings.selected != "r-one" || p.settings.preferredRuntime != "r-one" || !p.settings.directoryLoaded || len(p.settings.runtimes) != 1 {
		t.Fatalf("connected Runtime display state was lost after epoch invalidation: %+v", p.settings)
	}
}

func TestRuntimeRestoreRequiresMatchingPrincipalAndCompleteDirectory(t *testing.T) {
	s := settingsState{
		region:                  "cn-beijing",
		project:                 "default",
		preferredRuntime:        "r-one",
		preferredPrincipalScope: "account-one/User/principal-one",
		directoryRegion:         "cn-beijing",
		directoryProject:        "default",
		directoryLoaded:         true,
		runtimes:                []dto.RuntimeSummary{{RuntimeID: "r-one", Region: "cn-beijing", ProjectName: "default", Name: "Primary MPA"}},
	}
	platform := dto.PlatformState{SignedIn: true, Region: "cn-beijing", Principal: &dto.Principal{Scope: "account-one/User/principal-one"}}
	if runtime, ok := s.restorableRuntime(platform); !ok || runtime.RuntimeID != "r-one" {
		t.Fatalf("matching saved Runtime was not restorable: %+v %v", runtime, ok)
	}
	s.preferredPrincipalScope = ""
	if runtime, ok := s.restorableRuntime(platform); !ok || runtime.RuntimeID != "r-one" {
		t.Fatalf("legacy Runtime visible to the current account was not adopted: %+v %v", runtime, ok)
	}
	s.preferredPrincipalScope = "account-one/User/principal-one"
	platform.Principal.Scope = "account-two/User/principal-two"
	if _, ok := s.restorableRuntime(platform); ok {
		t.Fatal("saved Runtime crossed cloud principal scopes")
	}
	platform.Principal.Scope = "account-one/User/principal-one"
	s.directoryLoaded = false
	if _, ok := s.restorableRuntime(platform); ok {
		t.Fatal("incomplete Runtime directory authorized automatic restore")
	}
	s.directoryLoaded = true
	platform.Region = "cn-shanghai"
	if _, ok := s.restorableRuntime(platform); ok {
		t.Fatal("vault login Region mismatch authorized automatic restore")
	}
}

func TestRuntimePageUsesAutomaticDirectoryAndImmediateSelectionModel(t *testing.T) {
	localization.SetCurrentLocale(localization.English)
	p := testController(t)
	p.settings.tab = "Runtime"
	p.settings.region = "cn-beijing"
	p.settings.project = "default"
	p.settings.directoryRegion = "cn-beijing"
	p.settings.directoryProject = "default"
	p.settings.directoryLoaded = true
	p.settings.preferredRuntime = "r-legacy"
	tt := ui.NewTester(func(c *ui.Context) { p.Render(c, host.ShellState{View: "settings"}) }, 1200, 760)
	if _, ok := tt.Find("Find by name or ID"); !ok {
		t.Fatal("Runtime switch combobox disappeared for an empty directory")
	}
	if _, ok := tt.Find("Refresh MPA Runtimes"); ok {
		t.Fatal("manual refresh remained in the automatic directory model")
	}
	if _, ok := tt.Find("Connect selected Runtime"); ok {
		t.Fatal("separate connect action remained in the immediate selection model")
	}
	if _, ok := tt.Find("r-legacy"); ok {
		t.Fatal("unscoped legacy Runtime leaked into an account with an empty directory")
	}
	if _, ok := tt.Find("No Runtime selected for account current account."); !ok {
		t.Fatalf("missing account-scoped Runtime selection state: %q", tt.Texts())
	}
}

func TestRuntimePageHidesForeignPreferenceAndKeepsAccountSelectionEntry(t *testing.T) {
	localization.SetCurrentLocale(localization.English)
	p, _ := vaultTestController(t, true)
	p.settings.tab = "Runtime"
	p.settings.region = "cn-beijing"
	p.settings.project = "default"
	p.settings.directoryRegion = "cn-beijing"
	p.settings.directoryProject = "default"
	p.settings.directoryLoaded = true
	p.settings.preferredRuntime = "r-foreign"
	p.settings.preferredPrincipalScope = "another-account/User/another-principal"
	p.settings.runtimes = []dto.RuntimeSummary{{RuntimeID: "r-current", Region: "cn-beijing", ProjectName: "default", Name: "Current account MPA"}}
	tt := ui.NewTester(func(c *ui.Context) { p.Render(c, host.ShellState{View: "settings"}) }, 1200, 760)
	if _, ok := tt.Find("r-foreign"); ok {
		t.Fatal("foreign account Runtime was projected into the current account")
	}
	if _, ok := tt.Find("No Runtime selected for account account-one."); !ok {
		t.Fatalf("missing account-scoped empty selection state: %q", tt.Texts())
	}
	if _, ok := tt.Find("Select Runtime"); !ok {
		t.Fatal("current account has no visible Runtime selection entry")
	}
	if err := tt.Click("Find by name or ID"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("Current account MPA · r-current"); !ok {
		t.Fatal("current account Runtime was not available in the selector")
	}
}

func TestRuntimeRegionAndProjectControlsAlign(t *testing.T) {
	localization.SetCurrentLocale(localization.English)
	p := testController(t)
	p.settings.tab = "Runtime"
	p.settings.directoryRegion = "cn-beijing"
	p.settings.directoryProject = "default"
	p.settings.directoryLoaded = true
	tt := ui.NewTester(func(c *ui.Context) { p.Render(c, host.ShellState{View: "settings"}) }, 1200, 760)
	region, regionOK := tt.Find("Region")
	project, projectOK := tt.Find("Project")
	if !regionOK || !projectOK || region.Y != project.Y || region.H != project.H {
		t.Fatalf("Region and Project controls are not aligned: region=%+v project=%+v", region, project)
	}
}

func TestProjectEditImmediatelyInvalidatesRuntimeAcrossTabs(t *testing.T) {
	p := testController(t)
	ctx := context.Background()
	if err := p.services.Config.SetRuntime(ctx, "account-one/User/principal-one", "cn-beijing", "default", "r-one"); err != nil {
		t.Fatal(err)
	}
	p.settings.region = "cn-beijing"
	p.settings.project = "project-two"
	p.settings.directoryRegion = "cn-beijing"
	p.settings.directoryProject = "default"
	p.settings.directoryLoaded = true
	p.settings.selected = "r-one"
	p.settings.preferredRuntime = "r-one"
	p.settings.preferredPrincipalScope = "account-one/User/principal-one"
	p.settings.runtimes = []dto.RuntimeSummary{{RuntimeID: "r-one", Region: "cn-beijing", ProjectName: "default"}}
	if _, err := p.services.Connection.Client.Bind(connection.Binding{Endpoint: "https://runtime.example.com", RuntimeID: "r-one", Scope: "scope", RuntimeKey: "fixture-key"}, false); err != nil {
		t.Fatal(err)
	}

	p.changeProject()

	if p.services.Connection.Client.State().Connected || p.settings.selected != "" || p.settings.preferredRuntime != "" || p.settings.directoryLoaded || len(p.settings.runtimes) != 0 {
		t.Fatalf("Project edit retained stale Runtime state: %+v", p.settings)
	}
	snapshot, err := p.services.Config.Snapshot(ctx)
	if err != nil || snapshot.Runtime != "" || snapshot.PrincipalScope != "" || snapshot.Project != "default" {
		t.Fatalf("Project edit did not safely clear the saved binding: %+v %v", snapshot, err)
	}
}

func TestSettingsTabsExposeKeyboardSelection(t *testing.T) {
	localization.SetCurrentLocale(localization.English)
	p := testController(t)
	tt := ui.NewTester(func(c *ui.Context) { p.Render(c, host.ShellState{View: "settings"}) }, 760, 700)
	if err := tt.Click("Account"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyRight)
	if p.settings.tab != "Runtime" {
		t.Fatalf("settings navigation is not an arrow-key selectable tab list: %q", p.settings.tab)
	}
}

func TestRuntimeComboboxFiltersAndChoosesByID(t *testing.T) {
	localization.SetCurrentLocale(localization.English)
	p := testController(t)
	p.settings.tab = "Runtime"
	p.settings.region = "cn-beijing"
	p.settings.project = "default"
	p.settings.directoryRegion = "cn-beijing"
	p.settings.directoryProject = "default"
	p.settings.directoryLoaded = true
	p.settings.runtimes = []dto.RuntimeSummary{
		{RuntimeID: "r-one", Region: "cn-beijing", ProjectName: "default", Name: "Primary MPA", Status: "Ready"},
		{RuntimeID: "r-two", Region: "cn-beijing", ProjectName: "default", Name: "Secondary MPA", Status: "Ready"},
	}
	if _, err := p.services.Connection.Client.Bind(connection.Binding{Endpoint: "https://runtime.example.com", RuntimeID: "r-two", Scope: "scope", RuntimeKey: "fixture-key"}, false); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) { p.Render(c, host.ShellState{View: "settings"}) }, 760, 700)
	if err := tt.Click("Find by name or ID"); err != nil {
		t.Fatal(err)
	}
	tt.Type("r-two")
	tt.Key(0, ui.KeyEnter)
	if p.settings.selected != "r-two" {
		t.Fatalf("Runtime combobox chose %q, want r-two", p.settings.selected)
	}
	if _, ok := tt.Find("Primary MPA · r-one"); ok {
		t.Fatal("name/ID filtering left an unrelated Runtime option visible")
	}
}

func TestFailedRuntimeBindAllowsSameChoiceRetry(t *testing.T) {
	p := testController(t)
	runtime := dto.RuntimeSummary{RuntimeID: "r-one", Region: "cn-beijing", ProjectName: "default", Name: "Primary MPA"}
	p.settings.region = runtime.Region
	p.settings.project = runtime.ProjectName
	p.settings.directoryRegion = runtime.Region
	p.settings.directoryProject = runtime.ProjectName
	p.settings.directoryLoaded = true
	p.settings.runtimes = []dto.RuntimeSummary{runtime}
	p.settings.selected = runtime.RuntimeID
	p.settings.runtimeChoice = runtimeChoiceLabel(runtime)
	generation := p.settings.directoryGeneration

	p.finishRuntimeBinding(runtime, false, nil, false)

	if p.settings.runtimeChoice != "" || p.settings.selected != "" || p.settings.directoryGeneration == generation {
		t.Fatalf("failed Bind left the same Runtime unselectable: %+v", p.settings)
	}
	if selected, ok := p.settings.chooseRuntimeLabel(runtimeChoiceLabel(runtime)); !ok || selected.RuntimeID != runtime.RuntimeID {
		t.Fatal("same Runtime could not be selected again after Bind failure")
	}
}
