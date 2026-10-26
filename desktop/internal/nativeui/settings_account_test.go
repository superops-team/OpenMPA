package nativeui

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/configstore"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/host"
	"github.com/superops-team/OpenMPA/desktop/internal/ipc"
	"github.com/superops-team/OpenMPA/desktop/internal/localization"
	"github.com/superops-team/OpenMPA/desktop/internal/vault"
)

type vaultUITransport struct{}

func (vaultUITransport) RoundTrip(*http.Request) (*http.Response, error) {
	body := `{"Result":{"AccountId":"account-one","UserId":"user-one","IdentityType":"User"}}`
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func vaultTestController(t *testing.T, signedIn bool) (*Controller, *vault.Service) {
	t.Helper()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "openmpa.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	service := vault.NewWithIterations(store, vault.MinIterations)
	conn := connection.New()
	platform := ipc.NewPlatformWithVault(cloud.NewWithTransport(vaultUITransport{}), conn, service)
	if signedIn {
		if result := platform.SignIn(context.Background(), dto.SignInRequest{Region: "cn-beijing", AccessKey: "AK_FIXTURE", SecretKey: "SK_FIXTURE"}); !result.OK {
			t.Fatal(result.Error)
		}
	}
	return &Controller{services: Services{Platform: platform, Connection: &ipc.Connection{Client: conn}, Config: store}, settings: settingsState{tab: "Account", region: "cn-beijing", project: "default", language: localization.System}}, service
}

func TestVaultSectionOffersSaveAndUnlock(t *testing.T) {
	localization.SetCurrentLocale(localization.English)
	p, service := vaultTestController(t, true)
	tt := ui.NewTester(func(c *ui.Context) { p.Render(c, host.ShellState{View: "settings"}) }, 760, 900)
	if _, ok := tt.Find("Saved credentials"); !ok {
		t.Fatal("vault section missing")
	}
	if _, ok := tt.Find("Save credentials"); !ok {
		t.Fatal("save action missing while signed in")
	}
	p.settings.vaultPassphrase = "correct horse battery"
	p.settings.vaultConfirm = "different passphrase"
	if err := tt.Click("Save credentials"); err != nil {
		t.Fatal(err)
	}
	if p.err == nil {
		t.Fatal("mismatched passphrase was accepted")
	}

	if err := service.Save(context.Background(), []byte("correct horse battery"), vault.Payload{AccessKey: "AK_FIXTURE", SecretKey: "SK_FIXTURE"}, "account-one", "cn-beijing"); err != nil {
		t.Fatal(err)
	}
	if result := p.services.Platform.SignOut(); !result.OK {
		t.Fatal(result.Error)
	}
	p.settings.vaultPassphrase = ""
	p.settings.vaultConfirm = ""
	p.refreshVault()
	tt = ui.NewTester(func(c *ui.Context) { p.Render(c, host.ShellState{View: "settings"}) }, 760, 900)
	if _, ok := tt.Find("Unlock and sign in"); !ok {
		t.Fatal("unlock action missing with a saved vault")
	}
	if _, ok := tt.Find("Forget saved credentials"); !ok {
		t.Fatal("forget action missing")
	}
	if _, ok := tt.Find("Passphrase"); !ok {
		t.Fatal("passphrase field missing")
	}
}

func TestVaultSectionRequiresSignIn(t *testing.T) {
	localization.SetCurrentLocale(localization.English)
	p, _ := vaultTestController(t, false)
	tt := ui.NewTester(func(c *ui.Context) { p.Render(c, host.ShellState{View: "settings"}) }, 760, 900)
	if _, ok := tt.Find("Sign in to save credentials for next time."); !ok {
		t.Fatal("signed-out vault guidance missing")
	}
	if _, ok := tt.Find("Save credentials"); ok {
		t.Fatal("save action offered without sign-in")
	}
}
