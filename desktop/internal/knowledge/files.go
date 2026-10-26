package knowledge

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo"
)

type Dialog interface {
	Open(context.Context, *mygo.Window) (string, error)
}
type NativeDialog struct{}

func (NativeDialog) Open(ctx context.Context, parent *mygo.Window) (string, error) {
	if parent == nil || ctx.Err() != nil {
		return "", errors.New("native selection unavailable")
	}
	paths, e := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: parent, Title: "Import session knowledge", ButtonLabel: "Import", Message: "Send the selected file as knowledge for this session. Maximum size: 20 MiB.", Multiple: false, Directory: false, Filters: []mygo.FileFilter{{Name: "Knowledge files", Extensions: []string{"txt", "md", "markdown", "json", "html", "htm", "pdf", "docx"}}}})
	if e != nil {
		return "", errors.New("native selection failed")
	}
	if len(paths) == 0 {
		return "", nil
	}
	if len(paths) != 1 {
		return "", errors.New("multiple selection")
	}
	return paths[0], nil
}
func (s *Service) ImportFile(ctx context.Context, r FileImportRequest) ImportResult {
	return s.importFile(ctx, r, false)
}
func (s *Service) AddFileVersion(ctx context.Context, r FileImportRequest) ImportResult {
	return s.importFile(ctx, r, true)
}

// ImportFileFromWindow is a trusted Go assembly helper, never an IPC method.
func ImportFileFromWindow(s *Service, ctx context.Context, parent *mygo.Window, r FileImportRequest, version bool) ImportResult {
	if parent == nil {
		return ImportResult{Error: fail("fileSelectionUnavailable"), IdempotencyKey: r.IdempotencyKey}
	}
	return s.importFileWindow(ctx, parent, r, version)
}
func (s *Service) importFile(ctx context.Context, r FileImportRequest, version bool) ImportResult {
	return s.importFileWindow(ctx, mygo.CallerWindow(ctx), r, version)
}
func (s *Service) importFileWindow(ctx context.Context, parent *mygo.Window, r FileImportRequest, version bool) ImportResult {
	result := ImportResult{IdempotencyKey: r.IdempotencyKey}
	if !validSession(r.ConnectionEpoch, r.SessionID) || !validKey(r.IdempotencyKey) || (r.DisplayName != "" && !validText(r.DisplayName, 512)) || (version && !validID(r.KnowledgeID)) || (!version && r.KnowledgeID != "") {
		result.Error = fail("invalidInput")
		return result
	}
	if e := s.guard(ctx, r.ConnectionEpoch); e != nil {
		result.Error = e
		return result
	}
	// Verify the bound business session before granting local file selection.
	var session struct {
		SessionID string `json:"sessionId"`
		ID        string `json:"id"`
	}
	if e := s.do(ctx, r.ConnectionEpoch, "GET", "/api/v1/sessions/"+r.SessionID, nil, &session); e != nil {
		result.Error = e
		return result
	}
	if session.SessionID != r.SessionID && session.ID != r.SessionID {
		result.Error = fail("invalidResponse")
		return result
	}
	if s.dialog == nil {
		result.Error = fail("fileSelectionUnavailable")
		return result
	}
	selected, e := s.dialog.Open(ctx, parent)
	if e != nil {
		result.Error = fail("fileSelectionUnavailable")
		return result
	}
	if e := s.guard(ctx, r.ConnectionEpoch); e != nil {
		result.Error = e
		return result
	}
	if selected == "" {
		result.Error = fail("cancelled")
		return result
	}
	body, contentType, err := selectedMultipart(ctx, selected, r.DisplayName)
	if err != nil {
		result.Error = fail("fileRejected")
		return result
	}
	if e := s.guard(ctx, r.ConnectionEpoch); e != nil {
		result.Error = e
		return result
	}
	path := base(r.SessionID) + "/imports/file"
	if version {
		path = base(r.SessionID) + "/" + r.KnowledgeID + "/versions/file"
	}
	return s.submit(ctx, r.ConnectionEpoch, path, r.KnowledgeID, r.IdempotencyKey, body, contentType)
}
func selectedMultipart(ctx context.Context, selected, displayName string) ([]byte, string, error) {
	if !filepath.IsAbs(selected) || strings.ContainsRune(selected, 0) {
		return nil, "", errors.New("invalid selection")
	}
	// Resolve the directory separately: parent symlinks may be normal on macOS,
	// but the selected leaf itself must not redirect to an unselected file.
	parent, e := filepath.EvalSymlinks(filepath.Dir(selected))
	if e != nil {
		return nil, "", e
	}
	selected = filepath.Join(parent, filepath.Base(selected))
	before, e := os.Lstat(selected)
	if e != nil || !before.Mode().IsRegular() || before.Size() < 1 || before.Size() > MaxFileBytes {
		return nil, "", errors.New("invalid selected file")
	}
	file, e := os.Open(selected)
	if e != nil {
		return nil, "", e
	}
	defer file.Close()
	opened, e := file.Stat()
	if e != nil || !os.SameFile(before, opened) || !opened.Mode().IsRegular() {
		return nil, "", errors.New("selected file changed")
	}
	data, e := io.ReadAll(io.LimitReader(file, MaxFileBytes+1))
	if e != nil || len(data) == 0 || len(data) > MaxFileBytes || ctx.Err() != nil {
		return nil, "", errors.New("file read failed")
	}
	after, e := file.Stat()
	leaf, e2 := os.Lstat(selected)
	if e != nil || e2 != nil || !leaf.Mode().IsRegular() || !os.SameFile(opened, leaf) || opened.Size() != after.Size() || opened.ModTime() != after.ModTime() || int64(len(data)) != after.Size() {
		return nil, "", errors.New("file changed")
	}
	name := filepath.Base(selected)
	if !validText(name, 512) || strings.ContainsAny(name, "\r\n\\") {
		return nil, "", errors.New("invalid name")
	}
	mediaType := fileMIME(name, data)
	if mediaType == "" {
		return nil, "", errors.New("unsupported file")
	}
	if displayName == "" {
		displayName = name
	}
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": name}))
	header.Set("Content-Type", mediaType)
	part, e := writer.CreatePart(header)
	if e != nil {
		return nil, "", e
	}
	if _, e = part.Write(data); e != nil {
		return nil, "", e
	}
	if e = writer.WriteField("display_name", displayName); e != nil {
		return nil, "", e
	}
	if e = writer.Close(); e != nil {
		return nil, "", e
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}
func fileMIME(name string, data []byte) string {
	ext := strings.ToLower(filepath.Ext(name))
	contentType := ""
	switch ext {
	case ".txt":
		contentType = "text/plain"
	case ".md", ".markdown":
		contentType = "text/markdown"
	case ".json":
		contentType = "application/json"
	case ".html", ".htm":
		contentType = "text/html"
	case ".pdf":
		if bytes.HasPrefix(data, []byte("%PDF-")) {
			return "application/pdf"
		}
		return ""
	case ".docx":
		if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
			return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		}
		return ""
	default:
		return ""
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return ""
	}
	return contentType
}
