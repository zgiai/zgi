package service

import (
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func TestAssetOperationAuditRecordsFromMessageUsesCanonicalDecisionAndInvocationFallback(t *testing.T) {
	conversationID := uuid.New()
	messageID := uuid.New()
	createdAt := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	message := &runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		CreatedAt:      createdAt,
		Metadata: map[string]interface{}{
			"tool_governance_decisions": []interface{}{
				map[string]interface{}{
					"conversation_id":   conversationID.String(),
					"message_id":        messageID.String(),
					"runtime_id":        "tool_governance:corr-canonical",
					"correlation_id":    "corr-canonical",
					"skill_id":          "file-reader",
					"tool_name":         "delete_file",
					"approval_status":   "approved",
					"requires_approval": false,
					"governance": map[string]interface{}{
						"status":            "needs_approval",
						"correlation_id":    "corr-canonical",
						"approval_status":   "approved",
						"requires_approval": false,
						"approval_event": map[string]interface{}{
							"correlation_id": "corr-canonical",
							"tool_id":        "file.delete",
							"effect":         "delete",
							"asset_type":     "file",
							"risk_level":     "high",
						},
					},
					"asset_operation_audit": map[string]interface{}{
						"schema_version":             "tool_governance.asset_operation.v1",
						"created_at":                 createdAt.Unix() + 10,
						"correlation_id":             "corr-canonical",
						"tool_id":                    "file.delete",
						"effect":                     "delete",
						"asset_type":                 "file",
						"risk_level":                 "high",
						"approval_status":            "approved",
						"action":                     "approve",
						"resolved_at":                "2026-06-15T12:00:10Z",
						"resolved_by":                "account-1",
						"remember_for_session":       true,
						"approved_by_correlation_id": "corr-canonical",
						"approved_grant": map[string]interface{}{
							"conversation_id":         conversationID.String(),
							"tool_id":                 "file.delete",
							"effect":                  "delete",
							"asset_type":              "file",
							"risk_level":              "high",
							"approval_correlation_id": "corr-canonical",
						},
						"session_grant": map[string]interface{}{
							"conversation_id":         conversationID.String(),
							"tool_id":                 "file.delete",
							"effect":                  "delete",
							"asset_type":              "file",
							"risk_level":              "high",
							"approval_correlation_id": "corr-canonical",
						},
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
			},
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":            "tool_governance",
					"runtime_id":      "tool_governance:corr-canonical",
					"skill_id":        "file-reader",
					"tool_name":       "delete_file",
					"status":          "needs_approval",
					"approval_status": "pending",
					"governance": map[string]interface{}{
						"status":            "needs_approval",
						"correlation_id":    "corr-canonical",
						"requires_approval": true,
					},
				},
				map[string]interface{}{
					"kind":            "tool_governance",
					"runtime_id":      "tool_governance:corr-fallback",
					"skill_id":        "file-reader",
					"tool_name":       "read_file",
					"status":          "allowed",
					"approval_status": "allowed",
					"governance": map[string]interface{}{
						"status":         "allowed",
						"correlation_id": "corr-fallback",
						"manifest": map[string]interface{}{
							"effect":     "read",
							"asset_type": "file",
							"risk_level": "low",
						},
					},
					"asset_operation_audit": map[string]interface{}{
						"schema_version":    "tool_governance.asset_operation.v1",
						"correlation_id":    "corr-fallback",
						"tool_id":           "file.read",
						"effect":            "read",
						"asset_type":        "file",
						"risk_level":        "low",
						"approval_status":   "allowed",
						"governance_status": "allowed",
						"assets": []interface{}{
							map[string]interface{}{
								"id":           "file-2",
								"type":         "file",
								"name":         "notes.xlsx",
								"workspace_id": "workspace-1",
							},
						},
					},
				},
				map[string]interface{}{
					"kind":       "tool_governance",
					"runtime_id": "tool_governance:missing-correlation",
				},
			},
		},
	}

	records := assetOperationAuditRecordsFromMessage(message)
	if len(records) != 2 {
		t.Fatalf("records len = %d, want 2: %#v", len(records), records)
	}
	canonical := records[0]
	if canonical.Source != "tool_governance_decision" || canonical.CorrelationID != "corr-canonical" {
		t.Fatalf("canonical record identity = source %q correlation %q, want canonical decision corr-canonical", canonical.Source, canonical.CorrelationID)
	}
	if canonical.ApprovalStatus != "approved" || canonical.Action != "approve" || canonical.RequiresApproval {
		t.Fatalf("canonical approval = status %q action %q requires %v, want approved approve false", canonical.ApprovalStatus, canonical.Action, canonical.RequiresApproval)
	}
	if canonical.ResolvedAt != "2026-06-15T12:00:10Z" || canonical.ResolvedBy != "account-1" || !canonical.RememberForSession {
		t.Fatalf("canonical resolution = resolved_at %q resolved_by %q remember %v, want resolved audit fields", canonical.ResolvedAt, canonical.ResolvedBy, canonical.RememberForSession)
	}
	if canonical.ApprovedByCorrelationID != "corr-canonical" {
		t.Fatalf("canonical approved_by_correlation_id = %q, want corr-canonical", canonical.ApprovedByCorrelationID)
	}
	if canonical.ApprovedGrant["tool_id"] != "file.delete" || canonical.SessionGrant["approval_correlation_id"] != "corr-canonical" {
		t.Fatalf("canonical grants = approved %#v session %#v, want approved/session replay grants", canonical.ApprovedGrant, canonical.SessionGrant)
	}
	if canonical.ToolID != "file.delete" || canonical.Effect != "delete" || canonical.RiskLevel != "high" {
		t.Fatalf("canonical tool fields = %#v, want file.delete delete high", canonical)
	}
	if canonical.AssetCount != 1 || canonical.WorkspaceID != "workspace-1" || canonical.Assets[0]["name"] != "report.pdf" {
		t.Fatalf("canonical assets = count %d workspace %q assets %#v, want report.pdf in workspace-1", canonical.AssetCount, canonical.WorkspaceID, canonical.Assets)
	}
	if canonical.CreatedAt != createdAt.Unix()+10 || canonical.MessageCreatedAt != createdAt.Unix() {
		t.Fatalf("canonical timestamps = created %d message %d, want audit created_at and message created_at", canonical.CreatedAt, canonical.MessageCreatedAt)
	}

	fallback := records[1]
	if fallback.Source != "skill_invocation" || fallback.CorrelationID != "corr-fallback" {
		t.Fatalf("fallback record identity = source %q correlation %q, want skill invocation corr-fallback", fallback.Source, fallback.CorrelationID)
	}
	if fallback.ToolID != "file.read" || fallback.Effect != "read" || fallback.ApprovalStatus != "allowed" {
		t.Fatalf("fallback fields = tool %q effect %q approval %q, want file.read read allowed", fallback.ToolID, fallback.Effect, fallback.ApprovalStatus)
	}
	if fallback.AssetCount != 1 || fallback.Assets[0]["id"] != "file-2" {
		t.Fatalf("fallback assets = count %d assets %#v, want file-2", fallback.AssetCount, fallback.Assets)
	}
}
