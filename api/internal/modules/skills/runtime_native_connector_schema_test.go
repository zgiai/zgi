package skills_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	toolprovider "github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestNativeConnectorSchemasPreserveFirstReleaseOperationContracts(t *testing.T) {
	definitions, err := catalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range definitions {
		// Only built-ins that support the Chat surface project through this
		// Chat connector runtime. The DigitalHuman-only SSH contract carries
		// the serverops Skill identity instead and is covered by the focused
		// test below.
		if !definition.Supports(catalog.SurfaceChat) {
			continue
		}
		t.Run(definition.ID, func(t *testing.T) {
			runtime, dispatcher := nativeConnectorSchemaRuntime(t, definition)
			resolved := nativeConnectorSchemaSkills(definition)
			toolSet := runtime.BuildNativeToolSet(context.Background(), resolved, skills.NativeToolSetOptions{})
			if len(toolSet.SkippedSkills) != 0 || len(toolSet.ActiveSkillIDs) != 1 || len(toolSet.ProviderTools) != len(definition.Operations) {
				t.Fatalf("native Connector projection: active=%v skipped=%#v tools=%d want=%d", toolSet.ActiveSkillIDs, toolSet.SkippedSkills, len(toolSet.ProviderTools), len(definition.Operations))
			}
			for _, operation := range definition.Operations {
				binding, exists := toolSet.ToolBindings[operation.ToolName]
				if !exists || binding.ToolName != operation.ToolName || binding.SkillID != operation.SkillID {
					t.Fatalf("operation %s lost its unambiguous Tool alias: %#v", operation.ToolName, toolSet.ToolBindings)
				}
				var schema any
				for _, projected := range toolSet.ProviderTools {
					if projected.Function.Name == operation.ToolName {
						schema = projected.Function.Parameters
					}
				}
				if !reflect.DeepEqual(schema, tools.ModelVisibleJSONSchema(operation.InputSchema)) {
					t.Fatalf("operation %s lost its authoritative input schema: %#v", operation.ToolName, schema)
				}
			}
			// This covers both Exa tools (arrays and date-time) and GitHub's
			// labels/assignees arrays without invoking a provider or loading keys.
			if dispatcher.calls != 0 {
				t.Fatalf("schema projection dispatched %d provider calls", dispatcher.calls)
			}
		})
	}
}

// TestNativeConnectorSchemaDigitalHumanOnlySSHContract covers the built-in that
// the first-release Chat loop intentionally skips: the SSH Server Operations
// contract is DigitalHuman-only and owns the serverops Skill identity, not the
// Chat connector-runtime Skill. Its admitted schemas must still project
// unchanged under that Skill identity.
func TestNativeConnectorSchemaDigitalHumanOnlySSHContract(t *testing.T) {
	definitions, err := catalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	var ssh *catalog.Definition
	for index := range definitions {
		if definitions[index].ID == catalog.ServerOpsSSHConnectorID {
			ssh = &definitions[index]
			break
		}
	}
	if ssh == nil {
		t.Fatal("admitted SSH Server Operations contract is missing")
	}
	if ssh.Supports(catalog.SurfaceChat) || !ssh.Supports(catalog.SurfaceDigitalHuman) {
		t.Fatalf("SSH surfaces = %v, want DigitalHuman-only", ssh.Surfaces)
	}
	if len(ssh.Operations) == 0 {
		t.Fatal("SSH contract declares no operation")
	}
	for _, operation := range ssh.Operations {
		if operation.SkillID != skills.SkillConnectorRuntime {
			t.Fatalf("operation %s SkillID = %q, want %q", operation.ID, operation.SkillID, skills.SkillConnectorRuntime)
		}
	}

	runtime, dispatcher := nativeConnectorSchemaRuntime(t, *ssh)
	resolved := nativeConnectorSchemaSkillsForSkill(*ssh, skills.SkillConnectorRuntime)
	toolSet := runtime.BuildNativeToolSet(context.Background(), resolved, skills.NativeToolSetOptions{})
	if len(toolSet.SkippedSkills) != 0 || len(toolSet.ActiveSkillIDs) != 1 ||
		toolSet.ActiveSkillIDs[0] != skills.SkillConnectorRuntime || len(toolSet.ProviderTools) != len(ssh.Operations) {
		t.Fatalf("DigitalHuman SSH projection: active=%v skipped=%#v tools=%d want=%d", toolSet.ActiveSkillIDs, toolSet.SkippedSkills, len(toolSet.ProviderTools), len(ssh.Operations))
	}
	for _, operation := range ssh.Operations {
		binding, exists := toolSet.ToolBindings[operation.ToolName]
		if !exists || binding.ToolName != operation.ToolName || binding.SkillID != skills.SkillConnectorRuntime {
			t.Fatalf("operation %s lost its SSH Skill binding: %#v", operation.ToolName, toolSet.ToolBindings)
		}
		var schema any
		for _, projected := range toolSet.ProviderTools {
			if projected.Function.Name == operation.ToolName {
				schema = projected.Function.Parameters
			}
		}
		if !reflect.DeepEqual(schema, tools.ModelVisibleJSONSchema(operation.InputSchema)) {
			t.Fatalf("operation %s lost its authoritative input schema: %#v", operation.ToolName, schema)
		}
	}
	if dispatcher.calls != 0 {
		t.Fatalf("SSH schema projection dispatched %d provider calls", dispatcher.calls)
	}
}

func TestNativeConnectorSchemasStripServerFieldsWithoutMutatingCatalog(t *testing.T) {
	definition := nativeConnectorSchemaDefinition(t)
	const hiddenValue = "native-connector-schema-hidden-sentinel"
	source := map[string]interface{}{
		"type": "object", "additionalProperties": false,
		"properties": map[string]interface{}{
			"api_key": map[string]interface{}{"type": "string", "readOnly": true, "default": hiddenValue},
			"filters": map[string]interface{}{
				"type": "object", "additionalProperties": false,
				"properties": map[string]interface{}{
					"binding_id": map[string]interface{}{"type": "string", "readOnly": true, "const": hiddenValue},
					"since":      map[string]interface{}{"type": "string", "format": "date-time"},
					"labels": map[string]interface{}{
						"type": "array", "minItems": 1, "maxItems": 3,
						"items": map[string]interface{}{"type": "string", "minLength": 1},
					},
				},
				"required": []string{"binding_id", "labels"},
			},
		},
		"required": []string{"api_key", "filters"},
	}
	original, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	// Admitted plugin documents use normalized JSON values, including array
	// values for required and JSON numbers for numeric constraints.
	if err := json.Unmarshal(original, &source); err != nil {
		t.Fatal(err)
	}
	definition.Operations = definition.Operations[:1]
	definition.Operations[0].InputSchema = source
	runtime, dispatcher := nativeConnectorSchemaRuntime(t, definition)
	toolSet := runtime.BuildNativeToolSet(context.Background(), nativeConnectorSchemaSkills(definition), skills.NativeToolSetOptions{})
	if len(toolSet.ProviderTools) != 1 || len(toolSet.SkippedSkills) != 0 {
		t.Fatalf("nested Connector schema was omitted: %#v", toolSet.SkippedSkills)
	}
	projected := toolSet.ProviderTools[0].Function.Parameters
	if !reflect.DeepEqual(projected, tools.ModelVisibleJSONSchema(source)) {
		t.Fatalf("model schema did not preserve its visible structural contract: %#v", projected)
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"api_key", "binding_id", hiddenValue} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("model schema exposed server field %q", forbidden)
		}
	}
	projected.(map[string]interface{})["properties"].(map[string]interface{})["filters"].(map[string]interface{})["required"] = []string{"changed"}
	after, err := json.Marshal(source)
	if err != nil || string(after) != string(original) {
		t.Fatal("model schema mutation changed the authoritative catalog schema")
	}
	if dispatcher.calls != 0 {
		t.Fatal("schema projection invoked the dispatcher")
	}
}

func TestNativeConnectorSchemaProjectionKeepsAliasesStable(t *testing.T) {
	definition := nativeConnectorSchemaDefinition(t)
	definition.Operations = definition.Operations[:1]
	runtime, _ := nativeConnectorSchemaRuntime(t, definition)
	resolved := nativeConnectorSchemaSkills(definition)
	name := definition.Operations[0].ToolName
	options := skills.NativeToolSetOptions{ReservedToolNames: []string{name}}
	first := runtime.BuildNativeToolSet(context.Background(), resolved, options)
	second := runtime.BuildNativeToolSet(context.Background(), resolved, options)
	if len(first.ProviderTools) != 1 || len(second.ProviderTools) != 1 {
		t.Fatalf("aliased Connector tool was omitted: %#v / %#v", first.SkippedSkills, second.SkippedSkills)
	}
	alias := first.ProviderTools[0].Function.Name
	if alias == name || alias != second.ProviderTools[0].Function.Name || first.ToolBindings[alias].ToolName != name {
		t.Fatalf("Connector alias no longer maps stably to the original tool: %q / %#v", alias, first.ToolBindings)
	}
}

func TestNativeConnectorSchemaProjectionFailsClosedWithoutChangingOrdinarySkills(t *testing.T) {
	definition := nativeConnectorSchemaDefinition(t)
	definition.Operations = definition.Operations[:1]
	for _, test := range []struct {
		name   string
		change func(*skills.SkillToolDefinition)
	}{
		{name: "missing_schema", change: func(tool *skills.SkillToolDefinition) { tool.InputSchema = nil }},
		{name: "invalid_schema", change: func(tool *skills.SkillToolDefinition) {
			tool.InputSchema = map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}}
		}},
		{name: "wrong_provider_type", change: func(tool *skills.SkillToolDefinition) { tool.ProviderType = tools.ToolProviderTypeBuiltin }},
		{name: "missing_operation", change: func(tool *skills.SkillToolDefinition) { tool.ConnectorOperationID = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, _ := nativeConnectorSchemaRuntime(t, definition)
			resolved := nativeConnectorSchemaSkills(definition)
			test.change(&resolved.Skills[0].Tools[0])
			toolSet := runtime.BuildNativeToolSet(context.Background(), resolved, skills.NativeToolSetOptions{})
			if len(toolSet.ProviderTools) != 0 || len(toolSet.SkippedSkills) != 1 || toolSet.SkippedSkills[0].Reason != "schema_unavailable" {
				t.Fatalf("malformed native projection did not fail closed: %#v", toolSet)
			}
		})
	}

	// Merely putting a schema on an ordinary Skill must not bypass its
	// provider-backed schema resolution or its existing argument contract.
	untrusted := nativeConnectorSchemaSkills(definition)
	untrusted.Skills[0].Tools[0].ConnectorNative = false
	toolSet := skills.NewRuntime(nil, nil).BuildNativeToolSet(context.Background(), untrusted, skills.NativeToolSetOptions{})
	if len(toolSet.ProviderTools) != 0 || len(toolSet.SkippedSkills) != 1 {
		t.Fatalf("non-native Skill injected an untrusted schema: %#v", toolSet)
	}
	ordinary := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillCalculator},
		Tools: []skills.SkillToolDefinition{{
			Name: "evaluate_expression", ProviderType: tools.ToolProviderTypeBuiltin, ProviderID: "calculator",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"untrusted": map[string]interface{}{"type": "string"}}},
		}},
	}}}
	toolSet = skills.NewRuntime(nil, nil).BuildNativeToolSet(context.Background(), ordinary, skills.NativeToolSetOptions{})
	contract, exists := skills.SkillToolArgumentContractFor(skills.SkillCalculator, "evaluate_expression")
	if !exists || len(toolSet.ProviderTools) != 1 || !reflect.DeepEqual(toolSet.ProviderTools[0].Function.Parameters, contract.Schema) {
		t.Fatal("ordinary Skill no longer prioritizes its existing argument contract")
	}
}

func nativeConnectorSchemaDefinition(t *testing.T) catalog.Definition {
	t.Helper()
	definitions, err := catalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range definitions {
		if definition.ID == "web-search" {
			return definition
		}
	}
	t.Fatal("first-release Web Search definition is missing")
	return catalog.Definition{}
}

func nativeConnectorSchemaSkills(definition catalog.Definition) *skills.ResolvedSkills {
	return nativeConnectorSchemaSkillsForSkill(definition, skills.SkillConnectorRuntime)
}

func nativeConnectorSchemaSkillsForSkill(definition catalog.Definition, skillID string) *skills.ResolvedSkills {
	document := skills.SkillDocument{Metadata: skills.SkillMetadata{ID: skillID}}
	for _, operation := range definition.Operations {
		document.Tools = append(document.Tools, skills.SkillToolDefinition{
			Name: operation.ToolName, ProviderType: tools.ToolProviderTypeConnector, ProviderID: definition.ID,
			InputSchema: operation.InputSchema, OutputSchema: operation.OutputSchema,
			RuntimeDescription: operation.Description, ConnectorNative: true, ConnectorOperationID: operation.ID,
		})
	}
	return &skills.ResolvedSkills{Skills: []skills.SkillDocument{document}}
}

func nativeConnectorSchemaRuntime(t *testing.T, definition catalog.Definition) (*skills.Runtime, *nativeConnectorSchemaDispatcher) {
	t.Helper()
	if err := catalog.Validate(definition); err != nil {
		t.Fatalf("invalid Connector schema fixture: %v", err)
	}
	dispatcher := &nativeConnectorSchemaDispatcher{}
	provider, err := toolprovider.NewProvider(definition)
	if err != nil {
		t.Fatal(err)
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(provider); err != nil {
		t.Fatal(err)
	}
	return skills.NewRuntime(tools.NewToolEngine(manager), manager).WithConnectorDispatcher(dispatcher), dispatcher
}

type nativeConnectorSchemaDispatcher struct{ calls int }

func (dispatcher *nativeConnectorSchemaDispatcher) Dispatch(context.Context, connectordispatch.Invocation) (execution.ExecutionResult, error) {
	dispatcher.calls++
	return execution.ExecutionResult{State: execution.ExecutionStateCompleted}, nil
}
