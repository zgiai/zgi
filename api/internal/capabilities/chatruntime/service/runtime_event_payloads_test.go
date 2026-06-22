package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
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
	if payload["refresh_before_resume"] != true || payload["observation_requested"] != true {
		t.Fatalf("payload = %#v, want refresh before resume", payload)
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
