package skills

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin/calculator"
)

func TestBuildNativeSkillCatalogUsesTopFiveWhenManySkillsAreAvailable(t *testing.T) {
	resolved := &ResolvedSkills{}
	for index := 0; index < 20; index++ {
		resolved.Skills = append(resolved.Skills, SkillDocument{
			Metadata: SkillMetadata{
				ID:          fmt.Sprintf("skill-%02d", index),
				Name:        fmt.Sprintf("Skill %02d", index),
				Description: strings.Repeat("compact metadata ", 20),
				WhenToUse:   "Use for the matching test task.",
			},
			Instructions: "Full instructions must not appear in the candidate catalog.",
		})
	}

	catalog := BuildNativeSkillCatalog(resolved, nil, DefaultNativeSkillCatalogBudgetChars, 1000, func(message llmadapter.Message) int {
		return len([]rune(stringFromNativeMessage(message.Content))) / 4
	})
	if len(catalog.CandidateSkillIDs) != 20 {
		t.Fatalf("candidate skills = %d, want 20", len(catalog.CandidateSkillIDs))
	}
	if len(catalog.ExposedSkillIDs) != DefaultNativeSkillSearchLimit {
		t.Fatalf("exposed skills = %#v, want top %d", catalog.ExposedSkillIDs, DefaultNativeSkillSearchLimit)
	}
	if catalog.OmittedCount != 15 {
		t.Fatalf("omitted count = %d, want 15", catalog.OmittedCount)
	}
	content := stringFromNativeMessage(catalog.Message.Content)
	if strings.Contains(content, "Full instructions must not appear") {
		t.Fatalf("candidate catalog leaked full skill instructions: %s", content)
	}
	if len([]rune(content)) > DefaultNativeSkillCatalogBudgetChars {
		t.Fatalf("candidate catalog chars = %d, want <= %d", len([]rune(content)), DefaultNativeSkillCatalogBudgetChars)
	}
	if catalog.MetadataTokens > 1000 {
		t.Fatalf("candidate catalog tokens = %d, want <= 1000", catalog.MetadataTokens)
	}
}

func TestNativeSkillSessionSearchFindsOmittedSkill(t *testing.T) {
	resolved := &ResolvedSkills{}
	for index := 0; index < 12; index++ {
		description := "General helper"
		whenToUse := "Use for general work."
		if index == 11 {
			description = "Generate a special Word teaching analysis report"
			whenToUse = "Use for special report generation."
		}
		resolved.Skills = append(resolved.Skills, SkillDocument{Metadata: SkillMetadata{
			ID:          fmt.Sprintf("skill-%02d", index),
			Description: description,
			WhenToUse:   whenToUse,
		}})
	}
	catalog := BuildNativeSkillCatalog(resolved, nil, DefaultNativeSkillCatalogBudgetChars, 0, nil)
	session := NewNativeSkillSession(NewRuntime(nil, nil), resolved, catalog, NativeToolSetOptions{BudgetChars: 10000})
	initiallyExposed := append([]string(nil), session.ExposedSkillIDs()...)

	matches := session.Search("special Word teaching report", 5)
	if len(matches) == 0 || matches[0].ID != "skill-11" {
		t.Fatalf("search matches = %#v, want omitted skill-11 first", matches)
	}
	if !containsNativeTestID(session.ExposedSkillIDs(), "skill-11") {
		t.Fatalf("exposed skills = %#v, want searched skill-11", session.ExposedSkillIDs())
	}
	for _, match := range matches {
		if containsNativeTestID(initiallyExposed, match.ID) {
			t.Fatalf("search returned already exposed candidate %q: %#v", match.ID, matches)
		}
	}
}

func TestNativeSkillSessionActivatesCompleteSkillsIncrementally(t *testing.T) {
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(calculator.NewProvider()); err != nil {
		t.Fatalf("register calculator: %v", err)
	}
	runtime := NewRuntime(tools.NewToolEngine(manager), manager)
	resolved := &ResolvedSkills{Skills: []SkillDocument{
		{
			Metadata:     SkillMetadata{ID: SkillCalculator, Description: "Exact arithmetic"},
			Instructions: "Use exact arithmetic.",
			Tools: []SkillToolDefinition{{
				Name:         "calculate",
				ProviderType: tools.ToolProviderTypeBuiltin,
				ProviderID:   "calculator",
			}},
		},
		{
			Metadata:     SkillMetadata{ID: "prompt-only", Description: "Tone guidance", RuntimeType: SkillRuntimeTypePrompt},
			Instructions: "Use a concise professional tone.",
		},
	}}
	catalog := BuildNativeSkillCatalog(resolved, nil, DefaultNativeSkillCatalogBudgetChars, 0, nil)
	session := NewNativeSkillSession(runtime, resolved, catalog, NativeToolSetOptions{BudgetChars: 10000})
	if current := session.ToolSet(); len(current.ActiveSkillIDs) != 0 || len(current.ProviderTools) != 0 {
		t.Fatalf("initial tool set = %#v, want no active skills", current)
	}

	calculatorResult := session.Activate(context.Background(), []string{SkillCalculator}, "runtime_activation")
	if !reflect.DeepEqual(calculatorResult.ActivatedSkillIDs, []string{SkillCalculator}) {
		t.Fatalf("calculator activation = %#v", calculatorResult)
	}
	current := session.ToolSet()
	if len(current.InstructionMessages) != 1 || len(current.ProviderTools) != 1 || current.ProviderTools[0].Function.Name != "calculate" {
		t.Fatalf("calculator tool set = %#v, want complete instruction and calculate schema", current)
	}
	candidateMessage := stringFromNativeMessage(session.CatalogMessage().Content)
	if strings.Contains(candidateMessage, `"skill_id":"calculator"`) || !strings.Contains(candidateMessage, `"skill_id":"prompt-only"`) {
		t.Fatalf("candidate message = %s, want inactive prompt-only without active calculator", candidateMessage)
	}

	promptResult := session.Activate(context.Background(), []string{"prompt-only"}, "runtime_activation")
	if !reflect.DeepEqual(promptResult.ActivatedSkillIDs, []string{"prompt-only"}) {
		t.Fatalf("prompt-only activation = %#v", promptResult)
	}
	current = session.ToolSet()
	if len(current.InstructionMessages) != 2 || len(current.ProviderTools) != 1 {
		t.Fatalf("prompt-only tool set = %#v, want second instruction without another tool", current)
	}

	repeated := session.Activate(context.Background(), []string{SkillCalculator}, "runtime_activation")
	if !reflect.DeepEqual(repeated.AlreadyActiveSkillIDs, []string{SkillCalculator}) || len(repeated.ActivatedSkillIDs) != 0 {
		t.Fatalf("repeated activation = %#v, want already active", repeated)
	}
}

func containsNativeTestID(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
