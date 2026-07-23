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

func TestPreferredRestoredSkillAfterNavigationUsesDestinationBusinessSkill(t *testing.T) {
	parts := &chatRequestParts{SkillIDs: []string{
		skills.SkillConsoleNavigator,
		skills.SkillFileReader,
		skills.SkillFileManager,
		skills.SkillAgentManagement,
	}}
	event := map[string]interface{}{
		"action_type": "route_navigation",
		"skill_id":    skills.SkillConsoleNavigator,
		"tool_name":   "navigate",
	}
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{
				map[string]interface{}{"id": "phase-1", "status": operationPlanStepStatusCompleted, "step": "读取章节"},
				map[string]interface{}{"id": "phase-2", "status": operationPlanStepStatusPending, "step": "更新当前智能体配置"},
			},
		},
	}

	got := preferredRestoredSkillAfterClientAction(parts, event, runtimedto.ClientActionResultRequest{
		Status: clientActionStatusSucceeded,
		Result: map[string]interface{}{"loaded_href": "/console/agents/agent-1/agent"},
	}, metadata)
	if got != skills.SkillAgentManagement {
		t.Fatalf("preferred skill = %q, want %s", got, skills.SkillAgentManagement)
	}

	got = preferredRestoredSkillAfterClientAction(parts, event, runtimedto.ClientActionResultRequest{
		Status: clientActionStatusSucceeded,
		Result: map[string]interface{}{"observed_path": "/console/files"},
	}, map[string]interface{}{})
	if got != skills.SkillFileReader {
		t.Fatalf("preferred files skill = %q, want %s", got, skills.SkillFileReader)
	}
}

func TestPreferredRestoredSkillAfterFailedNavigationKeepsNavigator(t *testing.T) {
	parts := &chatRequestParts{SkillIDs: []string{skills.SkillConsoleNavigator, skills.SkillAgentManagement}}
	event := map[string]interface{}{
		"action_type": "route_navigation",
		"skill_id":    skills.SkillConsoleNavigator,
		"tool_name":   "navigate",
	}
	got := preferredRestoredSkillAfterClientAction(parts, event, runtimedto.ClientActionResultRequest{Status: clientActionStatusFailed}, nil)
	if got != skills.SkillConsoleNavigator {
		t.Fatalf("preferred skill = %q, want navigator for retryable failure", got)
	}
}

func TestResolveClientActionContinuationCompletesMatchingNavigationPhase(t *testing.T) {
	actionID := "route_navigation:phase-navigation"
	metadata := map[string]interface{}{
		"client_actions": []interface{}{
			map[string]interface{}{
				"action_id": actionID, "action_type": "route_navigation", "status": clientActionStatusWaiting,
				"skill_id": skills.SkillConsoleNavigator, "tool_name": "navigate", "href": "/console/agents", "plan_phase_id": "phase-navigation",
			},
		},
		"operation_plan": map[string]interface{}{
			"status": operationPlanStatusRunning,
			"phases": []interface{}{
				map[string]interface{}{
					"id": "phase-navigation", "status": "in_progress", "step": "open Agent management",
					"expected_action": map[string]interface{}{
						"skill_id": skills.SkillConsoleNavigator, "tool_name": "navigate", "target": map[string]interface{}{"href": "/console/agents"},
					},
				},
				map[string]interface{}{
					"id": "phase-update", "status": operationPlanStepStatusPending, "step": "update Agent",
					"expected_action": map[string]interface{}{"skill_id": skills.SkillAgentManagement, "tool_name": "update_agent_config"},
				},
			},
		},
	}

	resolved := resolveClientActionContinuationMetadata(metadata, actionID, runtimedto.ClientActionResultRequest{
		Status: clientActionStatusSucceeded,
		Result: map[string]interface{}{"observed_path": "/console/agents", "loaded_href": "/console/agents"},
	})
	plan := mapFromOperationContext(resolved["operation_plan"])
	phases := mapSliceFromAny(plan["phases"])
	if got := stringFromAny(phases[0]["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("navigation phase status = %q, want completed", got)
	}
	if got := stringFromAny(phases[1]["status"]); got != "in_progress" {
		t.Fatalf("business phase status = %q, want in_progress", got)
	}
	if got := stringFromAny(plan["status"]); got != operationPlanStatusRunning {
		t.Fatalf("plan status = %q, want running", got)
	}
}

func TestResolveClientActionContinuationInfersStructuredPhaseAfterCompletedPhase(t *testing.T) {
	actionID := "route_navigation:inferred"
	metadata := map[string]interface{}{
		"client_actions": []interface{}{map[string]interface{}{
			"action_id": actionID, "action_type": "route_navigation", "status": clientActionStatusWaiting,
			"skill_id": skills.SkillConsoleNavigator, "tool_name": "navigate", "href": "/console/agents",
		}},
		"operation_plan": map[string]interface{}{
			"status": operationPlanStatusRunning,
			"phases": []interface{}{
				map[string]interface{}{"id": "phase-done", "status": operationPlanStepStatusCompleted},
				map[string]interface{}{
					"id": "phase-navigation", "status": "in_progress",
					"expected_action": map[string]interface{}{
						"skill_id":  skills.SkillConsoleNavigator,
						"tool_name": "navigate",
						"target":    map[string]interface{}{"href": "/console/agents"},
					},
				},
			},
		},
	}

	resolved := resolveClientActionContinuationMetadata(metadata, actionID, runtimedto.ClientActionResultRequest{
		Status: clientActionStatusSucceeded,
		Result: map[string]interface{}{"observed_path": "/console/agents"},
	})
	phases := mapSliceFromAny(mapFromOperationContext(resolved["operation_plan"])["phases"])
	if got := stringFromAny(phases[1]["status"]); got != operationPlanStepStatusCompleted {
		t.Fatalf("navigation phase status = %q, want completed", got)
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
		"first model response after this continuation",
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
