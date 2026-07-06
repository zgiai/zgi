package parameterextractor

import (
	"fmt"
	"reflect"
)

// Validator validates extracted parameters against their schema
type Validator struct {
	parameters []ParameterConfig
}

// NewValidator creates a new Validator instance
func NewValidator(parameters []ParameterConfig) *Validator {
	return &Validator{
		parameters: parameters,
	}
}

// Validate validates the extracted parameters against the schema
// It checks parameter count, required parameters, and type validation
func (v *Validator) Validate(result map[string]any) error {
	// Validate parameter count
	if err := v.validateParameterCount(result); err != nil {
		return err
	}

	// Validate required parameters
	if err := v.validateRequiredParameters(result); err != nil {
		return err
	}

	// Validate each parameter type
	for _, param := range v.parameters {
		value, exists := result[param.Name]
		if !exists {
			// Skip if parameter doesn't exist (already handled by required check)
			continue
		}

		if err := v.validateParameterType(param, value); err != nil {
			return err
		}
	}

	return nil
}

// validateParameterCount checks if the number of extracted parameters matches expected count
func (v *Validator) validateParameterCount(result map[string]any) error {
	expected := len(v.parameters)
	actual := len(result)

	// We allow fewer parameters if some are optional
	// But we don't allow more parameters than defined
	if actual > expected {
		return NewInvalidNumberOfParametersError(expected, actual)
	}

	return nil
}

// validateRequiredParameters checks if all required parameters are present
func (v *Validator) validateRequiredParameters(result map[string]any) error {
	for _, param := range v.parameters {
		if param.Required {
			value, exists := result[param.Name]
			if !exists {
				return NewRequiredParameterMissingError(param.Name)
			}

			// Check if value is nil or empty for required parameters
			if value == nil {
				return NewRequiredParameterMissingError(param.Name)
			}

			// For string type, check if empty
			if param.Type == ParameterTypeString {
				if strVal, ok := value.(string); ok && strVal == "" {
					return NewRequiredParameterMissingError(param.Name)
				}
			}
		}
	}

	return nil
}

// validateParameterType validates the type of a parameter value
func (v *Validator) validateParameterType(param ParameterConfig, value any) error {
	if value == nil {
		// Nil is allowed for optional parameters
		if !param.Required {
			return nil
		}
		return NewRequiredParameterMissingError(param.Name)
	}

	switch param.Type {
	case ParameterTypeString:
		return v.validateStringValue(param, value)
	case ParameterTypeNumber:
		return v.validateNumberValue(param, value)
	case ParameterTypeBool, ParameterTypeBoolean:
		return v.validateBoolValue(param, value)
	case ParameterTypeSelect:
		return v.validateSelectValue(param, value)
	case ParameterTypeArrayString:
		return v.validateArrayValue(param, value, "string")
	case ParameterTypeArrayNumber:
		return v.validateArrayValue(param, value, "number")
	case ParameterTypeArrayBool:
		return v.validateArrayValue(param, value, "bool")
	case ParameterTypeArrayObject:
		return v.validateArrayValue(param, value, "object")
	default:
		return fmt.Errorf("unknown parameter type: %s", param.Type)
	}
}

// validateStringValue validates a string parameter value
func (v *Validator) validateStringValue(param ParameterConfig, value any) error {
	switch value.(type) {
	case string:
		// Valid string
		return nil
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		// Numbers can be converted to strings
		return nil
	case bool:
		// Booleans can be converted to strings
		return nil
	default:
		return NewInvalidStringValueError(param.Name, value)
	}
}

// validateNumberValue validates a number parameter value
func (v *Validator) validateNumberValue(param ParameterConfig, value any) error {
	switch value.(type) {
	case float64, float32:
		// Valid float
		return nil
	case int, int8, int16, int32, int64:
		// Valid integer
		return nil
	case uint, uint8, uint16, uint32, uint64:
		// Valid unsigned integer
		return nil
	case string:
		// Strings that can be parsed as numbers are acceptable
		// The transformation layer will handle the conversion
		return nil
	default:
		return NewInvalidNumberValueError(param.Name, value)
	}
}

// validateBoolValue validates a boolean parameter value
func (v *Validator) validateBoolValue(param ParameterConfig, value any) error {
	switch val := value.(type) {
	case bool:
		// Valid boolean
		return nil
	case string:
		// Accept common boolean string representations
		if val == "true" || val == "false" || val == "1" || val == "0" ||
			val == "yes" || val == "no" || val == "True" || val == "False" {
			return nil
		}
		return NewInvalidBoolValueError(param.Name, value)
	case float64:
		// Accept 0 and 1 as boolean values
		if val == 0 || val == 1 {
			return nil
		}
		return NewInvalidBoolValueError(param.Name, value)
	case int:
		// Accept 0 and 1 as boolean values
		if val == 0 || val == 1 {
			return nil
		}
		return NewInvalidBoolValueError(param.Name, value)
	default:
		return NewInvalidBoolValueError(param.Name, value)
	}
}

// validateSelectValue validates a select parameter value against allowed options
func (v *Validator) validateSelectValue(param ParameterConfig, value any) error {
	// Convert value to string for comparison
	var strValue string
	switch val := value.(type) {
	case string:
		strValue = val
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		strValue = fmt.Sprintf("%v", val)
	default:
		return NewInvalidSelectValueError(param.Name, fmt.Sprintf("%v", value), param.Options)
	}

	// Check if value is in options
	for _, option := range param.Options {
		if strValue == option {
			return nil
		}
	}

	return NewInvalidSelectValueError(param.Name, strValue, param.Options)
}

// validateArrayValue validates an array parameter value and its elements
func (v *Validator) validateArrayValue(param ParameterConfig, value any, elementType string) error {
	// Check if value is a slice or array
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return NewInvalidArrayValueError(param.Name, value, elementType, "value is not an array")
	}

	// Validate each element
	for i := 0; i < rv.Len(); i++ {
		element := rv.Index(i).Interface()

		switch elementType {
		case "string":
			if err := v.validateArrayElementString(param.Name, element, i); err != nil {
				return err
			}
		case "number":
			if err := v.validateArrayElementNumber(param.Name, element, i); err != nil {
				return err
			}
		case "bool":
			if err := v.validateArrayElementBool(param.Name, element, i); err != nil {
				return err
			}
		case "object":
			if err := v.validateArrayElementObject(param.Name, element, i); err != nil {
				return err
			}
		default:
			return NewInvalidArrayValueError(param.Name, value, elementType, fmt.Sprintf("unknown element type: %s", elementType))
		}
	}

	return nil
}

func (v *Validator) validateArrayElementBool(paramName string, element any, index int) error {
	if err := v.validateBoolValue(ParameterConfig{Name: paramName, Type: ParameterTypeBool}, element); err != nil {
		return NewInvalidArrayValueError(paramName, element, "bool", fmt.Sprintf("element at index %d is not a boolean", index))
	}
	return nil
}

// validateArrayElementString validates a string array element
func (v *Validator) validateArrayElementString(paramName string, element any, index int) error {
	switch element.(type) {
	case string:
		return nil
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		// Numbers can be converted to strings
		return nil
	case bool:
		// Booleans can be converted to strings
		return nil
	default:
		return NewInvalidArrayValueError(paramName, element, "string", fmt.Sprintf("element at index %d is not a string", index))
	}
}

// validateArrayElementNumber validates a number array element
func (v *Validator) validateArrayElementNumber(paramName string, element any, index int) error {
	switch element.(type) {
	case float64, float32:
		return nil
	case int, int8, int16, int32, int64:
		return nil
	case uint, uint8, uint16, uint32, uint64:
		return nil
	case string:
		// Strings that can be parsed as numbers are acceptable
		return nil
	default:
		return NewInvalidArrayValueError(paramName, element, "number", fmt.Sprintf("element at index %d is not a number", index))
	}
}

// validateArrayElementObject validates an object array element
func (v *Validator) validateArrayElementObject(paramName string, element any, index int) error {
	// Check if element is a map or struct
	rv := reflect.ValueOf(element)
	kind := rv.Kind()

	if kind == reflect.Map || kind == reflect.Struct {
		return nil
	}

	// Also accept interface{} that contains a map
	if kind == reflect.Interface {
		innerValue := rv.Elem()
		if innerValue.IsValid() && (innerValue.Kind() == reflect.Map || innerValue.Kind() == reflect.Struct) {
			return nil
		}
	}

	return NewInvalidArrayValueError(paramName, element, "object", fmt.Sprintf("element at index %d is not an object", index))
}

// ResultTransformer transforms and normalizes extracted parameters
type ResultTransformer struct {
	parameters []ParameterConfig
}

// NewResultTransformer creates a new ResultTransformer instance
func NewResultTransformer(parameters []ParameterConfig) *ResultTransformer {
	return &ResultTransformer{
		parameters: parameters,
	}
}

// Transform transforms the extracted parameters to their correct types
// and fills in default values for missing parameters
func (rt *ResultTransformer) Transform(result map[string]any) (map[string]any, error) {
	transformed := make(map[string]any)

	for _, param := range rt.parameters {
		value, exists := result[param.Name]

		// If parameter doesn't exist, use default value
		if !exists || value == nil {
			transformed[param.Name] = rt.generateDefaultValue(param.Type)
			continue
		}

		// Transform based on type
		var transformedValue any
		var err error

		switch param.Type {
		case ParameterTypeString:
			transformedValue, err = rt.transformString(value)
		case ParameterTypeNumber:
			transformedValue, err = rt.transformNumber(value)
		case ParameterTypeBool, ParameterTypeBoolean:
			transformedValue, err = rt.transformBool(value)
		case ParameterTypeSelect:
			// Select is already validated, just convert to string
			transformedValue, err = rt.transformString(value)
		case ParameterTypeArrayString:
			transformedValue, err = rt.transformArray(value, "string")
		case ParameterTypeArrayNumber:
			transformedValue, err = rt.transformArray(value, "number")
		case ParameterTypeArrayBool:
			transformedValue, err = rt.transformArray(value, "bool")
		case ParameterTypeArrayObject:
			transformedValue, err = rt.transformArray(value, "object")
		default:
			return nil, fmt.Errorf("unknown parameter type: %s", param.Type)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to transform parameter '%s': %w", param.Name, err)
		}

		transformed[param.Name] = transformedValue
	}

	return transformed, nil
}

// transformString converts a value to string
func (rt *ResultTransformer) transformString(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case float64:
		return fmt.Sprintf("%g", v), nil
	case float32:
		return fmt.Sprintf("%g", v), nil
	case int:
		return fmt.Sprintf("%d", v), nil
	case int8:
		return fmt.Sprintf("%d", v), nil
	case int16:
		return fmt.Sprintf("%d", v), nil
	case int32:
		return fmt.Sprintf("%d", v), nil
	case int64:
		return fmt.Sprintf("%d", v), nil
	case uint:
		return fmt.Sprintf("%d", v), nil
	case uint8:
		return fmt.Sprintf("%d", v), nil
	case uint16:
		return fmt.Sprintf("%d", v), nil
	case uint32:
		return fmt.Sprintf("%d", v), nil
	case uint64:
		return fmt.Sprintf("%d", v), nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	default:
		return fmt.Sprintf("%v", value), nil
	}
}

// transformNumber converts a value to number (float64)
func (rt *ResultTransformer) transformNumber(value any) (any, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case string:
		// Try to parse string as number
		var num float64
		_, err := fmt.Sscanf(v, "%f", &num)
		if err != nil {
			return 0, fmt.Errorf("cannot parse '%s' as number: %w", v, err)
		}
		return num, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to number", value)
	}
}

// transformBool converts a value to boolean
func (rt *ResultTransformer) transformBool(value any) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		// Handle common boolean string representations
		switch v {
		case "true", "True", "TRUE", "1", "yes", "Yes", "YES":
			return true, nil
		case "false", "False", "FALSE", "0", "no", "No", "NO", "":
			return false, nil
		default:
			return false, fmt.Errorf("cannot parse '%s' as boolean", v)
		}
	case float64:
		return v != 0, nil
	case float32:
		return v != 0, nil
	case int:
		return v != 0, nil
	case int8:
		return v != 0, nil
	case int16:
		return v != 0, nil
	case int32:
		return v != 0, nil
	case int64:
		return v != 0, nil
	case uint:
		return v != 0, nil
	case uint8:
		return v != 0, nil
	case uint16:
		return v != 0, nil
	case uint32:
		return v != 0, nil
	case uint64:
		return v != 0, nil
	default:
		return false, fmt.Errorf("cannot convert %T to boolean", value)
	}
}

// transformArray converts and validates array elements
func (rt *ResultTransformer) transformArray(value any, elementType string) ([]any, error) {
	// Check if value is a slice or array
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, fmt.Errorf("value is not an array")
	}

	result := make([]any, rv.Len())

	// Transform each element
	for i := 0; i < rv.Len(); i++ {
		element := rv.Index(i).Interface()

		var transformedElement any
		var err error

		switch elementType {
		case "string":
			transformedElement, err = rt.transformString(element)
		case "number":
			transformedElement, err = rt.transformNumber(element)
		case "bool":
			transformedElement, err = rt.transformBool(element)
		case "object":
			// Objects are kept as-is (maps or structs)
			transformedElement = element
		default:
			return nil, fmt.Errorf("unknown element type: %s", elementType)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to transform element at index %d: %w", i, err)
		}

		result[i] = transformedElement
	}

	return result, nil
}

// generateDefaultValue generates a default value for a parameter type
func (rt *ResultTransformer) generateDefaultValue(paramType ParameterType) any {
	switch paramType {
	case ParameterTypeString:
		return ""
	case ParameterTypeNumber:
		return 0
	case ParameterTypeBool, ParameterTypeBoolean:
		return false
	case ParameterTypeSelect:
		return ""
	case ParameterTypeArrayString, ParameterTypeArrayNumber, ParameterTypeArrayBool, ParameterTypeArrayObject:
		return []any{}
	default:
		return nil
	}
}
