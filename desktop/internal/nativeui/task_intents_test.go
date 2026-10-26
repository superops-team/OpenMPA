package nativeui

import (
	"github.com/superops-team/OpenMPA/desktop/internal/knowledge"
	"github.com/superops-team/OpenMPA/desktop/internal/schedule"
	"os"
	"path/filepath"
	"testing"
)

func TestTaskIntentRestartScopeAndOriginalToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "intents.json")
	resolver := func() (string, error) { return path, nil }
	s := taskIntents{path: resolver}
	request := schedule.CreateRequest{ClientToken: "original-token", Name: "Original form", Prompt: "body"}
	if e := s.change("account/runtime/business", "create", &request, nil); e != nil {
		t.Fatal(e)
	}
	restarted := taskIntents{path: resolver}
	out, e := restarted.load("account/runtime/business")
	if e != nil || out.Create == nil || out.Create.ClientToken != "original-token" {
		t.Fatal(out, e)
	}
	isolated, e := restarted.load("other/runtime/business")
	if e != nil || isolated.Create != nil {
		t.Fatal("scope leak")
	}
	request.ClientToken = "new-token"
	if e = s.change("account/runtime/business", "create", &request, nil); e == nil {
		t.Fatal("original token replaced")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("intent permissions")
	}
	if e = s.change("account/runtime/business", "create", nil, nil); e != nil {
		t.Fatal(e)
	}
}
func TestCorruptTaskIntentStoreFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "intents.json")
	_ = os.WriteFile(path, []byte("broken"), 0600)
	s := taskIntents{path: func() (string, error) { return path, nil }}
	if e := s.change("scope", "create", &schedule.CreateRequest{ClientToken: "new"}, nil); e == nil {
		t.Fatal("corrupt pending store overwritten")
	}
}

func TestKnowledgeIntentRestartEpochAndContentIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "intents.json")
	resolver := func() (string, error) { return path, nil }
	s := taskIntents{path: resolver}
	input := importIntent{Text: &knowledge.TextImportRequest{ConnectionEpoch: 2, SessionID: "session", IdempotencyKey: "original", DisplayName: "name", Content: "exact content"}}
	if e := s.saveImport("account/runtime/session", &input); e != nil {
		t.Fatal(e)
	}
	restarted := taskIntents{path: resolver}
	v, e := restarted.load("account/runtime/session")
	if e != nil || v.Import == nil || v.Import.Text.Content != "exact content" {
		t.Fatal(v, e)
	}
	input.Text.ConnectionEpoch = 5
	if e := restarted.saveImport("account/runtime/session", &input); e != nil {
		t.Fatal("epoch change rejected exact recovery", e)
	}
	input.Text.Content = "changed"
	if e := restarted.saveImport("account/runtime/session", &input); e == nil {
		t.Fatal("different import overwrote pending request")
	}
	other, _ := restarted.load("account/runtime/other-session")
	if other.Import != nil {
		t.Fatal("session isolation lost")
	}
}
func TestTaskIntentSameTokenCannotChangeOriginalForm(t *testing.T) {
	s := taskIntents{}
	r := schedule.CreateRequest{ClientToken: "same", Name: "original", Prompt: "exact"}
	if e := s.change("scope", "create", &r, nil); e != nil {
		t.Fatal(e)
	}
	changed := r
	changed.Prompt = "different"
	if e := s.change("scope", "create", &changed, nil); e == nil {
		t.Fatal("same token changed request")
	}
}
