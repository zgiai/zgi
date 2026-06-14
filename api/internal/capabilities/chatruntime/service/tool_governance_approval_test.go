package service

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func TestMergeSkillInvocationMetadataKeepsToolGovernanceTrace(t *testing.T) {
	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{
		{
			"kind":       "tool_governance",
			"skill_id":   "file-reader",
			"tool_name":  "delete_file",
			"status":     "needs_approval",
			"runtime_id": "governance:corr-1",
			"governance": map[string]interface{}{
				"status":             "needs_approval",
				"correlation_id":     "corr-1",
				"requires_approval":  true,
				"reason":             "delete effect requires user approval",
				"approval_event":     map[string]interface{}{"correlation_id": "corr-1"},
				"manifest":           map[string]interface{}{"effect": "delete", "asset_type": "file", "risk_level": "high"},
				"model_feedback":     map[string]interface{}{"status": "needs_approval"},
				"approval_status":    "",
				"approval_result":    map[string]interface{}{},
				"approval_requested": true,
			},
		},
	})

	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	if len(invocations) != 1 {
		t.Fatalf("skill_invocations len = %d, want 1", len(invocations))
	}
	if invocations[0]["kind"] != "tool_governance" {
		t.Fatalf("kind = %#v, want tool_governance", invocations[0]["kind"])
	}
	if metadata["skill_step_count"] != 1 {
		t.Fatalf("skill_step_count = %#v, want 1", metadata["skill_step_count"])
	}
}

func TestProcessTimelineRecorderPersistsToolGovernanceDecisionEvent(t *testing.T) {
	prepared := preparedTimelineTestChat()
	recorder := newProcessTimelineRecorder(nil, nil, &service{}, prepared, nil)

	recorder.RecordEvent(streamEventToolGovernanceDecision, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        "file-reader",
		"tool_name":       "delete_file",
		"status":          "needs_approval",
		"decision":        "needs_approval",
		"correlation_id":  "corr-1",
		"governance": map[string]interface{}{
			"status":            "needs_approval",
			"correlation_id":    "corr-1",
			"requires_approval": true,
			"approval_event": map[string]interface{}{
				"correlation_id": "corr-1",
				"tool_id":        "file.delete",
			},
		},
		"approval_event": map[string]interface{}{
			"correlation_id": "corr-1",
			"tool_id":        "file.delete",
		},
	})

	event, ok := toolGovernanceDecisionEventFromMetadata(prepared.Message.Metadata, "corr-1")
	if !ok {
		t.Fatalf("governance event not found in metadata: %#v", prepared.Message.Metadata)
	}
	if event["runtime_id"] != "tool_governance:corr-1" {
		t.Fatalf("runtime_id = %#v, want tool_governance:corr-1", event["runtime_id"])
	}
}

func TestToolGovernanceDecisionMetadataRecordsApprovalAndSessionGrant(t *testing.T) {
	conversationID := uuid.New().String()
	metadata := map[string]interface{}{
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":       "tool_governance",
				"skill_id":   "file-reader",
				"tool_name":  "delete_file",
				"status":     "needs_approval",
				"runtime_id": "governance:corr-1",
				"governance": map[string]interface{}{
					"status":            "needs_approval",
					"correlation_id":    "corr-1",
					"requires_approval": true,
					"approval_event": map[string]interface{}{
						"correlation_id": "corr-1",
						"tool_id":        "file.delete",
						"effect":         "delete",
						"asset_type":     "file",
						"risk_level":     "high",
						"grant": map[string]interface{}{
							"conversation_id": conversationID,
							"tool_id":         "file.delete",
							"effect":          "delete",
							"asset_type":      "file",
							"risk_level":      "high",
						},
					},
				},
			},
		},
	}

	event, ok := toolGovernanceDecisionEventFromMetadata(metadata, "corr-1")
	if !ok {
		t.Fatalf("governance event not found")
	}
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	grant := toolGovernanceSessionGrantFromEvent(event, conversationID, now)
	if grant["conversation_id"] != conversationID || grant["tool_id"] != "file.delete" {
		t.Fatalf("session grant = %#v, want conversation-bound file.delete grant", grant)
	}
	updated := resolvedToolGovernanceDecisionEvent(event, map[string]interface{}{
		"action":               "approve",
		"approval_status":      "approved",
		"resolved_at":          now.Format(time.RFC3339),
		"resolved_by":          uuid.New().String(),
		"remember_for_session": true,
		"session_grant":        grant,
	})
	metadata = mergeToolGovernanceDecisionMetadata(metadata, updated)

	records := mapSliceFromAny(metadata["tool_governance_decisions"])
	if len(records) != 1 || records[0]["approval_status"] != "approved" {
		t.Fatalf("tool_governance_decisions = %#v, want approved record", records)
	}
	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	governance := governanceMapFromAny(invocations[0]["governance"])
	if governance["approval_status"] != "approved" || governance["requires_approval"] != false {
		t.Fatalf("governance = %#v, want approved and not pending", governance)
	}

	conversationMetadata := appendToolGovernanceSessionGrant(nil, grant)
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.MustParse(conversationID), Metadata: conversationMetadata},
	}
	params := applySkillToolGovernanceRuntimeParameters(nil, prepared)
	nested := governanceMapFromAny(params[skillToolGovernanceRuntimeKey])
	grants := mapSliceFromAny(nested["session_grants"])
	if len(grants) != 1 || grants[0]["tool_id"] != "file.delete" {
		t.Fatalf("session grants = %#v, want file.delete grant", grants)
	}

	oneShotMetadata := appendToolGovernanceOneShotGrant(nil, grant)
	oneShotPrepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.MustParse(conversationID), Metadata: map[string]interface{}{}},
		Message:      &runtimemodel.Message{Metadata: oneShotMetadata},
	}
	params = applySkillToolGovernanceRuntimeParameters(nil, oneShotPrepared)
	nested = governanceMapFromAny(params[skillToolGovernanceRuntimeKey])
	grants = mapSliceFromAny(nested["session_grants"])
	if len(grants) != 1 || grants[0]["tool_id"] != "file.delete" {
		t.Fatalf("one-shot grants = %#v, want file.delete grant", grants)
	}
}

func TestToolGovernanceDecisionMetadataRecordsRejectionWithoutGrant(t *testing.T) {
	metadata := map[string]interface{}{
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":       "tool_governance",
				"skill_id":   "file-reader",
				"tool_name":  "delete_file",
				"status":     "needs_approval",
				"runtime_id": "governance:corr-1",
				"governance": map[string]interface{}{
					"status":            "needs_approval",
					"correlation_id":    "corr-1",
					"requires_approval": true,
					"approval_event": map[string]interface{}{
						"correlation_id": "corr-1",
						"tool_id":        "file.delete",
						"effect":         "delete",
						"asset_type":     "file",
						"risk_level":     "high",
					},
				},
			},
		},
	}

	event, ok := toolGovernanceDecisionEventFromMetadata(metadata, "corr-1")
	if !ok {
		t.Fatalf("governance event not found")
	}
	updated := resolvedToolGovernanceDecisionEvent(event, map[string]interface{}{
		"action":          "reject",
		"approval_status": "rejected",
		"resolved_at":     time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"resolved_by":     uuid.New().String(),
		"model_feedback": map[string]interface{}{
			"status":      "user_rejected",
			"instruction": "Do not execute it.",
		},
	})
	metadata = mergeToolGovernanceDecisionMetadata(metadata, updated)

	records := mapSliceFromAny(metadata["tool_governance_decisions"])
	if len(records) != 1 || records[0]["approval_status"] != "rejected" {
		t.Fatalf("tool_governance_decisions = %#v, want rejected record", records)
	}
	if len(mapSliceFromAny(metadata["tool_governance_one_shot_grants"])) != 0 {
		t.Fatalf("one-shot grants = %#v, want none for rejection", metadata["tool_governance_one_shot_grants"])
	}
	if len(mapSliceFromAny(metadata["tool_governance_session_grants"])) != 0 {
		t.Fatalf("session grants = %#v, want none for rejection", metadata["tool_governance_session_grants"])
	}
	invocations := skillInvocationsFromMetadata(metadata["skill_invocations"])
	governance := governanceMapFromAny(invocations[0]["governance"])
	if governance["approval_status"] != "rejected" || governance["requires_approval"] != false {
		t.Fatalf("governance = %#v, want rejected and not pending", governance)
	}
	result := governanceMapFromAny(governance["approval_result"])
	feedback := governanceMapFromAny(result["model_feedback"])
	if feedback["status"] != "user_rejected" {
		t.Fatalf("model_feedback = %#v, want user_rejected", feedback)
	}
}

func TestToolGovernanceRejectionLLMRequestCannotCallTools(t *testing.T) {
	provider := "deepseek"
	message := &runtimemodel.Message{
		Query:         "delete the first file",
		ModelName:     "deepseek-chat",
		ModelProvider: &provider,
		ModelParameters: map[string]interface{}{
			"temperature": 0.2,
		},
	}
	req := toolGovernanceRejectionLLMRequest(message, runtimedto.ToolGovernanceDecisionRequest{
		Action: "reject",
		Reason: "keep the file",
	}, map[string]interface{}{
		"correlation_id": "corr-1",
		"tool_id":        "file.delete",
	})

	if req == nil {
		t.Fatal("toolGovernanceRejectionLLMRequest() = nil")
	}
	if req.Provider != "deepseek" || req.Model != "deepseek-chat" || !req.Stream {
		t.Fatalf("request identity = provider %q model %q stream %v", req.Provider, req.Model, req.Stream)
	}
	if len(req.Tools) != 0 {
		t.Fatalf("rejection continuation tools = %#v, want none", req.Tools)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(req.Messages))
	}
	system := messageContentText(req.Messages[0].Content)
	user := messageContentText(req.Messages[1].Content)
	for _, want := range []string{
		"Do not execute or claim the rejected action",
		"offer safe alternatives",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("system prompt missing %q in %q", want, system)
		}
	}
	for _, want := range []string{
		"delete the first file",
		"keep the file",
		"corr-1",
		"file.delete",
	} {
		if !strings.Contains(user, want) {
			t.Fatalf("user payload missing %q in %q", want, user)
		}
	}
}

func TestToolGovernanceApprovalContinuationMessageScopesRetryToGrant(t *testing.T) {
	message := toolGovernanceApprovalContinuationMessage(map[string]interface{}{
		"correlation_id":  "corr-1",
		"approval_status": "approved",
		"governance": map[string]interface{}{
			"approval_result": map[string]interface{}{
				"approved_grant": map[string]interface{}{
					"conversation_id": "conv-1",
					"tool_id":         "file.delete",
					"effect":          "delete",
					"asset_type":      "file",
					"risk_level":      "high",
				},
			},
		},
	})

	content := messageContentText(message.Content)
	for _, want := range []string{
		"The user approved the pending tool governance request",
		"The approval is scoped to the governance grant",
		"Do not claim that the action succeeded",
		"corr-1",
		"file.delete",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("approval continuation prompt missing %q in %q", want, content)
		}
	}
}

func TestToolGovernanceSessionGrantKeyIncludesConversationToolEffectAssetAndRisk(t *testing.T) {
	metadata := appendToolGovernanceSessionGrant(nil, map[string]interface{}{
		"conversation_id": "conversation-1",
		"tool_id":         "file.delete",
		"effect":          "delete",
		"asset_type":      "file",
		"risk_level":      "high",
		"granted_at":      "2026-06-15T12:00:00Z",
	})
	metadata = appendToolGovernanceSessionGrant(metadata, map[string]interface{}{
		"conversation_id": "conversation-1",
		"tool_id":         "file.delete",
		"effect":          "delete",
		"asset_type":      "file",
		"risk_level":      "high",
		"granted_at":      "2026-06-15T12:05:00Z",
	})
	grants := mapSliceFromAny(metadata["tool_governance_session_grants"])
	if len(grants) != 1 {
		t.Fatalf("session grants = %#v, want duplicate five-field scope to replace", grants)
	}
	if grants[0]["granted_at"] != "2026-06-15T12:05:00Z" {
		t.Fatalf("session grant = %#v, want latest duplicate grant", grants[0])
	}

	for _, variant := range []map[string]interface{}{
		{"conversation_id": "conversation-2", "tool_id": "file.delete", "effect": "delete", "asset_type": "file", "risk_level": "high"},
		{"conversation_id": "conversation-1", "tool_id": "file.update", "effect": "delete", "asset_type": "file", "risk_level": "high"},
		{"conversation_id": "conversation-1", "tool_id": "file.delete", "effect": "update", "asset_type": "file", "risk_level": "high"},
		{"conversation_id": "conversation-1", "tool_id": "file.delete", "effect": "delete", "asset_type": "database", "risk_level": "high"},
		{"conversation_id": "conversation-1", "tool_id": "file.delete", "effect": "delete", "asset_type": "file", "risk_level": "medium"},
	} {
		metadata = appendToolGovernanceSessionGrant(metadata, variant)
	}
	grants = mapSliceFromAny(metadata["tool_governance_session_grants"])
	if len(grants) != 6 {
		t.Fatalf("session grants = %#v, want one base plus five distinct scoped grants", grants)
	}
}
