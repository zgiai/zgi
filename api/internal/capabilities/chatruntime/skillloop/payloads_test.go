package skillloop

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestBlockedSkillPlanningFeedbackTraceIsNotGuardrail(t *testing.T) {
	trace := blockedSkillGuardrailTrace(skills.SkillFileReader, "read_file", "skill must be loaded before calling its tools")
	if trace.Kind != "planner_feedback" {
		t.Fatalf("Kind = %q, want planner_feedback", trace.Kind)
	}
	if trace.Status != "advisory" {
		t.Fatalf("Status = %q, want advisory", trace.Status)
	}
	if trace.Arguments["next_step"] != "load_skill" {
		t.Fatalf("next_step = %#v, want load_skill", trace.Arguments["next_step"])
	}
}

func TestClientActionRequiredPayloadEmitsObservationForPublishEffect(t *testing.T) {
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", nil)
	trace := skills.SkillTrace{
		SkillID:  "agent-manager",
		ToolName: "publish_agent",
		Status:   "success",
		Governance: &toolgovernance.Decision{
			Manifest: toolgovernance.Manifest{
				ToolID:    "agent.publish",
				Effect:    toolgovernance.EffectPublish,
				AssetType: "agent",
			},
			AssetOperationAudit: map[string]interface{}{
				"tool_id":    "agent.publish",
				"effect":     "publish",
				"asset_type": "agent",
				"assets": []interface{}{
					map[string]interface{}{
						"id":   "agent-1",
						"type": "agent",
						"name": "Support Agent",
					},
				},
			},
		},
	}

	payload := clientActionRequiredPayload(prepared, trace, "call-publish")
	if payload == nil {
		t.Fatal("clientActionRequiredPayload() = nil, want asset observation payload")
	}
	if payload["action_type"] != "asset_observation" ||
		payload["effect"] != "publish" ||
		payload["asset_type"] != "agent" {
		t.Fatalf("payload = %#v, want publish agent asset observation", payload)
	}
	if payload["refresh_before_resume"] != true || payload["observation_requested"] != true {
		t.Fatalf("payload = %#v, want refresh and observation flags", payload)
	}
	if payload["tool_id"] != "agent.publish" {
		t.Fatalf("tool_id = %#v, want agent.publish", payload["tool_id"])
	}
}

func TestClientActionRequiredPayloadSkipsPlainAgentCreateNavigation(t *testing.T) {
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", nil)
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "create_agent",
		Status:   "success",
		Result: map[string]interface{}{
			"status":   "completed",
			"effect":   "created",
			"agent_id": "agent-1",
		},
	}

	if payload := clientActionRequiredPayload(prepared, trace, "call-create"); payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil for plain Agent create", payload)
	}
}

func TestClientActionRequiredPayloadEmitsAgentCreateNavigationWhenRequested(t *testing.T) {
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", nil)
	prepared.Query = "\u521b\u5efa\u4e00\u4e2a\u667a\u80fd\u4f53\u5e76\u6253\u5f00\u8be6\u60c5\u9875"
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "create_agent",
		Status:   "success",
		Result: map[string]interface{}{
			"status":   "completed",
			"effect":   "created",
			"agent_id": "agent-1",
		},
	}

	payload := clientActionRequiredPayload(prepared, trace, "call-create")
	if payload == nil {
		t.Fatal("clientActionRequiredPayload() = nil, want route navigation payload")
	}
	if payload["action_type"] != "route_navigation" ||
		payload["skill_id"] != skills.SkillConsoleNavigator ||
		payload["tool_name"] != "navigate" ||
		payload["href"] != "/console/agents/agent-1/agent" ||
		payload["reason"] != "open_created_agent_detail" {
		t.Fatalf("payload = %#v, want created Agent detail navigation", payload)
	}
}

func TestClientActionRequiredPayloadEmitsAgentDeleteNavigationFromDetailPage(t *testing.T) {
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", nil)
	prepared.CurrentRoute = "/console/agents/agent-1/agent"
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agent",
		Status:   "success",
		Result: map[string]interface{}{
			"status":   "completed",
			"effect":   "deleted",
			"agent_id": "agent-1",
		},
	}

	payload := clientActionRequiredPayload(prepared, trace, "call-delete")
	if payload == nil {
		t.Fatal("clientActionRequiredPayload() = nil, want route navigation payload")
	}
	if payload["action_type"] != "route_navigation" ||
		payload["href"] != "/console/agents" ||
		payload["reason"] != "leave_deleted_agent_detail" {
		t.Fatalf("payload = %#v, want Agent list navigation after current detail delete", payload)
	}
}

func TestClientActionRequiredPayloadSkipsAgentDeleteNavigationFromListPage(t *testing.T) {
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", nil)
	prepared.CurrentRoute = "/console/agents"
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agent",
		Status:   "success",
		Result: map[string]interface{}{
			"status":     "completed",
			"effect":     "deleted",
			"agent_id":   "agent-1",
			"agent_name": "Old Agent",
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
					map[string]interface{}{"id": "agent-1", "type": "agent", "name": "Old Agent"},
				},
			},
		},
	}

	if payload := clientActionRequiredPayload(prepared, trace, "call-delete"); payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil for Agent list page deletion", payload)
	}
}

func TestClientActionRequiredPayloadSkipsFastPathAgentBatchDeleteObservation(t *testing.T) {
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", nil)
	prepared.CurrentRoute = "/console/agents"
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "delete_agents",
		Status:   "success",
		Result: map[string]interface{}{
			"status":         "completed",
			"operation_type": "agent.delete.batch",
			"target_count":   2,
			"deleted_count":  2,
			"operation_group": map[string]interface{}{
				"operation":     "agent.delete.batch",
				"success_count": 2,
				"item_results": []interface{}{
					map[string]interface{}{"status": "succeeded", "agent_name": "Agent A"},
					map[string]interface{}{"status": "succeeded", "agent_name": "Agent B"},
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
					map[string]interface{}{"id": "agent-1", "type": "agent", "name": "Agent A"},
					map[string]interface{}{"id": "agent-2", "type": "agent", "name": "Agent B"},
				},
			},
		},
	}

	if payload := clientActionRequiredPayload(prepared, trace, "call-delete-agents"); payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil because batch item results are enough for fast-path completion", payload)
	}
}

func TestClientActionRequiredPayloadSkipsAgentMemorySlotClientAction(t *testing.T) {
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", nil)
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "replace_agent_memory_slots",
		Status:   "success",
		Result: map[string]interface{}{
			"agent": map[string]interface{}{
				"id": "agent-1",
			},
		},
	}

	if payload := clientActionRequiredPayload(prepared, trace, "call-memory"); payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil for non-blocking Agent memory update", payload)
	}
}

func TestClientActionRequiredPayloadSkipsAgentBindingObservation(t *testing.T) {
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", nil)
	trace := skills.SkillTrace{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "replace_agent_knowledge_bindings",
		Status:   "success",
		Governance: &toolgovernance.Decision{
			Manifest: toolgovernance.Manifest{
				ToolID:    "agent.replace_knowledge_bindings",
				Effect:    toolgovernance.EffectUpdate,
				AssetType: "knowledge_base",
			},
			AssetOperationAudit: map[string]interface{}{
				"tool_id":    "agent.replace_knowledge_bindings",
				"effect":     "update",
				"asset_type": "knowledge_base",
				"assets": []interface{}{
					map[string]interface{}{
						"id":   "agent-1",
						"type": "agent",
						"name": "Support Agent",
					},
					map[string]interface{}{
						"id":   "dataset-1",
						"type": "knowledge_base",
						"name": "Policies",
					},
				},
			},
		},
	}

	if payload := clientActionRequiredPayload(prepared, trace, "call-bind"); payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil for non-blocking Agent binding update", payload)
	}
}

func TestClientActionRequiredPayloadSkipsTemporaryFileGeneration(t *testing.T) {
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", nil)
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
					map[string]interface{}{
						"id":   "tool-file-1",
						"type": "file",
						"name": "temporary.md",
					},
				},
			},
		},
	}

	if payload := clientActionRequiredPayload(prepared, trace, "call-generate"); payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil for temporary file generation", payload)
	}
}

func TestClientActionRequiredPayloadSkipsGenericTemporaryArtifactGeneration(t *testing.T) {
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", nil)
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
					map[string]interface{}{
						"id":   "tool-file-1",
						"type": "file",
						"name": "agents-distribution-pie.svg",
					},
				},
			},
		},
	}

	if payload := clientActionRequiredPayload(prepared, trace, "call-chart"); payload != nil {
		t.Fatalf("clientActionRequiredPayload() = %#v, want nil for generic temporary artifact generation", payload)
	}
}

func TestSummarizeSkillToolResultCompactsAgentKnowledgePayload(t *testing.T) {
	result := summarizeSkillToolResult("agent-knowledge", "retrieve_agent_knowledge", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"query":               "refund policy",
			"status":              "success",
			"result_count":        1,
			"top_score":           0.91,
			"source_summary":      []interface{}{map[string]interface{}{"position": 1, "dataset_name": "Policies"}},
			"context":             "full context should not be copied",
			"context_blocks":      []interface{}{map[string]interface{}{"content": "full block should not be copied"}},
			"retriever_resources": []interface{}{map[string]interface{}{"content": "full resource should not be copied"}},
		},
	}})
	if result["status"] != "success" {
		t.Fatalf("status = %#v, want success", result["status"])
	}
	if _, ok := result["source_summary"]; !ok {
		t.Fatalf("source_summary missing: %#v", result)
	}
	for _, key := range []string{"context", "context_blocks", "retriever_resources"} {
		if _, ok := result[key]; ok {
			t.Fatalf("%s should not be included in compact trace result: %#v", key, result)
		}
	}
}

func TestSummarizeSkillToolResultCompactsInternalKnowledgeListPayload(t *testing.T) {
	result := summarizeSkillToolResult("internal-knowledge", "list_accessible_knowledge_bases", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"query":           "refund",
			"status":          "fallback",
			"result_count":    1,
			"fallback_used":   true,
			"limit":           20,
			"warnings":        []interface{}{"no match"},
			"knowledge_bases": []interface{}{map[string]interface{}{"dataset_id": "ds-1", "name": "Policies"}},
		},
	}})
	if result["status"] != "fallback" {
		t.Fatalf("status = %#v, want fallback", result["status"])
	}
	if result["fallback_used"] != true {
		t.Fatalf("fallback_used = %#v, want true", result["fallback_used"])
	}
	if _, ok := result["knowledge_bases"]; ok {
		t.Fatalf("knowledge_bases should not be included in compact trace result: %#v", result)
	}
}
