package workflow

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/pkg/logger"
)

// ValidateConversationVariables validates conversation variables according to requirements
// Requirements: 1.4, 5.1
func ValidateConversationVariables(variables []dto.Variable) error {
	if len(variables) == 0 {
		return nil // Empty array is valid
	}

	// Track variable names to check for duplicates
	nameMap := make(map[string]bool)

	for i, variable := range variables {
		// Validate required fields
		if err := validateRequiredFields(variable, i); err != nil {
			return err
		}

		// Validate value_type is supported
		if err := validateValueType(variable.ValueType, variable.Name); err != nil {
			return err
		}

		// Validate name format (letters, numbers, underscores)
		if err := validateNameFormat(variable.Name); err != nil {
			return err
		}

		// Check for duplicate names
		if nameMap[variable.Name] {
			return fmt.Errorf("duplicate conversation variable name: %s", variable.Name)
		}
		nameMap[variable.Name] = true

		logger.Debug("Validated conversation variable", map[string]interface{}{
			"name":       variable.Name,
			"value_type": variable.ValueType,
			"id":         variable.ID,
		})
	}

	return nil
}

// validateRequiredFields validates that all required fields are present
func validateRequiredFields(variable dto.Variable, index int) error {
	if variable.ID == "" {
		return fmt.Errorf("conversation variable at index %d is missing required field 'id'", index)
	}

	if variable.Name == "" {
		return fmt.Errorf("conversation variable at index %d is missing required field 'name'", index)
	}

	if variable.ValueType == "" {
		return fmt.Errorf("conversation variable '%s' is missing required field 'value_type'", variable.Name)
	}

	// Note: 'value' field can be empty/nil for some types, so we don't strictly require it
	// The executor will handle default values appropriately

	return nil
}

// validateValueType validates that the value_type is one of the supported types
func validateValueType(valueType, name string) error {
	supportedTypes := map[string]bool{
		"string":        true,
		"number":        true,
		"object":        true,
		"boolean":       true,
		"array_string":  true,
		"array_number":  true,
		"array_object":  true,
		"array_boolean": true,
	}

	if !supportedTypes[valueType] {
		return fmt.Errorf("conversation variable '%s' has unsupported value_type '%s'. Supported types: string, number, object, boolean, array_string, array_number, array_object, array_boolean", name, valueType)
	}

	return nil
}

// validateNameFormat validates that the variable name contains only letters, numbers, and underscores
func validateNameFormat(name string) error {
	// Name must not be empty (already checked in validateRequiredFields)
	if name == "" {
		return fmt.Errorf("variable name cannot be empty")
	}

	// Name should start with a letter or underscore
	if !regexp.MustCompile(`^[a-zA-Z_]`).MatchString(name) {
		return fmt.Errorf("variable name '%s' must start with a letter or underscore", name)
	}

	// Name should only contain letters, numbers, and underscores
	if !regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(name) {
		return fmt.Errorf("variable name '%s' contains invalid characters. Only letters, numbers, and underscores are allowed", name)
	}

	// Name should not be too long (reasonable limit)
	if len(name) > 100 {
		return fmt.Errorf("variable name '%s' is too long (max 100 characters)", name)
	}

	// Name should not be a reserved keyword
	reservedKeywords := []string{"sys", "environment", "conversation", "user", "system"}
	nameLower := strings.ToLower(name)
	for _, keyword := range reservedKeywords {
		if nameLower == keyword {
			return fmt.Errorf("variable name '%s' is a reserved keyword", name)
		}
	}

	return nil
}

// ValidateConversationVariableValue validates that a value matches its declared type
// This is used during runtime to ensure type safety
func ValidateConversationVariableValue(valueType string, value interface{}, name string) error {
	if value == nil {
		// Nil values are allowed for optional variables
		return nil
	}

	switch valueType {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("conversation variable '%s' expects string value, got %T", name, value)
		}

	case "number":
		switch value.(type) {
		case float64, float32, int, int64, int32:
			// Valid number types
		default:
			return fmt.Errorf("conversation variable '%s' expects number value, got %T", name, value)
		}

	case "array_string":
		switch v := value.(type) {
		case []interface{}:
			// Validate each element is a string
			for i, item := range v {
				if _, ok := item.(string); !ok {
					return fmt.Errorf("conversation variable '%s' array element at index %d is not a string", name, i)
				}
			}
		case []string:
			// Already correct type
		default:
			return fmt.Errorf("conversation variable '%s' expects array of strings, got %T", name, value)
		}

	case "array_number":
		switch v := value.(type) {
		case []interface{}:
			// Validate each element is a number
			for i, item := range v {
				switch item.(type) {
				case float64, float32, int, int64, int32:
					// Valid number types
				default:
					return fmt.Errorf("conversation variable '%s' array element at index %d is not a number", name, i)
				}
			}
		case []float64, []int:
			// Already correct type
		default:
			return fmt.Errorf("conversation variable '%s' expects array of numbers, got %T", name, value)
		}

	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return fmt.Errorf("conversation variable '%s' expects object value, got %T", name, value)
		}

	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("conversation variable '%s' expects boolean value, got %T", name, value)
		}

	case "array_object":
		switch v := value.(type) {
		case []interface{}:
			for i, item := range v {
				if _, ok := item.(map[string]interface{}); !ok {
					return fmt.Errorf("conversation variable '%s' array element at index %d is not an object", name, i)
				}
			}
		case []map[string]interface{}:
			// Already correct type
		default:
			return fmt.Errorf("conversation variable '%s' expects array of objects, got %T", name, value)
		}

	case "array_boolean":
		switch v := value.(type) {
		case []interface{}:
			for i, item := range v {
				if _, ok := item.(bool); !ok {
					return fmt.Errorf("conversation variable '%s' array element at index %d is not a boolean", name, i)
				}
			}
		case []bool:
			// Already correct type
		default:
			return fmt.Errorf("conversation variable '%s' expects array of booleans, got %T", name, value)
		}

	default:
		return fmt.Errorf("conversation variable '%s' has unsupported value_type '%s'", name, valueType)
	}

	return nil
}
