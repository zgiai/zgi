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
		Status:        DecisionStatusAllowed,
		CorrelationID: correlationID,
		Manifest:      manifest,
		Assets:        assets,
	}

	if manifest.ToolID == "" {
		return base.withStatus(DecisionStatusDenied, "tool_id is required", false)
	}
	if !tierAllowed(manifest, tier) {
		return base.withStatus(DecisionStatusDenied, "permission tier is not allowed for this tool", false)
	}
	if isAssetOperation(manifest) && manifest.RequiresAssetResolution && len(assets) == 0 {
		decision := base.withStatus(DecisionStatusNeedsResolution, "asset resolution is required before this tool can run", false)
		decision.ModelFeedback = modelFeedback(decision, tier)
		return decision
	}
	if policy.CriticalRiskBlocked && manifest.RiskLevel == RiskLevelCritical {
		decision := base.withStatus(DecisionStatusBlocked, "critical risk tools are blocked by policy", false)
		decision.ModelFeedback = modelFeedback(decision, tier)
		return decision
	}
	if matchingSessionGrant(req.SessionGrants, req.ConversationID, manifest) {
		decision := base.withStatus(DecisionStatusAllowed, "allowed by matching session grant", false)
		decision.ModelFeedback = modelFeedback(decision, tier)
		return decision
	}
	if manifest.DefaultApprovalPolicy == ApprovalPolicyAlwaysAsk {
		return base.needsApproval(tier, req.ConversationID, "tool manifest requires approval")
	}
	if manifest.DefaultApprovalPolicy == ApprovalPolicyNeverAsk && !policy.hardRequiresApproval(manifest) {
		decision := base.withStatus(DecisionStatusAllowed, "allowed by manifest approval policy", false)
		decision.ModelFeedback = modelFeedback(decision, tier)
		return decision
	}
	if policy.hardRequiresApproval(manifest) {
		return base.needsApproval(tier, req.ConversationID, hardApprovalReason(policy, manifest))
	}

	switch tier {
	case PermissionTierFull:
		decision := base.withStatus(DecisionStatusAllowed, "allowed by full permission tier", false)
		decision.ModelFeedback = modelFeedback(decision, tier)
		return decision
	case PermissionTierAdvanced:
		if advancedTierAllows(manifest) {
			decision := base.withStatus(DecisionStatusAllowed, "allowed by advanced permission tier", false)
			decision.ModelFeedback = modelFeedback(decision, tier)
			return decision
		}
	case PermissionTierBasic:
		if basicTierAllows(manifest) {
			decision := base.withStatus(DecisionStatusAllowed, "allowed by basic permission tier", false)
			decision.ModelFeedback = modelFeedback(decision, tier)
			return decision
		}
	}
	return base.needsApproval(tier, req.ConversationID, "permission tier requires user approval for this tool")
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
			ConversationID: strings.TrimSpace(conversationID),
			ToolID:         manifest.ToolID,
			Effect:         manifest.Effect,
			AssetType:      manifest.AssetType,
			RiskLevel:      manifest.RiskLevel,
		},
	}
}

func modelFeedback(decision Decision, tier PermissionTier) map[string]interface{} {
	manifest := decision.Manifest
	return map[string]interface{}{
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

func matchingSessionGrant(grants []SessionGrant, conversationID string, manifest Manifest) bool {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return false
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
		return true
	}
	return false
}
