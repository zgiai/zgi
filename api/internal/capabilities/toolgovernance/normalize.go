package toolgovernance

import "strings"

func NormalizeManifest(manifest Manifest) Manifest {
	manifest.ToolID = strings.TrimSpace(manifest.ToolID)
	manifest.SkillID = strings.TrimSpace(manifest.SkillID)
	manifest.Domain = strings.ToLower(strings.TrimSpace(manifest.Domain))
	manifest.Effect = NormalizeEffect(manifest.Effect)
	manifest.AssetType = normalizeAssetType(manifest.AssetType)
	manifest.RiskLevel = NormalizeRiskLevel(manifest.RiskLevel)
	manifest.DefaultApprovalPolicy = NormalizeApprovalPolicy(manifest.DefaultApprovalPolicy)
	manifest.PermissionScopes = normalizeStringList(manifest.PermissionScopes)
	manifest.AllowedPermissionTiers = normalizePermissionTierList(manifest.AllowedPermissionTiers)
	return manifest
}

func NormalizeEffect(effect Effect) Effect {
	switch Effect(strings.ToLower(strings.TrimSpace(string(effect)))) {
	case EffectRead:
		return EffectRead
	case EffectCreate:
		return EffectCreate
	case EffectUpdate:
		return EffectUpdate
	case EffectDelete:
		return EffectDelete
	case EffectPublish:
		return EffectPublish
	case EffectInvoke:
		return EffectInvoke
	case EffectSchedule:
		return EffectSchedule
	case EffectExternalSend:
		return EffectExternalSend
	default:
		return EffectNone
	}
}

func NormalizeRiskLevel(risk RiskLevel) RiskLevel {
	switch RiskLevel(strings.ToLower(strings.TrimSpace(string(risk)))) {
	case RiskLevelCritical:
		return RiskLevelCritical
	case RiskLevelHigh:
		return RiskLevelHigh
	case RiskLevelMedium:
		return RiskLevelMedium
	default:
		return RiskLevelLow
	}
}

func NormalizePermissionTier(tier PermissionTier) PermissionTier {
	switch PermissionTier(strings.ToLower(strings.TrimSpace(string(tier)))) {
	case PermissionTierFull:
		return PermissionTierFull
	case PermissionTierAdvanced:
		return PermissionTierAdvanced
	case PermissionTierBasic:
		return PermissionTierBasic
	default:
		return ""
	}
}

func NormalizeApprovalPolicy(policy ApprovalPolicy) ApprovalPolicy {
	switch ApprovalPolicy(strings.ToLower(strings.TrimSpace(string(policy)))) {
	case ApprovalPolicyAlwaysAsk:
		return ApprovalPolicyAlwaysAsk
	case ApprovalPolicyNeverAsk:
		return ApprovalPolicyNeverAsk
	default:
		return ApprovalPolicyAutoByPermissionTier
	}
}

func RiskRank(risk RiskLevel) int {
	switch NormalizeRiskLevel(risk) {
	case RiskLevelCritical:
		return 4
	case RiskLevelHigh:
		return 3
	case RiskLevelMedium:
		return 2
	default:
		return 1
	}
}

func normalizeAssets(assets []AssetRef) []AssetRef {
	if len(assets) == 0 {
		return nil
	}
	out := make([]AssetRef, 0, len(assets))
	for _, asset := range assets {
		asset.ID = strings.TrimSpace(asset.ID)
		asset.Type = normalizeAssetType(asset.Type)
		asset.Name = strings.TrimSpace(asset.Name)
		asset.WorkspaceID = strings.TrimSpace(asset.WorkspaceID)
		asset.Source = strings.TrimSpace(asset.Source)
		if asset.ID == "" && asset.Name == "" && asset.Type == "" {
			continue
		}
		out = append(out, asset)
	}
	return out
}

func normalizeSessionGrant(grant SessionGrant) SessionGrant {
	grant.ConversationID = strings.TrimSpace(grant.ConversationID)
	grant.ToolID = strings.TrimSpace(grant.ToolID)
	grant.Effect = NormalizeEffect(grant.Effect)
	grant.AssetType = normalizeAssetType(grant.AssetType)
	grant.RiskLevel = NormalizeRiskLevel(grant.RiskLevel)
	return grant
}

func normalizeAssetType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizePermissionTierList(values []PermissionTier) []PermissionTier {
	if len(values) == 0 {
		return nil
	}
	seen := map[PermissionTier]struct{}{}
	out := make([]PermissionTier, 0, len(values))
	for _, raw := range values {
		tier := NormalizePermissionTier(raw)
		if tier == "" {
			continue
		}
		if _, ok := seen[tier]; ok {
			continue
		}
		seen[tier] = struct{}{}
		out = append(out, tier)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
