package toolgovernance

import (
	"testing"
	"time"
)

func TestDecideBasicLowRiskFileReadAllowed(t *testing.T) {
	decision := Decide(Request{
		Manifest: fileManifest(EffectRead, RiskLevelLow),
		Assets:   []AssetRef{{ID: "file-1", Type: "file", Name: "report.pdf"}},
	}, DefaultPolicy())

	if decision.Status != DecisionStatusAllowed {
		t.Fatalf("expected allowed, got %s (%s)", decision.Status, decision.Reason)
	}
	if decision.RequiresApproval {
		t.Fatal("read should not require approval")
	}
	if decision.CorrelationID == "" {
		t.Fatal("expected correlation id")
	}
}

func TestDecideRequiresResolutionWhenAssetMissing(t *testing.T) {
	decision := Decide(Request{
		Manifest: fileManifest(EffectRead, RiskLevelLow),
	}, DefaultPolicy())

	if decision.Status != DecisionStatusNeedsResolution {
		t.Fatalf("expected needs_resolution, got %s", decision.Status)
	}
	if decision.ModelFeedback["status"] != string(DecisionStatusNeedsResolution) {
		t.Fatalf("expected model feedback status, got %#v", decision.ModelFeedback)
	}
}

func TestDecideBasicCreateNeedsApproval(t *testing.T) {
	decision := Decide(Request{
		Manifest: fileManifest(EffectCreate, RiskLevelLow),
		Assets:   []AssetRef{{ID: "file-1", Type: "file"}},
	}, DefaultPolicy())

	assertNeedsApproval(t, decision, "create")
}

func TestDecideAdvancedUpdateAllowed(t *testing.T) {
	decision := Decide(Request{
		Manifest:       fileManifest(EffectUpdate, RiskLevelMedium),
		PermissionTier: PermissionTierAdvanced,
		Assets:         []AssetRef{{ID: "file-1", Type: "file"}},
	}, DefaultPolicy())

	if decision.Status != DecisionStatusAllowed {
		t.Fatalf("expected allowed, got %s (%s)", decision.Status, decision.Reason)
	}
}

func TestDecideAdvancedDeleteNeedsApproval(t *testing.T) {
	decision := Decide(Request{
		Manifest:       fileManifest(EffectDelete, RiskLevelMedium),
		PermissionTier: PermissionTierAdvanced,
		Assets:         []AssetRef{{ID: "file-1", Type: "file", Name: "report.pdf"}},
	}, DefaultPolicy())

	assertNeedsApproval(t, decision, "delete")
	if decision.ApprovalEvent.Assets[0].Name != "report.pdf" {
		t.Fatalf("approval event should include assets, got %#v", decision.ApprovalEvent.Assets)
	}
}

func TestDecideBuildsAssetOperationAuditPayload(t *testing.T) {
	decision := Decide(Request{
		Manifest:       fileManifest(EffectDelete, RiskLevelHigh),
		PermissionTier: PermissionTierBasic,
		ConversationID: "conversation-1",
		CorrelationID:  "corr-1",
		Assets: []AssetRef{{
			ID:          "file-1",
			Type:        "file",
			Name:        "report.pdf",
			WorkspaceID: "workspace-1",
			Source:      "console.files",
		}},
	}, DefaultPolicy())

	audit := decision.AssetOperationAudit
	if audit["schema_version"] != "tool_governance.asset_operation.v1" {
		t.Fatalf("audit = %#v, want schema version", audit)
	}
	for key, want := range map[string]interface{}{
		"event_type":        "asset_operation",
		"correlation_id":    "corr-1",
		"conversation_id":   "conversation-1",
		"governance_status": string(DecisionStatusNeedsApproval),
		"approval_status":   "pending",
		"tool_id":           "file.delete",
		"skill_id":          "internal-files",
		"domain":            "files",
		"effect":            "delete",
		"asset_type":        "file",
		"asset_count":       1,
		"risk_level":        "high",
		"permission_tier":   "basic",
		"audit_required":    true,
	} {
		if audit[key] != want {
			t.Fatalf("audit[%s] = %#v, want %#v in %#v", key, audit[key], want, audit)
		}
	}
	assets, ok := audit["assets"].([]AssetRef)
	if !ok || len(assets) != 1 || assets[0].ID != "file-1" || assets[0].WorkspaceID != "workspace-1" {
		t.Fatalf("audit assets = %#v, want resolved file asset", audit["assets"])
	}
	feedbackAudit, ok := decision.ModelFeedback["asset_operation_audit"].(map[string]interface{})
	if !ok || feedbackAudit["correlation_id"] != "corr-1" {
		t.Fatalf("model feedback audit = %#v, want audit payload", decision.ModelFeedback["asset_operation_audit"])
	}
}

func TestDecideFullDeleteStillNeedsApprovalByDefault(t *testing.T) {
	decision := Decide(Request{
		Manifest:       fileManifest(EffectDelete, RiskLevelLow),
		PermissionTier: PermissionTierFull,
		Assets:         []AssetRef{{ID: "file-1", Type: "file"}},
	}, DefaultPolicy())

	assertNeedsApproval(t, decision, "delete")
}

func TestDecideSessionGrantAllowsMatchingToolEffectAssetAndRisk(t *testing.T) {
	decision := Decide(Request{
		Manifest:       fileManifest(EffectDelete, RiskLevelHigh),
		PermissionTier: PermissionTierBasic,
		ConversationID: "conversation-1",
		Assets:         []AssetRef{{ID: "file-1", Type: "file"}},
		SessionGrants: []SessionGrant{{
			ConversationID:        "conversation-1",
			ToolID:                "file.delete",
			Effect:                EffectDelete,
			AssetType:             "file",
			Assets:                []AssetRef{{ID: "file-1", Type: "file", Name: "report.pdf"}},
			RiskLevel:             RiskLevelHigh,
			ApprovalCorrelationID: "approval-corr-1",
			ExpiresAt:             time.Now().Add(time.Hour),
		}},
	}, DefaultPolicy())

	if decision.Status != DecisionStatusAllowed {
		t.Fatalf("expected session grant to allow, got %s (%s)", decision.Status, decision.Reason)
	}
	if decision.ApprovedByCorrelationID != "approval-corr-1" {
		t.Fatalf("approved_by_correlation_id = %q, want approval-corr-1", decision.ApprovedByCorrelationID)
	}
	if decision.MatchedGrant == nil || decision.MatchedGrant.ApprovalCorrelationID != "approval-corr-1" {
		t.Fatalf("matched grant = %#v, want approval correlation", decision.MatchedGrant)
	}
	if decision.ModelFeedback["approved_by_correlation_id"] != "approval-corr-1" {
		t.Fatalf("model feedback = %#v, want approval correlation", decision.ModelFeedback)
	}
	if matchedAssets, ok := decision.ModelFeedback["matched_assets"].([]AssetRef); !ok || len(matchedAssets) != 1 || matchedAssets[0].ID != "file-1" {
		t.Fatalf("model feedback matched_assets = %#v, want approved file", decision.ModelFeedback["matched_assets"])
	}
}

func TestDecideSessionGrantAllowsDifferentAssetWithinSameScopedTool(t *testing.T) {
	decision := Decide(Request{
		Manifest:       fileManifest(EffectDelete, RiskLevelHigh),
		PermissionTier: PermissionTierBasic,
		ConversationID: "conversation-1",
		Assets:         []AssetRef{{ID: "file-2", Type: "file", Name: "other.pdf"}},
		SessionGrants: []SessionGrant{{
			ConversationID:        "conversation-1",
			ToolID:                "file.delete",
			Effect:                EffectDelete,
			AssetType:             "file",
			Assets:                []AssetRef{{ID: "file-1", Type: "file", Name: "report.pdf"}},
			RiskLevel:             RiskLevelHigh,
			ApprovalCorrelationID: "approval-corr-1",
			ExpiresAt:             time.Now().Add(time.Hour),
		}},
	}, DefaultPolicy())

	if decision.Status != DecisionStatusAllowed {
		t.Fatalf("expected session grant to allow same scoped tool, got %s (%s)", decision.Status, decision.Reason)
	}
	if decision.ApprovedByCorrelationID != "approval-corr-1" {
		t.Fatalf("approved_by_correlation_id = %q, want approval-corr-1", decision.ApprovedByCorrelationID)
	}
	matchedAssets, ok := decision.ModelFeedback["matched_assets"].([]AssetRef)
	if !ok || len(matchedAssets) != 1 || matchedAssets[0].ID != "file-2" {
		t.Fatalf("matched_assets = %#v, want current requested asset", decision.ModelFeedback["matched_assets"])
	}
}

func TestDecideSessionGrantAllowsAssetlessScopedGrant(t *testing.T) {
	decision := Decide(Request{
		Manifest:       fileManifest(EffectDelete, RiskLevelHigh),
		PermissionTier: PermissionTierBasic,
		ConversationID: "conversation-1",
		Assets:         []AssetRef{{ID: "file-1", Type: "file"}},
		SessionGrants: []SessionGrant{{
			ConversationID:        "conversation-1",
			ToolID:                "file.delete",
			Effect:                EffectDelete,
			AssetType:             "file",
			RiskLevel:             RiskLevelHigh,
			ApprovalCorrelationID: "legacy-corr",
			ExpiresAt:             time.Now().Add(time.Hour),
		}},
	}, DefaultPolicy())

	if decision.Status != DecisionStatusAllowed {
		t.Fatalf("expected assetless scoped grant to allow, got %s (%s)", decision.Status, decision.Reason)
	}
	if decision.ApprovedByCorrelationID != "legacy-corr" {
		t.Fatalf("approved_by_correlation_id = %q, want legacy-corr", decision.ApprovedByCorrelationID)
	}
}

func TestDecideSessionGrantAllowsAdditionalAssetWithinSameScopedTool(t *testing.T) {
	decision := Decide(Request{
		Manifest:       fileManifest(EffectDelete, RiskLevelHigh),
		PermissionTier: PermissionTierBasic,
		ConversationID: "conversation-1",
		Assets: []AssetRef{
			{ID: "file-1", Type: "file"},
			{ID: "file-2", Type: "file"},
		},
		SessionGrants: []SessionGrant{{
			ConversationID:        "conversation-1",
			ToolID:                "file.delete",
			Effect:                EffectDelete,
			AssetType:             "file",
			Assets:                []AssetRef{{ID: "file-1", Type: "file"}},
			RiskLevel:             RiskLevelHigh,
			ApprovalCorrelationID: "approval-corr-1",
			ExpiresAt:             time.Now().Add(time.Hour),
		}},
	}, DefaultPolicy())

	if decision.Status != DecisionStatusAllowed {
		t.Fatalf("expected same scoped grant to allow additional requested assets, got %s (%s)", decision.Status, decision.Reason)
	}
	matchedAssets, ok := decision.ModelFeedback["matched_assets"].([]AssetRef)
	if !ok || len(matchedAssets) != 2 {
		t.Fatalf("matched_assets = %#v, want current requested asset set", decision.ModelFeedback["matched_assets"])
	}
}

func TestDecideSessionGrantMatchesNameOnlyAssetWhenScoped(t *testing.T) {
	decision := Decide(Request{
		Manifest:       fileManifest(EffectDelete, RiskLevelHigh),
		PermissionTier: PermissionTierBasic,
		ConversationID: "conversation-1",
		Assets:         []AssetRef{{Type: "file", Name: "report.pdf", WorkspaceID: "workspace-1"}},
		SessionGrants: []SessionGrant{{
			ConversationID:        "conversation-1",
			ToolID:                "file.delete",
			Effect:                EffectDelete,
			AssetType:             "file",
			Assets:                []AssetRef{{Type: "file", Name: "Report.pdf", WorkspaceID: "workspace-1"}},
			RiskLevel:             RiskLevelHigh,
			ApprovalCorrelationID: "approval-corr-1",
			ExpiresAt:             time.Now().Add(time.Hour),
		}},
	}, DefaultPolicy())

	if decision.Status != DecisionStatusAllowed {
		t.Fatalf("expected name-only grant to allow, got %s (%s)", decision.Status, decision.Reason)
	}
}

func TestDecideSessionGrantDoesNotCrossConversationOrEffect(t *testing.T) {
	request := Request{
		Manifest:       fileManifest(EffectDelete, RiskLevelHigh),
		PermissionTier: PermissionTierFull,
		ConversationID: "conversation-1",
		Assets:         []AssetRef{{ID: "file-1", Type: "file"}},
		SessionGrants: []SessionGrant{{
			ConversationID: "conversation-2",
			ToolID:         "file.delete",
			Effect:         EffectUpdate,
			AssetType:      "file",
			RiskLevel:      RiskLevelHigh,
			ExpiresAt:      time.Now().Add(time.Hour),
		}},
	}

	decision := Decide(request, DefaultPolicy())
	assertNeedsApproval(t, decision, "mismatch")
}

func TestDecideSessionGrantDoesNotMatchDifferentToolAssetTypeOrLowerRisk(t *testing.T) {
	baseGrant := SessionGrant{
		ConversationID:        "conversation-1",
		ToolID:                "file.delete",
		Effect:                EffectDelete,
		AssetType:             "file",
		Assets:                []AssetRef{{ID: "file-1", Type: "file"}},
		RiskLevel:             RiskLevelHigh,
		ApprovalCorrelationID: "approval-corr-1",
		ExpiresAt:             time.Now().Add(time.Hour),
	}

	tests := []struct {
		name  string
		grant SessionGrant
	}{
		{
			name: "different tool",
			grant: func() SessionGrant {
				grant := baseGrant
				grant.ToolID = "file.update"
				return grant
			}(),
		},
		{
			name: "different asset type",
			grant: func() SessionGrant {
				grant := baseGrant
				grant.AssetType = "database"
				return grant
			}(),
		},
		{
			name: "lower risk",
			grant: func() SessionGrant {
				grant := baseGrant
				grant.RiskLevel = RiskLevelMedium
				return grant
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := Decide(Request{
				Manifest:       fileManifest(EffectDelete, RiskLevelHigh),
				PermissionTier: PermissionTierFull,
				ConversationID: "conversation-1",
				Assets:         []AssetRef{{ID: "file-1", Type: "file"}},
				SessionGrants:  []SessionGrant{tt.grant},
			}, DefaultPolicy())

			assertNeedsApproval(t, decision, tt.name)
			if decision.ApprovedByCorrelationID != "" || decision.MatchedGrant != nil {
				t.Fatalf("decision matched mismatched session grant: %#v", decision)
			}
		})
	}
}

func TestDecideDeniedWhenTierIsNotAllowed(t *testing.T) {
	manifest := fileManifest(EffectRead, RiskLevelLow)
	manifest.AllowedPermissionTiers = []PermissionTier{PermissionTierAdvanced}

	decision := Decide(Request{
		Manifest:       manifest,
		PermissionTier: PermissionTierBasic,
		Assets:         []AssetRef{{ID: "file-1", Type: "file"}},
	}, DefaultPolicy())

	if decision.Status != DecisionStatusDenied {
		t.Fatalf("expected denied, got %s", decision.Status)
	}
}

func TestDecideAlwaysAskBuildsApprovalEventGrant(t *testing.T) {
	manifest := fileManifest(EffectRead, RiskLevelLow)
	manifest.DefaultApprovalPolicy = ApprovalPolicyAlwaysAsk

	decision := Decide(Request{
		Manifest:       manifest,
		PermissionTier: PermissionTierAdvanced,
		ConversationID: "conversation-1",
		CorrelationID:  "corr-1",
		Assets:         []AssetRef{{ID: "file-1", Type: "file", Name: "report.pdf"}},
	}, DefaultPolicy())

	assertNeedsApproval(t, decision, "always ask")
	if decision.ApprovalEvent.CorrelationID != "corr-1" {
		t.Fatalf("expected correlation id in approval event, got %s", decision.ApprovalEvent.CorrelationID)
	}
	if decision.ApprovalEvent.Grant.ToolID != "file.read" || decision.ApprovalEvent.Grant.Effect != EffectRead {
		t.Fatalf("unexpected grant scope: %#v", decision.ApprovalEvent.Grant)
	}
	if decision.ApprovalEvent.Grant.ConversationID != "conversation-1" {
		t.Fatalf("expected conversation-bound grant, got %#v", decision.ApprovalEvent.Grant)
	}
	if decision.ApprovalEvent.Grant.ApprovalCorrelationID != "corr-1" {
		t.Fatalf("expected grant approval correlation, got %#v", decision.ApprovalEvent.Grant)
	}
	if len(decision.ApprovalEvent.Grant.Assets) != 1 || decision.ApprovalEvent.Grant.Assets[0].ID != "file-1" {
		t.Fatalf("expected grant assets, got %#v", decision.ApprovalEvent.Grant.Assets)
	}
}

func fileManifest(effect Effect, risk RiskLevel) Manifest {
	return Manifest{
		ToolID:                  "file." + string(effect),
		SkillID:                 "internal-files",
		Domain:                  "files",
		Effect:                  effect,
		AssetType:               "file",
		RiskLevel:               risk,
		RequiresAssetResolution: true,
		AuditRequired:           true,
	}
}

func assertNeedsApproval(t *testing.T, decision Decision, label string) {
	t.Helper()
	if decision.Status != DecisionStatusNeedsApproval {
		t.Fatalf("%s: expected needs_approval, got %s (%s)", label, decision.Status, decision.Reason)
	}
	if !decision.RequiresApproval {
		t.Fatalf("%s: expected requires approval", label)
	}
	if decision.ApprovalEvent == nil {
		t.Fatalf("%s: expected approval event", label)
	}
	if decision.ApprovalEvent.Type != ApprovalEventTypeAssetToolApproval {
		t.Fatalf("%s: unexpected event type %s", label, decision.ApprovalEvent.Type)
	}
	if decision.ModelFeedback["status"] != string(DecisionStatusNeedsApproval) {
		t.Fatalf("%s: expected model feedback, got %#v", label, decision.ModelFeedback)
	}
}
