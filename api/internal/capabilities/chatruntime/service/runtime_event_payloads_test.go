package service

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestClientActionRequiredPayloadEmitsManagedFileSaveObservation(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &model.Conversation{ID: uuid.New()},
		Message:      &model.Message{ID: uuid.New()},
	}
	trace := skills.SkillTrace{
		SkillID:  skills.SkillFileManager,
		ToolName: "save_file_to_management",
		Status:   "success",
		Governance: &toolgovernance.Decision{
			Manifest: toolgovernance.Manifest{
				ToolID:    "file.save_to_management",
				Effect:    toolgovernance.EffectCreate,
				AssetType: "file",
			},
			AssetOperationAudit: map[string]interface{}{
				"tool_id":    "file.save_to_management",
				"effect":     "create",
				"asset_type": "file",
				"assets": []interface{}{
					map[string]interface{}{"id": "file-1", "type": "file", "name": "saved.md"},
				},
			},
		},
	}

	payload := clientActionRequiredPayload(prepared, trace, "call-save")
	if payload == nil {
		t.Fatal("clientActionRequiredPayload() = nil, want asset observation payload")
	}
	if payload["action_type"] != "asset_observation" ||
		payload["effect"] != "create" ||
		payload["asset_type"] != "file" ||
		payload["tool_id"] != "file.save_to_management" {
		t.Fatalf("payload = %#v, want file create observation", payload)
	}
	if payload["continuation_policy"] != clientActionContinuationPolicyRecordOnly ||
		payload["status"] != clientActionStatusSucceeded ||
		payload["refresh_before_resume"] != false ||
		payload["observation_requested"] != true {
		t.Fatalf("payload = %#v, want record-only observation", payload)
	}
}

func TestAgentManagementCreateResultFromMessagesDoesNotEmitDetailNavigationByDefault(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &model.Conversation{ID: uuid.New()},
		Message:      &model.Message{ID: uuid.New()},
		parts: &chatRequestParts{
			Query: "\u521b\u5efa\u4e00\u4e2a\u667a\u80fd\u4f53",
		},
	}
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "create_agent",
		Status:   "success",
	}
	trace = enrichSkillTraceResultFromMessages(trace, []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":     "completed",
			"effect":     "created",
			"agent_id":   "agent-1",
			"agent_name": "Smoke Agent",
			"href":       "/console/agents/agent-1/agent",
		},
	}})

	payload := clientActionRequiredPayload(prepared, trace, "call-create")
	if payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil for plain create without open-detail request", payload)
	}
}

func TestAgentManagementCreateResultFromMessagesEmitsDetailNavigationWhenRequested(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &model.Conversation{ID: uuid.New()},
		Message:      &model.Message{ID: uuid.New()},
		parts: &chatRequestParts{
			Query: "\u521b\u5efa\u4e00\u4e2a\u667a\u80fd\u4f53\u5e76\u6253\u5f00\u8be6\u60c5\u9875",
		},
	}
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "create_agent",
		Status:   "success",
	}
	trace = enrichSkillTraceResultFromMessages(trace, []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":     "completed",
			"effect":     "created",
			"agent_id":   "agent-1",
			"agent_name": "Smoke Agent",
			"href":       "/console/agents/agent-1/agent",
		},
	}})

	payload := clientActionRequiredPayload(prepared, trace, "call-create")
	if payload == nil {
		t.Fatal("clientActionRequiredPayload() = nil, want route navigation payload")
	}
	if payload["action_type"] != "route_navigation" ||
		payload["skill_id"] != skills.SkillConsoleNavigator ||
		payload["tool_name"] != "navigate" ||
		payload["href"] != "/console/agents/agent-1/agent" ||
		payload["label_key"] != "agentDetail" ||
		payload["route_kind"] != "agent_detail" ||
		payload["reason"] != "open_created_agent_detail" {
		t.Fatalf("payload = %#v, want created Agent detail navigation", payload)
	}
	result, _ := payload["result"].(map[string]interface{})
	if result["label_key"] != "agentDetail" || result["route_kind"] != "agent_detail" {
		t.Fatalf("payload result = %#v, want created Agent detail label metadata", result)
	}
}

func TestAgentManagementUpdateIdentityResultDoesNotRequireClientAction(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &model.Conversation{ID: uuid.New()},
		Message:      &model.Message{ID: uuid.New()},
		parts:        &chatRequestParts{Query: "\u4fee\u6539\u8fd9\u4e2a\u667a\u80fd\u4f53\u540d\u79f0"},
	}
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_identity",
		Status:   "success",
		Result: map[string]interface{}{
			"status":   "completed",
			"effect":   "updated",
			"agent_id": "agent-1",
			"agent": map[string]interface{}{
				"id":   "agent-1",
				"name": "Updated Agent",
			},
		},
	}

	payload := clientActionRequiredPayload(prepared, trace, "call-update")
	if payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil for non-blocking Agent update", payload)
	}
}

func TestAgentManagementUpdateConfigResultDoesNotRequireClientAction(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &model.Conversation{ID: uuid.New()},
		Message:      &model.Message{ID: uuid.New()},
		parts:        &chatRequestParts{Query: "\u66f4\u65b0\u8fd9\u4e2a\u667a\u80fd\u4f53\u7684\u7cfb\u7edf\u63d0\u793a\u8bcd"},
	}
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "update_agent_config",
		Status:   "success",
		Result: map[string]interface{}{
			"status":   "completed",
			"effect":   "updated",
			"agent_id": "agent-1",
		},
	}

	payload := clientActionRequiredPayload(prepared, trace, "call-config")
	if payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil for non-blocking Agent config update", payload)
	}
}

func TestAgentManagementReplaceMemorySlotsResultDoesNotRequireClientAction(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &model.Conversation{ID: uuid.New()},
		Message:      &model.Message{ID: uuid.New()},
		parts:        &chatRequestParts{Query: "update this Agent memory slots"},
	}
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "replace_agent_memory_slots",
		Status:   "success",
		Result: map[string]interface{}{
			"status":   "completed",
			"effect":   "updated",
			"agent_id": "agent-1",
		},
	}

	payload := clientActionRequiredPayload(prepared, trace, "call-memory")
	if payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil for non-blocking Agent memory update", payload)
	}
}

func TestAgentManagementBindingResultDoesNotRequireClientAction(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &model.Conversation{ID: uuid.New()},
		Message:      &model.Message{ID: uuid.New()},
		parts:        &chatRequestParts{Query: "bind this Agent to a knowledge base"},
	}
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "replace_agent_knowledge_bindings",
		Status:   "success",
		Result: map[string]interface{}{
			"status":          "completed",
			"effect":          "updated",
			"binding_kind":    "knowledge_base",
			"resource_count":  1,
			"resource_names":  []string{"测试库2"},
			"knowledge_count": 1,
		},
		Governance: &toolgovernance.Decision{
			Assets: []toolgovernance.AssetRef{
				{
					ID:   "agent-1",
					Type: "agent",
					Name: "Support Agent",
					Metadata: map[string]interface{}{
						"binding_owner": true,
					},
				},
				{
					ID:   "dataset-1",
					Type: "knowledge_base",
					Name: "测试库2",
				},
			},
			Manifest: toolgovernance.Manifest{
				ToolID:    "agent.replace_knowledge_bindings",
				Effect:    toolgovernance.EffectUpdate,
				AssetType: "knowledge_base",
			},
			AssetOperationAudit: map[string]interface{}{
				"effect":     "update",
				"asset_type": "knowledge_base",
			},
		},
	}

	payload := clientActionRequiredPayload(prepared, trace, "call-bind-knowledge")
	if payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil for non-blocking Agent binding update", payload)
	}
}

func TestAgentManagementBindingResultEventUsesResourceNames(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &model.Conversation{ID: uuid.New()},
		Message:      &model.Message{ID: uuid.New()},
	}
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "replace_agent_workflow_bindings",
		Status:   "success",
	}
	trace = enrichSkillTraceResultFromMessages(trace, []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":     "completed",
			"effect":     "updated",
			"agent_id":   "agent-1",
			"agent_name": "Support Agent",
			"bindings": []interface{}{map[string]interface{}{
				"binding_id":  "binding-1",
				"workflow_id": "workflow-1",
				"label":       "Approval Flow",
			}},
		},
	}})

	payload := skillCallEndPayload(prepared, trace)
	result, ok := payload["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("result = %#v, want compact binding summary", payload["result"])
	}
	names, ok := result["resource_names"].([]string)
	if !ok || len(names) != 1 || names[0] != "Approval Flow" {
		t.Fatalf("resource_names = %#v, want Approval Flow", result["resource_names"])
	}
	for _, rawID := range []string{"agent-1", "binding-1", "workflow-1"} {
		if eventSummaryContainsString(result, rawID) {
			t.Fatalf("event result contains raw id %q: %#v", rawID, result)
		}
	}
}

func TestAgentManagementDeleteResultEmitsListNavigationFromAgentPage(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &model.Conversation{ID: uuid.New()},
		Message:      &model.Message{ID: uuid.New()},
		parts: &chatRequestParts{
			Query:          "\u5220\u9664\u5f53\u524d\u667a\u80fd\u4f53",
			RuntimeContext: "route=/console/agents/agent-1/agent",
		},
	}
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agent",
		Status:   "success",
		Result: map[string]interface{}{
			"status":             "completed",
			"effect":             "deleted",
			"agent_id":           "agent-1",
			"route_after_delete": "/console/agents",
		},
	}

	payload := clientActionRequiredPayload(prepared, trace, "call-delete")
	if payload == nil {
		t.Fatal("clientActionRequiredPayload() = nil, want route navigation payload")
	}
	if payload["action_type"] != "route_navigation" ||
		payload["href"] != "/console/agents" ||
		payload["reason"] != "leave_deleted_agent_detail" {
		t.Fatalf("payload = %#v, want Agent list navigation", payload)
	}
}

func TestAgentManagementDeleteResultFromListPageDoesNotNavigate(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &model.Conversation{ID: uuid.New()},
		Message:      &model.Message{ID: uuid.New()},
		parts: &chatRequestParts{
			Query:          "\u5220\u9664\u5217\u8868\u91cc\u7684\u667a\u80fd\u4f53",
			RuntimeContext: "route=/console/agents",
		},
	}
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agent",
		Status:   "success",
		Result: map[string]interface{}{
			"status":             "completed",
			"effect":             "deleted",
			"agent_id":           "agent-1",
			"route_after_delete": "/console/agents",
		},
	}

	if payload := clientActionRequiredPayload(prepared, trace, "call-delete"); payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil for Agent list page deletion", payload)
	}
}

func TestAgentManagementDeleteResultWithGovernanceFastPathsWithoutObservation(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &model.Conversation{ID: uuid.New()},
		Message:      &model.Message{ID: uuid.New()},
		parts: &chatRequestParts{
			Query:          "delete Agent One from the list page",
			RuntimeContext: "route=/console/agents",
		},
	}
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agent",
		Status:   "success",
		Result: map[string]interface{}{
			"status":   "completed",
			"effect":   "deleted",
			"agent_id": "agent-1",
			"agent": map[string]interface{}{
				"name": "Agent One",
			},
		},
		Governance: &toolgovernance.Decision{
			Manifest: toolgovernance.Manifest{
				ToolID:    "agent.delete",
				Effect:    toolgovernance.EffectDelete,
				AssetType: "agent",
			},
			AssetOperationAudit: map[string]interface{}{
				"tool_id":    "agent.delete",
				"effect":     "delete",
				"asset_type": "agent",
				"assets": []interface{}{
					map[string]interface{}{"id": "agent-1", "type": "agent", "name": "Agent One"},
				},
			},
		},
	}

	if payload := clientActionRequiredPayload(prepared, trace, "call-delete"); payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil because delete result is enough for fast-path completion", payload)
	}
}

func TestAgentManagementBatchDeleteWithGovernanceFastPathsWithoutObservation(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &model.Conversation{ID: uuid.New()},
		Message:      &model.Message{ID: uuid.New()},
		parts: &chatRequestParts{
			Query:          "delete the first two visible Agents",
			RuntimeContext: "route=/console/agents",
		},
	}
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agents",
		Status:   "success",
		Result: map[string]interface{}{
			"status":        "completed",
			"effect":        "deleted",
			"target_count":  2,
			"deleted_count": 2,
			"failed_count":  0,
			"operation_group": map[string]interface{}{
				"operation":     "agent.delete",
				"target_count":  2,
				"success_count": 2,
				"failed_count":  0,
				"item_results": []interface{}{
					map[string]interface{}{"status": "succeeded", "agent_name": "Agent One"},
					map[string]interface{}{"status": "succeeded", "agent_name": "Agent Two"},
				},
			},
		},
		Governance: &toolgovernance.Decision{
			Manifest: toolgovernance.Manifest{
				ToolID:    "agent.delete.batch",
				Effect:    toolgovernance.EffectDelete,
				AssetType: "agent",
			},
			AssetOperationAudit: map[string]interface{}{
				"tool_id":    "agent.delete.batch",
				"effect":     "delete",
				"asset_type": "agent",
				"assets": []interface{}{
					map[string]interface{}{"id": "agent-1", "type": "agent", "name": "Agent One"},
					map[string]interface{}{"id": "agent-2", "type": "agent", "name": "Agent Two"},
				},
			},
		},
	}

	if payload := clientActionRequiredPayload(prepared, trace, "call-delete-agents"); payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil because batch item_results are enough for fast-path completion", payload)
	}
}

func TestConsoleNavigationResolvedTargetsRecognizesChineseRouteAliases(t *testing.T) {
	targets := consoleNavigationResolvedTargets("请先切到文件管理页，然后再切回智能体页")
	if len(targets) != 2 {
		t.Fatalf("targets = %#v, want two resolved routes", targets)
	}
	if targets[0].Href != "/console/files" || targets[1].Href != "/console/agents" {
		t.Fatalf("targets = %#v, want files then agents", targets)
	}
}

func TestAgentManagementIntentIgnoresNegatedFileMutationConstraint(t *testing.T) {
	query := "\u8bf7\u5148\u5207\u5230\u667a\u80fd\u4f53\u9875\uff0c\u7136\u540e\u518d\u5207\u56de\u6587\u4ef6\u7ba1\u7406\u9875\uff1b\u4e0d\u8981\u521b\u5efa\u3001\u4fdd\u5b58\u3001\u5220\u9664\u6587\u4ef6\u3002"
	if isAgentManagementIntent(query) {
		t.Fatal("isAgentManagementIntent() = true, want false for navigation with negated file mutation constraint")
	}
	if isFileDeleteIntent(query) {
		t.Fatal("isFileDeleteIntent() = true, want false for negated delete constraint")
	}
	assetQuery := "\u8bf7\u8fde\u7eed\u5bfc\u822a\u5e76\u89c2\u5bdf\u9875\u9762\uff0c\u4e0d\u8981\u521b\u5efa\u6216\u5220\u9664\u4efb\u4f55\u8d44\u4ea7\u3002"
	if isFileDeleteIntent(assetQuery) {
		t.Fatal("isFileDeleteIntent() = true, want false for negated asset delete constraint")
	}
	deleteWithTargetFreezeQuery := "\u8bf7\u5148\u57fa\u4e8e\u5f53\u524d\u6587\u4ef6\u7ba1\u7406\u9875\u9762\u6700\u65b0\u53ef\u89c1\u5217\u8868\u51bb\u7ed3\u201c\u5f53\u524d\u7b2c\u4e09\u4e2a\u6587\u4ef6\u201d\u4e3a\u5220\u9664\u76ee\u6807\uff0c\u7136\u540e\u5220\u9664\u8fd9\u4e2a\u51bb\u7ed3\u76ee\u6807\u3002\u6700\u7ec8\u56de\u7b54\u8bf7\u5199\u51fa\u88ab\u51bb\u7ed3\u5e76\u5220\u9664\u7684\u6587\u4ef6\u540d\uff0c\u4e14\u4e0d\u8981\u91cd\u65b0\u89e3\u6790\u7b2c\u4e09\u4e2a\u6587\u4ef6\u3002"
	if !isFileDeleteIntent(deleteWithTargetFreezeQuery) {
		t.Fatal("isFileDeleteIntent() = false, want true for delete request with target-freeze negation")
	}
	if isTemporaryFileGenerateIntent(deleteWithTargetFreezeQuery) {
		t.Fatal("isTemporaryFileGenerateIntent() = true, want false for delete request with target-freeze negation")
	}
}

func TestAgentManagementIntentIgnoresFileContentAgentNameReference(t *testing.T) {
	query := "\u518d\u8fdb\u5165\u6587\u4ef6\u7ba1\u7406\uff0c\u521b\u5efa\u5e76\u4fdd\u5b58 smoke.txt\uff1btxt \u5185\u5bb9\u5199\u5165\u8bfb\u53d6\u5230\u7684\u667a\u80fd\u4f53\u540d\u79f0"
	if isAgentManagementIntent(query) {
		t.Fatal("isAgentManagementIntent() = true, want false for file content referencing an Agent name")
	}
	if !isManagedFileCreateIntent(query) {
		t.Fatal("isManagedFileCreateIntent() = false, want true for managed file create")
	}
}

func TestAgentManagementIntentRecognizesNearbyAgentMutation(t *testing.T) {
	for _, query := range []string{
		"\u4fee\u6539\u667a\u80fd\u4f53\u540d\u79f0",
		"\u628a\u7b2c\u4e00\u4e2a\u667a\u80fd\u4f53\u7684\u56fe\u6807\u6539\u6389",
		"\u8bf7\u628a\u5f53\u524d\u667a\u80fd\u4f53\u7684\u56fe\u8868\u751f\u6210\u5668 Skill \u89e3\u7ed1/\u505c\u7528",
		"agent.update_identity",
	} {
		if !isAgentManagementIntent(query) {
			t.Fatalf("isAgentManagementIntent(%q) = false, want true", query)
		}
	}
}

func TestAgentManagementIntentRecognizesConfigEditWithFileUploadAndSkillCandidate(t *testing.T) {
	query := "edit current agent config; enable \u6587\u4ef6\u4e0a\u4f20 and memory; call list_available_models with use_case text-chat; call list_agent_skill_candidates with query \u56fe\u8868\u751f\u6210\u5668 and add that skill; \u4e0d\u8981\u7ed1\u5b9a\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u6216\u5de5\u4f5c\u6d41"
	if !isAgentManagementIntent(query) {
		t.Fatal("isAgentManagementIntent() = false, want true for explicit Agent config tool flow")
	}
	if got := requiredAgentBindingMutationTools(query); len(got) != 0 {
		t.Fatalf("requiredAgentBindingMutationTools() = %#v, want no knowledge/database/workflow binding tools for explicit no-bind constraint", got)
	}
}

func TestClientActionContinuationMessageIncludesPrecedingToolResult(t *testing.T) {
	actionID := "route_navigation:call-delete"
	message := &model.Message{
		Query: "delete current agent",
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":      "tool_call",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "delete_agent",
					"status":    "success",
					"result": map[string]interface{}{
						"status":     "completed",
						"effect":     "deleted",
						"agent_name": "Smoke Agent",
					},
				},
				map[string]interface{}{
					"kind":        "client_action",
					"action_id":   actionID,
					"action_type": "route_navigation",
					"status":      clientActionStatusWaiting,
				},
			},
		},
	}
	event := map[string]interface{}{
		"action_id":   actionID,
		"action_type": "route_navigation",
		"reason":      "leave_deleted_agent_detail",
		"href":        "/console/agents",
	}
	req := runtimedto.ClientActionResultRequest{Status: clientActionStatusSucceeded}

	got := clientActionContinuationMessage(message, event, req)
	content := stringFromAny(got.Content)
	if !strings.Contains(content, "Current-turn tool result immediately before this frontend action") ||
		!strings.Contains(content, "authoritative evidence for the current user request") ||
		!strings.Contains(content, "delete_agent") ||
		!strings.Contains(content, "Smoke Agent") {
		t.Fatalf("clientActionContinuationMessage() content = %q, want preceding delete result", got.Content)
	}
	if strings.Contains(content, "Previously completed tool call") {
		t.Fatalf("clientActionContinuationMessage() content = %q, want current-turn wording", got.Content)
	}
}

func TestClientActionContinuationMessageIncludesCompletedClientActions(t *testing.T) {
	actionID := "route_navigation:call-files"
	message := &model.Message{
		Query: "先切到智能体页，再到文件管理创建文件",
		Metadata: map[string]interface{}{
			"client_actions": []interface{}{
				map[string]interface{}{
					"action_id":   "route_navigation:call-agents",
					"action_type": "route_navigation",
					"status":      clientActionStatusSucceeded,
					"href":        "/console/agents",
					"label":       "Agents",
					"reason":      "读取第一个智能体名称",
					"result": map[string]interface{}{
						"context_items": []interface{}{
							map[string]interface{}{"type": "page", "title": "Agents", "id": "console.agents", "href": "/console/agents"},
							map[string]interface{}{"type": "agent", "title": "AIChat配置验证06231035-已编辑", "id": "agent-1", "href": "/console/agents/agent-1/agent"},
						},
						"context_item_count": 2,
					},
				},
				map[string]interface{}{
					"action_id":   actionID,
					"action_type": "route_navigation",
					"status":      clientActionStatusSucceeded,
					"href":        "/console/files",
					"label":       "Files",
					"reason":      "创建测试文件",
				},
			},
		},
	}
	event := map[string]interface{}{
		"action_id":   actionID,
		"action_type": "route_navigation",
		"href":        "/console/files",
		"label":       "Files",
	}
	req := runtimedto.ClientActionResultRequest{Status: clientActionStatusSucceeded}

	got := clientActionContinuationMessage(message, event, req)
	content := stringFromAny(got.Content)
	for _, want := range []string{
		"Completed client actions in this same AIChat turn",
		"/console/agents",
		"/console/files",
		"AIChat配置验证06231035-已编辑",
		"Continue from the next unfinished step instead of restarting the original plan",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("clientActionContinuationMessage() missing %q in:\n%s", want, content)
		}
	}
}

func TestMergeSkillInvocationMetadataDeduplicatesGuardrail(t *testing.T) {
	guardrail := map[string]interface{}{
		"kind":      "guardrail",
		"skill_id":  skills.SkillFileGenerator,
		"tool_name": "generate_file",
		"status":    "blocked",
		"message":   "Use file-generator instead of chart-generator.",
		"error":     "Use file-generator instead of chart-generator.",
	}
	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{guardrail})
	metadata = mergeSkillInvocationMetadata(metadata, []map[string]interface{}{{
		"kind":      "guardrail",
		"skill_id":  skills.SkillFileGenerator,
		"tool_name": "generate_file",
		"status":    "blocked",
		"message":   "Use file-generator instead of chart-generator.",
		"error":     "Use file-generator instead of chart-generator.",
	}})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	if len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want one deduplicated guardrail", invocations)
	}
	if metadata["guardrail_count"] != 1 {
		t.Fatalf("guardrail_count = %#v, want 1", metadata["guardrail_count"])
	}
}

func TestClientActionRequiredPayloadSkipsTemporaryFileGeneration(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &model.Conversation{ID: uuid.New()},
		Message:      &model.Message{ID: uuid.New()},
	}
	trace := skills.SkillTrace{
		SkillID:  skills.SkillFileGenerator,
		ToolName: "generate_file",
		Status:   "success",
		Governance: &toolgovernance.Decision{
			Manifest: toolgovernance.Manifest{
				ToolID:    "file.generate",
				Effect:    toolgovernance.EffectCreate,
				AssetType: "file",
			},
			AssetOperationAudit: map[string]interface{}{
				"tool_id":    "file.generate",
				"effect":     "create",
				"asset_type": "file",
				"assets": []interface{}{
					map[string]interface{}{"id": "tool-file-1", "type": "file", "name": "temporary.md"},
				},
			},
		},
	}

	if payload := clientActionRequiredPayload(prepared, trace, "call-generate"); payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil for temporary file generation", payload)
	}
}

func TestClientActionRequiredPayloadSkipsGenericTemporaryArtifactGeneration(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &model.Conversation{ID: uuid.New()},
		Message:      &model.Message{ID: uuid.New()},
	}
	trace := skills.SkillTrace{
		SkillID:  skills.SkillChartGenerator,
		ToolName: "generate_chart",
		Status:   "success",
		Result: map[string]interface{}{
			"artifact_type":   "file",
			"target":          "temporary_artifact",
			"transfer_method": "tool_file",
			"tool_file_id":    "tool-file-1",
			"filename":        "agents-distribution-pie.svg",
		},
		Governance: &toolgovernance.Decision{
			Manifest: toolgovernance.Manifest{
				ToolID:    "chart.generate",
				Effect:    toolgovernance.EffectCreate,
				AssetType: "file",
			},
			AssetOperationAudit: map[string]interface{}{
				"tool_id":    "chart.generate",
				"effect":     "create",
				"asset_type": "file",
				"assets": []interface{}{
					map[string]interface{}{"id": "tool-file-1", "type": "file", "name": "agents-distribution-pie.svg"},
				},
			},
		},
	}

	if payload := clientActionRequiredPayload(prepared, trace, "call-chart"); payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil for generic temporary artifact generation", payload)
	}
}

func eventSummaryContainsString(value interface{}, needle string) bool {
	switch typed := value.(type) {
	case string:
		return typed == needle
	case map[string]interface{}:
		for _, item := range typed {
			if eventSummaryContainsString(item, needle) {
				return true
			}
		}
	case []string:
		for _, item := range typed {
			if eventSummaryContainsString(item, needle) {
				return true
			}
		}
	case []interface{}:
		for _, item := range typed {
			if eventSummaryContainsString(item, needle) {
				return true
			}
		}
	}
	return false
}
