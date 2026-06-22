package service

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const skillToolGovernanceRuntimeKey = "tool_governance"

type toolGovernanceRuntimeProfile struct {
	PermissionTier string
	CallerType     string
	RuntimeSurface string
	Assets         []map[string]interface{}
	SessionGrants  []map[string]interface{}
}

func applySkillToolGovernanceRuntimeParameters(params map[string]interface{}, prepared *PreparedChat) map[string]interface{} {
	if params == nil {
		params = map[string]interface{}{}
	}
	governance := map[string]interface{}{}
	if existing, ok := params[skillToolGovernanceRuntimeKey].(map[string]interface{}); ok {
		governance = copyStringAnyMap(existing)
	}
	if governance == nil {
		governance = map[string]interface{}{}
	}
	profile := buildToolGovernanceRuntimeProfile(params, prepared)
	if strings.TrimSpace(stringMetadataValue(governance["permission_tier"])) == "" {
		governance["permission_tier"] = profile.PermissionTier
	}
	if strings.TrimSpace(stringMetadataValue(governance["caller_type"])) == "" && profile.CallerType != "" {
		governance["caller_type"] = profile.CallerType
	}
	if strings.TrimSpace(stringMetadataValue(governance["runtime_surface"])) == "" && profile.RuntimeSurface != "" {
		governance["runtime_surface"] = profile.RuntimeSurface
	}
	if _, exists := governance["assets"]; !exists {
		if _, flatAssetsExist := params["tool_governance_assets"]; !flatAssetsExist {
			if len(profile.Assets) > 0 {
				governance["assets"] = profile.Assets
			}
		}
	}
	if _, exists := governance["session_grants"]; !exists {
		if _, flatGrantsExist := params["tool_governance_session_grants"]; !flatGrantsExist {
			if len(profile.SessionGrants) > 0 {
				governance["session_grants"] = profile.SessionGrants
			}
		}
	}
	params[skillToolGovernanceRuntimeKey] = governance
	return params
}

func buildToolGovernanceRuntimeProfile(params map[string]interface{}, prepared *PreparedChat) toolGovernanceRuntimeProfile {
	profile := toolGovernanceRuntimeProfile{
		PermissionTier: skillToolGovernancePermissionTierFromPrepared(params, prepared),
		Assets:         skillToolGovernanceAssetsFromPrepared(prepared),
		SessionGrants:  skillToolGovernanceSessionGrantsFromPrepared(prepared),
	}
	if profile.PermissionTier == "" {
		profile.PermissionTier = string(toolgovernance.PermissionTierBasic)
	}
	if prepared == nil {
		return profile
	}
	profile.CallerType = normalizeCallerType(prepared.Caller.Type)
	if prepared.parts != nil {
		profile.RuntimeSurface = normalizeAIChatSurface(prepared.parts.Surface)
	}
	return profile
}

func skillToolGovernancePermissionTierFromPrepared(params map[string]interface{}, prepared *PreparedChat) string {
	if tier := normalizeSkillToolGovernancePermissionTier(params["tool_governance_permission_tier"]); tier != "" {
		return tier
	}
	if prepared == nil || prepared.parts == nil {
		return ""
	}
	if tier := skillToolGovernancePermissionTierFromOperationContext(prepared.parts.RawOperationContext); tier != "" {
		return tier
	}
	return skillToolGovernancePermissionTierFromOperationContext(prepared.parts.OperationContext)
}

func skillToolGovernancePermissionTierFromOperationContext(context map[string]interface{}) string {
	governance := mapFromOperationContext(firstMapValue(context, "tool_governance", "toolGovernance"))
	if governance == nil {
		return ""
	}
	return normalizeSkillToolGovernancePermissionTier(firstMapValue(governance, "permission_tier", "permissionTier"))
}

func normalizeSkillToolGovernancePermissionTier(value interface{}) string {
	tier := toolgovernance.NormalizePermissionTier(toolgovernance.PermissionTier(stringMetadataValue(value)))
	if tier == "" {
		return ""
	}
	return string(tier)
}

func skillToolGovernanceAssetsFromPrepared(prepared *PreparedChat) []map[string]interface{} {
	if prepared == nil || prepared.parts == nil {
		return nil
	}
	refGroups := make([][]PlannerResourceRef, 0, 2)
	if refs := plannerResourceRefsFromConsoleFilesQuery(prepared.parts); len(refs) > 0 {
		refGroups = append(refGroups, refs)
	}
	refGroups = append(refGroups, []PlannerResourceRef{{Type: resourceTypeFile}})

	for _, refs := range refGroups {
		result := resolveChatResourceRefs(prepared.parts, refs)
		if !allResourceRefsResolved(result.Results) || len(result.Resources) == 0 {
			continue
		}
		return toolGovernanceAssetMapsFromResources(result.Resources)
	}
	return nil
}

func skillToolGovernanceSessionGrantsFromPrepared(prepared *PreparedChat) []map[string]interface{} {
	if prepared == nil || prepared.Conversation == nil {
		return nil
	}
	grants := mapSliceFromAny(prepared.Conversation.Metadata["tool_governance_session_grants"])
	if prepared.Message != nil {
		grants = append(grants, mapSliceFromAny(prepared.Message.Metadata["tool_governance_one_shot_grants"])...)
	}
	return mirrorLegacyFileReaderDeleteGrants(grants)
}

func mirrorLegacyFileReaderDeleteGrants(grants []map[string]interface{}) []map[string]interface{} {
	if len(grants) == 0 {
		return grants
	}
	out := append([]map[string]interface{}(nil), grants...)
	seen := map[string]struct{}{}
	for _, grant := range out {
		seen[toolGovernanceSessionGrantKey(grant)] = struct{}{}
	}
	for _, grant := range grants {
		if !isLegacyFileReaderDeleteGrant(grant) {
			continue
		}
		mirrored := copyStringAnyMap(grant)
		mirrored["skill_id"] = skills.SkillFileManager
		key := toolGovernanceSessionGrantKey(mirrored)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, mirrored)
	}
	return out
}

func isLegacyFileReaderDeleteGrant(grant map[string]interface{}) bool {
	if len(grant) == 0 {
		return false
	}
	if strings.TrimSpace(firstNonEmptyString(grant["skill_id"], grant["skillId"])) != skills.SkillFileReader {
		return false
	}
	if strings.TrimSpace(stringFromAny(grant["tool_id"])) != "file.delete" {
		return false
	}
	effect := strings.TrimSpace(stringFromAny(grant["effect"]))
	return effect == "" || effect == string(toolgovernance.EffectDelete)
}

func toolGovernanceAssetMapsFromResources(resources []ResourceRef) []map[string]interface{} {
	if len(resources) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	assets := make([]map[string]interface{}, 0, len(resources))
	for _, resource := range resources {
		id := strings.TrimSpace(resource.ID)
		name := strings.TrimSpace(resource.Name)
		if id == "" && name == "" {
			continue
		}
		resourceType := strings.TrimSpace(resource.Type)
		if resourceType == "" {
			resourceType = resourceTypeFile
		}
		key := resourceType + ":" + id + ":" + name
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		asset := map[string]interface{}{
			"type": resourceType,
		}
		if id != "" {
			asset["id"] = id
		}
		if name != "" {
			asset["name"] = name
		}
		if source := strings.TrimSpace(resource.Source); source != "" {
			asset["source"] = source
		}
		if metadata := copyStringAnyMap(resource.Metadata); len(metadata) > 0 {
			if workspaceID := strings.TrimSpace(stringMetadataValue(firstMapValue(metadata, "workspace_id", "workspaceId"))); workspaceID != "" {
				asset["workspace_id"] = workspaceID
			}
			asset["metadata"] = metadata
		}
		assets = append(assets, asset)
	}
	return assets
}
