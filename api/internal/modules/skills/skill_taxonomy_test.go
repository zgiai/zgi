package skills

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeSkillDisplayScenarios(t *testing.T) {
	frontmatter := SkillFrontmatter{
		Name: "taxonomy-test",
		Display: SkillDisplayMetadata{
			Scenarios: []string{
				" Document_Handling ",
				"",
				"document_handling",
				"LEGAL_COMPLIANCE",
			},
		},
	}

	got := normalizeSkillDisplay(frontmatter).Scenarios
	want := []string{"document_handling", "legal_compliance"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scenarios = %#v, want %#v", got, want)
	}
}

func TestEmbeddedSkillCatalogUsesKnownTaxonomy(t *testing.T) {
	knownCategories := map[string]struct{}{
		"general_tools":       {},
		"office_productivity": {},
		"document_processing": {},
		"data_analysis":       {},
		"knowledge_retrieval": {},
		"content_creation":    {},
		"planning_decision":   {},
		"workflow_automation": {},
		"security_compliance": {},
	}
	knownScenarios := map[string]struct{}{
		"general":               {},
		"office_collaboration":  {},
		"document_handling":     {},
		"content_creation":      {},
		"data_insights":         {},
		"knowledge_research":    {},
		"planning_decision":     {},
		"business_operations":   {},
		"customer_service":      {},
		"hr_recruiting":         {},
		"legal_compliance":      {},
		"technical_development": {},
	}

	catalog, err := NewRuntime(nil, nil).ListSystemSkillsBestEffort(context.Background())
	if err != nil {
		t.Fatalf("ListSystemSkillsBestEffort() error = %v", err)
	}
	if len(catalog) != 30 {
		t.Fatalf("catalog size = %d, want 30", len(catalog))
	}

	iconOwners := make(map[string]string, len(catalog))
	for _, skill := range catalog {
		if owner, duplicated := iconOwners[skill.Display.Icon]; duplicated {
			t.Errorf("skills %q and %q share display icon %q", owner, skill.ID, skill.Display.Icon)
		} else if skill.Display.Icon == "" {
			t.Errorf("skill %q display icon is empty", skill.ID)
		} else {
			iconOwners[skill.Display.Icon] = skill.ID
		}

		for locale, prefix := range map[string]string{
			"en_US":   "Designed for",
			"zh_Hans": "适用于",
		} {
			description := skill.Display.Description[locale]
			if !strings.HasPrefix(description, prefix) {
				t.Errorf("skill %q %s description = %q, want prefix %q", skill.ID, locale, description, prefix)
			}
		}

		if _, ok := knownCategories[skill.Display.Category]; !ok {
			t.Errorf("skill %q category = %q, want known category", skill.ID, skill.Display.Category)
		}
		if len(skill.Display.Scenarios) == 0 || len(skill.Display.Scenarios) > 4 {
			t.Errorf("skill %q scenarios = %#v, want 1-4 values", skill.ID, skill.Display.Scenarios)
		}
		seen := make(map[string]struct{}, len(skill.Display.Scenarios))
		for _, scenario := range skill.Display.Scenarios {
			if _, ok := knownScenarios[scenario]; !ok {
				t.Errorf("skill %q scenario = %q, want known scenario", skill.ID, scenario)
			}
			if _, ok := seen[scenario]; ok {
				t.Errorf("skill %q scenario %q is duplicated", skill.ID, scenario)
			}
			seen[scenario] = struct{}{}
		}
	}
}
