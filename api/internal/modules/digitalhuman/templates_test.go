package digitalhuman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Every built-in template is complete in both languages, starts with the
// shared safety rules, and names only skills that exist (ADR 0022).
func TestBuiltinTemplatesAreCompleteInBothLanguages(t *testing.T) {
	for _, locale := range []string{"zh-Hans", "en-US"} {
		catalog := BuiltinTemplates(locale)
		if len(catalog.SharedRules) != len(sharedTemplateRules) || len(catalog.Templates) != len(builtinTemplateSources) {
			t.Fatalf("%s catalog = %d shared rules, %d templates", locale, len(catalog.SharedRules), len(catalog.Templates))
		}
		seen := map[string]bool{}
		for _, template := range catalog.Templates {
			if seen[template.ID] || template.Source != TemplateSourceBuiltin {
				t.Fatalf("%s template %q duplicated or not built in", locale, template.ID)
			}
			seen[template.ID] = true
			for _, text := range []string{template.Title, template.Description, template.Responsibility, template.Persona} {
				if strings.TrimSpace(text) == "" {
					t.Fatalf("%s template %q has empty text", locale, template.ID)
				}
			}
			if utf8.RuneCountInString(template.Persona) > maxPersonaRunes || len(template.Goals) == 0 || len(template.Goals) > maxGoals || len(template.Rules) > maxRules {
				t.Fatalf("%s template %q exceeds DigitalHuman limits", locale, template.ID)
			}
			for index, rule := range catalog.SharedRules {
				if template.Rules[index] != rule {
					t.Fatalf("%s template %q does not start with the shared rules", locale, template.ID)
				}
			}
			if utf8.RuneCountInString(template.MemoryInstructions) > maxMemoryInstructionsRunes {
				t.Fatalf("%s template %q memory instructions too long", locale, template.ID)
			}
			for _, skill := range template.Skills {
				if _, err := os.Stat(filepath.Join("..", "skills", "catalog", skill.ID, "SKILL.md")); err != nil {
					t.Fatalf("%s template %q names unknown skill %q", locale, template.ID, skill.ID)
				}
			}
		}
	}
	chinese, english := BuiltinTemplates("zh-Hans"), BuiltinTemplates("en")
	if chinese.Templates[0].Persona == english.Templates[0].Persona {
		t.Fatal("locales must get their own text")
	}
}

// Each kind of work with memory instructions has a template that carries
// them, and the templates that need something say so.
func TestBuiltinTemplatesCarryMemoryInstructionsAndNeeds(t *testing.T) {
	catalog := BuiltinTemplates("zh-Hans")
	byID := map[string]Template{}
	for _, template := range catalog.Templates {
		byID[template.ID] = template
	}
	for _, source := range memoryTemplateSources {
		template, ok := byID[source.id]
		if !ok || template.MemoryInstructions != source.instructions.zh {
			t.Fatalf("memory template %q has no template carrying its instructions", source.id)
		}
	}
	if file := byID["file_generation"]; len(file.Skills) != 1 || file.Skills[0].ID != "file-generator" || !file.Skills[0].Required {
		t.Fatalf("file template skills = %#v", file.Skills)
	}
	ops := byID["ops_specialist"]
	if len(ops.Connectors) != 1 || ops.Connectors[0].ConnectorID != "serverops_ssh" || !ops.Connectors[0].Required ||
		len(ops.Connectors[0].RequiredOperationIDs) != 3 || len(ops.Connectors[0].OptionalOperationIDs) != 4 {
		t.Fatalf("operations template connectors = %#v", ops.Connectors)
	}
	if len(ops.Skills) != 1 || ops.Skills[0].ID != "ops-runbook" || !ops.Skills[0].Required || ops.MemoryInstructions == "" {
		t.Fatalf("operations template = %#v", ops)
	}
}

// A saved template keeps its own title apart from the name in conversation,
// and its skills are suggestions.
func TestSavedTemplateInTheCatalog(t *testing.T) {
	item := &PersonaTemplate{
		ID: uuid.New(), Name: "客服模板 v2", IdentityName: "小客", Description: "回答客户问题",
		Persona: "你是客服。", Goals: datatypes.JSON(`["解答问题"]`), Rules: datatypes.JSON(`["不泄露隐私"]`),
		MemoryInstructions: "记住客户套餐。", SkillIDs: datatypes.JSON(`["ticket-routing"]`), Revision: 3,
	}
	template, err := savedTemplate(item)
	if err != nil {
		t.Fatal(err)
	}
	if template.Source != TemplateSourceWorkspace || template.Title != "客服模板 v2" || template.IdentityName != "小客" ||
		template.Responsibility != "回答客户问题" || template.MemoryInstructions != "记住客户套餐。" || template.Revision != 3 ||
		len(template.Skills) != 1 || template.Skills[0].Required || template.Skills[0].ID != "ticket-routing" {
		t.Fatalf("saved template = %#v", template)
	}
	if validatePersonaTemplateWork("", "", strings.Repeat("长", maxMemoryInstructionsRunes+1), nil) == nil {
		t.Fatal("over-long memory instructions must be rejected")
	}
	if validatePersonaTemplateWork("", "", "", []string{""}) == nil {
		t.Fatal("an empty skill ID must be rejected")
	}
}
