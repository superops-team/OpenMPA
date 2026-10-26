package host

import (
	"github.com/egoist/mygo/ui"
	"math"
	"strings"
	"testing"
)

func TestTitlebarDividerIsCentered(t *testing.T) {
	for _, width := range []int{640, 760, 1200} {
		for _, collapsed := range []bool{false, true} {
			rail := &nativeRail{state: ShellState{Collapsed: collapsed, Location: strings.Repeat("Runtime", 60), PageTitle: strings.Repeat("中文标题", 30)}}
			tt := ui.NewTester(rail.view, width, 700)
			tt.SetTitleBar(ui.TitleBar{Height: 44, Left: 72})
			tt.Frame()
			bar, _ := tt.Find("OpenMPA title bar")
			divider, found := tt.Find("Title bar separator")
			toggle, _ := tt.Find("Toggle navigation")
			location, _ := tt.Find("Current location")
			if !found || divider.W != 1 || divider.H != 16 || math.Abs(float64(divider.Y+divider.H/2-(bar.Y+bar.H/2))) > 0.5 || math.Abs(float64(divider.Y+divider.H/2-(toggle.Y+toggle.H/2))) > 0.5 {
				t.Fatalf("width %d collapsed %v: divider %v bar %v toggle %v", width, collapsed, divider, bar, toggle)
			}
			if divider.X <= toggle.X+toggle.W || divider.X+divider.W >= location.X {
				t.Fatalf("divider overlaps adjacent controls: %v %v %v", toggle, divider, location)
			}
		}
	}
}

func TestNativeShellLayoutAndState(t *testing.T) {
	rail := &nativeRail{state: ShellState{View: "chat", Location: strings.Repeat("Runtime", 60), PageTitle: strings.Repeat("中文标题", 30), Sessions: []NavigationItem{{ID: "s", Title: "My assistant"}}, Spaces: []NavigationItem{{ID: "space", Title: "Personal"}}}}
	var receivedAction string
	rail.request = func(a, id string) { receivedAction = a }
	tt := ui.NewTester(rail.view, 760, 700)
	if err := tt.Click("Toggle sidebar"); err != nil {
		t.Fatal(err)
	}
	if receivedAction != "toggleSidebar" {
		t.Fatalf("full shell action: %q", receivedAction)
	}
	bar, found := tt.Find("OpenMPA title bar")
	if !found || bar.Y != 0 || bar.H != 44 {
		t.Fatalf("top bar %v %v", bar, found)
	}
	sidebar, found := tt.Find("Workspace navigation")
	if !found || sidebar.Y != 44 || sidebar.X != 48 || sidebar.W != 240 {
		t.Fatalf("sidebar %v %v", sidebar, found)
	}
	toggle, _ := tt.Find("Toggle navigation")
	location, _ := tt.Find("Current location")
	action, _ := tt.Find("Show capabilities and artifacts")
	if toggle.Y+toggle.H/2 != location.Y+location.H/2 || action.Y+action.H/2 != toggle.Y+toggle.H/2 || location.X < toggle.X+toggle.W || location.X+location.W > action.X {
		t.Fatalf("top controls overlap or misalign: %v %v %v", toggle, location, action)
	}
	rail.state.Collapsed = true
	tt.Frame()
	if _, found := tt.Find("Workspace navigation"); found {
		t.Fatal("sidebar did not collapse")
	}
	if _, found := tt.Find("Open Spaces"); !found {
		t.Fatal("native rail disappeared")
	}
	if validShellState(ShellState{View: "chat", Sessions: []NavigationItem{{ID: "s"}, {ID: "s"}}}) {
		t.Fatal("duplicate ids accepted")
	}
}

func TestNativeRailActions(t *testing.T) {
	var actions []string
	rail := &nativeRail{request: func(action, id string) { actions = append(actions, action) }}
	tt := ui.NewTester(rail.controls, 48, 600)
	tt.Frame()
	tt.Frame()
	tt.Frame()
	for _, item := range []struct{ label, action string }{
		{"OpenMPA home", "home"}, {"Toggle sidebar", "toggleSidebar"}, {"New conversation", "newConversation"},
		{"Open Spaces", "pages"}, {"Open scheduled activity", "tasks"}, {"Open settings", "settings"},
	} {
		if err := tt.Click(item.label); err != nil {
			t.Fatalf("missing native button %s", item.label)
		}
		if len(actions) == 0 || actions[len(actions)-1] != item.action {
			t.Fatalf("action mismatch for %s: %v", item.label, actions)
		}
	}
	if len(actions) != 6 {
		t.Fatalf("duplicate actions %v", actions)
	}
	if New().SetShellState(ShellState{View: "arbitrary"}).OK {
		t.Fatal("unknown navigation accepted")
	}
}

func TestNativeSidebarSelection(t *testing.T) {
	var action, id string
	rail := &nativeRail{state: ShellState{View: "chat", Sessions: []NavigationItem{{ID: "session-id", Title: "My assistant"}}, Spaces: []NavigationItem{{ID: "space-id", Title: "Personal"}}}, request: func(a, i string) { action, id = a, i }}
	tt := ui.NewTester(rail.sidebar, 240, 700)
	if err := tt.Click("My assistant"); err != nil {
		t.Fatal(err)
	}
	if action != "selectSession" || id != "session-id" {
		t.Fatalf("session action %s %s", action, id)
	}
	if err := tt.Click("Personal"); err != nil {
		t.Fatal(err)
	}
	if action != "selectSpace" || id != "space-id" {
		t.Fatalf("space action %s %s", action, id)
	}
	if err := tt.Click("Create Space"); err != nil {
		t.Fatal(err)
	}
	if action != "createSpace" {
		t.Fatal("create space action missing")
	}
}

func TestNativeSidebarSearchKeepsExactSessionIdentity(t *testing.T) {
	var selected string
	rail := &nativeRail{state: ShellState{View: "chat", Sessions: []NavigationItem{{ID: "session-a", Title: "Same title"}, {ID: "session-b", Title: "Same title"}}}, request: func(a, id string) {
		if a == "selectSession" {
			selected = id
		}
	}}
	tt := ui.NewTester(rail.sidebar, 240, 700)
	if err := tt.Click("Find conversations"); err != nil {
		t.Fatal(err)
	}
	tt.Type("session-b")
	if err := tt.Click("Same title"); err != nil {
		t.Fatal(err)
	}
	if selected != "session-b" {
		t.Fatalf("selected %s", selected)
	}
}
