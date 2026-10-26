package session

import (
	"context"
	"encoding/json"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

var firstAttachment = "ho_" + strings.Repeat("a", 32)
var secondAttachment = "ho_" + strings.Repeat("b", 32)

func TestUndeployedAttachmentsAreRejectedWithoutDispatch(t *testing.T) {
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		t.Error("undeployed attachment reached network")
	}))
	defer server.Close()
	conn := connection.New()
	epoch, _ := conn.Bind(connection.Binding{Endpoint: server.URL, Scope: "scope"}, true)
	dir := t.TempDir()
	store := NewPendingStore(func() (string, error) { return dir, nil })
	service := New(conn, store)
	out := service.Submit(context.Background(), dto.SubmitRequest{ConnectionEpoch: epoch, SessionID: "s", OperationID: "o", IdempotencyKey: "key", Content: "original", AttachmentRefs: []string{firstAttachment}})
	if out.OK || out.Error.Code != "invalidInput" || posts.Load() != 0 {
		t.Fatal("unsupported attachment accepted")
	}
	original := Pending{Scope: "scope", SessionID: "s", OperationID: "old", IdempotencyKey: "original", Content: "original", AttachmentRefs: []string{firstAttachment}, ConfigVersion: "2", DurableBot: true}
	if e := store.Put(original); e != nil {
		t.Fatal(e)
	}
	recovered := service.Recover(context.Background(), dto.RecoverRequest{ConnectionEpoch: epoch, OperationID: "old", Mode: "retryDurable"})
	if recovered.OK || posts.Load() != 0 {
		t.Fatal("old attachment was replayed")
	}
	list, _ := store.List("scope")
	if len(list) != 1 || list[0].AttachmentRefs[0] != firstAttachment {
		t.Fatal("old request was discarded")
	}
}
func TestAttachmentInputRejectsUnsafeDuplicateAndOversizedRefs(t *testing.T) {
	conn := connection.New()
	epoch, _ := conn.Bind(connection.Binding{Endpoint: "https://example.invalid", Scope: "scope"}, false)
	service := New(conn, NewPendingStore(func() (string, error) { return t.TempDir(), nil }))
	for _, refs := range [][]string{{"/local/file"}, {"ho_bad"}, {firstAttachment, firstAttachment}, make([]string, 17)} {
		out := service.Submit(context.Background(), dto.SubmitRequest{ConnectionEpoch: epoch, SessionID: "s", OperationID: "o", IdempotencyKey: "key", Content: "content", AttachmentRefs: refs})
		if out.OK || out.Error.Code != "invalidInput" {
			t.Fatal("unsafe attachments accepted")
		}
	}
}
func TestLegacyPendingMissingAttachmentsRemainsEmpty(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, pendingFile("scope", "o"))
	os.WriteFile(name, []byte(`{"scope":"scope","sessionId":"s","operationId":"o","idempotencyKey":"key","content":"content","state":"outcomeUnknown"}`), 0600)
	store := NewPendingStore(func() (string, error) { return dir, nil })
	list, err := store.List("scope")
	if err != nil || len(list) != 1 || list[0].AttachmentRefs == nil || len(list[0].AttachmentRefs) != 0 {
		t.Fatal("legacy pending not safely normalized")
	}
	wire, _ := json.Marshal(list[0].Summary())
	if !strings.Contains(string(wire), `"attachmentRefs":[]`) {
		t.Fatal("legacy summary missing empty refs")
	}
}
