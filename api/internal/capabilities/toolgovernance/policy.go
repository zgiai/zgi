package toolgovernance

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

func DefaultPolicy() Policy {
	return Policy{
		DefaultPermissionTier:              PermissionTierBasic,
		HighRiskRequiresApproval:           true,
		CriticalRiskRequiresApproval:       true,
		DeleteRequiresApproval:             true,
		BulkSensitiveRequiresApproval:      true,
		ExternalSideEffectRequiresApproval: true,
	}
}

func Decide(req Request, policy Policy) Decision {
	policy = policy.withDefaults()
	manifest := NormalizeManifest(req.Manifest)
	assets := normalizeAssets(req.Assets)
	expectedAssets := normalizeAssets(req.ExpectedAssets)
	tier := NormalizePermissionTier(req.PermissionTier)
	if tier == "" {
		tier = policy.DefaultPermissionTier
	}
	if tier == "" {
		tier = PermissionTierBasic
	}
	correlationID := strings.TrimSpace(req.CorrelationID)
	if correlationID == "" {
		correlationID = uuid.NewString()
	}

	preauthorization := normalizePreauthorization(req.Preauthorization)
	base := Decision{
		Status:         DecisionStatusAllowed,
		CorrelationID:  correlationID,
		Manifest:       manifest,
		Assets:         assets,
		ExpectedAssets: expectedAssets,
	}
	if manifest.ToolID == "" {
		return finalizeDecision(base.withStatus(DecisionStatusDenied, "tool_id is required", false), tier, req)
	}
	if !tierAllowed(manifest, tier) {
		return finalizeDecision(base.withStatus(DecisionStatusDenied, "permission tier is not allowed for this tool", false), tier, req)
	}
	if isAssetOperation(manifest) && manifest.RequiresAssetResolution && len(assets) == 0 {
		decision := base.withStatus(DecisionStatusNeedsResolution, "asset resolution is required before this tool can run", false)
		return finalizeDecision(decision, tier, req)
	}
	if isAssetOperation(manifest) && manifest.RequiresAssetResolution && len(expectedAssets) > 0 && !assetsMatchExpectedAssets(assets, expectedAssets) {
		decision := base.withStatus(DecisionStatusNeedsResolution, "tool arguments do not match resolved target assets", false)
		return finalizeDecision(decision, tier, req)
	}
	if policy.CriticalRiskBlocked && manifest.RiskLevel == RiskLevelCritical {
		decision := base.withStatus(DecisionStatusBlocked, "critical risk tools are blocked by policy", false)
		return finalizeDecision(decision, tier, req)
	}
	if preauthorization != nil && preauthorization.Required {
		base.Preauthorization = preauthorization
		if base.Preauthorization.Matched {
			reason := firstNonEmptyString(base.Preauthorization.Reason, "allowed by persistent preauthorization")
			decision := base.withStatus(DecisionStatusAllowed, reason, false)
			return finalizeDecision(decision, tier, req)
		}
		reason := firstNonEmptyString(base.Preauthorization.Reason, "persistent preauthorization is required")
		decision := base.withStatus(DecisionStatusDenied, reason, false)
		return finalizeDecision(decision, tier, req)
	}
	if grant, ok := matchingSessionGrant(req, manifest, assets); ok {
		decision := base.withStatus(DecisionStatusAllowed, "allowed by matching session grant", false)
		decision = decision.withMatchedGrant(grant)
		return finalizeDecision(decision, tier, req)
	}
	if manifest.DefaultApprovalPolicy == ApprovalPolicyAlwaysAsk {
		return finalizeDecision(base.needsApproval(tier, req, "tool manifest requires approval"), tier, req)
	}
	if manifest.DefaultApprovalPolicy == ApprovalPolicyNeverAsk && !policy.hardRequiresApproval(manifest) {
		decision := base.withStatus(DecisionStatusAllowed, "allowed by manifest approval policy", false)
		return finalizeDecision(decision, tier, req)
	}
	if policy.hardRequiresApproval(manifest) {
		return finalizeDecision(base.needsApproval(tier, req, hardApprovalReason(policy, manifest)), tier, req)
	}

	switch tier {
	case PermissionTierFull:
		decision := base.withStatus(DecisionStatusAllowed, "allowed by full permission tier", false)
		return finalizeDecision(decision, tier, req)
	case PermissionTierAdvanced:
		if advancedTierAllows(manifest) {
			decision := base.withStatus(DecisionStatusAllowed, "allowed by advanced permission tier", false)
			return finalizeDecision(decision, tier, req)
		}
	case PermissionTierBasic:
		if basicTierAllows(manifest) {
			decision := base.withStatus(DecisionStatusAllowed, "allowed by basic permission tier", false)
			return finalizeDecision(decision, tier, req)
		}
	}
	return finalizeDecision(base.needsApproval(tier, req, "permission tier requires user approval for this tool"), tier, req)
}

func (d Decision) withStatus(status DecisionStatus, reason string, requiresApproval bool) Decision {
	d.Status = status
	d.Reason = reason
	d.RequiresApproval = requiresApproval
	return d
}

func (d Decision) needsApproval(tier PermissionTier, req Request, reason string) Decision {
	if normalizeApprovalMode(req.ApprovalMode) == ApprovalModeNonInteractive {
		if d.Preauthorization == nil {
			d.Preauthorization = normalizePreauthorization(req.Preauthorization)
		}
		if d.Preauthorization == nil {
			d.Preauthorization = &Preauthorization{
				Source: "non_interactive_runtime",
				Code:   "interactive_approval_unavailable",
				Reason: "interactive approval is unavailable in this runtime",
			}
		}
		denialReason := firstNonEmptyString(d.Preauthorization.Reason, reason)
		return d.withStatus(DecisionStatusDenied, denialReason, false)
	}
	d = d.withStatus(DecisionStatusNeedsApproval, reason, true)
	d.ApprovalEvent = approvalEvent(d, tier, req)
	d.ModelFeedback = modelFeedback(d, tier)
	return d
}

func (d Decision) withMatchedGrant(grant SessionGrant) Decision {
	grant = normalizeSessionGrant(grant)
	d.MatchedGrant = &grant
	d.ApprovedByCorrelationID = strings.TrimSpace(grant.ApprovalCorrelationID)
	return d
}

func finalizeDecision(decision Decision, tier PermissionTier, req Request) Decision {
	decision.AssetOperationAudit = assetOperationAuditPayload(decision, tier, req)
	decision.ModelFeedback = modelFeedback(decision, tier)
	return decision
}

func approvalEvent(decision Decision, tier PermissionTier, req Request) *ApprovalEvent {
	manifest := decision.Manifest
	now := time.Now().UTC()
	return &ApprovalEvent{
		Type:               ApprovalEventTypeAssetToolApproval,
		CorrelationID:      decision.CorrelationID,
		ToolID:             manifest.ToolID,
		SkillID:            manifest.SkillID,
		Domain:             manifest.Domain,
		Effect:             manifest.Effect,
		AssetType:          manifest.AssetType,
		RiskLevel:          manifest.RiskLevel,
		Assets:             decision.Assets,
		Reversible:         manifest.Reversible,
		BulkSensitive:      manifest.BulkSensitive,
		ExternalSideEffect: manifest.ExternalSideEffect,
		PermissionTier:     tier,
		Grant: SessionGrant{
			ConversationID:        strings.TrimSpace(req.ConversationID),
			OrganizationID:        strings.TrimSpace(req.OrganizationID),
			UserID:                strings.TrimSpace(req.UserID),
			SkillID:               firstNonEmptyString(req.SkillID, manifest.SkillID),
			ProviderType:          strings.TrimSpace(req.ProviderType),
			ProviderID:            strings.TrimSpace(req.ProviderID),
			ToolID:                manifest.ToolID,
			Effect:                manifest.Effect,
			AssetType:             manifest.AssetType,
			Assets:                decision.Assets,
			RiskLevel:             manifest.RiskLevel,
			ApprovalCorrelationID: decision.CorrelationID,
			GrantedAt:             now,
			ExpiresAt:             now.Add(DefaultSessionGrantTTL),
		},
	}
}

func modelFeedback(decision Decision, tier PermissionTier) map[string]interface{} {
	manifest := decision.Manifest
	feedback := map[string]interface{}{
		"status":            string(decision.Status),
		"reason":            decision.Reason,
		"correlation_id":    decision.CorrelationID,
		"tool_id":           manifest.ToolID,
		"skill_id":          manifest.SkillID,
		"effect":            string(manifest.Effect),
		"asset_type":        manifest.AssetType,
		"asset_count":       len(decision.Assets),
		"risk_level":        string(manifest.RiskLevel),
		"permission_tier":   string(tier),
		"requires_approval": decision.RequiresApproval,
	}
	if decision.ApprovedByCorrelationID != "" {
		feedback["approved_by_correlation_id"] = decision.ApprovedByCorrelationID
	}
	if decision.MatchedGrant != nil {
		feedback["matched_grant"] = *decision.MatchedGrant
		if len(decision.Assets) > 0 {
			feedback["matched_assets"] = decision.Assets
		}
	}
	if len(decision.Assets) > 0 {
		feedback["assets"] = decision.Assets
	}
	if decision.Preauthorization != nil {
		feedback["preauthorization"] = *decision.Preauthorization
		if decision.Preauthorization.Source != "" {
			feedback["authorization_source"] = decision.Preauthorization.Source
		}
		if decision.Preauthorization.Code != "" {
			feedback["authorization_code"] = decision.Preauthorization.Code
		}
		if len(decision.Preauthorization.Resources) > 0 {
			feedback["authorization_resources"] = decision.Preauthorization.Resources
		}
	}
	if len(decision.AssetOperationAudit) > 0 {
		feedback["asset_operation_audit"] = decision.AssetOperationAudit
	}
	if len(decision.ExpectedAssets) > 0 {
		feedback["expected_assets"] = decision.ExpectedAssets
		feedback["expected_asset_count"] = len(decision.ExpectedAssets)
	}
	return feedback
}

func assetOperationAuditPayload(decision Decision, tier PermissionTier, req Request) map[string]interface{} {
	manifest := decision.Manifest
	if !manifest.AuditRequired && !isAssetOperation(manifest) && !decision.RequiresApproval && decision.Preauthorization == nil {
		return nil
	}
	audit := map[string]interface{}{
		"schema_version":       "tool_governance.asset_operation.v1",
		"event_type":           "asset_operation",
		"correlation_id":       strings.TrimSpace(decision.CorrelationID),
		"conversation_id":      strings.TrimSpace(req.ConversationID),
		"organization_id":      strings.TrimSpace(req.OrganizationID),
		"user_id":              strings.TrimSpace(req.UserID),
		"governance_status":    string(decision.Status),
		"requires_approval":    decision.RequiresApproval,
		"decision_reason":      strings.TrimSpace(decision.Reason),
		"tool_id":              manifest.ToolID,
		"skill_id":             firstNonEmptyString(req.SkillID, manifest.SkillID),
		"provider_type":        strings.TrimSpace(req.ProviderType),
		"provider_id":          strings.TrimSpace(req.ProviderID),
		"domain":               manifest.Domain,
		"effect":               string(manifest.Effect),
		"asset_type":           manifest.AssetType,
		"asset_count":          len(decision.Assets),
		"risk_level":           string(manifest.RiskLevel),
		"permission_tier":      string(tier),
		"reversible":           manifest.Reversible,
		"bulk_sensitive":       manifest.BulkSensitive,
		"external_side_effect": manifest.ExternalSideEffect,
		"audit_required":       manifest.AuditRequired,
		"idempotency_required": manifest.IdempotencyRequired,
	}
	if len(manifest.PermissionScopes) > 0 {
		audit["permission_scopes"] = manifest.PermissionScopes
	}
	if len(decision.Assets) > 0 {
		audit["assets"] = decision.Assets
	}
	if len(decision.ExpectedAssets) > 0 {
		audit["expected_assets"] = decision.ExpectedAssets
	}
	if decision.RequiresApproval {
		audit["approval_status"] = "pending"
	}
	if decision.ApprovedByCorrelationID != "" {
		audit["approval_status"] = "approved"
		audit["approved_by_correlation_id"] = decision.ApprovedByCorrelationID
	}
	if decision.MatchedGrant != nil {
		audit["matched_grant"] = *decision.MatchedGrant
	}
	if decision.Preauthorization != nil {
		audit["preauthorization"] = *decision.Preauthorization
		audit["authorization_source"] = decision.Preauthorization.Source
		audit["authorization_binding_type"] = decision.Preauthorization.BindingType
		audit["authorization_actor_id"] = decision.Preauthorization.AuthorizedBy
		if decision.Preauthorization.AuthorizedAt != nil {
			audit["authorization_granted_at"] = *decision.Preauthorization.AuthorizedAt
		}
		if len(decision.Preauthorization.Resources) > 0 {
			audit["authorization_resources"] = decision.Preauthorization.Resources
		}
		if decision.Preauthorization.Code != "" {
			audit["authorization_code"] = decision.Preauthorization.Code
		}
	}
	return compactAuditPayload(audit)
}

func compactAuditPayload(payload map[string]interface{}) map[string]interface{} {
	for key, value := range payload {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) == "" {
				delete(payload, key)
			}
		case []AssetRef:
			if len(typed) == 0 {
				delete(payload, key)
			}
		case []string:
			if len(typed) == 0 {
				delete(payload, key)
			}
		}
	}
	return payload
}

func (p Policy) withDefaults() Policy {
	defaults := DefaultPolicy()
	if p.DefaultPermissionTier == "" {
		p.DefaultPermissionTier = defaults.DefaultPermissionTier
	}
	if !p.HighRiskRequiresApproval {
		p.HighRiskRequiresApproval = defaults.HighRiskRequiresApproval
	}
	if !p.CriticalRiskRequiresApproval {
		p.CriticalRiskRequiresApproval = defaults.CriticalRiskRequiresApproval
	}
	if !p.DeleteRequiresApproval {
		p.DeleteRequiresApproval = defaults.DeleteRequiresApproval
	}
	if !p.BulkSensitiveRequiresApproval {
		p.BulkSensitiveRequiresApproval = defaults.BulkSensitiveRequiresApproval
	}
	if !p.ExternalSideEffectRequiresApproval {
		p.ExternalSideEffectRequiresApproval = defaults.ExternalSideEffectRequiresApproval
	}
	return p
}

func (p Policy) hardRequiresApproval(manifest Manifest) bool {
	if p.ExternalSideEffectRequiresApproval && manifest.ExternalSideEffect {
		return true
	}
	if p.BulkSensitiveRequiresApproval && manifest.BulkSensitive {
		return true
	}
	if p.DeleteRequiresApproval && manifest.Effect == EffectDelete {
		return true
	}
	if p.CriticalRiskRequiresApproval && manifest.RiskLevel == RiskLevelCritical {
		return true
	}
	if p.HighRiskRequiresApproval && RiskRank(manifest.RiskLevel) >= RiskRank(RiskLevelHigh) {
		return true
	}
	return false
}

func hardApprovalReason(policy Policy, manifest Manifest) string {
	if policy.ExternalSideEffectRequiresApproval && manifest.ExternalSideEffect {
		return "external side effect requires user approval"
	}
	if policy.BulkSensitiveRequiresApproval && manifest.BulkSensitive {
		return "bulk-sensitive tool requires user approval"
	}
	if policy.DeleteRequiresApproval && manifest.Effect == EffectDelete {
		return "delete effect requires user approval"
	}
	if policy.CriticalRiskRequiresApproval && manifest.RiskLevel == RiskLevelCritical {
		return "critical risk requires user approval"
	}
	if policy.HighRiskRequiresApproval && RiskRank(manifest.RiskLevel) >= RiskRank(RiskLevelHigh) {
		return "high risk requires user approval"
	}
	return "tool requires user approval"
}

func basicTierAllows(manifest Manifest) bool {
	if !isAssetOperation(manifest) {
		return RiskRank(manifest.RiskLevel) <= RiskRank(RiskLevelLow)
	}
	return manifest.Effect == EffectRead &&
		RiskRank(manifest.RiskLevel) <= RiskRank(RiskLevelLow) &&
		!manifest.BulkSensitive &&
		!manifest.ExternalSideEffect
}

func advancedTierAllows(manifest Manifest) bool {
	if !isAssetOperation(manifest) {
		return RiskRank(manifest.RiskLevel) <= RiskRank(RiskLevelMedium)
	}
	switch manifest.Effect {
	case EffectRead, EffectCreate, EffectUpdate:
		return RiskRank(manifest.RiskLevel) <= RiskRank(RiskLevelMedium) &&
			!manifest.BulkSensitive &&
			!manifest.ExternalSideEffect
	default:
		return false
	}
}

func isAssetOperation(manifest Manifest) bool {
	if manifest.AssetType != "" {
		return true
	}
	switch manifest.Effect {
	case EffectRead, EffectCreate, EffectUpdate, EffectDelete, EffectPublish, EffectInvoke, EffectSchedule, EffectExternalSend:
		return true
	default:
		return false
	}
}

func tierAllowed(manifest Manifest, tier PermissionTier) bool {
	if len(manifest.AllowedPermissionTiers) == 0 {
		return true
	}
	for _, allowed := range manifest.AllowedPermissionTiers {
		if allowed == tier {
			return true
		}
	}
	return false
}

func assetsMatchExpectedAssets(assets []AssetRef, expected []AssetRef) bool {
	if len(assets) == 0 || len(expected) == 0 {
		return true
	}
	for _, asset := range assets {
		matched := false
		for _, expectedAsset := range expected {
			if assetMatchesExpectedAsset(asset, expectedAsset) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func assetMatchesExpectedAsset(asset AssetRef, expected AssetRef) bool {
	if asset.Type != "" && expected.Type != "" && asset.Type != expected.Type {
		return false
	}
	if asset.WorkspaceID != "" && expected.WorkspaceID != "" && asset.WorkspaceID != expected.WorkspaceID {
		return false
	}
	if asset.ID != "" || expected.ID != "" {
		return asset.ID != "" && expected.ID != "" && asset.ID == expected.ID
	}
	if asset.Name != "" || expected.Name != "" {
		return asset.Name != "" && expected.Name != "" && strings.EqualFold(asset.Name, expected.Name)
	}
	return false
}

func matchingSessionGrant(req Request, manifest Manifest, assets []AssetRef) (SessionGrant, bool) {
	conversationID := strings.TrimSpace(req.ConversationID)
	if conversationID == "" {
		return SessionGrant{}, false
	}
	now := time.Now()
	for _, raw := range req.SessionGrants {
		grant := normalizeSessionGrant(raw)
		if grant.ConversationID != conversationID {
			continue
		}
		if !grantScopeMatches(strings.TrimSpace(req.OrganizationID), grant.OrganizationID) ||
			!grantScopeMatches(strings.TrimSpace(req.UserID), grant.UserID) ||
			!grantScopeMatches(strings.TrimSpace(req.SkillID), grant.SkillID) ||
			!grantScopeMatches(strings.TrimSpace(req.ProviderType), grant.ProviderType) ||
			!grantScopeMatches(strings.TrimSpace(req.ProviderID), grant.ProviderID) {
			continue
		}
		if grant.ToolID != manifest.ToolID || grant.Effect != manifest.Effect || grant.AssetType != manifest.AssetType {
			continue
		}
		if RiskRank(grant.RiskLevel) < RiskRank(manifest.RiskLevel) {
			continue
		}
		if requiresScopedGrantExpiry(req, grant) && grant.ExpiresAt.IsZero() {
			continue
		}
		if !grant.ExpiresAt.IsZero() && !grant.ExpiresAt.After(now) {
			continue
		}
		if len(grant.Assets) > 0 && !assetsMatchExpectedAssets(assets, grant.Assets) {
			continue
		}
		return grant, true
	}
	return SessionGrant{}, false
}

func grantScopeMatches(requestValue string, grantValue string) bool {
	requestValue = strings.TrimSpace(requestValue)
	grantValue = strings.TrimSpace(grantValue)
	if requestValue == "" {
		return grantValue == ""
	}
	return grantValue == requestValue
}

func requiresScopedGrantExpiry(req Request, grant SessionGrant) bool {
	return strings.TrimSpace(req.OrganizationID) != "" ||
		strings.TrimSpace(req.UserID) != "" ||
		strings.TrimSpace(req.SkillID) != "" ||
		strings.TrimSpace(req.ProviderType) != "" ||
		strings.TrimSpace(req.ProviderID) != "" ||
		strings.TrimSpace(grant.OrganizationID) != "" ||
		strings.TrimSpace(grant.UserID) != "" ||
		strings.TrimSpace(grant.SkillID) != "" ||
		strings.TrimSpace(grant.ProviderType) != "" ||
		strings.TrimSpace(grant.ProviderID) != ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
