// Package knowledge adapts Business Session knowledge. These imports are not
// executable sandbox attachments or local filesystem references.
package knowledge

import "github.com/superops-team/OpenMPA/desktop/internal/dto"

const MaxFileBytes = 20 * 1024 * 1024
const MaxTextBytes = 1024 * 1024

type SessionRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SessionID       string `json:"sessionId"`
}
type ItemRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SessionID       string `json:"sessionId"`
	KnowledgeID     string `json:"knowledgeId"`
	Version         *int   `json:"version,omitempty"`
}
type ImportStatusRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SessionID       string `json:"sessionId"`
	ImportID        string `json:"importId"`
}
type TextImportRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SessionID       string `json:"sessionId"`
	KnowledgeID     string `json:"knowledgeId,omitempty"`
	IdempotencyKey  string `json:"idempotencyKey"`
	DisplayName     string `json:"displayName"`
	Content         string `json:"content"`
}

// There is deliberately no local path or renderer MIME field. ImportFile and
// AddFileVersion ask the calling window to select one file for this submission.
type FileImportRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SessionID       string `json:"sessionId"`
	KnowledgeID     string `json:"knowledgeId,omitempty"`
	IdempotencyKey  string `json:"idempotencyKey"`
	DisplayName     string `json:"displayName,omitempty"`
}
type SearchRequest struct {
	ConnectionEpoch uint64   `json:"connectionEpoch"`
	SessionID       string   `json:"sessionId"`
	Query           string   `json:"query"`
	KnowledgeIDs    []string `json:"knowledgeIds"`
	SourceTypes     []string `json:"sourceTypes"`
	UpdatedAfter    *string  `json:"updatedAfter,omitempty"`
	Limit           int      `json:"limit,omitempty"`
	Cursor          *string  `json:"cursor,omitempty"`
}
type ContentRequest struct {
	ConnectionEpoch uint64  `json:"connectionEpoch"`
	SessionID       string  `json:"sessionId"`
	KnowledgeID     string  `json:"knowledgeId"`
	Version         *int    `json:"version,omitempty"`
	Offset          int     `json:"offset,omitempty"`
	Limit           int     `json:"limit,omitempty"`
	Cursor          *string `json:"cursor,omitempty"`
}
type ImportReceipt struct {
	ImportID    string  `json:"importId"`
	KnowledgeID string  `json:"knowledgeId"`
	Version     int     `json:"version"`
	Status      string  `json:"status"`
	CreatedAt   *string `json:"createdAt"`
}
type Item struct {
	ImportID    string  `json:"importId"`
	KnowledgeID string  `json:"knowledgeId"`
	Version     int     `json:"version"`
	SourceType  string  `json:"sourceType"`
	DisplayName string  `json:"displayName"`
	Status      string  `json:"status"`
	ChunkCount  int     `json:"chunkCount"`
	Checksum    *string `json:"checksum"`
	Active      bool    `json:"active"`
	Retryable   bool    `json:"retryable"`
	ErrorCode   *string `json:"errorCode"`
	// SafeMessage is synthesized from known error codes; upstream exception
	// messages are excluded even when a backend calls them safeMessage.
	SafeMessage *string `json:"safeMessage"`
	CreatedAt   *string `json:"createdAt"`
	UpdatedAt   *string `json:"updatedAt"`
}
type ItemPage struct {
	Items []Item `json:"items"`
}
type Citation struct {
	Ref         string `json:"ref"`
	KnowledgeID string `json:"knowledgeId"`
	Version     int    `json:"version"`
}
type SearchItem struct {
	Ref         string   `json:"ref"`
	KnowledgeID string   `json:"knowledgeId"`
	Version     int      `json:"version"`
	SourceType  string   `json:"sourceType"`
	DisplayName string   `json:"displayName"`
	Content     string   `json:"content"`
	Score       float64  `json:"score"`
	Citation    Citation `json:"citation"`
}
type SearchPage struct {
	Items         []SearchItem `json:"items"`
	Completeness  string       `json:"completeness"`
	NextCursor    *string      `json:"nextCursor"`
	BoundedReason *string      `json:"boundedReason"`
}
type ContentPage struct {
	KnowledgeID  string   `json:"knowledgeId"`
	Version      int      `json:"version"`
	Content      string   `json:"content"`
	VisibleBytes int      `json:"visibleBytes"`
	Completeness string   `json:"completeness"`
	NextCursor   *string  `json:"nextCursor"`
	Citation     Citation `json:"citation"`
}
type DeleteReceipt struct {
	KnowledgeID string `json:"knowledgeId"`
	Status      string `json:"status"`
}
type ListResult struct {
	OK    bool           `json:"ok"`
	Value *ItemPage      `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type ItemResult struct {
	OK    bool           `json:"ok"`
	Value *Item          `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type ImportResult struct {
	OK             bool           `json:"ok"`
	Value          *ImportReceipt `json:"value,omitempty"`
	Error          *dto.SafeError `json:"error,omitempty"`
	IdempotencyKey string         `json:"idempotencyKey"`
}
type SearchResult struct {
	OK    bool           `json:"ok"`
	Value *SearchPage    `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type ContentResult struct {
	OK    bool           `json:"ok"`
	Value *ContentPage   `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type DeleteResult struct {
	OK    bool           `json:"ok"`
	Value *DeleteReceipt `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
