package workspace

import (
	"context"
	"unicode/utf8"
)

func (s *Service) Init(ctx context.Context, _ InitRequest) StateResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, false); e != nil {
		return StateResult{Error: e}
	}
	return StateResult{OK: true, Value: &State{Snapshot: cloneSnapshot(s.snapshot), ReadOnly: s.readOnly, RecoveryCode: s.recoveryCode, RecoveryAvailable: s.recoveryAvailable}}
}
func (s *Service) ListSpaces(ctx context.Context, _ ListSpacesRequest) SpaceListResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, false); e != nil {
		return SpaceListResult{Error: e}
	}
	snap := cloneSnapshot(s.snapshot)
	return SpaceListResult{OK: true, Value: &SpaceList{Spaces: snap.Spaces, StoreRevision: snap.StoreRevision}}
}
func (s *Service) CreateSpace(ctx context.Context, req CreateSpaceRequest) SpaceResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, true); e != nil {
		return SpaceResult{Error: e}
	}
	if !validText(req.Name, "") {
		return SpaceResult{Error: validation()}
	}
	next := cloneSnapshot(s.snapshot)
	time := now()
	space := Space{ID: newID(), Name: trimTitle(req.Name), Revision: 1, CreatedAt: time, UpdatedAt: time}
	next.Spaces = append(next.Spaces, space)
	if e := s.commit(ctx, next); e != nil {
		return SpaceResult{Error: e}
	}
	return SpaceResult{OK: true, Value: &space}
}
func (s *Service) PatchSpace(ctx context.Context, req PatchSpaceRequest) SpaceResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, true); e != nil {
		return SpaceResult{Error: e}
	}
	i := spaceIndex(s.snapshot, req.ID)
	if i < 0 {
		return SpaceResult{Error: notFound()}
	}
	if req.ExpectedRevision != s.snapshot.Spaces[i].Revision {
		return SpaceResult{Error: safeError("CONFLICT", 409, "This Space changed. Reload its current revision.")}
	}
	if !validText(req.Name, "") {
		return SpaceResult{Error: validation()}
	}
	next := cloneSnapshot(s.snapshot)
	space := next.Spaces[i]
	space.Name = trimTitle(req.Name)
	space.Revision++
	space.UpdatedAt = now()
	next.Spaces[i] = space
	if e := s.commit(ctx, next); e != nil {
		return SpaceResult{Error: e}
	}
	return SpaceResult{OK: true, Value: &space}
}
func (s *Service) DeleteSpace(ctx context.Context, req DeleteSpaceRequest) MutationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, true); e != nil {
		return MutationResult{Error: e}
	}
	i := spaceIndex(s.snapshot, req.ID)
	if i < 0 {
		return MutationResult{Error: notFound()}
	}
	if req.ExpectedRevision != s.snapshot.Spaces[i].Revision {
		return MutationResult{Error: safeError("CONFLICT", 409, "This Space changed. Reload its current revision.")}
	}
	ids := map[string]bool{}
	for _, p := range s.snapshot.Pages {
		if p.SpaceID == req.ID {
			ids[p.ID] = true
		}
	}
	if len(ids) > 0 && !req.ConfirmNonEmpty {
		return MutationResult{Error: safeError("SPACE_NOT_EMPTY", 409, "This Space contains pages. Review them and explicitly confirm local deletion.")}
	}
	next := cloneSnapshot(s.snapshot)
	next.Spaces = append(next.Spaces[:i], next.Spaces[i+1:]...)
	archivePages(&next, ids, now())
	if e := s.commit(ctx, next); e != nil {
		return MutationResult{Error: e}
	}
	return MutationResult{OK: true, Value: &Mutation{StoreRevision: s.snapshot.StoreRevision, Changed: true}}
}
func (s *Service) ListPages(ctx context.Context, req ListPagesRequest) PageListResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, false); e != nil {
		return PageListResult{Error: e}
	}
	if spaceIndex(s.snapshot, req.SpaceID) < 0 {
		return PageListResult{Error: notFound()}
	}
	pages := []Page{}
	for _, p := range cloneSnapshot(s.snapshot).Pages {
		if p.SpaceID == req.SpaceID {
			pages = append(pages, p)
		}
	}
	return PageListResult{OK: true, Value: &PageList{Pages: pages, StoreRevision: s.snapshot.StoreRevision}}
}
func (s *Service) GetPage(ctx context.Context, req GetPageRequest) PageResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, false); e != nil {
		return PageResult{Error: e}
	}
	i := pageIndex(s.snapshot, req.ID)
	if i < 0 {
		return PageResult{Error: notFound()}
	}
	page := cloneSnapshot(s.snapshot).Pages[i]
	return PageResult{OK: true, Value: &page}
}
func (s *Service) CreatePage(ctx context.Context, req CreatePageRequest) PageResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, true); e != nil {
		return PageResult{Error: e}
	}
	if spaceIndex(s.snapshot, req.SpaceID) < 0 {
		return PageResult{Error: notFound()}
	}
	if !validText(req.Title, req.Content) || !validParent(s.snapshot, "", req.SpaceID, req.ParentID) {
		return PageResult{Error: validation()}
	}
	next := cloneSnapshot(s.snapshot)
	time := now()
	page := Page{ID: newID(), SpaceID: req.SpaceID, ParentID: req.ParentID, Title: trimTitle(req.Title), Content: req.Content, Revision: 1, CreatedAt: time, UpdatedAt: time}
	// Copy input pointers as well as response pointers.
	next.Pages = append(next.Pages, page)
	next = cloneSnapshot(next)
	if e := s.commit(ctx, next); e != nil {
		return PageResult{Error: e}
	}
	page = cloneSnapshot(s.snapshot).Pages[len(next.Pages)-1]
	return PageResult{OK: true, Value: &page}
}
func (s *Service) PatchPage(ctx context.Context, req PatchPageRequest) PageResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, true); e != nil {
		return PageResult{Error: e}
	}
	i := pageIndex(s.snapshot, req.ID)
	if i < 0 {
		return PageResult{Error: notFound()}
	}
	current := cloneSnapshot(s.snapshot).Pages[i]
	if req.ExpectedRevision != current.Revision {
		return PageResult{Error: conflict(current)}
	}
	next := cloneSnapshot(s.snapshot)
	page := next.Pages[i]
	if req.Title != nil {
		page.Title = trimTitle(*req.Title)
	}
	if req.Content != nil {
		page.Content = *req.Content
	}
	if req.SetParent || req.ParentID != nil {
		page.ParentID = req.ParentID
	}
	if (req.Title != nil && !validText(*req.Title, page.Content)) || !validText(page.Title, page.Content) || !validParent(next, page.ID, page.SpaceID, page.ParentID) {
		return PageResult{Error: validation()}
	}
	page.Revision++
	page.UpdatedAt = now()
	next.Pages[i] = page
	next = cloneSnapshot(next)
	if e := s.commit(ctx, next); e != nil {
		return PageResult{Error: e}
	}
	page = cloneSnapshot(s.snapshot).Pages[i]
	return PageResult{OK: true, Value: &page}
}
func (s *Service) MovePage(ctx context.Context, req MovePageRequest) PageResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, true); e != nil {
		return PageResult{Error: e}
	}
	i := pageIndex(s.snapshot, req.ID)
	if i < 0 || spaceIndex(s.snapshot, req.SpaceID) < 0 {
		return PageResult{Error: notFound()}
	}
	current := cloneSnapshot(s.snapshot).Pages[i]
	if current.Revision != req.ExpectedRevision {
		return PageResult{Error: conflict(current)}
	}
	if !validParent(s.snapshot, req.ID, req.SpaceID, req.ParentID) {
		return PageResult{Error: validation()}
	}
	subtree := map[string]bool{req.ID: true}
	changed := true
	for changed {
		changed = false
		for _, p := range s.snapshot.Pages {
			if p.ParentID != nil && subtree[*p.ParentID] && !subtree[p.ID] {
				subtree[p.ID] = true
				changed = true
			}
		}
	}
	if req.ParentID != nil && subtree[*req.ParentID] {
		return PageResult{Error: validation()}
	}
	next := cloneSnapshot(s.snapshot)
	time := now()
	for j := range next.Pages {
		if subtree[next.Pages[j].ID] {
			next.Pages[j].SpaceID = req.SpaceID
			next.Pages[j].Revision++
			next.Pages[j].UpdatedAt = time
		}
	}
	next.Pages[i].ParentID = req.ParentID
	next = cloneSnapshot(next)
	if e := s.commit(ctx, next); e != nil {
		return PageResult{Error: e}
	}
	page := cloneSnapshot(s.snapshot).Pages[i]
	return PageResult{OK: true, Value: &page}
}
func archivePages(snap *Snapshot, ids map[string]bool, time int64) {
	keep := []Page{}
	for _, p := range snap.Pages {
		if ids[p.ID] {
			snap.DeletedPages = append(snap.DeletedPages, DeletedPage{Page: p, DeletedAt: time})
		} else {
			keep = append(keep, p)
		}
	}
	snap.Pages = keep
	for i := range snap.ConversationLinks {
		if ids[snap.ConversationLinks[i].PageID] {
			timestamp := time
			snap.ConversationLinks[i].DeletedAt = &timestamp
		}
	}
	for i := range snap.ReviewReceipts {
		if ids[snap.ReviewReceipts[i].PageID] {
			snap.ReviewReceipts[i].PageDeleted = true
		}
	}
}
func (s *Service) DeletePage(ctx context.Context, req DeletePageRequest) MutationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, true); e != nil {
		return MutationResult{Error: e}
	}
	i := pageIndex(s.snapshot, req.ID)
	if i < 0 {
		return MutationResult{Error: notFound()}
	}
	current := cloneSnapshot(s.snapshot).Pages[i]
	if current.Revision != req.ExpectedRevision {
		return MutationResult{Error: conflict(current)}
	}
	next := cloneSnapshot(s.snapshot)
	time := now()
	for j := range next.Pages {
		p := &next.Pages[j]
		if p.ParentID != nil && *p.ParentID == req.ID {
			p.ParentID = current.ParentID
			p.Revision++
			p.UpdatedAt = time
		}
	}
	archivePages(&next, map[string]bool{req.ID: true}, time)
	if e := s.commit(ctx, next); e != nil {
		return MutationResult{Error: e}
	}
	return MutationResult{OK: true, Value: &Mutation{StoreRevision: s.snapshot.StoreRevision, Changed: true}}
}
func (s *Service) ListConversationLinks(ctx context.Context, req ListConversationLinksRequest) LinkListResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, false); e != nil {
		return LinkListResult{Error: e}
	}
	links := []ConversationLink{}
	for _, l := range cloneSnapshot(s.snapshot).ConversationLinks {
		if req.PageID == "" || l.PageID == req.PageID {
			links = append(links, l)
		}
	}
	return LinkListResult{OK: true, Value: &LinkList{Links: links}}
}
func (s *Service) LinkConversation(ctx context.Context, req LinkConversationRequest) LinkResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, true); e != nil {
		return LinkResult{Error: e}
	}
	if pageIndex(s.snapshot, req.PageID) < 0 {
		return LinkResult{Error: notFound()}
	}
	if !validBot(req.BotRef) {
		return LinkResult{Error: validation()}
	}
	for _, l := range cloneSnapshot(s.snapshot).ConversationLinks {
		if l.PageID == req.PageID && l.BotRef == req.BotRef {
			return LinkResult{OK: true, Value: &l}
		}
	}
	next := cloneSnapshot(s.snapshot)
	link := ConversationLink{PageID: req.PageID, BotRef: req.BotRef, CreatedAt: now()}
	next.ConversationLinks = append(next.ConversationLinks, link)
	if e := s.commit(ctx, next); e != nil {
		return LinkResult{Error: e}
	}
	return LinkResult{OK: true, Value: &link}
}

func reviewed(snap Snapshot, receipt ReviewReceipt) *ReviewedResult {
	r := &ReviewedResult{Receipt: receipt}
	if i := pageIndex(snap, receipt.PageID); i >= 0 {
		page := snap.Pages[i]
		r.Page = &page
	}
	return r
}
func (s *Service) GetReviewedResult(ctx context.Context, req GetReviewedResultRequest) ReviewedResultResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, false); e != nil {
		return ReviewedResultResponse{Error: e}
	}
	snap := cloneSnapshot(s.snapshot)
	for _, r := range snap.ReviewReceipts {
		if r.ID == req.ReceiptID {
			return ReviewedResultResponse{OK: true, Value: reviewed(snap, r)}
		}
	}
	return ReviewedResultResponse{Error: notFound()}
}
func (s *Service) SaveReviewedResult(ctx context.Context, req SaveReviewedResultRequest) ReviewedResultResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, true); e != nil {
		return ReviewedResultResponse{Error: e}
	}
	if !safeRefPart(req.ReceiptID) || !validSource(req.SourceRef) || !validText(req.Title, req.Content) || req.DraftDigest != DraftDigest(req.Title, req.Content) {
		return ReviewedResultResponse{Error: validation()}
	}
	digest := payloadDigest(req)
	for _, r := range cloneSnapshot(s.snapshot).ReviewReceipts {
		if r.ID == req.ReceiptID {
			if r.PayloadDigest != digest {
				return ReviewedResultResponse{Error: safeError("CONFLICT", 409, "This receipt was already used for a different reviewed result. Confirm a new review.")}
			}
			return ReviewedResultResponse{OK: true, Value: reviewed(cloneSnapshot(s.snapshot), r)}
		}
	}
	if spaceIndex(s.snapshot, req.SpaceID) < 0 {
		return ReviewedResultResponse{Error: notFound()}
	}
	next := cloneSnapshot(s.snapshot)
	time := now()
	var page Page
	if req.PageID != "" {
		i := pageIndex(next, req.PageID)
		if i < 0 {
			return ReviewedResultResponse{Error: notFound()}
		}
		page = next.Pages[i]
		if page.Revision != req.ExpectedRevision {
			return ReviewedResultResponse{Error: conflict(page)}
		}
		if page.SpaceID != req.SpaceID {
			return ReviewedResultResponse{Error: validation()}
		}
		page.Title = trimTitle(req.Title)
		page.Content = req.Content
		page.ParentID = req.ParentID
		page.SourceRef = &req.SourceRef
		page.Revision++
		page.UpdatedAt = time
		if !validParent(next, page.ID, page.SpaceID, page.ParentID) {
			return ReviewedResultResponse{Error: validation()}
		}
		next.Pages[i] = page
	} else {
		if req.ExpectedRevision != 0 || !validParent(next, "", req.SpaceID, req.ParentID) {
			return ReviewedResultResponse{Error: validation()}
		}
		page = Page{ID: newID(), SpaceID: req.SpaceID, ParentID: req.ParentID, Title: trimTitle(req.Title), Content: req.Content, Revision: 1, CreatedAt: time, UpdatedAt: time, SourceRef: &req.SourceRef}
		next.Pages = append(next.Pages, page)
	}
	receipt := ReviewReceipt{ID: req.ReceiptID, SourceRef: req.SourceRef, DraftDigest: req.DraftDigest, PayloadDigest: digest, PageID: page.ID, SpaceID: page.SpaceID, PageRevision: page.Revision, CreatedAt: time}
	next.ReviewReceipts = append(next.ReviewReceipts, receipt)
	next = cloneSnapshot(next)
	if e := s.commit(ctx, next); e != nil {
		return ReviewedResultResponse{Error: e}
	}
	return ReviewedResultResponse{OK: true, Value: reviewed(cloneSnapshot(s.snapshot), receipt)}
}
func (s *Service) ListRecoverableDrafts(ctx context.Context, req ListRecoverableDraftsRequest) DraftListResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, false); e != nil {
		return DraftListResult{Error: e}
	}
	drafts := []RecoverableDraft{}
	for _, d := range cloneSnapshot(s.snapshot).RecoverableDrafts {
		if req.PageID == "" || req.PageID == d.PageID {
			drafts = append(drafts, d)
		}
	}
	return DraftListResult{OK: true, Value: &DraftList{Drafts: drafts}}
}
func (s *Service) SaveDraft(ctx context.Context, req SaveDraftRequest) DraftResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, false); e != nil {
		return DraftResult{Error: e}
	}
	i := pageIndex(s.snapshot, req.PageID)
	if i < 0 {
		return DraftResult{Error: notFound()}
	}
	if req.BaseRevision < 1 || req.BaseRevision > s.snapshot.Pages[i].Revision || !utf8.ValidString(req.Title) || !utf8.ValidString(req.Content) || utf16Length(req.Title) > 160 || utf16Length(req.Content) > 100000 || req.CapturedAt < 1 {
		return DraftResult{Error: validation()}
	}
	draft := RecoverableDraft{PageID: req.PageID, BaseRevision: req.BaseRevision, Title: req.Title, Content: req.Content, ParentID: req.ParentID, CapturedAt: req.CapturedAt}
	if prior, ok := s.unsavedDrafts[req.PageID]; ok && (draft.CapturedAt < prior.CapturedAt || (draft.CapturedAt == prior.CapturedAt && !sameDraft(draft, prior))) {
		return DraftResult{Error: safeError("CONFLICT", 409, "A newer unsaved draft exists. It was preserved.")}
	}
	next := cloneSnapshot(s.snapshot)
	found := false
	for j, d := range next.RecoverableDrafts {
		if d.PageID == req.PageID {
			if req.CapturedAt < d.CapturedAt || (req.CapturedAt == d.CapturedAt && (draft.BaseRevision != d.BaseRevision || draft.Title != d.Title || draft.Content != d.Content || !sameParent(draft.ParentID, d.ParentID))) {
				return DraftResult{Error: safeError("CONFLICT", 409, "A newer recovery draft exists. It was preserved.")}
			}
			if req.CapturedAt == d.CapturedAt {
				delete(s.unsavedDrafts, req.PageID)
				return DraftResult{OK: true, Value: &d}
			}
			next.RecoverableDrafts[j] = draft
			found = true
			break
		}
	}
	if !found {
		next.RecoverableDrafts = append(next.RecoverableDrafts, draft)
	}
	next = cloneSnapshot(next)
	if e := s.ready(ctx, true); e != nil {
		s.preserveUnsaved(draft)
		e.CloseState = s.closeState()
		return DraftResult{Error: e}
	}
	if e := s.commit(ctx, next); e != nil {
		s.preserveUnsaved(draft)
		e.CloseState = s.closeState()
		return DraftResult{Error: e}
	}
	delete(s.unsavedDrafts, req.PageID)
	for _, d := range cloneSnapshot(s.snapshot).RecoverableDrafts {
		if d.PageID == req.PageID {
			return DraftResult{OK: true, Value: &d}
		}
	}
	return DraftResult{Error: notFound()}
}
func sameParent(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
func (s *Service) DeleteDraft(ctx context.Context, req DeleteDraftRequest) MutationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, true); e != nil {
		return MutationResult{Error: e}
	}
	next := cloneSnapshot(s.snapshot)
	found := false
	for i, d := range next.RecoverableDrafts {
		if d.PageID == req.PageID {
			if d.CapturedAt != req.CapturedAt {
				return MutationResult{Error: safeError("CONFLICT", 409, "The recovery draft changed. The newer draft was preserved.")}
			}
			next.RecoverableDrafts = append(next.RecoverableDrafts[:i], next.RecoverableDrafts[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return MutationResult{OK: true, Value: &Mutation{StoreRevision: s.snapshot.StoreRevision}}
	}
	if e := s.commit(ctx, next); e != nil {
		return MutationResult{Error: e}
	}
	return MutationResult{OK: true, Value: &Mutation{StoreRevision: s.snapshot.StoreRevision, Changed: true}}
}

// All writes are synchronous. Read-only recovery with no new undurable drafts
// is safe to close: this barrier never mutates the recovered snapshot.
func (s *Service) Flush(ctx context.Context, _ FlushRequest) MutationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx, false); e != nil {
		return MutationResult{Error: e}
	}
	state := s.closeState()
	if !state.SafeClose {
		return MutationResult{Error: &ServiceError{Code: "UNSAVED_DRAFTS", Status: 409, Message: "New drafts have not been saved. Continue editing, export them, or explicitly abandon them before closing.", CloseState: state}}
	}
	return MutationResult{OK: true, Value: &Mutation{StoreRevision: s.snapshot.StoreRevision, CloseState: state}}
}
