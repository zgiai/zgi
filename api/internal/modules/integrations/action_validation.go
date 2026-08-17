package integrations

import (
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

// ActionInputValidationFeedback extracts a fresh public recovery payload from
// a possibly wrapped validation error.
func ActionInputValidationFeedback(err error) map[string]interface{} {
	var validationErr *ActionInputValidationError
	if !errors.As(err, &validationErr) || validationErr == nil {
		return nil
	}
	return validationErr.PublicErrorRecovery()
}
