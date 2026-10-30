package artifact

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

// Connection supplies the currently bound runtime endpoint, authentication and
// owner scope. Artifact cannot construct a URL host or supply another identity.
// Session/path read authorization remains the server download route's decision.
type Connection interface {
	Current(uint64) bool
	Request(context.Context, uint64, string, string, []byte, http.Header) (*http.Response, *dto.SafeError)
}

// epochCommitter lets a connection hold its epoch lock across the final rename.
// Connections without it still fail closed at every transfer boundary.
type epochCommitter interface {
	WithCurrent(uint64, func() *dto.SafeError) *dto.SafeError
}

type Service struct {
	connection Connection
	dialog     Dialog
	files      Files
}

func New(connection Connection) *Service {
	return NewWithDependencies(connection, NativeDialog{}, OSFiles{})
}
func NewWithDependencies(connection Connection, dialog Dialog, files Files) *Service {
	return &Service{connection: connection, dialog: dialog, files: files}
}
func failure(code string) *dto.SafeError {
	retry := "never"
	if code == "staleConnection" {
		retry = "reload"
	}
	e := dto.Failure(code, "desktopArtifact", retry, 0)
	switch code {
	case "cancelled":
		e.Message = "Saving was cancelled."
	case "downloadInterrupted":
		e.Message = "The transfer did not complete. The selected file was preserved."
	case "saveFailed":
		e.Message = "The file could not be saved."
	case "saveUnavailable":
		e.Message = "A native save dialog is unavailable for this window."
	case "targetChanged":
		e.Message = "The selected file changed while saving. Choose the target again."
	}
	return e
}
func (s *Service) guard(ctx context.Context, epoch uint64) *dto.SafeError {
	if ctx.Err() != nil {
		return failure("cancelled")
	}
	if s.connection == nil {
		return failure("notConnected")
	}
	if !s.connection.Current(epoch) {
		return failure("staleConnection")
	}
	return nil
}

// Download remains pending until the save succeeds or fails. Closing its channel
// cancels only this transfer through MyGo's call context; it never aborts a run.
func (s *Service) Download(ctx context.Context, r DownloadRequest, progress *mygo.Channel[DownloadProgress]) DownloadResult {
	if progress == nil {
		return DownloadResult{Error: failure("invalidInput")}
	}
	return s.download(ctx, r, progress.Send)
}

// DownloadFromWindow binds the shipped transfer to its trusted native parent.
// It is a Go assembly helper and exposes no renderer-selected window or path grant.
func DownloadFromWindow(s *Service, ctx context.Context, parent *mygo.Window, r DownloadRequest, send func(DownloadProgress) error) DownloadResult {
	if parent == nil {
		return DownloadResult{Error: failure("saveUnavailable")}
	}
	return s.downloadWindow(ctx, parent, r, send)
}
func (s *Service) download(ctx context.Context, r DownloadRequest, send func(DownloadProgress) error) DownloadResult {
	return s.downloadWindow(ctx, mygo.CallerWindow(ctx), r, send)
}
func (s *Service) downloadWindow(ctx context.Context, parent *mygo.Window, r DownloadRequest, send func(DownloadProgress) error) DownloadResult {
	if r.ConnectionEpoch == 0 || !validSessionID(r.SessionID) || !validRemotePath(r.RemotePath) {
		return DownloadResult{Error: failure("invalidInput")}
	}
	if e := s.guard(ctx, r.ConnectionEpoch); e != nil {
		return DownloadResult{Error: e}
	}
	// A closed channel cannot authorize a completed save, even for an empty file.
	if send == nil || send(DownloadProgress{}) != nil {
		return DownloadResult{Error: failure("cancelled")}
	}
	// Read access is established by the bound server before asking for a local
	// target. It does not authorize a sandbox path: download checks that again.
	if e := s.verifySession(ctx, r); e != nil {
		return DownloadResult{Error: e}
	}
	target, old, e := s.chooseTargetWindow(ctx, parent, SaveOptions{Title: "Download artifact", SuggestedName: suggestedFilename(path.Base(r.RemotePath), "artifact")})
	if e != nil {
		return DownloadResult{Error: e}
	}
	if e = s.guard(ctx, r.ConnectionEpoch); e != nil {
		return DownloadResult{Error: e}
	}
	query := url.Values{"path": []string{r.RemotePath}}.Encode()
	remote := "/api/v1/sessions/" + url.PathEscape(r.SessionID) + "/files/download?" + query
	response, e := s.connection.Request(ctx, r.ConnectionEpoch, http.MethodGet, remote, nil, nil)
	if e != nil {
		if ctx.Err() != nil {
			e = failure("cancelled")
		}
		return DownloadResult{Error: e}
	}
	if response == nil || response.Body == nil {
		return DownloadResult{Error: failure("invalidResponse")}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return DownloadResult{Error: failure("downloadFailed")}
	}
	var total *int64
	if response.ContentLength >= 0 && !response.Uncompressed {
		length := response.ContentLength
		total = &length
	}
	guard := func() *dto.SafeError { return s.guard(ctx, r.ConnectionEpoch) }
	written, e := s.save(ctx, target, old, response.Body, total, guard, send, func(commit func() *dto.SafeError) *dto.SafeError {
		if locked, ok := s.connection.(epochCommitter); ok {
			return locked.WithCurrent(r.ConnectionEpoch, commit)
		}
		if e := guard(); e != nil {
			return e
		}
		return commit()
	})
	if e != nil {
		return DownloadResult{Error: e}
	}
	integrity := "unverified"
	if total != nil {
		integrity = "length-verified"
	}
	return DownloadResult{OK: true, Value: &DownloadReceipt{Saved: true, BytesWritten: written, Integrity: integrity}}
}

func (s *Service) verifySession(ctx context.Context, r DownloadRequest) *dto.SafeError {
	remote := "/api/v1/sessions/" + url.PathEscape(r.SessionID)
	response, e := s.connection.Request(ctx, r.ConnectionEpoch, http.MethodGet, remote, nil, nil)
	if e != nil {
		if ctx.Err() != nil {
			return failure("cancelled")
		}
		return e
	}
	if response == nil || response.Body == nil {
		return failure("invalidResponse")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return failure("invalidResponse")
	}
	var summary struct {
		ID        string `json:"id"`
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&summary); err != nil {
		if e := s.guard(ctx, r.ConnectionEpoch); e != nil {
			return e
		}
		return failure("invalidResponse")
	}
	if (summary.ID == "" && summary.SessionID == "") || summary.ID != "" && summary.ID != r.SessionID || summary.SessionID != "" && summary.SessionID != r.SessionID {
		return failure("invalidResponse")
	}
	return s.guard(ctx, r.ConnectionEpoch)
}

// ExportMarkdown saves exactly the text supplied by the page view. Text does not
// grant access to any local path; only the native Save dialog supplies a target.
func ExportMarkdownFromWindow(s *Service, ctx context.Context, parent *mygo.Window, r ExportMarkdownRequest) ExportResult {
	if parent == nil {
		return ExportResult{Error: failure("saveUnavailable")}
	}
	return s.exportMarkdownWindow(ctx, parent, r)
}
func (s *Service) ExportMarkdown(ctx context.Context, r ExportMarkdownRequest) ExportResult {
	return s.exportMarkdownWindow(ctx, mygo.CallerWindow(ctx), r)
}
func (s *Service) exportMarkdownWindow(ctx context.Context, parent *mygo.Window, r ExportMarkdownRequest) ExportResult {
	if !validPageText(r.Title, r.Content) {
		return ExportResult{Error: failure("invalidInput")}
	}
	target, old, e := s.chooseTargetWindow(ctx, parent, SaveOptions{Title: "Export Markdown", SuggestedName: suggestedFilename(r.Title, "page") + ".md", Markdown: true})
	if e != nil {
		return ExportResult{Error: e}
	}
	guard := func() *dto.SafeError {
		if ctx.Err() != nil {
			return failure("cancelled")
		}
		return nil
	}
	written, e := s.save(ctx, target, old, strings.NewReader(r.Content), nil, guard, nil, func(commit func() *dto.SafeError) *dto.SafeError { return commit() })
	if e != nil {
		return ExportResult{Error: e}
	}
	return ExportResult{OK: true, Value: &ExportReceipt{Saved: true, BytesWritten: written}}
}

func (s *Service) chooseTarget(ctx context.Context, options SaveOptions) (string, os.FileInfo, *dto.SafeError) {
	return s.chooseTargetWindow(ctx, mygo.CallerWindow(ctx), options)
}
func (s *Service) chooseTargetWindow(ctx context.Context, parent *mygo.Window, options SaveOptions) (string, os.FileInfo, *dto.SafeError) {
	if ctx.Err() != nil {
		return "", nil, failure("cancelled")
	}
	if s.dialog == nil || s.files == nil {
		return "", nil, failure("saveUnavailable")
	}
	target, err := s.dialog.Save(ctx, parent, options)
	if ctx.Err() != nil {
		return "", nil, failure("cancelled")
	}
	if err != nil {
		return "", nil, failure("saveUnavailable")
	}
	if target == "" {
		return "", nil, failure("cancelled")
	}
	if !filepath.IsAbs(target) || strings.ContainsRune(target, 0) {
		return "", nil, failure("saveFailed")
	}
	old, err := s.files.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return target, nil, nil
	}
	if err != nil || !old.Mode().IsRegular() {
		return "", nil, failure("saveFailed")
	}
	confirmed, err := s.dialog.ConfirmOverwrite(ctx, parent)
	if ctx.Err() != nil {
		return "", nil, failure("cancelled")
	}
	if err != nil {
		return "", nil, failure("saveUnavailable")
	}
	if !confirmed {
		return "", nil, failure("cancelled")
	}
	return target, old, nil
}

// save stages a private file alongside the granted target. Sync and Close must
// succeed before commit. Cleanup only touches the exact temp this call created.
func (s *Service) save(ctx context.Context, target string, old os.FileInfo, reader io.Reader, total *int64, guard func() *dto.SafeError, send func(DownloadProgress) error, commitEpoch func(func() *dto.SafeError) *dto.SafeError) (int64, *dto.SafeError) {
	if e := guard(); e != nil {
		return 0, e
	}
	temp, err := s.files.CreateTemp(filepath.Dir(target), ".openmpa-save-*")
	if err != nil {
		return 0, failure("saveFailed")
	}
	closed := false
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		_ = s.files.Remove(temp.Name())
	}()
	buffer := make([]byte, 64<<10)
	var written int64
	noProgress := 0
	for {
		if e := guard(); e != nil {
			return written, e
		}
		n, readErr := reader.Read(buffer)
		if n > 0 {
			noProgress = 0
			if e := guard(); e != nil {
				return written, e
			}
			if total != nil && written+int64(n) > *total {
				return written, failure("downloadInterrupted")
			}
			count, writeErr := temp.Write(buffer[:n])
			written += int64(count)
			if writeErr != nil || count != n {
				return written, failure("saveFailed")
			}
			if send != nil && send(DownloadProgress{BytesWritten: written, TotalBytes: total}) != nil {
				return written, failure("cancelled")
			}
		} else if readErr == nil {
			noProgress++
			if noProgress >= 100 {
				return written, failure("downloadInterrupted")
			}
		}
		if readErr != nil {
			if e := guard(); e != nil {
				return written, e
			}
			if readErr != io.EOF {
				return written, failure("downloadInterrupted")
			}
			break
		}
	}
	if total != nil && written != *total {
		return written, failure("downloadInterrupted")
	}
	if send != nil && send(DownloadProgress{BytesWritten: written, TotalBytes: total}) != nil {
		return written, failure("cancelled")
	}
	if err = temp.Sync(); err != nil {
		return written, failure("saveFailed")
	}
	if err = temp.Close(); err != nil {
		return written, failure("saveFailed")
	}
	closed = true
	if e := guard(); e != nil {
		return written, e
	}
	e := commitEpoch(func() *dto.SafeError {
		if ctx.Err() != nil {
			return failure("cancelled")
		}
		current, statErr := s.files.Lstat(target)
		if old == nil {
			if !errors.Is(statErr, os.ErrNotExist) {
				return failure("targetChanged")
			}
			// Linking within one directory atomically claims a new name without
			// overwriting a file another process created after the dialog.
			if err := s.files.Link(temp.Name(), target); err != nil {
				if errors.Is(err, os.ErrExist) {
					return failure("targetChanged")
				}
				return failure("saveFailed")
			}
		} else {
			if statErr != nil || !current.Mode().IsRegular() || !os.SameFile(old, current) || old.Size() != current.Size() || !old.ModTime().Equal(current.ModTime()) {
				return failure("targetChanged")
			}
			if err := s.files.Rename(temp.Name(), target); err != nil {
				return failure("saveFailed")
			}
		}
		return nil
	})
	return written, e
}

func validSessionID(id string) bool {
	if id == "" || len(id) > 256 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func validRemotePath(remote string) bool {
	if remote == "" || len(remote) > 4096 || !utf8.ValidString(remote) || strings.Contains(remote, "://") || strings.HasPrefix(remote, "//") {
		return false
	}
	for _, r := range remote {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func utf16Length(text string) int {
	length := 0
	for _, r := range text {
		length++
		if r > 0xffff {
			length++
		}
	}
	return length
}
func trimTitle(title string) string {
	return strings.TrimFunc(title, func(c rune) bool {
		return c >= '\t' && c <= '\r' || c == ' ' || c == '\u00a0' || c == '\u1680' || c >= '\u2000' && c <= '\u200a' || c == '\u2028' || c == '\u2029' || c == '\u202f' || c == '\u205f' || c == '\u3000' || c == '\ufeff'
	})
}
func validPageText(title, content string) bool {
	return utf8.ValidString(title) && utf8.ValidString(content) && trimTitle(title) != "" && utf16Length(title) <= 160 && utf16Length(content) <= 100000
}
func suggestedFilename(value, fallback string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune("/\\:*?\"<>|", r) {
			return '_'
		}
		return r
	}, value)
	value = strings.Trim(value, " .")
	if value == "" {
		return fallback
	}
	// Keep the native panel's suggestion within a conservative filesystem byte
	// limit without splitting UTF-8; the user can still change its name.
	for len(value) > 180 {
		_, n := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-n]
	}
	return value
}
