package skills

import (
	"sort"
	"strings"
)

// Resident skills are built-in skills whose tools are offered from the first
// round of every turn, without activation (ADR 0043). Their documents are not
// sent: their tool descriptions carry what a model needs to use them.
func IsResidentSkill(skillID string) bool {
	switch normalizeSkillID(skillID) {
	case SkillCalculator, SkillTime:
		return true
	default:
		return false
	}
}

// residentToolNames are the tools a resident skill offers the model. Its other
// tools stay available to workflows and older loops; offering them in every
// request would cost more than the rare call they save (ADR 0043).
var residentToolNames = map[string][]string{
	SkillCalculator: {"evaluate_expression"},
}

// residentTools keeps the tools of a resident skill that it offers the model.
// A skill whose document lists none of them keeps all of its tools.
func residentTools(skillID string, definitions []SkillToolDefinition) []SkillToolDefinition {
	allowed := residentToolNames[normalizeSkillID(skillID)]
	if len(allowed) == 0 || !IsResidentSkill(skillID) {
		return definitions
	}
	kept := make([]SkillToolDefinition, 0, len(allowed))
	for _, definition := range definitions {
		for _, name := range allowed {
			if strings.TrimSpace(definition.Name) == name {
				kept = append(kept, definition)
				break
			}
		}
	}
	if len(kept) == 0 {
		return definitions
	}
	return kept
}

// ResidentSkillIDs returns the resident skills of resolved in skill ID order,
// so their tools are listed the same way in every turn.
func ResidentSkillIDs(resolved *ResolvedSkills) []string {
	if resolved == nil {
		return nil
	}
	ids := make([]string, 0)
	for _, doc := range resolved.Skills {
		if skillID := normalizeSkillID(doc.Metadata.ID); skillID != "" && IsResidentSkill(skillID) {
			ids = appendUniqueNativeID(ids, skillID)
		}
	}
	sort.Strings(ids)
	return ids
}

// HasNonResidentSkill reports whether resolved holds a skill that must be
// activated before use, which is a business capability a plan can track.
func HasNonResidentSkill(resolved *ResolvedSkills) bool {
	if resolved == nil {
		return false
	}
	for _, doc := range resolved.Skills {
		if skillID := normalizeSkillID(doc.Metadata.ID); skillID != "" && !IsResidentSkill(skillID) {
			return true
		}
	}
	return false
}
