package service

import (
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const skillToolGovernanceRuntimeKey = "tool_governance"

type toolGovernanceRuntimeProfile struct {
	PermissionTier string
	CallerType     string
	RuntimeSurface string
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
	if !toolGovernancePermissionTierOverrideAllowed(prepared) {
		governance["permission_tier"] = profile.PermissionTier
	} else if strings.TrimSpace(stringMetadataValue(governance["permission_tier"])) == "" {
		governance["permission_tier"] = profile.PermissionTier
	}
	if profile.CallerType != "" {
		governance["caller_type"] = profile.CallerType
	}
	if profile.RuntimeSurface != "" {
		governance["runtime_surface"] = profile.RuntimeSurface
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
		profile.RuntimeSurface = normalizeRuntimeSurfaceForCaller(prepared.Caller, prepared.parts.Surface)
	}
	return profile
}

func skillToolGovernancePermissionTierFromPrepared(params map[string]interface{}, prepared *PreparedChat) string {
	if !toolGovernancePermissionTierOverrideAllowed(prepared) {
		return ""
	}
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

func toolGovernancePermissionTierOverrideAllowed(prepared *PreparedChat) bool {
	if prepared == nil {
		return true
	}
	if normalizeCallerType(prepared.Caller.Type) != runtimemodel.ConversationCallerAIChat {
		return false
	}
	if prepared.parts == nil {
		return true
	}
	switch normalizeAIChatSurface(prepared.parts.Surface) {
	case aiChatSurfaceWorkChat, aiChatSurfaceContextualSidebar:
		return true
	default:
		return false
	}
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
