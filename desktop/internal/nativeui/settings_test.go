package nativeui

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/configstore"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/host"
	"github.com/superops-team/OpenMPA/desktop/internal/ipc"
	"github.com/superops-team/OpenMPA/desktop/internal/localization"
)

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
	p := &Controller{settings: settingsState{ak: "secret", sk: "secret", jwt: "secret", channelKey: "secret", selected: "r-old", vaultPassphrase: "secret", vaultConfirm: "secret", showVault: true}, busy: true, confirm: true, confirmed: func() { t.Fatal("stale confirm retained") }}
	p.invalidate()
	if p.busy || p.confirm || p.confirmed != nil || p.settings.ak != "" || p.settings.sk != "" || p.settings.jwt != "" || p.settings.channelKey != "" || p.settings.selected != "" || p.settings.vaultPassphrase != "" || p.settings.vaultConfirm != "" || p.settings.showVault {
		t.Fatal("old scope survived")
	}
}

type testTB interface {
	Helper()
	TempDir() string
	Cleanup(func())
	Fatal(...any)
}

func testController(t testTB) *Controller {
	t.Helper()
	conn := connection.New()
	platform := ipc.NewPlatform(cloud.New(), conn)
	config, err := configstore.Open(filepath.Join(t.TempDir(), "openmpa.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { config.Close() })
	language := localization.New(config)
	return &Controller{services: Services{Platform: platform, Connection: &ipc.Connection{Client: conn}, Localization: language, Config: config}, settings: settingsState{tab: "Account", region: "cn-beijing", project: "default", language: localization.System}}
}
func TestNativeSettingsLayoutAndKeyboardInput(t *testing.T) {
	p := testController(t)
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

func TestFailedIdentityRequestKeepsCredentialDraftForEditing(t *testing.T) {
	p := testController(t)
	p.host = host.New()
	p.settings.ak, p.settings.sk, p.settings.securityToken = "access", "secret", "session"
	p.busy = true
	p.complete(p.generation, p.epoch(), p.services.Platform.Status().Value.Epoch, true, nil, dto.Failure("authRequired", "platform", "reauthenticate", 401))
	if p.busy || p.err == nil || p.settings.ak != "access" || p.settings.sk != "secret" || p.settings.securityToken != "session" {
		t.Fatalf("failed login destroyed editable draft: %+v", p.settings)
	}
}

func TestCredentialVisibilityIsExplicitAndReversible(t *testing.T) {
	p := testController(t)
	tt := ui.NewTester(func(c *ui.Context) { p.Render(c, host.ShellState{View: "settings"}) }, 760, 700)
	if err := tt.Click("Show credentials"); err != nil {
		t.Fatal(err)
	}
	if !p.settings.showCredentials {
		t.Fatal("credential visibility did not turn on")
	}
	if err := tt.Click("Show credentials"); err != nil {
		t.Fatal(err)
	}
	if p.settings.showCredentials {
		t.Fatal("credential visibility did not turn off")
	}
}

func TestNativeSettingsAndPromptFollowChineseLocale(t *testing.T) {
	localization.SetCurrentLocale(localization.Chinese)
	defer localization.SetCurrentLocale(localization.English)
	p := testController(t)
	p.settings.tab = "Language"
	tt := ui.NewTester(func(c *ui.Context) { p.Render(c, host.ShellState{View: "settings"}) }, 760, 700)
	if _, ok := tt.Find("显示语言"); !ok {
		t.Fatal("Chinese language settings missing")
	}
	if _, ok := tt.Find("✓ 跟随系统"); !ok {
		t.Fatal("system language mode not translated")
	}
	p.prompt = promptState{open: true, kind: "newConversation"}
	tt = ui.NewTester(func(c *ui.Context) { p.RenderOverlay(c, p.shell) }, 760, 700)
	if _, ok := tt.Find("助手名称"); !ok {
		t.Fatal("assistant prompt did not follow Chinese locale")
	}
}
func TestNativeCompletionRejectsLatePageAndEpochResults(t *testing.T) {
	p := testController(t)
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
	p := testController(t)
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
	p := testController(t)
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
	p := testController(b)
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
