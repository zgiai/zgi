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

	base := Decision{
		Status:         DecisionStatusAllowed,
		CorrelationID:  correlationID,
		Manifest:       manifest,
		Assets:         assets,
		ExpectedAssets: expectedAssets,
	}

	if manifest.ToolID == "" {
		return finalizeDecision(base.withStatus(DecisionStatusDenied, "tool_id is required", false), tier, req.ConversationID)
	}
	if !tierAllowed(manifest, tier) {
		return finalizeDecision(base.withStatus(DecisionStatusDenied, "permission tier is not allowed for this tool", false), tier, req.ConversationID)
	}
	if isAssetOperation(manifest) && manifest.RequiresAssetResolution && len(assets) == 0 {
		decision := base.withStatus(DecisionStatusNeedsResolution, "asset resolution is required before this tool can run", false)
		return finalizeDecision(decision, tier, req.ConversationID)
	}
	if isAssetOperation(manifest) && manifest.RequiresAssetResolution && len(expectedAssets) > 0 && !assetsMatchExpectedAssets(assets, expectedAssets) {
		decision := base.withStatus(DecisionStatusNeedsResolution, "tool arguments do not match resolved target assets", false)
		return finalizeDecision(decision, tier, req.ConversationID)
	}
	if policy.CriticalRiskBlocked && manifest.RiskLevel == RiskLevelCritical {
		decision := base.withStatus(DecisionStatusBlocked, "critical risk tools are blocked by policy", false)
		return finalizeDecision(decision, tier, req.ConversationID)
	}
	if grant, ok := matchingSessionGrant(req.SessionGrants, req.ConversationID, manifest, assets); ok {
		decision := base.withStatus(DecisionStatusAllowed, "allowed by matching session grant", false)
		decision = decision.withMatchedGrant(grant)
		return finalizeDecision(decision, tier, req.ConversationID)
	}
	if manifest.DefaultApprovalPolicy == ApprovalPolicyAlwaysAsk {
		return finalizeDecision(base.needsApproval(tier, req.ConversationID, "tool manifest requires approval"), tier, req.ConversationID)
	}
	if manifest.DefaultApprovalPolicy == ApprovalPolicyNeverAsk && !policy.hardRequiresApproval(manifest) {
		decision := base.withStatus(DecisionStatusAllowed, "allowed by manifest approval policy", false)
		return finalizeDecision(decision, tier, req.ConversationID)
	}
	if policy.hardRequiresApproval(manifest) {
		return finalizeDecision(base.needsApproval(tier, req.ConversationID, hardApprovalReason(policy, manifest)), tier, req.ConversationID)
	}

	switch tier {
	case PermissionTierFull:
		decision := base.withStatus(DecisionStatusAllowed, "allowed by full permission tier", false)
		return finalizeDecision(decision, tier, req.ConversationID)
	case PermissionTierAdvanced:
		if advancedTierAllows(manifest) {
			decision := base.withStatus(DecisionStatusAllowed, "allowed by advanced permission tier", false)
			return finalizeDecision(decision, tier, req.ConversationID)
		}
	case PermissionTierBasic:
		if basicTierAllows(manifest) {
			decision := base.withStatus(DecisionStatusAllowed, "allowed by basic permission tier", false)
			return finalizeDecision(decision, tier, req.ConversationID)
		}
	}
	return finalizeDecision(base.needsApproval(tier, req.ConversationID, "permission tier requires user approval for this tool"), tier, req.ConversationID)
}

func (d Decision) withStatus(status DecisionStatus, reason string, requiresApproval bool) Decision {
	d.Status = status
	d.Reason = reason
	d.RequiresApproval = requiresApproval
	return d
}

func (d Decision) needsApproval(tier PermissionTier, conversationID string, reason string) Decision {
	d = d.withStatus(DecisionStatusNeedsApproval, reason, true)
	d.ApprovalEvent = approvalEvent(d, tier, conversationID)
	d.ModelFeedback = modelFeedback(d, tier)
	return d
}

func (d Decision) withMatchedGrant(grant SessionGrant) Decision {
	grant = normalizeSessionGrant(grant)
	d.MatchedGrant = &grant
	d.ApprovedByCorrelationID = strings.TrimSpace(grant.ApprovalCorrelationID)
	return d
}

func finalizeDecision(decision Decision, tier PermissionTier, conversationID string) Decision {
	decision.AssetOperationAudit = assetOperationAuditPayload(decision, tier, conversationID)
	decision.ModelFeedback = modelFeedback(decision, tier)
	return decision
}

func approvalEvent(decision Decision, tier PermissionTier, conversationID string) *ApprovalEvent {
	manifest := decision.Manifest
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
			ConversationID:        strings.TrimSpace(conversationID),
			ToolID:                manifest.ToolID,
			Effect:                manifest.Effect,
			AssetType:             manifest.AssetType,
			Assets:                decision.Assets,
			RiskLevel:             manifest.RiskLevel,
			ApprovalCorrelationID: decision.CorrelationID,
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
	if len(decision.AssetOperationAudit) > 0 {
		feedback["asset_operation_audit"] = decision.AssetOperationAudit
	}
	if len(decision.ExpectedAssets) > 0 {
		feedback["expected_assets"] = decision.ExpectedAssets
		feedback["expected_asset_count"] = len(decision.ExpectedAssets)
	}
	return feedback
}

func assetOperationAuditPayload(decision Decision, tier PermissionTier, conversationID string) map[string]interface{} {
	manifest := decision.Manifest
	if !manifest.AuditRequired && !isAssetOperation(manifest) && !decision.RequiresApproval {
		return nil
	}
	audit := map[string]interface{}{
		"schema_version":       "tool_governance.asset_operation.v1",
		"event_type":           "asset_operation",
		"correlation_id":       strings.TrimSpace(decision.CorrelationID),
		"conversation_id":      strings.TrimSpace(conversationID),
		"governance_status":    string(decision.Status),
		"requires_approval":    decision.RequiresApproval,
		"decision_reason":      strings.TrimSpace(decision.Reason),
		"tool_id":              manifest.ToolID,
		"skill_id":             manifest.SkillID,
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

func matchingSessionGrant(grants []SessionGrant, conversationID string, manifest Manifest, assets []AssetRef) (SessionGrant, bool) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return SessionGrant{}, false
	}
	now := time.Now()
	for _, raw := range grants {
		grant := normalizeSessionGrant(raw)
		if grant.ConversationID != conversationID {
			continue
		}
		if grant.ToolID != manifest.ToolID || grant.Effect != manifest.Effect || grant.AssetType != manifest.AssetType {
			continue
		}
		if RiskRank(grant.RiskLevel) < RiskRank(manifest.RiskLevel) {
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
