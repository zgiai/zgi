package service

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func skillLoopToolLooksAssetMutation(skillID string, toolName string) bool {
	manifest, ok := skillLoopToolGovernanceManifest(skillID, toolName)
	if !ok {
		return false
	}
	return skillLoopGovernanceEffectIsMutation(toolgovernance.NormalizeManifest(manifest).Effect)
}

func skillLoopToolLooksReadOnly(skillID string, toolName string) bool {
	manifest, ok := skillLoopToolGovernanceManifest(skillID, toolName)
	return ok && toolgovernance.NormalizeManifest(manifest).Effect == toolgovernance.EffectRead
}

func skillLoopToolGovernanceManifest(skillID string, toolName string) (toolgovernance.Manifest, bool) {
	if manifest, ok := skills.SystemSkillToolGovernanceManifest(skillID, toolName); ok {
		return manifest, true
	}
	if strings.TrimSpace(skillID) != "" {
		return toolgovernance.Manifest{}, false
	}
	return skills.SystemSkillToolGovernanceManifestByToolName(toolName)
}

func skillLoopGovernanceEffectIsMutation(effect toolgovernance.Effect) bool {
	switch effect {
	case toolgovernance.EffectCreate,
		toolgovernance.EffectUpdate,
		toolgovernance.EffectDelete,
		toolgovernance.EffectPublish,
		toolgovernance.EffectInvoke,
		toolgovernance.EffectSchedule,
		toolgovernance.EffectExternalSend:
		return true
	default:
		return false
	}
}

func modelTurnIntentAssetEffectIsDelete(effect string) bool {
	switch strings.ToLower(strings.TrimSpace(effect)) {
	case "delete", "remove", "destroy":
		return true
	default:
		return false
	}
}

func modelTurnIntentAssetEffectIsMutation(effect string) bool {
	switch strings.ToLower(strings.TrimSpace(effect)) {
	case "create", "update", "edit", "modify", "change", "delete", "remove", "bind", "unbind", "replace", "asset_mutation":
		return true
	default:
		return false
	}
}
