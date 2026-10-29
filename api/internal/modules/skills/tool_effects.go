package skills

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

// SkillSideEffectsNone declares a skill whose tools change nothing.
const SkillSideEffectsNone = "none"

// ToolIsReadOnly reports a tool whose call changes nothing, so it may run
// alongside other calls of a round, or again after an interrupted turn. Its
// governance manifest says it only reads and has no external side effect, or
// it belongs to a system skill that declares side_effects: none. A member's
// own skill cannot declare that, and a connector operation never counts: its
// dispatch keeps a ledger per call.
func ToolIsReadOnly(doc SkillDocument, toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	for _, tool := range doc.Tools {
		if !strings.EqualFold(strings.TrimSpace(tool.Name), toolName) {
			continue
		}
		if tool.ConnectorNative {
			return false
		}
		if tool.Governance != nil {
			manifest := toolgovernance.NormalizeManifest(*tool.Governance)
			return manifest.Effect == toolgovernance.EffectRead && !manifest.ExternalSideEffect
		}
		return normalizeSkillSource(doc.Metadata.Source) == SkillSourceSystem && doc.Metadata.SideEffects == SkillSideEffectsNone
	}
	return false
}

// SystemSkillToolIsReadOnly is ToolIsReadOnly for a tool of a system skill,
// looked up by IDs alone.
func SystemSkillToolIsReadOnly(skillID, toolName string) bool {
	if manifest, ok := SystemSkillToolGovernanceManifest(skillID, toolName); ok {
		manifest = toolgovernance.NormalizeManifest(manifest)
		return manifest.Effect == toolgovernance.EffectRead && !manifest.ExternalSideEffect
	}
	systemToolGovernanceOnce.Do(loadSystemToolGovernanceCatalog)
	return systemToolGovernanceErr == nil && systemSideEffectFreeSkills[normalizeSkillID(skillID)]
}
