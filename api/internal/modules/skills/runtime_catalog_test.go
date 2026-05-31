package skills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestKnowledgeSystemSkillsExposeExpectedTools(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillInternalKnowledge, SkillAgentKnowledge})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	internal, ok := resolved.Get(SkillInternalKnowledge)
	if !ok {
		t.Fatalf("internal knowledge skill was not resolved")
	}
	if got := toolNames(internal.Tools); !sameStrings(got, []string{"list_accessible_knowledge_bases", "retrieve_knowledge"}) {
		t.Fatalf("internal knowledge tools = %v", got)
	}
	if internal.Metadata.MaxCallsPerTurn != 6 {
		t.Fatalf("internal knowledge max calls = %d, want 6", internal.Metadata.MaxCallsPerTurn)
	}
	if internal.Metadata.Display.Label["zh_Hans"] != "内部知识库" {
		t.Fatalf("internal knowledge zh label = %q", internal.Metadata.Display.Label["zh_Hans"])
	}
	agent, ok := resolved.Get(SkillAgentKnowledge)
	if !ok {
		t.Fatalf("agent knowledge skill was not resolved")
	}
	if got := toolNames(agent.Tools); !sameStrings(got, []string{"retrieve_agent_knowledge"}) {
		t.Fatalf("agent knowledge tools = %v", got)
	}
	if agent.Metadata.MaxCallsPerTurn != 3 {
		t.Fatalf("agent knowledge max calls = %d, want 3", agent.Metadata.MaxCallsPerTurn)
	}
	if agent.Metadata.Display.Label["zh_Hans"] != "智能体知识库" {
		t.Fatalf("agent knowledge zh label = %q", agent.Metadata.Display.Label["zh_Hans"])
	}
	if strings.Contains(agent.Metadata.Display.Description["zh_Hans"], "�") || strings.Contains(agent.Metadata.Display.Description["zh_Hans"], "?") {
		t.Fatalf("agent knowledge zh description looks corrupted: %q", agent.Metadata.Display.Description["zh_Hans"])
	}
}

func TestAgentMemorySystemSkillIsNotLoadable(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	_, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillAgentMemory})
	if !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("ResolveEnabledSkills(agent-memory) error = %v, want ErrSkillNotFound", err)
	}
	for _, toolName := range []string{"read_agent_memory", "update_agent_memory", "clear_agent_memory"} {
		if got := ExpectedSkillToolArguments(SkillAgentMemory, toolName); got != nil {
			t.Fatalf("ExpectedSkillToolArguments(agent-memory/%s) = %#v, want nil", toolName, got)
		}
	}
}

func TestUserMemorySystemSkillIsNotLoadable(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	_, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillUserMemory})
	if !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("ResolveEnabledSkills(user-memory) error = %v, want ErrSkillNotFound", err)
	}
	for _, toolName := range []string{"read_user_memory", "add_user_memory", "update_user_memory", "delete_user_memory", "list_temporary_memories"} {
		if got := ExpectedSkillToolArguments(SkillUserMemory, toolName); got != nil {
			t.Fatalf("ExpectedSkillToolArguments(user-memory/%s) = %#v, want nil", toolName, got)
		}
	}
}

func TestCustomSkillCannotDeclareTools(t *testing.T) {
	root := t.TempDir()
	content := `---
name: custom-tool-skill
description: Invalid custom skill.
when_to_use: Never.
provider_type: builtin
provider_id: knowledge
runtime_type: prompt
tools:
  - retrieve_knowledge
---

# Invalid

This custom skill should not be allowed to declare tools.
`
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := LoadCustomSkillDocument(root); err == nil {
		t.Fatalf("LoadCustomSkillDocument() error = nil, want custom tool declaration rejection")
	}
}

func TestCalculatorMetaToolArgumentsExposeRequiredExpressionSchema(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillCalculator})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	metaTools := MetaToolsForSkillState(resolved, map[string]struct{}{SkillCalculator: {}})
	callTool := findMetaTool(metaTools, MetaToolCallSkillTool)
	if callTool == nil {
		t.Fatalf("call_skill_tool meta tool not found")
	}
	params, ok := callTool.Function.Parameters.(map[string]interface{})
	if !ok {
		t.Fatalf("parameters type = %T, want map[string]interface{}", callTool.Function.Parameters)
	}
	properties, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("parameters.properties missing")
	}
	arguments, ok := properties["arguments"].(map[string]interface{})
	if !ok {
		t.Fatalf("arguments schema missing")
	}
	oneOf, ok := arguments["oneOf"].([]interface{})
	if !ok || len(oneOf) == 0 {
		t.Fatalf("arguments.oneOf = %#v, want calculator tool schemas", arguments["oneOf"])
	}
	expressionSchema := findSchemaWithRequired(oneOf, "expression")
	if expressionSchema == nil {
		t.Fatalf("evaluate_expression schema requiring expression not found in %#v", oneOf)
	}
	expressionProperties, _ := expressionSchema["properties"].(map[string]interface{})
	if _, ok := expressionProperties["expression"]; !ok {
		t.Fatalf("expression property missing from %#v", expressionSchema)
	}
}

func TestExpectedSkillToolArgumentsForCalculator(t *testing.T) {
	expected := ExpectedSkillToolArguments(SkillCalculator, "evaluate_expression")
	if expected == nil {
		t.Fatalf("ExpectedSkillToolArguments() = nil")
	}
	schema, ok := expected["schema"].(map[string]interface{})
	if !ok {
		t.Fatalf("schema type = %T, want map[string]interface{}", expected["schema"])
	}
	if !hasRequired(schema, "expression") {
		t.Fatalf("schema does not require expression: %#v", schema)
	}
	example, ok := expected["example"].(map[string]interface{})
	if !ok || example["expression"] == "" {
		t.Fatalf("example missing expression: %#v", expected["example"])
	}
}

func TestSystemToolSkillsExposeArgumentContracts(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	skillIDs := []string{
		SkillAgentKnowledge,
		SkillCalculator,
		SkillFileGenerator,
		SkillInternalKnowledge,
		SkillTime,
	}
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), skillIDs)
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	for _, doc := range resolved.Skills {
		for _, tool := range doc.Tools {
			if _, ok := SkillToolArgumentContractFor(doc.Metadata.ID, tool.Name); !ok {
				t.Fatalf("missing argument contract for %s/%s", doc.Metadata.ID, tool.Name)
			}
		}
	}
}

func TestExpectedSkillToolArgumentsForBuiltInRequiredTools(t *testing.T) {
	tests := []struct {
		skillID  string
		toolName string
		required []string
	}{
		{SkillFileGenerator, "generate_file", []string{"content", "format"}},
		{SkillInternalKnowledge, "retrieve_knowledge", []string{"query", "dataset_ids"}},
		{SkillAgentKnowledge, "retrieve_agent_knowledge", []string{"query"}},
		{SkillTime, "date_calculate", []string{"operation"}},
	}
	for _, tt := range tests {
		t.Run(tt.skillID+"/"+tt.toolName, func(t *testing.T) {
			expected := ExpectedSkillToolArguments(tt.skillID, tt.toolName)
			if expected == nil {
				t.Fatalf("ExpectedSkillToolArguments() = nil")
			}
			schema, ok := expected["schema"].(map[string]interface{})
			if !ok {
				t.Fatalf("schema type = %T, want map[string]interface{}", expected["schema"])
			}
			for _, required := range tt.required {
				if !hasRequired(schema, required) {
					t.Fatalf("schema does not require %s: %#v", required, schema)
				}
			}
			example, ok := expected["example"].(map[string]interface{})
			if !ok || len(example) == 0 {
				t.Fatalf("example missing: %#v", expected["example"])
			}
		})
	}
}

func TestMetaToolArgumentsExposeAllLoadedSystemToolContracts(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	skillIDs := []string{
		SkillAgentKnowledge,
		SkillCalculator,
		SkillFileGenerator,
		SkillInternalKnowledge,
		SkillTime,
	}
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), skillIDs)
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	loaded := map[string]struct{}{}
	for _, id := range skillIDs {
		loaded[id] = struct{}{}
	}
	metaTools := MetaToolsForSkillState(resolved, loaded)
	callTool := findMetaTool(metaTools, MetaToolCallSkillTool)
	if callTool == nil {
		t.Fatalf("call_skill_tool meta tool not found")
	}
	params, ok := callTool.Function.Parameters.(map[string]interface{})
	if !ok {
		t.Fatalf("parameters type = %T, want map[string]interface{}", callTool.Function.Parameters)
	}
	properties, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("parameters.properties missing")
	}
	arguments, ok := properties["arguments"].(map[string]interface{})
	if !ok {
		t.Fatalf("arguments schema missing")
	}
	if _, hasOneOf := arguments["oneOf"]; hasOneOf {
		t.Fatalf("arguments.oneOf should not be used when optional-only contracts are loaded: %#v", arguments)
	}
	anyOf, ok := arguments["anyOf"].([]interface{})
	if !ok || len(anyOf) < 7 {
		t.Fatalf("arguments.anyOf = %#v, want built-in tool schemas", arguments["anyOf"])
	}
	for _, required := range []string{"content", "query", "operation"} {
		if findSchemaWithRequired(anyOf, required) == nil {
			t.Fatalf("schema requiring %s not found in %#v", required, anyOf)
		}
	}
}

func toolNames(tools []SkillToolDefinition) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.Name)
	}
	return out
}

func findMetaTool(metaTools []llmadapter.Tool, name string) *llmadapter.Tool {
	for idx := range metaTools {
		if metaTools[idx].Function.Name == name {
			return &metaTools[idx]
		}
	}
	return nil
}

func findSchemaWithRequired(schemas []interface{}, required string) map[string]interface{} {
	for _, raw := range schemas {
		schema, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if hasRequired(schema, required) {
			return schema
		}
	}
	return nil
}

func hasRequired(schema map[string]interface{}, required string) bool {
	values, ok := schema["required"].([]string)
	if ok {
		for _, value := range values {
			if value == required {
				return true
			}
		}
	}
	rawValues, ok := schema["required"].([]interface{})
	if ok {
		for _, value := range rawValues {
			if value == required {
				return true
			}
		}
	}
	return false
}

func sameStrings(left []string, right []string) bool {
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
