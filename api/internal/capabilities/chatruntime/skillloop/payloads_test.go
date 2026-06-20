package skillloop

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

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
