package workspace

import (
	"os"
	"testing"
)

func TestReadOnlyRecoveredSnapshotCanCloseWithoutNewDraft(t *testing.T) {
	s, path, _ := setup(t)
	Shutdown(s)
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	r := NewWithPath(path)
	defer Shutdown(r)
	state := r.Init(ctx, InitRequest{})
	if !state.OK || !state.Value.ReadOnly {
		t.Fatal("expected recovered readonly")
	}
	before, _ := os.ReadFile(path)
	flush := r.Flush(ctx, FlushRequest{})
	after, _ := os.ReadFile(path)
	if !flush.OK || flush.Value.CloseState == nil || !flush.Value.CloseState.SafeClose || !flush.Value.CloseState.ReadOnly || flush.Value.Changed || string(before) != string(after) {
		t.Fatalf("unsafe readonly flush %+v", flush)
	}
}
func TestReadOnlyUnsavedDraftBlocksAndExactAbandonAllowsClose(t *testing.T) {
	s, path, p := setup(t)
	if !s.SaveDraft(ctx, SaveDraftRequest{PageID: p.ID, BaseRevision: 1, Title: p.Title, Content: p.Content, CapturedAt: 1}).OK {
		t.Fatal("prepare page backup")
	}
	Shutdown(s)
	os.WriteFile(path, []byte("broken"), 0600)
	r := NewWithPath(path)
	defer Shutdown(r)
	r.Init(ctx, InitRequest{})
	draft := SaveDraftRequest{PageID: p.ID, BaseRevision: 1, Title: p.Title, Content: "new unsaved draft", CapturedAt: 123}
	result := r.SaveDraft(ctx, draft)
	if result.OK {
		t.Fatal("readonly draft falsely durable")
	}
	flush := r.Flush(ctx, FlushRequest{})
	if flush.OK || flush.Error.Code != "UNSAVED_DRAFTS" || flush.Error.CloseState == nil || flush.Error.CloseState.SafeClose || len(flush.Error.CloseState.UnsavedDrafts) != 1 {
		t.Fatalf("lost unsaved state %+v", flush)
	}
	bad := r.AbandonUnsavedDraft(ctx, AbandonUnsavedDraftRequest{PageID: p.ID, CapturedAt: 123, DraftDigest: DraftDigest(p.Title, "wrong"), ConfirmAbandon: true})
	if bad.OK {
		t.Fatal("wrong draft abandoned")
	}
	good := r.AbandonUnsavedDraft(ctx, AbandonUnsavedDraftRequest{PageID: p.ID, CapturedAt: 123, DraftDigest: DraftDigest(p.Title, draft.Content), ConfirmAbandon: true})
	if !good.OK || !r.CanClose(ctx, CanCloseRequest{}).Value.SafeClose {
		t.Fatal("exact abandon did not allow close")
	}
	after, _ := os.ReadFile(path)
	if string(after) != "broken" {
		t.Fatal("read-only disk changed")
	}
}
func TestFutureSchemaReadOnlyCloseDoesNotMutate(t *testing.T) {
	path := t.TempDir() + "/workspace.json"
	original := []byte(`{"schemaVersion":999}`)
	os.WriteFile(path, original, 0600)
	s := NewWithPath(path)
	defer Shutdown(s)
	state := s.Init(ctx, InitRequest{})
	if !state.OK || !state.Value.ReadOnly {
		t.Fatal("future schema not readonly")
	}
	flush := s.Flush(ctx, FlushRequest{})
	if !flush.OK || !flush.Value.CloseState.SafeClose {
		t.Fatal("future readonly safe close blocked")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("future schema overwritten")
	}
}
func TestFailedDiskDraftRemainsUnsafeUntilDurableRetry(t *testing.T) {
	s, _, p := setup(t)
	persist := s.persist
	s.persist = func(string, []byte, []byte) error { return os.ErrPermission }
	draft := SaveDraftRequest{PageID: p.ID, BaseRevision: 1, Title: p.Title, Content: "disk failure draft", CapturedAt: 1}
	if s.SaveDraft(ctx, draft).OK || s.Flush(ctx, FlushRequest{}).OK {
		t.Fatal("undurable draft allowed close")
	}
	s.persist = persist
	if !s.SaveDraft(ctx, draft).OK || !s.Flush(ctx, FlushRequest{}).Value.CloseState.SafeClose {
		t.Fatal("durable retry still blocked")
	}
}
func TestUnsavedDraftClearsAfterMatchingPageBecomesDurable(t *testing.T) {
	s, _, p := setup(t)
	persist := s.persist
	s.persist = func(string, []byte, []byte) error { return os.ErrPermission }
	draft := SaveDraftRequest{PageID: p.ID, BaseRevision: 1, Title: p.Title, Content: "saved by page retry", CapturedAt: 1}
	s.SaveDraft(ctx, draft)
	s.persist = persist
	content := draft.Content
	if !s.PatchPage(ctx, PatchPageRequest{ID: p.ID, ExpectedRevision: 1, Content: &content}).OK {
		t.Fatal("page retry failed")
	}
	flush := s.Flush(ctx, FlushRequest{})
	if !flush.OK || !flush.Value.CloseState.SafeClose {
		t.Fatal("durable page retry still blocks close")
	}
}
