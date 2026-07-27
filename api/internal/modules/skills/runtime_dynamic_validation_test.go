package skills

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestDynamicToolValidationErrorIsStructuralAndDataEgressSafe(t *testing.T) {
	const (
		skillID       = "dynamic-connector-test"
		toolName      = "execute_action"
		rejectedValue = "not-a-uuid-with-private-material"
	)
	inputSchema := map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"connection_id": map[string]interface{}{
				"type":        "string",
				"format":      "uuid",
				"description": "untrusted provider annotation",
			},
		},
		"required": []string{"connection_id"},
	}
	resolved := &ResolvedSkills{Skills: []SkillDocument{{
		Metadata: SkillMetadata{ID: skillID},
		Tools: []SkillToolDefinition{{
			Name:               toolName,
			InputSchema:        inputSchema,
			RuntimeDescription: "untrusted runtime description",
			Governance: &toolgovernance.Manifest{
				DataEgress:           true,
				SensitiveDataAllowed: false,
			},
		}},
	}}}

	invocation, err := NewRuntime(nil, nil).CallSkillTool(
		context.Background(),
		resolved,
		skillID,
		toolName,
		map[string]interface{}{"connection_id": rejectedValue},
		ExecutionContext{},
		"call-dynamic-validation",
	)
	if err == nil {
		t.Fatal("CallSkillTool() expected validation error")
	}
	if invocation == nil {
		t.Fatal("CallSkillTool() returned nil invocation")
	}
	for label, value := range map[string]string{
		"error":       err.Error(),
		"trace error": invocation.Trace.Error,
	} {
		if strings.Contains(value, rejectedValue) {
			t.Fatalf("%s leaked rejected value: %s", label, value)
		}
		if !strings.Contains(value, "connection_id") || !strings.Contains(value, "format uuid") {
			t.Fatalf("%s = %q, want field and structural constraint", label, value)
		}
	}
	if invocation.Trace.Arguments["data_egress_redacted"] != true {
		t.Fatalf("trace arguments = %#v, want data-egress redaction", invocation.Trace.Arguments)
	}
	if invocation.Trace.Arguments["arguments_redacted"] != true {
		t.Fatalf("trace arguments = %#v, want validation redaction", invocation.Trace.Arguments)
	}
	encodedTrace, marshalErr := json.Marshal(invocation.Trace)
	if marshalErr != nil {
		t.Fatalf("json.Marshal(trace) error = %v", marshalErr)
	}
	if strings.Contains(string(encodedTrace), rejectedValue) {
		t.Fatalf("trace leaked rejected value: %s", encodedTrace)
	}

	issues := SkillToolArgumentValidationIssues(err)
	if len(issues) != 1 || issues[0] != (tools.JSONSchemaValidationIssue{
		Path: "connection_id", Keyword: "format", Expected: "format uuid",
	}) {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestDynamicToolValidationTraceRedactsRejectedValuesWithoutDataEgress(t *testing.T) {
	const rejectedValue = "private-invalid-value"
	resolved := &ResolvedSkills{Skills: []SkillDocument{{
		Metadata: SkillMetadata{ID: "dynamic-local-test"},
		Tools: []SkillToolDefinition{{
			Name: "inspect",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"mode": map[string]interface{}{"type": "integer"},
				},
				"required": []string{"mode"},
			},
		}},
	}}}
	invocation, err := NewRuntime(nil, nil).CallSkillTool(
		context.Background(), resolved, "dynamic-local-test", "inspect",
		map[string]interface{}{"mode": rejectedValue}, ExecutionContext{}, "call-local-validation",
	)
	if err == nil || invocation == nil {
		t.Fatalf("CallSkillTool() invocation=%#v err=%v, want validation failure", invocation, err)
	}
	encoded, marshalErr := json.Marshal(invocation.Trace)
	if marshalErr != nil {
		t.Fatalf("json.Marshal(trace) error = %v", marshalErr)
	}
	if strings.Contains(string(encoded), rejectedValue) {
		t.Fatalf("trace leaked rejected value: %s", encoded)
	}
	if invocation.Trace.Arguments["arguments_redacted"] != true {
		t.Fatalf("trace arguments = %#v, want validation redaction", invocation.Trace.Arguments)
	}
}

func TestExpectedSkillToolArgumentsForResolvedUsesSafeDynamicSchema(t *testing.T) {
	const (
		skillID      = "dynamic-connector-test"
		toolName     = "execute_action"
		connectionID = "00000000-0000-4000-8000-000000000001"
	)
	resolved := &ResolvedSkills{Skills: []SkillDocument{{
		Metadata: SkillMetadata{ID: skillID},
		Tools: []SkillToolDefinition{{
			Name:               toolName,
			RuntimeDescription: "untrusted runtime description",
			InputSchema: map[string]interface{}{
				"type":        "object",
				"description": "untrusted schema description",
				"properties": map[string]interface{}{
					"connection_id": map[string]interface{}{
						"type":        "string",
						"format":      "uuid",
						"description": "untrusted field description",
						"default":     connectionID,
						"enum":        []string{connectionID},
					},
				},
				"required": []string{"connection_id"},
			},
		}},
	}}}

	if static := ExpectedSkillToolArguments(skillID, toolName); static != nil {
		t.Fatalf("static contract = %#v, want no static contract", static)
	}
	expected := ExpectedSkillToolArgumentsForResolved(resolved, skillID, toolName)
	if expected == nil {
		t.Fatal("ExpectedSkillToolArgumentsForResolved() returned nil")
	}
	encoded, err := json.Marshal(expected)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	text := string(encoded)
	for _, forbidden := range []string{connectionID, "untrusted runtime description", "untrusted schema description", "untrusted field description", "default"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("expected arguments contain %q: %s", forbidden, text)
		}
	}
	for _, required := range []string{`"skill_id":"dynamic-connector-test"`, `"tool_name":"execute_action"`, `"connection_id"`, `"format":"uuid"`} {
		if !strings.Contains(text, required) {
			t.Fatalf("expected arguments missing %s: %s", required, text)
		}
	}
}

func TestDynamicToolModelContractAndRecoveryHideServerOwnedConnectionFields(t *testing.T) {
	const (
		skillID      = "external-apps"
		toolName     = "execute_action"
		connectionID = "00000000-0000-4000-8000-000000000001"
	)
	rawSchema := map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"integration_id": map[string]interface{}{"type": "string"},
			"action_id":      map[string]interface{}{"type": "string"},
			"connection_id": map[string]interface{}{
				"type": "string", "format": "uuid", "readOnly": true,
				"enum": []string{connectionID},
			},
			"connection_name":  map[string]interface{}{"type": "string", "readOnly": true},
			"catalog_revision": map[string]interface{}{"type": "string", "readOnly": true},
			"arguments":        map[string]interface{}{"type": "object"},
		},
		"required": []string{"integration_id", "action_id", "connection_id", "arguments"},
	}
	resolved := &ResolvedSkills{Skills: []SkillDocument{{
		Metadata: SkillMetadata{ID: skillID},
		Tools:    []SkillToolDefinition{{Name: toolName, InputSchema: rawSchema}},
	}}}

	_, _, _, contracts := loadedToolOptions(resolved, map[string]struct{}{skillID: {}})
	if len(contracts) != 1 {
		t.Fatalf("contracts = %#v, want one", contracts)
	}
	var callSkillToolParameters interface{}
	for _, metaTool := range MetaToolsForSkillState(resolved, map[string]struct{}{skillID: {}}) {
		if metaTool.Function.Name == MetaToolCallSkillTool {
			callSkillToolParameters = metaTool.Function.Parameters
			break
		}
	}
	if callSkillToolParameters == nil {
		t.Fatal("call_skill_tool model contract not found")
	}
	expected := ExpectedSkillToolArgumentsForResolved(resolved, skillID, toolName)
	for label, value := range map[string]interface{}{
		"projected contract":       contracts[0].Schema,
		"call_skill_tool contract": callSkillToolParameters,
		"recovery":                 expected,
	} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("json.Marshal(%s) error = %v", label, err)
		}
		text := string(encoded)
		for _, forbidden := range []string{"connection_id", "connection_name", "catalog_revision", connectionID} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s contains %q: %s", label, forbidden, text)
			}
		}
		for _, required := range []string{`"integration_id"`, `"action_id"`, `"arguments"`} {
			if !strings.Contains(text, required) {
				t.Fatalf("%s missing %s: %s", label, required, text)
			}
		}
	}

	if err := tools.ValidateJSONSchemaValue(rawSchema, map[string]interface{}{
		"integration_id": "github",
		"action_id":      "github.user.get",
		"connection_id":  connectionID,
		"arguments":      map[string]interface{}{},
	}); err != nil {
		t.Fatalf("server-side raw schema rejected enriched canonical UUID: %v", err)
	}
}
