package skills

import (
	"errors"
	"strings"
)

type publicErrorCoder interface {
	PublicErrorCode() string
}

type publicErrorRecoveryProvider interface {
	PublicErrorRecovery() map[string]interface{}
}

// Keep provider details out of traces while preserving stable integration codes.
func skillTraceError(err error) (message string, code string) {
	if err == nil {
		return "", ""
	}
	var coded publicErrorCoder
	if errors.As(err, &coded) {
		code = strings.TrimSpace(coded.PublicErrorCode())
		if validIntegrationErrorCode(code) {
			return code, code
		}
	}
	return err.Error(), ""
}

func validIntegrationErrorCode(code string) bool {
	if !strings.HasPrefix(code, "integration_") || len(code) > 80 {
		return false
	}
	for _, character := range code {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' {
			continue
		}
		return false
	}
	return true
}

// PublicToolErrorRecovery extracts a provider-owned, value-free recovery
// payload. Implementations are responsible for returning a fresh map containing
// only stable structural metadata safe for model and UI feedback.
func PublicToolErrorRecovery(err error) map[string]interface{} {
	if err == nil {
		return nil
	}
	var provider publicErrorRecoveryProvider
	if !errors.As(err, &provider) || provider == nil {
		return nil
	}
	return provider.PublicErrorRecovery()
}

// PublicToolErrorRecoveryForInvocation translates provider-owned structural
// feedback into instructions for the current tool surface. Provider errors
// remain unaware of Skill names, meta tools, and model protocol details.
func PublicToolErrorRecoveryForInvocation(
	err error,
	skillID string,
	toolName string,
	expected map[string]interface{},
) map[string]interface{} {
	recovery := PublicToolErrorRecovery(err)
	if len(recovery) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(recovery)+2)
	for key, value := range recovery {
		out[key] = value
	}
	if strings.TrimSpace(recoveryString(out["recovery_kind"])) != "action_schema" {
		return out
	}
	actionExpected, _ := out["expected_arguments"].(map[string]interface{})
	if strings.EqualFold(strings.TrimSpace(skillID), SkillExternalApps) &&
		strings.EqualFold(strings.TrimSpace(toolName), "execute_action") {
		wrapped := map[string]interface{}{
			"tool_name":      "execute_action",
			"integration_id": recoveryString(out["integration_id"]),
			"action_id":      recoveryString(out["action_id"]),
			"arguments_path": "arguments",
		}
		if schema, ok := actionExpected["schema"]; ok {
			wrapped["schema"] = schema
		}
		out["expected_arguments"] = wrapped
		out["recovery_action"] = "get_action_guide"
		out["retry_action"] = "Call get_action_guide for this integration and action, then retry execute_action once with corrected arguments."
		return out
	}
	if len(expected) > 0 {
		out["expected_arguments"] = expected
	}
	out["recovery_action"] = "retry_current_tool"
	out["retry_action"] = "Retry call_skill_tool once for the current skill and tool with arguments matching expected_arguments.schema."
	return out
}

func recoveryString(value interface{}) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
