package nativeui

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/host"
	"github.com/superops-team/OpenMPA/desktop/internal/ipc"
	"testing"
)

func TestRuntimeDirectorySelectionRequiresLoadedScopeAndMembership(t *testing.T) {
	s := settingsState{region: "cn-beijing", project: "default", directoryRegion: "cn-beijing", directoryProject: "default", selected: "r-one", runtimes: []dto.RuntimeSummary{{RuntimeID: "r-one", Region: "cn-beijing", ProjectName: "default", Name: "Assistant"}}}
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
	s.selected = "r-one"
	s.search = "another"
	if _, ok := s.selectedRuntime(); ok {
		t.Fatal("filtered runtime selected")
	}
}

func TestRuntimeNumericFieldsDoNotSilentlyBecomeZero(t *testing.T) {
	s := settingsState{cpu: "1000", memory: "2048", min: "0", max: "1", concurrency: "20"}
	v, e := s.configValues()
	if e != nil || v.CPUMilli != 1000 || v.MinInstance != 0 {
		t.Fatal(v, e)
	}
	s.cpu = "not-a-number"
	if _, e = s.configValues(); e == nil {
		t.Fatal("invalid cpu accepted")
	}
	s.cpu = "0"
	if _, e = s.configValues(); e == nil {
		t.Fatal("invalid cpu range accepted")
	}
}

func TestScopeInvalidationDropsSecretsReviewsAndConfirmations(t *testing.T) {
	p := &Controller{settings: settingsState{ak: "secret", sk: "secret", jwt: "secret", channelKey: "secret", selected: "r-old"}, busy: true, confirm: true, confirmed: func() { t.Fatal("stale confirm retained") }}
	p.invalidate()
	if p.busy || p.confirm || p.confirmed != nil || p.settings.ak != "" || p.settings.sk != "" || p.settings.jwt != "" || p.settings.channelKey != "" || p.settings.selected != "" {
		t.Fatal("old scope survived")
	}
}

func testController() *Controller {
	conn := connection.New()
	platform := ipc.NewPlatform(cloud.New(), conn)
	return &Controller{services: Services{Platform: platform, Connection: &ipc.Connection{Client: conn}}, settings: settingsState{tab: "Account", region: "cn-beijing", project: "default"}}
}
func TestNativeSettingsLayoutAndKeyboardInput(t *testing.T) {
	p := testController()
	state := host.ShellState{View: "settings"}
	tt := ui.NewTester(func(c *ui.Context) { p.Render(c, state) }, 760, 700)
	if r, ok := tt.Find("Native settings"); !ok || r.Y != 0 || r.W != 760 {
		t.Fatal(r, ok)
	}
	if err := tt.Click("Access key"); err != nil {
		t.Fatal(err)
	}
	tt.Type("private-test-value")
	if p.settings.ak != "private-test-value" {
		t.Fatal("native input did not update")
	}
	if err := tt.Click("Runtime"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("Region"); !ok {
		t.Fatal("runtime form missing")
	}
	if _, ok := tt.Find("Access key"); ok {
		t.Fatal("account inputs visible on Runtime tab")
	}
	state.View = "chat"
	tt.Frame()
	if p.settings.ak != "" {
		t.Fatal("secret survived leaving settings")
	}
}
func TestNativeCompletionRejectsLatePageAndEpochResults(t *testing.T) {
	p := testController()
	applied := false
	p.generation = 2
	p.complete(1, p.epoch(), p.services.Platform.Status().Value.Epoch, false, func() { applied = true }, nil)
	if applied {
		t.Fatal("old page completion applied")
	}
	epoch := p.epoch()
	p.services.Connection.Client.Disconnect()
	p.complete(2, epoch, p.services.Platform.Status().Value.Epoch, false, func() { applied = true }, nil)
	if applied {
		t.Fatal("old connection completion applied")
	}
	p.closed = true
	p.complete(p.generation, p.epoch(), p.services.Platform.Status().Value.Epoch, false, func() { applied = true }, nil)
	if applied {
		t.Fatal("closed window completion applied")
	}
}

func TestNativeCatalogueRenderingIsBounded(t *testing.T) {
	p := testController()
	draws := 0
	tt := ui.NewTester(func(c *ui.Context) {
		p.rows(c, "large-directory", 1000, 220, func(i int) { draws++; ui.Text(c, "row").Height(36) })
	}, 760, 700)
	if tt.FramePending() {
		t.Fatal("native list did not settle")
	}
	if draws > 100 {
		t.Fatalf("rendered %d rows for a 220px viewport", draws)
	}
}

func TestNativeSessionMembershipRejectsStaleRendererScope(t *testing.T) {
	p := testController()
	p.shell.Sessions = []host.NavigationItem{{ID: "session", Title: "Title"}}
	p.knowledge.sessionID = "session"
	old := p.epoch()
	p.services.Connection.Client.Disconnect()
	p.shell.ConnectionEpoch = old
	if p.knowledgeMember() {
		t.Fatal("stale projected session accepted under new connection")
	}
	p.shell.ConnectionEpoch = p.epoch()
	if !p.knowledgeMember() {
		t.Fatal("current projected session rejected")
	}
}

func BenchmarkNativeThousandRowCatalogue(b *testing.B) {
	p := testController()
	label := "Assistant"
	tt := ui.NewTester(func(c *ui.Context) {
		p.rows(c, "large-directory", 1000, 220, func(i int) { ui.Text(c, label).Height(36) })
	}, 760, 700)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		label = fmt.Sprintf("Assistant frame %d", i)
		tt.Frame()
	}
}
