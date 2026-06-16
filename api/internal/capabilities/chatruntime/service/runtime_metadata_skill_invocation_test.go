package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestMergeSkillTraceMetadataRedactsFileReaderResultContent(t *testing.T) {
	const rawContent = "SKILL_TRACE_SECRET_SHOULD_NOT_PERSIST"

	metadata := mergeSkillTraceMetadata(nil, []skills.SkillTrace{{
		Kind:     "tool_call",
		SkillID:  skills.SkillFileReader,
		ToolName: "read_file",
		Status:   "success",
		Result: map[string]interface{}{
			"status":            "completed",
			"content":           rawContent,
			"content_chars":     500,
			"content_truncated": true,
			"content_status":    "extracted",
			"file": map[string]interface{}{
				"id":           "file-1",
				"name":         "invoice.xlsx",
				"workspace_id": "workspace-1",
				"created_by":   "account-1",
			},
		},
	}})
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if strings.Contains(string(encoded), rawContent) {
		t.Fatalf("skill invocation metadata leaked raw file content: %s", string(encoded))
	}

	invocations, ok := metadata["skill_invocations"].([]interface{})
	if !ok || len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want one invocation", metadata["skill_invocations"])
	}
	invocation, _ := invocations[0].(map[string]interface{})
	result, ok := invocation["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("result = %#v, want map", invocation["result"])
	}
	if result["content_redacted"] != true || result["content_chars"] != 500 || result["content_returned_chars"] != len([]rune(rawContent)) {
		t.Fatalf("result content summary = %#v, want redaction with original and returned char counts", result)
	}
	if _, ok := result["content"]; ok {
		t.Fatalf("content should not be persisted in skill invocation result: %#v", result)
	}
	file, ok := result["file"].(map[string]interface{})
	if !ok {
		t.Fatalf("file = %#v, want safe file summary", result["file"])
	}
	if file["id"] != "file-1" || file["name"] != "invoice.xlsx" || file["workspace_id"] != "workspace-1" {
		t.Fatalf("file summary = %#v, want safe file metadata", file)
	}
	if _, ok := file["created_by"]; ok {
		t.Fatalf("created_by should not be persisted in skill invocation file summary: %#v", file)
	}
}

func TestMergeSkillInvocationMetadataRedactsFileReaderFilePreviews(t *testing.T) {
	const rawPreview = "SKILL_EVENT_PREVIEW_SHOULD_NOT_PERSIST"

	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{{
		"kind":      "tool_call",
		"skill_id":  skills.SkillFileReader,
		"tool_name": "list_visible_files",
		"status":    "success",
		"result": map[string]interface{}{
			"status":         "completed",
			"count":          1,
			"selected_count": 1,
			"files": []map[string]interface{}{{
				"visible_index":   1,
				"file_id":         "file-1",
				"name":            "notes.pdf",
				"workspace_id":    "workspace-1",
				"content_preview": rawPreview,
				"content_chars":   300,
			}},
		},
	}})
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if strings.Contains(string(encoded), rawPreview) {
		t.Fatalf("skill invocation metadata leaked raw file preview: %s", string(encoded))
	}

	invocations := metadata["skill_invocations"].([]interface{})
	invocation := invocations[0].(map[string]interface{})
	result := invocation["result"].(map[string]interface{})
	files, ok := result["files"].([]map[string]interface{})
	if !ok || len(files) != 1 {
		t.Fatalf("files = %#v, want one summarized file", result["files"])
	}
	if files[0]["content_preview_redacted"] != true || files[0]["content_preview_chars"] != len([]rune(rawPreview)) {
		t.Fatalf("file preview summary = %#v, want redaction markers", files[0])
	}
	if files[0]["content_chars"] != 300 || files[0]["file_id"] != "file-1" {
		t.Fatalf("file audit fields = %#v, want content_chars and file_id preserved", files[0])
	}
	if result["files_content_redacted"] != true {
		t.Fatalf("files_content_redacted = %#v, want true", result["files_content_redacted"])
	}
}

func TestMergeSkillInvocationMetadataKeepsNonFileResultContent(t *testing.T) {
	const calculatorContent = "not sensitive calculator explanation"
	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{{
		"kind":      "tool_call",
		"skill_id":  skills.SkillCalculator,
		"tool_name": "calculate",
		"status":    "success",
		"result": map[string]interface{}{
			"content": calculatorContent,
		},
	}})

	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if !strings.Contains(string(encoded), calculatorContent) {
		t.Fatalf("non-file skill result content was unexpectedly redacted: %s", string(encoded))
	}
}
