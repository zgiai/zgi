package skills_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	calculatorpkg "github.com/zgiai/zgi/api/internal/modules/tools/builtin/calculator"
	filegeneratorpkg "github.com/zgiai/zgi/api/internal/modules/tools/builtin/filegenerator"
	intentrouterpkg "github.com/zgiai/zgi/api/internal/modules/tools/builtin/intentrouter"
	timepkg "github.com/zgiai/zgi/api/internal/modules/tools/builtin/time"
)

func TestRuntime_ResolveEnabledSkills_LoadsCatalogMetadata(t *testing.T) {
	runtime := newSkillRuntime(t, "time")

	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{"time"})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	if len(resolved.Skills) != 1 {
		t.Fatalf("skills = %d, want 1", len(resolved.Skills))
	}
	got := resolved.Skills[0]
	if got.Metadata.ID != "time" || got.Metadata.Name == "" || got.Instructions == "" {
		t.Fatalf("resolved skill = %#v", got)
	}
	if !containsString(got.Metadata.Tools, "current_time") || !containsString(got.Metadata.Tools, "date_calculate") {
		t.Fatalf("tools = %v, want current_time and date_calculate", got.Metadata.Tools)
	}
	message := skills.SkillMetadataSystemMessage(resolved.PromptMetadata())
	content := message.Content.(string)
	if strings.Contains(content, "current_time") || strings.Contains(content, "date_calculate") {
		t.Fatalf("prompt metadata content = %q, did not want business tool names", content)
	}
}

func TestSkillMetadataSystemMessageWithBudget_TruncatesLongFields(t *testing.T) {
	longDescription := strings.Repeat("description ", 120)
	message, stats := skills.SkillMetadataSystemMessageWithBudget([]skills.SkillPromptMetadata{
		{
			ID:          "long-skill",
			Source:      skills.SkillSourceCustom,
			Name:        "Long Skill",
			Description: longDescription,
			WhenToUse:   strings.Repeat("when ", 120),
			RuntimeType: skills.SkillRuntimeTypePrompt,
		},
	}, 1200)

	content := message.Content.(string)
	if !stats.Truncated || stats.ExposedCount != 1 || stats.OmittedCount != 0 {
		t.Fatalf("stats = %#v, want one exposed truncated skill", stats)
	}
	if strings.Contains(content, longDescription) {
		t.Fatal("metadata prompt contains the full long description, want truncated content")
	}
	if !strings.Contains(content, "long-skill") {
		t.Fatal("metadata prompt omitted the skill id, want it preserved")
	}
}

func TestSkillMetadataSystemMessageWithBudget_OmitsSkillsOverBudget(t *testing.T) {
	metadata := make([]skills.SkillPromptMetadata, 0, 20)
	for i := range 20 {
		metadata = append(metadata, skills.SkillPromptMetadata{
			ID:          fmt.Sprintf("skill-%02d", i),
			Source:      skills.SkillSourceCustom,
			Name:        fmt.Sprintf("Skill %02d", i),
			Description: strings.Repeat("description ", 20),
			WhenToUse:   strings.Repeat("when ", 20),
			RuntimeType: skills.SkillRuntimeTypePrompt,
		})
	}

	message, stats := skills.SkillMetadataSystemMessageWithBudget(metadata, 900)
	content := message.Content.(string)
	if stats.EnabledCount != len(metadata) || stats.ExposedCount == 0 || stats.ExposedCount >= len(metadata) || stats.OmittedCount == 0 {
		t.Fatalf("stats = %#v, want partial exposure with omitted skills", stats)
	}
	if !stats.Truncated {
		t.Fatalf("stats.Truncated = false, want true when skills are omitted")
	}
	if strings.Contains(content, "skill-19") {
		t.Fatal("metadata prompt contains omitted skill-19")
	}
}

func TestRuntime_ResolveEnabledSkills_NotFound_ReturnsError(t *testing.T) {
	runtime := newSkillRuntime(t)

	_, err := runtime.ResolveEnabledSkills(context.Background(), []string{"missing"})
	if err == nil || !strings.Contains(err.Error(), "skill missing not found") {
		t.Fatalf("ResolveEnabledSkills() error = %v, want not found", err)
	}
}

func TestRuntime_ListSkills_ReturnsSortedLightweightMetadata(t *testing.T) {
	catalogDir := t.TempDir()
	writeTimeSkill(t, catalogDir)
	writeCalculatorSkill(t, catalogDir)
	runtime := newSkillRuntimeFromCatalog(t, catalogDir)

	metadata, err := runtime.ListSkills(context.Background())
	if err != nil {
		t.Fatalf("ListSkills() error = %v", err)
	}
	if len(metadata) != 2 {
		t.Fatalf("metadata = %d, want 2", len(metadata))
	}
	if metadata[0].ID != "calculator" || metadata[1].ID != "time" {
		t.Fatalf("skill order = %s,%s; want calculator,time", metadata[0].ID, metadata[1].ID)
	}
	if !metadata[0].HasTools || metadata[0].HasReferences || metadata[0].HasScripts || metadata[0].ScriptsSupported {
		t.Fatalf("calculator metadata = %#v", metadata[0])
	}
	if metadata[0].RuntimeType != skills.SkillRuntimeTypeTool {
		t.Fatalf("calculator runtime_type = %q, want tool", metadata[0].RuntimeType)
	}
	if metadata[0].Display.Icon != "calculator" || metadata[0].Display.Label["en_US"] != "Calculator" || metadata[0].Display.Label["zh_Hans"] != "Calculator Localized" {
		t.Fatalf("calculator display = %#v, want display metadata", metadata[0].Display)
	}
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{"calculator"})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	message := skills.SkillMetadataSystemMessage(resolved.PromptMetadata())
	content := message.Content.(string)
	if strings.Contains(content, "evaluate_expression") || strings.Contains(content, "calculate") || strings.Contains(content, "percentage") {
		t.Fatalf("prompt metadata content = %q, did not want business tool names", content)
	}
	if strings.Contains(content, "productivity") || strings.Contains(content, "Math") {
		t.Fatalf("prompt metadata content = %q, did not want display metadata", content)
	}
}

func TestRuntime_ResolveEnabledSkills_AcceptsCRLFSkillMarkdown(t *testing.T) {
	catalogDir := t.TempDir()
	writeSkillMarkdown(t, catalogDir, "calculator", strings.ReplaceAll(testCalculatorSkillMarkdown(), "\n", "\r\n"))
	runtime := newSkillRuntimeFromCatalog(t, catalogDir)

	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{"calculator"})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	if len(resolved.Skills) != 1 {
		t.Fatalf("skills = %d, want 1", len(resolved.Skills))
	}
	if resolved.Skills[0].Metadata.ID != "calculator" {
		t.Fatalf("skill id = %q, want calculator", resolved.Skills[0].Metadata.ID)
	}
}

func TestRuntime_ResolveEnabledSkills_AcceptsBOMAndCRSkillMarkdown(t *testing.T) {
	catalogDir := t.TempDir()
	markdown := "\ufeff" + strings.ReplaceAll(testCalculatorSkillMarkdown(), "\n", "\r")
	writeSkillMarkdown(t, catalogDir, "calculator", markdown)
	runtime := newSkillRuntimeFromCatalog(t, catalogDir)

	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{"calculator"})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	if len(resolved.Skills) != 1 || resolved.Skills[0].Metadata.ID != "calculator" {
		t.Fatalf("resolved skills = %#v, want calculator", resolved.Skills)
	}
}

func TestRuntime_GetSkillMetadata_ReturnsLightweightMetadata(t *testing.T) {
	catalogDir := t.TempDir()
	writeCalculatorSkill(t, catalogDir)
	runtime := newSkillRuntimeFromCatalog(t, catalogDir)

	metadata, err := runtime.GetSkillMetadata(context.Background(), "calculator")
	if err != nil {
		t.Fatalf("GetSkillMetadata() error = %v", err)
	}
	if metadata.ID != "calculator" || !metadata.HasTools {
		t.Fatalf("metadata = %#v", metadata)
	}
	if metadata.RuntimeType != skills.SkillRuntimeTypeTool {
		t.Fatalf("runtime_type = %q, want tool", metadata.RuntimeType)
	}
	if metadata.Display.Icon != "calculator" || metadata.Display.Label["en_US"] != "Calculator" || metadata.Display.Label["zh_Hans"] != "Calculator Localized" {
		t.Fatalf("display = %#v, want calculator display metadata", metadata.Display)
	}
	payload := fmt.Sprintf("%#v", metadata)
	if strings.Contains(payload, "SKILL.md") || strings.Contains(payload, "evaluate_expression") || strings.Contains(payload, "calculate") || strings.Contains(payload, "percentage") {
		t.Fatalf("metadata payload = %q, did not want skill body or business tool schema", payload)
	}
}

func TestRuntime_GetSkillMetadata_UsesDisplayFallback(t *testing.T) {
	runtime := newSkillRuntime(t, "time")

	metadata, err := runtime.GetSkillMetadata(context.Background(), "time")
	if err != nil {
		t.Fatalf("GetSkillMetadata() error = %v", err)
	}
	if metadata.Display.Icon != "sparkles" {
		t.Fatalf("display icon = %q, want fallback", metadata.Display.Icon)
	}
	if metadata.Display.Label["en_US"] != "time" {
		t.Fatalf("display label = %#v, want name fallback", metadata.Display.Label)
	}
	if metadata.Display.Description["en_US"] != "Answer time questions." {
		t.Fatalf("display description = %#v, want description fallback", metadata.Display.Description)
	}
}

func TestRuntime_ListSkills_InfersPromptRuntimeWithoutTools(t *testing.T) {
	catalogDir := t.TempDir()
	writePromptOnlySkill(t, catalogDir)
	runtime := newSkillRuntimeFromCatalog(t, catalogDir)

	metadata, err := runtime.ListSkills(context.Background())
	if err != nil {
		t.Fatalf("ListSkills() error = %v", err)
	}
	if len(metadata) != 1 {
		t.Fatalf("metadata = %d, want 1", len(metadata))
	}
	if metadata[0].ID != "style-guide" || metadata[0].RuntimeType != skills.SkillRuntimeTypePrompt || metadata[0].HasTools {
		t.Fatalf("prompt skill metadata = %#v", metadata[0])
	}
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{"style-guide"})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	if len(resolved.Skills) != 1 || len(resolved.Skills[0].Tools) != 0 {
		t.Fatalf("resolved prompt skill = %#v", resolved)
	}
}

func TestRuntime_ListSkillsWithCustom_MergesSystemAndCustomSkills(t *testing.T) {
	catalogDir := t.TempDir()
	writeTimeSkill(t, catalogDir)
	customDir := t.TempDir()
	writeCustomPromptSkill(t, customDir)
	runtime := newSkillRuntimeFromCatalog(t, catalogDir)

	metadata, err := runtime.ListSkillsWithCustom(context.Background(), []skills.CustomSkillCatalogEntry{{
		SkillID: "brief-writer",
		Root:    customDir,
	}})
	if err != nil {
		t.Fatalf("ListSkillsWithCustom() error = %v", err)
	}
	if len(metadata) != 2 {
		t.Fatalf("metadata = %d, want 2", len(metadata))
	}
	if metadata[0].ID != "brief-writer" || metadata[0].Source != skills.SkillSourceCustom {
		t.Fatalf("custom metadata = %#v", metadata[0])
	}
	if metadata[0].RuntimeType != skills.SkillRuntimeTypePrompt || metadata[0].HasTools {
		t.Fatalf("custom runtime metadata = %#v", metadata[0])
	}
	if !metadata[0].HasReferences || !metadata[0].HasScripts || metadata[0].ScriptsSupported {
		t.Fatalf("custom capability metadata = %#v", metadata[0])
	}
	if metadata[1].ID != "time" || metadata[1].Source != skills.SkillSourceSystem {
		t.Fatalf("system metadata = %#v", metadata[1])
	}
}

func TestRuntime_ListSystemSkillsBestEffort_SkipsBrokenSkill(t *testing.T) {
	catalogDir := t.TempDir()
	writeTimeSkill(t, catalogDir)
	writeSkillMarkdown(t, catalogDir, "broken", `---
name: broken
description: Broken system skill.
provider_type: builtin
provider_id: missing
tools:
  - missing_tool
---

# Broken
`)
	runtime := newSkillRuntimeFromCatalog(t, catalogDir)

	metadata, err := runtime.ListSystemSkillsBestEffort(context.Background())
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("ListSystemSkillsBestEffort() error = %v, want broken skill error", err)
	}
	if len(metadata) != 1 || metadata[0].ID != "time" {
		t.Fatalf("metadata = %#v, want only time", metadata)
	}
}

func TestRuntime_SystemSkillExists_DoesNotParseSkillMarkdown(t *testing.T) {
	catalogDir := t.TempDir()
	writeSkillMarkdown(t, catalogDir, "broken", `---
name: broken
description: Broken system skill.
provider_type: builtin
provider_id: missing
tools:
  - missing_tool
---

# Broken
`)
	runtime := newSkillRuntimeFromCatalog(t, catalogDir)

	if !runtime.SystemSkillExists("broken") {
		t.Fatal("SystemSkillExists() = false, want true for existing broken skill directory")
	}
	if runtime.SystemSkillExists("missing") {
		t.Fatal("SystemSkillExists(missing) = true, want false")
	}
}

func TestRuntime_ReadReference_AllowsCustomRootAndReferencesMarkdown(t *testing.T) {
	catalogDir := t.TempDir()
	writeTimeSkill(t, catalogDir)
	customDir := t.TempDir()
	writeCustomPromptSkill(t, customDir)
	runtime := newSkillRuntimeFromCatalog(t, catalogDir)
	custom := []skills.CustomSkillCatalogEntry{{SkillID: "brief-writer", Root: customDir}}
	resolved, err := runtime.ResolveEnabledSkillsWithCustom(context.Background(), []string{"brief-writer"}, custom)
	if err != nil {
		t.Fatalf("ResolveEnabledSkillsWithCustom() error = %v", err)
	}

	rootContent, _, err := runtime.ReadReference(context.Background(), resolved, "brief-writer", "style.md")
	if err != nil {
		t.Fatalf("ReadReference(root) error = %v", err)
	}
	if !strings.Contains(rootContent, "root reference") {
		t.Fatalf("root reference content = %q", rootContent)
	}
	nestedContent, _, err := runtime.ReadReference(context.Background(), resolved, "brief-writer", "references/details.md")
	if err != nil {
		t.Fatalf("ReadReference(nested) error = %v", err)
	}
	if !strings.Contains(nestedContent, "nested reference") {
		t.Fatalf("nested reference content = %q", nestedContent)
	}
}

func TestRuntime_LoadCustomSkillDocument_RejectsTools(t *testing.T) {
	root := t.TempDir()
	markdown := `---
name: custom-tool
description: Invalid custom tool skill.
tools:
  - calculate
---

# Invalid
`
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(markdown), 0o644); err != nil {
		t.Fatalf("write custom skill: %v", err)
	}

	_, err := skills.LoadCustomSkillDocument(root)
	if err == nil || !strings.Contains(err.Error(), "must use prompt runtime_type") {
		t.Fatalf("LoadCustomSkillDocument() error = %v, want prompt-only error", err)
	}
}

func TestRuntime_ReadReference_RejectsPathTraversal(t *testing.T) {
	runtime := newSkillRuntime(t, "time", "references/guide.md")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{"time"})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}

	_, _, err = runtime.ReadReference(context.Background(), resolved, "time", "../SKILL.md")
	if err == nil || !strings.Contains(err.Error(), "invalid skill reference path") {
		t.Fatalf("ReadReference() error = %v, want invalid path", err)
	}
}

func TestRuntime_ResolveEnabledSkills_DetectsScriptsWithoutExecution(t *testing.T) {
	runtime := newSkillRuntime(t, "time", "scripts/run.py")

	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{"time"})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	metadata := resolved.Skills[0].Metadata
	if !metadata.HasScripts {
		t.Fatalf("HasScripts = false, want true")
	}
	if metadata.ScriptsSupported {
		t.Fatalf("ScriptsSupported = true, want false")
	}
}

func TestRuntime_CallSkillTool_RequiresAllowedTool(t *testing.T) {
	runtime := newSkillRuntime(t, "time")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{"time"})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}

	_, err = runtime.CallSkillTool(context.Background(), resolved, "time", "not_allowed", nil, skills.ExecutionContext{
		OrganizationID: "organization-1",
		UserID:         "user-1",
	}, "call_1")
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("CallSkillTool() error = %v, want not available", err)
	}
}

func TestRuntime_ValidateCatalog_AcceptsTimeSkill(t *testing.T) {
	runtime := newSkillRuntime(t, "time")

	if err := runtime.ValidateCatalog(context.Background()); err != nil {
		t.Fatalf("ValidateCatalog() error = %v", err)
	}
}

func TestRuntime_ValidateCatalog_AcceptsCalculatorSkill(t *testing.T) {
	catalogDir := t.TempDir()
	writeCalculatorSkill(t, catalogDir)
	runtime := newSkillRuntimeFromCatalog(t, catalogDir)

	if err := runtime.ValidateCatalog(context.Background()); err != nil {
		t.Fatalf("ValidateCatalog() error = %v", err)
	}
}

func TestRuntime_ValidateCatalog_AcceptsFileGeneratorSkill(t *testing.T) {
	catalogDir := t.TempDir()
	writeFileGeneratorSkill(t, catalogDir)
	runtime := newSkillRuntimeFromCatalog(t, catalogDir)

	if err := runtime.ValidateCatalog(context.Background()); err != nil {
		t.Fatalf("ValidateCatalog() error = %v", err)
	}
}

func TestRuntime_ValidateCatalog_AcceptsIntentRouterSkill(t *testing.T) {
	catalogDir := t.TempDir()
	writeIntentRouterSkill(t, catalogDir)
	runtime := newSkillRuntimeFromCatalog(t, catalogDir)

	if err := runtime.ValidateCatalog(context.Background()); err != nil {
		t.Fatalf("ValidateCatalog() error = %v", err)
	}
}

func TestRuntime_ValidateCatalog_RejectsMissingTool(t *testing.T) {
	catalogDir := t.TempDir()
	timeDir := filepath.Join(catalogDir, "time")
	if err := os.MkdirAll(timeDir, 0o755); err != nil {
		t.Fatalf("mkdir time skill: %v", err)
	}
	markdown := strings.Replace(testSkillMarkdown(), "  - date_calculate", "  - missing_tool", 1)
	if err := os.WriteFile(filepath.Join(timeDir, "SKILL.md"), []byte(markdown), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(timepkg.NewTimeProvider()); err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)

	err := runtime.ValidateCatalog(context.Background())
	if err == nil || !strings.Contains(err.Error(), "missing_tool") {
		t.Fatalf("ValidateCatalog() error = %v, want missing tool", err)
	}
}

func TestRuntime_ValidateCatalog_RejectsInvalidSkillDirectoryName(t *testing.T) {
	catalogDir := t.TempDir()
	skillDir := filepath.Join(catalogDir, "file_generator")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir invalid skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(testFileGeneratorSkillMarkdown()), 0o644); err != nil {
		t.Fatalf("write invalid skill: %v", err)
	}
	runtime := newSkillRuntimeFromCatalog(t, catalogDir)

	err := runtime.ValidateCatalog(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid skill directory file_generator") {
		t.Fatalf("ValidateCatalog() error = %v, want invalid directory", err)
	}
}

func TestRuntime_ValidateCatalog_RejectsInvalidSkillName(t *testing.T) {
	catalogDir := t.TempDir()
	skillDir := filepath.Join(catalogDir, "file-generator")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir invalid skill: %v", err)
	}
	markdown := strings.Replace(testFileGeneratorSkillMarkdown(), "name: file-generator", "name: file_generator", 1)
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(markdown), 0o644); err != nil {
		t.Fatalf("write invalid skill: %v", err)
	}
	runtime := newSkillRuntimeFromCatalog(t, catalogDir)

	err := runtime.ValidateCatalog(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid skill name file_generator") {
		t.Fatalf("ValidateCatalog() error = %v, want invalid skill name", err)
	}
}

func newSkillRuntime(t *testing.T, extraPaths ...string) *skills.Runtime {
	t.Helper()
	catalogDir := t.TempDir()
	writeTimeSkill(t, catalogDir)
	timeDir := filepath.Join(catalogDir, "time")
	for _, extraPath := range extraPaths {
		full := filepath.Join(timeDir, extraPath)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir extra path: %v", err)
		}
		if err := os.WriteFile(full, []byte("test"), 0o644); err != nil {
			t.Fatalf("write extra path: %v", err)
		}
	}
	return newSkillRuntimeFromCatalog(t, catalogDir)
}

func newSkillRuntimeFromCatalog(t *testing.T, catalogDir string) *skills.Runtime {
	t.Helper()
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(timepkg.NewTimeProvider()); err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}
	if err := manager.RegisterProvider(calculatorpkg.NewProvider()); err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}
	if err := manager.RegisterProvider(filegeneratorpkg.NewProvider()); err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}
	if err := manager.RegisterProvider(intentrouterpkg.NewProvider()); err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}
	return skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
}

func writeTimeSkill(t *testing.T, catalogDir string) {
	t.Helper()
	timeDir := filepath.Join(catalogDir, "time")
	if err := os.MkdirAll(timeDir, 0o755); err != nil {
		t.Fatalf("mkdir time skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(timeDir, "SKILL.md"), []byte(testSkillMarkdown()), 0o644); err != nil {
		t.Fatalf("write time skill: %v", err)
	}
}

func writeCalculatorSkill(t *testing.T, catalogDir string) {
	t.Helper()
	writeSkillMarkdown(t, catalogDir, "calculator", testCalculatorSkillMarkdown())
}

func writeFileGeneratorSkill(t *testing.T, catalogDir string) {
	t.Helper()
	skillDir := filepath.Join(catalogDir, "file-generator")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir file generator skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(testFileGeneratorSkillMarkdown()), 0o644); err != nil {
		t.Fatalf("write file generator skill: %v", err)
	}
}

func writeIntentRouterSkill(t *testing.T, catalogDir string) {
	t.Helper()
	writeSkillMarkdown(t, catalogDir, "intent-router", testIntentRouterSkillMarkdown())
}

func writeSkillMarkdown(t *testing.T, catalogDir string, skillID string, markdown string) {
	t.Helper()
	skillDir := filepath.Join(catalogDir, skillID)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir %s skill: %v", skillID, err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(markdown), 0o644); err != nil {
		t.Fatalf("write %s skill: %v", skillID, err)
	}
}

func writePromptOnlySkill(t *testing.T, catalogDir string) {
	t.Helper()
	skillDir := filepath.Join(catalogDir, "style-guide")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir prompt skill: %v", err)
	}
	markdown := `---
name: style-guide
description: Provide concise answer style guidance.
when_to_use: Use when the answer should follow a specific writing style.
max_calls_per_turn: 1
timeout_seconds: 5
---

# Style Guide Skill

Answer with concise wording.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(markdown), 0o644); err != nil {
		t.Fatalf("write prompt skill: %v", err)
	}
}

func writeCustomPromptSkill(t *testing.T, root string) {
	t.Helper()
	markdown := `---
name: brief-writer
description: Help draft short writing briefs.
display:
  icon: file-text
---

# Brief Writer

Use the references before drafting a brief.
`
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(markdown), 0o644); err != nil {
		t.Fatalf("write custom skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "style.md"), []byte("root reference"), 0o644); err != nil {
		t.Fatalf("write root reference: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "references"), 0o755); err != nil {
		t.Fatalf("mkdir references: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "references", "details.md"), []byte("nested reference"), 0o644); err != nil {
		t.Fatalf("write nested reference: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "run.py"), []byte("print('skip')"), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
}

func testSkillMarkdown() string {
	return `---
name: time
description: Answer time questions.
when_to_use: Use for current time and date calculations.
provider_type: builtin
provider_id: time
tools:
  - current_time
  - date_calculate
max_calls_per_turn: 3
timeout_seconds: 5
---

# Time Skill

Always load this skill before calling time tools.
`
}

func testCalculatorSkillMarkdown() string {
	return `---
name: calculator
description: Perform deterministic arithmetic and percent-based math.
when_to_use: Use for exact arithmetic and percents.
provider_type: builtin
provider_id: calculator
tools:
  - evaluate_expression
  - calculate
  - percentage
max_calls_per_turn: 3
timeout_seconds: 5
display:
  icon: calculator
  category: productivity
  label:
    en_US: Calculator
    zh_Hans: Calculator Localized
  description:
    en_US: Exact arithmetic.
  when_to_use:
    en_US: Use for exact arithmetic.
  tags:
    en_US:
      - Math
---

# Calculator Skill

Always load this skill before calling calculator tools.
`
}

func testFileGeneratorSkillMarkdown() string {
	return `---
name: file-generator
description: Generate downloadable text-based files.
when_to_use: Use for creating txt, markdown, html, json, or csv files.
provider_type: builtin
provider_id: file_generator
tools:
  - generate_file
  - generate_docx
  - generate_pdf
  - generate_pptx
max_calls_per_turn: 3
timeout_seconds: 5
---

# File Generator Skill

Always load this skill before generating files.
`
}

func testIntentRouterSkillMarkdown() string {
	return `---
name: intent-router
description: Classify user intent and route tasks.
when_to_use: Use for intent recognition and task routing before selecting downstream tools.
provider_type: builtin
provider_id: intent_router
runtime_type: hybrid
tools:
  - route_intent
max_calls_per_turn: 3
timeout_seconds: 5
---

# Intent Router Skill

Always load this skill before routing ambiguous user requests.
`
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
