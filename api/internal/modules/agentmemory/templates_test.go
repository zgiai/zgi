package agentmemory

import (
	"testing"
	"unicode/utf8"
)

func TestTemplateCatalogFitsTheBlockRules(t *testing.T) {
	for _, locale := range []string{"zh-Hans", "en-US"} {
		catalog := Templates(locale)
		if catalog.Version != TemplateCatalogVersion || len(catalog.Templates) == 0 {
			t.Fatalf("%s catalog = %+v", locale, catalog)
		}
		ids := map[string]bool{}
		for _, template := range catalog.Templates {
			if ids[template.ID] || template.Name == "" || template.Description == "" || template.Instructions == "" {
				t.Fatalf("%s template %q is incomplete or duplicated", locale, template.ID)
			}
			ids[template.ID] = true
			if utf8.RuneCountInString(template.Instructions) > 2000 || len(template.Samples) == 0 {
				t.Fatalf("%s template %q breaks a limit", locale, template.ID)
			}
			for _, sample := range template.Samples {
				if sample.Name == "" || len(sample.Turns) == 0 {
					t.Fatalf("%s template %q has an empty sample", locale, template.ID)
				}
				for _, turn := range sample.Turns {
					if turn.User == "" {
						t.Fatalf("%s template %q sample %q has a turn without user text", locale, template.ID, sample.Name)
					}
				}
			}
		}
	}
	if Templates("zh-Hans").Templates[0].Name == Templates("en-US").Templates[0].Name {
		t.Fatal("locales are not applied")
	}
}
