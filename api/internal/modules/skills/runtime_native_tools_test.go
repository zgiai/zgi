package skills

import (
	"context"
	"strings"
	"testing"

	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin/calculator"
)

func TestBuildNativeToolSetUsesContractAndActivatesPromptOnlySkill(t *testing.T) {
	runtime := NewRuntime(nil, nil)
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
			Metadata:     SkillMetadata{ID: "prompt-only", Description: "Writing guidance", RuntimeType: SkillRuntimeTypePrompt},
			Instructions: "Follow the requested tone.",
		},
	}}

	toolSet := runtime.BuildNativeToolSet(context.Background(), resolved, NativeToolSetOptions{BudgetChars: 10000})
	if len(toolSet.ActiveSkillIDs) != 2 {
		t.Fatalf("active skills = %#v, want calculator and prompt-only", toolSet.ActiveSkillIDs)
	}
	if len(toolSet.ProviderTools) != 1 || toolSet.ProviderTools[0].Function.Name != "calculate" {
		t.Fatalf("provider tools = %#v, want native calculate", toolSet.ProviderTools)
	}
	binding := toolSet.ToolBindings["calculate"]
	if binding.SkillID != SkillCalculator || binding.ToolName != "calculate" {
		t.Fatalf("binding = %#v, want calculator/calculate", binding)
	}
	if len(toolSet.InstructionMessages) != 2 {
		t.Fatalf("instruction messages = %d, want 2", len(toolSet.InstructionMessages))
	}
}

func TestBuildNativeToolSetActivatesStructuredFileGeneratorWithinBudget(t *testing.T) {
	runtime := NewRuntime(nil, nil)
	resolved := &ResolvedSkills{Skills: []SkillDocument{{
		Metadata:     SkillMetadata{ID: SkillFileGenerator, Description: "Generate files"},
		Instructions: "Generate the requested file with the matching native tool.",
		Tools: []SkillToolDefinition{
			{Name: "generate_file"},
			{Name: "generate_docx"},
			{Name: "generate_pdf"},
			{Name: "generate_pptx"},
		},
	}}}

	toolSet := runtime.BuildNativeToolSet(context.Background(), resolved, NativeToolSetOptions{BudgetChars: 100000})
	if len(toolSet.SkippedSkills) != 0 || len(toolSet.ActiveSkillIDs) != 1 {
		t.Fatalf("tool set activation = %#v, want active file-generator", toolSet)
	}
	if len(toolSet.ProviderTools) != 4 {
		t.Fatalf("provider tools = %d, want 4", len(toolSet.ProviderTools))
	}
	if toolSet.SchemaChars <= 0 || toolSet.SchemaChars >= toolSet.BudgetChars {
		t.Fatalf("schema chars = %d, budget = %d", toolSet.SchemaChars, toolSet.BudgetChars)
	}
	for _, tool := range toolSet.ProviderTools {
		parameters, _ := tool.Function.Parameters.(map[string]interface{})
		properties, _ := parameters["properties"].(map[string]interface{})
		switch tool.Function.Name {
		case "generate_docx":
			document, _ := properties["document"].(map[string]interface{})
			if document["type"] != "object" {
				t.Fatalf("generate_docx document schema = %#v", document)
			}
		case "generate_pptx":
			presentation, _ := properties["presentation"].(map[string]interface{})
			if presentation["type"] != "object" {
				t.Fatalf("generate_pptx presentation schema = %#v", presentation)
			}
		}
	}
}

func TestBuildNativeToolSetSkipsSkillAtomically(t *testing.T) {
	tests := []struct {
		name     string
		resolved *ResolvedSkills
		options  NativeToolSetOptions
		reason   string
	}{
		{
			name: "schema unavailable",
			resolved: &ResolvedSkills{Skills: []SkillDocument{{
				Metadata:     SkillMetadata{ID: "custom"},
				Instructions: "Use the custom tool.",
				Tools:        []SkillToolDefinition{{Name: "unknown_tool"}},
			}}},
			options: NativeToolSetOptions{BudgetChars: 10000},
			reason:  "schema_unavailable",
		},
		{
			name: "context budget",
			resolved: &ResolvedSkills{Skills: []SkillDocument{{
				Metadata:     SkillMetadata{ID: "prompt-only"},
				Instructions: strings.Repeat("instruction", 20),
			}}},
			options: NativeToolSetOptions{BudgetChars: 1},
			reason:  "context_budget_exceeded",
		},
		{
			name: "missing dependency",
			resolved: &ResolvedSkills{Skills: []SkillDocument{{
				Metadata:     SkillMetadata{ID: SkillImageGenerator},
				Instructions: "Generate an image.",
				Tools:        []SkillToolDefinition{{Name: "generate_image"}},
			}}},
			options: NativeToolSetOptions{BudgetChars: 10000},
			reason:  "missing_dependency",
		},
		{
			name: "unauthorized",
			resolved: &ResolvedSkills{Skills: []SkillDocument{{
				Metadata:     SkillMetadata{ID: "prompt-only"},
				Instructions: "Do work.",
			}}},
			options: NativeToolSetOptions{
				BudgetChars: 10000,
				AuthorizeSkill: func(context.Context, string) (bool, error) {
					return false, nil
				},
			},
			reason: "unauthorized",
		},
		{
			name: "token budget",
			resolved: &ResolvedSkills{Skills: []SkillDocument{{
				Metadata:     SkillMetadata{ID: "prompt-only"},
				Instructions: "Do work.",
			}}},
			options: NativeToolSetOptions{
				BudgetTokens: 5,
				EstimateTokens: func([]llmadapter.Message, []llmadapter.Tool) int {
					return 10
				},
			},
			reason: "context_budget_exceeded",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolSet := NewRuntime(nil, nil).BuildNativeToolSet(context.Background(), tt.resolved, tt.options)
			if len(toolSet.ActiveSkillIDs) != 0 || len(toolSet.ProviderTools) != 0 {
				t.Fatalf("tool set = %#v, want whole skill skipped", toolSet)
			}
			if len(toolSet.SkippedSkills) != 1 || toolSet.SkippedSkills[0].Reason != tt.reason {
				t.Fatalf("skipped = %#v, want reason %q", toolSet.SkippedSkills, tt.reason)
			}
		})
	}
}

func TestNativeProviderToolNameKeepsValidUniqueNameAndAliasesConflicts(t *testing.T) {
	reserved := nativeControlToolNames()
	local := map[string]struct{}{}
	if got := nativeProviderToolName("calculator", "calculate", reserved, local); got != "calculate" {
		t.Fatalf("unique name = %q, want calculate", got)
	}
	aliased := nativeProviderToolName("custom.skill", MetaToolUpdatePlan, reserved, local)
	if aliased == MetaToolUpdatePlan || !validNativeToolName(aliased) {
		t.Fatalf("aliased name = %q, want stable valid alias", aliased)
	}
	again := nativeProviderToolName("custom.skill", MetaToolUpdatePlan, reserved, local)
	if again != aliased {
		t.Fatalf("alias changed: %q != %q", again, aliased)
	}
}

func TestAppendNativeToolProjectionsKeepsBusinessSchemaAndGovernedBinding(t *testing.T) {
	toolSet := NativeToolSet{
		ActiveSkillIDs: []string{SkillExternalApps},
		ToolBindings:   map[string]NativeToolBinding{},
		BudgetChars:    10000,
	}
	fixed := map[string]interface{}{
		"integration_id":         "wecom",
		"action_id":              "wecom.message.send",
		"action_schema_hash":     "hash-1",
		"action_schema_revision": "schema-1",
		"catalog_revision":       "catalog-1",
	}
	added := AppendNativeToolProjections(&toolSet, []NativeToolProjection{{
		Name:        "wecom_send_message",
		Description: "Send a WeCom message.",
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"content":  map[string]interface{}{"type": "string"},
				"internal": map[string]interface{}{"type": "string", "readOnly": true},
			},
			"required": []interface{}{"content"},
		},
		Binding: NativeToolBinding{
			SkillID:          SkillExternalApps,
			ToolName:         "execute_action",
			ArgumentEnvelope: "arguments",
			FixedArguments:   fixed,
		},
	}}, NativeToolProjectionOptions{})
	if added != 1 || len(toolSet.ProviderTools) != 1 {
		t.Fatalf("AppendNativeToolProjections() added=%d tools=%#v", added, toolSet.ProviderTools)
	}
	tool := toolSet.ProviderTools[0]
	if tool.Function.Name != "wecom_send_message" {
		t.Fatalf("provider function name = %q", tool.Function.Name)
	}
	parameters, _ := tool.Function.Parameters.(map[string]interface{})
	properties, _ := parameters["properties"].(map[string]interface{})
	if _, ok := properties["content"]; !ok {
		t.Fatalf("business schema = %#v, want content", parameters)
	}
	if _, ok := properties["internal"]; ok {
		t.Fatalf("model-visible schema retained readOnly field: %#v", parameters)
	}
	if _, ok := properties["integration_id"]; ok {
		t.Fatalf("model-visible schema exposes fixed identity: %#v", parameters)
	}
	binding := toolSet.ToolBindings[tool.Function.Name]
	if binding.SkillID != SkillExternalApps || binding.ToolName != "execute_action" || binding.ArgumentEnvelope != "arguments" ||
		binding.FixedArguments["action_id"] != "wecom.message.send" {
		t.Fatalf("binding = %#v", binding)
	}
	fixed["action_id"] = "spoofed.after.append"
	if binding.FixedArguments["action_id"] != "wecom.message.send" {
		t.Fatalf("binding aliased caller-owned fixed arguments: %#v", binding.FixedArguments)
	}
	if toolSet.SchemaChars <= 0 || toolSet.SchemaChars >= toolSet.BudgetChars {
		t.Fatalf("schema chars = %d budget = %d", toolSet.SchemaChars, toolSet.BudgetChars)
	}
}

func TestAppendNativeToolProjectionsUsesStableLimitAndRemainingBudget(t *testing.T) {
	projection := func(name string) NativeToolProjection {
		return NativeToolProjection{
			Name: name,
			InputSchema: map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           map[string]interface{}{},
			},
			Binding: NativeToolBinding{SkillID: SkillExternalApps, ToolName: "execute_action"},
		}
	}
	toolSet := NativeToolSet{ToolBindings: map[string]NativeToolBinding{}, BudgetChars: 10000}
	if added := AppendNativeToolProjections(&toolSet, []NativeToolProjection{
		projection("first_action"), projection("second_action"), projection("third_action"),
	}, NativeToolProjectionOptions{MaxTools: 2}); added != 2 {
		t.Fatalf("AppendNativeToolProjections() added = %d, want 2", added)
	}
	if len(toolSet.ProviderTools) != 2 || toolSet.ProviderTools[0].Function.Name != "first_action" ||
		toolSet.ProviderTools[1].Function.Name != "second_action" {
		t.Fatalf("stable projection order = %#v", toolSet.ProviderTools)
	}
	if len(toolSet.SkippedTools) != 1 || toolSet.SkippedTools[0].ToolName != "third_action" ||
		toolSet.SkippedTools[0].Reason != "tool_limit_exceeded" {
		t.Fatalf("skipped projections = %#v", toolSet.SkippedTools)
	}

	budgeted := NativeToolSet{
		ToolBindings:     map[string]NativeToolBinding{},
		InstructionChars: 5,
		SchemaChars:      5,
		BudgetChars:      11,
	}
	if added := AppendNativeToolProjections(&budgeted, []NativeToolProjection{projection("too_large")}, NativeToolProjectionOptions{}); added != 0 {
		t.Fatalf("budgeted AppendNativeToolProjections() added = %d", added)
	}
	if len(budgeted.SkippedTools) != 1 || budgeted.SkippedTools[0].Reason != "context_budget_exceeded" {
		t.Fatalf("budget skip = %#v", budgeted.SkippedTools)
	}
}

func TestNativeSchemaFromToolParametersUsesLLMFields(t *testing.T) {
	minimum := 1.0
	maximum := 10.0
	schema, ok := nativeSchemaFromToolParameters([]tools.ToolParameter{
		{Name: "query", Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormLLM, Required: true, LLMDescription: "Search query."},
		{Name: "limit", Type: tools.ToolParameterTypeNumber, Form: tools.ToolParameterFormLLM, MinValue: &minimum, MaxValue: &maximum},
		{Name: "hidden", Type: tools.ToolParameterTypeString, Form: tools.ToolParameterFormForm},
	})
	if !ok {
		t.Fatal("nativeSchemaFromToolParameters() ok = false")
	}
	properties, _ := schema["properties"].(map[string]interface{})
	if _, exists := properties["hidden"]; exists {
		t.Fatalf("properties = %#v, form-only field should be excluded", properties)
	}
	if properties["query"].(map[string]interface{})["description"] != "Search query." {
		t.Fatalf("query schema = %#v", properties["query"])
	}
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "query" {
		t.Fatalf("required = %#v, want query", required)
	}
}

func TestBuildNativeToolSetAliasesGlobalConflictsStably(t *testing.T) {
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(calculator.NewProvider()); err != nil {
		t.Fatalf("register calculator: %v", err)
	}
	runtime := NewRuntime(tools.NewToolEngine(manager), manager)
	resolved := &ResolvedSkills{Skills: []SkillDocument{
		{
			Metadata:     SkillMetadata{ID: "calculator-a"},
			Instructions: "Use calculator A.",
			Tools:        []SkillToolDefinition{{Name: "calculate", ProviderType: tools.ToolProviderTypeBuiltin, ProviderID: "calculator"}},
		},
		{
			Metadata:     SkillMetadata{ID: "calculator-b"},
			Instructions: "Use calculator B.",
			Tools:        []SkillToolDefinition{{Name: "calculate", ProviderType: tools.ToolProviderTypeBuiltin, ProviderID: "calculator"}},
		},
	}}

	build := func(skillID string) NativeToolSet {
		return runtime.BuildNativeToolSet(context.Background(), resolved, NativeToolSetOptions{
			BudgetChars:      10000,
			SelectedSkillIDs: []string{skillID},
		})
	}
	firstA := build("calculator-a")
	secondA := build("calculator-a")
	b := build("calculator-b")
	if len(firstA.ProviderTools) != 1 || len(secondA.ProviderTools) != 1 || len(b.ProviderTools) != 1 {
		t.Fatalf("provider tools = %#v %#v %#v, want one per selected skill", firstA.ProviderTools, secondA.ProviderTools, b.ProviderTools)
	}
	aName := firstA.ProviderTools[0].Function.Name
	if aName == "calculate" || aName != secondA.ProviderTools[0].Function.Name {
		t.Fatalf("calculator-a aliases = %q and %q, want stable non-original alias", aName, secondA.ProviderTools[0].Function.Name)
	}
	if bName := b.ProviderTools[0].Function.Name; bName == "calculate" || bName == aName {
		t.Fatalf("calculator-b alias = %q, want distinct stable alias from %q", bName, aName)
	}
}
