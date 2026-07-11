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

func TestAgentManagementCreateResultFromMessagesRequiresExplicitNavigationTool(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &model.Conversation{ID: uuid.New()},
		Message:      &model.Message{ID: uuid.New()},
		parts: &chatRequestParts{
			Query: "create a draft Agent",
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:                 "manage_agent_asset",
				OpenCreatedAgentDetail: true,
			},
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

	if payload := clientActionRequiredPayload(prepared, trace, "call-create"); payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want model to call console navigation explicitly", payload)
	}
}

func TestAgentManagementCreateResultFromMessagesDoesNotUseQueryDetailFallbackWhenModelIntentExists(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &model.Conversation{ID: uuid.New()},
		Message:      &model.Message{ID: uuid.New()},
		parts: &chatRequestParts{
			Query: "\u521b\u5efa\u667a\u80fd\u4f53\u5e76\u8fdb\u5165\u8be6\u60c5\u9875",
			ModelTurnIntent: &AIChatModelTurnIntent{
				Intent:                 "manage_agent_asset",
				OpenCreatedAgentDetail: false,
			},
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
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil because model intent did not request detail navigation", payload)
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

func TestAgentManagementDeleteResultWithGovernanceRecordsObservation(t *testing.T) {
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

	payload := clientActionRequiredPayload(prepared, trace, "call-delete")
	if payload["action_type"] != "asset_observation" || payload["continuation_policy"] != clientActionContinuationPolicyRecordOnly {
		t.Fatalf("clientActionRequiredPayload() = %#v, want non-blocking asset observation", payload)
	}
}

func TestAgentManagementBatchDeleteWithGovernanceRecordsObservation(t *testing.T) {
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

	payload := clientActionRequiredPayload(prepared, trace, "call-delete-agents")
	if payload["action_type"] != "asset_observation" || payload["continuation_policy"] != clientActionContinuationPolicyRecordOnly {
		t.Fatalf("clientActionRequiredPayload() = %#v, want non-blocking batch asset observation", payload)
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
		"message":   "Artifact generation evidence is missing.",
		"error":     "Artifact generation evidence is missing.",
	}
	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{guardrail})
	metadata = mergeSkillInvocationMetadata(metadata, []map[string]interface{}{{
		"kind":      "guardrail",
		"skill_id":  skills.SkillFileGenerator,
		"tool_name": "generate_file",
		"status":    "blocked",
		"message":   "Artifact generation evidence is missing.",
		"error":     "Artifact generation evidence is missing.",
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
