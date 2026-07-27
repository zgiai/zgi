package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestRecoverableFrozenDynamicInvocationUsesResolvedSchemaAndRedactsArguments(t *testing.T) {
	const (
		skillID      = "dynamic-external-apps-test"
		toolName     = "execute_action"
		connectionID = "00000000-0000-4000-8000-000000000001"
	)
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skillID},
		Tools: []skills.SkillToolDefinition{{
			Name: toolName,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"connection_id": map[string]interface{}{
						"type":        "string",
						"format":      "uuid",
						"description": "provider annotation must not be echoed",
					},
				},
				"required": []string{"connection_id"},
			},
		}},
	}}}
	validationErr := &skills.SkillToolArgumentValidationError{
		SkillID:  skillID,
		ToolName: toolName,
		Issues: []tools.JSONSchemaValidationIssue{{
			Path: "connection_id", Keyword: "format", Expected: "format uuid",
		}},
	}
	args := map[string]interface{}{"connection_id": connectionID}
	result := recoverableFrozenInvocationFailure(
		nil,
		toolgovernance.FrozenInvocation{SkillID: skillID, ToolName: toolName, Arguments: args},
		args,
		"call-frozen-dynamic",
		validationErr,
		resolved,
	)
	if result == nil {
		t.Fatal("recoverableFrozenInvocationFailure() returned nil")
	}
	if result.Trace.Arguments["schema_bound_arguments_redacted"] != true {
		t.Fatalf("trace arguments = %#v, want schema-bound redaction", result.Trace.Arguments)
	}
	content, ok := result.ToolMessage.Content.(string)
	if !ok {
		t.Fatalf("tool message content = %#v, want string", result.ToolMessage.Content)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("json.Unmarshal(tool message) error = %v", err)
	}
	if payload["recoverable"] != true || payload["expected_arguments"] == nil || payload["argument_errors"] == nil {
		t.Fatalf("payload = %#v, want recoverable resolved-schema feedback", payload)
	}
	encoded, err := json.Marshal(map[string]interface{}{"trace": result.Trace, "payload": payload})
	if err != nil {
		t.Fatalf("json.Marshal(result) error = %v", err)
	}
	text := string(encoded)
	for _, forbidden := range []string{connectionID, "provider annotation"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("frozen recovery leaked %q: %s", forbidden, text)
		}
	}
}
