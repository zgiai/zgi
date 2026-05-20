package entities

import (
	"fmt"
	"strconv"
	"strings"
)

// ValueConversionMode controls strictness when converting values.
type ValueConversionMode int

const (
	ValueConversionStrict ValueConversionMode = iota
	ValueConversionLenient
)

// ConvertValue converts a value into a Segment based on the provided valueType.
func ConvertValue(valueType string, value any, mode ValueConversionMode) (Segment, []string, error) {
	if valueType == "" {
		return nil, nil, fmt.Errorf("value_type is required")
	}

	normalized := strings.ToLower(strings.TrimSpace(valueType))
	if mode == ValueConversionStrict && value == nil {
		return nil, nil, fmt.Errorf("value is required for type %s", normalized)
	}

	switch normalized {
	case "string":
		return convertString(value, mode)
	case "number":
		return convertNumber(value, mode)
	case "object":
		return convertObject(value, mode)
	case "boolean":
		return convertBoolean(value, mode)
	case "array_string":
		return convertArrayString(value, mode)
	case "array_number":
		return convertArrayNumber(value, mode)
	case "array_object":
		return convertArrayObject(value, mode)
	case "array_boolean":
		return convertArrayBoolean(value, mode)
	case "secret":
		return convertSecret(value, mode)
	default:
		if mode == ValueConversionStrict {
			return nil, nil, fmt.Errorf("unsupported value_type %s", normalized)
		}
		return &StringSegment{Value: fmt.Sprintf("%v", value)}, []string{fmt.Sprintf("unsupported value_type %s, defaulted to string", normalized)}, nil
	}
}

func convertString(value any, mode ValueConversionMode) (Segment, []string, error) {
	if str, ok := value.(string); ok {
		return &StringSegment{Value: str}, nil, nil
	}
	if mode == ValueConversionStrict {
		return nil, nil, fmt.Errorf("expected string value")
	}
	return &StringSegment{Value: fmt.Sprintf("%v", value)}, []string{"converted non-string to string"}, nil
}

func convertSecret(value any, mode ValueConversionMode) (Segment, []string, error) {
	if str, ok := value.(string); ok {
		return &SecretSegment{Value: str}, nil, nil
	}
	if mode == ValueConversionStrict {
		return nil, nil, fmt.Errorf("expected secret value")
	}
	return &SecretSegment{Value: fmt.Sprintf("%v", value)}, []string{"converted non-string to secret"}, nil
}

func convertNumber(value any, mode ValueConversionMode) (Segment, []string, error) {
	if value == nil && mode == ValueConversionLenient {
		return &NumberSegment{Value: 0}, []string{"defaulted nil to 0"}, nil
	}

	if num, ok := numberToFloat(value); ok {
		return &NumberSegment{Value: num}, nil, nil
	}

	if mode == ValueConversionStrict {
		return nil, nil, fmt.Errorf("expected number value")
	}
	return &NumberSegment{Value: 0}, []string{"invalid number value, defaulted to 0"}, nil
}

func convertObject(value any, mode ValueConversionMode) (Segment, []string, error) {
	if obj, ok := value.(map[string]interface{}); ok {
		return &ObjectSegment{Value: obj}, nil, nil
	}
	if mode == ValueConversionStrict {
		return nil, nil, fmt.Errorf("expected object value")
	}
	return &ObjectSegment{Value: map[string]interface{}{}}, []string{"invalid object value, defaulted to empty object"}, nil
}

func convertBoolean(value any, mode ValueConversionMode) (Segment, []string, error) {
	if value == nil && mode == ValueConversionLenient {
		return &BooleanSegment{Value: false}, []string{"defaulted nil to false"}, nil
	}
	if b, ok := value.(bool); ok {
		return &BooleanSegment{Value: b}, nil, nil
	}
	if mode == ValueConversionLenient {
		if str, ok := value.(string); ok {
			parsed, err := strconv.ParseBool(strings.TrimSpace(str))
			if err == nil {
				return &BooleanSegment{Value: parsed}, []string{"parsed string to boolean"}, nil
			}
		}
		return &BooleanSegment{Value: false}, []string{"invalid boolean value, defaulted to false"}, nil
	}
	return nil, nil, fmt.Errorf("expected boolean value")
}

func convertArrayString(value any, mode ValueConversionMode) (Segment, []string, error) {
	if arr, ok := value.([]string); ok {
		return &ArrayStringSegment{Value: arr}, nil, nil
	}

	if arr, ok := value.([]interface{}); ok {
		result := make([]string, 0, len(arr))
		warnings := make([]string, 0)
		for _, item := range arr {
			if str, ok := item.(string); ok {
				result = append(result, str)
				continue
			}
			if mode == ValueConversionStrict {
				return nil, nil, fmt.Errorf("array_string contains non-string item")
			}
			result = append(result, fmt.Sprintf("%v", item))
			warnings = append(warnings, "converted non-string array element to string")
		}
		return &ArrayStringSegment{Value: result}, warnings, nil
	}

	if mode == ValueConversionStrict {
		return nil, nil, fmt.Errorf("expected array_string value")
	}
	return &ArrayStringSegment{Value: []string{}}, []string{"invalid array_string value, defaulted to empty array"}, nil
}

func convertArrayNumber(value any, mode ValueConversionMode) (Segment, []string, error) {
	if arr, ok := value.([]float64); ok {
		return &ArrayNumberSegment{Value: arr}, nil, nil
	}
	if arr, ok := value.([]int); ok {
		result := make([]float64, 0, len(arr))
		for _, item := range arr {
			result = append(result, float64(item))
		}
		return &ArrayNumberSegment{Value: result}, nil, nil
	}

	if arr, ok := value.([]interface{}); ok {
		result := make([]float64, 0, len(arr))
		warnings := make([]string, 0)
		for _, item := range arr {
			if num, ok := numberToFloat(item); ok {
				result = append(result, num)
				continue
			}
			if mode == ValueConversionStrict {
				return nil, nil, fmt.Errorf("array_number contains non-number item")
			}
			warnings = append(warnings, "skipped non-number array element")
		}
		return &ArrayNumberSegment{Value: result}, warnings, nil
	}

	if mode == ValueConversionStrict {
		return nil, nil, fmt.Errorf("expected array_number value")
	}
	return &ArrayNumberSegment{Value: []float64{}}, []string{"invalid array_number value, defaulted to empty array"}, nil
}

func convertArrayObject(value any, mode ValueConversionMode) (Segment, []string, error) {
	if arr, ok := value.([]map[string]interface{}); ok {
		return &ArrayObjectSegment{Value: arr}, nil, nil
	}

	if arr, ok := value.([]interface{}); ok {
		result := make([]map[string]interface{}, 0, len(arr))
		warnings := make([]string, 0)
		for _, item := range arr {
			obj, ok := item.(map[string]interface{})
			if ok {
				result = append(result, obj)
				continue
			}
			if mode == ValueConversionStrict {
				return nil, nil, fmt.Errorf("array_object contains non-object item")
			}
			warnings = append(warnings, "skipped non-object array element")
		}
		return &ArrayObjectSegment{Value: result}, warnings, nil
	}

	if mode == ValueConversionStrict {
		return nil, nil, fmt.Errorf("expected array_object value")
	}
	return &ArrayObjectSegment{Value: []map[string]interface{}{}}, []string{"invalid array_object value, defaulted to empty array"}, nil
}

func convertArrayBoolean(value any, mode ValueConversionMode) (Segment, []string, error) {
	if arr, ok := value.([]bool); ok {
		return &ArrayBooleanSegment{Value: arr}, nil, nil
	}

	if arr, ok := value.([]interface{}); ok {
		result := make([]bool, 0, len(arr))
		warnings := make([]string, 0)
		for _, item := range arr {
			if b, ok := item.(bool); ok {
				result = append(result, b)
				continue
			}
			if mode == ValueConversionLenient {
				if str, ok := item.(string); ok {
					parsed, err := strconv.ParseBool(strings.TrimSpace(str))
					if err == nil {
						result = append(result, parsed)
						warnings = append(warnings, "parsed string array element to boolean")
						continue
					}
				}
				warnings = append(warnings, "skipped non-boolean array element")
				continue
			}
			return nil, nil, fmt.Errorf("array_boolean contains non-boolean item")
		}
		return &ArrayBooleanSegment{Value: result}, warnings, nil
	}

	if mode == ValueConversionStrict {
		return nil, nil, fmt.Errorf("expected array_boolean value")
	}
	return &ArrayBooleanSegment{Value: []bool{}}, []string{"invalid array_boolean value, defaulted to empty array"}, nil
}

func numberToFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case int16:
		return float64(v), true
	case int8:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint64:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint8:
		return float64(v), true
	default:
		return 0, false
	}
}
