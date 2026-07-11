package toolgovernance

import (
	"fmt"
	"strings"
)

// ValidateManifest normalizes a manifest and fails closed on fields that would
// otherwise be silently defaulted by NormalizeManifest.
func ValidateManifest(manifest Manifest) (Manifest, error) {
	normalized := NormalizeManifest(manifest)
	var problems []string

	if normalized.ToolID == "" {
		problems = append(problems, "tool_id is required")
	}
	if normalized.SkillID == "" {
		problems = append(problems, "skill_id is required")
	}
	if normalized.Domain == "" {
		problems = append(problems, "domain is required")
	}
	if normalized.AssetType == "" {
		problems = append(problems, "asset_type is required")
	}
	if !validManifestEffect(manifest.Effect) {
		problems = append(problems, fmt.Sprintf("effect %q is invalid", strings.TrimSpace(string(manifest.Effect))))
	}
	if !validManifestRiskLevel(manifest.RiskLevel) {
		problems = append(problems, fmt.Sprintf("risk_level %q is invalid", strings.TrimSpace(string(manifest.RiskLevel))))
	}
	if !validManifestApprovalPolicy(manifest.DefaultApprovalPolicy) {
		problems = append(problems, fmt.Sprintf("default_approval_policy %q is invalid", strings.TrimSpace(string(manifest.DefaultApprovalPolicy))))
	}
	if len(manifest.AllowedPermissionTiers) == 0 {
		problems = append(problems, "allowed_permission_tiers is required")
	} else {
		for _, raw := range manifest.AllowedPermissionTiers {
			if !validManifestPermissionTier(raw) {
				problems = append(problems, fmt.Sprintf("allowed_permission_tiers contains invalid tier %q", strings.TrimSpace(string(raw))))
			}
		}
	}
	if len(normalized.PermissionScopes) == 0 {
		problems = append(problems, "permission_scopes is required")
	}

	if len(problems) > 0 {
		return Manifest{}, fmt.Errorf("invalid tool governance manifest: %s", strings.Join(problems, "; "))
	}
	return normalized, nil
}

func validManifestEffect(effect Effect) bool {
	switch Effect(strings.ToLower(strings.TrimSpace(string(effect)))) {
	case EffectRead, EffectCreate, EffectUpdate, EffectDelete, EffectPublish, EffectInvoke, EffectSchedule, EffectExternalSend:
		return true
	default:
		return false
	}
}

func validManifestRiskLevel(risk RiskLevel) bool {
	switch RiskLevel(strings.ToLower(strings.TrimSpace(string(risk)))) {
	case RiskLevelLow, RiskLevelMedium, RiskLevelHigh, RiskLevelCritical:
		return true
	default:
		return false
	}
}

func validManifestApprovalPolicy(policy ApprovalPolicy) bool {
	switch ApprovalPolicy(strings.ToLower(strings.TrimSpace(string(policy)))) {
	case ApprovalPolicyAutoByPermissionTier, ApprovalPolicyAlwaysAsk, ApprovalPolicyNeverAsk:
		return true
	default:
		return false
	}
}

func validManifestPermissionTier(tier PermissionTier) bool {
	switch PermissionTier(strings.ToLower(strings.TrimSpace(string(tier)))) {
	case PermissionTierBasic, PermissionTierAdvanced, PermissionTierFull:
		return true
	default:
		return false
	}
}
