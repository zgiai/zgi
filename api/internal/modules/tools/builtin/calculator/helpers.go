package calculator

import (
	"fmt"
	"math"
	"strings"
)

const (
	defaultPrecision = 6
	maxPrecision     = 12
)

func stringParam(params map[string]interface{}, key string) string {
	return strings.ToLower(rawStringParam(params, key))
}

func rawStringParam(params map[string]interface{}, key string) string {
	if params == nil {
		return ""
	}
	value, ok := params[key]
	if !ok || value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func numberParam(params map[string]interface{}, key string) (float64, error) {
	if params == nil {
		return 0, fmt.Errorf("%s is required", key)
	}
	value, ok := params[key]
	if !ok || value == nil {
		return 0, fmt.Errorf("%s is required", key)
	}
	number, ok := toFloat64(value)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("%s must be a finite number", key)
	}
	return number, nil
}

func precisionParam(params map[string]interface{}) (int, error) {
	if params == nil {
		return defaultPrecision, nil
	}
	value, ok := params["precision"]
	if !ok || value == nil {
		return defaultPrecision, nil
	}
	number, ok := toFloat64(value)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number {
		return 0, fmt.Errorf("precision must be an integer between 0 and %d", maxPrecision)
	}
	precision := int(number)
	if precision < 0 || precision > maxPrecision {
		return 0, fmt.Errorf("precision must be between 0 and %d", maxPrecision)
	}
	return precision, nil
}

func toFloat64(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func roundValue(value float64, precision int) (float64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("result is not a finite number")
	}
	factor := math.Pow10(precision)
	return math.Round(value*factor) / factor, nil
}
