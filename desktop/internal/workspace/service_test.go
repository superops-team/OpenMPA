package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var ctx = context.Background()

func setup(t *testing.T) (*Service, string, Page) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "workspace.json")
	s := NewWithPath(path)
	t.Cleanup(func() { Shutdown(s) })
	space := s.CreateSpace(ctx, CreateSpaceRequest{Name: "Personal"})
	if !space.OK {
		t.Fatalf("create space: %+v", space.Error)
	}
	page := s.CreatePage(ctx, CreatePageRequest{SpaceID: space.Value.ID, Title: "Page", Content: "initial"})
	if !page.OK {
		t.Fatalf("create page: %+v", page.Error)
	}
	return s, path, *page.Value
}

func TestLazyConstructor(t *testing.T) {
	calls := 0
	s := New(func() (string, error) { calls++; return filepath.Join(t.TempDir(), "w.json"), nil })
	if calls != 0 {
		t.Fatal("constructor accessed user data")
	}
	defer Shutdown(s)
	if !s.Init(ctx, InitRequest{}).OK || calls != 1 {
		t.Fatal("Init did not resolve path exactly once")
	}
}

func TestCASSerializesConcurrentEditors(t *testing.T) {
	s, _, p := setup(t)
	var wg sync.WaitGroup
	responses := make(chan PageResult, 2)
	for _, text := range []string{"editor one", "editor two"} {
		wg.Add(1)
		go func(text string) {
			defer wg.Done()
			responses <- s.PatchPage(ctx, PatchPageRequest{ID: p.ID, ExpectedRevision: 1, Content: &text})
		}(text)
	}
	wg.Wait()
	close(responses)
	wins, conflicts := 0, 0
	for r := range responses {
		if r.OK {
			wins++
		} else if r.Error.Code == "CONFLICT" && r.Error.Status == 409 && r.Error.CurrentPage != nil && r.Error.CurrentPage.Revision == 2 {
			conflicts++
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
	}
}

func TestHierarchyMoveAndDeletePreserveReferences(t *testing.T) {
	s, _, root := setup(t)
	child := s.CreatePage(ctx, CreatePageRequest{SpaceID: root.SpaceID, ParentID: &root.ID, Title: "child"})
	grand := s.CreatePage(ctx, CreatePageRequest{SpaceID: root.SpaceID, ParentID: &child.Value.ID, Title: "grand"})
	if !child.OK || !grand.OK {
		t.Fatal("hierarchy create")
	}
	r := s.PatchPage(ctx, PatchPageRequest{ID: root.ID, ExpectedRevision: 1, ParentID: &grand.Value.ID, SetParent: true})
	if r.OK || r.Error.Code != "VALIDATION" {
		t.Fatal("cycle accepted")
	}
	other := s.CreateSpace(ctx, CreateSpaceRequest{Name: "Other"})
	bot := BotRef{ConnectionScope: "scope-a", SessionID: "session-a"}
	if !s.LinkConversation(ctx, LinkConversationRequest{PageID: root.ID, BotRef: bot}).OK {
		t.Fatal("link")
	}
	moved := s.MovePage(ctx, MovePageRequest{ID: root.ID, ExpectedRevision: 1, SpaceID: other.Value.ID})
	if !moved.OK {
		t.Fatalf("move: %+v", moved.Error)
	}
	descendants := s.ListPages(ctx, ListPagesRequest{SpaceID: other.Value.ID})
	if len(descendants.Value.Pages) != 3 {
		t.Fatal("subtree not moved")
	}
	if !s.DeletePage(ctx, DeletePageRequest{ID: root.ID, ExpectedRevision: 2}).OK {
		t.Fatal("delete")
	}
	remaining := s.GetPage(ctx, GetPageRequest{ID: child.Value.ID})
	if remaining.Value.ParentID != nil {
		t.Fatal("child not reparented")
	}
	links := s.ListConversationLinks(ctx, ListConversationLinksRequest{PageID: root.ID})
	if len(links.Value.Links) != 1 || links.Value.Links[0].DeletedAt == nil || links.Value.Links[0].BotRef != bot {
		t.Fatal("deleted page lost stable session reference")
	}
}

func TestWriteFailureLeavesMemoryAndSnapshotUnchanged(t *testing.T) {
	s, path, p := setup(t)
	before, _ := os.ReadFile(path)
	s.persist = func(string, []byte, []byte) error { return os.ErrPermission }
	changed := "should not save"
	r := s.PatchPage(ctx, PatchPageRequest{ID: p.ID, ExpectedRevision: 1, Content: &changed})
	if r.OK || r.Error.Code != "STORAGE_WRITE_FAILED" {
		t.Fatal("write failure reported saved")
	}
	got := s.GetPage(ctx, GetPageRequest{ID: p.ID})
	after, _ := os.ReadFile(path)
	if got.Value.Content != "initial" || string(before) != string(after) {
		t.Fatal("failed save overwrote authoritative data")
	}
}

func TestDraftRestartAndCorruptRecoveryAreReadOnly(t *testing.T) {
	s, path, p := setup(t)
	if !s.SaveDraft(ctx, SaveDraftRequest{PageID: p.ID, BaseRevision: 1, Title: p.Title, Content: "unsaved", CapturedAt: 1}).OK {
		t.Fatal("draft save")
	}
	Shutdown(s)
	reopened := NewWithPath(path)
	defer Shutdown(reopened)
	drafts := reopened.ListRecoverableDrafts(ctx, ListRecoverableDraftsRequest{})
	if !drafts.OK || len(drafts.Value.Drafts) != 1 || drafts.Value.Drafts[0].Content != "unsaved" {
		t.Fatal("draft not recovered")
	}
	Shutdown(reopened)
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	recover := NewWithPath(path)
	defer Shutdown(recover)
	state := recover.Init(ctx, InitRequest{})
	if !state.OK || !state.Value.ReadOnly || state.Value.RecoveryCode != "CORRUPT_SNAPSHOT" || len(state.Value.Snapshot.Pages) != 1 {
		t.Fatalf("invalid recovery: %+v", state)
	}
	r := recover.CreateSpace(ctx, CreateSpaceRequest{Name: "overwrite"})
	after, _ := os.ReadFile(path)
	if r.OK || string(after) != "broken" {
		t.Fatal("corrupt file replaced")
	}
}

func TestFutureSchemaDoesNotOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	bytes := []byte(`{"schemaVersion":999,"futureSecretShape":"unknown"}`)
	os.WriteFile(path, bytes, 0600)
	s := NewWithPath(path)
	defer Shutdown(s)
	r := s.Init(ctx, InitRequest{})
	if !r.OK || !r.Value.ReadOnly || r.Value.RecoveryCode != "FUTURE_SCHEMA" {
		t.Fatal("future schema not read-only")
	}
	if s.CreateSpace(ctx, CreateSpaceRequest{Name: "oops"}).OK {
		t.Fatal("future schema accepted write")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(bytes) {
		t.Fatal("future schema overwritten")
	}
}

func TestReviewedReceiptIsIdempotentAcrossEditsAndDeletion(t *testing.T) {
	s, _, p := setup(t)
	source := SourceRef{ConnectionScope: "scope", SessionID: "session", InvocationID: "turn", EventRef: "event"}
	req := SaveReviewedResultRequest{ReceiptID: "review-1", SourceRef: source, SpaceID: p.SpaceID, Title: "reviewed", Content: "result", DraftDigest: DraftDigest("reviewed", "result")}
	first := s.SaveReviewedResult(ctx, req)
	second := s.SaveReviewedResult(ctx, req)
	if !first.OK || !second.OK || first.Value.Page.ID != second.Value.Page.ID {
		t.Fatal("receipt duplicated page")
	}
	req.Content = "changed"
	req.DraftDigest = DraftDigest(req.Title, req.Content)
	if r := s.SaveReviewedResult(ctx, req); r.OK || r.Error.Code != "CONFLICT" {
		t.Fatal("changed reviewed payload accepted")
	}
	if !s.DeletePage(ctx, DeletePageRequest{ID: first.Value.Page.ID, ExpectedRevision: 1}).OK {
		t.Fatal("delete reviewed")
	}
	req.Content = "result"
	req.DraftDigest = DraftDigest(req.Title, req.Content)
	after := s.SaveReviewedResult(ctx, req)
	if !after.OK || after.Value.Page != nil || !after.Value.Receipt.PageDeleted {
		t.Fatal("retry recreated deleted result")
	}
}

func TestUTF16Limits(t *testing.T) {
	s, _, p := setup(t)
	good := s.CreatePage(ctx, CreatePageRequest{SpaceID: p.SpaceID, Title: strings.Repeat("😀", 80), Content: strings.Repeat("😀", 50000)})
	if !good.OK {
		t.Fatal("exact UTF16 limit rejected")
	}
	badTitle := strings.Repeat("😀", 80) + "a"
	if s.CreatePage(ctx, CreatePageRequest{SpaceID: p.SpaceID, Title: badTitle}).OK {
		t.Fatal("161 UTF16 title accepted")
	}
	over := strings.Repeat("😀", 50000) + "a"
	if s.PatchPage(ctx, PatchPageRequest{ID: p.ID, ExpectedRevision: 1, Content: &over}).OK {
		t.Fatal("100001 UTF16 content accepted")
	}
}

func TestSecondProcessRejectsWrites(t *testing.T) {
	if path := os.Getenv("WORKSPACE_LOCK_TEST_PATH"); path != "" {
		s := NewWithPath(path)
		defer Shutdown(s)
		r := s.CreateSpace(ctx, CreateSpaceRequest{Name: "second"})
		if r.OK || r.Error.Code != "STORE_LOCKED" {
			t.Fatalf("second process wrote: %+v", r)
		}
		return
	}
	s, path, _ := setup(t)
	_ = s
	command := exec.Command(os.Args[0], "-test.run=^TestSecondProcessRejectsWrites$")
	command.Env = append(os.Environ(), "WORKSPACE_LOCK_TEST_PATH="+path)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("child: %v %s", err, out)
	}
	bytes, _ := os.ReadFile(path)
	var snap Snapshot
	if err := json.Unmarshal(bytes, &snap); err != nil || len(snap.Spaces) != 1 {
		t.Fatal("child changed snapshot")
	}
}

func TestAtomicBackupFailureAndSemanticCorruption(t *testing.T) {
	s, path, p := setup(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if backup, err := os.ReadFile(path + ".bak"); err != nil {
		t.Fatal(err)
	} else if _, code := decodeSnapshot(backup); code != "" {
		t.Fatal("invalid previous recovery snapshot")
	}
	// A destination directory makes the real backup rename fail after writing
	// and syncing the same-directory temporary file.
	if err := os.Remove(path + ".bak"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".bak", 0700); err != nil {
		t.Fatal(err)
	}
	text := "new"
	result := s.PatchPage(ctx, PatchPageRequest{ID: p.ID, ExpectedRevision: 1, Content: &text})
	after, _ := os.ReadFile(path)
	if result.OK || result.Error.Code != "STORAGE_WRITE_FAILED" || string(after) != string(before) || s.GetPage(ctx, GetPageRequest{ID: p.ID}).Value.Revision != 1 {
		t.Fatal("backup failure altered authority")
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".workspace-*.tmp"))
	if len(matches) != 0 {
		t.Fatal("temporary files leaked")
	}
	Shutdown(s)
	if err := os.Remove(path + ".bak"); err != nil {
		t.Fatal(err)
	}
	var invalid Snapshot
	if err := json.Unmarshal(before, &invalid); err != nil {
		t.Fatal(err)
	}
	invalid.Pages[0].ParentID = &invalid.Pages[0].ID
	bad, _ := json.Marshal(invalid)
	os.WriteFile(path, bad, 0600)
	reopened := NewWithPath(path)
	defer Shutdown(reopened)
	state := reopened.Init(ctx, InitRequest{})
	if !state.OK || !state.Value.ReadOnly || state.Value.RecoveryCode != "CORRUPT_SNAPSHOT" || state.Value.RecoveryAvailable {
		t.Fatal("semantically corrupt hierarchy became writable")
	}
}

func TestMissingMainRecoversWithoutRecreatingFile(t *testing.T) {
	s, path, _ := setup(t)
	Shutdown(s)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	reopened := NewWithPath(path)
	defer Shutdown(reopened)
	state := reopened.Init(ctx, InitRequest{})
	if !state.OK || !state.Value.ReadOnly || !state.Value.RecoveryAvailable || state.Value.RecoveryCode != "MISSING_SNAPSHOT" {
		t.Fatal("missing main ignored existing backup")
	}
	if reopened.CreateSpace(ctx, CreateSpaceRequest{Name: "empty"}).OK {
		t.Fatal("missing main recreated")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("missing main rewritten")
	}
}

func TestDraftOrderingAndCleanupCAS(t *testing.T) {
	s, _, p := setup(t)
	newer := SaveDraftRequest{PageID: p.ID, BaseRevision: 1, Title: "", Content: "newer", CapturedAt: 200}
	if !s.SaveDraft(ctx, newer).OK {
		t.Fatal("blank draft title must remain recoverable")
	}
	older := newer
	older.CapturedAt = 100
	older.Content = "late older response"
	if r := s.SaveDraft(ctx, older); r.OK || r.Error.Status != 409 {
		t.Fatal("late draft overwrote newer draft")
	}
	if r := s.DeleteDraft(ctx, DeleteDraftRequest{PageID: p.ID, CapturedAt: 100}); r.OK || r.Error.Status != 409 {
		t.Fatal("late save cleanup deleted newer draft")
	}
	if got := s.ListRecoverableDrafts(ctx, ListRecoverableDraftsRequest{}); len(got.Value.Drafts) != 1 || got.Value.Drafts[0].Content != "newer" {
		t.Fatal("new draft lost")
	}
	if !s.DeleteDraft(ctx, DeleteDraftRequest{PageID: p.ID, CapturedAt: 200}).OK {
		t.Fatal("explicit current draft cleanup rejected")
	}
}

func TestReviewedUpdateCASAndReceiptRestart(t *testing.T) {
	s, path, p := setup(t)
	req := SaveReviewedResultRequest{ReceiptID: "review-update", SourceRef: SourceRef{ConnectionScope: "scope", SessionID: "session", InvocationID: "turn", EventRef: "event"}, SpaceID: p.SpaceID, PageID: p.ID, ExpectedRevision: 1, Title: p.Title, Content: "reviewed"}
	req.DraftDigest = DraftDigest(req.Title, req.Content)
	edited := "manual edit"
	if !s.PatchPage(ctx, PatchPageRequest{ID: p.ID, ExpectedRevision: 1, Content: &edited}).OK {
		t.Fatal("edit")
	}
	denied := s.SaveReviewedResult(ctx, req)
	if denied.OK || denied.Error.CurrentPage == nil || denied.Error.CurrentPage.Content != "manual edit" || denied.Error.Status != 409 {
		t.Fatal("review replaced concurrent edit")
	}
	if s.GetReviewedResult(ctx, GetReviewedResultRequest{ReceiptID: req.ReceiptID}).OK {
		t.Fatal("failed review consumed receipt")
	}
	req.ExpectedRevision = 2
	saved := s.SaveReviewedResult(ctx, req)
	if !saved.OK || saved.Value.Page.Revision != 3 {
		t.Fatal("review update")
	}
	Shutdown(s)
	reopened := NewWithPath(path)
	defer Shutdown(reopened)
	retried := reopened.SaveReviewedResult(ctx, req)
	if !retried.OK || retried.Value.Page.Revision != 3 || retried.Value.Receipt.PageID != p.ID {
		t.Fatal("review retry after restart wrote again")
	}
}

func TestReadAndRequestPointersCannotMutateAuthority(t *testing.T) {
	s, _, p := setup(t)
	parent := p.ID
	child := s.CreatePage(ctx, CreatePageRequest{SpaceID: p.SpaceID, ParentID: &parent, Title: "child"})
	if !child.OK {
		t.Fatal("child")
	}
	parent = "injected"
	*child.Value.ParentID = "response injection"
	state := s.Init(ctx, InitRequest{})
	state.Value.Snapshot.Pages[0].Title = "snapshot injection"
	got := s.GetPage(ctx, GetPageRequest{ID: child.Value.ID})
	if got.Value.ParentID == nil || *got.Value.ParentID != p.ID || s.GetPage(ctx, GetPageRequest{ID: p.ID}).Value.Title != "Page" {
		t.Fatal("external pointer mutated store")
	}
}

func TestDigestMatchesJavaScriptJSONAndSafeReferences(t *testing.T) {
	// JSON.stringify emits separators literally but escapes an actual backslash.
	payload := []byte("[\"<title>\",\"x\u2028y\u2029z\\\\u2028\"]")
	hash := sha256.Sum256(payload)
	if DraftDigest("<title>", "x\u2028y\u2029z\\u2028") != hex.EncodeToString(hash[:]) {
		t.Fatal("digest differs from JavaScript JSON")
	}
	s, _, p := setup(t)
	bad := BotRef{ConnectionScope: "https://host/?token=do-not-persist", SessionID: "session"}
	if s.LinkConversation(ctx, LinkConversationRequest{PageID: p.ID, BotRef: bad}).OK {
		t.Fatal("credential-bearing reference persisted")
	}
}

func TestTitleWhitespaceMatchesJavaScriptTrim(t *testing.T) {
	s, _, p := setup(t)
	if s.CreatePage(ctx, CreatePageRequest{SpaceID: p.SpaceID, Title: "\ufeff\u00a0"}).OK {
		t.Fatal("JavaScript blank title accepted")
	}
	title := "\ufeff Page \ufeff"
	created := s.CreatePage(ctx, CreatePageRequest{SpaceID: p.SpaceID, Title: title})
	if !created.OK || created.Value.Title != "Page" {
		t.Fatal("JavaScript title whitespace not trimmed")
	}
	// NEL is Go whitespace, but ECMAScript deliberately does not trim it.
	if r := s.CreatePage(ctx, CreatePageRequest{SpaceID: p.SpaceID, Title: "\u0085"}); !r.OK || r.Value.Title != "\u0085" {
		t.Fatal("Go whitespace changed JavaScript title semantics")
	}
}

func TestDeleteSpaceRequiresConfirmationAndKeepsRecoverableSources(t *testing.T) {
	s, _, p := setup(t)
	if !s.LinkConversation(ctx, LinkConversationRequest{PageID: p.ID, BotRef: BotRef{ConnectionScope: "scope", SessionID: "session"}}).OK {
		t.Fatal("link")
	}
	if !s.SaveDraft(ctx, SaveDraftRequest{PageID: p.ID, BaseRevision: 1, Title: p.Title, Content: "unsaved", CapturedAt: 1}).OK {
		t.Fatal("draft")
	}
	denied := s.DeleteSpace(ctx, DeleteSpaceRequest{ID: p.SpaceID, ExpectedRevision: 1})
	if denied.OK || denied.Error.Code != "SPACE_NOT_EMPTY" {
		t.Fatal("unconfirmed nonempty space deleted")
	}
	if s.DeleteSpace(ctx, DeleteSpaceRequest{ID: p.SpaceID, ExpectedRevision: 2, ConfirmNonEmpty: true}).OK {
		t.Fatal("stale space revision accepted")
	}
	deleted := s.DeleteSpace(ctx, DeleteSpaceRequest{ID: p.SpaceID, ExpectedRevision: 1, ConfirmNonEmpty: true})
	if !deleted.OK {
		t.Fatalf("delete: %+v", deleted.Error)
	}
	state := s.Init(ctx, InitRequest{})
	if len(state.Value.Snapshot.Spaces) != 0 || len(state.Value.Snapshot.Pages) != 0 || len(state.Value.Snapshot.DeletedPages) != 1 || len(state.Value.Snapshot.RecoverableDrafts) != 1 || state.Value.Snapshot.ConversationLinks[0].DeletedAt == nil {
		t.Fatal("local deletion lost source/draft recovery")
	}
}

func TestInvalidUTF8DraftIsRejectedWithoutReplacement(t *testing.T) {
	s, _, p := setup(t)
	result := s.SaveDraft(ctx, SaveDraftRequest{PageID: p.ID, BaseRevision: 1, Title: p.Title, Content: string([]byte{0xff}), CapturedAt: 1})
	if result.OK {
		t.Fatal("invalid UTF8 was silently rewritten into a different draft")
	}
	if len(s.ListRecoverableDrafts(ctx, ListRecoverableDraftsRequest{}).Value.Drafts) != 0 {
		t.Fatal("invalid draft persisted")
	}
}
