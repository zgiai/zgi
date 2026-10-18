package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

func TestBuildNativeToolSetExposesPackageScriptContract(t *testing.T) {
	runtime := NewRuntime(nil, nil)
	resolved := &ResolvedSkills{Skills: []SkillDocument{{
		Metadata:     SkillMetadata{ID: "private-example", Description: "Synthetic script contract"},
		Instructions: "Use run_script for the supplied input.",
		Tools:        []SkillToolDefinition{{Name: SkillScriptToolRun, InputSchema: objectSchema(map[string]interface{}{"request": map[string]interface{}{"type": "object"}, "payload": map[string]interface{}{"type": "string"}}, []string{"request"})}},
	}}}

	toolSet := runtime.BuildNativeToolSet(context.Background(), resolved, NativeToolSetOptions{BudgetChars: 100000})
	if len(toolSet.SkippedSkills) != 0 || len(toolSet.ActiveSkillIDs) != 1 {
		t.Fatalf("private skill activation = %#v, want active without skips", toolSet)
	}
	if len(toolSet.ProviderTools) != 1 || toolSet.ProviderTools[0].Function.Name != SkillScriptToolRun {
		t.Fatalf("provider tools = %#v, want native run_script", toolSet.ProviderTools)
	}
	parameters, ok := toolSet.ProviderTools[0].Function.Parameters.(map[string]interface{})
	if !ok {
		t.Fatalf("run_script parameters type = %T, want object schema", toolSet.ProviderTools[0].Function.Parameters)
	}
	properties, _ := parameters["properties"].(map[string]interface{})
	if _, ok := properties["request"].(map[string]interface{}); !ok {
		t.Fatalf("request schema missing from %#v", properties)
	}
	if _, ok := properties["payload"].(map[string]interface{}); !ok {
		t.Fatalf("payload schema missing from %#v", properties)
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

func TestBuildNativeToolSetUsesProviderInputSchemaWithoutLegacyParameters(t *testing.T) {
	catalogDir := t.TempDir()
	skillRoot := filepath.Join(catalogDir, "input-schema-only")
	if err := os.MkdirAll(skillRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	skill := `---
name: input-schema-only
description: Input schema only provider test
when_to_use: Use when testing native provider schema projection.
provider_type: connector
provider_id: input-schema-only
tools:
  - execute_action
runtime_type: tool
---
Use the provider action.
`
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	provider := newInputSchemaOnlyProviderForTest()
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(provider); err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}
	runtime := NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(t.Context(), []string{"input-schema-only"})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}

	toolSet := runtime.BuildNativeToolSet(t.Context(), resolved, NativeToolSetOptions{BudgetChars: 10000})
	if len(toolSet.SkippedSkills) != 0 || len(toolSet.ProviderTools) != 1 {
		t.Fatalf("tool set = %#v, want one active provider tool", toolSet)
	}
	parameters, ok := toolSet.ProviderTools[0].Function.Parameters.(map[string]interface{})
	if !ok {
		t.Fatalf("parameters = %#v, want object schema", toolSet.ProviderTools[0].Function.Parameters)
	}
	properties, ok := parameters["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties = %#v, want provider properties", parameters["properties"])
	}
	for _, name := range []string{"integration_id", "action_id", "arguments"} {
		if _, exists := properties[name]; !exists {
			t.Fatalf("properties = %#v, missing %q", properties, name)
		}
	}
	if _, exists := properties["connection_id"]; exists {
		t.Fatalf("properties = %#v, read-only connection_id must not be model-visible", properties)
	}
	required, ok := parameters["required"].([]interface{})
	if !ok || len(required) != 3 {
		t.Fatalf("required = %#v, want three model-owned fields", parameters["required"])
	}

	resolvedProperties := resolved.Skills[0].Tools[0].InputSchema["properties"].(map[string]interface{})
	if _, exists := resolvedProperties["connection_id"]; !exists {
		t.Fatalf("resolved input schema was mutated: %#v", resolved.Skills[0].Tools[0].InputSchema)
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

func TestNativeProviderToolNameAliasesForbiddenLegacyProtocolNamesCaseInsensitively(t *testing.T) {
	reserved := nativeControlToolNames()
	for _, name := range []string{"LOAD_SKILL", "Call_Skill_Tool", "submit_intermediate_answer", "SUBMIT_FINAL_ANSWER"} {
		got := nativeProviderToolName("external-apps/wecom.message.send", name, reserved, map[string]struct{}{})
		if got == "" || strings.EqualFold(got, name) {
			t.Fatalf("nativeProviderToolName(%q) = %q, want stable non-protocol alias", name, got)
		}
	}
}

func TestAppendNativeToolProjectionsKeepsBusinessSchemaAndGovernedBinding(t *testing.T) {
	toolSet := NativeToolSet{
		ActiveSkillIDs: []string{SkillConnectorRuntime},
		ToolBindings:   map[string]NativeToolBinding{},
		BudgetChars:    10000,
	}
	operation := &NativeConnectorOperation{ConnectorID: "wecom", OperationID: "wecom.message.send"}
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
			SkillID: SkillConnectorRuntime, ToolName: "wecom.message.send", ConnectorOperation: operation,
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
	if binding.SkillID != SkillConnectorRuntime || binding.ToolName != "wecom.message.send" || binding.ConnectorOperation == nil || *binding.ConnectorOperation != *operation {
		t.Fatalf("binding = %#v", binding)
	}
	operation.OperationID = "changed.after.append"
	if binding.ConnectorOperation.OperationID != "wecom.message.send" {
		t.Fatal("projection aliases caller-owned operation")
	}
	if toolSet.SchemaChars <= 0 || toolSet.SchemaChars >= toolSet.BudgetChars {
		t.Fatalf("schema chars = %d budget = %d", toolSet.SchemaChars, toolSet.BudgetChars)
	}
}

func TestAppendNativeToolProjectionsReservesRuntimeNamesCaseInsensitively(t *testing.T) {
	toolSet := NativeToolSet{ToolBindings: map[string]NativeToolBinding{}, BudgetChars: 10000}
	projection := NativeToolProjection{
		Name:      "Final_Answer",
		NameScope: "wecom/wecom.message.send",
		InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{}, "additionalProperties": false,
		},
		Binding: NativeToolBinding{
			SkillID: SkillConnectorRuntime, ToolName: "run_operation",
			ConnectorOperation: &NativeConnectorOperation{ConnectorID: "wecom", OperationID: "wecom.message.send"},
		},
	}
	if added := AppendNativeToolProjections(&toolSet, []NativeToolProjection{projection}, NativeToolProjectionOptions{
		ReservedToolNames: []string{"final_answer"},
	}); added != 1 {
		t.Fatalf("AppendNativeToolProjections() added = %d, want 1", added)
	}
	name := toolSet.ProviderTools[0].Function.Name
	if strings.EqualFold(name, "final_answer") {
		t.Fatalf("projection retained runtime-reserved name %q", name)
	}
	if binding := toolSet.ToolBindings[name]; binding.ConnectorOperation.OperationID != "wecom.message.send" {
		t.Fatalf("aliased binding = %#v", binding)
	}
}

func TestAppendNativeToolProjectionsAliasesThreeConnectorNameCollisions(t *testing.T) {
	toolSet := NativeToolSet{ToolBindings: map[string]NativeToolBinding{}, BudgetChars: 20000}
	projections := make([]NativeToolProjection, 0, 3)
	for _, identity := range []struct{ integrationID, actionID string }{
		{"wecom", "wecom.message.send"},
		{"dingtalk", "dingtalk.message.send"},
		{"feishu", "feishu.message.send"},
	} {
		projections = append(projections, NativeToolProjection{
			Name:      "send_message",
			NameScope: identity.integrationID + "/" + identity.actionID,
			InputSchema: map[string]interface{}{
				"type": "object", "properties": map[string]interface{}{}, "additionalProperties": false,
			},
			Binding: NativeToolBinding{
				SkillID: SkillConnectorRuntime, ToolName: "run_operation",
				ConnectorOperation: &NativeConnectorOperation{ConnectorID: identity.integrationID, OperationID: identity.actionID},
			},
		})
	}
	if added := AppendNativeToolProjections(&toolSet, projections, NativeToolProjectionOptions{}); added != 3 {
		t.Fatalf("AppendNativeToolProjections() added = %d skipped=%#v, want 3", added, toolSet.SkippedTools)
	}
	seenNames := map[string]struct{}{}
	seenActions := map[string]struct{}{}
	for _, tool := range toolSet.ProviderTools {
		name := strings.ToLower(tool.Function.Name)
		if _, duplicate := seenNames[name]; duplicate {
			t.Fatalf("duplicate projected alias %q", tool.Function.Name)
		}
		seenNames[name] = struct{}{}
		binding := toolSet.ToolBindings[tool.Function.Name]
		actionID := binding.ConnectorOperation.OperationID
		seenActions[actionID] = struct{}{}
	}
	if len(seenActions) != 3 {
		t.Fatalf("cross-integration bindings = %#v, want 3 distinct Actions", seenActions)
	}
}

func TestAppendNativeToolProjectionsKeepsDescriptionWhenAnotherToolIsBudgetOmitted(t *testing.T) {
	projection := func(name, actionID, description string) NativeToolProjection {
		return NativeToolProjection{
			Name: name, NameScope: "wecom/" + actionID, Description: description,
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			Binding: NativeToolBinding{
				SkillID: SkillConnectorRuntime, ToolName: "run_operation",
				ConnectorOperation: &NativeConnectorOperation{ConnectorID: "wecom", OperationID: actionID},
			},
		}
	}
	send := projection("send_message", "wecom.message.send", "Send a message to the supplied recipient.")
	lookup := projection("raw_contact_lookup", "wecom.contact.search", strings.Repeat("large schema description ", 20))
	probe := NativeToolSet{ToolBindings: map[string]NativeToolBinding{}}
	if AppendNativeToolProjections(&probe, []NativeToolProjection{send}, NativeToolProjectionOptions{}) != 1 {
		t.Fatal("failed to size the send projection")
	}
	toolSet := NativeToolSet{ToolBindings: map[string]NativeToolBinding{}, BudgetChars: probe.SchemaChars + 1}
	if added := AppendNativeToolProjections(&toolSet, []NativeToolProjection{send, lookup}, NativeToolProjectionOptions{}); added != 1 {
		t.Fatalf("added = %d skipped=%#v, want only send projection", added, toolSet.SkippedTools)
	}
	if len(toolSet.SkippedTools) != 1 || toolSet.SkippedTools[0].Reason != "context_budget_exceeded" {
		t.Fatalf("skipped tools = %#v, want budget omission", toolSet.SkippedTools)
	}
	if description := toolSet.ProviderTools[0].Function.Description; strings.Contains(description, "raw_contact_lookup") ||
		description != send.Description {
		t.Fatalf("send description depends on omitted preparation alias: %q", description)
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
			Binding: NativeToolBinding{SkillID: SkillConnectorRuntime, ToolName: "run_operation"},
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

func TestAppendNativeToolProjectionsDefaultsToFullActionCatalogWithinProviderCapacity(t *testing.T) {
	projection := func(index int) NativeToolProjection {
		actionID := fmt.Sprintf("action.%03d", index)
		return NativeToolProjection{
			Name: fmt.Sprintf("action_%03d", index), NameScope: "catalog/" + actionID,
			InputSchema: map[string]interface{}{
				"type": "object", "properties": map[string]interface{}{}, "additionalProperties": false,
			},
			Binding: NativeToolBinding{
				SkillID: SkillConnectorRuntime, ToolName: "run_operation",
				ConnectorOperation: &NativeConnectorOperation{ConnectorID: "catalog", OperationID: actionID},
			},
		}
	}
	projections := make([]NativeToolProjection, DefaultMaxNativeToolProjections)
	for index := range projections {
		projections[index] = projection(index)
	}
	estimateTokens := func(_ []llmadapter.Message, tools []llmadapter.Tool) int {
		return len(tools)
	}

	t.Run("default exposes all 128 candidates", func(t *testing.T) {
		toolSet := NativeToolSet{
			ToolBindings: map[string]NativeToolBinding{},
			BudgetChars:  1_000_000,
			BudgetTokens: 1_000_000,
		}
		added := AppendNativeToolProjections(&toolSet, projections, NativeToolProjectionOptions{
			EstimateTokens: estimateTokens,
		})
		if added != DefaultMaxNativeToolProjections || len(toolSet.ProviderTools) != DefaultMaxNativeToolProjections {
			t.Fatalf("added=%d provider tools=%d, want full %d-Action catalog", added, len(toolSet.ProviderTools), DefaultMaxNativeToolProjections)
		}
		if len(toolSet.SkippedTools) != 0 {
			t.Fatalf("default catalog projection skipped tools: %#v", toolSet.SkippedTools)
		}
		if toolSet.SchemaChars <= 0 || toolSet.SchemaChars > toolSet.BudgetChars ||
			toolSet.SchemaTokens != DefaultMaxNativeToolProjections || toolSet.SchemaTokens > toolSet.BudgetTokens {
			t.Fatalf("schema usage chars=%d/%d tokens=%d/%d", toolSet.SchemaChars, toolSet.BudgetChars, toolSet.SchemaTokens, toolSet.BudgetTokens)
		}
	})

	t.Run("existing provider tools consume shared capacity with diagnostics", func(t *testing.T) {
		const existingTools = 4
		toolSet := NativeToolSet{
			ToolBindings:  map[string]NativeToolBinding{},
			ProviderTools: make([]llmadapter.Tool, existingTools),
			BudgetChars:   1_000_000,
			BudgetTokens:  1_000_000,
		}
		for index := range toolSet.ProviderTools {
			toolSet.ProviderTools[index] = llmadapter.Tool{Type: "function", Function: llmadapter.Function{
				Name: fmt.Sprintf("existing_%d", index),
			}}
		}
		added := AppendNativeToolProjections(&toolSet, projections, NativeToolProjectionOptions{
			EstimateTokens: estimateTokens,
		})
		wantAdded := DefaultMaxNativeToolProjections - existingTools
		if added != wantAdded || len(toolSet.ProviderTools) != DefaultMaxNativeToolProjections {
			t.Fatalf("added=%d provider tools=%d, want added=%d total=%d", added, len(toolSet.ProviderTools), wantAdded, DefaultMaxNativeToolProjections)
		}
		if len(toolSet.SkippedTools) != existingTools {
			t.Fatalf("skipped tools=%d, want %d capacity diagnostics: %#v", len(toolSet.SkippedTools), existingTools, toolSet.SkippedTools)
		}
		for _, skip := range toolSet.SkippedTools {
			if skip.Reason != "tool_limit_exceeded" {
				t.Fatalf("capacity skip=%#v, want tool_limit_exceeded", skip)
			}
		}
	})
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

type inputSchemaOnlyProviderForTest struct {
	tool *inputSchemaOnlyToolForTest
}

func newInputSchemaOnlyProviderForTest() *inputSchemaOnlyProviderForTest {
	return &inputSchemaOnlyProviderForTest{tool: &inputSchemaOnlyToolForTest{}}
}

func (p *inputSchemaOnlyProviderForTest) GetEntity() tools.ToolProviderEntity {
	return tools.ToolProviderEntity{
		Identity:     tools.ToolProviderIdentity{Name: "input-schema-only"},
		ProviderType: tools.ToolProviderTypeConnector,
		Tools:        []tools.ToolEntity{p.tool.GetEntity()},
	}
}

func (p *inputSchemaOnlyProviderForTest) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeConnector
}

func (p *inputSchemaOnlyProviderForTest) GetTool(name string) (tools.Tool, error) {
	if name != "execute_action" {
		return nil, tools.ErrToolNotFound
	}
	return p.tool, nil
}

func (p *inputSchemaOnlyProviderForTest) GetTools() []tools.Tool { return []tools.Tool{p.tool} }

func (p *inputSchemaOnlyProviderForTest) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type inputSchemaOnlyToolForTest struct{}

func (t *inputSchemaOnlyToolForTest) GetEntity() tools.ToolEntity {
	return tools.ToolEntity{
		Identity:    tools.ToolIdentity{Name: "execute_action", Provider: "input-schema-only"},
		Description: tools.ToolDescription{LLM: "Execute an action using its provider-defined input schema."},
		InputSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"integration_id": map[string]interface{}{"type": "string"},
				"action_id":      map[string]interface{}{"type": "string"},
				"arguments":      map[string]interface{}{"type": "object", "additionalProperties": true},
				"connection_id":  map[string]interface{}{"type": "string", "format": "uuid", "readOnly": true},
			},
			"required": []string{"integration_id", "action_id", "arguments", "connection_id"},
		},
	}
}

func (t *inputSchemaOnlyToolForTest) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeConnector
}

func (t *inputSchemaOnlyToolForTest) GetTenantID() string { return "" }

func (t *inputSchemaOnlyToolForTest) Invoke(context.Context, string, map[string]interface{}, *string, *string, *string) ([]tools.ToolInvokeMessage, error) {
	return nil, nil
}

func (t *inputSchemaOnlyToolForTest) GetRuntimeParameters(context.Context, *string, *string, *string) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *inputSchemaOnlyToolForTest) ForkToolRuntime(*tools.ToolRuntime) tools.Tool { return t }

func (t *inputSchemaOnlyToolForTest) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}
