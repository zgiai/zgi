package gateway

import (
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func matchImageCondition(req *adapter.ImageRequest, conditions map[string]interface{}) bool {
	if len(conditions) == 0 {
		return true
	}

	for key, val := range conditions {
		switch key {
		case "size":
			if !matchStringOrArray(req.Size, val) {
				return false
			}
		case "quality":
			if !matchStringOrArray(req.Quality, val) {
				return false
			}
		case "style":
			if !matchStringOrArray(req.Style, val) {
				return false
			}
		default:
			return false
		}
	}

	return true
}

func matchStringOrArray(actual string, ruleValue interface{}) bool {
	if s, ok := ruleValue.(string); ok {
		return actual == s
	}
	if arr, ok := ruleValue.([]string); ok {
		for _, s := range arr {
			if actual == s {
				return true
			}
		}
		return false
	}
	if arr, ok := ruleValue.([]interface{}); ok {
		for _, v := range arr {
			if s, ok := v.(string); ok && actual == s {
				return true
			}
		}
		return false
	}
	return false
}
