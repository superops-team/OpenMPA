package nativeui

import (
	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/host"
	"github.com/superops-team/OpenMPA/desktop/internal/knowledge"
	"testing"
)

func TestNativeKnowledgeFormKeepsSessionAndVersion(t *testing.T) {
	p := testController()
	_, e := p.services.Connection.Client.Bind(connection.Binding{Endpoint: "https://example.com", RuntimeID: "r", Scope: "scope", RuntimeKey: "key"}, false)
	if e != nil {
		t.Fatal(e)
	}
	p.shell = host.ShellState{ConnectionEpoch: p.epoch(), Sessions: []host.NavigationItem{{ID: "s", Title: "Assistant"}}}
	p.knowledge = knowledgeState{sessionID: "s", selected: &knowledge.Item{KnowledgeID: "k", Version: 3, DisplayName: "Document"}}
	tt := ui.NewTester(p.renderKnowledge, 760, 1000)
	if _, ok := tt.Find("Knowledge Session"); !ok {
		t.Fatal("session selector missing")
	}
	if err := tt.Click("Knowledge name"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Version draft")
	if p.knowledge.name != "Version draft" || p.knowledge.selected.Version != 3 || p.knowledge.sessionID != "s" {
		t.Fatal("knowledge draft lost identity")
	}
}
