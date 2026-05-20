package httprequest

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

type defaultValueEntry struct {
	Key       string
	Value     any
	ValueType shared.DefaultValueType
}

func parseErrorStrategy(raw any) shared.ErrorStrategy {
	strategy, ok := raw.(string)
	if !ok {
		return ""
	}
	switch shared.ErrorStrategy(strategy) {
	case shared.FailBranch, shared.DefaultVal:
		return shared.ErrorStrategy(strategy)
	default:
		return ""
	}
}

func parseRetryConfig(raw any) shared.RetryConfig {
	cfg := shared.RetryConfig{}
	data, ok := raw.(map[string]interface{})
	if !ok {
		return cfg
	}

	if value, ok := data["max_times"]; ok {
		cfg.MaxTimes = intFromAny(value)
	}
	if value, ok := data["max_retries"]; ok {
		cfg.MaxTimes = intFromAny(value)
	}
	if value, ok := data["interval"]; ok {
		cfg.Interval = intFromAny(value)
	}
	if value, ok := data["retry_interval"]; ok {
		cfg.Interval = intFromAny(value)
	}
	if value, ok := data["enable"]; ok {
		cfg.Enable = boolFromAny(value)
	}
	if value, ok := data["retry_enabled"]; ok {
		cfg.Enable = boolFromAny(value)
	}

	return cfg
}

func parseDefaultValueEntries(raw any) ([]defaultValueEntry, []shared.DefaultValue) {
	items, ok := raw.([]any)
	if !ok {
		return nil, nil
	}

	entries := make([]defaultValueEntry, 0, len(items))
	defaults := make([]shared.DefaultValue, 0, len(items))

	for _, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}

		key := firstString(itemMap, "key", "name")
		if key == "" {
			continue
		}

		valueTypeStr := firstString(itemMap, "type")
		if valueTypeStr == "" {
			continue
		}

		value := itemMap["value"]
		valueType := shared.DefaultValueType(valueTypeStr)

		entries = append(entries, defaultValueEntry{
			Key:       key,
			Value:     value,
			ValueType: valueType,
		})

		defaults = append(defaults, shared.DefaultValue{
			Key:   key,
			Value: stringifyValue(value),
			Type:  valueType,
		})
	}

	return entries, defaults
}

func buildErrorOutputs(strategy shared.ErrorStrategy, defaults []defaultValueEntry, errMsg, errType string) map[string]any {
	outputs := make(map[string]any)

	if strategy == shared.DefaultVal {
		for _, entry := range defaults {
			if entry.Key == "" {
				continue
			}
			outputs[entry.Key] = normalizeDefaultValue(entry)
		}
	}

	outputs["error_message"] = errMsg
	outputs["error_type"] = errType

	return outputs
}

func normalizeDefaultValue(entry defaultValueEntry) any {
	switch entry.ValueType {
	case shared.TypeString:
		return toString(entry.Value)
	case shared.TypeNumber:
		return toNumber(entry.Value)
	case shared.TypeObject:
		return toObject(entry.Value)
	case shared.TypeArrayNumber:
		return toArray(entry.Value, shared.TypeNumber)
	case shared.TypeArrayString:
		return toArray(entry.Value, shared.TypeString)
	case shared.TypeArrayObject:
		return toArray(entry.Value, shared.TypeObject)
	case shared.TypeArrayFiles:
		return toArray(entry.Value, shared.TypeArrayFiles)
	default:
		return entry.Value
	}
}

func toString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", value)
	}
}

func toNumber(value any) any {
	switch v := value.(type) {
	case int:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case float32:
		return float64(v)
	case float64:
		return v
	case json.Number:
		if num, err := v.Float64(); err == nil {
			return num
		}
	case string:
		if v == "" {
			return float64(0)
		}
		if num, err := strconv.ParseFloat(v, 64); err == nil {
			return num
		}
	}
	return value
}

func toObject(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return v
	case string:
		return parseJSONValue(v)
	case []byte:
		return parseJSONValue(string(v))
	default:
		return value
	}
}

func toArray(value any, elementType shared.DefaultValueType) any {
	switch v := value.(type) {
	case []any:
		return normalizeArray(v, elementType)
	case string:
		parsed := parseJSONValue(v)
		if list, ok := parsed.([]any); ok {
			return normalizeArray(list, elementType)
		}
		if list, ok := parsed.([]interface{}); ok {
			return normalizeArray(list, elementType)
		}
		return parsed
	case []byte:
		parsed := parseJSONValue(string(v))
		if list, ok := parsed.([]any); ok {
			return normalizeArray(list, elementType)
		}
		if list, ok := parsed.([]interface{}); ok {
			return normalizeArray(list, elementType)
		}
		return parsed
	default:
		return value
	}
}

func normalizeArray(values []any, elementType shared.DefaultValueType) []any {
	normalized := make([]any, 0, len(values))
	for _, item := range values {
		switch elementType {
		case shared.TypeString:
			normalized = append(normalized, toString(item))
		case shared.TypeNumber:
			normalized = append(normalized, toNumber(item))
		case shared.TypeObject:
			normalized = append(normalized, toObject(item))
		default:
			normalized = append(normalized, item)
		}
	}
	return normalized
}

func parseJSONValue(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}

	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return raw
	}
	return decoded
}

func stringifyValue(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		if data, err := json.Marshal(value); err == nil {
			return string(data)
		}
		return fmt.Sprintf("%v", value)
	}
}

func firstString(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if raw, ok := m[key]; ok {
			if str, ok := raw.(string); ok {
				return str
			}
		}
	}
	return ""
}

func intFromAny(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float32:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if num, err := v.Int64(); err == nil {
			return int(num)
		}
		if num, err := v.Float64(); err == nil {
			return int(num)
		}
	case string:
		if v == "" {
			return 0
		}
		if num, err := strconv.Atoi(v); err == nil {
			return num
		}
		if num, err := strconv.ParseFloat(v, 64); err == nil {
			return int(num)
		}
	}
	return 0
}

func boolFromAny(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case int64:
		return v != 0
	case float32:
		return v != 0
	case float64:
		return v != 0
	case json.Number:
		if num, err := v.Int64(); err == nil {
			return num != 0
		}
		if num, err := v.Float64(); err == nil {
			return num != 0
		}
	case string:
		trimmed := strings.ToLower(strings.TrimSpace(v))
		if trimmed == "" {
			return false
		}
		if trimmed == "true" || trimmed == "1" || trimmed == "yes" {
			return true
		}
		if trimmed == "false" || trimmed == "0" || trimmed == "no" {
			return false
		}
	}
	return false
}
