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

func TestMergeSkillInvocationMetadataSortsTimelineByCreatedAt(t *testing.T) {
	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{
		{
			"kind":       "tool_call",
			"skill_id":   "file-manager",
			"tool_name":  "save_file_to_management",
			"status":     "success",
			"created_at": 300,
			"runtime_id": "save-late",
		},
		{
			"kind":       "client_action",
			"skill_id":   "console-navigator",
			"tool_name":  "navigate",
			"status":     "succeeded",
			"created_at": 100,
			"runtime_id": "route-early",
		},
		{
			"kind":       "reference_read",
			"skill_id":   "file-generator",
			"path":       "format-svg.md",
			"status":     "success",
			"created_at": "200",
			"runtime_id": "read-middle",
		},
	})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	if len(invocations) != 3 {
		t.Fatalf("skill_invocations len = %d in %#v, want 3", len(invocations), metadata["skill_invocations"])
	}
	got := []string{
		stringFromAny(invocations[0]["runtime_id"]),
		stringFromAny(invocations[1]["runtime_id"]),
		stringFromAny(invocations[2]["runtime_id"]),
	}
	want := []string{"route-early", "read-middle", "save-late"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("runtime_id order = %#v, want %#v", got, want)
		}
	}
}

func TestMergeSkillInvocationMetadataKeepsStableOrderForMissingCreatedAt(t *testing.T) {
	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{
		{
			"kind":       "tool_call",
			"skill_id":   "file-generator",
			"tool_name":  "generate_file",
			"status":     "success",
			"created_at": 100,
			"runtime_id": "dated",
		},
		{
			"kind":       "guardrail",
			"skill_id":   "file-manager",
			"tool_name":  "save_file_to_management",
			"status":     "blocked",
			"runtime_id": "missing-one",
		},
		{
			"kind":       "guardrail",
			"skill_id":   "file-generator",
			"tool_name":  "generate_file",
			"status":     "blocked",
			"runtime_id": "missing-two",
		},
	})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	got := []string{
		stringFromAny(invocations[0]["runtime_id"]),
		stringFromAny(invocations[1]["runtime_id"]),
		stringFromAny(invocations[2]["runtime_id"]),
	}
	want := []string{"dated", "missing-one", "missing-two"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("runtime_id order = %#v, want %#v", got, want)
		}
	}
}

func TestMergeSkillInvocationMetadataOmitsInternalPlannerGuardrail(t *testing.T) {
	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{
		{
			"kind":      "guardrail",
			"skill_id":  skills.SkillFileGenerator,
			"tool_name": "generate_file",
			"status":    "blocked",
			"error":     "use the existing temporary artifact",
			"arguments": map[string]interface{}{
				"next_step": "continue_planning",
			},
			"runtime_id": "internal-feedback",
		},
		{
			"kind":       "guardrail",
			"skill_id":   skills.SkillFileManager,
			"tool_name":  "save_file_to_management",
			"status":     "blocked",
			"error":      "visible governance guardrail",
			"runtime_id": "visible-guardrail",
		},
	})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	if len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want only the visible guardrail", invocations)
	}
	if got := stringFromAny(invocations[0]["runtime_id"]); got != "visible-guardrail" {
		t.Fatalf("runtime_id = %q, want visible-guardrail", got)
	}
	if metadata["guardrail_count"] != 1 {
		t.Fatalf("guardrail_count = %#v, want 1", metadata["guardrail_count"])
	}
}

func TestMergeSkillTraceMetadataOmitsInternalPlannerGuardrail(t *testing.T) {
	metadata := mergeSkillTraceMetadata(nil, []skills.SkillTrace{{
		Kind:     "guardrail",
		SkillID:  skills.SkillFileGenerator,
		ToolName: "generate_file",
		Status:   "blocked",
		Error:    "use the existing temporary artifact",
		Arguments: map[string]interface{}{
			"next_step": "continue_planning",
		},
	}})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	if len(invocations) != 0 {
		t.Fatalf("skill_invocations = %#v, want no user-visible planner feedback guardrail", invocations)
	}
	if metadata["guardrail_count"] != 0 {
		t.Fatalf("guardrail_count = %#v, want 0", metadata["guardrail_count"])
	}
}

func TestMergeSkillTraceMetadataOmitsPlannerFeedback(t *testing.T) {
	metadata := mergeSkillTraceMetadata(nil, []skills.SkillTrace{{
		Kind:     "planner_feedback",
		SkillID:  skills.SkillFileReader,
		ToolName: "read_file",
		Status:   "blocked",
		Error:    "skill must be loaded before calling its tools",
		Arguments: map[string]interface{}{
			"next_step": "load_skill",
		},
	}})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	if len(invocations) != 0 {
		t.Fatalf("skill_invocations = %#v, want no user-visible planner feedback", invocations)
	}
	if metadata["guardrail_count"] != 0 {
		t.Fatalf("guardrail_count = %#v, want 0", metadata["guardrail_count"])
	}
}

func TestMergeClientActionMetadataDoesNotReviveInternalPlannerGuardrail(t *testing.T) {
	metadata := map[string]interface{}{
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "guardrail",
				"skill_id":  skills.SkillFileGenerator,
				"tool_name": "generate_file",
				"status":    "blocked",
				"error":     "generated artifact already exists",
				"arguments": map[string]interface{}{
					"next_step": "continue_planning",
				},
				"runtime_id": "internal-feedback",
			},
			map[string]interface{}{
				"kind":       "tool_call",
				"skill_id":   skills.SkillConsoleNavigator,
				"tool_name":  "navigate",
				"status":     "success",
				"runtime_id": "route-tool",
			},
		},
	}

	metadata = mergeClientActionMetadata(metadata, map[string]interface{}{
		"action_id":   "route_navigation:call-files",
		"action_type": "route_navigation",
		"skill_id":    skills.SkillConsoleNavigator,
		"tool_name":   "navigate",
		"status":      "succeeded",
		"href":        "/console/files",
	})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	for _, invocation := range invocations {
		if stringFromAny(invocation["runtime_id"]) == "internal-feedback" {
			t.Fatalf("skill_invocations = %#v, want internal planner guardrail omitted", invocations)
		}
	}
	if metadata["guardrail_count"] != 0 {
		t.Fatalf("guardrail_count = %#v, want 0", metadata["guardrail_count"])
	}
}

func TestMergeToolGovernanceDecisionMetadataDoesNotReviveInternalPlannerGuardrail(t *testing.T) {
	metadata := map[string]interface{}{
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "guardrail",
				"skill_id":  skills.SkillFileManager,
				"tool_name": "save_file_to_management",
				"status":    "blocked",
				"error":     "continue with the existing generated artifact",
				"arguments": map[string]interface{}{
					"next_step": "continue_planning",
				},
				"runtime_id": "internal-feedback",
			},
			map[string]interface{}{
				"kind":           "tool_call",
				"skill_id":       skills.SkillFileManager,
				"tool_name":      "save_file_to_management",
				"status":         "waiting_approval",
				"runtime_id":     "save-tool",
				"correlation_id": "corr-save",
			},
		},
	}

	metadata = mergeToolGovernanceDecisionMetadata(metadata, map[string]interface{}{
		"correlation_id":    "corr-save",
		"skill_id":          skills.SkillFileManager,
		"tool_name":         "save_file_to_management",
		"approval_status":   "approved",
		"status":            "allowed",
		"requires_approval": true,
	})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	for _, invocation := range invocations {
		if stringFromAny(invocation["runtime_id"]) == "internal-feedback" {
			t.Fatalf("skill_invocations = %#v, want internal planner guardrail omitted", invocations)
		}
	}
	if metadata["guardrail_count"] != 0 {
		t.Fatalf("guardrail_count = %#v, want 0", metadata["guardrail_count"])
	}
}

func TestMergeToolGovernanceDecisionMetadataUpdatesOperationPlan(t *testing.T) {
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": operationPlanStatusRunning,
			"steps": []interface{}{
				map[string]interface{}{
					"id":        operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"),
					"status":    operationPlanStepStatusPending,
					"skill_id":  skills.SkillFileManager,
					"tool_name": "save_file_to_management",
				},
			},
			"step_status": map[string]interface{}{
				operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"): operationPlanStepStatusPending,
			},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":           "tool_call",
				"skill_id":       skills.SkillFileManager,
				"tool_name":      "save_file_to_management",
				"status":         "waiting_approval",
				"runtime_id":     "save-tool",
				"correlation_id": "corr-save",
			},
		},
	}

	metadata = mergeToolGovernanceDecisionMetadata(metadata, map[string]interface{}{
		"correlation_id":  "corr-save",
		"skill_id":        skills.SkillFileManager,
		"tool_name":       "save_file_to_management",
		"approval_status": "approved",
		"status":          "allowed",
	})

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusRunning {
		t.Fatalf("operation_plan.status = %q, want %q until save result evidence arrives; plan=%#v", got, operationPlanStatusRunning, plan)
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	stepID := operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management")
	if got := stringFromAny(stepStatus[stepID]); got != operationPlanStepStatusPending {
		t.Fatalf("step_status[%s] = %q, want %q until save result evidence arrives; plan=%#v", stepID, got, operationPlanStepStatusPending, plan)
	}
}
