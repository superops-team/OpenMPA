package trace

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func archivedEvent(kind, id string) dto.ClientEvent {
	return dto.ClientEvent{SchemaVersion: 2, SessionID: "session", InvocationID: "turn", EventID: id, Kind: kind, Author: "assistant"}
}

func TestArchiveKeepsFastWorkerToolAfterRemoteTeardown(t *testing.T) {
	cache, _ := NewCache(t.TempDir())
	call := archivedEvent("toolCall", "call-event")
	call.SourceType, call.ToolID, call.ToolName, call.ToolStatus = "codex.tool.call", "tool-id", "exec_command", "running"
	call.ToolSubject = "printf done"
	result := call
	duration := 7.5
	result.EventID, result.Kind, result.SourceType, result.ToolStatus, result.DurationMS = "result-event", "toolResult", "codex.tool.result", "success", &duration
	result.OutputPreview = "done"
	terminal := archivedEvent("done", "")
	terminal.Terminal = true
	for _, event := range []dto.ClientEvent{call, result, result, call, terminal} {
		if err := cache.AppendEvent(event); err != nil {
			t.Fatal(err)
		}
	}
	local, ok := cache.LoadTrace("session", "turn")
	if !ok || len(local.Steps) != 1 || local.Steps[0].Status != "success" || local.Steps[0].StartedAt != "" || *local.Steps[0].DurationMS != duration {
		t.Fatalf("fast tool was lost, duplicated or given invented timing: %+v", local)
	}
	if local.Coverage["complete"] != false || local.Steps[0].Category != "worker" {
		t.Fatal("local source misrepresented coverage or origin")
	}
	remote := Trace{TraceVersion: "1.4", SessionID: "session", InvocationID: "turn", Run: map[string]any{"status": "success"}, Coverage: map[string]any{"active": false, "workerTrajectory": map[string]any{"status": "unavailable"}}, Steps: []Step{{StepID: "root", Kind: "run.total", Status: "success"}}}
	merged, err := cache.MergeTrace("session", "turn", remote)
	if err != nil || len(merged.Steps) != 2 {
		t.Fatalf("teardown discarded local worker: %+v %v", merged, err)
	}
	// When the full trace arrives, correlate the exact tool id rather than title.
	remote.Steps = append(remote.Steps, Step{StepID: "canonical-tool", Kind: "tool.call", Status: "success", Correlation: map[string]any{"toolId": "tool-id"}})
	merged, err = cache.MergeTrace("session", "turn", remote)
	if err != nil || len(merged.Steps) != 2 || merged.Steps[1].Details["toolSubject"] != "printf done" {
		t.Fatalf("correlated tool duplicated or lost details: %+v %v", merged, err)
	}
}

func TestArchiveReasoningReplayAndParts(t *testing.T) {
	cache, _ := NewCache(t.TempDir())
	first := archivedEvent("reasoning", "delta-1")
	zero, one := 0, 1
	first.Text, first.Partial, first.PartIndex = "First ", true, &zero
	second := first
	second.EventID, second.Text = "delta-2", "line."
	full := second
	full.EventID, full.Text, full.Partial = "final", "First line.\nSecond line.", false
	part := full
	part.PartIndex, part.Text = &one, "Separate part."
	for _, event := range []dto.ClientEvent{first, first, second, full, part} {
		if err := cache.AppendEvent(event); err != nil {
			t.Fatal(err)
		}
	}
	value, ok := cache.LoadTrace("session", "turn")
	if !ok || len(value.Steps) != 2 || value.Steps[0].Details["text"] != full.Text || value.Steps[1].Details["text"] != part.Text {
		t.Fatalf("reasoning replay/parts corrupted: %+v", value)
	}
	if value.Steps[0].Category == "worker" {
		t.Fatal("worker origin was inferred without evidence")
	}
}

func TestConcurrentSnapshotsPreserveAllEvidenceAndTerminalState(t *testing.T) {
	cache, _ := NewCache(t.TempDir())
	var work sync.WaitGroup
	for index := range 20 {
		work.Add(1)
		go func() {
			defer work.Done()
			value := Trace{SessionID: "session", InvocationID: "turn", Run: map[string]any{"status": "running"}, Steps: []Step{{StepID: fmt.Sprintf("step-%d", index), Status: "running"}}}
			if index == 0 {
				value.Run["status"] = "success"
				value.Steps[0].Status = "success"
			}
			if _, err := cache.MergeTrace("session", "turn", value); err != nil {
				t.Error(err)
			}
		}()
	}
	work.Wait()
	value, ok := cache.LoadTrace("session", "turn")
	if !ok || len(value.Steps) != 20 || value.Run["status"] != "success" {
		t.Fatalf("racing snapshot lost evidence/terminal: %+v", value)
	}
}

func TestArchiveCannotRecreateDeletedDataAndReportsWriteFailure(t *testing.T) {
	cache, _ := NewCache(t.TempDir())
	event := archivedEvent("reasoning", "event")
	event.Text = "Recorded"
	if err := cache.AppendEvent(event); err != nil {
		t.Fatal(err)
	}
	if err := cache.DeleteSession("session"); err != nil {
		t.Fatal(err)
	}
	if err := cache.AppendEvent(event); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.LoadTrace("session", "turn"); ok {
		t.Fatal("late event restored deleted session")
	}
	if _, err := os.Stat(cache.dir("session", "turn")); !os.IsNotExist(err) {
		t.Fatal("deleted directory reappeared")
	}
	other, _ := NewCache(t.TempDir())
	if err := other.SaveTrace("session", "turn", Trace{SessionID: "session", InvocationID: "turn"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(other.dir("session", "turn"), "events.jsonl"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := other.AppendEvent(event); err == nil {
		t.Fatal("archive write failure hidden")
	}
	value, ok := other.LoadTrace("session", "turn")
	if !ok || value.Coverage["localArchiveStatus"] != "failed" {
		t.Fatal("archive failure was not surfaced")
	}
}
