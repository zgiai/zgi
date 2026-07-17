package skillloop

import (
	"encoding/json"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestProjectMaterializedFileContentReplacesHistoricalToolPayloads(t *testing.T) {
	content := strings.Repeat("long chapter body ", 160)
	messages := []adapter.Message{
		{Role: "user", Content: content},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{
			ID: "intermediate-1",
			Function: adapter.FunctionCall{
				Name:      "call_skill_tool",
				Arguments: mustJSON(t, map[string]interface{}{"skill_id": "agent-management", "tool_name": "submit_intermediate_answer", "arguments": map[string]interface{}{"content": content}}),
			},
		}}},
		{Role: "tool", ToolCallID: "intermediate-1", Content: mustJSON(t, map[string]interface{}{"content": content})},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{
			ID: "generate-1",
			Function: adapter.FunctionCall{
				Name:      "call_skill_tool",
				Arguments: mustJSON(t, map[string]interface{}{"skill_id": "file-generator", "tool_name": "generate_file", "arguments": map[string]interface{}{"content": content, "filename": "chapter.md"}}),
			},
		}}},
		{Role: "tool", ToolCallID: "generate-1", Content: `{"status":"completed","file_id":"file-1"}`},
	}
	if detected, ok := generatedFileContentForCall(messages, "generate-1"); !ok || detected != content {
		t.Fatalf("generatedFileContentForCall() = (%d chars, %v), want (%d chars, true)", len([]rune(detected)), ok, len([]rune(content)))
	}

	projected, stats := projectMaterializedFileContent(messages, "generate-1", map[string]interface{}{
		"file_id":  "file-1",
		"filename": "chapter.md",
	})
	if stats.removedRunes <= 0 {
		t.Fatalf("removedRunes = %d, want positive", stats.removedRunes)
	}
	if len(stats.refs) != 1 || stats.refs[0] != "file_id:file-1" {
		t.Fatalf("refs = %#v, want file_id:file-1", stats.refs)
	}
	if got, _ := projected[0].Content.(string); got != content {
		t.Fatal("user-authored content must remain unchanged")
	}
	for _, index := range []int{1, 2, 3} {
		encoded, err := json.Marshal(projected[index])
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), content) {
			t.Fatalf("projected message %d still contains materialized body", index)
		}
		if !strings.Contains(string(encoded), "sha256:") {
			t.Fatalf("projected message %d has no digest reference: %s", index, encoded)
		}
	}
}

func TestManagedFileArtifactFromSaveResultCarriesSourceReference(t *testing.T) {
	artifact := managedFileArtifactFromSaveResult(skills.SkillTrace{
		SkillID:   skills.SkillFileManager,
		ToolName:  "save_file_to_management",
		Arguments: map[string]interface{}{"tool_file_id": "tool-1"},
	}, []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"target":         "managed_file",
			"file_id":        "managed-1",
			"upload_file_id": "managed-1",
			"filename":       "chapter.md",
			"size":           2048,
		},
	}})
	for key, want := range map[string]interface{}{
		"file_id":             "managed-1",
		"upload_file_id":      "managed-1",
		"source_tool_file_id": "tool-1",
		"filename":            "chapter.md",
		"size":                2048,
	} {
		if artifact[key] != want {
			t.Fatalf("artifact %s = %#v, want %#v in %#v", key, artifact[key], want, artifact)
		}
	}
}

func TestProjectMaterializedFileContentKeepsSmallPayload(t *testing.T) {
	content := "small file"
	messages := []adapter.Message{{
		Role: "assistant",
		ToolCalls: []adapter.ToolCall{{ID: "generate-1", Function: adapter.FunctionCall{
			Name:      "call_skill_tool",
			Arguments: mustJSON(t, map[string]interface{}{"skill_id": "file-generator", "tool_name": "generate_file", "arguments": map[string]interface{}{"content": content}}),
		}}},
	}}

	projected, stats := projectMaterializedFileContent(messages, "generate-1", map[string]interface{}{"file_id": "file-1"})
	if stats.removedRunes != 0 {
		t.Fatalf("removedRunes = %d, want 0", stats.removedRunes)
	}
	if !strings.Contains(projected[0].ToolCalls[0].Function.Arguments, content) {
		t.Fatal("small payload was unexpectedly projected")
	}
}

func mustJSON(t *testing.T, value interface{}) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
