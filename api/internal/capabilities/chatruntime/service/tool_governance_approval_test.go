package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
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
		"asset_operation_audit": map[string]interface{}{
			"schema_version":    "tool_governance.asset_operation.v1",
			"correlation_id":    "corr-1",
			"tool_id":           "file.delete",
			"effect":            "delete",
			"asset_type":        "file",
			"approval_status":   "pending",
			"governance_status": "needs_approval",
		},
	})

	event, ok := toolGovernanceDecisionEventFromMetadata(prepared.Message.Metadata, "corr-1")
	if !ok {
		t.Fatalf("governance event not found in metadata: %#v", prepared.Message.Metadata)
	}
	if event["runtime_id"] != "tool_governance:corr-1" {
		t.Fatalf("runtime_id = %#v, want tool_governance:corr-1", event["runtime_id"])
	}
	audit := governanceMapFromAny(event["asset_operation_audit"])
	if audit["tool_id"] != "file.delete" || audit["approval_status"] != "pending" {
		t.Fatalf("asset_operation_audit = %#v, want persisted audit payload", audit)
	}
	records := mapSliceFromAny(prepared.Message.Metadata["tool_governance_decisions"])
	if len(records) != 1 {
		t.Fatalf("tool_governance_decisions len = %d in %#v, want 1", len(records), prepared.Message.Metadata["tool_governance_decisions"])
	}
	if records[0]["runtime_id"] != "tool_governance:corr-1" {
		t.Fatalf("decision runtime_id = %#v, want tool_governance:corr-1", records[0]["runtime_id"])
	}

	recorder.RecordEvent(streamEventToolGovernanceDecision, map[string]interface{}{
		"conversation_id":   prepared.Conversation.ID.String(),
		"message_id":        prepared.Message.ID.String(),
		"skill_id":          "file-reader",
		"tool_name":         "delete_file",
		"status":            "success",
		"correlation_id":    "corr-1",
		"approval_status":   "approved",
		"requires_approval": false,
		"governance": map[string]interface{}{
			"status":            "needs_approval",
			"correlation_id":    "corr-1",
			"requires_approval": false,
			"approval_status":   "approved",
			"approval_event": map[string]interface{}{
				"correlation_id": "corr-1",
				"tool_id":        "file.delete",
			},
		},
		"approval_event": map[string]interface{}{
			"correlation_id": "corr-1",
			"tool_id":        "file.delete",
		},
		"asset_operation_audit": map[string]interface{}{
			"schema_version":    "tool_governance.asset_operation.v1",
			"correlation_id":    "corr-1",
			"tool_id":           "file.delete",
			"effect":            "delete",
			"asset_type":        "file",
			"approval_status":   "approved",
			"governance_status": "needs_approval",
		},
	})

	records = mapSliceFromAny(prepared.Message.Metadata["tool_governance_decisions"])
	if len(records) != 1 {
		t.Fatalf("tool_governance_decisions len after update = %d in %#v, want 1", len(records), prepared.Message.Metadata["tool_governance_decisions"])
	}
	if records[0]["approval_status"] != "approved" {
		t.Fatalf("decision approval_status = %#v, want approved", records[0]["approval_status"])
	}
	audit = governanceMapFromAny(records[0]["asset_operation_audit"])
	if audit["approval_status"] != "approved" || audit["tool_id"] != "file.delete" {
		t.Fatalf("updated asset_operation_audit = %#v, want approved file.delete audit", audit)
	}
	invocations := skillInvocationsFromMetadata(prepared.Message.Metadata["skill_invocations"])
	if len(invocations) != 1 {
		t.Fatalf("skill_invocations len = %d in %#v, want one updated governance invocation", len(invocations), prepared.Message.Metadata["skill_invocations"])
	}
	if invocations[0]["approval_status"] != "approved" {
		t.Fatalf("invocation approval_status = %#v, want approved", invocations[0]["approval_status"])
	}
}

func TestProcessTimelineRecorderMarksGovernedToolCallWaitingApproval(t *testing.T) {
	prepared := preparedTimelineTestChat()
	recorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, nil)

	recorder.RecordEvent(streamEventSkillCallStart, map[string]interface{}{
		"conversation_id":   prepared.Conversation.ID.String(),
		"message_id":        prepared.Message.ID.String(),
		"skill_id":          "file-reader",
		"tool_name":         "delete_file",
		"arguments_summary": map[string]interface{}{"file_id": "file-1"},
	})
	recorder.RecordEvent(streamEventToolGovernanceDecision, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        "file-reader",
		"tool_name":       "delete_file",
		"status":          "needs_approval",
		"decision":        "needs_approval",
		"correlation_id":  "corr-wait",
		"approval_status": "pending",
		"approval_event":  map[string]interface{}{"correlation_id": "corr-wait", "tool_id": "file.delete"},
		"asset_operation_audit": map[string]interface{}{
			"schema_version":  "tool_governance.asset_operation.v1",
			"correlation_id":  "corr-wait",
			"tool_id":         "file.delete",
			"effect":          "delete",
			"asset_type":      "file",
			"approval_status": "pending",
		},
		"governance": map[string]interface{}{
			"status":            "needs_approval",
			"correlation_id":    "corr-wait",
			"requires_approval": true,
			"approval_status":   "pending",
			"approval_event":    map[string]interface{}{"correlation_id": "corr-wait", "tool_id": "file.delete"},
		},
	})

	var toolCall map[string]interface{}
	var governanceTrace map[string]interface{}
	for _, invocation := range skillInvocationsFromMetadata(prepared.Message.Metadata["skill_invocations"]) {
		if invocation["kind"] == "tool_call" {
			toolCall = invocation
		}
		if invocation["kind"] == "tool_governance" {
			governanceTrace = invocation
		}
	}
	if toolCall == nil || governanceTrace == nil {
		t.Fatalf("skill_invocations = %#v, want tool_call and tool_governance", prepared.Message.Metadata["skill_invocations"])
	}
	if toolCall["status"] != "waiting_approval" {
		t.Fatalf("tool_call status = %#v, want waiting_approval", toolCall["status"])
	}
	if toolGovernanceCorrelationID(toolGovernanceDecisionEventFromInvocation(toolCall)) != "corr-wait" {
		t.Fatalf("tool_call governance = %#v, want corr-wait", toolCall["governance"])
	}

	event, ok := toolGovernanceDecisionEventFromMetadata(prepared.Message.Metadata, "corr-wait")
	if !ok {
		t.Fatalf("tool governance event not found in %#v", prepared.Message.Metadata)
	}
	metadata := mergeToolGovernanceDecisionMetadata(prepared.Message.Metadata, resolvedToolGovernanceDecisionEvent(event, map[string]interface{}{
		"action":          "reject",
		"approval_status": "rejected",
		"reason":          "regression guard",
	}))
	for _, invocation := range skillInvocationsFromMetadata(metadata["skill_invocations"]) {
		if invocation["kind"] == "tool_call" {
			toolCall = invocation
			break
		}
	}
	if toolCall["status"] != "rejected" || toolCall["approval_status"] != "rejected" {
		t.Fatalf("tool_call after rejection = %#v, want rejected approval trace", toolCall)
	}
}

func TestToolGovernanceDecisionMetadataRecordsApprovalAndSessionGrant(t *testing.T) {
	conversationID := uuid.New().String()
	organizationID := uuid.New()
	accountID := uuid.New()
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
						"assets": []interface{}{
							map[string]interface{}{
								"id":           "file-1",
								"type":         "file",
								"name":         "smoke.txt",
								"workspace_id": "workspace-1",
							},
						},
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
	grant := toolGovernanceSessionGrantFromEvent(event, conversationID, Scope{OrganizationID: organizationID, AccountID: accountID}, now)
	if grant["conversation_id"] != conversationID || grant["tool_id"] != "file.delete" {
		t.Fatalf("session grant = %#v, want conversation-bound file.delete grant", grant)
	}
	if grant["organization_id"] != organizationID.String() || grant["user_id"] != accountID.String() {
		t.Fatalf("session grant = %#v, want scope-bound grant", grant)
	}
	if grant["approval_correlation_id"] != "corr-1" {
		t.Fatalf("session grant = %#v, want approval correlation", grant)
	}
	if grant["granted_at"] != now.Format(time.RFC3339) {
		t.Fatalf("session grant = %#v, want granted_at", grant)
	}
	if grant["expires_at"] != now.Add(toolgovernance.DefaultSessionGrantTTL).Format(time.RFC3339) {
		t.Fatalf("session grant = %#v, want default expiry", grant)
	}
	grantAssets := mapSliceFromAny(grant["assets"])
	if len(grantAssets) != 1 || grantAssets[0]["id"] != "file-1" || grantAssets[0]["workspace_id"] != "workspace-1" {
		t.Fatalf("session grant assets = %#v, want approved file asset", grantAssets)
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
	if len(grants) != 1 || grants[0]["tool_id"] != "file.delete" || grants[0]["approval_correlation_id"] != "corr-1" {
		t.Fatalf("session grants = %#v, want file.delete grant", grants)
	}
	if assets := mapSliceFromAny(grants[0]["assets"]); len(assets) != 1 || assets[0]["id"] != "file-1" {
		t.Fatalf("session grant assets = %#v, want approved file", assets)
	}

	oneShotMetadata := appendToolGovernanceOneShotGrant(nil, grant)
	oneShotPrepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.MustParse(conversationID), Metadata: map[string]interface{}{}},
		Message:      &runtimemodel.Message{Metadata: oneShotMetadata},
	}
	params = applySkillToolGovernanceRuntimeParameters(nil, oneShotPrepared)
	nested = governanceMapFromAny(params[skillToolGovernanceRuntimeKey])
	grants = mapSliceFromAny(nested["session_grants"])
	if len(grants) != 1 || grants[0]["tool_id"] != "file.delete" || grants[0]["approval_correlation_id"] != "corr-1" {
		t.Fatalf("one-shot grants = %#v, want file.delete grant", grants)
	}
	if assets := mapSliceFromAny(grants[0]["assets"]); len(assets) != 1 || assets[0]["id"] != "file-1" {
		t.Fatalf("one-shot grant assets = %#v, want approved file", assets)
	}
}

func TestToolGovernanceSessionGrantFromEventFallsBackToGovernanceAssetsAndCompacts(t *testing.T) {
	conversationID := uuid.New().String()
	event := map[string]interface{}{
		"correlation_id": "corr-1",
		"governance": map[string]interface{}{
			"assets": []interface{}{
				map[string]interface{}{
					"file_id":      "file-1",
					"asset_type":   "file",
					"file_name":    "smoke.txt",
					"workspace_id": "workspace-1",
					"source":       "resolver",
					"metadata":     map[string]interface{}{"created_by": "account-1"},
				},
			},
			"approval_event": map[string]interface{}{
				"correlation_id": "corr-1",
				"tool_id":        "file.delete",
				"effect":         "delete",
				"asset_type":     "file",
				"risk_level":     "high",
				"grant": map[string]interface{}{
					"tool_id":    "file.delete",
					"effect":     "delete",
					"asset_type": "file",
					"risk_level": "high",
				},
			},
		},
	}

	grant := toolGovernanceSessionGrantFromEvent(event, conversationID, Scope{}, time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC))
	assets := mapSliceFromAny(grant["assets"])
	if len(assets) != 1 {
		t.Fatalf("grant assets = %#v, want one compact asset", assets)
	}
	for key, want := range map[string]interface{}{
		"id":           "file-1",
		"type":         "file",
		"name":         "smoke.txt",
		"workspace_id": "workspace-1",
		"source":       "resolver",
	} {
		if assets[0][key] != want {
			t.Fatalf("asset %s = %#v, want %#v in %#v", key, assets[0][key], want, assets[0])
		}
	}
	if _, ok := assets[0]["metadata"]; ok {
		t.Fatalf("grant asset should not persist metadata: %#v", assets[0])
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

func TestResolvedToolGovernanceDecisionEventUpdatesAssetOperationAudit(t *testing.T) {
	event := map[string]interface{}{
		"correlation_id": "corr-1",
		"governance": map[string]interface{}{
			"status":            "needs_approval",
			"requires_approval": true,
			"asset_operation_audit": map[string]interface{}{
				"schema_version":    "tool_governance.asset_operation.v1",
				"correlation_id":    "corr-1",
				"tool_id":           "file.delete",
				"effect":            "delete",
				"asset_type":        "file",
				"approval_status":   "pending",
				"governance_status": "needs_approval",
				"assets": []interface{}{
					map[string]interface{}{"id": "file-1", "type": "file", "name": "smoke.txt"},
				},
			},
		},
	}
	resolvedBy := uuid.New().String()
	updated := resolvedToolGovernanceDecisionEvent(event, map[string]interface{}{
		"action":          "approve",
		"approval_status": "approved",
		"resolved_at":     "2026-06-15T12:00:00Z",
		"resolved_by":     resolvedBy,
		"approved_grant": map[string]interface{}{
			"conversation_id":         "conversation-1",
			"tool_id":                 "file.delete",
			"effect":                  "delete",
			"asset_type":              "file",
			"risk_level":              "high",
			"approval_correlation_id": "corr-1",
		},
	})

	audit := governanceMapFromAny(updated["asset_operation_audit"])
	if audit["approval_status"] != "approved" || audit["action"] != "approve" || audit["resolved_by"] != resolvedBy {
		t.Fatalf("audit = %#v, want approved resolution metadata", audit)
	}
	if audit["approved_by_correlation_id"] != "corr-1" {
		t.Fatalf("audit = %#v, want approval correlation", audit)
	}
	nested := governanceMapFromAny(governanceMapFromAny(updated["governance"])["asset_operation_audit"])
	if nested["approval_status"] != "approved" || nested["resolved_at"] != "2026-06-15T12:00:00Z" {
		t.Fatalf("nested audit = %#v, want updated audit payload", nested)
	}
}

func TestResolvedToolGovernanceDecisionEventBuildsApprovalResultAndAuditFallback(t *testing.T) {
	event := map[string]interface{}{
		"conversation_id":   "conversation-1",
		"correlation_id":    "corr-1",
		"skill_id":          "file-reader",
		"tool_name":         "delete_file",
		"status":            "needs_approval",
		"decision":          "needs_approval",
		"requires_approval": true,
		"governance": map[string]interface{}{
			"status":            "needs_approval",
			"correlation_id":    "corr-1",
			"requires_approval": true,
			"reason":            "delete requires approval",
			"manifest": map[string]interface{}{
				"skill_id":       "file-reader",
				"tool_id":        "file.delete",
				"effect":         "delete",
				"asset_type":     "file",
				"risk_level":     "high",
				"audit_required": true,
			},
			"approval_event": map[string]interface{}{
				"correlation_id": "corr-1",
				"tool_id":        "file.delete",
				"skill_id":       "file-reader",
				"effect":         "delete",
				"asset_type":     "file",
				"risk_level":     "high",
				"assets": []interface{}{
					map[string]interface{}{
						"id":           "file-1",
						"type":         "file",
						"name":         "smoke.txt",
						"workspace_id": "workspace-1",
					},
				},
				"grant": map[string]interface{}{
					"conversation_id": "conversation-1",
					"tool_id":         "file.delete",
					"effect":          "delete",
					"asset_type":      "file",
					"risk_level":      "high",
				},
			},
		},
	}
	updated := resolvedToolGovernanceDecisionEvent(event, map[string]interface{}{
		"action":               "approve",
		"approval_status":      "approved",
		"reason":               "ok",
		"resolved_at":          "2026-06-15T12:00:00Z",
		"resolved_by":          "account-1",
		"remember_for_session": true,
		"approved_grant": map[string]interface{}{
			"conversation_id":         "conversation-1",
			"tool_id":                 "file.delete",
			"effect":                  "delete",
			"asset_type":              "file",
			"risk_level":              "high",
			"approval_correlation_id": "corr-1",
			"assets": []interface{}{
				map[string]interface{}{"id": "file-1", "type": "file", "name": "smoke.txt", "workspace_id": "workspace-1"},
			},
		},
		"session_grant": map[string]interface{}{
			"conversation_id":         "conversation-1",
			"tool_id":                 "file.delete",
			"effect":                  "delete",
			"asset_type":              "file",
			"risk_level":              "high",
			"approval_correlation_id": "corr-1",
			"assets": []interface{}{
				map[string]interface{}{"id": "file-1", "type": "file", "name": "smoke.txt", "workspace_id": "workspace-1"},
			},
		},
	})

	governance := governanceMapFromAny(updated["governance"])
	result := governanceMapFromAny(governance["approval_result"])
	if result["correlation_id"] != "corr-1" ||
		result["tool_id"] != "file.delete" ||
		result["effect"] != "delete" ||
		result["asset_type"] != "file" ||
		result["risk_level"] != "high" ||
		result["remember_for_session"] != true {
		t.Fatalf("approval_result = %#v, want correlated approval scope", result)
	}
	resultAssets := mapSliceFromAny(result["assets"])
	if len(resultAssets) != 1 || resultAssets[0]["id"] != "file-1" || resultAssets[0]["workspace_id"] != "workspace-1" {
		t.Fatalf("approval_result assets = %#v, want approved file asset", resultAssets)
	}

	audit := governanceMapFromAny(updated["asset_operation_audit"])
	if audit["schema_version"] != "tool_governance.asset_operation.v1" ||
		audit["correlation_id"] != "corr-1" ||
		audit["approval_status"] != "approved" ||
		audit["action"] != "approve" ||
		audit["resolved_at"] != "2026-06-15T12:00:00Z" ||
		audit["resolved_by"] != "account-1" ||
		audit["remember_for_session"] != true {
		t.Fatalf("asset_operation_audit = %#v, want resolved fallback audit", audit)
	}
	auditAssets := mapSliceFromAny(audit["assets"])
	if len(auditAssets) != 1 || auditAssets[0]["id"] != "file-1" || auditAssets[0]["workspace_id"] != "workspace-1" {
		t.Fatalf("audit assets = %#v, want approved file asset", auditAssets)
	}
	if governanceMapFromAny(audit["approved_grant"])["approval_correlation_id"] != "corr-1" ||
		governanceMapFromAny(audit["session_grant"])["approval_correlation_id"] != "corr-1" {
		t.Fatalf("audit grants = approved %#v session %#v, want replay grants", audit["approved_grant"], audit["session_grant"])
	}
}

func TestSubmitToolGovernanceDecisionRejectsUnresolvedEventWhenMessageNotWaitingApproval(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	messageRepo := &toolGovernanceDecisionMessageRepo{
		message: &runtimemodel.Message{
			ID:             messageID,
			ConversationID: conversationID,
			Status:         runtimemodel.MessageStatusCompleted,
			Metadata:       pendingToolGovernanceDecisionMetadata("corr-1"),
		},
	}
	svc := &service{
		repos: &repository.Repositories{
			Access: toolGovernanceDecisionAccessRepo{},
			Conversation: toolGovernanceDecisionConversationRepo{
				conversation: &runtimemodel.Conversation{
					ID:             conversationID,
					OrganizationID: organizationID,
					AccountID:      accountID,
				},
			},
			Message: messageRepo,
		},
	}

	_, err := svc.SubmitToolGovernanceDecision(context.Background(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, conversationID, messageID, "corr-1", runtimedto.ToolGovernanceDecisionRequest{Action: "approve"})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("SubmitToolGovernanceDecision() error = %v, want ErrInvalidInput", err)
	}
	if messageRepo.updateMetadataAnyStatusCalled {
		t.Fatalf("UpdateMetadataAnyStatus was called for a non-waiting unresolved approval")
	}
}

func TestSubmitToolGovernanceDecisionRejectsNonApprovalGovernanceEvent(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	metadata := pendingToolGovernanceDecisionMetadata("corr-1")
	invocation := metadata["skill_invocations"].([]interface{})[0].(map[string]interface{})
	invocation["status"] = "success"
	governance := invocation["governance"].(map[string]interface{})
	governance["status"] = "allowed"
	governance["requires_approval"] = false
	messageRepo := &toolGovernanceDecisionMessageRepo{
		message: &runtimemodel.Message{
			ID:             messageID,
			ConversationID: conversationID,
			Status:         runtimemodel.MessageStatusWaitingApproval,
			Metadata:       metadata,
		},
	}
	svc := &service{
		repos: &repository.Repositories{
			Access: toolGovernanceDecisionAccessRepo{},
			Conversation: toolGovernanceDecisionConversationRepo{
				conversation: &runtimemodel.Conversation{
					ID:             conversationID,
					OrganizationID: organizationID,
					AccountID:      accountID,
				},
			},
			Message: messageRepo,
		},
	}

	_, err := svc.SubmitToolGovernanceDecision(context.Background(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, conversationID, messageID, "corr-1", runtimedto.ToolGovernanceDecisionRequest{Action: "approve"})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("SubmitToolGovernanceDecision() error = %v, want ErrInvalidInput", err)
	}
	if messageRepo.updateMetadataAnyStatusCalled {
		t.Fatalf("UpdateMetadataAnyStatus was called for a non-approval governance event")
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
		"tool_name":       "delete_file",
		"governance": map[string]interface{}{
			"approval_event": map[string]interface{}{
				"tool_id":    "file.delete",
				"skill_id":   "file-reader",
				"effect":     "delete",
				"asset_type": "file",
				"assets": []interface{}{
					map[string]interface{}{
						"id":   "file-1",
						"type": "file",
						"name": "smoke.txt",
					},
				},
			},
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
		"authoritative asset resolution",
		"do not ask the user to identify the approved assets again",
		"Approved assets: smoke.txt type=file id=file-1",
		"call file-reader/delete_file with file_id equal to the approved file asset id",
		"Do not claim that the action succeeded",
		"corr-1",
		"file.delete",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("approval continuation prompt missing %q in %q", want, content)
		}
	}
}

func TestToolGovernanceApprovedFinalAnswerGuardBlocksUntilDeleteToolAttempted(t *testing.T) {
	event := map[string]interface{}{
		"correlation_id":  "corr-1",
		"approval_status": "approved",
		"tool_name":       "delete_file",
		"governance": map[string]interface{}{
			"approval_event": map[string]interface{}{
				"tool_id":    "file.delete",
				"skill_id":   "file-reader",
				"effect":     "delete",
				"asset_type": "file",
				"assets": []interface{}{
					map[string]interface{}{
						"id":   "file-1",
						"type": "file",
						"name": "smoke.txt",
					},
				},
			},
		},
	}

	guard := toolGovernanceApprovedFinalAnswerGuard(event)
	if guard == nil {
		t.Fatal("toolGovernanceApprovedFinalAnswerGuard() = nil, want guard")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "The file was deleted.",
	})
	if !blocked {
		t.Fatal("guard did not block final answer before delete_file was attempted")
	}
	for _, want := range []string{
		"approval is not the operation itself",
		"delete_file",
		"smoke.txt (file-1)",
	} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("guard message missing %q in %q", want, result.Message)
		}
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "The delete_file tool failed because the file was already missing.",
		AttemptedToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: skills.SkillFileReader, ToolName: "delete_file"},
		},
	})
	if blocked {
		t.Fatal("guard blocked after delete_file was attempted")
	}
}

func TestToolGovernanceApprovedFinalAnswerGuardBlocksGenericApprovedTool(t *testing.T) {
	guard := toolGovernanceApprovedFinalAnswerGuard(map[string]interface{}{
		"tool_name": "publish_agent",
		"governance": map[string]interface{}{
			"approval_event": map[string]interface{}{
				"tool_id":    "agent.publish",
				"skill_id":   "agent-manager",
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
	})
	if guard == nil {
		t.Fatal("toolGovernanceApprovedFinalAnswerGuard() = nil, want generic approved tool guard")
	}
	result, blocked := guard(skillloop.FinalAnswerGuardRequest{
		Answer: "The agent has been published.",
	})
	if !blocked {
		t.Fatal("guard did not block final answer before approved tool was attempted")
	}
	for _, want := range []string{
		"approval is not the operation itself",
		"agent-manager",
		"publish_agent",
		"Support Agent (agent-1)",
	} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("guard message missing %q in %q", want, result.Message)
		}
	}

	_, blocked = guard(skillloop.FinalAnswerGuardRequest{
		Answer: "The publish_agent tool failed.",
		AttemptedToolCalls: []skillloop.SkillToolCallRef{
			{SkillID: "agent-manager", ToolName: "publish_agent"},
		},
	})
	if blocked {
		t.Fatal("guard blocked after approved tool was attempted")
	}
}

func pendingToolGovernanceDecisionMetadata(correlationID string) map[string]interface{} {
	return map[string]interface{}{
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_governance",
				"skill_id":  "file-reader",
				"tool_name": "delete_file",
				"status":    "needs_approval",
				"governance": map[string]interface{}{
					"status":            "needs_approval",
					"correlation_id":    correlationID,
					"requires_approval": true,
					"approval_event": map[string]interface{}{
						"correlation_id": correlationID,
						"tool_id":        "file.delete",
						"skill_id":       "file-reader",
						"effect":         "delete",
						"asset_type":     "file",
						"risk_level":     "high",
						"grant": map[string]interface{}{
							"conversation_id": "conversation-1",
							"tool_id":         "file.delete",
							"effect":          "delete",
							"asset_type":      "file",
							"risk_level":      "high",
						},
					},
				},
			},
		},
		"tool_governance_continuation": map[string]interface{}{
			"status":         "waiting_approval",
			"correlation_id": correlationID,
			"skill_id":       "file-reader",
			"tool_name":      "delete_file",
			"original_query": "delete file",
			"resume_policy":  "same_message",
		},
	}
}

type toolGovernanceDecisionAccessRepo struct {
	repository.AccessRepository
}

func (toolGovernanceDecisionAccessRepo) IsOrganizationMember(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

type toolGovernanceDecisionConversationRepo struct {
	repository.ConversationRepository
	conversation *runtimemodel.Conversation
}

func (r toolGovernanceDecisionConversationRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Conversation, error) {
	return r.conversation, nil
}

type toolGovernanceDecisionMessageRepo struct {
	repository.MessageRepository
	message                       *runtimemodel.Message
	updateMetadataAnyStatusCalled bool
}

func (r *toolGovernanceDecisionMessageRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Message, error) {
	return r.message, nil
}

func (r *toolGovernanceDecisionMessageRepo) UpdateMetadataAnyStatus(context.Context, uuid.UUID, map[string]interface{}) error {
	r.updateMetadataAnyStatusCalled = true
	return nil
}

func TestToolGovernanceSessionGrantKeyIncludesConversationToolEffectAssetAndRisk(t *testing.T) {
	metadata := appendToolGovernanceSessionGrant(nil, map[string]interface{}{
		"conversation_id": "conversation-1",
		"tool_id":         "file.delete",
		"effect":          "delete",
		"asset_type":      "file",
		"risk_level":      "high",
		"granted_at":      "2026-06-15T12:00:00Z",
		"assets": []interface{}{
			map[string]interface{}{"id": "file-1", "type": "file"},
		},
	})
	metadata = appendToolGovernanceSessionGrant(metadata, map[string]interface{}{
		"conversation_id": "conversation-1",
		"tool_id":         "file.delete",
		"effect":          "delete",
		"asset_type":      "file",
		"risk_level":      "high",
		"granted_at":      "2026-06-15T12:05:00Z",
		"assets": []interface{}{
			map[string]interface{}{"id": "file-1", "type": "file"},
		},
	})
	grants := mapSliceFromAny(metadata["tool_governance_session_grants"])
	if len(grants) != 1 {
		t.Fatalf("session grants = %#v, want duplicate scoped grant to replace", grants)
	}
	if grants[0]["granted_at"] != "2026-06-15T12:05:00Z" {
		t.Fatalf("session grant = %#v, want latest duplicate grant", grants[0])
	}

	for _, variant := range []map[string]interface{}{
		{"conversation_id": "conversation-2", "tool_id": "file.delete", "effect": "delete", "asset_type": "file", "risk_level": "high"},
		{"conversation_id": "conversation-1", "organization_id": "organization-2", "tool_id": "file.delete", "effect": "delete", "asset_type": "file", "risk_level": "high"},
		{"conversation_id": "conversation-1", "user_id": "user-2", "tool_id": "file.delete", "effect": "delete", "asset_type": "file", "risk_level": "high"},
		{"conversation_id": "conversation-1", "skill_id": "other-skill", "tool_id": "file.delete", "effect": "delete", "asset_type": "file", "risk_level": "high"},
		{"conversation_id": "conversation-1", "provider_type": "custom", "tool_id": "file.delete", "effect": "delete", "asset_type": "file", "risk_level": "high"},
		{"conversation_id": "conversation-1", "provider_id": "other-provider", "tool_id": "file.delete", "effect": "delete", "asset_type": "file", "risk_level": "high"},
		{"conversation_id": "conversation-1", "tool_id": "file.update", "effect": "delete", "asset_type": "file", "risk_level": "high"},
		{"conversation_id": "conversation-1", "tool_id": "file.delete", "effect": "update", "asset_type": "file", "risk_level": "high"},
		{"conversation_id": "conversation-1", "tool_id": "file.delete", "effect": "delete", "asset_type": "database", "risk_level": "high"},
		{"conversation_id": "conversation-1", "tool_id": "file.delete", "effect": "delete", "asset_type": "file", "risk_level": "medium"},
		{"conversation_id": "conversation-1", "tool_id": "file.delete", "effect": "delete", "asset_type": "file", "risk_level": "high", "assets": []interface{}{map[string]interface{}{"id": "file-2", "type": "file"}}},
	} {
		metadata = appendToolGovernanceSessionGrant(metadata, variant)
	}
	grants = mapSliceFromAny(metadata["tool_governance_session_grants"])
	if len(grants) != 12 {
		t.Fatalf("session grants = %#v, want one base plus eleven distinct scoped grants", grants)
	}
}

func TestToolGovernanceSessionGrantKeyCanonicalizesAssetOrder(t *testing.T) {
	metadata := appendToolGovernanceSessionGrant(nil, map[string]interface{}{
		"conversation_id": "conversation-1",
		"tool_id":         "file.delete",
		"effect":          "delete",
		"asset_type":      "file",
		"risk_level":      "high",
		"granted_at":      "2026-06-15T12:00:00Z",
		"assets": []interface{}{
			map[string]interface{}{"id": "file-1", "type": "file"},
			map[string]interface{}{"id": "file-2", "type": "file"},
		},
	})
	metadata = appendToolGovernanceSessionGrant(metadata, map[string]interface{}{
		"conversation_id": "conversation-1",
		"tool_id":         "file.delete",
		"effect":          "delete",
		"asset_type":      "file",
		"risk_level":      "high",
		"granted_at":      "2026-06-15T12:05:00Z",
		"assets": []interface{}{
			map[string]interface{}{"id": "file-2", "type": "file"},
			map[string]interface{}{"id": "file-1", "type": "file"},
		},
	})

	grants := mapSliceFromAny(metadata["tool_governance_session_grants"])
	if len(grants) != 1 {
		t.Fatalf("session grants = %#v, want asset order to canonicalize", grants)
	}
	if grants[0]["granted_at"] != "2026-06-15T12:05:00Z" {
		t.Fatalf("session grant = %#v, want latest duplicate grant", grants[0])
	}
}
