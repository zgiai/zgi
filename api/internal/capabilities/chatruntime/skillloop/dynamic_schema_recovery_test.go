package skillloop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestDynamicSchemaFailureReturnsSafeExpectedArgumentsAndRetryFeedback(t *testing.T) {
	const (
		skillID       = "dynamic-external-apps-test"
		toolName      = "execute_action"
		rejectedValue = "invalid-connection-id-private-value"
	)
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skillID},
		Tools: []skills.SkillToolDefinition{{
			Name: toolName,
			InputSchema: map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"connection_id": map[string]interface{}{
						"type":        "string",
						"format":      "uuid",
						"description": "provider-controlled annotation that must not be repeated",
					},
				},
				"required": []string{"connection_id"},
			},
			Governance: &toolgovernance.Manifest{
				DataEgress:           true,
				SensitiveDataAllowed: false,
			},
		}},
	}}}
	runner := &Runner{SkillRuntime: skills.NewRuntime(nil, nil)}
	step := runner.handleCallSkillTool(
		context.Background(),
		NewPreparedChat("conversation", "message", "provider", "auto", nil),
		resolved,
		"call-dynamic-retry",
		map[string]interface{}{
			"skill_id":  skillID,
			"tool_name": toolName,
			"arguments": map[string]interface{}{
				"connection_id": rejectedValue,
			},
		},
		skills.ExecutionContext{},
		nil,
	)
	if !step.recoverable || step.fatalErr != nil {
		t.Fatalf("step recoverable=%v fatal=%v", step.recoverable, step.fatalErr)
	}
	if step.trace.Arguments["data_egress_redacted"] != true {
		t.Fatalf("trace arguments = %#v, want data-egress redaction", step.trace.Arguments)
	}

	var payload map[string]interface{}
	content, ok := step.toolMessage.Content.(string)
	if !ok {
		t.Fatalf("tool message content = %#v, want string", step.toolMessage.Content)
	}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("json.Unmarshal(tool message) error = %v; content=%s", err, step.toolMessage.Content)
	}
	if payload["recoverable"] != true {
		t.Fatalf("payload = %#v, want recoverable", payload)
	}
	nextAction, _ := payload["next_action"].(string)
	if !strings.Contains(nextAction, "Retry call_skill_tool") || !strings.Contains(nextAction, "expected_arguments.schema") {
		t.Fatalf("next_action = %q", nextAction)
	}
	expected, ok := payload["expected_arguments"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected_arguments = %#v", payload["expected_arguments"])
	}
	if expected["skill_id"] != skillID || expected["tool_name"] != toolName {
		t.Fatalf("expected_arguments identity = %#v", expected)
	}
	expectedSchema, ok := expected["schema"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected schema = %#v", expected["schema"])
	}
	properties, ok := expectedSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected schema properties = %#v", expectedSchema["properties"])
	}
	connectionSchema, ok := properties["connection_id"].(map[string]interface{})
	if !ok || connectionSchema["format"] != "uuid" {
		t.Fatalf("connection schema = %#v", properties["connection_id"])
	}
	argumentErrors, ok := payload["argument_errors"].([]interface{})
	if !ok || len(argumentErrors) != 1 {
		t.Fatalf("argument_errors = %#v", payload["argument_errors"])
	}
	issue, ok := argumentErrors[0].(map[string]interface{})
	if !ok || issue["path"] != "connection_id" || issue["keyword"] != "format" || issue["expected"] != "format uuid" {
		t.Fatalf("argument error = %#v", argumentErrors[0])
	}

	encodedStep, err := json.Marshal(map[string]interface{}{
		"trace":   step.trace,
		"payload": payload,
	})
	if err != nil {
		t.Fatalf("json.Marshal(step) error = %v", err)
	}
	text := string(encodedStep)
	for _, forbidden := range []string{rejectedValue, "provider-controlled annotation"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("recoverable feedback leaked %q: %s", forbidden, text)
		}
	}
}

func TestDynamicSchemaRecoveryHidesReadOnlyConnectionIdentity(t *testing.T) {
	const (
		skillID       = "external-apps"
		toolName      = "execute_action"
		rejectedValue = "private-invalid-connection-value"
	)
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skillID},
		Tools: []skills.SkillToolDefinition{{
			Name: toolName,
			InputSchema: map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"integration_id": map[string]interface{}{"type": "string"},
					"action_id":      map[string]interface{}{"type": "string"},
					"connection_id": map[string]interface{}{
						"type": "string", "format": "uuid", "readOnly": true,
					},
					"connection_name": map[string]interface{}{"type": "string", "readOnly": true},
					"arguments":       map[string]interface{}{"type": "object"},
				},
				"required": []string{"integration_id", "action_id", "arguments"},
			},
			Governance: &toolgovernance.Manifest{DataEgress: true, SensitiveDataAllowed: false},
		}},
	}}}
	step := (&Runner{SkillRuntime: skills.NewRuntime(nil, nil)}).handleCallSkillTool(
		context.Background(),
		NewPreparedChat("conversation", "message", "provider", "auto", nil),
		resolved,
		"call-hidden-connection",
		map[string]interface{}{
			"skill_id":  skillID,
			"tool_name": toolName,
			"arguments": map[string]interface{}{
				"integration_id": "github",
				"action_id":      "github.user.get",
				"connection_id":  rejectedValue,
				"arguments":      map[string]interface{}{},
			},
		},
		skills.ExecutionContext{},
		nil,
	)
	if !step.recoverable || step.fatalErr != nil {
		t.Fatalf("step recoverable=%v fatal=%v", step.recoverable, step.fatalErr)
	}
	encoded, err := json.Marshal(map[string]interface{}{
		"trace": step.trace,
		"tool":  step.toolMessage,
	})
	if err != nil {
		t.Fatalf("json.Marshal(recovery) error = %v", err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"connection_id", "connection_name", rejectedValue} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("recovery contains %q: %s", forbidden, text)
		}
	}
	for _, required := range []string{"integration_id", "action_id", "arguments", "Retry call_skill_tool"} {
		if !strings.Contains(text, required) {
			t.Fatalf("recovery missing %q: %s", required, text)
		}
	}
}
