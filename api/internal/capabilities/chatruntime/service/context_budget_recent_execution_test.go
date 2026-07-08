package service

import (
	"context"
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

func TestPreparedResultMetadataRecomputesStaleOperationResultSummary(t *testing.T) {
	metadata := preparedResultMetadata(map[string]interface{}{
		"operation_result_summary": map[string]interface{}{
			"source":              "execution_summary",
			"plan_status":         operationPlanStatusRunning,
			"pending_next_action": "Run tool:agent-management/update_agent_config",
			"tool_name":           "list_agent_database_tables",
		},
		"operation_plan": map[string]interface{}{
			"status":              operationPlanStatusCompleted,
			"pending_next_action": "none",
			"tool_result": map[string]interface{}{
				"kind":      "tool_call",
				"status":    "success",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "get_agent_config",
				"result_summary": map[string]interface{}{
					"status":                  "completed",
					"agent_id":                "agent-1",
					"knowledge_dataset_count": 1,
					"database_binding_count":  1,
					"workflow_binding_count":  1,
				},
			},
		},
	}, nil)

	summary := mapFromOperationContext(metadata["operation_result_summary"])
	if len(summary) == 0 {
		t.Fatalf("operation_result_summary missing in metadata: %#v", metadata)
	}
	if got := stringFromAny(summary["plan_status"]); got != operationPlanStatusCompleted {
		t.Fatalf("operation_result_summary.plan_status = %q, want completed; summary=%#v", got, summary)
	}
	if got := stringFromAny(summary["pending_next_action"]); got != "none" {
		t.Fatalf("operation_result_summary.pending_next_action = %q, want none; summary=%#v", got, summary)
	}
	if got := stringFromAny(summary["tool_name"]); got != "get_agent_config" {
		t.Fatalf("operation_result_summary.tool_name = %q, want post-update get_agent_config; summary=%#v", got, summary)
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

func TestCompactOperationPlanForPromptIncludesPlanDeviations(t *testing.T) {
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

	deviations := mapSliceFromAny(compact["deviations"])
	if len(deviations) != 1 {
		t.Fatalf("compact deviations = %#v, want one allowed deviation", compact["deviations"])
	}
	if got := stringFromAny(deviations[0]["outcome"]); got != "allowed" {
		t.Fatalf("compact deviation outcome = %q, want allowed", got)
	}
	blockedDeviations := mapSliceFromAny(compact["blocked_deviations"])
	if len(blockedDeviations) != 1 {
		t.Fatalf("compact blocked_deviations = %#v, want one blocked deviation", compact["blocked_deviations"])
	}
	if got := stringFromAny(blockedDeviations[0]["outcome"]); got != "blocked" {
		t.Fatalf("compact blocked deviation outcome = %q, want blocked", got)
	}
}

func TestCompactOperationPlanForPromptKeepsRiskApprovalAndSuccessCriteria(t *testing.T) {
	compact := compactOperationPlanForPrompt(map[string]interface{}{
		"version":           operationPlanVersion,
		"task_id":           "task-agent-config",
		"status":            operationPlanStatusRunning,
		"risk_level":        "medium",
		"approval":          "agent-management mutations are governed",
		"approval_required": true,
		"approval_actions": []interface{}{
			"tool:agent-management/update_agent_config",
		},
		"success_criteria": []interface{}{
			"update_agent_config succeeds for the requested Agent",
			"page observation confirms the updated Agent configuration",
		},
		"completion_criteria": []interface{}{
			"final answer reports only observed configuration changes",
		},
		"page_evidence": map[string]interface{}{
			"current_page": "/console/agents/agent-1/agent",
			"resources": []interface{}{
				map[string]interface{}{
					"resource_type": "agent",
					"id":            "agent-1",
					"title":         "Support Agent",
				},
			},
		},
		"completed_steps": []interface{}{
			map[string]interface{}{
				"id":        "tool:agent-management/list_agent_knowledge_candidates",
				"status":    operationPlanStepStatusCompleted,
				"title":     "list knowledge candidates",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "list_agent_knowledge_candidates",
			},
		},
		"failed_steps": []interface{}{
			map[string]interface{}{
				"id":        "tool:agent-management/update_agent_config",
				"status":    operationPlanStepStatusFailed,
				"title":     "update agent config",
				"skill_id":  skills.SkillAgentManagement,
				"tool_name": "update_agent_config",
				"error":     "provider/model pair is invalid",
			},
		},
	})

	if compact["risk_level"] != "medium" || compact["approval"] != "agent-management mutations are governed" || compact["approval_required"] != true {
		t.Fatalf("compact plan risk/approval = %#v, want preserved fields", compact)
	}
	assertStringSliceContains(t, stringSliceFromAny(compact["approval_actions"]), "tool:agent-management/update_agent_config")
	assertStringSliceContains(t, stringSliceFromAny(compact["success_criteria"]), "update_agent_config succeeds for the requested Agent")
	assertStringSliceContains(t, stringSliceFromAny(compact["completion_criteria"]), "final answer reports only observed configuration changes")
	if pageEvidence := mapFromOperationContext(compact["page_evidence"]); pageEvidence["current_page"] != "/console/agents/agent-1/agent" {
		t.Fatalf("compact page_evidence = %#v, want current page evidence", pageEvidence)
	}
	completedSteps := mapSliceFromAny(compact["completed_steps"])
	if len(completedSteps) != 1 || stringFromAny(completedSteps[0]["tool_name"]) != "list_agent_knowledge_candidates" {
		t.Fatalf("compact completed_steps = %#v, want candidate list step", compact["completed_steps"])
	}
	failedSteps := mapSliceFromAny(compact["failed_steps"])
	if len(failedSteps) != 1 || stringFromAny(failedSteps[0]["error"]) != "provider/model pair is invalid" {
		t.Fatalf("compact failed_steps = %#v, want failure reason", compact["failed_steps"])
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
