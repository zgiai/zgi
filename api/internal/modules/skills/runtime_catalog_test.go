package skills

import (
	"context"
	"os"
	"path/filepath"
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
	agent, ok := resolved.Get(SkillAgentKnowledge)
	if !ok {
		t.Fatalf("agent knowledge skill was not resolved")
	}
	if got := toolNames(agent.Tools); !sameStrings(got, []string{"retrieve_agent_knowledge"}) {
		t.Fatalf("agent knowledge tools = %v", got)
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
