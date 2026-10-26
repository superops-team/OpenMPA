package nativeui

import (
	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/host"
	"testing"
)

func TestNativeCreationDialogIsReversibleAndDoesNotCreateOnOpen(t *testing.T) {
	p := testController(t)
	p.prompt = promptState{open: true, kind: "createSpace"}
	tt := ui.NewTester(func(c *ui.Context) { p.RenderOverlay(c, p.shell) }, 760, 700)
	if _, ok := tt.Find("Space name"); !ok {
		t.Fatal("native space input missing")
	}
	if err := tt.Click("Space name"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Draft Space")
	if p.prompt.name != "Draft Space" {
		t.Fatal("native input did not change")
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if p.prompt.open {
		t.Fatal("cancel did not close dialog")
	}
}
func TestAssistantCreationIntentKeepsOriginalIDAfterRestart(t *testing.T) {
	s := taskIntents{}
	intent := assistantIntent{SessionID: "original-id", Name: "Original assistant"}
	if err := s.saveAssistant("scope/native-assistant", &intent); err != nil {
		t.Fatal(err)
	}
	got, err := s.load("scope/native-assistant")
	if err != nil || got.Assistant == nil || *got.Assistant != intent {
		t.Fatal(got, err)
	}
	other := assistantIntent{SessionID: "different-id", Name: "Original assistant"}
	if s.saveAssistant("scope/native-assistant", &other) == nil {
		t.Fatal("pending assistant replaced")
	}
}

func TestAssistantCreationAllowsRuntimeKeyIdentity(t *testing.T) {
	tests := []struct {
		name  string
		state dto.ConnectionState
		code  string
	}{
		{name: "agentkit runtime key", state: dto.ConnectionState{Connected: true, HasCredential: true}},
		{name: "explicit business identity", state: dto.ConnectionState{Connected: true, HasCredential: true, HasBusinessIdentity: true}},
		{name: "disconnected", state: dto.ConnectionState{}, code: "notConnected"},
		{name: "credential missing", state: dto.ConnectionState{Connected: true}, code: "authRequired"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := assistantCreationPrecondition(test.state)
			if test.code == "" && err != nil {
				t.Fatalf("valid Runtime identity rejected: %+v", err)
			}
			if test.code != "" && (err == nil || err.Code != test.code) {
				t.Fatalf("got %+v, want %s", err, test.code)
			}
		})
	}
}

func TestDisconnectedNewAssistantRoutesToRuntimeSettingsWithoutDialog(t *testing.T) {
	p := testController(t)
	p.settings.tab = "Account"
	if handled := p.HandleAction("newConversation", ""); handled {
		t.Fatal("native handler swallowed the navigation to Runtime settings")
	}
	if p.prompt.open {
		t.Fatal("disconnected new assistant opened an unusable creation dialog")
	}
	if p.settings.tab != "Runtime" {
		t.Fatalf("new assistant routed to %q, want Runtime settings", p.settings.tab)
	}
}

func TestNativeSessionDialogRejectsOldDirectory(t *testing.T) {
	p := testController(t)
	p.shell.SelectedSession = "session"
	p.shell.ConnectionEpoch = p.epoch() + 1
	p.shell.Sessions = []host.NavigationItem{{ID: "session", Title: "Title"}}
	if !p.sessionAction("renameSession") || p.prompt.open || p.err == nil {
		t.Fatal("stale directory opened rename dialog")
	}
}

func TestNativeRenameDialogShowsCurrentTitle(t *testing.T) {
	p := testController(t)
	p.shell = host.ShellState{ConnectionEpoch: p.epoch(), SelectedSession: "session", Sessions: []host.NavigationItem{{ID: "session", Title: "Current title"}}}
	if !p.sessionAction("renameSession") {
		t.Fatal("rename not handled")
	}
	tt := ui.NewTester(func(c *ui.Context) { p.RenderOverlay(c, p.shell) }, 760, 700)
	if _, ok := tt.Find("Conversation title"); !ok {
		t.Fatal("native rename field missing")
	}
	if p.prompt.name != "Current title" || p.prompt.sessionID != "session" {
		t.Fatal("rename id or title lost")
	}
}

func TestNativeActionFailureRemainsVisibleOverChat(t *testing.T) {
	p := testController(t)
	p.err = dto.Failure("staleConnection", "desktop", "reload", 0)
	tt := ui.NewTester(func(c *ui.Context) { p.RenderOverlay(c, host.ShellState{View: "chat"}) }, 760, 700)
	if _, ok := tt.Find("Native operation status"); !ok {
		t.Fatal("native action error hidden behind WebView")
	}
	if err := tt.Click("Dismiss"); err != nil {
		t.Fatal(err)
	}
	if p.err != nil {
		t.Fatal("error did not dismiss")
	}
}
