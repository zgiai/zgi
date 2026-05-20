package base

import "fmt"

type BaseNodeError struct {
	message string
}

func (e *BaseNodeError) Error() string {
	return e.message
}

func NewBaseNodeError(message string) *BaseNodeError {
	return &BaseNodeError{message: message}
}

type DefaultValueTypeError struct {
	*BaseNodeError
}

func (e *DefaultValueTypeError) Error() string {
	return fmt.Sprintf("DefaultValueTypeError: %s", e.message)
}

func NewDefaultValueTypeError(message string) *DefaultValueTypeError {
	return &DefaultValueTypeError{
		BaseNodeError: NewBaseNodeError(message),
	}
}
