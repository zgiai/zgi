package dispatch

// InputWithDefaults copies business input and materializes optional defaults
// from the authorized operation schema before hashing, freezing or dispatch.
// Required/read-only fields and conditional branches cannot supply defaults.
func InputWithDefaults(schema, input map[string]any) map[string]any {
	return inputObjectDefaults(schema, input, 0)
}

func inputObjectDefaults(schema, input map[string]any, depth int) map[string]any {
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	if depth >= 16 {
		return result
	}
	properties, _ := schema["properties"].(map[string]any)
	required := map[string]bool{}
	switch fields := schema["required"].(type) {
	case []any:
		for _, field := range fields {
			if key, ok := field.(string); ok {
				required[key] = true
			}
		}
	case []string:
		for _, field := range fields {
			required[field] = true
		}
	}
	for key, raw := range properties {
		property, _ := raw.(map[string]any)
		if property == nil || property["readOnly"] == true {
			continue
		}
		value, present := result[key]
		if !present && !required[key] {
			value, present = property["default"]
		}
		if !present {
			continue
		}
		result[key] = inputValueDefaults(property, value, depth+1)
	}
	return result
}

func inputValueDefaults(schema map[string]any, value any, depth int) any {
	if depth >= 16 {
		return value
	}
	switch typed := value.(type) {
	case map[string]any:
		return inputObjectDefaults(schema, typed, depth)
	case []any:
		items, _ := schema["items"].(map[string]any)
		result := make([]any, len(typed))
		for index, entry := range typed {
			result[index] = inputValueDefaults(items, entry, depth+1)
		}
		return result
	default:
		return value
	}
}
