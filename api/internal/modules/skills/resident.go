package skills

import "sort"

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
