package digitalhuman

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type recordingEditor struct {
	dir     string
	applied []memoryFileEdit
	reject  string
}

func (e *recordingEditor) directory() string { return e.dir }
func (e *recordingEditor) purpose() string   { return "things for " + e.dir }
func (e *recordingEditor) modes() []string   { return []string{"explicit"} }
func (e *recordingEditor) apply(_ context.Context, _ uuid.UUID, edit memoryFileEdit) (string, error) {
	e.applied = append(e.applied, edit)
	return e.reject, nil
}

func TestMemoryFileKeyAcceptsOnlyFilesDirectlyInTheDirectory(t *testing.T) {
	for path, want := range map[string]string{
		"/user/prefers_email.md":   "prefers_email",
		"/user/notes/quote_due.md": "",
		"/user/":                   "",
		"/user/x.txt":              "",
		"/me/x.md":                 "",
	} {
		got, ok := memoryFileKey("/user/", path)
		if got != want || ok != (want != "") {
			t.Fatalf("key of %s = %q, %v", path, got, ok)
		}
	}
	if key, ok := memoryFileKey("/user/notes/", "/user/notes/quote_due.md"); !ok || key != "quote_due" {
		t.Fatalf("notes key = %q, %v", key, ok)
	}
}

func TestMemoryEditRoutesEachEditByPathAndHintsAtWrongDirectories(t *testing.T) {
	notes := &recordingEditor{dir: "/user/notes/"}
	user := &recordingEditor{dir: "/user/", reject: "evidence is not a quote from the user's message"}
	tool := newMemoryEditTool(&Run{ID: uuid.New()}, []memoryFileEditor{notes, user})
	raw, _ := json.Marshal(map[string]any{"edits": []map[string]any{
		{"command": "create", "path": "/user/notes/quote_due.md", "text": "周五前发报价单", "tags": "open_loop", "evidence": "你答应了周五前把报价单发给我", "mode": "explicit"},
		{"command": "create", "path": "/user/prefers_short.md", "text": "喜欢简短回答", "tags": "preference", "evidence": "made up", "mode": "explicit"},
		{"command": "create", "path": "/me/discount.md", "text": "全场五折", "evidence": "全场五折", "mode": "explicit"},
	}})
	result := tool.Handler(context.Background(), adapter.ToolCall{ID: "call-1", Function: adapter.FunctionCall{Name: memoryEditToolName, Arguments: string(raw)}})
	outcomes := result.Result["edits"].([]memoryFileEditOutcome)
	if result.Status != "success" || result.Result["accepted"] != 1 || len(notes.applied) != 1 || len(user.applied) != 1 {
		t.Fatalf("result = %+v", result.Result)
	}
	if outcomes[0].Status != memoryEditAccepted || outcomes[1].Status != memoryEditRejected || outcomes[1].Reason == "" {
		t.Fatalf("outcomes = %+v", outcomes)
	}
	if outcomes[2].Status != memoryEditRejected || !strings.Contains(outcomes[2].Reason, "/user/notes/") || !strings.Contains(outcomes[2].Reason, "/user/") {
		t.Fatalf("wrong directory was not rejected with a hint: %+v", outcomes[2])
	}

	// Nothing accepted: the call fails and may be retried.
	raw, _ = json.Marshal(map[string]any{"edits": []map[string]any{{"command": "delete", "path": "/conversation/x.md", "evidence": "x", "mode": "explicit"}}})
	failed := tool.Handler(context.Background(), adapter.ToolCall{ID: "call-2", Function: adapter.FunctionCall{Name: memoryEditToolName, Arguments: string(raw)}})
	if failed.Status != "failed" || !failed.Recoverable {
		t.Fatalf("all-rejected call = %+v", failed)
	}
	if bad := tool.Handler(context.Background(), adapter.ToolCall{ID: "call-3", Function: adapter.FunctionCall{Name: memoryEditToolName, Arguments: `{"edits":[],"extra":1}`}}); bad.Status == "success" {
		t.Fatal("malformed arguments were accepted")
	}
}

func TestMemoryEditGuidanceListsWhatEachDirectoryHolds(t *testing.T) {
	guidance := memoryEditGuidance([]memoryFileEditor{&recordingEditor{dir: "/user/notes/"}, &recordingEditor{dir: "/user/"}})
	if !strings.Contains(guidance, "- /user/notes/: things for /user/notes/") || !strings.Contains(guidance, "- /user/: things for /user/") || !strings.Contains(guidance, "memory_edit") {
		t.Fatalf("guidance = %s", guidance)
	}
}
