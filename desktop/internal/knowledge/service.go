package knowledge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type Connection interface {
	Do(context.Context, uint64, string, string, interface{}, interface{}, string) *dto.SafeError
	Request(context.Context, uint64, string, string, []byte, http.Header) (*http.Response, *dto.SafeError)
	State() dto.ConnectionState
	Current(uint64) bool
}
type Service struct {
	conn   Connection
	dialog Dialog
}

func New(conn *connection.Client) *Service {
	if conn == nil {
		return &Service{dialog: NativeDialog{}}
	}
	return NewWithDependencies(conn, NativeDialog{})
}
func NewWithDependencies(conn Connection, dialog Dialog) *Service {
	return &Service{conn: conn, dialog: dialog}
}
func fail(code string) *dto.SafeError {
	retry := "never"
	if code == "invalidResponse" {
		retry = "verifyOnly"
	}
	if code == "staleConnection" {
		retry = "reload"
	}
	e := dto.Failure(code, "knowledge", retry, 0)
	switch code {
	case "fileSelectionUnavailable":
		e.Message = "A native file selection dialog is unavailable for this window."
	case "fileRejected":
		e.Message = "Select a supported regular file of at most 20 MiB."
	case "cancelled":
		e.Message = "Knowledge import was cancelled."
	}
	return e
}
func (s *Service) guard(ctx context.Context, epoch uint64) *dto.SafeError {
	if ctx.Err() != nil {
		return fail("cancelled")
	}
	if s.conn == nil || !s.conn.State().Connected {
		return fail("notConnected")
	}
	if !s.conn.Current(epoch) {
		return fail("staleConnection")
	}
	return nil
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var refPattern = regexp.MustCompile(`^knref_[0-9a-f]{24}$`)

func validID(v string) bool { return uuidPattern.MatchString(v) }
func validText(v string, limit int) bool {
	return utf8.ValidString(v) && utf8.RuneCountInString(v) <= limit && strings.TrimSpace(v) != "" && !strings.ContainsRune(v, 0)
}
func validKey(v string) bool {
	if !validText(v, 256) {
		return false
	}
	for _, r := range v {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}
func validSession(epoch uint64, id string) bool { return epoch != 0 && validID(id) }
func validVersion(v *int) bool                  { return v == nil || *v > 0 }
func base(session string) string                { return "/api/v1/sessions/" + url.PathEscape(session) + "/knowledge" }
func itemPath(session, id string, version *int) string {
	p := base(session) + "/" + url.PathEscape(id)
	if version != nil {
		p += "?version=" + strconv.Itoa(*version)
	}
	return p
}
func (s *Service) do(ctx context.Context, epoch uint64, method, path string, input, output interface{}) *dto.SafeError {
	if e := s.guard(ctx, epoch); e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if e := s.conn.Do(ctx, epoch, method, path, input, output, "session"); e != nil {
		return writeError(e, method)
	}
	if e := s.guard(ctx, epoch); e != nil {
		return writeError(e, method)
	}
	return nil
}
func writeError(e *dto.SafeError, method string) *dto.SafeError {
	out := *e
	if method != http.MethodGet && (e.HTTPStatus >= 500 || e.Code == "outcomeUnknown" || e.Code == "invalidResponse" || e.Code == "staleConnection" || e.Code == "cancelled") {
		out.RetryDisposition = "verifyOnly"
	}
	return &out
}
func validStatus(v string) bool {
	switch v {
	case "STAGING", "RECEIVED", "VALIDATING", "INGESTING", "READY", "FAILED", "DELETING", "DELETED":
		return true
	}
	return false
}
func source(v string) bool { return v == "text" || v == "file" || v == "url" }
func validItem(v *Item) bool {
	return validID(v.ImportID) && validID(v.KnowledgeID) && v.Version > 0 && source(v.SourceType) && validText(v.DisplayName, 512) && validStatus(v.Status) && v.ChunkCount >= 0
}
func cleanItem(v *Item) {
	v.SafeMessage = nil
	cleanDate(&v.CreatedAt)
	cleanDate(&v.UpdatedAt)
	if v.ErrorCode != nil {
		message, ok := knowledgeErrorMessage(*v.ErrorCode)
		if !ok {
			v.ErrorCode = nil
		} else {
			v.SafeMessage = &message
		}
	}
	if v.Checksum != nil {
		value := *v.Checksum
		if len(value) != 64 || strings.Trim(value, "0123456789abcdef") != "" {
			v.Checksum = nil
		}
	}
}
func knowledgeErrorMessage(v string) (string, bool) {
	switch v {
	case "SESSION_DELETED":
		return "The session is deleted.", true
	case "FILE_REJECTED":
		return "The file was rejected by the knowledge scanner.", true
	case "SOURCE_MISSING":
		return "The knowledge source is unavailable.", true
	case "KNOWLEDGE_INGEST_FAILED":
		return "Knowledge indexing failed.", true
	case "KNOWLEDGE_UNAVAILABLE":
		return "Knowledge is currently unavailable.", true
	case "URL_REJECTED":
		return "The knowledge URL was rejected.", true
	case "URL_INGEST_FAILED":
		return "URL knowledge indexing failed.", true
	case "EXTERNAL_OPERATION_UNSETTLED", "EXTERNAL_OPERATION_IDENTITY_MISSING", "EXTERNAL_OPERATION_CAPABILITY_MISSING", "EXTERNAL_OPERATION_RECEIPT_MISMATCH", "EXTERNAL_OPERATION_LOOKUP_UNAVAILABLE", "EXTERNAL_ABSENCE_CAPABILITY_MISSING", "EXTERNAL_DELETE_NOT_ABSENT", "EXTERNAL_OPERATION_PURGE_PENDING":
		return "The original knowledge operation is still being verified.", true
	}
	return "", false
}
func (s *Service) List(ctx context.Context, r SessionRequest) ListResult {
	if !validSession(r.ConnectionEpoch, r.SessionID) {
		return ListResult{Error: fail("invalidInput")}
	}
	var wire listWire
	if e := s.do(ctx, r.ConnectionEpoch, "GET", base(r.SessionID), nil, &wire); e != nil {
		return ListResult{Error: e}
	}
	if wire.Items == nil {
		return ListResult{Error: fail("invalidResponse")}
	}
	v := ItemPage{Items: make([]Item, len(wire.Items))}
	for i := range wire.Items {
		v.Items[i] = wire.Items[i].value
		if !validItem(&v.Items[i]) {
			return ListResult{Error: fail("invalidResponse")}
		}
		cleanItem(&v.Items[i])
	}
	return ListResult{OK: true, Value: &v}
}
func (s *Service) Read(ctx context.Context, r ItemRequest) ItemResult {
	if !validSession(r.ConnectionEpoch, r.SessionID) || !validID(r.KnowledgeID) || !validVersion(r.Version) {
		return ItemResult{Error: fail("invalidInput")}
	}
	var wire itemWire
	if e := s.do(ctx, r.ConnectionEpoch, "GET", itemPath(r.SessionID, r.KnowledgeID, r.Version), nil, &wire); e != nil {
		return ItemResult{Error: e}
	}
	v := wire.value
	if !validItem(&v) || v.KnowledgeID != r.KnowledgeID || (r.Version != nil && v.Version != *r.Version) {
		return ItemResult{Error: fail("invalidResponse")}
	}
	cleanItem(&v)
	return ItemResult{OK: true, Value: &v}
}

// ImportStatus performs exactly one backend read; it never schedules a timer or
// infers READY from a previous accepted import receipt.
func (s *Service) ImportStatus(ctx context.Context, r ImportStatusRequest) ItemResult {
	if !validSession(r.ConnectionEpoch, r.SessionID) || !validID(r.ImportID) {
		return ItemResult{Error: fail("invalidInput")}
	}
	var wire itemWire
	if e := s.do(ctx, r.ConnectionEpoch, "GET", base(r.SessionID)+"/imports/"+r.ImportID, nil, &wire); e != nil {
		return ItemResult{Error: e}
	}
	v := wire.value
	if !validItem(&v) || v.ImportID != r.ImportID {
		return ItemResult{Error: fail("invalidResponse")}
	}
	cleanItem(&v)
	return ItemResult{OK: true, Value: &v}
}

type textPayload struct {
	SourceType  string `json:"sourceType"`
	DisplayName string `json:"displayName"`
	Content     string `json:"content"`
}

func (s *Service) ImportText(ctx context.Context, r TextImportRequest) ImportResult {
	return s.importText(ctx, r, false)
}
func (s *Service) AddTextVersion(ctx context.Context, r TextImportRequest) ImportResult {
	return s.importText(ctx, r, true)
}
func (s *Service) importText(ctx context.Context, r TextImportRequest, version bool) ImportResult {
	result := ImportResult{IdempotencyKey: r.IdempotencyKey}
	if !validSession(r.ConnectionEpoch, r.SessionID) || !validKey(r.IdempotencyKey) || !validText(r.DisplayName, 512) || !validText(r.Content, MaxTextBytes) || len(r.Content) > MaxTextBytes || (version && !validID(r.KnowledgeID)) || (!version && r.KnowledgeID != "") {
		result.Error = fail("invalidInput")
		return result
	}
	path := base(r.SessionID) + "/imports"
	if version {
		path = base(r.SessionID) + "/" + r.KnowledgeID + "/versions"
	}
	body, _ := json.Marshal(textPayload{SourceType: "text", DisplayName: r.DisplayName, Content: r.Content})
	return s.submit(ctx, r.ConnectionEpoch, path, r.KnowledgeID, r.IdempotencyKey, body, "application/json")
}
func (s *Service) submit(ctx context.Context, epoch uint64, path, id, key string, body []byte, contentType string) ImportResult {
	result := ImportResult{IdempotencyKey: key}
	if e := s.guard(ctx, epoch); e != nil {
		result.Error = e
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, e := s.conn.Request(ctx, epoch, "POST", path, body, http.Header{"Idempotency-Key": {key}, "Content-Type": {contentType}})
	if e != nil {
		result.Error = writeError(e, "POST")
		return result
	}
	if resp == nil || resp.Body == nil {
		result.Error = fail("invalidResponse")
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode != 202 {
		result.Error = fail("invalidResponse")
		return result
	}
	var value ImportReceipt
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 64<<10))
	if decoder.Decode(&value) != nil || decoder.Decode(new(struct{})) != io.EOF || !validID(value.ImportID) || !validID(value.KnowledgeID) || value.Version < 1 || !validStatus(value.Status) || (id != "" && value.KnowledgeID != id) {
		result.Error = fail("invalidResponse")
		return result
	}
	if e = s.guard(ctx, epoch); e != nil {
		result.Error = writeError(e, "POST")
		return result
	}
	cleanDate(&value.CreatedAt)
	result.OK = true
	result.Value = &value
	return result
}

type searchPayload struct {
	Query        string   `json:"query"`
	KnowledgeIDs []string `json:"knowledgeIds"`
	SourceTypes  []string `json:"sourceTypes"`
	UpdatedAfter *string  `json:"updatedAfter,omitempty"`
	Limit        int      `json:"limit"`
	Cursor       *string  `json:"cursor,omitempty"`
}

func validCursor(v *string) bool {
	return v == nil || (validText(*v, 16384) && !strings.ContainsAny(*v, "\r\n"))
}
func complete(v string) bool { return v == "complete" || v == "bounded" }
func validCitation(c Citation, id string, version int) bool {
	return c.KnowledgeID == id && c.Version == version && refPattern.MatchString(c.Ref)
}
func (s *Service) Search(ctx context.Context, r SearchRequest) SearchResult {
	if r.Limit == 0 {
		r.Limit = 10
	}
	if !validSession(r.ConnectionEpoch, r.SessionID) || !validText(r.Query, 4096) || len(r.KnowledgeIDs) > 50 || len(r.SourceTypes) > 3 || r.Limit < 1 || r.Limit > 50 || !validCursor(r.Cursor) {
		return SearchResult{Error: fail("invalidInput")}
	}
	for _, id := range r.KnowledgeIDs {
		if !validID(id) {
			return SearchResult{Error: fail("invalidInput")}
		}
	}
	for _, v := range r.SourceTypes {
		if !source(v) {
			return SearchResult{Error: fail("invalidInput")}
		}
	}
	if r.UpdatedAfter != nil {
		if _, e := time.Parse(time.RFC3339Nano, *r.UpdatedAfter); e != nil {
			return SearchResult{Error: fail("invalidInput")}
		}
	}
	ids := r.KnowledgeIDs
	if ids == nil {
		ids = []string{}
	}
	types := r.SourceTypes
	if types == nil {
		types = []string{}
	}
	var v SearchPage
	if e := s.do(ctx, r.ConnectionEpoch, "POST", base(r.SessionID)+"/search", searchPayload{r.Query, ids, types, r.UpdatedAfter, r.Limit, r.Cursor}, &v); e != nil {
		return SearchResult{Error: e}
	}
	if v.Items == nil || !complete(v.Completeness) || !validCursor(v.NextCursor) || (v.Completeness == "complete" && v.NextCursor != nil) || len(v.Items) > r.Limit {
		return SearchResult{Error: fail("invalidResponse")}
	}
	for _, item := range v.Items {
		if !validID(item.KnowledgeID) || item.Version < 1 || !source(item.SourceType) || !validText(item.DisplayName, 512) || !utf8.ValidString(item.Content) || utf8.RuneCountInString(item.Content) > 8192 || !validCitation(item.Citation, item.KnowledgeID, item.Version) || item.Ref != item.Citation.Ref || !inFilter(item.KnowledgeID, r.KnowledgeIDs) || !inFilter(item.SourceType, r.SourceTypes) {
			return SearchResult{Error: fail("invalidResponse")}
		}
	}
	if v.BoundedReason != nil && *v.BoundedReason != "UPSTREAM_CURSOR_UNAVAILABLE" && *v.BoundedReason != "UPSTREAM_WINDOW_LIMIT" {
		v.BoundedReason = nil
	}
	return SearchResult{OK: true, Value: &v}
}
func inFilter(v string, values []string) bool {
	if len(values) == 0 {
		return true
	}
	for _, s := range values {
		if s == v {
			return true
		}
	}
	return false
}
func (s *Service) Content(ctx context.Context, r ContentRequest) ContentResult {
	if r.Limit == 0 {
		r.Limit = 8192
	}
	if !validSession(r.ConnectionEpoch, r.SessionID) || !validID(r.KnowledgeID) || !validVersion(r.Version) || r.Offset < 0 || r.Limit < 1 || r.Limit > 32768 || !validCursor(r.Cursor) {
		return ContentResult{Error: fail("invalidInput")}
	}
	q := url.Values{"offset": {strconv.Itoa(r.Offset)}, "limit": {strconv.Itoa(r.Limit)}}
	if r.Version != nil {
		q.Set("version", strconv.Itoa(*r.Version))
	}
	if r.Cursor != nil {
		q.Set("cursor", *r.Cursor)
	}
	var wire contentWire
	if e := s.do(ctx, r.ConnectionEpoch, "GET", base(r.SessionID)+"/"+r.KnowledgeID+"/content?"+q.Encode(), nil, &wire); e != nil {
		return ContentResult{Error: e}
	}
	v := wire.value
	if v.KnowledgeID != r.KnowledgeID || v.Version < 1 || (r.Version != nil && *r.Version != v.Version) || !utf8.ValidString(v.Content) || len(v.Content) > r.Limit || v.VisibleBytes != len(v.Content) || !complete(v.Completeness) || !validCursor(v.NextCursor) || (v.Completeness == "complete" && v.NextCursor != nil) || (v.Completeness == "bounded" && v.NextCursor == nil) || !validCitation(v.Citation, v.KnowledgeID, v.Version) {
		return ContentResult{Error: fail("invalidResponse")}
	}
	return ContentResult{OK: true, Value: &v}
}
func (s *Service) Delete(ctx context.Context, r ItemRequest) DeleteResult {
	if !validSession(r.ConnectionEpoch, r.SessionID) || !validID(r.KnowledgeID) || r.Version != nil {
		return DeleteResult{Error: fail("invalidInput")}
	}
	var v DeleteReceipt
	if e := s.do(ctx, r.ConnectionEpoch, "DELETE", itemPath(r.SessionID, r.KnowledgeID, nil), nil, &v); e != nil {
		return DeleteResult{Error: e}
	}
	if v.KnowledgeID != r.KnowledgeID || (v.Status != "DELETING" && v.Status != "DELETED") {
		return DeleteResult{Error: fail("invalidResponse")}
	}
	return DeleteResult{OK: true, Value: &v}
}
