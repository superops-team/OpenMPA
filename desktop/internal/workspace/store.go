package workspace

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

type Service struct {
	mu                sync.Mutex
	pathProvider      func() (string, error)
	path              string
	initialized       bool
	closed            bool
	readOnly          bool
	recoveryCode      string
	recoveryAvailable bool
	snapshot          Snapshot
	diskBytes         []byte
	lock              *os.File
	persist           func(string, []byte, []byte) error
	unsavedDrafts     map[string]RecoverableDraft
}

// New is deliberately inert: MyGo's binding generator constructs services.
// The provider is evaluated on Init/first call, after the native App is ready.
func New(dataPath func() (string, error)) *Service {
	return &Service{pathProvider: dataPath, persist: persistSnapshot}
}
func NewWithPath(path string) *Service { return New(func() (string, error) { return path, nil }) }

// Shutdown is native lifecycle only. It is a function so MyGo does not expose
// lock release as a bound Service method to the renderer.
func Shutdown(s *Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock != nil {
		releaseLock(s.lock)
		s.lock = nil
	}
	s.closed = true
}

func emptySnapshot() Snapshot {
	return Snapshot{SchemaVersion: SchemaVersion, Spaces: []Space{}, Pages: []Page{}, ConversationLinks: []ConversationLink{}, ReviewReceipts: []ReviewReceipt{}, RecoverableDrafts: []RecoverableDraft{}, DeletedPages: []DeletedPage{}}
}
func safeError(code string, status int, message string) *ServiceError {
	return &ServiceError{Code: code, Status: status, Message: message}
}
func conflict(p Page) *ServiceError {
	return &ServiceError{Code: "CONFLICT", Status: 409, Message: "This page changed. Your draft is preserved; load the current revision before saving.", CurrentPage: &p}
}
func validation() *ServiceError {
	return safeError("VALIDATION", 400, "The document input or hierarchy is invalid.")
}
func notFound() *ServiceError {
	return safeError("NOT_FOUND", 404, "The requested local document was not found.")
}

func (s *Service) initialize(ctx context.Context) *ServiceError {
	if err := ctx.Err(); err != nil {
		return safeError("CANCELLED", 499, "The operation was cancelled.")
	}
	if s.closed {
		return safeError("STORE_CLOSED", 503, "The workspace has closed.")
	}
	if s.initialized {
		return nil
	}
	if s.pathProvider == nil {
		return safeError("STORAGE_UNAVAILABLE", 503, "Workspace storage is unavailable.")
	}
	path, err := s.pathProvider()
	if err != nil || path == "" {
		return safeError("STORAGE_UNAVAILABLE", 503, "Workspace storage is unavailable.")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return safeError("STORAGE_UNAVAILABLE", 503, "Workspace storage is unavailable.")
	}
	s.path = path
	// A real directory and canonical path make aliases share the same OS lock.
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return safeError("STORAGE_UNAVAILABLE", 503, "Workspace storage is unavailable.")
	}
	directory, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return safeError("STORAGE_UNAVAILABLE", 503, "Workspace storage is unavailable.")
	}
	s.path = filepath.Join(directory, filepath.Base(path))
	if info, e := os.Lstat(s.path); e == nil && info.Mode()&os.ModeSymlink != 0 {
		return safeError("STORAGE_UNAVAILABLE", 503, "Workspace storage must be a regular local snapshot.")
	}
	lock, err := acquireLock(s.path + ".lock")
	if err != nil {
		s.readOnly = true
		s.recoveryCode = "STORE_LOCKED"
	} else {
		s.lock = lock
	}
	s.snapshot = emptySnapshot()
	data, readErr := readSnapshotFile(s.path)
	if errors.Is(readErr, os.ErrNotExist) {
		// A lost main snapshot with an existing backup is recovery, never a new DB.
		if backup, e := readSnapshotFile(s.path + ".bak"); e == nil {
			s.readOnly = true
			if s.recoveryCode == "" {
				s.recoveryCode = "MISSING_SNAPSHOT"
			}
			if parsed, code := decodeSnapshot(backup); code == "" {
				s.snapshot = parsed
				s.recoveryAvailable = true
			}
		} else if !errors.Is(e, os.ErrNotExist) {
			s.readOnly = true
			s.recoveryCode = "CORRUPT_SNAPSHOT"
		}
	} else if readErr != nil {
		s.readOnly = true
		if s.recoveryCode == "" {
			s.recoveryCode = "STORAGE_READ_FAILED"
		}
	} else {
		parsed, code := decodeSnapshot(data)
		if code == "" {
			s.snapshot = parsed
			s.diskBytes = data
		} else {
			s.readOnly = true
			if s.recoveryCode == "" {
				s.recoveryCode = code
			}
			if backup, e := readSnapshotFile(s.path + ".bak"); e == nil {
				if recovered, c := decodeSnapshot(backup); c == "" {
					s.snapshot = recovered
					s.recoveryAvailable = true
				}
			}
		}
	}
	s.initialized = true
	return nil
}

func (s *Service) ready(ctx context.Context, write bool) *ServiceError {
	if e := s.initialize(ctx); e != nil {
		return e
	}
	if write && s.readOnly {
		if s.recoveryCode == "STORE_LOCKED" {
			return safeError("STORE_LOCKED", 423, "Another application owns this workspace. This instance is read-only.")
		}
		return safeError("READ_ONLY_RECOVERY", 503, "The workspace requires recovery before changes can be saved.")
	}
	return nil
}

func cloneSnapshot(in Snapshot) Snapshot {
	// Every read and transaction gets independent pointers and slices: no caller
	// can mutate authority outside the mutex by changing a returned DTO.
	data, _ := json.Marshal(in)
	var out Snapshot
	_ = json.Unmarshal(data, &out)
	return out
}

func (s *Service) commit(ctx context.Context, next Snapshot) *ServiceError {
	if err := ctx.Err(); err != nil {
		return safeError("CANCELLED", 499, "The operation was cancelled.")
	}
	next.StoreRevision = s.snapshot.StoreRevision + 1
	if code := validateSnapshot(next); code != "" {
		return validation()
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return safeError("STORAGE_WRITE_FAILED", 503, "The workspace could not be saved. Your draft is preserved.")
	}
	if err = s.persist(s.path, append(data, '\n'), s.diskBytes); err != nil {
		return safeError("STORAGE_WRITE_FAILED", 503, "The workspace could not be saved. Your draft is preserved.")
	}
	s.snapshot = next
	for id, draft := range s.unsavedDrafts {
		durable := false
		for _, page := range next.Pages {
			if page.ID == id && page.Title == draft.Title && page.Content == draft.Content && sameParent(page.ParentID, draft.ParentID) {
				durable = true
			}
		}
		for _, saved := range next.RecoverableDrafts {
			if sameDraft(saved, draft) {
				durable = true
			}
		}
		if durable {
			delete(s.unsavedDrafts, id)
		}
	}
	s.diskBytes = append(data, '\n')
	return nil
}

func readSnapshotFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("not regular")
	}
	const maxSize = 128 << 20
	if info.Size() > maxSize {
		return nil, errors.New("snapshot too large")
	}
	return io.ReadAll(io.LimitReader(file, maxSize+1))
}

func decodeSnapshot(data []byte) (Snapshot, string) {
	var header struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if json.Unmarshal(data, &header) != nil {
		return Snapshot{}, "CORRUPT_SNAPSHOT"
	}
	if header.SchemaVersion > SchemaVersion {
		return Snapshot{}, "FUTURE_SCHEMA"
	}
	if header.SchemaVersion != SchemaVersion {
		return Snapshot{}, "UNSUPPORTED_SCHEMA"
	}
	var snap Snapshot
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&snap) != nil {
		return Snapshot{}, "CORRUPT_SNAPSHOT"
	}
	if validateSnapshot(snap) != "" {
		return Snapshot{}, "CORRUPT_SNAPSHOT"
	}
	return snap, ""
}

func persistSnapshot(path string, next, previous []byte) error {
	if len(previous) > 0 {
		if _, code := decodeSnapshot(previous); code != "" {
			return errors.New("invalid previous snapshot")
		}
		if err := atomicReplace(path+".bak", previous); err != nil {
			return err
		}
	}
	return atomicReplace(path, next)
}

func atomicReplace(path string, data []byte) error {
	directory := filepath.Dir(path)
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer dir.Close()
	temp, err := os.CreateTemp(directory, ".workspace-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err = temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err = temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = syncDirectory(dir); err != nil {
		return err
	}
	// Rename is the commit point; all errors before it leave the prior file.
	// The post-rename sync is best effort because after the atomic commit a
	// failure cannot truthfully be reported as an unsaved prior revision.
	if err = os.Rename(tempName, path); err != nil {
		return err
	}
	_ = syncDirectory(dir)
	return nil
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto random unavailable")
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func now() int64                  { return time.Now().UnixMilli() }
func utf16Length(text string) int { return len(utf16.Encode([]rune(text))) }
func trimTitle(title string) string {
	// ECMAScript String.trim: BOM is whitespace, NEL is not. Matching the
	// editor here also avoids a title that the UI considers entirely blank.
	return strings.TrimFunc(title, func(c rune) bool {
		return c >= '\t' && c <= '\r' || c == ' ' || c == '\u00a0' || c == '\u1680' ||
			c >= '\u2000' && c <= '\u200a' || c == '\u2028' || c == '\u2029' ||
			c == '\u202f' || c == '\u205f' || c == '\u3000' || c == '\ufeff'
	})
}
func validText(title, content string) bool {
	return utf8.ValidString(title) && utf8.ValidString(content) && trimTitle(title) != "" && utf16Length(title) <= 160 && utf16Length(content) <= 100000
}
func safeRefPart(part string) bool {
	if part == "" || len(part) > 256 || !utf8.ValidString(part) {
		return false
	}
	for _, c := range part {
		if c <= 32 || c == 127 {
			return false
		}
	}
	// IDs/scopes are references, never credential-bearing URLs or headers.
	lower := strings.ToLower(part)
	return !strings.Contains(part, "://") && !strings.ContainsAny(part, "?&#") && !strings.HasPrefix(lower, "bearer")
}
func validSource(ref SourceRef) bool {
	return safeRefPart(ref.ConnectionScope) && safeRefPart(ref.SessionID) && safeRefPart(ref.InvocationID) && safeRefPart(ref.EventRef)
}
func validBot(ref BotRef) bool { return safeRefPart(ref.ConnectionScope) && safeRefPart(ref.SessionID) }

// DraftDigest is SHA-256 of UTF-8 JSON [title, content], with HTML escaping
// disabled. JS computes sha256(JSON.stringify([title, content])).
func DraftDigest(title, content string) string {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode([]string{title, content})
	// Go always escapes Unicode line separators; JSON.stringify emits them
	// literally. Skip escaped backslash pairs to preserve a literal "\\u2028".
	data := bytes.TrimSuffix(b.Bytes(), []byte{'\n'})
	js := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		if data[i] == '\\' && i+1 < len(data) {
			if bytes.HasPrefix(data[i:], []byte(`\u2028`)) {
				js = append(js, []byte("\u2028")...)
				i += 6
				continue
			}
			if bytes.HasPrefix(data[i:], []byte(`\u2029`)) {
				js = append(js, []byte("\u2029")...)
				i += 6
				continue
			}
			js = append(js, data[i], data[i+1])
			i += 2
			continue
		}
		js = append(js, data[i])
		i++
	}
	sum := sha256.Sum256(js)
	return hex.EncodeToString(sum[:])
}
func payloadDigest(req SaveReviewedResultRequest) string {
	data, _ := json.Marshal(req)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func pageIndex(snap Snapshot, id string) int {
	for i, p := range snap.Pages {
		if p.ID == id {
			return i
		}
	}
	return -1
}
func spaceIndex(snap Snapshot, id string) int {
	for i, s := range snap.Spaces {
		if s.ID == id {
			return i
		}
	}
	return -1
}
func validParent(snap Snapshot, id, space string, parent *string) bool {
	seen := map[string]bool{id: true}
	for parent != nil {
		if seen[*parent] {
			return false
		}
		seen[*parent] = true
		i := pageIndex(snap, *parent)
		if i < 0 || snap.Pages[i].SpaceID != space {
			return false
		}
		parent = snap.Pages[i].ParentID
	}
	return true
}

func validateSnapshot(snap Snapshot) string {
	if snap.SchemaVersion != SchemaVersion || snap.StoreRevision < 0 || snap.Spaces == nil || snap.Pages == nil || snap.ConversationLinks == nil || snap.ReviewReceipts == nil || snap.RecoverableDrafts == nil || snap.DeletedPages == nil {
		return "invalid"
	}
	spaces := map[string]bool{}
	pages := map[string]bool{}
	deleted := map[string]bool{}
	for _, s := range snap.Spaces {
		if !safeRefPart(s.ID) || spaces[s.ID] || !validText(s.Name, "") || s.Revision < 1 || s.CreatedAt < 0 || s.UpdatedAt < s.CreatedAt {
			return "invalid"
		}
		spaces[s.ID] = true
	}
	for _, p := range snap.Pages {
		if !safeRefPart(p.ID) || pages[p.ID] || !spaces[p.SpaceID] || !validText(p.Title, p.Content) || p.Revision < 1 || p.CreatedAt < 0 || p.UpdatedAt < p.CreatedAt || (p.SourceRef != nil && !validSource(*p.SourceRef)) {
			return "invalid"
		}
		pages[p.ID] = true
	}
	for _, p := range snap.Pages {
		if !validParent(snap, p.ID, p.SpaceID, p.ParentID) {
			return "invalid"
		}
	}
	for _, p := range snap.DeletedPages {
		if pages[p.Page.ID] || deleted[p.Page.ID] || !safeRefPart(p.Page.ID) || !validText(p.Page.Title, p.Page.Content) || p.DeletedAt < 0 || (p.Page.SourceRef != nil && !validSource(*p.Page.SourceRef)) {
			return "invalid"
		}
		deleted[p.Page.ID] = true
	}
	links := map[string]bool{}
	for _, l := range snap.ConversationLinks {
		key := l.PageID + "\x00" + l.BotRef.ConnectionScope + "\x00" + l.BotRef.SessionID
		if !validBot(l.BotRef) || links[key] || (!pages[l.PageID] && !deleted[l.PageID]) || (pages[l.PageID] && l.DeletedAt != nil) || (deleted[l.PageID] && l.DeletedAt == nil) {
			return "invalid"
		}
		links[key] = true
	}
	receipts := map[string]bool{}
	for _, r := range snap.ReviewReceipts {
		if !safeRefPart(r.ID) || receipts[r.ID] || !validSource(r.SourceRef) || len(r.DraftDigest) != 64 || len(r.PayloadDigest) != 64 || r.PageRevision < 1 || (!pages[r.PageID] && !deleted[r.PageID]) || r.PageDeleted != deleted[r.PageID] {
			return "invalid"
		}
		receipts[r.ID] = true
	}
	drafts := map[string]bool{}
	for _, d := range snap.RecoverableDrafts {
		if drafts[d.PageID] || (!pages[d.PageID] && !deleted[d.PageID]) || d.BaseRevision < 1 || d.CapturedAt < 1 || !utf8.ValidString(d.Title) || !utf8.ValidString(d.Content) || utf16Length(d.Title) > 160 || utf16Length(d.Content) > 100000 {
			return "invalid"
		}
		drafts[d.PageID] = true
	}
	return ""
}
