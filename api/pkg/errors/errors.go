package errors

import (
	"errors"
	"fmt"
)

// Standard errors
var (
	ErrNotFound      = errors.New("not found")
	ErrInvalidInput  = errors.New("invalid input")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrForbidden     = errors.New("forbidden")
	ErrInternal      = errors.New("internal error")
	ErrAlreadyExists = errors.New("already exists")
)

// Wrap wraps an error with a message
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

// Is reports whether any error in err's chain matches target
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// New creates a new error with the given message
func New(message string) error {
	return errors.New(message)
}

// NewBadRequestError creates a new bad request error
func NewBadRequestError(message string) error {
	return fmt.Errorf("bad request: %s", message)
}

// Errorf formats an error message
func Errorf(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}
