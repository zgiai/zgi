package integrations

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	ActionValidationReasonSchemaMismatch = "action_arguments_schema_mismatch"
	ActionValidationStagePreflight       = "action_preflight"
)

// ActionInputValidationError carries only provider-schema-derived structural
// feedback. Rejected argument values, connection identifiers, credentials, and
// provider response content must never be added to this contract.
type ActionInputValidationError struct {
	integrationID  string
	actionID       string
	schemaRevision string
	issues         []tools.JSONSchemaValidationIssue
	expected       map[string]interface{}
	cause          error
}

func (e *ActionInputValidationError) Error() string {
	return ErrorCodeInvalidInput
}

func (e *ActionInputValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *ActionInputValidationError) PublicErrorCode() string {
	return ErrorCodeInvalidInput
}

// PublicErrorRecovery returns bounded, value-free guidance that a model or UI
// can use to correct a dynamic Action invocation safely.
func (e *ActionInputValidationError) PublicErrorRecovery() map[string]interface{} {
	if e == nil {
		return nil
	}
	feedback := map[string]interface{}{
		"error_code":            ErrorCodeInvalidInput,
		"reason_code":           ActionValidationReasonSchemaMismatch,
		"recovery_kind":         "action_schema",
		"failure_stage":         ActionValidationStagePreflight,
		"integration_id":        e.integrationID,
		"action_id":             e.actionID,
		"provider_request_sent": false,
		"recoverable":           true,
	}
	if e.schemaRevision != "" {
		feedback["schema_revision"] = e.schemaRevision
	}
	if len(e.issues) > 0 {
		feedback["argument_errors"] = append([]tools.JSONSchemaValidationIssue(nil), e.issues...)
	}
	if len(e.expected) > 0 {
		feedback["expected_arguments"] = map[string]interface{}{
			"integration_id": e.integrationID,
			"action_id":      e.actionID,
			"schema":         cloneJSONMap(e.expected),
		}
	}
	return feedback
}

// ValidateActionInput applies the canonical Action schema and returns a typed,
// safe recovery error. Callers should still retain validation at the final
// Executor boundary as defense in depth.
func ValidateActionInput(integrationID string, action ActionDefinition, input map[string]interface{}) error {
	if input == nil {
		input = map[string]interface{}{}
	}
	if err := tools.ValidateJSONSchemaValue(action.InputSchema, input); err != nil {
		return &ActionInputValidationError{
			integrationID:  strings.ToLower(strings.TrimSpace(integrationID)),
			actionID:       strings.ToLower(strings.TrimSpace(action.ID)),
			schemaRevision: strings.TrimSpace(action.SchemaRevision),
			issues:         tools.SafeJSONSchemaValidationIssues(action.InputSchema, err),
			expected:       tools.SafeJSONSchemaForFeedback(action.InputSchema),
			cause:          invalidInput("arguments do not match the action schema", err),
		}
	}
	return nil
}

// CanonicalizeActionInput applies only explicit, provider-owned schema
// normalization rules. Rules are limited to success-deduplication targets so
// they cannot rewrite ordinary business content. Executor invokes this before
// safety, validation and operation identity derivation.
func CanonicalizeActionInput(action ActionDefinition, input map[string]interface{}) map[string]interface{} {
	out := cloneJSONMap(input)
	if action.SuccessDeduplication == nil || len(out) == 0 {
		return out
	}
	guardedTargets := make(map[string]struct{}, len(action.SuccessDeduplication.TargetArgumentPaths))
	for _, path := range action.SuccessDeduplication.TargetArgumentPaths {
		if path = strings.TrimSpace(path); path != "" {
			guardedTargets[path] = struct{}{}
		}
	}
	properties, _ := action.InputSchema["properties"].(map[string]interface{})
	for path, rawProperty := range properties {
		if _, guarded := guardedTargets[path]; !guarded {
			continue
		}
		property, _ := rawProperty.(map[string]interface{})
		rule, _ := property["x-zgi-discard-when"].(map[string]interface{})
		whenArgument, _ := rule["argument"].(string)
		whenArgument = strings.TrimSpace(whenArgument)
		whenEquals, hasEquals := rule["equals"]
		actual, hasActual := out[whenArgument]
		if whenArgument != "" && hasEquals && hasActual &&
			actionInputDiscardRuleMatchesConditional(action.InputSchema, path, whenArgument, whenEquals) &&
			canonicalActionScalarEqual(actual, whenEquals) {
			delete(out, path)
		}
	}
	return out
}

func actionInputDiscardRuleMatchesConditional(
	schema map[string]interface{},
	path string,
	whenArgument string,
	whenEquals interface{},
) bool {
	if schemaStringListContains(schema["required"], path) {
		return false
	}
	clauses, _ := schema["allOf"].([]interface{})
	for _, rawClause := range clauses {
		clause, _ := rawClause.(map[string]interface{})
		condition, _ := clause["if"].(map[string]interface{})
		otherwise, _ := clause["else"].(map[string]interface{})
		conditionProperties, _ := condition["properties"].(map[string]interface{})
		whenProperty, _ := conditionProperties[whenArgument].(map[string]interface{})
		conditionalValue, hasConditionalValue := whenProperty["const"]
		if hasConditionalValue &&
			schemaStringListContains(condition["required"], whenArgument) &&
			schemaStringListContains(otherwise["required"], path) &&
			canonicalActionScalarEqual(conditionalValue, whenEquals) {
			return true
		}
	}
	return false
}

func schemaStringListContains(value interface{}, expected string) bool {
	switch values := value.(type) {
	case []interface{}:
		for _, value := range values {
			if text, ok := value.(string); ok && strings.TrimSpace(text) == expected {
				return true
			}
		}
	case []string:
		for _, value := range values {
			if strings.TrimSpace(value) == expected {
				return true
			}
		}
	}
	return false
}

func canonicalActionScalarEqual(left, right interface{}) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

// ActionInputValidationFeedback extracts a fresh public recovery payload from
// a possibly wrapped validation error.
func ActionInputValidationFeedback(err error) map[string]interface{} {
	var validationErr *ActionInputValidationError
	if !errors.As(err, &validationErr) || validationErr == nil {
		return nil
	}
	return validationErr.PublicErrorRecovery()
}
