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

func TestDecideRequiresResolutionWhenAssetDoesNotMatchExpectedTarget(t *testing.T) {
	decision := Decide(Request{
		Manifest:       fileManifest(EffectRead, RiskLevelLow),
		PermissionTier: PermissionTierBasic,
		Assets:         []AssetRef{{ID: "file-1", Type: "file", Name: "first.xlsx"}},
		ExpectedAssets: []AssetRef{{ID: "file-2", Type: "file", Name: "second.xlsx", WorkspaceID: "workspace-1"}},
	}, DefaultPolicy())

	if decision.Status != DecisionStatusNeedsResolution {
		t.Fatalf("expected needs_resolution, got %s (%s)", decision.Status, decision.Reason)
	}
	if decision.Reason != "tool arguments do not match resolved target assets" {
		t.Fatalf("reason = %q, want mismatch reason", decision.Reason)
	}
	expected, ok := decision.ModelFeedback["expected_assets"].([]AssetRef)
	if !ok || len(expected) != 1 || expected[0].ID != "file-2" || expected[0].Name != "second.xlsx" {
		t.Fatalf("expected_assets feedback = %#v", decision.ModelFeedback["expected_assets"])
	}
	auditExpected, ok := decision.AssetOperationAudit["expected_assets"].([]AssetRef)
	if !ok || len(auditExpected) != 1 || auditExpected[0].WorkspaceID != "workspace-1" {
		t.Fatalf("audit expected_assets = %#v", decision.AssetOperationAudit["expected_assets"])
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

func TestDecideSessionGrantRequiresApprovalForDifferentAssetWithinSameScopedTool(t *testing.T) {
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

	assertNeedsApproval(t, decision, "different asset")
	if decision.MatchedGrant != nil || decision.ApprovedByCorrelationID != "" {
		t.Fatalf("decision matched mismatched session grant: %#v", decision)
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

func TestDecideSessionGrantRequiresApprovalForAdditionalAssetWithinSameScopedTool(t *testing.T) {
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

	assertNeedsApproval(t, decision, "additional asset")
	if decision.MatchedGrant != nil || decision.ApprovedByCorrelationID != "" {
		t.Fatalf("decision matched partial session grant: %#v", decision)
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

func TestDecideSessionGrantDoesNotCrossIdentityOrRuntimeScope(t *testing.T) {
	baseGrant := SessionGrant{
		ConversationID:        "conversation-1",
		OrganizationID:        "organization-1",
		UserID:                "user-1",
		SkillID:               "file-reader",
		ProviderType:          "builtin",
		ProviderID:            "files",
		ToolID:                "file.delete",
		Effect:                EffectDelete,
		AssetType:             "file",
		Assets:                []AssetRef{{ID: "file-1", Type: "file"}},
		RiskLevel:             RiskLevelHigh,
		ApprovalCorrelationID: "approval-corr-1",
		ExpiresAt:             time.Now().Add(time.Hour),
	}
	baseRequest := Request{
		Manifest:       fileManifest(EffectDelete, RiskLevelHigh),
		PermissionTier: PermissionTierBasic,
		ConversationID: "conversation-1",
		OrganizationID: "organization-1",
		UserID:         "user-1",
		SkillID:        "file-reader",
		ProviderType:   "builtin",
		ProviderID:     "files",
		Assets:         []AssetRef{{ID: "file-1", Type: "file"}},
	}

	allowed := baseRequest
	allowed.SessionGrants = []SessionGrant{baseGrant}
	decision := Decide(allowed, DefaultPolicy())
	if decision.Status != DecisionStatusAllowed {
		t.Fatalf("expected scoped grant to allow, got %s (%s)", decision.Status, decision.Reason)
	}

	tests := []struct {
		name  string
		grant SessionGrant
	}{
		{
			name: "legacy grant missing identity scope",
			grant: func() SessionGrant {
				grant := baseGrant
				grant.OrganizationID = ""
				grant.UserID = ""
				grant.SkillID = ""
				grant.ProviderType = ""
				grant.ProviderID = ""
				return grant
			}(),
		},
		{
			name: "scoped grant without expiry",
			grant: func() SessionGrant {
				grant := baseGrant
				grant.ExpiresAt = time.Time{}
				return grant
			}(),
		},
		{
			name: "different organization",
			grant: func() SessionGrant {
				grant := baseGrant
				grant.OrganizationID = "organization-2"
				return grant
			}(),
		},
		{
			name: "different user",
			grant: func() SessionGrant {
				grant := baseGrant
				grant.UserID = "user-2"
				return grant
			}(),
		},
		{
			name: "different skill",
			grant: func() SessionGrant {
				grant := baseGrant
				grant.SkillID = "other-skill"
				return grant
			}(),
		},
		{
			name: "different provider",
			grant: func() SessionGrant {
				grant := baseGrant
				grant.ProviderID = "other-provider"
				return grant
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := baseRequest
			request.SessionGrants = []SessionGrant{tt.grant}
			decision := Decide(request, DefaultPolicy())
			assertNeedsApproval(t, decision, tt.name)
			if decision.MatchedGrant != nil || decision.ApprovedByCorrelationID != "" {
				t.Fatalf("decision matched cross-scope session grant: %#v", decision)
			}
		})
	}
}

func TestDecideSessionGrantRequiresRequestScopeForScopedGrant(t *testing.T) {
	scopedGrant := SessionGrant{
		ConversationID:        "conversation-1",
		OrganizationID:        "organization-1",
		UserID:                "user-1",
		SkillID:               "file-reader",
		ProviderType:          "builtin",
		ProviderID:            "files",
		ToolID:                "file.delete",
		Effect:                EffectDelete,
		AssetType:             "file",
		Assets:                []AssetRef{{ID: "file-1", Type: "file"}},
		RiskLevel:             RiskLevelHigh,
		ApprovalCorrelationID: "approval-corr-1",
		ExpiresAt:             time.Now().Add(time.Hour),
	}

	decision := Decide(Request{
		Manifest:       fileManifest(EffectDelete, RiskLevelHigh),
		PermissionTier: PermissionTierBasic,
		ConversationID: "conversation-1",
		Assets:         []AssetRef{{ID: "file-1", Type: "file"}},
		SessionGrants:  []SessionGrant{scopedGrant},
	}, DefaultPolicy())

	assertNeedsApproval(t, decision, "unscoped request should not consume scoped grant")
	if decision.MatchedGrant != nil || decision.ApprovedByCorrelationID != "" {
		t.Fatalf("decision matched scoped session grant without request scope: %#v", decision)
	}
}

func TestDecideSessionGrantRequiresExpiryWhenGrantHasScope(t *testing.T) {
	scopedGrant := SessionGrant{
		ConversationID:        "conversation-1",
		SkillID:               "file-reader",
		ToolID:                "file.delete",
		Effect:                EffectDelete,
		AssetType:             "file",
		Assets:                []AssetRef{{ID: "file-1", Type: "file"}},
		RiskLevel:             RiskLevelHigh,
		ApprovalCorrelationID: "approval-corr-1",
	}

	decision := Decide(Request{
		Manifest:       fileManifest(EffectDelete, RiskLevelHigh),
		PermissionTier: PermissionTierBasic,
		ConversationID: "conversation-1",
		SkillID:        "file-reader",
		Assets:         []AssetRef{{ID: "file-1", Type: "file"}},
		SessionGrants:  []SessionGrant{scopedGrant},
	}, DefaultPolicy())

	assertNeedsApproval(t, decision, "scoped grant without expiry")
	if decision.MatchedGrant != nil || decision.ApprovedByCorrelationID != "" {
		t.Fatalf("decision matched scoped session grant without expiry: %#v", decision)
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
		OrganizationID: "organization-1",
		UserID:         "user-1",
		SkillID:        "file-reader",
		ProviderType:   "builtin",
		ProviderID:     "files",
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
	if decision.ApprovalEvent.Grant.OrganizationID != "organization-1" || decision.ApprovalEvent.Grant.UserID != "user-1" ||
		decision.ApprovalEvent.Grant.SkillID != "file-reader" || decision.ApprovalEvent.Grant.ProviderType != "builtin" ||
		decision.ApprovalEvent.Grant.ProviderID != "files" {
		t.Fatalf("expected request-scoped grant, got %#v", decision.ApprovalEvent.Grant)
	}
	if decision.ApprovalEvent.Grant.ApprovalCorrelationID != "corr-1" {
		t.Fatalf("expected grant approval correlation, got %#v", decision.ApprovalEvent.Grant)
	}
	if decision.ApprovalEvent.Grant.GrantedAt.IsZero() || decision.ApprovalEvent.Grant.ExpiresAt.IsZero() ||
		decision.ApprovalEvent.Grant.ExpiresAt.Sub(decision.ApprovalEvent.Grant.GrantedAt) != DefaultSessionGrantTTL {
		t.Fatalf("expected grant TTL %s, got %#v", DefaultSessionGrantTTL, decision.ApprovalEvent.Grant)
	}
	if len(decision.ApprovalEvent.Grant.Assets) != 1 || decision.ApprovalEvent.Grant.Assets[0].ID != "file-1" {
		t.Fatalf("expected grant assets, got %#v", decision.ApprovalEvent.Grant.Assets)
	}
}

func TestDecidePersistentPreauthorizationOverridesApprovalPolicies(t *testing.T) {
	manifest := fileManifest(EffectDelete, RiskLevelHigh)
	manifest.DefaultApprovalPolicy = ApprovalPolicyAlwaysAsk
	authorizedAt := time.Unix(1_720_000_000, 0).UTC()

	decision := Decide(Request{
		Manifest:       manifest,
		PermissionTier: PermissionTierBasic,
		Assets:         []AssetRef{{ID: "table-1", Type: "file"}},
		Preauthorization: &Preauthorization{
			Required:     true,
			Matched:      true,
			Source:       "agent_binding",
			BindingType:  "database",
			AuthorizedBy: "account-1",
			AuthorizedAt: &authorizedAt,
			Resources:    []AssetRef{{ID: "table-1", Type: "database_table"}},
			Reason:       "allowed by the current Agent database binding",
		},
	}, DefaultPolicy())

	if decision.Status != DecisionStatusAllowed || decision.RequiresApproval {
		t.Fatalf("decision = %#v, want persistent authorization to allow", decision)
	}
	if decision.ApprovalEvent != nil || decision.MatchedGrant != nil {
		t.Fatalf("decision = %#v, persistent authorization must not create a session grant", decision)
	}
	if decision.ModelFeedback["authorization_source"] != "agent_binding" {
		t.Fatalf("model feedback = %#v, want agent_binding source", decision.ModelFeedback)
	}
	if decision.AssetOperationAudit["authorization_actor_id"] != "account-1" ||
		decision.AssetOperationAudit["authorization_granted_at"] != authorizedAt {
		t.Fatalf("audit = %#v, want persistent authorization evidence", decision.AssetOperationAudit)
	}
}

func TestDecideMissingPersistentPreauthorizationDeniesBeforeSessionGrant(t *testing.T) {
	decision := Decide(Request{
		Manifest:       fileManifest(EffectDelete, RiskLevelHigh),
		PermissionTier: PermissionTierBasic,
		ConversationID: "conversation-1",
		Assets:         []AssetRef{{ID: "file-1", Type: "file"}},
		Preauthorization: &Preauthorization{
			Required: true,
			Source:   "agent_binding",
			Code:     "agent_resource_not_bound",
			Reason:   "the requested resource is not bound",
		},
		SessionGrants: []SessionGrant{{
			ConversationID: "conversation-1",
			ToolID:         "file.delete",
			Effect:         EffectDelete,
			AssetType:      "file",
			Assets:         []AssetRef{{ID: "file-1", Type: "file"}},
			RiskLevel:      RiskLevelHigh,
			ExpiresAt:      time.Now().Add(time.Hour),
		}},
	}, DefaultPolicy())

	if decision.Status != DecisionStatusDenied || decision.RequiresApproval {
		t.Fatalf("decision = %#v, want denied without approval", decision)
	}
	if decision.ApprovalEvent != nil || decision.MatchedGrant != nil {
		t.Fatalf("decision = %#v, invalid persistent authorization must not use session approval", decision)
	}
	if decision.ModelFeedback["authorization_code"] != "agent_resource_not_bound" {
		t.Fatalf("model feedback = %#v, want stable authorization code", decision.ModelFeedback)
	}
}

func TestDecideNonInteractiveModeConvertsApprovalToDenial(t *testing.T) {
	decision := Decide(Request{
		Manifest:       fileManifest(EffectDelete, RiskLevelHigh),
		PermissionTier: PermissionTierBasic,
		ApprovalMode:   ApprovalModeNonInteractive,
		Assets:         []AssetRef{{ID: "file-1", Type: "file"}},
		Preauthorization: &Preauthorization{
			Source: "agent_runtime",
			Code:   "agent_tool_not_preauthorized",
			Reason: "the current Agent does not have persistent authorization for this tool action",
		},
	}, DefaultPolicy())

	if decision.Status != DecisionStatusDenied || decision.RequiresApproval || decision.ApprovalEvent != nil {
		t.Fatalf("decision = %#v, want non-interactive denial without approval event", decision)
	}
	if decision.ModelFeedback["authorization_code"] != "agent_tool_not_preauthorized" {
		t.Fatalf("model feedback = %#v, want agent_tool_not_preauthorized", decision.ModelFeedback)
	}
}

func TestDecideCriticalBlockPrecedesPersistentPreauthorization(t *testing.T) {
	policy := DefaultPolicy()
	policy.CriticalRiskBlocked = true
	decision := Decide(Request{
		Manifest:       fileManifest(EffectDelete, RiskLevelCritical),
		PermissionTier: PermissionTierBasic,
		Assets:         []AssetRef{{ID: "file-1", Type: "file"}},
		Preauthorization: &Preauthorization{
			Required: true,
			Matched:  true,
			Source:   "agent_binding",
		},
	}, policy)

	if decision.Status != DecisionStatusBlocked {
		t.Fatalf("decision = %#v, want critical policy block", decision)
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
