package workspace

// Workspace DTOs contain document data and safe references only. Connection
// credentials, chat transcripts and arbitrary tool payloads have no fields here.
const SchemaVersion = 1

type Space struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Revision  int64  `json:"revision"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}
type SourceRef struct {
	ConnectionScope string `json:"connectionScope"`
	SessionID       string `json:"sessionId"`
	InvocationID    string `json:"invocationId"`
	EventRef        string `json:"eventRef"`
}

// A local association with an existing remote session; this confers no authority.
type BotRef struct {
	ConnectionScope string `json:"connectionScope"`
	SessionID       string `json:"sessionId"`
}
type Page struct {
	ID        string     `json:"id"`
	SpaceID   string     `json:"spaceId"`
	ParentID  *string    `json:"parentId"`
	Title     string     `json:"title"`
	Content   string     `json:"content"`
	Revision  int64      `json:"revision"`
	CreatedAt int64      `json:"createdAt"`
	UpdatedAt int64      `json:"updatedAt"`
	SourceRef *SourceRef `json:"sourceRef"`
}
type ConversationLink struct {
	PageID    string `json:"pageId"`
	BotRef    BotRef `json:"botRef"`
	CreatedAt int64  `json:"createdAt"`
	DeletedAt *int64 `json:"deletedAt"`
}
type ReviewReceipt struct {
	ID            string    `json:"id"`
	SourceRef     SourceRef `json:"sourceRef"`
	DraftDigest   string    `json:"draftDigest"`
	PayloadDigest string    `json:"payloadDigest"`
	PageID        string    `json:"pageId"`
	SpaceID       string    `json:"spaceId"`
	PageRevision  int64     `json:"pageRevision"`
	CreatedAt     int64     `json:"createdAt"`
	PageDeleted   bool      `json:"pageDeleted"`
}
type RecoverableDraft struct {
	PageID       string  `json:"pageId"`
	BaseRevision int64   `json:"baseRevision"`
	Title        string  `json:"title"`
	Content      string  `json:"content"`
	ParentID     *string `json:"parentId"`
	CapturedAt   int64   `json:"capturedAt"`
}
type DeletedPage struct {
	Page      Page  `json:"page"`
	DeletedAt int64 `json:"deletedAt"`
}
type Snapshot struct {
	SchemaVersion     int                `json:"schemaVersion"`
	StoreRevision     int64              `json:"storeRevision"`
	Spaces            []Space            `json:"spaces"`
	Pages             []Page             `json:"pages"`
	ConversationLinks []ConversationLink `json:"conversationLinks"`
	ReviewReceipts    []ReviewReceipt    `json:"reviewReceipts"`
	RecoverableDrafts []RecoverableDraft `json:"recoverableDrafts"`
	DeletedPages      []DeletedPage      `json:"deletedPages"`
}
type ServiceError struct {
	CloseState  *CloseState `json:"closeState,omitempty"`
	Code        string      `json:"code"`
	Message     string      `json:"message"`
	Status      int         `json:"status"`
	CurrentPage *Page       `json:"currentPage"`
}
type State struct {
	Snapshot          Snapshot `json:"snapshot"`
	ReadOnly          bool     `json:"readOnly"`
	RecoveryCode      string   `json:"recoveryCode"`
	RecoveryAvailable bool     `json:"recoveryAvailable"`
}
type SpaceList struct {
	Spaces        []Space `json:"spaces"`
	StoreRevision int64   `json:"storeRevision"`
}
type PageList struct {
	Pages         []Page `json:"pages"`
	StoreRevision int64  `json:"storeRevision"`
}
type LinkList struct {
	Links []ConversationLink `json:"links"`
}
type DraftList struct {
	Drafts []RecoverableDraft `json:"drafts"`
}
type ReviewedResult struct {
	Page    *Page         `json:"page"`
	Receipt ReviewReceipt `json:"receipt"`
}
type Mutation struct {
	CloseState    *CloseState `json:"closeState,omitempty"`
	StoreRevision int64       `json:"storeRevision"`
	Changed       bool        `json:"changed"`
}
type StateResult struct {
	OK    bool          `json:"ok"`
	Value *State        `json:"value"`
	Error *ServiceError `json:"error"`
}
type SpaceResult struct {
	OK    bool          `json:"ok"`
	Value *Space        `json:"value"`
	Error *ServiceError `json:"error"`
}
type SpaceListResult struct {
	OK    bool          `json:"ok"`
	Value *SpaceList    `json:"value"`
	Error *ServiceError `json:"error"`
}
type PageResult struct {
	OK    bool          `json:"ok"`
	Value *Page         `json:"value"`
	Error *ServiceError `json:"error"`
}
type PageListResult struct {
	OK    bool          `json:"ok"`
	Value *PageList     `json:"value"`
	Error *ServiceError `json:"error"`
}
type LinkResult struct {
	OK    bool              `json:"ok"`
	Value *ConversationLink `json:"value"`
	Error *ServiceError     `json:"error"`
}
type LinkListResult struct {
	OK    bool          `json:"ok"`
	Value *LinkList     `json:"value"`
	Error *ServiceError `json:"error"`
}
type DraftResult struct {
	OK    bool              `json:"ok"`
	Value *RecoverableDraft `json:"value"`
	Error *ServiceError     `json:"error"`
}
type DraftListResult struct {
	OK    bool          `json:"ok"`
	Value *DraftList    `json:"value"`
	Error *ServiceError `json:"error"`
}
type ReviewedResultResponse struct {
	OK    bool            `json:"ok"`
	Value *ReviewedResult `json:"value"`
	Error *ServiceError   `json:"error"`
}
type MutationResult struct {
	OK    bool          `json:"ok"`
	Value *Mutation     `json:"value"`
	Error *ServiceError `json:"error"`
}
type InitRequest struct{}
type ListSpacesRequest struct{}
type CreateSpaceRequest struct {
	Name string `json:"name"`
}
type PatchSpaceRequest struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	ExpectedRevision int64  `json:"expectedRevision"`
}
type DeleteSpaceRequest struct {
	ID               string `json:"id"`
	ExpectedRevision int64  `json:"expectedRevision"`
	ConfirmNonEmpty  bool   `json:"confirmNonEmpty"`
}
type ListPagesRequest struct {
	SpaceID string `json:"spaceId"`
}
type GetPageRequest struct {
	ID string `json:"id"`
}
type CreatePageRequest struct {
	SpaceID  string  `json:"spaceId"`
	ParentID *string `json:"parentId"`
	Title    string  `json:"title"`
	Content  string  `json:"content"`
}
type PatchPageRequest struct {
	ID               string  `json:"id"`
	ExpectedRevision int64   `json:"expectedRevision"`
	Title            *string `json:"title"`
	Content          *string `json:"content"`
	ParentID         *string `json:"parentId"`
	// Null and absent are identical when decoding into a pointer. SetParent must
	// be true to explicitly reparent to root; non-null ParentID always updates.
	SetParent bool `json:"setParent"`
}
type MovePageRequest struct {
	ID               string  `json:"id"`
	SpaceID          string  `json:"spaceId"`
	ParentID         *string `json:"parentId"`
	ExpectedRevision int64   `json:"expectedRevision"`
}
type DeletePageRequest struct {
	ID               string `json:"id"`
	ExpectedRevision int64  `json:"expectedRevision"`
}
type ListConversationLinksRequest struct {
	PageID string `json:"pageId"`
}
type LinkConversationRequest struct {
	PageID string `json:"pageId"`
	BotRef BotRef `json:"botRef"`
}
type GetReviewedResultRequest struct {
	ReceiptID string `json:"receiptId"`
}
type SaveReviewedResultRequest struct {
	ReceiptID        string    `json:"receiptId"`
	SourceRef        SourceRef `json:"sourceRef"`
	DraftDigest      string    `json:"draftDigest"`
	SpaceID          string    `json:"spaceId"`
	PageID           string    `json:"pageId"`
	ParentID         *string   `json:"parentId"`
	ExpectedRevision int64     `json:"expectedRevision"`
	Title            string    `json:"title"`
	Content          string    `json:"content"`
}
type ListRecoverableDraftsRequest struct {
	PageID string `json:"pageId"`
}
type SaveDraftRequest struct {
	PageID       string  `json:"pageId"`
	BaseRevision int64   `json:"baseRevision"`
	Title        string  `json:"title"`
	Content      string  `json:"content"`
	ParentID     *string `json:"parentId"`
	CapturedAt   int64   `json:"capturedAt"`
}
type DeleteDraftRequest struct {
	PageID     string `json:"pageId"`
	CapturedAt int64  `json:"capturedAt"`
}
type FlushRequest struct{}

type CloseState struct {
	SafeClose     bool               `json:"safeClose"`
	ReadOnly      bool               `json:"readOnly"`
	RecoveryCode  string             `json:"recoveryCode"`
	UnsavedDrafts []RecoverableDraft `json:"unsavedDrafts"`
	Actions       []string           `json:"actions"`
}
type CloseStateResult struct {
	OK    bool          `json:"ok"`
	Value *CloseState   `json:"value,omitempty"`
	Error *ServiceError `json:"error,omitempty"`
}
type CanCloseRequest struct{}
type AbandonUnsavedDraftRequest struct {
	PageID         string `json:"pageId"`
	CapturedAt     int64  `json:"capturedAt"`
	DraftDigest    string `json:"draftDigest"`
	ConfirmAbandon bool   `json:"confirmAbandon"`
}
