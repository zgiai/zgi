package service

import (
	"strings"
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestRecentExecutionContextIncludesRedactedFileReaderResult(t *testing.T) {
	const rawContent = "RAW_FILE_CONTENT_SHOULD_NOT_APPEAR"
	message := &runtimemodel.Message{
		Query:  "read that file",
		Status: runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":      "tool_call",
					"skill_id":  skills.SkillFileReader,
					"tool_name": "read_file",
					"status":    "success",
					"arguments": map[string]interface{}{"file_id": "file-1"},
					"result": map[string]interface{}{
						"status":         "completed",
						"content":        rawContent,
						"content_status": "extracted",
						"file": map[string]interface{}{
							"id":   "file-1",
							"name": "invoice.xlsx",
						},
					},
				},
			},
		},
	}

	recent, stats := buildRecentExecutionContextMessage([]*runtimemodel.Message{message})

	if recent == nil {
		t.Fatal("recent execution context = nil, want message")
	}
	content, ok := recent.Content.(string)
	if !ok {
		t.Fatalf("recent content type = %T, want string", recent.Content)
	}
	for _, want := range []string{"result=", "file-1", "invoice.xlsx", "content_status", "content_redacted"} {
		if !strings.Contains(content, want) {
			t.Fatalf("recent execution context missing %q: %s", want, content)
		}
	}
	if strings.Contains(content, rawContent) {
		t.Fatalf("recent execution context leaked raw file content: %s", content)
	}
	if stats.IncludedToolEvents != 1 {
		t.Fatalf("IncludedToolEvents = %d, want 1", stats.IncludedToolEvents)
	}
}

func TestRecentExecutionContextDoesNotIncludeGenericToolResult(t *testing.T) {
	const sensitiveContent = "GENERIC_TOOL_RESULT_SHOULD_NOT_APPEAR"
	message := &runtimemodel.Message{
		Query:  "calculate",
		Status: runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":      "tool_call",
					"skill_id":  skills.SkillCalculator,
					"tool_name": "calculate",
					"status":    "success",
					"result":    map[string]interface{}{"content": sensitiveContent},
				},
			},
		},
	}

	recent, _ := buildRecentExecutionContextMessage([]*runtimemodel.Message{message})

	if recent == nil {
		t.Fatal("recent execution context = nil, want message")
	}
	content, ok := recent.Content.(string)
	if !ok {
		t.Fatalf("recent content type = %T, want string", recent.Content)
	}
	if strings.Contains(content, "result=") || strings.Contains(content, sensitiveContent) {
		t.Fatalf("generic tool result should not be included: %s", content)
	}
}
