package artifact

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/egoist/mygo"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type fixtureDialog struct {
	target          string
	confirm         bool
	err             error
	saves, confirms int
	onSave          func()
}

func (d *fixtureDialog) Save(_ context.Context, _ *mygo.Window, _ SaveOptions) (string, error) {
	d.saves++
	if d.onSave != nil {
		d.onSave()
	}
	return d.target, d.err
}
func (d *fixtureDialog) ConfirmOverwrite(_ context.Context, _ *mygo.Window) (bool, error) {
	d.confirms++
	return d.confirm, d.err
}

type fixtureConnection struct {
	current       bool
	response      *http.Response
	err           *dto.SafeError
	path, method  string
	calls         int
	downloadCalls int
	summaryBody   string
}

func (c *fixtureConnection) Current(uint64) bool { return c.current }
func (c *fixtureConnection) Request(_ context.Context, _ uint64, method, path string, _ []byte, _ http.Header) (*http.Response, *dto.SafeError) {
	c.calls++
	if strings.Contains(path, "/files/download?") {
		c.downloadCalls++
	} else if c.err == nil {
		body := c.summaryBody
		if body == "" {
			body = `{"id":"session-1"}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	}
	c.method = method
	c.path = path
	return c.response, c.err
}
func fixture(t *testing.T, body io.ReadCloser, length int64) (*Service, *fixtureConnection, *fixtureDialog) {
	t.Helper()
	d := &fixtureDialog{target: filepath.Join(t.TempDir(), "saved.md"), confirm: true}
	c := &fixtureConnection{current: true, response: &http.Response{StatusCode: 200, Body: body, ContentLength: length}}
	return NewWithDependencies(c, d, OSFiles{}), c, d
}
func request() DownloadRequest {
	return DownloadRequest{ConnectionEpoch: 1, SessionID: "session-1", RemotePath: "reports/report.md"}
}
func successfulProgress(DownloadProgress) error { return nil }
func assertOriginal(t *testing.T, d *fixtureDialog) {
	t.Helper()
	b, e := os.ReadFile(d.target)
	if e != nil || string(b) != "original" {
		t.Fatalf("original replaced: %q %v", b, e)
	}
	entries, e := os.ReadDir(filepath.Dir(d.target))
	if e != nil || len(entries) != 1 {
		t.Fatalf("temporary files leaked: %v %v", entries, e)
	}
}
func original(t *testing.T, d *fixtureDialog) {
	t.Helper()
	if e := os.WriteFile(d.target, []byte("original"), 0600); e != nil {
		t.Fatal(e)
	}
}

func TestDownloadEncodedPathAndUnknownIntegrity(t *testing.T) {
	s, c, d := fixture(t, io.NopCloser(strings.NewReader("complete")), -1)
	r := request()
	r.RemotePath = "/sandbox/报告 +#?&%..md"
	var progress []DownloadProgress
	result := s.download(context.Background(), r, func(p DownloadProgress) error { progress = append(progress, p); return nil })
	if !result.OK || result.Value.BytesWritten != 8 || result.Value.Integrity != "unverified" {
		t.Fatalf("result: %+v", result)
	}
	u, e := url.Parse(c.path)
	if e != nil || u.Path != "/api/v1/sessions/session-1/files/download" || u.Query().Get("path") != r.RemotePath || len(u.Query()) != 1 || c.method != "GET" {
		t.Fatalf("path changed: %s %v", c.path, e)
	}
	b, e := os.ReadFile(d.target)
	if e != nil || string(b) != "complete" || len(progress) == 0 {
		t.Fatalf("write/progress: %q %v %v", b, e, progress)
	}
	if progress[len(progress)-1].BytesWritten != 8 || progress[len(progress)-1].TotalBytes != nil {
		t.Fatalf("progress: %+v", progress)
	}
}
func TestDownloadVerifiedLengthAndOverwrite(t *testing.T) {
	s, _, d := fixture(t, io.NopCloser(strings.NewReader("complete")), 8)
	original(t, d)
	r := s.download(context.Background(), request(), successfulProgress)
	if !r.OK || r.Value.Integrity != "length-verified" || d.confirms != 1 {
		t.Fatalf("result=%+v confirms=%d", r, d.confirms)
	}
	b, _ := os.ReadFile(d.target)
	if string(b) != "complete" {
		t.Fatalf("%q", b)
	}
}

type brokenBody struct{ sent bool }

func (b *brokenBody) Read(p []byte) (int, error) {
	if !b.sent {
		b.sent = true
		return copy(p, "partial"), nil
	}
	return 0, errors.New("secret remote host/token")
}
func (*brokenBody) Close() error { return nil }
func TestDownloadInterruptedPreservesOriginal(t *testing.T) {
	s, _, d := fixture(t, &brokenBody{}, -1)
	original(t, d)
	r := s.download(context.Background(), request(), successfulProgress)
	if r.OK || r.Error == nil || r.Error.Code != "downloadInterrupted" {
		t.Fatalf("%+v", r)
	}
	assertOriginal(t, d)
	wire, _ := json.Marshal(r)
	if strings.Contains(string(wire), "secret") || strings.Contains(string(wire), d.target) {
		t.Fatalf("leak: %s", wire)
	}
}
func TestDownloadLengthMismatchPreservesOriginal(t *testing.T) {
	for _, length := range []int64{3, 100} {
		t.Run(strconv.FormatInt(length, 10), func(t *testing.T) {
			s, _, d := fixture(t, io.NopCloser(strings.NewReader("complete")), length)
			original(t, d)
			r := s.download(context.Background(), request(), successfulProgress)
			if r.OK || r.Error.Code != "downloadInterrupted" {
				t.Fatalf("%+v", r)
			}
			assertOriginal(t, d)
		})
	}
}
func TestDownloadCancelAndChannelClosePreserveOriginal(t *testing.T) {
	for _, kind := range []string{"context", "channel"} {
		t.Run(kind, func(t *testing.T) {
			s, _, d := fixture(t, io.NopCloser(strings.NewReader(strings.Repeat("x", 100000))), -1)
			original(t, d)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := s.download(ctx, request(), func(p DownloadProgress) error {
				if p.BytesWritten > 0 {
					if kind == "context" {
						cancel()
						return nil
					}
					return mygo.ErrChannelClosed
				}
				return nil
			})
			if r.OK || r.Error.Code != "cancelled" {
				t.Fatalf("%+v", r)
			}
			assertOriginal(t, d)
		})
	}
}
func TestDownloadDialogCancelOrRejectedOverwriteDoesNotFetch(t *testing.T) {
	for _, kind := range []string{"save", "overwrite"} {
		t.Run(kind, func(t *testing.T) {
			s, c, d := fixture(t, io.NopCloser(strings.NewReader("complete")), 8)
			original(t, d)
			if kind == "save" {
				d.target = ""
			} else {
				d.confirm = false
			}
			r := s.download(context.Background(), request(), successfulProgress)
			if r.OK || r.Error.Code != "cancelled" || c.downloadCalls != 0 {
				t.Fatalf("%+v calls=%d", r, c.calls)
			}
			if kind == "overwrite" {
				assertOriginal(t, d)
			}
		})
	}
}
func TestDownloadCrossEpochBeforeDuringAndAtEnd(t *testing.T) {
	for _, kind := range []string{"before", "dialog", "stream"} {
		t.Run(kind, func(t *testing.T) {
			s, c, d := fixture(t, io.NopCloser(strings.NewReader("complete")), 8)
			original(t, d)
			if kind == "before" {
				c.current = false
			}
			if kind == "dialog" {
				d.onSave = func() { c.current = false }
			}
			r := s.download(context.Background(), request(), func(p DownloadProgress) error {
				if kind == "stream" && p.BytesWritten > 0 {
					c.current = false
				}
				return nil
			})
			if r.OK || r.Error.Code != "staleConnection" {
				t.Fatalf("%+v", r)
			}
			assertOriginal(t, d)
			if kind == "before" && d.saves != 0 {
				t.Fatal("stale request opened dialog")
			}
		})
	}
}
func TestDownloadInvalidInputAndDeniedResponse(t *testing.T) {
	for _, path := range []string{"", "https://evil.example/report.md", "file:///etc/passwd", "name\x00md", "name\nmd", strings.Repeat("x", 4097)} {
		s, c, d := fixture(t, io.NopCloser(strings.NewReader("ignored")), 7)
		r := request()
		r.RemotePath = path
		result := s.download(context.Background(), r, successfulProgress)
		if result.OK || result.Error.Code != "invalidInput" || d.saves != 0 || c.calls != 0 {
			t.Fatalf("invalid accepted: %+v", result)
		}
	}
	s, c, d := fixture(t, io.NopCloser(strings.NewReader("ignored")), 7)
	original(t, d)
	c.err = dto.Failure("denied", "mpa", "never", 403)
	r := s.download(context.Background(), request(), successfulProgress)
	if r.OK || r.Error.Code != "denied" || d.saves != 0 {
		t.Fatalf("%+v", r)
	}
	assertOriginal(t, d)
}

func TestDownloadSummaryMismatchDoesNotOpenDialogOrDownload(t *testing.T) {
	for _, summary := range []string{`{}`, `not json`, `{"id":"different-session"}`, `{"id":"session-1","sessionId":"other"}`} {
		s, c, d := fixture(t, io.NopCloser(strings.NewReader("body")), 4)
		c.summaryBody = summary
		r := s.download(context.Background(), request(), successfulProgress)
		if r.OK || r.Error.Code != "invalidResponse" || d.saves != 0 || c.downloadCalls != 0 {
			t.Fatalf("%+v", r)
		}
	}
	s, c, _ := fixture(t, io.NopCloser(strings.NewReader("body")), 4)
	c.summaryBody = `{"sessionId":"session-1"}`
	if r := s.download(context.Background(), request(), successfulProgress); !r.OK {
		t.Fatalf("documented sessionId alias rejected: %+v", r)
	}
}
func TestDownloadTargetAppearingDuringStreamIsPreserved(t *testing.T) {
	s, _, d := fixture(t, io.NopCloser(strings.NewReader("complete")), 8)
	r := s.download(context.Background(), request(), func(p DownloadProgress) error {
		if p.BytesWritten > 0 {
			original(t, d)
		}
		return nil
	})
	if r.OK || r.Error.Code != "targetChanged" {
		t.Fatalf("%+v", r)
	}
	assertOriginal(t, d)
}
func TestDownloadClosedChannelCannotSave(t *testing.T) {
	s, _, d := fixture(t, io.NopCloser(strings.NewReader("complete")), 8)
	original(t, d)
	r := s.Download(context.Background(), request(), &mygo.Channel[DownloadProgress]{})
	if r.OK || r.Error.Code != "cancelled" {
		t.Fatalf("%+v", r)
	}
	assertOriginal(t, d)
}
func TestExportFullUTF8Content(t *testing.T) {
	s, _, d := fixture(t, nil, 0)
	content := "# 标题\n\n完整正文 😀\n"
	r := s.ExportMarkdown(context.Background(), ExportMarkdownRequest{Title: "page", Content: content})
	if !r.OK || r.Value.BytesWritten != int64(len(content)) {
		t.Fatalf("%+v", r)
	}
	b, _ := os.ReadFile(d.target)
	if string(b) != content {
		t.Fatalf("%q", b)
	}
	wire, _ := json.Marshal(r)
	if strings.Contains(string(wire), d.target) {
		t.Fatal("local path leaked")
	}
}
func TestExportValidationMatchesM04(t *testing.T) {
	for _, r := range []ExportMarkdownRequest{{Title: "\ufeff ", Content: "x"}, {Title: "ok", Content: strings.Repeat("😀", 50001)}, {Title: strings.Repeat("😀", 81)}, {Title: "ok", Content: string([]byte{0xff})}} {
		s, _, d := fixture(t, nil, 0)
		result := s.ExportMarkdown(context.Background(), r)
		if result.OK || result.Error.Code != "invalidInput" || d.saves != 0 {
			t.Fatalf("%+v", result)
		}
	}
}
func TestExportCancelPreservesOriginal(t *testing.T) {
	s, _, d := fixture(t, nil, 0)
	original(t, d)
	d.confirm = false
	r := s.ExportMarkdown(context.Background(), ExportMarkdownRequest{Title: "ok", Content: "new"})
	if r.OK || r.Error.Code != "cancelled" {
		t.Fatalf("%+v", r)
	}
	assertOriginal(t, d)
}

type failingFiles struct {
	OSFiles
	fail string
}
type failingTemp struct {
	TempFile
	fail string
}

func (f failingTemp) Sync() error {
	if f.fail == "sync" {
		return errors.New("secret filesystem path")
	}
	return f.TempFile.Sync()
}
func (f failingTemp) Write(p []byte) (int, error) {
	if f.fail == "write" {
		return 0, errors.New("secret filesystem path")
	}
	return f.TempFile.Write(p)
}
func (f failingTemp) Close() error {
	if f.fail == "close" {
		_ = f.TempFile.Close()
		return errors.New("secret filesystem path")
	}
	return f.TempFile.Close()
}
func (f failingFiles) CreateTemp(dir, pattern string) (TempFile, error) {
	if f.fail == "create" {
		return nil, errors.New("secret filesystem path")
	}
	t, e := f.OSFiles.CreateTemp(dir, pattern)
	if (f.fail == "sync" || f.fail == "write" || f.fail == "close") && e == nil {
		return failingTemp{t, f.fail}, nil
	}
	return t, e
}
func (f failingFiles) Rename(old, new string) error {
	if f.fail == "rename" {
		return errors.New("secret filesystem path")
	}
	return f.OSFiles.Rename(old, new)
}
func (f failingFiles) Link(old, new string) error {
	if f.fail == "link" {
		return errors.New("hard links unsupported")
	}
	return f.OSFiles.Link(old, new)
}
func TestDownloadDiskFailuresPreserveOriginal(t *testing.T) {
	for _, kind := range []string{"create", "write", "sync", "close", "rename"} {
		t.Run(kind, func(t *testing.T) {
			s, _, d := fixture(t, io.NopCloser(strings.NewReader("complete")), 8)
			original(t, d)
			s.files = failingFiles{fail: kind}
			r := s.download(context.Background(), request(), successfulProgress)
			if r.OK || r.Error.Code != "saveFailed" {
				t.Fatalf("%+v", r)
			}
			assertOriginal(t, d)
			wire, _ := json.Marshal(r)
			if strings.Contains(string(wire), "secret") {
				t.Fatalf("%s", wire)
			}
		})
	}
}

func TestNativeDialogRequiresCallerWindow(t *testing.T) {
	s := New(nil)
	r := s.ExportMarkdown(context.Background(), ExportMarkdownRequest{Title: "page", Content: "body"})
	if r.OK || r.Error.Code != "saveUnavailable" {
		t.Fatalf("%+v", r)
	}
}
func TestUnsupportedHardLinkFailsSafely(t *testing.T) {
	s, _, d := fixture(t, io.NopCloser(strings.NewReader("complete")), 8)
	s.files = failingFiles{fail: "link"}
	r := s.download(context.Background(), request(), successfulProgress)
	if r.OK || r.Error.Code != "saveFailed" {
		t.Fatalf("%+v", r)
	}
	if _, e := os.Stat(d.target); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("failed publish created target")
	}
	entries, e := os.ReadDir(filepath.Dir(d.target))
	if e != nil || len(entries) != 0 {
		t.Fatalf("temporary files leaked: %v %v", entries, e)
	}
}
func TestTargetModifiedDuringTransferRemainsUnchanged(t *testing.T) {
	s, _, d := fixture(t, io.NopCloser(strings.NewReader("complete")), 8)
	original(t, d)
	r := s.download(context.Background(), request(), func(p DownloadProgress) error {
		if p.BytesWritten > 0 {
			if e := os.WriteFile(d.target, []byte("edited by another process"), 0600); e != nil {
				t.Fatal(e)
			}
		}
		return nil
	})
	if r.OK || r.Error.Code != "targetChanged" {
		t.Fatalf("%+v", r)
	}
	b, e := os.ReadFile(d.target)
	if e != nil || string(b) != "edited by another process" {
		t.Fatalf("%q %v", b, e)
	}
}
func TestDownloadEmptyFileRequiresOpenChannel(t *testing.T) {
	s, _, d := fixture(t, io.NopCloser(strings.NewReader("")), 0)
	r := s.download(context.Background(), request(), successfulProgress)
	if !r.OK || r.Value.BytesWritten != 0 {
		t.Fatalf("%+v", r)
	}
	b, e := os.ReadFile(d.target)
	if e != nil || len(b) != 0 {
		t.Fatalf("%q %v", b, e)
	}
}

func TestExportFilesystemFailureAndCancelledContext(t *testing.T) {
	for _, kind := range []string{"write", "sync", "close", "rename", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			s, _, d := fixture(t, nil, 0)
			original(t, d)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "cancel" {
				d.onSave = cancel
			} else {
				s.files = failingFiles{fail: kind}
			}
			r := s.ExportMarkdown(ctx, ExportMarkdownRequest{Title: "page", Content: "body"})
			if r.OK || r.Error == nil {
				t.Fatalf("%+v", r)
			}
			assertOriginal(t, d)
		})
	}
}

func TestSaveRejectsSymlinkAndRelativeTargets(t *testing.T) {
	for _, kind := range []string{"symlink", "directory", "relative"} {
		t.Run(kind, func(t *testing.T) {
			s, c, d := fixture(t, io.NopCloser(strings.NewReader("body")), 4)
			switch kind {
			case "symlink":
				originalPath := filepath.Join(filepath.Dir(d.target), "original.md")
				if e := os.WriteFile(originalPath, []byte("original"), 0600); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(originalPath, d.target); e != nil {
					t.Fatal(e)
				}
			case "directory":
				if e := os.Mkdir(d.target, 0700); e != nil {
					t.Fatal(e)
				}
			case "relative":
				d.target = "relative.md"
			}
			r := s.download(context.Background(), request(), successfulProgress)
			if r.OK || r.Error.Code != "saveFailed" || c.downloadCalls != 0 {
				t.Fatalf("%+v calls=%d", r, c.calls)
			}
		})
	}
}
