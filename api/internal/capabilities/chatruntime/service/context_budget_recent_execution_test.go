package service

import (
	"fmt"
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

func TestRecentExecutionContextOmitsGuardrailHistory(t *testing.T) {
	message := &runtimemodel.Message{
		Query:  "continue",
		Status: runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":      "guardrail",
					"skill_id":  skills.SkillConsoleNavigator,
					"tool_name": "navigate",
					"status":    "blocked",
					"message":   "do not repeat navigation",
					"error":     "old guardrail feedback",
				},
			},
		},
	}

	recent, stats := buildRecentExecutionContextMessage([]*runtimemodel.Message{message})

	if recent != nil {
		t.Fatalf("recent execution context = %#v, want nil for guardrail-only history", recent)
	}
	if stats.IncludedToolEvents != 0 {
		t.Fatalf("IncludedToolEvents = %d, want 0 for guardrail history", stats.IncludedToolEvents)
	}
}

func TestRecentExecutionContextIncludesFileManagerSaveResult(t *testing.T) {
	message := &runtimemodel.Message{
		Query:  "save it to file management",
		Status: runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":      "tool_call",
					"skill_id":  skills.SkillFileManager,
					"tool_name": "save_file_to_management",
					"status":    "success",
					"arguments": map[string]interface{}{
						"source_type":  "tool_file",
						"tool_file_id": "tool-file-1",
						"filename":     "saved.svg",
					},
					"result": map[string]interface{}{
						"file_id":             "managed-file-1",
						"name":                "saved.svg",
						"source_tool_file_id": "tool-file-1",
						"status":              "saved",
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
	for _, want := range []string{"save_file_to_management", "managed-file-1", "saved.svg", "source_tool_file_id"} {
		if !strings.Contains(content, want) {
			t.Fatalf("recent execution context missing %q: %s", want, content)
		}
	}
	if stats.IncludedToolEvents != 1 {
		t.Fatalf("IncludedToolEvents = %d, want 1", stats.IncludedToolEvents)
	}
}

func TestContinuationTaskStateMessageUsesPriorTaskAndSaveState(t *testing.T) {
	branch := []*runtimemodel.Message{
		{
			Query:  "帮我在文件管理中创建一个 svg",
			Status: runtimemodel.MessageStatusCompleted,
			Metadata: map[string]interface{}{
				"skill_invocations": []interface{}{
					map[string]interface{}{
						"kind":      "tool_call",
						"skill_id":  skills.SkillFileGenerator,
						"tool_name": "generate_file",
						"status":    "success",
						"result": map[string]interface{}{
							"tool_file_id": "tool-file-1",
							"filename":     "draft.svg",
						},
					},
				},
			},
		},
		{
			Query:  "继续",
			Status: runtimemodel.MessageStatusCompleted,
			Metadata: map[string]interface{}{
				"skill_invocations": []interface{}{
					map[string]interface{}{
						"kind":      "tool_call",
						"skill_id":  skills.SkillFileManager,
						"tool_name": "save_file_to_management",
						"status":    "success",
						"arguments": map[string]interface{}{
							"source_type":  "tool_file",
							"tool_file_id": "tool-file-1",
							"filename":     "draft.svg",
						},
						"result": map[string]interface{}{
							"file_id":             "managed-file-1",
							"name":                "draft.svg",
							"source_tool_file_id": "tool-file-1",
							"status":              "saved",
						},
					},
				},
			},
		},
	}
	parts := &chatRequestParts{Query: "继续"}

	message := buildContinuationTaskStateMessage(parts, branch)

	if message == nil {
		t.Fatal("continuation task state = nil, want message")
	}
	content, ok := message.Content.(string)
	if !ok {
		t.Fatalf("continuation content type = %T, want string", message.Content)
	}
	for _, want := range []string{
		"Most recent non-continuation user goal: 帮我在文件管理中创建一个 svg",
		"save_file_to_management",
		"managed-file-1",
		"do not repeat successful side-effecting tool calls",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("continuation task state missing %q: %s", want, content)
		}
	}
}

func TestContinuationTaskStateOmitsGuardrailToolState(t *testing.T) {
	branch := []*runtimemodel.Message{
		{
			Query:  "delete the first file",
			Status: runtimemodel.MessageStatusCompleted,
			Metadata: map[string]interface{}{
				"skill_invocations": []interface{}{
					map[string]interface{}{
						"kind":      "guardrail",
						"skill_id":  skills.SkillFileManager,
						"tool_name": "delete_file",
						"status":    "blocked",
						"message":   "do not delete yet",
						"error":     "old guardrail feedback",
					},
				},
			},
		},
	}

	message := buildContinuationTaskStateMessage(&chatRequestParts{Query: "continue"}, branch)

	if message == nil {
		t.Fatal("continuation task state = nil, want base continuation state without guardrail tool state")
	}
	content, ok := message.Content.(string)
	if !ok {
		t.Fatalf("continuation content type = %T, want string", message.Content)
	}
	for _, blocked := range []string{"Recent completed/blocked execution state", "old guardrail feedback", "do not delete yet"} {
		if strings.Contains(content, blocked) {
			t.Fatalf("continuation state contains guardrail history %q: %s", blocked, content)
		}
	}
}

func TestContinuationTaskStateMessageRequiresGovernedDeleteWithoutNaturalConfirmation(t *testing.T) {
	branch := []*runtimemodel.Message{
		{
			Query:  "\u5148\u5230\u6587\u4ef6\u7ba1\u7406\u521b\u5efa\u6587\u4ef6\uff0c\u6700\u540e\u5220\u9664\u5f53\u524d\u7b2c\u4e09\u4e2a\u6587\u4ef6",
			Status: runtimemodel.MessageStatusCompleted,
			Metadata: map[string]interface{}{
				"skill_invocations": []interface{}{
					map[string]interface{}{
						"kind":      "tool_call",
						"skill_id":  skills.SkillFileGenerator,
						"tool_name": "generate_file",
						"status":    "success",
					},
					map[string]interface{}{
						"kind":      "tool_call",
						"skill_id":  skills.SkillFileManager,
						"tool_name": "save_file_to_management",
						"status":    "success",
					},
				},
			},
		},
	}
	parts := &chatRequestParts{Query: "\u7ee7\u7eed\u3002\u4efb\u52a1\u6807\u8bb0\uff1aSMOKE-CONTINUE-1782312653811"}

	message := buildContinuationTaskStateMessage(parts, branch)

	if message == nil {
		t.Fatal("continuation task state = nil, want message")
	}
	content, ok := message.Content.(string)
	if !ok {
		t.Fatalf("continuation content type = %T, want string", message.Content)
	}
	for _, want := range []string{
		"file-manager/delete_file",
		"Do not ask for a separate natural-language confirmation",
		"tool governance handles the approval card",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("continuation task state missing %q: %s", want, content)
		}
	}
}

func TestContinuationTaskStateMessageIncludesOperationPlan(t *testing.T) {
	branch := []*runtimemodel.Message{
		{
			Query:  "\u8bf7\u751f\u6210\u5e76\u4fdd\u5b58 SVG\uff0c\u7136\u540e\u7b49\u6211\u8bf4\u7ee7\u7eed\u540e\u5220\u9664\u7b2c\u4e09\u4e2a\u6587\u4ef6",
			Status: runtimemodel.MessageStatusCompleted,
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"version":             operationPlanVersion,
					"task_id":             "task-op-1",
					"original_user_goal":  "\u8bf7\u751f\u6210\u5e76\u4fdd\u5b58 SVG\uff0c\u7136\u540e\u7b49\u6211\u8bf4\u7ee7\u7eed\u540e\u5220\u9664\u7b2c\u4e09\u4e2a\u6587\u4ef6",
					"intent":              "save_generated_file_to_file_management",
					"status":              operationPlanStatusRunning,
					"pending_next_action": "Delete frozen third file",
					"step_status": map[string]interface{}{
						"skill:file-generator":                      operationPlanStepStatusCompleted,
						"tool:file-manager/save_file_to_management": operationPlanStepStatusCompleted,
						"tool:file-manager/delete_file":             operationPlanStepStatusPending,
					},
					"steps": []interface{}{
						map[string]interface{}{
							"id":       "skill:file-generator",
							"title":    "Generate SVG",
							"status":   operationPlanStepStatusCompleted,
							"skill_id": skills.SkillFileGenerator,
						},
						map[string]interface{}{
							"id":        "tool:file-manager/delete_file",
							"title":     "Delete frozen third file",
							"status":    operationPlanStepStatusPending,
							"skill_id":  skills.SkillFileManager,
							"tool_name": "delete_file",
						},
					},
					"tool_result": map[string]interface{}{
						"kind":      "tool_call",
						"status":    "success",
						"skill_id":  skills.SkillFileManager,
						"tool_name": "save_file_to_management",
					},
				},
			},
		},
	}
	parts := &chatRequestParts{Query: "\u7ee7\u7eed"}

	message := buildContinuationTaskStateMessage(parts, branch)

	if message == nil {
		t.Fatal("continuation task state = nil, want message")
	}
	content, ok := message.Content.(string)
	if !ok {
		t.Fatalf("continuation content type = %T, want string", message.Content)
	}
	for _, want := range []string{
		"Authoritative operation plan state",
		"task-op-1",
		"pending_next_action",
		"Delete frozen third file",
		"tool:file-manager/delete_file",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("continuation task state missing %q: %s", want, content)
		}
	}
}

func TestContinuationTaskStateMessageKeepsActiveOriginalGoalAcrossIntermediateStage(t *testing.T) {
	originalGoal := "\u603b\u76ee\u6807\uff1a\u5148\u8bfb\u53d6\u667a\u80fd\u4f53\u9875\u7b2c\u4e00\u4e2a\u667a\u80fd\u4f53\u540d\u79f0\uff0c\u518d\u5230\u6587\u4ef6\u7ba1\u7406\u521b\u5efa\u5e76\u4fdd\u5b58 txt \u548c svg\uff0c\u6700\u540e\u5220\u9664\u5f53\u524d\u7b2c\u4e09\u4e2a\u6587\u4ef6"
	intermediateGoal := "\u7b2c\u4e8c\u9636\u6bb5\uff1a\u5230\u6587\u4ef6\u7ba1\u7406\u521b\u5efa\u5e76\u4fdd\u5b58 txt \u548c svg\uff0c\u4fdd\u5b58\u6210\u529f\u540e\u6682\u505c\uff0c\u4e0d\u8981\u5220\u9664\u6587\u4ef6"
	branch := []*runtimemodel.Message{
		{
			Query:  originalGoal,
			Status: runtimemodel.MessageStatusCompleted,
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"version":             operationPlanVersion,
					"task_id":             "task-original",
					"original_user_goal":  originalGoal,
					"intent":              "multi_step_asset_task",
					"status":              operationPlanStatusRunning,
					"pending_next_action": "Delete current third file",
					"step_status": map[string]interface{}{
						"skill:file-generator":                      operationPlanStepStatusCompleted,
						"tool:file-manager/save_file_to_management": operationPlanStepStatusCompleted,
						"tool:file-manager/delete_file":             operationPlanStepStatusPending,
					},
					"steps": []interface{}{
						map[string]interface{}{
							"id":       "skill:file-generator",
							"title":    "Generate files",
							"status":   operationPlanStepStatusCompleted,
							"skill_id": skills.SkillFileGenerator,
						},
						map[string]interface{}{
							"id":        "tool:file-manager/save_file_to_management",
							"title":     "Save generated files",
							"status":    operationPlanStepStatusCompleted,
							"skill_id":  skills.SkillFileManager,
							"tool_name": "save_file_to_management",
						},
						map[string]interface{}{
							"id":        "tool:file-manager/delete_file",
							"title":     "Delete current third file",
							"status":    operationPlanStepStatusPending,
							"skill_id":  skills.SkillFileManager,
							"tool_name": "delete_file",
						},
					},
				},
				"skill_invocations": []interface{}{
					map[string]interface{}{
						"kind":      "tool_call",
						"skill_id":  skills.SkillFileGenerator,
						"tool_name": "generate_file",
						"status":    "success",
					},
					map[string]interface{}{
						"kind":      "tool_call",
						"skill_id":  skills.SkillFileManager,
						"tool_name": "save_file_to_management",
						"status":    "success",
					},
				},
			},
		},
		{
			Query:  intermediateGoal,
			Status: runtimemodel.MessageStatusCompleted,
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"version":             operationPlanVersion,
					"task_id":             "task-intermediate",
					"original_user_goal":  intermediateGoal,
					"intent":              "save_generated_file_to_file_management",
					"status":              operationPlanStatusCompleted,
					"pending_next_action": "none",
					"step_status": map[string]interface{}{
						"skill:file-generator": operationPlanStepStatusCompleted,
						"skill:file-manager":   operationPlanStepStatusCompleted,
						"observe":              operationPlanStepStatusCompleted,
					},
				},
			},
		},
	}
	parts := &chatRequestParts{Query: "\u7ee7\u7eed"}

	message := buildContinuationTaskStateMessage(parts, branch)

	if message == nil {
		t.Fatal("continuation task state = nil, want message")
	}
	content, ok := message.Content.(string)
	if !ok {
		t.Fatalf("continuation content type = %T, want string", message.Content)
	}
	for _, want := range []string{
		"Most recent non-continuation user goal: " + intermediateGoal,
		"Prior active operation goals still relevant to this continuation",
		originalGoal,
		"file-manager/delete_file",
		"Do not ask for a separate natural-language confirmation",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("continuation task state missing %q: %s", want, content)
		}
	}
}

func TestContinuationPendingHintsDoNotTreatGenericFileManagerStepAsDelete(t *testing.T) {
	branch := []*runtimemodel.Message{
		{
			Query:  "\u5230\u6587\u4ef6\u7ba1\u7406\u4fdd\u5b58\u6587\u4ef6",
			Status: runtimemodel.MessageStatusCompleted,
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"version":            operationPlanVersion,
					"task_id":            "task-save-only",
					"original_user_goal": "\u5230\u6587\u4ef6\u7ba1\u7406\u4fdd\u5b58\u6587\u4ef6",
					"status":             operationPlanStatusRunning,
					"step_status": map[string]interface{}{
						"skill:file-manager": operationPlanStepStatusPending,
					},
					"steps": []interface{}{
						map[string]interface{}{
							"id":       "skill:file-manager",
							"title":    "Save file",
							"status":   operationPlanStepStatusPending,
							"skill_id": skills.SkillFileManager,
						},
					},
				},
			},
		},
	}

	if continuationHasPendingOperationPlanTool(branch, skills.SkillFileManager, "delete_file") {
		t.Fatal("generic file-manager step was treated as a pending delete_file step")
	}
	for _, hint := range continuationPendingHints("\u7ee7\u7eed", branch) {
		if strings.Contains(hint, "delete_file") {
			t.Fatalf("pending hint = %q, should not mention delete_file for a generic file-manager step", hint)
		}
	}
}

func TestCompactOperationPlanForPromptKeepsPendingStepsPastDefaultLimit(t *testing.T) {
	steps := make([]interface{}, 0, 10)
	stepStatus := map[string]interface{}{}
	for i := 1; i <= 8; i++ {
		id := fmt.Sprintf("skill:completed-%d", i)
		steps = append(steps, map[string]interface{}{
			"id":       id,
			"title":    fmt.Sprintf("Completed step %d", i),
			"status":   operationPlanStepStatusCompleted,
			"skill_id": fmt.Sprintf("completed-%d", i),
		})
		stepStatus[id] = operationPlanStepStatusCompleted
	}
	steps = append(steps,
		map[string]interface{}{
			"id":        "tool:file-manager/delete_file",
			"title":     "Delete frozen third file",
			"status":    operationPlanStepStatusPending,
			"skill_id":  skills.SkillFileManager,
			"tool_name": "delete_file",
		},
		map[string]interface{}{
			"id":     "observe",
			"title":  "Observe result",
			"status": operationPlanStepStatusPending,
		},
	)
	stepStatus["tool:file-manager/delete_file"] = operationPlanStepStatusPending
	stepStatus["observe"] = operationPlanStepStatusPending

	compact := compactOperationPlanForPrompt(map[string]interface{}{
		"version":             operationPlanVersion,
		"task_id":             "task-long-plan",
		"status":              operationPlanStatusRunning,
		"pending_next_action": "Delete frozen third file",
		"step_status":         stepStatus,
		"steps":               steps,
	})

	promptSteps := mapSliceFromAny(compact["steps"])
	if len(promptSteps) != 8 {
		t.Fatalf("prompt steps len = %d, want 8: %#v", len(promptSteps), promptSteps)
	}
	promptPlan := map[string]interface{}{"steps": compact["steps"]}
	if got := operationPlanStepStatusForTest(promptPlan, "tool:file-manager/delete_file"); got != operationPlanStepStatusPending {
		t.Fatalf("compressed steps missing pending delete step, status = %#v, steps = %#v", got, promptSteps)
	}
	if got := operationPlanStepStatusForTest(promptPlan, "observe"); got != operationPlanStepStatusPending {
		t.Fatalf("compressed steps missing pending observe step, status = %#v, steps = %#v", got, promptSteps)
	}
	deleteStep := operationPlanStepForTest(promptPlan, "tool:file-manager/delete_file")
	if got := intValueFromAny(deleteStep["sequence_index"]); got != 9 {
		t.Fatalf("delete step sequence_index = %d, want 9 in %#v", got, deleteStep)
	}
}

func operationPlanStepForTest(plan map[string]interface{}, id string) map[string]interface{} {
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if stringFromAny(step["id"]) == id {
			return step
		}
	}
	return nil
}
