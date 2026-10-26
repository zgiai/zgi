package digitalhuman

import (
	"testing"
	"unicode/utf8"
)

func TestMemoryInstructionsTemplatesFitTheLimits(t *testing.T) {
	for _, locale := range []string{"zh-Hans", "en-US"} {
		catalog := MemoryInstructionsTemplates(locale)
		if catalog.Version != MemoryInstructionsCatalogVersion || len(catalog.Templates) < 5 {
			t.Fatalf("%s catalog = %+v", locale, catalog)
		}
		ids := map[string]bool{}
		for _, template := range catalog.Templates {
			if ids[template.ID] || template.ID == "" || template.Name == "" || template.Description == "" || template.Instructions == "" {
				t.Fatalf("%s template %q is incomplete or duplicated", locale, template.ID)
			}
			ids[template.ID] = true
			if err := validateMemoryPolicy(map[string]any{memoryInstructionsKey: template.Instructions}); err != nil ||
				utf8.RuneCountInString(template.Instructions) > maxMemoryInstructionsRunes || len(template.Samples) == 0 {
				t.Fatalf("%s template %q breaks a limit", locale, template.ID)
			}
			for _, sample := range template.Samples {
				if sample.Name == "" || len(sample.Turns) == 0 || len(sample.Turns) > maxMemoryPreviewTurns {
					t.Fatalf("%s template %q has an unusable sample", locale, template.ID)
				}
				for _, turn := range sample.Turns {
					if turn.User == "" {
						t.Fatalf("%s template %q sample %q has a turn without user text", locale, template.ID, sample.Name)
					}
				}
			}
		}
	}
	if MemoryInstructionsTemplates("zh-Hans").Templates[0].Instructions == MemoryInstructionsTemplates("en-US").Templates[0].Instructions {
		t.Fatal("locales are not applied")
	}
}
