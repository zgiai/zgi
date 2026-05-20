package llm

import "fmt"

// NodeError is the base error type for LLM Node errors
type NodeError struct {
	message string
}

func (e *NodeError) Error() string {
	return e.message
}

// NewLLMNodeError creates a new NodeError
func NewLLMNodeError(message string) *NodeError {
	return &NodeError{message: message}
}

// VariableNotFoundError is raised when a required variable is not found
type VariableNotFoundError struct {
	*NodeError
}

// NewVariableNotFoundError creates a new VariableNotFoundError
func NewVariableNotFoundError(message string) *VariableNotFoundError {
	return &VariableNotFoundError{
		NodeError: NewLLMNodeError(message),
	}
}

// InvalidContextStructureError is raised when the context structure is invalid
type InvalidContextStructureError struct {
	*NodeError
}

// NewInvalidContextStructureError creates a new InvalidContextStructureError
func NewInvalidContextStructureError(message string) *InvalidContextStructureError {
	return &InvalidContextStructureError{
		NodeError: NewLLMNodeError(message),
	}
}

// InvalidVariableTypeError is raised when the variable type is invalid
type InvalidVariableTypeError struct {
	*NodeError
}

// NewInvalidVariableTypeError creates a new InvalidVariableTypeError
func NewInvalidVariableTypeError(message string) *InvalidVariableTypeError {
	return &InvalidVariableTypeError{
		NodeError: NewLLMNodeError(message),
	}
}

// ModelNotExistError is raised when the specified model does not exist
type ModelNotExistError struct {
	*NodeError
}

// NewModelNotExistError creates a new ModelNotExistError
func NewModelNotExistError(message string) *ModelNotExistError {
	return &ModelNotExistError{
		NodeError: NewLLMNodeError(message),
	}
}

// LLMModeRequiredError is raised when LLM mode is required but not provided
type LLMModeRequiredError struct {
	*NodeError
}

// NewLLMModeRequiredError creates a new LLMModeRequiredError
func NewLLMModeRequiredError(message string) *LLMModeRequiredError {
	return &LLMModeRequiredError{
		NodeError: NewLLMNodeError(message),
	}
}

// NoPromptFoundError is raised when no prompt is found in the LLM configuration
type NoPromptFoundError struct {
	*NodeError
}

// NewNoPromptFoundError creates a new NoPromptFoundError
func NewNoPromptFoundError(message string) *NoPromptFoundError {
	return &NoPromptFoundError{
		NodeError: NewLLMNodeError(message),
	}
}

// TemplateTypeNotSupportError is raised when the prompt type is not supported
type TemplateTypeNotSupportError struct {
	*NodeError
	TypeName string
}

// NewTemplateTypeNotSupportError creates a new TemplateTypeNotSupportError
func NewTemplateTypeNotSupportError(typeName string) *TemplateTypeNotSupportError {
	message := fmt.Sprintf("Prompt type %s is not supported.", typeName)
	return &TemplateTypeNotSupportError{
		NodeError: NewLLMNodeError(message),
		TypeName:  typeName,
	}
}

// MemoryRolePrefixRequiredError is raised when memory role prefix is required for completion model
type MemoryRolePrefixRequiredError struct {
	*NodeError
}

// NewMemoryRolePrefixRequiredError creates a new MemoryRolePrefixRequiredError
func NewMemoryRolePrefixRequiredError(message string) *MemoryRolePrefixRequiredError {
	return &MemoryRolePrefixRequiredError{
		NodeError: NewLLMNodeError(message),
	}
}

// FileTypeNotSupportError is raised when file type is not supported by the model
type FileTypeNotSupportError struct {
	*NodeError
	TypeName string
}

// NewFileTypeNotSupportError creates a new FileTypeNotSupportError
func NewFileTypeNotSupportError(typeName string) *FileTypeNotSupportError {
	message := fmt.Sprintf("%s type is not supported by this model", typeName)
	return &FileTypeNotSupportError{
		NodeError: NewLLMNodeError(message),
		TypeName:  typeName,
	}
}

// UnsupportedPromptContentTypeError is raised when prompt content type is not supported
type UnsupportedPromptContentTypeError struct {
	*NodeError
	TypeName string
}

// NewUnsupportedPromptContentTypeError creates a new UnsupportedPromptContentTypeError
func NewUnsupportedPromptContentTypeError(typeName string) *UnsupportedPromptContentTypeError {
	message := fmt.Sprintf("Prompt content type %s is not supported.", typeName)
	return &UnsupportedPromptContentTypeError{
		NodeError: NewLLMNodeError(message),
		TypeName:  typeName,
	}
}

// ModelAccessDeniedError is raised when tenant does not have access to the specified model
type ModelAccessDeniedError struct {
	*NodeError
}

// NewModelAccessDeniedError creates a new ModelAccessDeniedError
func NewModelAccessDeniedError(message string) *ModelAccessDeniedError {
	return &ModelAccessDeniedError{
		NodeError: NewLLMNodeError(message),
	}
}

// InvalidModelConfigError is raised when model configuration is invalid
type InvalidModelConfigError struct {
	*NodeError
	Field string
}

// NewInvalidModelConfigError creates a new InvalidModelConfigError
func NewInvalidModelConfigError(field string, message string) *InvalidModelConfigError {
	fullMessage := fmt.Sprintf("invalid model config field '%s': %s", field, message)
	return &InvalidModelConfigError{
		NodeError: NewLLMNodeError(fullMessage),
		Field:     field,
	}
}
