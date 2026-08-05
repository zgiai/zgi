package skills

import (
	"strings"
	"testing"
)

type recoverableActionSchemaError struct{}

func (recoverableActionSchemaError) Error() string { return "integration_invalid_input" }

func (recoverableActionSchemaError) PublicErrorRecovery() map[string]interface{} {
	return map[string]interface{}{
		"recovery_kind":  "action_schema",
		"integration_id": "feishu",
		"action_id":      "feishu.contact.search",
		"expected_arguments": map[string]interface{}{
			"schema": map[string]interface{}{"type": "object"},
		},
	}
}

func TestPublicToolErrorRecoveryForExternalAppsAddsMetaToolInstructions(t *testing.T) {
	recovery := PublicToolErrorRecoveryForInvocation(
		recoverableActionSchemaError{},
		SkillExternalApps,
		"execute_action",
		nil,
	)
	if recovery["recovery_action"] != "get_action_guide" {
		t.Fatalf("external-apps recovery = %#v", recovery)
	}
	retryInstruction, _ := recovery["retry_action"].(string)
	if !strings.Contains(retryInstruction, "execute_action") {
		t.Fatalf("external-apps retry instruction = %q", retryInstruction)
	}
	expected, _ := recovery["expected_arguments"].(map[string]interface{})
	if expected["tool_name"] != "execute_action" || expected["arguments_path"] != "arguments" {
		t.Fatalf("external-apps expected arguments = %#v", expected)
	}
}

func TestPublicToolErrorRecoveryForDirectToolStaysOnCurrentSurface(t *testing.T) {
	directExpected := map[string]interface{}{
		"skill_id":  "file-reader",
		"tool_name": "read_file",
		"schema":    map[string]interface{}{"type": "object"},
	}
	recovery := PublicToolErrorRecoveryForInvocation(
		recoverableActionSchemaError{},
		"file-reader",
		"read_file",
		directExpected,
	)
	if recovery["recovery_action"] != "retry_current_tool" {
		t.Fatalf("direct-tool recovery = %#v", recovery)
	}
	retryInstruction, _ := recovery["retry_action"].(string)
	if !strings.Contains(retryInstruction, "call_skill_tool") {
		t.Fatalf("direct-tool retry instruction = %q", retryInstruction)
	}
	expected, _ := recovery["expected_arguments"].(map[string]interface{})
	if expected["skill_id"] != "file-reader" || expected["tool_name"] != "read_file" {
		t.Fatalf("direct-tool expected arguments = %#v", expected)
	}
}
