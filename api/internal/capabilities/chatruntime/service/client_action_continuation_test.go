package service

import (
	"fmt"
	"strings"
	"testing"

	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestInjectClientActionContinuationContextPromotesLoadedRouteContext(t *testing.T) {
	parts := &chatRequestParts{
		Query:   "open the files page and tell me what is visible",
		Surface: aiChatSurfaceContextualSidebar,
	}
	event := map[string]interface{}{
		"action_id":   "route_navigation:files",
		"action_type": "route_navigation",
		"status":      clientActionStatusWaiting,
		"skill_id":    "console-navigator",
		"tool_name":   "navigate",
	}
	req := runtimedto.ClientActionResultRequest{
		Status: clientActionStatusSucceeded,
		Result: map[string]interface{}{
			"event_type":         "route_loaded",
			"loaded_href":        "/console/files",
			"page_context_ready": true,
			"context_items": []interface{}{
				map[string]interface{}{
					"id":                 "console.files",
					"type":               "page",
					"title":              "Files",
					"href":               "/console/files",
					"context_ready":      true,
					"files_query_status": "ready",
					"total_file_count":   7,
					"visible_file_count": 7,
				},
			},
		},
	}

	injectClientActionContinuationContext(parts, event, req, nil)

	if got := contextualTurnCurrentPage(parts); got != "/console/files" {
		t.Fatalf("contextualTurnCurrentPage() = %q, want /console/files", got)
	}
	if !clientActionContinuationLoadedRoute(parts, "/console/files") {
		t.Fatal("clientActionContinuationLoadedRoute() = false, want true for loaded_href")
	}
	if !consoleNavigationRouteAlreadyAvailable(parts, "/console/files") {
		t.Fatal("consoleNavigationRouteAlreadyAvailable() = false, want true")
	}

	evidence := skillLoopCompletionPageContextEvidence(parts)
	resources := operationItemsFromValue(evidence["resources"])
	if len(resources) == 0 {
		t.Fatalf("page context resources = %#v, want promoted route resources", evidence["resources"])
	}
	var filesPage map[string]interface{}
	for _, item := range resources {
		resource := mapFromOperationContext(item)
		if stringFromAny(resource["href"]) == "/console/files" {
			filesPage = resource
			break
		}
	}
	if len(filesPage) == 0 {
		t.Fatalf("page context resources = %#v, want files page resource", resources)
	}
	if filesPage["context_ready"] != true {
		t.Fatalf("files page context_ready = %#v, want true", filesPage["context_ready"])
	}
	if filesPage["total_file_count"] != 7 {
		t.Fatalf("files page total_file_count = %#v, want 7", filesPage["total_file_count"])
	}
	if filesPage["visible_file_count"] != 7 {
		t.Fatalf("files page visible_file_count = %#v, want 7", filesPage["visible_file_count"])
	}
}

func TestClientActionContinuationRouteFailureFeedbackIsRecoverable(t *testing.T) {
	parts := &chatRequestParts{
		Query:   "创建智能体后进入详情继续配置",
		Surface: aiChatSurfaceContextualSidebar,
	}
	event := map[string]interface{}{
		"action_id":   "route_navigation:agent-detail",
		"action_type": "route_navigation",
		"status":      clientActionStatusWaiting,
		"skill_id":    skills.SkillConsoleNavigator,
		"tool_name":   "navigate",
		"href":        "/console/agents/agent-1",
		"label":       "Agent detail",
		"label_key":   "agentDetail",
		"route_kind":  "agent_detail",
	}
	req := runtimedto.ClientActionResultRequest{
		Status: clientActionStatusFailed,
		Error:  "Route navigation target is unsupported.",
		Result: map[string]interface{}{
			"event_type":     "route_navigation_invalid",
			"action_type":    "route_navigation",
			"requested_href": "/console/agents/agent-1",
			"observed_path":  "/console/agents",
		},
	}

	injectClientActionContinuationContext(parts, event, req, nil)
	if clientActionContinuationLoadedRoute(parts, "/console/agents/agent-1/agent") {
		t.Fatal("clientActionContinuationLoadedRoute() = true for failed route, want false")
	}
	resources := operationItemsFromValue(parts.OperationContext["resources"])
	for _, item := range resources {
		resource := mapFromOperationContext(item)
		if stringFromAny(resource["href"]) == "/console/agents/agent-1/agent" {
			t.Fatalf("failed route was promoted as loaded page resource: %#v", resource)
		}
	}

	record := clientActionObservationRecord(event, req)
	feedback := mapFromOperationContext(record["model_feedback"])
	if feedback["failure_kind"] != "route_navigation_failed" ||
		feedback["target_completed"] != false ||
		feedback["recoverable"] != true {
		t.Fatalf("model_feedback = %#v, want recoverable route failure", feedback)
	}

	msg := clientActionContinuationMessage(&runtimemodel.Message{
		Query:    "创建智能体后进入详情继续配置",
		Metadata: map[string]interface{}{},
	}, event, req)
	content := strings.TrimSpace(fmt.Sprint(msg.Content))
	for _, want := range []string{
		"Client action failure feedback JSON",
		"route_navigation_failed",
		"the target page is not open",
		"/console/agents/{agent_id}/agent",
		"retry with a corrected supported route",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("continuation message missing %q in:\n%s", want, content)
		}
	}
}

func TestClientActionContinuationMessageFramesToolResultAsCurrentTurnEvidence(t *testing.T) {
	message := &runtimemodel.Message{
		Query: "请删除刚刚创建的文件 aichat-plan-smoke.md",
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":      "tool_call",
					"skill_id":  skills.SkillFileManager,
					"tool_name": "delete_file",
					"status":    "success",
					"result": map[string]interface{}{
						"status":        "completed",
						"deleted_count": 1,
						"file_name":     "aichat-plan-smoke.md",
					},
				},
				map[string]interface{}{
					"kind":        "client_action",
					"action_id":   "asset_observation:delete-file",
					"action_type": "asset_observation",
					"status":      clientActionStatusWaiting,
					"skill_id":    skills.SkillFileManager,
					"tool_name":   "delete_file",
				},
			},
		},
	}
	event := map[string]interface{}{
		"action_id":   "asset_observation:delete-file",
		"action_type": "asset_observation",
		"status":      clientActionStatusWaiting,
		"skill_id":    skills.SkillFileManager,
		"tool_name":   "delete_file",
	}

	msg := clientActionContinuationMessage(message, event, runtimedto.ClientActionResultRequest{
		Status: clientActionStatusSucceeded,
		Result: map[string]interface{}{"visible_count": 0},
	})
	content := strings.TrimSpace(fmt.Sprint(msg.Content))
	for _, want := range []string{
		"Current-turn tool result immediately before this frontend action JSON",
		"authoritative evidence for the current user request",
		"do not describe it as a previous round",
		"上一轮",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("continuation message missing %q in:\n%s", want, content)
		}
	}
	if strings.Contains(content, "Previously completed tool call") {
		t.Fatalf("continuation message still uses previous-turn wording:\n%s", content)
	}
}

func TestClientActionContinuationMessageIncludesOperationEvidenceLedger(t *testing.T) {
	readStepID := operationPlanToolStepID(skills.SkillFileReader, "read_file")
	createStepID := operationPlanToolStepID(skills.SkillAgentManagement, "create_agent")
	message := &runtimemodel.Message{
		Query: "\u5230\u6587\u4ef6\u7ba1\u7406\u8bfb\u53d6\u7b2c\u4e00\u4e2a\u6587\u4ef6\uff0c\u518d\u5230\u667a\u80fd\u4f53\u9875\u521b\u5efa\u540c\u540d\u667a\u80fd\u4f53",
		Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"status": operationPlanStatusRunning,
				"steps": []interface{}{
					map[string]interface{}{
						"id":        readStepID,
						"status":    operationPlanStepStatusCompleted,
						"skill_id":  skills.SkillFileReader,
						"tool_name": "read_file",
					},
					map[string]interface{}{
						"id":        createStepID,
						"status":    operationPlanStepStatusPending,
						"skill_id":  skills.SkillAgentManagement,
						"tool_name": "create_agent",
					},
				},
				"step_status": map[string]interface{}{
					readStepID:   operationPlanStepStatusCompleted,
					createStepID: operationPlanStepStatusPending,
				},
				operationPlanEvidenceLedgerKey: []interface{}{
					map[string]interface{}{
						"keys":      []interface{}{"file:read"},
						"skill_id":  skills.SkillFileReader,
						"tool_name": "read_file",
						"kind":      "tool_call",
						"status":    operationPlanStepStatusCompleted,
						"result_facts": map[string]interface{}{
							"file_name":             "\u65b0\u5efa \u6587\u672c\u6587\u6863.txt",
							"content_status":        "extracted",
							"content_value_preview": "\u6d4b\u8bd5\u4ee3\u7801111",
							"content_value_source":  "read_file.content",
						},
					},
				},
			},
		},
	}
	event := map[string]interface{}{
		"action_id":   "route_navigation:agents",
		"action_type": "route_navigation",
		"status":      clientActionStatusWaiting,
		"skill_id":    skills.SkillConsoleNavigator,
		"tool_name":   "navigate",
	}

	msg := clientActionContinuationMessage(message, event, runtimedto.ClientActionResultRequest{
		Status: clientActionStatusSucceeded,
		Result: map[string]interface{}{
			"event_type":  "route_loaded",
			"loaded_href": "/console/agents",
		},
	})
	content := strings.TrimSpace(fmt.Sprint(msg.Content))
	for _, want := range []string{
		"Current operation plan continuation state JSON",
		"evidence_ledger",
		"content_value_preview",
		"\u6d4b\u8bd5\u4ee3\u7801111",
		"placeholder words such as file content",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("continuation message missing %q in:\n%s", want, content)
		}
	}
}

func TestClientActionContinuationMessageIncludesTurnState(t *testing.T) {
	message := &runtimemodel.Message{
		Query: "create an agent from the file theme",
		Metadata: map[string]interface{}{
			"turn_state": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"kind":       "working_fact",
						"visibility": "model_only",
						"key":        "agent_theme",
						"value":      "water fee confirmation",
						"source":     "file-reader/read_file",
					},
				},
			},
		},
	}
	msg := clientActionContinuationMessage(message, map[string]interface{}{
		"action_id":   "route_navigation:agents",
		"action_type": "route_navigation",
		"status":      clientActionStatusWaiting,
	}, runtimedto.ClientActionResultRequest{
		Status: clientActionStatusSucceeded,
		Result: map[string]interface{}{"event_type": "route_loaded", "loaded_href": "/console/agents"},
	})
	content := strings.TrimSpace(fmt.Sprint(msg.Content))
	for _, want := range []string{
		"Current turn structured state JSON",
		"agent_theme",
		"water fee confirmation",
		"authoritative same-turn memory",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("continuation message missing %q in:\n%s", want, content)
		}
	}
}

func TestClientActionAgentCreateProgressIdentifiesMissingTarget(t *testing.T) {
	query := "冒烟准备：请创建两个草稿智能体，名称分别为 Agent One 和 Agent Two，描述都写“AIChat 快路径创建回归测试”。不要导航到详情页。完成后告诉我创建结果。"
	preparedMessage := &runtimemodel.Message{
		Query: query,
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{
				agentCreateInvocationForTest("agent-1", "Agent One"),
			},
			"client_actions": []interface{}{
				agentCreateObservationForTest("Agent One"),
			},
		},
	}
	preparedMessage.Metadata["operation_plan"] = agentCreateProgressPlanForTest("AIChat 快路径创建回归测试")

	progress := clientActionAgentCreateProgress(preparedMessage)
	if progress["status"] != "partial" {
		t.Fatalf("progress status = %#v, want partial; progress=%#v", progress["status"], progress)
	}
	if got := mapSliceFromAny(progress["missing_targets"]); len(got) != 0 {
		t.Fatalf("missing_targets should be string slice, got maps %#v", got)
	}
	missing, ok := progress["missing_targets"].([]string)
	if !ok || len(missing) != 1 || missing[0] != "Agent Two" {
		t.Fatalf("missing_targets = %#v, want [Agent Two]", progress["missing_targets"])
	}
	if desc := stringFromAny(progress["requested_description"]); desc != "AIChat 快路径创建回归测试" {
		t.Fatalf("requested_description = %q", desc)
	}
}

func TestClientActionContinuationMessageIncludesMissingAgentCreateTargets(t *testing.T) {
	message := &runtimemodel.Message{
		Query: "请创建两个草稿智能体，名称分别为 Agent One 和 Agent Two，描述都写“AIChat 快路径创建回归测试”。",
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{
				agentCreateInvocationForTest("agent-1", "Agent One"),
			},
			"client_actions": []interface{}{
				agentCreateObservationForTest("Agent One"),
			},
		},
	}
	message.Metadata["operation_plan"] = agentCreateProgressPlanForTest("AIChat fast-path create regression")
	event := map[string]interface{}{
		"action_id":   "asset_observation:create-agent",
		"action_type": "asset_observation",
		"status":      clientActionStatusWaiting,
		"skill_id":    skills.SkillAgentManagement,
		"tool_name":   "create_agent",
	}

	msg := clientActionContinuationMessage(message, event, runtimedto.ClientActionResultRequest{
		Status: clientActionStatusSucceeded,
		Result: map[string]interface{}{"visible_count": 1},
	})
	content := strings.TrimSpace(fmt.Sprint(msg.Content))
	for _, want := range []string{
		"Current-turn agent creation progress JSON",
		"missing_targets",
		"Agent Two",
		"do not give a final completion answer yet",
		"similar visible Agent with a different exact name",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("continuation message missing %q in:\n%s", want, content)
		}
	}
}

func TestSkillLoopCompletionEvidenceIncludesAgentCreateProgress(t *testing.T) {
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{
			Query: "create two draft agents named Agent One and Agent Two",
			Metadata: map[string]interface{}{
				"skill_invocations": []interface{}{
					agentCreateInvocationForTest("agent-1", "Agent One"),
				},
				"client_actions": []interface{}{
					agentCreateObservationForTest("Agent One"),
				},
			},
		},
		parts: &chatRequestParts{
			Query: "create two draft agents named Agent One and Agent Two",
		},
	}

	prepared.Message.Metadata["operation_plan"] = agentCreateProgressPlanForTest("AIChat fast-path create regression")
	evidence := skillLoopCompletionEvidence(prepared)()
	progress := mapFromOperationContext(evidence["agent_create_progress"])
	if progress["status"] != "partial" {
		t.Fatalf("agent_create_progress status = %#v, want partial; progress=%#v", progress["status"], progress)
	}
	missing, ok := progress["missing_targets"].([]string)
	if !ok || len(missing) != 1 || missing[0] != "Agent Two" {
		t.Fatalf("missing_targets = %#v, want [Agent Two]", progress["missing_targets"])
	}
	ledger := mapFromOperationContext(evidence["execution_ledger"])
	if len(mapFromOperationContext(ledger["agent_create_progress"])) == 0 {
		t.Fatalf("execution_ledger missing agent_create_progress: %#v", ledger)
	}
}

func TestTerminalRecentAgentMutationContinuationUsesModelIntent(t *testing.T) {
	recentPlans := []map[string]interface{}{
		{
			"status": operationPlanStatusCompleted,
			"steps": []interface{}{
				map[string]interface{}{
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "delete_agent",
					"status":    operationPlanStepStatusCompleted,
				},
			},
		},
	}
	prepared := &PreparedChat{
		parts: &chatRequestParts{
			Query:                "继续",
			RecentOperationPlans: recentPlans,
		},
	}
	if !skillLoopShouldBlockTerminalRecentAgentMutationContinuation(prepared, skills.SkillAgentManagement, "delete_agent") {
		t.Fatalf("weak continuation should block repeated terminal Agent mutation")
	}
	prepared.parts.ModelTurnIntent = &AIChatModelTurnIntent{
		Intent:      "manage_agent_asset",
		AssetEffect: "delete",
	}
	if skillLoopShouldBlockTerminalRecentAgentMutationContinuation(prepared, skills.SkillAgentManagement, "delete_agent") {
		t.Fatalf("explicit model mutation intent should allow a new Agent mutation request")
	}
	prepared.parts.ModelTurnIntent = &AIChatModelTurnIntent{
		Intent:                  "manage_agent_asset",
		RecommendedCapabilities: []string{"agent.skill_backed_capability:file-generator:enable"},
	}
	if skillLoopShouldBlockTerminalRecentAgentMutationContinuation(prepared, skills.SkillAgentManagement, "update_agent_config") {
		t.Fatalf("explicit model capability mutation should allow Agent config mutation continuation")
	}
}

func agentCreateProgressPlanForTest(description string) map[string]interface{} {
	return map[string]interface{}{
		"agent_create_count":       2,
		"agent_create_targets":     []interface{}{"Agent One", "Agent Two"},
		"agent_create_description": description,
	}
}

func agentCreateInvocationForTest(id, name string) map[string]interface{} {
	return map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "create_agent",
		"result": map[string]interface{}{
			"status":     "completed",
			"effect":     "created",
			"agent_id":   id,
			"agent_name": name,
		},
	}
}

func agentCreateObservationForTest(name string) map[string]interface{} {
	return map[string]interface{}{
		"kind":      "client_action",
		"status":    clientActionStatusSucceeded,
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": "create_agent",
		"result": map[string]interface{}{
			"effect":        "create",
			"asset_type":    "agent",
			"visible_count": 1,
			"observed_assets": []interface{}{
				map[string]interface{}{
					"name":    name,
					"type":    "agent",
					"visible": true,
				},
			},
		},
	}
}
