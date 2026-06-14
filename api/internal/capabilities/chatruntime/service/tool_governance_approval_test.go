package service

import (
	"testing"
	"time"

	"github.com/google/uuid"
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
