package service

import (
	"context"
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

func TestRecentExecutionContextOmitsAgentManagementArgumentShapeSummaries(t *testing.T) {
	message := &runtimemodel.Message{
		Query:  "update agent runtime config",
		Status: runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":      "tool_call",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "get_agent_config",
					"status":    "success",
					"arguments": map[string]interface{}{
						"agent_id": map[string]interface{}{"type": "string", "length": 36},
					},
					"result": map[string]interface{}{
						"status":            "completed",
						"agent_id":          "agent-1",
						"model_provider":    "openai",
						"model":             "gpt-4o",
						"home_title":        "Agent Home",
						"input_placeholder": "Ask the agent",
						"theme_color":       "emerald",
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
	for _, forbidden := range []string{`arguments=`, `"type":"string"`, `"length":36`, "map[length:36 type:string]"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("recent execution context leaked unusable argument shape %q: %s", forbidden, content)
		}
	}
	for _, want := range []string{"result=", "agent-1", "model_provider", "openai", "home_title", "Agent Home", "theme_color", "emerald"} {
		if !strings.Contains(content, want) {
			t.Fatalf("recent execution context missing %q: %s", want, content)
		}
	}
	if stats.IncludedToolEvents != 1 {
		t.Fatalf("IncludedToolEvents = %d, want 1", stats.IncludedToolEvents)
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

func TestRecentExecutionContextIncludesOperationResultSummary(t *testing.T) {
	message := &runtimemodel.Message{
		Query:  "delete the first two agents",
		Status: runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusCompleted,
				"operation_group": map[string]interface{}{
					"status":        "partial_failed",
					"operation":     "agent.delete",
					"asset_type":    "agent",
					"target_count":  2,
					"success_count": 1,
					"failed_count":  1,
					"item_results": []interface{}{
						map[string]interface{}{"name": "Agent A", "status": "success"},
						map[string]interface{}{"name": "Agent B", "status": "failed", "error": "permission denied"},
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
	for _, want := range []string{"Most recent operation result facts", "operation_result_summary", "partial_failed", "agent.delete", "failed_count"} {
		if !strings.Contains(content, want) {
			t.Fatalf("recent execution context missing %q: %s", want, content)
		}
	}
	if stats.IncludedOperationSummaries != 1 {
		t.Fatalf("IncludedOperationSummaries = %d, want 1", stats.IncludedOperationSummaries)
	}
}

func TestRecentExecutionContextForNewRequestOmitsPriorOperationHistory(t *testing.T) {
	branch := []*runtimemodel.Message{
		{
			Query:  "删除页面中的第一个智能体",
			Status: runtimemodel.MessageStatusCompleted,
			Answer: "第一个智能体已删除。",
			Metadata: map[string]interface{}{
				"skill_invocations": []interface{}{
					map[string]interface{}{
						"kind":      "tool_call",
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "delete_agent",
						"status":    "success",
					},
				},
				"operation_plan": map[string]interface{}{
					"status": operationPlanStatusCompleted,
					"operation_group": map[string]interface{}{
						"operation":     "agent.delete",
						"asset_type":    "agent",
						"target_count":  1,
						"success_count": 1,
						"failed_count":  0,
					},
				},
			},
		},
	}
	parts := &chatRequestParts{Query: "创建一个临时智能体，取名叫 AICHAT-MANIFEST-SMOKE，模型配置为 deepseek flash。"}

	recent, stats := buildRecentExecutionContextMessageForRequest(parts, branch)

	if recent != nil {
		t.Fatalf("recent execution context = %#v, want nil for independent new request", recent)
	}
	if stats.IncludedToolEvents != 0 || stats.IncludedOperationSummaries != 0 {
		t.Fatalf("recent stats = %#v, want no prior operation context for independent new request", stats)
	}
	boundary := currentTurnBoundaryMessage(parts)
	if boundary == nil {
		t.Fatal("current turn boundary = nil, want boundary for independent new request")
	}
	content, ok := boundary.Content.(string)
	if !ok {
		t.Fatalf("boundary content type = %T, want string", boundary.Content)
	}
	if !strings.Contains(content, "latest user request") || !strings.Contains(content, "Do not continue") {
		t.Fatalf("boundary content missing latest-request guidance: %s", content)
	}
}

func TestRecentExecutionContextForContinuationKeepsPriorOperationHistory(t *testing.T) {
	branch := []*runtimemodel.Message{
		{
			Query:  "删除页面中的第一个智能体",
			Status: runtimemodel.MessageStatusCompleted,
			Metadata: map[string]interface{}{
				"skill_invocations": []interface{}{
					map[string]interface{}{
						"kind":      "tool_call",
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "delete_agent",
						"status":    "success",
					},
				},
			},
		},
	}
	parts := &chatRequestParts{Query: "继续刚才的操作"}

	recent, stats := buildRecentExecutionContextMessageForRequest(parts, branch)

	if recent == nil {
		t.Fatal("recent execution context = nil, want prior tool history for continuation")
	}
	content, ok := recent.Content.(string)
	if !ok {
		t.Fatalf("recent content type = %T, want string", recent.Content)
	}
	if !strings.Contains(content, "delete_agent") {
		t.Fatalf("recent execution context missing prior tool history: %s", content)
	}
	if stats.IncludedToolEvents != 1 {
		t.Fatalf("IncludedToolEvents = %d, want 1", stats.IncludedToolEvents)
	}
	if currentTurnBoundaryMessage(parts) != nil {
		t.Fatal("current turn boundary should be omitted for explicit continuation")
	}
}

func TestHistoryIsolationForIndependentContextualOperationRequest(t *testing.T) {
	branch := []*runtimemodel.Message{
		{
			Query:  "删掉页面中的第一个智能体，然后创建一个新的智能体，取名叫小说创作大师",
			Answer: "已完成智能体「小说创作大师」的多项配置更新。",
			Status: runtimemodel.MessageStatusCompleted,
		},
	}
	parts := &chatRequestParts{
		Query:     "创建一个临时智能体，取名叫 AICHAT-MANIFEST-SMOKE，模型配置为 deepseek flash。",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator},
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent:      "manage_agent_asset",
			AssetEffect: "create",
			TargetPage:  "/console/agents",
			Confidence:  0.92,
		},
	}

	if !shouldIsolateHistoryForCurrentTurn(parts) {
		t.Fatal("shouldIsolateHistoryForCurrentTurn = false, want true for independent contextual create request")
	}
	groups, err := (&service{}).historyMessageGroupsForCurrentRequest(context.Background(), branch, parts)
	if err != nil {
		t.Fatalf("historyMessageGroupsForCurrentRequest() error = %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("history groups = %#v, want old operation turns isolated", groups)
	}
}

func TestHistoryIsolationKeepsHistoryForExplicitContinuation(t *testing.T) {
	branch := []*runtimemodel.Message{
		{
			Query:  "删掉页面中的第一个智能体，然后创建一个新的智能体，取名叫小说创作大师",
			Answer: "已完成第一步。",
			Status: runtimemodel.MessageStatusCompleted,
		},
	}
	parts := &chatRequestParts{
		Query:     "继续刚才的操作",
		Surface:   aiChatSurfaceContextualSidebar,
		SkillMode: skillModeAuto,
		SkillIDs:  []string{skills.SkillAgentManagement, skills.SkillConsoleNavigator},
	}

	if shouldIsolateHistoryForCurrentTurn(parts) {
		t.Fatal("shouldIsolateHistoryForCurrentTurn = true, want false for explicit continuation")
	}
	groups, err := (&service{}).historyMessageGroupsForCurrentRequest(context.Background(), branch, parts)
	if err != nil {
		t.Fatalf("historyMessageGroupsForCurrentRequest() error = %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("history groups empty, want prior turn for explicit continuation")
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

func TestContinuationTaskStateMessageIncludesOperationResultSummary(t *testing.T) {
	branch := []*runtimemodel.Message{
		{
			Query:  "删除前两个智能体",
			Status: runtimemodel.MessageStatusCompleted,
			Metadata: map[string]interface{}{
				"operation_plan": map[string]interface{}{
					"status":              operationPlanStatusCompleted,
					"original_user_goal":  "删除前两个智能体",
					"pending_next_action": "none",
					"operation_group": map[string]interface{}{
						"status":        "success",
						"operation":     "agent.delete",
						"asset_type":    "agent",
						"target_count":  2,
						"success_count": 2,
						"failed_count":  0,
					},
				},
			},
		},
	}

	message := buildContinuationTaskStateMessage(&chatRequestParts{Query: "继续"}, branch)

	if message == nil {
		t.Fatal("continuation task state = nil, want message")
	}
	content, ok := message.Content.(string)
	if !ok {
		t.Fatalf("continuation content type = %T, want string", message.Content)
	}
	for _, want := range []string{"Authoritative operation result facts", "operation_result_summary", "agent.delete", "success_count"} {
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

func TestPreparedResultMetadataStoresOperationResultSummary(t *testing.T) {
	metadata := preparedResultMetadata(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": operationPlanStatusCompleted,
			"operation_group": map[string]interface{}{
				"status":        "success",
				"operation":     "file.save",
				"asset_type":    "file",
				"target_count":  1,
				"success_count": 1,
				"failed_count":  0,
			},
		},
	}, nil)

	summary := mapFromOperationContext(metadata["operation_result_summary"])
	if len(summary) == 0 {
		t.Fatalf("operation_result_summary missing in metadata: %#v", metadata)
	}
	if got := stringFromAny(summary["operation"]); got != "file.save" {
		t.Fatalf("operation_result_summary.operation = %q, want file.save", got)
	}
	if got := intValueFromAny(summary["success_count"]); got != 1 {
		t.Fatalf("operation_result_summary.success_count = %d, want 1; summary=%#v", got, summary)
	}
}

func TestPreparedResultMetadataDoesNotFinalizeRunningOperationPlan(t *testing.T) {
	metadata := preparedResultMetadata(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"status": operationPlanStatusRunning,
			"phases": []interface{}{
				map[string]interface{}{
					"id":                "phase-reconcile",
					"status":            operationPlanStepStatusPending,
					"verification_mode": "model_reconciliation",
				},
			},
		},
	}, nil)

	plan := mapFromOperationContext(metadata["operation_plan"])
	if got := stringFromAny(plan["status"]); got != operationPlanStatusRunning {
		t.Fatalf("operation_plan.status = %q, want %q", got, operationPlanStatusRunning)
	}
	phases := mapSliceFromAny(plan["phases"])
	if len(phases) != 1 {
		t.Fatalf("operation_plan.phases = %#v, want one running phase", phases)
	}
	if got := stringFromAny(phases[0]["status"]); got != operationPlanStepStatusPending {
		t.Fatalf("phase.status = %q, want %q", got, operationPlanStepStatusPending)
	}
	if _, exists := phases[0]["completed_at"]; exists {
		t.Fatalf("phase.completed_at should not be written by intermediate metadata persistence: %#v", phases[0])
	}
}

func TestCompactOperationPlanForPromptOmitsLegacyPlanDeviations(t *testing.T) {
	compact := compactOperationPlanForPrompt(map[string]interface{}{
		"version": operationPlanVersion,
		"task_id": "task-with-deviation",
		"status":  operationPlanStatusRunning,
		"deviations": []interface{}{
			map[string]interface{}{
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "list_agents",
				"reason":    "model_collected_unplanned_readonly_evidence",
				"outcome":   "allowed",
			},
		},
		"blocked_deviations": []interface{}{
			map[string]interface{}{
				"skill_id":  skills.SkillFileManager,
				"tool_name": "delete_file",
				"reason":    "model_attempted_unrelated_mutation",
				"outcome":   "blocked",
			},
		},
	})

	for _, key := range []string{"deviations", "blocked_deviations"} {
		if _, exists := compact[key]; exists {
			t.Fatalf("compact plan contains audit-only field %q: %#v", key, compact[key])
		}
	}
}
