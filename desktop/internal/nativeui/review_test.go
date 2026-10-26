package nativeui

import (
	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/host"
	"github.com/superops-team/OpenMPA/desktop/internal/schedule"
	"testing"
)

func TestNativeTaskRunOpensReviewForExactVersionAndID(t *testing.T) {
	p := testController(t)
	_, e := p.services.Connection.Client.Bind(connection.Binding{Endpoint: "https://example.com", RuntimeID: "r", Scope: "scope", RuntimeKey: "key"}, false)
	if e != nil {
		t.Fatal(e)
	}
	p.schedules.detail = &schedule.Task{ID: "task-id", Name: "Task", Version: 8, Prompt: "instructions", Enabled: true}
	tt := ui.NewTester(func(c *ui.Context) { p.renderTaskDetail(c); p.RenderOverlay(c, host.ShellState{}) }, 760, 700)
	if err := tt.Click("Run task now"); err != nil {
		t.Fatal(err)
	}
	if !p.confirm || p.confirmed == nil {
		t.Fatal("run did not open review")
	}
	if p.confirmMessage != "Task · task-id" {
		t.Fatal(p.confirmMessage)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if p.confirm || p.confirmed != nil {
		t.Fatal("cancel left run action available")
	}
}
