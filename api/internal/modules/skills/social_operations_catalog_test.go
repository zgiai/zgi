package skills_test

import (
	"context"
	"strings"

	skillspkg "github.com/zgiai/zgi/api/internal/modules/skills"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin/socialoperations"
)

const socialOperationsSkillID = "social-operations"

func TestSocialOperationsCatalogLoadsThroughRuntime(t *testing.T) {
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(socialoperations.NewProvider(nil, nil)); err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}
	runtime := skillspkg.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{socialOperationsSkillID})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, trace, err := runtime.LoadSkill(context.Background(), resolved, socialOperationsSkillID)
	if err != nil {
		t.Fatalf("LoadSkill() error = %v", err)
	}
	if trace.Status != "success" {
		t.Fatalf("LoadSkill() trace status = %q, want success", trace.Status)
	}
	if doc.Metadata.RuntimeType != skillspkg.SkillRuntimeTypeTool {
		t.Fatalf("runtime type = %q, want tool", doc.Metadata.RuntimeType)
	}
	wantTools := []string{
		"compose_image", "inspect_image", "list_publications", "prepare_outbound", "review_outbound", "dispatch_outbound", "read_outbound", "capture_metrics", "dispatch_publication", "due_metrics", "import_image", "reconcile_publication", "replace_draft", "list_records", "prepare_publication", "read_artifact",
		"read_publication", "read_record", "review_publication", "save_record", "write_daily_archive",
	}
	if got := toolNames(doc.Tools); !sameStrings(got, wantTools) {
		t.Fatalf("tools = %#v, want %#v", got, wantTools)
	}
	for _, tool := range doc.Tools {
		if tool.ProviderType != "builtin" || tool.ProviderID != "social_operations" {
			t.Fatalf("tool %q provider = %q/%q, want builtin/social_operations", tool.Name, tool.ProviderType, tool.ProviderID)
		}
	}
	toolSet := runtime.BuildNativeToolSet(context.Background(), resolved, skillspkg.NativeToolSetOptions{BudgetChars: 100000})
	if len(toolSet.SkippedSkills) != 0 || len(toolSet.ProviderTools) != len(wantTools) {
		t.Fatalf("native tool projection = %#v, want all social operations tools active", toolSet)
	}
	for _, projected := range toolSet.ProviderTools {
		schema, ok := projected.Function.Parameters.(map[string]interface{})
		if !ok {
			t.Fatalf("projected tool %q parameters = %#v, want object schema", projected.Function.Name, projected.Function.Parameters)
		}
		properties, ok := schema["properties"].(map[string]interface{})
		if !ok || len(properties) != 1 {
			t.Fatalf("projected tool %q properties = %#v, want only input", projected.Function.Name, schema["properties"])
		}
		input, ok := properties["input"].(map[string]interface{})
		if !ok || input["type"] != "string" {
			t.Fatalf("projected tool %q input schema = %#v, want JSON string", projected.Function.Name, properties["input"])
		}
		if required, ok := schema["required"].([]string); !ok || !sameStrings(required, []string{"input"}) {
			t.Fatalf("projected tool %q required = %#v, want [input]", projected.Function.Name, schema["required"])
		}
	}
	if !sameStrings(doc.Metadata.SupportedCallers, []string{skillspkg.SkillCallerDigitalHuman}) {
		t.Fatalf("supported callers = %#v, want digital_human", doc.Metadata.SupportedCallers)
	}
	if doc.Metadata.Display.Label["en_US"] == "" || doc.Metadata.Display.Label["zh_Hans"] == "" {
		t.Fatalf("bilingual labels missing: %#v", doc.Metadata.Display.Label)
	}
	for _, path := range []string{"configuration.md", "execution-contract.md", "ten-step-sop.md"} {
		if !hasReference(doc.Metadata.References, path) {
			t.Fatalf("references = %#v, missing %s", doc.Metadata.References, path)
		}
	}
	if strings.Contains(strings.ToLower(doc.Instructions), "zgi.ai") || strings.Contains(strings.ToLower(doc.Instructions), "zgiai") {
		t.Fatal("reusable skill instructions contain a product-specific website or repository")
	}
}

func TestSocialOperationsReferencesExposeOperationalContract(t *testing.T) {
	runtime := skillspkg.NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{socialOperationsSkillID})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}

	configuration, _, err := runtime.ReadReference(context.Background(), resolved, socialOperationsSkillID, "configuration.md")
	if err != nil {
		t.Fatalf("ReadReference(configuration.md) error = %v", err)
	}
	for _, required := range []string{"product", "timezone", "enabled platforms", "bound accounts", "research providers", "approved brand assets", "escalation destination"} {
		if !strings.Contains(strings.ToLower(configuration), required) {
			t.Fatalf("configuration reference missing %q", required)
		}
	}

	contract, _, err := runtime.ReadReference(context.Background(), resolved, socialOperationsSkillID, "execution-contract.md")
	if err != nil {
		t.Fatalf("ReadReference(execution-contract.md) error = %v", err)
	}
	for _, required := range []string{"missing capability", "outcome_unknown", "never retry", "verified parent", "local archive", "missing, not zero"} {
		if !strings.Contains(strings.ToLower(contract), required) {
			t.Fatalf("execution contract missing %q", required)
		}
	}

	workflow, _, err := runtime.ReadReference(context.Background(), resolved, socialOperationsSkillID, "ten-step-sop.md")
	if err != nil {
		t.Fatalf("ReadReference(ten-step-sop.md) error = %v", err)
	}
	for _, heading := range []string{
		"## 1. Account check", "## 2. Product knowledge", "## 3. Research", "## 4. Selection", "## 5. Production",
		"## 6. Review", "## 7. Publication", "## 8. Interaction", "## 9. Archive", "## 10. Review loop",
	} {
		if !strings.Contains(workflow, heading) {
			t.Fatalf("ten-step reference missing heading %q", heading)
		}
	}
	for _, required := range []string{"English", "Chinese", "release", "license", "supplied logo", "16:9"} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("ten-step reference missing production/evidence rule %q", required)
		}
	}
}

func toolNames(definitions []skillspkg.SkillToolDefinition) []string {
	out := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		out = append(out, definition.Name)
	}
	return out
}
func hasReference(references []skillspkg.SkillReference, path string) bool {
	for _, reference := range references {
		if reference.Path == path {
			return true
		}
	}
	return false
}
func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := map[string]int{}
	for _, item := range left {
		counts[item]++
	}
	for _, item := range right {
		counts[item]--
		if counts[item] < 0 {
			return false
		}
	}
	return true
}
