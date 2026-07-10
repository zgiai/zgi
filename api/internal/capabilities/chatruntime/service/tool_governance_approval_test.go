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
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
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
	if records[0]["status"] != "approved" || records[0]["decision"] != "approved" {
		t.Fatalf("decision status/decision = %#v/%#v, want approved/approved", records[0]["status"], records[0]["decision"])
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
				"skill_id":   "file-manager",
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
	if governance["status"] != "approved" {
		t.Fatalf("governance status = %#v, want approved", governance["status"])
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

func TestApplySkillToolGovernanceRuntimeParametersMirrorsLegacyFileReaderDeleteGrant(t *testing.T) {
	conversationID := uuid.New().String()
	legacyGrant := map[string]interface{}{
		"conversation_id":         conversationID,
		"organization_id":         uuid.New().String(),
		"user_id":                 uuid.New().String(),
		"skill_id":                skills.SkillFileReader,
		"provider_type":           "builtin",
		"provider_id":             "files",
		"tool_id":                 "file.delete",
		"effect":                  "delete",
		"asset_type":              "file",
		"risk_level":              "high",
		"approval_correlation_id": "corr-legacy",
		"expires_at":              time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		"assets": []interface{}{
			map[string]interface{}{"id": "file-1", "type": "file", "name": "report.pdf", "workspace_id": "workspace-1"},
		},
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{
			ID:       uuid.MustParse(conversationID),
			Metadata: appendToolGovernanceSessionGrant(nil, legacyGrant),
		},
	}

	params := applySkillToolGovernanceRuntimeParameters(nil, prepared)
	nested := governanceMapFromAny(params[skillToolGovernanceRuntimeKey])
	grants := mapSliceFromAny(nested["session_grants"])
	if len(grants) != 2 {
		t.Fatalf("runtime session grants = %#v, want legacy reader and mirrored manager grants", grants)
	}
	seen := map[string]bool{}
	for _, grant := range grants {
		seen[stringFromAny(grant["skill_id"])] = true
		if grant["tool_id"] != "file.delete" || grant["approval_correlation_id"] != "corr-legacy" {
			t.Fatalf("runtime grant = %#v, want same file.delete approval scope", grant)
		}
	}
	if !seen[skills.SkillFileReader] || !seen[skills.SkillFileManager] {
		t.Fatalf("runtime session grants = %#v, want reader legacy plus manager mirror", grants)
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

func TestSubmitToolGovernanceDecisionRejectCannotOverwriteApprovedDecision(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	conversation := &runtimemodel.Conversation{
		ID:             conversationID,
		OrganizationID: organizationID,
		AccountID:      accountID,
		CallerType:     runtimemodel.ConversationCallerAIChat,
		Title:          "Files",
		Status:         runtimemodel.ConversationStatusNormal,
		RuntimeStatus:  runtimemodel.ConversationRuntimeStatusIdle,
		Metadata:       map[string]interface{}{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	message := &runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Query:          "Delete report.pdf",
		Status:         runtimemodel.MessageStatusWaitingApproval,
		ModelName:      "deepseek-chat",
		Metadata:       pendingToolGovernanceDecisionMetadata("corr-1"),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	messageRepo := &toolGovernanceDecisionMessageRepo{message: message}
	svc := &service{
		repos: &repository.Repositories{
			Access:       toolGovernanceDecisionAccessRepo{},
			Conversation: toolGovernanceDecisionConversationRepo{conversation: conversation},
			Message:      messageRepo,
		},
	}

	if _, err := svc.SubmitToolGovernanceDecision(ctx, Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, conversationID, messageID, "corr-1", runtimedto.ToolGovernanceDecisionRequest{Action: "approve"}); err != nil {
		t.Fatalf("approve decision: %v", err)
	}
	_, err := svc.SubmitToolGovernanceDecision(ctx, Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, conversationID, messageID, "corr-1", runtimedto.ToolGovernanceDecisionRequest{Action: "reject", Reason: "deny it"})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("reject after approve error = %v, want ErrInvalidInput", err)
	}

	event, ok := toolGovernanceDecisionEventFromMetadata(message.Metadata, "corr-1")
	if !ok {
		t.Fatalf("stored governance event missing in %#v", message.Metadata)
	}
	if event["approval_status"] != toolGovernanceApprovalStatusApproved {
		t.Fatalf("approval_status = %#v, want approved after rejected request lost the claim", event["approval_status"])
	}
	if grants := mapSliceFromAny(message.Metadata["tool_governance_one_shot_grants"]); len(grants) != 1 {
		t.Fatalf("one-shot grants = %#v, want one approved grant", grants)
	}
	continuation := governanceMapFromAny(message.Metadata["tool_governance_continuation"])
	if continuation["approval_status"] != toolGovernanceApprovalStatusApproved {
		t.Fatalf("continuation = %#v, want approved status preserved", continuation)
	}
}

func TestSubmitToolGovernanceDecisionRememberForSessionPreservesExistingConversationGrants(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	existingGrant := map[string]interface{}{
		"conversation_id":         conversationID.String(),
		"organization_id":         organizationID.String(),
		"user_id":                 accountID.String(),
		"skill_id":                skills.SkillFileReader,
		"provider_type":           "builtin",
		"provider_id":             "files",
		"tool_id":                 "file.delete",
		"effect":                  "delete",
		"asset_type":              "file",
		"risk_level":              "high",
		"approval_correlation_id": "corr-existing",
		"expires_at":              now.Add(time.Hour).Format(time.RFC3339),
		"assets": []interface{}{
			map[string]interface{}{"id": "file-existing", "type": "file", "workspace_id": "workspace-1"},
		},
	}
	conversation := &runtimemodel.Conversation{
		ID:             conversationID,
		OrganizationID: organizationID,
		AccountID:      accountID,
		CallerType:     runtimemodel.ConversationCallerAIChat,
		Title:          "Files",
		Status:         runtimemodel.ConversationStatusNormal,
		RuntimeStatus:  runtimemodel.ConversationRuntimeStatusIdle,
		Metadata:       appendToolGovernanceSessionGrant(nil, existingGrant),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	metadata := pendingToolGovernanceDecisionMetadata("corr-1")
	invocation := metadata["skill_invocations"].([]interface{})[0].(map[string]interface{})
	governance := invocation["governance"].(map[string]interface{})
	approvalEvent := governance["approval_event"].(map[string]interface{})
	approvalEvent["assets"] = []interface{}{
		map[string]interface{}{"id": "file-new", "type": "file", "workspace_id": "workspace-1"},
	}
	approvalGrant := approvalEvent["grant"].(map[string]interface{})
	approvalGrant["conversation_id"] = conversationID.String()
	approvalGrant["organization_id"] = organizationID.String()
	approvalGrant["user_id"] = accountID.String()
	approvalGrant["skill_id"] = skills.SkillFileManager
	approvalGrant["provider_type"] = "builtin"
	approvalGrant["provider_id"] = "files"
	approvalGrant["assets"] = []interface{}{
		map[string]interface{}{"id": "file-new", "type": "file", "workspace_id": "workspace-1"},
	}
	message := &runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Query:          "Delete file-new",
		Status:         runtimemodel.MessageStatusWaitingApproval,
		ModelName:      "deepseek-chat",
		Metadata:       metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	svc := &service{
		repos: &repository.Repositories{
			Access:       toolGovernanceDecisionAccessRepo{},
			Conversation: toolGovernanceDecisionConversationRepo{conversation: conversation},
			Message:      &toolGovernanceDecisionMessageRepo{message: message},
		},
	}

	response, err := svc.SubmitToolGovernanceDecision(ctx, Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, conversationID, messageID, "corr-1", runtimedto.ToolGovernanceDecisionRequest{Action: "approve", RememberForSession: true})
	if err != nil {
		t.Fatalf("SubmitToolGovernanceDecision() error = %v", err)
	}
	if response.SessionGrant == nil || response.SessionGrant["approval_correlation_id"] != "corr-1" {
		t.Fatalf("response session grant = %#v, want corr-1", response.SessionGrant)
	}
	grants := mapSliceFromAny(conversation.Metadata["tool_governance_session_grants"])
	if len(grants) != 2 {
		t.Fatalf("conversation session grants = %#v, want existing and new grants", grants)
	}
	seen := map[string]bool{}
	for _, grant := range grants {
		seen[stringFromAny(grant["approval_correlation_id"])] = true
	}
	if !seen["corr-existing"] || !seen["corr-1"] {
		t.Fatalf("conversation session grants = %#v, want both corr-existing and corr-1", grants)
	}
}

func TestCompleteToolGovernanceContinuationMetadataMarksApprovedContinuationCompleted(t *testing.T) {
	metadata := map[string]interface{}{
		"tool_governance_continuation": map[string]interface{}{
			"status":          "approved",
			"approval_status": "approved",
			"correlation_id":  "corr-1",
			"skill_id":        skills.SkillAgentManagement,
			"tool_name":       "create_agent",
		},
	}
	updated := completeToolGovernanceContinuationMetadata(metadata)
	continuation := governanceMapFromAny(updated["tool_governance_continuation"])
	if continuation["status"] != "completed" || continuation["approval_status"] != "approved" {
		t.Fatalf("continuation = %#v, want completed approved continuation", continuation)
	}
	if strings.TrimSpace(stringFromAny(continuation["completed_at"])) == "" {
		t.Fatalf("continuation = %#v, want completed_at", continuation)
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
		"Answer in the user's language",
		"\u672a\u6267\u884c",
		"offer safe alternatives",
		"Do not expose internal IDs",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("system prompt missing %q in %q", want, system)
		}
	}
	for _, want := range []string{
		"delete the first file",
		"keep the file",
		`"action":"delete"`,
		"not_executed",
	} {
		if !strings.Contains(user, want) {
			t.Fatalf("user payload missing %q in %q", want, user)
		}
	}
	for _, hidden := range []string{
		"corr-1",
		"Rejected governance event JSON",
	} {
		if strings.Contains(user, hidden) {
			t.Fatalf("user payload exposed %q in %q", hidden, user)
		}
	}
}

func TestToolGovernanceExecutionResultLLMRequestUsesModelVisibleSummary(t *testing.T) {
	provider := "deepseek"
	message := &runtimemodel.Message{
		Query:         "\u5220\u9664 report.pdf",
		ModelName:     "deepseek-chat",
		ModelProvider: &provider,
	}
	event := map[string]interface{}{
		"correlation_id": "corr-1",
		"governance": map[string]interface{}{
			"approval_event": map[string]interface{}{
				"tool_id":    "file.delete",
				"skill_id":   skills.SkillFileManager,
				"effect":     "delete",
				"asset_type": "file",
				"assets": []interface{}{
					map[string]interface{}{
						"id":           "file-1",
						"type":         "file",
						"name":         "report.pdf",
						"workspace_id": "workspace-1",
					},
				},
			},
		},
	}
	invocation := &skills.ToolInvocationResult{
		Trace: skills.SkillTrace{
			Kind:     "tool_call",
			SkillID:  skills.SkillFileManager,
			ToolName: "delete_file",
			Status:   "success",
			Arguments: map[string]interface{}{
				"file_id": "file-1",
			},
		},
		Messages: []tools.ToolInvokeMessage{
			{
				Type: tools.ToolInvokeMessageTypeJSON,
				Data: map[string]interface{}{
					"status":        "completed",
					"deleted_count": 1,
					"reversible":    false,
					"file": map[string]interface{}{
						"id":           "file-1",
						"name":         "report.pdf",
						"workspace_id": "workspace-1",
					},
				},
			},
		},
	}

	req := toolGovernanceExecutionResultLLMRequest(message, event, invocation, nil)
	text := toolGovernanceStreamRequestText(req)
	for _, want := range []string{
		"Answer in the user's language",
		"For successful file actions, mention only the file name and action result",
		`"action":"delete"`,
		`"action_result":"deleted"`,
		`"name":"report.pdf"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("execution summary request missing %q in %q", want, text)
		}
	}
	for _, hidden := range []string{
		`"file_id"`,
		`"deleted_count"`,
		`"workspace_id"`,
		`"correlation_id"`,
		"file-1",
		"workspace-1",
		"corr-1",
	} {
		if strings.Contains(text, hidden) {
			t.Fatalf("execution summary request exposed %q in %q", hidden, text)
		}
	}
}

func TestToolGovernanceExecutionResultLLMRequestTurnsToolFailureIntoRecoverableFeedback(t *testing.T) {
	message := &runtimemodel.Message{
		Query:     "\u5220\u9664 report.pdf",
		ModelName: "deepseek-chat",
	}
	event := map[string]interface{}{
		"governance": map[string]interface{}{
			"approval_event": map[string]interface{}{
				"effect":     "delete",
				"asset_type": "file",
				"assets": []interface{}{
					map[string]interface{}{"id": "file-1", "type": "file", "name": "report.pdf"},
				},
			},
		},
	}
	invocation := &skills.ToolInvocationResult{
		Trace: skills.SkillTrace{
			Kind:     "tool_call",
			SkillID:  skills.SkillFileManager,
			ToolName: "delete_file",
			Status:   "error",
			Error:    "file file-1 not found",
			Arguments: map[string]interface{}{
				"file_id": "file-1",
			},
		},
	}

	req := toolGovernanceExecutionResultLLMRequest(message, event, invocation, errors.New("file file-1 not found"))
	text := toolGovernanceStreamRequestText(req)
	for _, want := range []string{
		"recoverable model feedback",
		`"recoverable_feedback":true`,
		"Runtime failure feedback:\nfile report.pdf not found",
		`"error":"file report.pdf not found"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("failure summary request missing %q in %q", want, text)
		}
	}
	if strings.Contains(text, "file-1") {
		t.Fatalf("failure summary request exposed internal file id in %q", text)
	}
}

func TestToolGovernanceSafeErrorTextScrubsUnmappedInternalTokens(t *testing.T) {
	got := toolGovernanceSafeErrorText(
		nil,
		nil,
		errors.New("asset 2f3f7b2e-99f8-4c2d-8f10-0e0f4d1c8a12 failed in 0123456789abcdef01234567"),
	)
	for _, hidden := range []string{
		"2f3f7b2e-99f8-4c2d-8f10-0e0f4d1c8a12",
		"0123456789abcdef01234567",
	} {
		if strings.Contains(got, hidden) {
			t.Fatalf("toolGovernanceSafeErrorText() exposed %q in %q", hidden, got)
		}
	}
	if !strings.Contains(got, "the asset") {
		t.Fatalf("toolGovernanceSafeErrorText() = %q, want scrubbed placeholder", got)
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
		"first model response after this continuation",
		"The approval is scoped to the governance grant",
		"authoritative asset resolution",
		"do not ask the user to identify the approved assets again",
		"Answer in the user's language",
		"do not mention internal IDs",
		"Approved asset names: smoke.txt type=file",
		"call file-manager/delete_file with file_id equal to the approved file asset id",
		"Approved tool target JSON for tool arguments only",
		`"file_id":"file-1"`,
		"Do not claim that the action succeeded",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("approval continuation prompt missing %q in %q", want, content)
		}
	}
	for _, hidden := range []string{
		"corr-1",
		"workspace_id",
		"approved_grant",
		"frozen_invocation",
	} {
		if strings.Contains(content, hidden) {
			t.Fatalf("approval continuation prompt exposed %q in %q", hidden, content)
		}
	}
}

func pendingToolGovernanceDecisionMetadata(correlationID string) map[string]interface{} {
	return map[string]interface{}{
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":      "tool_governance",
				"skill_id":  "file-manager",
				"tool_name": "delete_file",
				"status":    "needs_approval",
				"governance": map[string]interface{}{
					"status":            "needs_approval",
					"correlation_id":    correlationID,
					"requires_approval": true,
					"approval_event": map[string]interface{}{
						"correlation_id": correlationID,
						"tool_id":        "file.delete",
						"skill_id":       "file-manager",
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
			"skill_id":       "file-manager",
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

func (r toolGovernanceDecisionConversationRepo) UpdateMetadata(_ context.Context, _ uuid.UUID, metadata map[string]interface{}) error {
	if r.conversation != nil {
		r.conversation.Metadata = copyStringAnyMap(metadata)
	}
	return nil
}

type toolGovernanceDecisionMessageRepo struct {
	repository.MessageRepository
	message                       *runtimemodel.Message
	updateMetadataAnyStatusCalled bool
}

func (r *toolGovernanceDecisionMessageRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Message, error) {
	return r.message, nil
}

func (r *toolGovernanceDecisionMessageRepo) UpdateMetadataAnyStatus(_ context.Context, _ uuid.UUID, metadata map[string]interface{}) error {
	r.updateMetadataAnyStatusCalled = true
	if r.message != nil {
		r.message.Metadata = copyStringAnyMap(metadata)
	}
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
