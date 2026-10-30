package workspace

import (
	"context"
	"sort"
)

func sameDraft(a, b RecoverableDraft) bool {
	return a.PageID == b.PageID && a.BaseRevision == b.BaseRevision && a.Title == b.Title && a.Content == b.Content && sameParent(a.ParentID, b.ParentID)
}
func (s *Service) preserveUnsaved(d RecoverableDraft) {
	if s.unsavedDrafts == nil {
		s.unsavedDrafts = map[string]RecoverableDraft{}
	}
	copy := cloneSnapshot(Snapshot{RecoverableDrafts: []RecoverableDraft{d}}).RecoverableDrafts[0]
	s.unsavedDrafts[d.PageID] = copy
}
func (s *Service) closeState() *CloseState {
	out := &CloseState{SafeClose: len(s.unsavedDrafts) == 0, ReadOnly: s.readOnly, RecoveryCode: s.recoveryCode, UnsavedDrafts: []RecoverableDraft{}, Actions: []string{}}
	for _, d := range s.unsavedDrafts {
		out.UnsavedDrafts = append(out.UnsavedDrafts, d)
	}
	sort.Slice(out.UnsavedDrafts, func(i, j int) bool { return out.UnsavedDrafts[i].PageID < out.UnsavedDrafts[j].PageID })
	out.UnsavedDrafts = cloneSnapshot(Snapshot{RecoverableDrafts: out.UnsavedDrafts}).RecoverableDrafts
	if !out.SafeClose {
		out.Actions = []string{"continueEditing", "export", "abandon"}
	}
	return out
}
func (s *Service) SafeClose() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.unsavedDrafts) == 0
}
func (s *Service) CanClose(ctx context.Context, _ CanCloseRequest) CloseStateResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, false); e != nil {
		return CloseStateResult{Error: e}
	}
	return CloseStateResult{OK: true, Value: s.closeState()}
}

// AbandonUnsavedDraft consumes only an explicitly reviewed in-memory draft.
// Durable recovery drafts remain untouched; exporting alone never calls this.
func (s *Service) AbandonUnsavedDraft(ctx context.Context, r AbandonUnsavedDraftRequest) MutationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, false); e != nil {
		return MutationResult{Error: e}
	}
	d, ok := s.unsavedDrafts[r.PageID]
	if !ok || !r.ConfirmAbandon || r.CapturedAt != d.CapturedAt || r.DraftDigest != DraftDigest(d.Title, d.Content) {
		return MutationResult{Error: safeError("CONFLICT", 409, "The unsaved draft changed or abandonment was not confirmed.")}
	}
	delete(s.unsavedDrafts, r.PageID)
	return MutationResult{OK: true, Value: &Mutation{StoreRevision: s.snapshot.StoreRevision, Changed: true, CloseState: s.closeState()}}
}
