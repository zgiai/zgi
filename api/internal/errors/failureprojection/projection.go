// Package failureprojection builds user-safe failure payloads while retaining
// the original diagnostic data for durable logs and authorized debug views.
package failureprojection

import (
	"reflect"
	"strings"
)

// IsFailureStatus reports whether an execution status represents a failure
// whose diagnostic fields must be projected at a public boundary.
func IsFailureStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error", "exception":
		return true
	default:
		return false
	}
}

// ProjectPublicPayload returns a deep copy with diagnostic failure fields
// replaced by message. Terminal projections also discard outputs that could
// carry an alternate copy of the failure reason.
func ProjectPublicPayload(input map[string]interface{}, message string, terminal bool) map[string]interface{} {
	projected, _ := cloneValue(input).(map[string]interface{})
	if projected == nil {
		projected = map[string]interface{}{}
	}
	redactFields(projected, message)
	projected["error"] = message
	if _, exists := projected["message"]; exists {
		projected["message"] = message
	}
	if terminal {
		clearOutputs(projected)
	}
	return projected
}

func cloneValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			out[key] = cloneValue(item)
		}
		return out
	case []map[string]interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			cloned, _ := cloneValue(item).(map[string]interface{})
			out = append(out, cloned)
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, cloneValue(item))
		}
		return out
	}

	// Event producers may use named map and slice types (for example gin.H
	// and []gin.H). Normalize those containers so nested diagnostics cannot
	// bypass the recursive projection through their dynamic Go type.
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return nil
	}
	switch reflected.Kind() {
	case reflect.Map:
		if reflected.Type().Key().Kind() != reflect.String {
			return value
		}
		if reflected.IsNil() {
			return map[string]interface{}(nil)
		}
		out := make(map[string]interface{}, reflected.Len())
		iterator := reflected.MapRange()
		for iterator.Next() {
			out[iterator.Key().String()] = cloneValue(iterator.Value().Interface())
		}
		return out
	case reflect.Array, reflect.Slice:
		if reflected.Kind() == reflect.Slice && reflected.IsNil() {
			return []interface{}(nil)
		}
		out := make([]interface{}, 0, reflected.Len())
		for index := 0; index < reflected.Len(); index++ {
			out = append(out, cloneValue(reflected.Index(index).Interface()))
		}
		return out
	default:
		return value
	}
}

func redactFields(payload map[string]interface{}, message string) {
	for key, value := range payload {
		if detailKey(key) {
			payload[key] = message
			continue
		}
		redactValue(value, message)
	}
}

func redactValue(value interface{}, message string) {
	switch typed := value.(type) {
	case map[string]interface{}:
		redactFields(typed, message)
	case []map[string]interface{}:
		for _, item := range typed {
			redactFields(item, message)
		}
	case []interface{}:
		for _, item := range typed {
			redactValue(item, message)
		}
	}
}

func detailKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "error", "errors", "error_message", "error_detail", "error_details", "exception", "exceptions", "failure", "failure_reason", "failed_reason", "reason", "message", "detail", "details", "cause", "causes", "stack", "stack_trace", "traceback", "diagnosis_result", "__workflow_error__":
		return true
	default:
		return strings.HasSuffix(normalized, "_error") || strings.HasSuffix(normalized, "_errors")
	}
}

func clearOutputs(payload map[string]interface{}) {
	if _, exists := payload["outputs"]; exists {
		payload["outputs"] = map[string]interface{}{}
	}
	if _, exists := payload["primary_output"]; exists {
		payload["primary_output"] = ""
	}
	if _, exists := payload["output_keys"]; exists {
		payload["output_keys"] = []string{}
	}
	if _, exists := payload["process_data"]; exists {
		payload["process_data"] = map[string]interface{}{}
	}
	if _, exists := payload["execution_metadata"]; exists {
		payload["execution_metadata"] = map[string]interface{}{}
	}
}
