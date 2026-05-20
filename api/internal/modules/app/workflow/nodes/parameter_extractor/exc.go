package parameterextractor

import "fmt"

// ParameterExtractorError is the base error type for Parameter Extractor Node errors
type ParameterExtractorError struct {
	message string
	code    string
}

func (e *ParameterExtractorError) Error() string {
	return e.message
}

func (e *ParameterExtractorError) Code() string {
	return e.code
}

// NewParameterExtractorError creates a new ParameterExtractorError
func NewParameterExtractorError(code, message string) *ParameterExtractorError {
	return &ParameterExtractorError{
		code:    code,
		message: message,
	}
}

// InvalidModelTypeError is raised when the model type is not LLM
type InvalidModelTypeError struct {
	*ParameterExtractorError
	ModelType string
}

// NewInvalidModelTypeError creates a new InvalidModelTypeError
func NewInvalidModelTypeError(modelType string) *InvalidModelTypeError {
	message := fmt.Sprintf("Invalid model type: %s. Only LLM models are supported for parameter extraction.", modelType)
	return &InvalidModelTypeError{
		ParameterExtractorError: NewParameterExtractorError("INVALID_MODEL_TYPE", message),
		ModelType:               modelType,
	}
}

// ModelSchemaNotFoundError is raised when the model schema cannot be found
type ModelSchemaNotFoundError struct {
	*ParameterExtractorError
	ModelName string
}

// NewModelSchemaNotFoundError creates a new ModelSchemaNotFoundError
func NewModelSchemaNotFoundError(modelName string) *ModelSchemaNotFoundError {
	message := fmt.Sprintf("Model schema not found for model: %s", modelName)
	return &ModelSchemaNotFoundError{
		ParameterExtractorError: NewParameterExtractorError("MODEL_SCHEMA_NOT_FOUND", message),
		ModelName:               modelName,
	}
}

// InvalidInvokeResultError is raised when the LLM invocation result is invalid or empty
type InvalidInvokeResultError struct {
	*ParameterExtractorError
	Reason string
}

// NewInvalidInvokeResultError creates a new InvalidInvokeResultError
func NewInvalidInvokeResultError(reason string) *InvalidInvokeResultError {
	message := fmt.Sprintf("Invalid LLM invocation result: %s", reason)
	return &InvalidInvokeResultError{
		ParameterExtractorError: NewParameterExtractorError("INVALID_INVOKE_RESULT", message),
		Reason:                  reason,
	}
}

// InvalidNumberOfParametersError is raised when the number of extracted parameters doesn't match expected count
type InvalidNumberOfParametersError struct {
	*ParameterExtractorError
	Expected int
	Actual   int
}

// NewInvalidNumberOfParametersError creates a new InvalidNumberOfParametersError
func NewInvalidNumberOfParametersError(expected, actual int) *InvalidNumberOfParametersError {
	message := fmt.Sprintf("Invalid number of parameters: expected %d, got %d", expected, actual)
	return &InvalidNumberOfParametersError{
		ParameterExtractorError: NewParameterExtractorError("INVALID_NUMBER_OF_PARAMETERS", message),
		Expected:                expected,
		Actual:                  actual,
	}
}

// RequiredParameterMissingError is raised when a required parameter is missing from extraction results
type RequiredParameterMissingError struct {
	*ParameterExtractorError
	ParameterName string
}

// NewRequiredParameterMissingError creates a new RequiredParameterMissingError
func NewRequiredParameterMissingError(parameterName string) *RequiredParameterMissingError {
	message := fmt.Sprintf("Required parameter '%s' is missing from extraction results", parameterName)
	return &RequiredParameterMissingError{
		ParameterExtractorError: NewParameterExtractorError("REQUIRED_PARAMETER_MISSING", message),
		ParameterName:           parameterName,
	}
}

// InvalidSelectValueError is raised when a select parameter value is not in the allowed options
type InvalidSelectValueError struct {
	*ParameterExtractorError
	ParameterName string
	Value         string
	Options       []string
}

// NewInvalidSelectValueError creates a new InvalidSelectValueError
func NewInvalidSelectValueError(parameterName, value string, options []string) *InvalidSelectValueError {
	message := fmt.Sprintf("Invalid select value for parameter '%s': '%s' is not in allowed options %v", parameterName, value, options)
	return &InvalidSelectValueError{
		ParameterExtractorError: NewParameterExtractorError("INVALID_SELECT_VALUE", message),
		ParameterName:           parameterName,
		Value:                   value,
		Options:                 options,
	}
}

// InvalidNumberValueError is raised when a number parameter value cannot be converted to a number
type InvalidNumberValueError struct {
	*ParameterExtractorError
	ParameterName string
	Value         interface{}
}

// NewInvalidNumberValueError creates a new InvalidNumberValueError
func NewInvalidNumberValueError(parameterName string, value interface{}) *InvalidNumberValueError {
	message := fmt.Sprintf("Invalid number value for parameter '%s': cannot convert '%v' to number", parameterName, value)
	return &InvalidNumberValueError{
		ParameterExtractorError: NewParameterExtractorError("INVALID_NUMBER_VALUE", message),
		ParameterName:           parameterName,
		Value:                   value,
	}
}

// InvalidBoolValueError is raised when a boolean parameter value cannot be converted to a boolean
type InvalidBoolValueError struct {
	*ParameterExtractorError
	ParameterName string
	Value         interface{}
}

// NewInvalidBoolValueError creates a new InvalidBoolValueError
func NewInvalidBoolValueError(parameterName string, value interface{}) *InvalidBoolValueError {
	message := fmt.Sprintf("Invalid boolean value for parameter '%s': cannot convert '%v' to boolean", parameterName, value)
	return &InvalidBoolValueError{
		ParameterExtractorError: NewParameterExtractorError("INVALID_BOOL_VALUE", message),
		ParameterName:           parameterName,
		Value:                   value,
	}
}

// InvalidStringValueError is raised when a string parameter value cannot be converted to a string
type InvalidStringValueError struct {
	*ParameterExtractorError
	ParameterName string
	Value         interface{}
}

// NewInvalidStringValueError creates a new InvalidStringValueError
func NewInvalidStringValueError(parameterName string, value interface{}) *InvalidStringValueError {
	message := fmt.Sprintf("Invalid string value for parameter '%s': cannot convert '%v' to string", parameterName, value)
	return &InvalidStringValueError{
		ParameterExtractorError: NewParameterExtractorError("INVALID_STRING_VALUE", message),
		ParameterName:           parameterName,
		Value:                   value,
	}
}

// InvalidArrayValueError is raised when an array parameter value is invalid or elements have wrong type
type InvalidArrayValueError struct {
	*ParameterExtractorError
	ParameterName string
	Value         interface{}
	ElementType   string
	Reason        string
}

// NewInvalidArrayValueError creates a new InvalidArrayValueError
func NewInvalidArrayValueError(parameterName string, value interface{}, elementType, reason string) *InvalidArrayValueError {
	message := fmt.Sprintf("Invalid array value for parameter '%s': expected array[%s], %s", parameterName, elementType, reason)
	return &InvalidArrayValueError{
		ParameterExtractorError: NewParameterExtractorError("INVALID_ARRAY_VALUE", message),
		ParameterName:           parameterName,
		Value:                   value,
		ElementType:             elementType,
		Reason:                  reason,
	}
}
