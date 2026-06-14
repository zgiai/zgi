package skills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
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
	if internal.Metadata.MaxCallsPerTurn != 20 {
		t.Fatalf("internal knowledge max calls = %d, want 20", internal.Metadata.MaxCallsPerTurn)
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
	if agent.Metadata.MaxCallsPerTurn != 20 {
		t.Fatalf("agent knowledge max calls = %d, want 20", agent.Metadata.MaxCallsPerTurn)
	}
	if agent.Metadata.Display.Label["zh_Hans"] != "智能体知识库" {
		t.Fatalf("agent knowledge zh label = %q", agent.Metadata.Display.Label["zh_Hans"])
	}
	if strings.Contains(agent.Metadata.Display.Description["zh_Hans"], "�") || strings.Contains(agent.Metadata.Display.Description["zh_Hans"], "?") {
		t.Fatalf("agent knowledge zh description looks corrupted: %q", agent.Metadata.Display.Description["zh_Hans"])
	}
}

func TestDatabaseSystemSkillsExposeExpectedTools(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillInternalDatabase, SkillAgentDatabase})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	expectedTools := []string{
		"list_accessible_databases",
		"list_database_tables",
		"describe_database_table",
		"query_table_records",
		"insert_table_records",
		"update_table_records",
		"delete_table_records",
	}
	internal, ok := resolved.Get(SkillInternalDatabase)
	if !ok {
		t.Fatalf("internal database skill was not resolved")
	}
	if got := toolNames(internal.Tools); !sameStrings(got, expectedTools) {
		t.Fatalf("internal database tools = %v", got)
	}
	if internal.Metadata.MaxCallsPerTurn != 40 {
		t.Fatalf("internal database max calls = %d, want 40", internal.Metadata.MaxCallsPerTurn)
	}
	agent, ok := resolved.Get(SkillAgentDatabase)
	if !ok {
		t.Fatalf("agent database skill was not resolved")
	}
	if got := toolNames(agent.Tools); !sameStrings(got, expectedTools) {
		t.Fatalf("agent database tools = %v", got)
	}
	if agent.Metadata.MaxCallsPerTurn != 40 {
		t.Fatalf("agent database max calls = %d, want 40", agent.Metadata.MaxCallsPerTurn)
	}
}

func TestAgentWorkflowSystemSkillExposeExpectedTools(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillAgentWorkflow})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	agent, ok := resolved.Get(SkillAgentWorkflow)
	if !ok {
		t.Fatalf("agent workflow skill was not resolved")
	}
	expectedTools := []string{"get_workflow_run_status", "list_agent_workflows", "run_agent_workflow"}
	if got := toolNames(agent.Tools); !sameStrings(got, expectedTools) {
		t.Fatalf("agent workflow tools = %v", got)
	}
	if !sameStrings(agent.Metadata.SupportedCallers, []string{SkillCallerAgent}) {
		t.Fatalf("supported callers = %#v, want agent", agent.Metadata.SupportedCallers)
	}
	if !sameStrings(agent.Metadata.RequiredConfig, []string{SkillRequiredConfigAgentWorkflow}) {
		t.Fatalf("required config = %#v, want agent_workflow", agent.Metadata.RequiredConfig)
	}
	if !IsHiddenSystemSkill(SkillAgentWorkflow) {
		t.Fatal("agent-workflow should be hidden")
	}
	if got := ExpectedSkillToolArguments(SkillAgentWorkflow, "run_agent_workflow"); got == nil {
		t.Fatal("run_agent_workflow contract missing")
	}
}

func TestWorkReportSystemSkillMetadata(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillWorkReport})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillWorkReport)
	if !ok {
		t.Fatalf("work report skill was not resolved")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypeHybrid {
		t.Fatalf("runtime type = %q, want hybrid", doc.Metadata.RuntimeType)
	}
	if got := doc.Metadata.Display.Label["zh_Hans"]; got != "周报月报生成" {
		t.Fatalf("zh label = %q", got)
	}
	if got := doc.Metadata.Display.WhenToUse["zh_Hans"]; got != "当用户需要生成周报、月报、工作总结、项目进展汇报或管理汇报时使用。" {
		t.Fatalf("zh when_to_use = %q", got)
	}
	if got := doc.Metadata.Display.Tags["zh_Hans"]; !sameStrings(got, []string{"周报", "月报", "工作总结"}) {
		t.Fatalf("zh tags = %v", got)
	}
}

func TestSchedulePlannerSystemSkillMetadata(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillSchedulePlanner})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillSchedulePlanner)
	if !ok {
		t.Fatalf("schedule planner skill was not resolved")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypePrompt {
		t.Fatalf("runtime type = %q, want prompt", doc.Metadata.RuntimeType)
	}
	if len(doc.Tools) != 0 {
		t.Fatalf("tools = %v, want none", doc.Tools)
	}
	if got := doc.Metadata.Display.Label["zh_Hans"]; got != "日程规划" {
		t.Fatalf("zh label = %q", got)
	}
	if got := doc.Metadata.Display.WhenToUse["zh_Hans"]; got != "用于规划每日安排、每周计划、任务排期、会议议程、学习计划或工作负载。" {
		t.Fatalf("zh when_to_use = %q", got)
	}
	if got := doc.Metadata.Display.Tags["zh_Hans"]; !sameStrings(got, []string{"日程", "计划", "效率"}) {
		t.Fatalf("zh tags = %v", got)
	}
}

func TestChartGeneratorSystemSkillMetadata(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillChartGenerator})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillChartGenerator)
	if !ok {
		t.Fatalf("chart generator skill was not resolved")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypeTool {
		t.Fatalf("runtime type = %q, want tool", doc.Metadata.RuntimeType)
	}
	if doc.Metadata.HasScripts {
		t.Fatalf("expected chart generator not to have scripts")
	}
	if doc.Metadata.ScriptsSupported {
		t.Fatalf("scripts supported = true for builtin chart generator")
	}
	tool, ok := findSkillTool(*doc, "generate_chart")
	if !ok {
		t.Fatalf("expected generate_chart tool")
	}
	if tool.ProviderType != "builtin" || tool.ProviderID != "chart_generator" {
		t.Fatalf("tool provider = %s/%s, want builtin/chart_generator", tool.ProviderType, tool.ProviderID)
	}
	if got := doc.Metadata.Display.Label["zh_Hans"]; got != "图表生成器" {
		t.Fatalf("zh label = %q", got)
	}
	if got := doc.Metadata.Display.WhenToUse["zh_Hans"]; got != "当回答需要生成图表文件时使用。" {
		t.Fatalf("zh when_to_use = %q", got)
	}
	if got := doc.Metadata.Display.Tags["zh_Hans"]; !sameStrings(got, []string{"图表", "可视化", "数据"}) {
		t.Fatalf("zh tags = %v", got)
	}
	if len(doc.Metadata.References) != 7 {
		t.Fatalf("references = %#v, want 7 chart references", doc.Metadata.References)
	}
	for _, path := range []string{"chart-radar.md", "chart-bar.md", "chart-line.md", "chart-pie.md", "chart-doughnut.md", "chart-scatter.md", "chart-score-distribution.md"} {
		if !hasReference(doc.Metadata.References, path) {
			t.Fatalf("references = %#v, missing %s", doc.Metadata.References, path)
		}
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

func TestRequestUserInputMetaToolIsAlwaysExposed(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillCalculator})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	metaTools := MetaToolsForSkillState(resolved, map[string]struct{}{})
	tool := findMetaTool(metaTools, MetaToolRequestUserInput)
	if tool == nil {
		t.Fatalf("request_user_input meta tool not found")
	}
	params, ok := tool.Function.Parameters.(map[string]interface{})
	if !ok {
		t.Fatalf("parameters type = %T, want map[string]interface{}", tool.Function.Parameters)
	}
	required, _ := params["required"].([]string)
	if len(required) != 2 || required[0] != "message" || required[1] != "questions" {
		t.Fatalf("required = %#v, want message and questions", params["required"])
	}
	properties, _ := params["properties"].(map[string]interface{})
	if _, ok := properties["message"]; !ok {
		t.Fatalf("message property missing from %#v", properties)
	}
	if _, ok := properties["questions"]; !ok {
		t.Fatalf("questions property missing from %#v", properties)
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
		SkillAgentDatabase,
		SkillCalculator,
		SkillFileGenerator,
		SkillChartGenerator,
		SkillWorkReport,
		SkillInternalDatabase,
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
		{SkillFileGenerator, "generate_docx", []string{"document"}},
		{SkillFileGenerator, "generate_pdf", []string{"html"}},
		{SkillFileGenerator, "generate_pptx", []string{"presentation"}},
		{SkillChartGenerator, "generate_chart", []string{"chart_type", "data"}},
		{SkillWorkReport, "generate_file", []string{"content", "format"}},
		{SkillInternalKnowledge, "retrieve_knowledge", []string{"query", "dataset_ids"}},
		{SkillAgentKnowledge, "retrieve_agent_knowledge", []string{"query"}},
		{SkillInternalDatabase, "query_table_records", []string{"data_source_id", "table_id"}},
		{SkillAgentDatabase, "insert_table_records", []string{"data_source_id", "table_id", "records"}},
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

func TestChartGeneratorContractSupportsBarAndLinePayloads(t *testing.T) {
	expected := ExpectedSkillToolArguments(SkillChartGenerator, "generate_chart")
	if expected == nil {
		t.Fatalf("ExpectedSkillToolArguments() = nil")
	}
	schema, ok := expected["schema"].(map[string]interface{})
	if !ok {
		t.Fatalf("schema type = %T, want map[string]interface{}", expected["schema"])
	}
	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("schema.properties missing")
	}
	dataSchema, ok := properties["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data schema missing")
	}
	if hasRequired(dataSchema, "dimensions") {
		t.Fatalf("top-level data schema should not require dimensions: %#v", dataSchema)
	}
	branches, ok := dataSchema["anyOf"].([]interface{})
	if !ok || len(branches) < 7 {
		t.Fatalf("data anyOf = %#v, want radar/bar/line/pie/scatter/distribution branches", dataSchema["anyOf"])
	}
	for _, required := range []string{"dimensions", "categories", "x_axis", "items", "points", "bands"} {
		if findSchemaWithRequired(branches, required) == nil {
			t.Fatalf("data schema branch requiring %s not found: %#v", required, branches)
		}
	}
	if findSchemaWithRequired(branches, "scores") == nil {
		t.Fatalf("data schema branch requiring scores not found: %#v", branches)
	}
	for _, rawBranch := range branches {
		branch, ok := rawBranch.(map[string]interface{})
		if !ok || !hasRequired(branch, "bands") {
			continue
		}
		if branchAllowsLabelOnlyBands(branch) {
			t.Fatalf("score distribution bands schema allows label-only bands: %#v", branch)
		}
	}
}

func TestMetaToolArgumentsExposeAllLoadedSystemToolContracts(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	skillIDs := []string{
		SkillAgentKnowledge,
		SkillAgentDatabase,
		SkillCalculator,
		SkillFileGenerator,
		SkillChartGenerator,
		SkillWorkReport,
		SkillInternalDatabase,
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

func TestSystemSkillToolGovernanceManifestLoadedFromFrontmatter(t *testing.T) {
	catalog := t.TempDir()
	root := filepath.Join(catalog, "governance-skill")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	skill := `---
name: governance-skill
description: governance test skill
when_to_use: verify governance manifest parsing
provider_type: builtin
provider_id: files
tools:
  - file.read
  - file.delete
runtime_type: tool
tool_governance:
  file.read:
    domain: files
    effect: read
    asset_type: File
    risk_level: LOW
    requires_asset_resolution: true
    audit_required: true
  file.delete:
    tool_id: files.delete
    domain: files
    effect: delete
    asset_type: file
    risk_level: high
    default_approval_policy: always_ask
    allowed_permission_tiers:
      - advanced
      - full
---
Use governed tools.
`
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runtime := NewRuntimeWithCatalog(nil, nil, catalog)
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{"governance-skill"})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get("governance-skill")
	if !ok {
		t.Fatalf("governance skill was not resolved")
	}
	readTool, ok := findSkillTool(*doc, "file.read")
	if !ok {
		t.Fatalf("file.read tool not found")
	}
	if readTool.Governance == nil {
		t.Fatalf("file.read governance manifest missing")
	}
	if readTool.Governance.ToolID != "file.read" {
		t.Fatalf("file.read tool_id = %q", readTool.Governance.ToolID)
	}
	if readTool.Governance.SkillID != "governance-skill" {
		t.Fatalf("file.read skill_id = %q", readTool.Governance.SkillID)
	}
	if readTool.Governance.Effect != toolgovernance.EffectRead || readTool.Governance.AssetType != "file" || readTool.Governance.RiskLevel != toolgovernance.RiskLevelLow {
		t.Fatalf("file.read governance not normalized: %#v", readTool.Governance)
	}

	deleteTool, ok := findSkillTool(*doc, "file.delete")
	if !ok {
		t.Fatalf("file.delete tool not found")
	}
	if deleteTool.Governance == nil {
		t.Fatalf("file.delete governance manifest missing")
	}
	if deleteTool.Governance.ToolID != "files.delete" {
		t.Fatalf("file.delete tool_id = %q", deleteTool.Governance.ToolID)
	}
	if deleteTool.Governance.DefaultApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk {
		t.Fatalf("file.delete approval policy = %q", deleteTool.Governance.DefaultApprovalPolicy)
	}
	if got := deleteTool.Governance.AllowedPermissionTiers; len(got) != 2 || got[0] != toolgovernance.PermissionTierAdvanced || got[1] != toolgovernance.PermissionTierFull {
		t.Fatalf("file.delete allowed tiers = %#v", got)
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

func branchAllowsLabelOnlyBands(branch map[string]interface{}) bool {
	properties, ok := branch["properties"].(map[string]interface{})
	if !ok {
		return false
	}
	bands, ok := properties["bands"].(map[string]interface{})
	if !ok {
		return false
	}
	items, ok := bands["items"].(map[string]interface{})
	if !ok {
		return false
	}
	required, ok := items["required"].([]string)
	if !ok {
		values, ok := items["required"].([]interface{})
		if !ok {
			return true
		}
		required = make([]string, 0, len(values))
		for _, value := range values {
			text, _ := value.(string)
			if text != "" {
				required = append(required, text)
			}
		}
	}
	return len(required) == 1 && required[0] == "label"
}

func hasReference(references []SkillReference, path string) bool {
	for _, reference := range references {
		if reference.Path == path {
			return true
		}
	}
	return false
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
