package knowledge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

const sid = "cb7e2eac-8628-4d3a-86d1-0be6f8ab2c0a"
const kid = "f750d430-cd6f-4d8b-bae4-4b6b9a2dfd3f"
const iid = "b39d99ec-14c0-4c24-9682-8a55116c72e5"
const itemJSON = `{"importId":"` + iid + `","knowledgeId":"` + kid + `","version":1,"sourceType":"text","displayName":"Notes","status":"READY","chunkCount":1,"active":true,"retryable":false}`
const importJSON = `{"importId":"` + iid + `","knowledgeId":"` + kid + `","version":1,"status":"RECEIVED"}`

type fakeConn struct {
	current              bool
	calls                int
	method, path, family string
	input                []byte
	reply                string
	err                  *dto.SafeError
	after                func()
}

func (f *fakeConn) Current(uint64) bool        { return f.current }
func (f *fakeConn) State() dto.ConnectionState { return dto.ConnectionState{Connected: true} }
func (f *fakeConn) Do(_ context.Context, _ uint64, method, path string, input, output interface{}, family string) *dto.SafeError {
	f.calls++
	f.method = method
	f.path = path
	f.family = family
	f.input, _ = json.Marshal(input)
	if f.err != nil {
		return f.err
	}
	if f.after != nil {
		defer f.after()
	}
	if json.Unmarshal([]byte(f.reply), output) != nil {
		return dto.Failure("invalidResponse", "transport", "verifyOnly", 200)
	}
	return nil
}
func (f *fakeConn) Request(_ context.Context, _ uint64, method, path string, b []byte, h http.Header) (*http.Response, *dto.SafeError) {
	f.calls++
	f.method = method
	f.path = path
	f.input = b
	if f.err != nil {
		return nil, f.err
	}
	if f.after != nil {
		f.after()
	}
	return &http.Response{StatusCode: 202, Body: io.NopCloser(strings.NewReader(f.reply))}, nil
}

type fakeDialog struct {
	path  string
	err   error
	calls int
	after func()
}

func (d *fakeDialog) Open(context.Context, *mygo.Window) (string, error) {
	d.calls++
	if d.after != nil {
		d.after()
	}
	return d.path, d.err
}

func TestListReadStatusContracts(t *testing.T) {
	f := &fakeConn{current: true, reply: `{"items":[` + itemJSON + `]}`}
	s := NewWithDependencies(f, &fakeDialog{})
	r := s.List(context.Background(), SessionRequest{ConnectionEpoch: 1, SessionID: sid})
	if !r.OK || len(r.Value.Items) != 1 || f.path != "/api/v1/sessions/"+sid+"/knowledge" || f.family != "session" {
		t.Fatalf("bad list %+v %s %s", r, f.path, f.family)
	}
	f.reply = itemJSON
	v := 2
	r2 := s.Read(context.Background(), ItemRequest{ConnectionEpoch: 1, SessionID: sid, KnowledgeID: kid, Version: &v})
	if r2.OK {
		t.Fatal("version mismatch accepted")
	}
	v = 1
	r2 = s.Read(context.Background(), ItemRequest{ConnectionEpoch: 1, SessionID: sid, KnowledgeID: kid, Version: &v})
	if !r2.OK || !strings.HasSuffix(f.path, "?version=1") {
		t.Fatal("read route")
	}
	status := s.ImportStatus(context.Background(), ImportStatusRequest{ConnectionEpoch: 1, SessionID: sid, ImportID: iid})
	if !status.OK || !strings.HasSuffix(f.path, "/imports/"+iid) {
		t.Fatal("status route")
	}
}
func TestMalformedItemNeverReady(t *testing.T) {
	for _, reply := range []string{`null`, `{}`, `{"items":null}`, `{"importId":"` + iid + `","knowledgeId":"` + kid + `","version":1,"sourceType":"text","status":"READY"}`, strings.Replace(itemJSON, `"READY"`, `"UNKNOWN"`, 1), strings.Replace(itemJSON, kid, sid, 1)} {
		t.Run(reply, func(t *testing.T) {
			f := &fakeConn{current: true, reply: reply}
			s := NewWithDependencies(f, &fakeDialog{})
			r := s.Read(context.Background(), ItemRequest{ConnectionEpoch: 1, SessionID: sid, KnowledgeID: kid})
			if r.OK || r.Value != nil || r.Error == nil {
				t.Fatal("malformed READY accepted")
			}
		})
	}
}
func TestEpochSwitchAndErrors(t *testing.T) {
	f := &fakeConn{current: true, reply: itemJSON}
	f.after = func() { f.current = false }
	s := NewWithDependencies(f, &fakeDialog{})
	r := s.Read(context.Background(), ItemRequest{ConnectionEpoch: 1, SessionID: sid, KnowledgeID: kid})
	if r.OK || r.Error.Code != "staleConnection" {
		t.Fatal("stale response")
	}
	for _, status := range []int{401, 403, 404, 409, 422, 503} {
		f.current = true
		f.after = nil
		f.err = dto.Failure("requestFailed", "runtimeIngress", "never", status)
		r = s.Read(context.Background(), ItemRequest{ConnectionEpoch: 1, SessionID: sid, KnowledgeID: kid})
		if r.OK || r.Error.HTTPStatus != status {
			t.Fatal("error lost")
		}
	}
}
func TestTextImportBodyIdempotencyAndNoCAS(t *testing.T) {
	f := &fakeConn{current: true, reply: importJSON}
	s := NewWithDependencies(f, &fakeDialog{})
	q := TextImportRequest{ConnectionEpoch: 1, SessionID: sid, IdempotencyKey: "same-key", DisplayName: "Notes", Content: "selected text"}
	r := s.ImportText(context.Background(), q)
	if !r.OK || r.IdempotencyKey != q.IdempotencyKey || f.path != "/api/v1/sessions/"+sid+"/knowledge/imports" {
		t.Fatalf("import %+v", r)
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(f.input, &body) != nil || len(body) != 3 || string(body["sourceType"]) != `"text"` || body["connectionEpoch"] != nil || body["expectedVersion"] != nil {
		t.Fatalf("wrong schema %s", f.input)
	}
	q.KnowledgeID = kid
	r = s.AddTextVersion(context.Background(), q)
	if !r.OK || !strings.HasSuffix(f.path, "/"+kid+"/versions") {
		t.Fatal("version route")
	}
	f.err = dto.Failure("outcomeUnknown", "transport", "verifyOnly", 0)
	r = s.AddTextVersion(context.Background(), q)
	if r.OK || r.IdempotencyKey != "same-key" || r.Error.RetryDisposition != "verifyOnly" {
		t.Fatal("recovery key lost")
	}
}
func TestInvalidInputsNoRequest(t *testing.T) {
	f := &fakeConn{current: true, reply: importJSON}
	s := NewWithDependencies(f, &fakeDialog{})
	q := TextImportRequest{ConnectionEpoch: 1, SessionID: sid, IdempotencyKey: "key", DisplayName: "Notes", Content: "text"}
	tests := []func(){func() { q.Content = "  "; s.ImportText(context.Background(), q) }, func() { q.Content = strings.Repeat("x", MaxTextBytes+1); s.ImportText(context.Background(), q) }, func() { q.Content = "text"; q.IdempotencyKey = "key\r\nsecret"; s.ImportText(context.Background(), q) }, func() { s.List(context.Background(), SessionRequest{ConnectionEpoch: 1, SessionID: "../secret"}) }, func() {
		s.Delete(context.Background(), ItemRequest{ConnectionEpoch: 1, SessionID: sid, KnowledgeID: kid, Version: new(int)})
	}}
	for _, fn := range tests {
		fn()
	}
	if f.calls != 0 {
		t.Fatal("invalid request sent")
	}
}
func TestSearchContentScopeAndCompleteness(t *testing.T) {
	f := &fakeConn{current: true, reply: `{"items":[],"completeness":"bounded","nextCursor":"opaque","boundedReason":"UPSTREAM_WINDOW_LIMIT"}`}
	s := NewWithDependencies(f, &fakeDialog{})
	cursor := "cursor"
	r := s.Search(context.Background(), SearchRequest{ConnectionEpoch: 1, SessionID: sid, Query: "notes", KnowledgeIDs: []string{kid}, SourceTypes: []string{"file"}, Cursor: &cursor, Limit: 5})
	if !r.OK || r.Value.Completeness != "bounded" || *r.Value.NextCursor != "opaque" || f.method != "POST" || !strings.HasSuffix(f.path, "/search") {
		t.Fatal("search")
	}
	f.reply = `{"knowledgeId":"` + kid + `","version":1,"content":"你好","visibleBytes":6,"completeness":"complete","citation":{"knowledgeId":"` + kid + `","version":1,"ref":"knref_012345678901234567890123"}}`
	c := s.Content(context.Background(), ContentRequest{ConnectionEpoch: 1, SessionID: sid, KnowledgeID: kid, Limit: 8192, Cursor: &cursor})
	if !c.OK || c.Value.Content != "你好" || !strings.Contains(f.path, "cursor=cursor") {
		t.Fatal("content")
	}
	f.reply = strings.Replace(f.reply, `"visibleBytes":6`, `"visibleBytes":5`, 1)
	c = s.Content(context.Background(), ContentRequest{ConnectionEpoch: 1, SessionID: sid, KnowledgeID: kid})
	if c.OK {
		t.Fatal("invalid bytes")
	}
}
func TestDeleteRequiresExactReceipt(t *testing.T) {
	f := &fakeConn{current: true, reply: `{"knowledgeId":"` + kid + `","status":"DELETING"}`}
	s := NewWithDependencies(f, &fakeDialog{})
	q := ItemRequest{ConnectionEpoch: 1, SessionID: sid, KnowledgeID: kid}
	r := s.Delete(context.Background(), q)
	if !r.OK || r.Value.Status != "DELETING" {
		t.Fatal("deletion scope")
	}
	f.reply = `{}`
	if s.Delete(context.Background(), q).OK {
		t.Fatal("missing receipt")
	}
}
func TestNativeSelectionAndMultipart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.md")
	if e := os.WriteFile(path, []byte("# selected notes"), 0600); e != nil {
		t.Fatal(e)
	}
	var epoch uint64
	var got bool
	versionUpload := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			io.WriteString(w, `{"sessionId":"`+sid+`"}`)
			return
		}
		if versionUpload {
			if r.URL.Path != "/api/v1/sessions/"+sid+"/knowledge/"+kid+"/versions/file" {
				t.Error("file version route")
			}
		} else if r.URL.Path != "/api/v1/sessions/"+sid+"/knowledge/imports/file" {
			t.Error("file import route")
		}
		if r.Header.Get("Authorization") != "Bearer runtime-test" || r.Header.Get("X-User-Id") != "user-test" || r.Header.Get("Idempotency-Key") != "selected-key" {
			t.Error("auth/key lost")
		}
		if e := r.ParseMultipartForm(MaxFileBytes + 4096); e != nil {
			t.Error(e)
		} else {
			defer r.MultipartForm.RemoveAll()
			file, h, e := r.FormFile("file")
			if e != nil {
				t.Error(e)
			} else {
				defer file.Close()
				data, _ := io.ReadAll(file)
				got = string(data) == "# selected notes" && h.Filename == "notes.md" && h.Header.Get("Content-Type") == "text/markdown" && r.FormValue("display_name") == "Notes"
			}
		}
		w.WriteHeader(202)
		io.WriteString(w, importJSON)
	}))
	defer server.Close()
	conn := connection.New()
	epoch, _ = conn.Bind(connection.Binding{Endpoint: server.URL, RuntimeKey: "runtime-test"}, true)
	conn.SetIdentity(epoch, "user-test")
	epoch = conn.State().ConnectionEpoch
	d := &fakeDialog{path: path}
	s := NewWithDependencies(conn, d)
	r := s.ImportFile(context.Background(), FileImportRequest{ConnectionEpoch: epoch, SessionID: sid, IdempotencyKey: "selected-key", DisplayName: "Notes"})
	if !r.OK || !got || d.calls != 1 {
		t.Fatalf("multipart %+v selected %v", r, got)
	}
	versionUpload = true
	r = s.AddFileVersion(context.Background(), FileImportRequest{ConnectionEpoch: epoch, SessionID: sid, KnowledgeID: kid, IdempotencyKey: "selected-key", DisplayName: "Notes"})
	if !r.OK || d.calls != 2 {
		t.Fatalf("native version %+v", r)
	}
}
func TestFileSelectionSafety(t *testing.T) {
	dir := t.TempDir()
	normal := filepath.Join(dir, "normal.txt")
	os.WriteFile(normal, []byte("user text"), 0600)
	symlink := filepath.Join(dir, "link.txt")
	os.Symlink(normal, symlink)
	large := filepath.Join(dir, "large.txt")
	file, _ := os.Create(large)
	file.Truncate(MaxFileBytes + 1)
	file.Close()
	for _, path := range []string{"", dir, symlink, large} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			f := &fakeConn{current: true, reply: `{"sessionId":"` + sid + `"}`}
			d := &fakeDialog{path: path}
			s := NewWithDependencies(f, d)
			r := s.ImportFile(context.Background(), FileImportRequest{ConnectionEpoch: 1, SessionID: sid, IdempotencyKey: "key"})
			if r.OK || f.method == "POST" {
				t.Fatal("unsafe file sent")
			}
		})
	}
	f := &fakeConn{current: true, reply: `{"sessionId":"` + sid + `"}`}
	d := &fakeDialog{path: normal, after: func() { f.current = false }}
	r := NewWithDependencies(f, d).ImportFile(context.Background(), FileImportRequest{ConnectionEpoch: 1, SessionID: sid, IdempotencyKey: "key"})
	if r.OK || r.Error.Code != "staleConnection" || f.method == "POST" {
		t.Fatal("cross epoch upload")
	}
}

func TestMissingContentFieldCannotClaimComplete(t *testing.T) {
	f := &fakeConn{current: true, reply: `{"knowledgeId":"` + kid + `","version":1,"visibleBytes":0,"completeness":"complete","citation":{"knowledgeId":"` + kid + `","version":1,"ref":"knref_012345678901234567890123"}}`}
	r := NewWithDependencies(f, &fakeDialog{}).Content(context.Background(), ContentRequest{ConnectionEpoch: 1, SessionID: sid, KnowledgeID: kid})
	if r.OK || r.Value != nil {
		t.Fatal("missing content claimed complete")
	}
}

func TestItemErrorProjection(t *testing.T) {
	for _, code := range []string{"SOURCE_MISSING", "secret-api-key"} {
		reply := strings.TrimSuffix(itemJSON, "}") + `,"errorCode":"` + code + `","safeMessage":"Bearer private-key https://example.org?token=secret","createdAt":"private-key","checksum":"private-key","unexpected":{"token":"private-key"}}`
		f := &fakeConn{current: true, reply: reply}
		r := NewWithDependencies(f, &fakeDialog{}).Read(context.Background(), ItemRequest{ConnectionEpoch: 1, SessionID: sid, KnowledgeID: kid})
		if !r.OK {
			t.Fatal(r.Error)
		}
		data, _ := json.Marshal(r)
		if strings.Contains(string(data), "private-key") || strings.Contains(string(data), "secret-api-key") {
			t.Fatal("error secret escaped")
		}
		if code == "SOURCE_MISSING" && (r.Value.ErrorCode == nil || *r.Value.ErrorCode != code || r.Value.SafeMessage == nil) {
			t.Fatal("known error lost")
		}
	}
}

func TestImportMalformedAndEpochUnknownRetainKey(t *testing.T) {
	for _, reply := range []string{`null`, `{}`, strings.Replace(importJSON, `"version":1`, `"version":0`, 1), importJSON + ` garbage`, importJSON + importJSON} {
		f := &fakeConn{current: true, reply: reply}
		r := NewWithDependencies(f, &fakeDialog{}).ImportText(context.Background(), TextImportRequest{ConnectionEpoch: 1, SessionID: sid, IdempotencyKey: "recover-key", DisplayName: "Notes", Content: "text"})
		if r.OK || r.Value != nil || r.IdempotencyKey != "recover-key" || r.Error.RetryDisposition != "verifyOnly" {
			t.Fatalf("false import %+v", r)
		}
	}
	f := &fakeConn{current: true, reply: importJSON}
	f.after = func() { f.current = false }
	r := NewWithDependencies(f, &fakeDialog{}).ImportText(context.Background(), TextImportRequest{ConnectionEpoch: 1, SessionID: sid, IdempotencyKey: "recover-key", DisplayName: "Notes", Content: "text"})
	if r.OK || r.Error.RetryDisposition != "verifyOnly" || r.IdempotencyKey != "recover-key" {
		t.Fatal("epoch write uncertainty lost")
	}
}

func TestSessionPermissionBeforeFileSelection(t *testing.T) {
	f := &fakeConn{current: true, err: dto.Failure("denied", "runtimeIngress", "never", 403)}
	d := &fakeDialog{path: "/private/file.txt"}
	r := NewWithDependencies(f, d).ImportFile(context.Background(), FileImportRequest{ConnectionEpoch: 1, SessionID: sid, IdempotencyKey: "key"})
	if r.OK || r.Error.Code != "denied" || d.calls != 0 || f.calls != 1 {
		t.Fatal("local file selected before permission")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.err = nil
	r = NewWithDependencies(f, d).ImportFile(ctx, FileImportRequest{ConnectionEpoch: 1, SessionID: sid, IdempotencyKey: "key"})
	if r.OK || d.calls != 0 || f.calls != 1 {
		t.Fatal("cancelled selection")
	}
}

func TestHTTPTextSchemaAndErrorSanitization(t *testing.T) {
	status := 202
	reply := importJSON
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Idempotency-Key") != "same-key" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("headers")
		}
		if r.URL.Path != "/api/v1/sessions/"+sid+"/knowledge/imports" {
			t.Error("path")
		}
		var body map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&body)
		if len(body) != 3 || string(body["content"]) != `"chosen text"` || string(body["sourceType"]) != `"text"` {
			t.Error("schema")
		}
		w.WriteHeader(status)
		io.WriteString(w, reply)
	}))
	defer server.Close()
	conn := connection.New()
	epoch, _ := conn.Bind(connection.Binding{Endpoint: server.URL}, true)
	s := New(conn)
	q := TextImportRequest{ConnectionEpoch: epoch, SessionID: sid, IdempotencyKey: "same-key", DisplayName: "Notes", Content: "chosen text"}
	if !s.ImportText(context.Background(), q).OK {
		t.Fatal("text failed")
	}
	if !s.ImportText(context.Background(), q).OK || calls != 2 {
		t.Fatal("explicit same-key action")
	}
	for _, code := range []int{401, 403, 404, 409, 422, 503} {
		status = code
		reply = `{"detail":"secret-api-key","extra":"Bearer token"}`
		r := s.ImportText(context.Background(), q)
		if r.OK || r.Error.HTTPStatus != code || r.IdempotencyKey != "same-key" {
			t.Fatal("http error")
		}
		out, _ := json.Marshal(r)
		if strings.Contains(string(out), "secret-api-key") || strings.Contains(string(out), "Bearer token") {
			t.Fatal("raw error leaked")
		}
		if code == 503 && r.Error.RetryDisposition != "verifyOnly" {
			t.Fatal("503 resend")
		}
	}
}

func TestFilePayloadValidation(t *testing.T) {
	for _, sample := range []struct {
		name string
		data []byte
	}{{"binary.txt", []byte("zero\x00byte")}, {"invalid.txt", []byte{0xff}}, {"fake.pdf", []byte("pdf")}, {"fake.docx", []byte("not zip")}, {"script.exe", []byte("executable")}} {
		t.Run(sample.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), sample.name)
			os.WriteFile(path, sample.data, 0600)
			if _, _, e := selectedMultipart(context.Background(), path, ""); e == nil {
				t.Fatal("invalid payload allowed")
			}
		})
	}
}

func TestHTTPMalformedReadAndEpochCancellation(t *testing.T) {
	t.Run("trailing garbage", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, itemJSON+` trailing`) }))
		defer server.Close()
		conn := connection.New()
		epoch, _ := conn.Bind(connection.Binding{Endpoint: server.URL}, true)
		r := New(conn).Read(context.Background(), ItemRequest{ConnectionEpoch: epoch, SessionID: sid, KnowledgeID: kid})
		if r.OK || r.Value != nil || r.Error.Code != "invalidResponse" {
			t.Fatalf("malformed item %+v", r)
		}
	})
	t.Run("switch while reading", func(t *testing.T) {
		started := make(chan struct{})
		cancelled := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"items":[`))
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
			close(cancelled)
		}))
		defer server.Close()
		conn := connection.New()
		epoch, _ := conn.Bind(connection.Binding{Endpoint: server.URL}, true)
		finished := make(chan ListResult, 1)
		go func() {
			finished <- New(conn).List(context.Background(), SessionRequest{ConnectionEpoch: epoch, SessionID: sid})
		}()
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("request missing")
		}
		conn.Disconnect()
		select {
		case r := <-finished:
			if r.OK || r.Value != nil || r.Error == nil {
				t.Fatal("old epoch read accepted")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("old body read not cancelled")
		}
		select {
		case <-cancelled:
		case <-time.After(3 * time.Second):
			t.Fatal("request not cancelled")
		}
	})
}
