package metatools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

const (
	maxExecuteActionArgumentsJSONBytes  = 1 << 20
	maxExecuteActionArgumentsJSONDepth  = 64
	maxExecuteActionArgumentsJSONFields = 4096

	executeActionArgumentsEncodingReason = "execute_action_arguments_encoding_invalid"
)

// normalizeExecuteActionParameters accepts the native object published by the
// execute_action schema and one narrowly defined compatibility representation:
// a string containing exactly one JSON object. This repairs model-side double
// serialization without guessing field names, values, or action semantics.
//
// The normalized object must still pass the selected Action's authoritative
// JSON Schema, authorization, governance, and safety checks later in the
// execution pipeline.
func normalizeExecuteActionParameters(parameters map[string]interface{}) (map[string]interface{}, error) {
	out := cloneMap(parameters)
	raw, exists := out["arguments"]
	if !exists {
		return out, nil
	}

	var arguments map[string]interface{}
	switch typed := raw.(type) {
	case map[string]interface{}:
		if typed == nil {
			return nil, newExecuteActionArgumentsEncodingError("null", nil)
		}
		arguments = cloneMap(typed)
	case string:
		decoded, err := decodeExecuteActionArgumentsObject(typed)
		if err != nil {
			return nil, err
		}
		arguments = decoded
	default:
		return nil, newExecuteActionArgumentsEncodingError(jsonValueKind(raw), nil)
	}

	if err := validateExecuteActionArgumentsBounds(arguments); err != nil {
		return nil, err
	}
	out["arguments"] = arguments
	return out, nil
}

// canonicalizeExecuteActionBusinessArguments removes target values that the
// selected, authoritative Action contract explicitly marks as semantically
// irrelevant. Core limits rules to targets protected by success
// deduplication; arbitrary optional arguments are never rewritten.
func canonicalizeExecuteActionBusinessArguments(
	parameters map[string]interface{},
	action integrations.ActionDefinition,
) map[string]interface{} {
	out := cloneMap(parameters)
	if arguments, ok := out["arguments"].(map[string]interface{}); ok && arguments != nil {
		out["arguments"] = integrations.CanonicalizeActionInput(action, arguments)
	}
	switch items := out["batch_items"].(type) {
	case []interface{}:
		canonical := make([]interface{}, len(items))
		for index, raw := range items {
			if item, ok := raw.(map[string]interface{}); ok && item != nil {
				canonical[index] = integrations.CanonicalizeActionInput(action, item)
			} else {
				canonical[index] = raw
			}
		}
		out["batch_items"] = canonical
	case []map[string]interface{}:
		canonical := make([]map[string]interface{}, len(items))
		for index, item := range items {
			canonical[index] = integrations.CanonicalizeActionInput(action, item)
		}
		out["batch_items"] = canonical
	}
	return out
}

func decodeExecuteActionArgumentsObject(value string) (map[string]interface{}, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, newExecuteActionArgumentsEncodingError("string", fmt.Errorf("arguments JSON is empty"))
	}
	if len([]byte(trimmed)) > maxExecuteActionArgumentsJSONBytes {
		return nil, newExecuteActionArgumentsEncodingError("string", fmt.Errorf("arguments JSON exceeds %d bytes", maxExecuteActionArgumentsJSONBytes))
	}

	decoder := json.NewDecoder(bytes.NewBufferString(trimmed))
	var decoded interface{}
	if err := decoder.Decode(&decoded); err != nil {
		return nil, newExecuteActionArgumentsEncodingError("string", fmt.Errorf("decode arguments JSON: %w", err))
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, newExecuteActionArgumentsEncodingError("string", fmt.Errorf("arguments string contains trailing JSON data"))
	}
	arguments, ok := decoded.(map[string]interface{})
	if !ok || arguments == nil {
		return nil, newExecuteActionArgumentsEncodingError(jsonValueKind(decoded), nil)
	}
	return arguments, nil
}

func validateExecuteActionArgumentsBounds(arguments map[string]interface{}) error {
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return newExecuteActionArgumentsEncodingError("object", fmt.Errorf("encode arguments JSON: %w", err))
	}
	if len(encoded) > maxExecuteActionArgumentsJSONBytes {
		return newExecuteActionArgumentsEncodingError("object", fmt.Errorf("arguments JSON exceeds %d bytes", maxExecuteActionArgumentsJSONBytes))
	}

	// Measure a decoded JSON tree so recursion is cycle-free even if a direct
	// internal caller supplied a cyclic Go value.
	var decoded interface{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return newExecuteActionArgumentsEncodingError("object", fmt.Errorf("decode bounded arguments JSON: %w", err))
	}
	fields := 0
	if !measureExecuteActionJSON(decoded, 1, &fields) {
		return newExecuteActionArgumentsEncodingError("object", fmt.Errorf("arguments JSON exceeds depth or field limits"))
	}
	return nil
}

func measureExecuteActionJSON(value interface{}, depth int, fields *int) bool {
	if depth > maxExecuteActionArgumentsJSONDepth {
		return false
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		*fields += len(typed)
		if *fields > maxExecuteActionArgumentsJSONFields {
			return false
		}
		for _, nested := range typed {
			if !measureExecuteActionJSON(nested, depth+1, fields) {
				return false
			}
		}
	case []interface{}:
		for _, nested := range typed {
			if !measureExecuteActionJSON(nested, depth+1, fields) {
				return false
			}
		}
	}
	return true
}

func jsonValueKind(value interface{}) string {
	switch value.(type) {
	case nil:
		return "null"
	case map[string]interface{}:
		return "object"
	case []interface{}:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return "number"
	default:
		return "unknown"
	}
}

type executeActionArgumentsEncodingError struct {
	actualType string
	cause      error
}

func newExecuteActionArgumentsEncodingError(actualType string, cause error) error {
	return &executeActionArgumentsEncodingError{
		actualType: strings.TrimSpace(actualType),
		cause: integrations.NewError(
			integrations.ErrorCodeInvalidInput,
			"execute_action arguments use an invalid encoding",
			cause,
		),
	}
}

func (e *executeActionArgumentsEncodingError) Error() string {
	return integrations.ErrorCodeInvalidInput
}

func (e *executeActionArgumentsEncodingError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *executeActionArgumentsEncodingError) PublicErrorCode() string {
	return integrations.ErrorCodeInvalidInput
}

func (e *executeActionArgumentsEncodingError) PublicErrorRecovery() map[string]interface{} {
	if e == nil {
		return nil
	}
	actualType := strings.TrimSpace(e.actualType)
	if actualType == "" {
		actualType = "unknown"
	}
	return map[string]interface{}{
		"error_code":            integrations.ErrorCodeInvalidInput,
		"reason_code":           executeActionArgumentsEncodingReason,
		"recovery_kind":         "arguments_encoding",
		"failure_stage":         "action_preflight",
		"provider_request_sent": false,
		"recoverable":           true,
		"invalid_fields":        []string{"arguments"},
		"expected_type":         "object",
		"actual_type":           actualType,
		"recovery_action":       "get_action_guide",
		"retry_action":          "Call get_action_guide, then retry once with arguments as a native JSON object. Do not quote or serialize the object.",
	}
}
