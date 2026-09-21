package handler

import (
	"errors"
	"fmt"
	"strings"
)

// publicInputError marks a request-validation error whose message is vetted for
// clients. Handlers must never forward an arbitrary error's Error() text, which
// may contain transport or upstream diagnostics.
type publicInputError struct {
	message string
}

func (e *publicInputError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

func newPublicInputErrorf(format string, args ...any) error {
	return &publicInputError{message: fmt.Sprintf(format, args...)}
}

// publicInputMessage returns the vetted message for a request-validation error
// and a generic fallback for every other error.
func publicInputMessage(err error) string {
	var inputErr *publicInputError
	if errors.As(err, &inputErr) && inputErr != nil && strings.TrimSpace(inputErr.message) != "" {
		return inputErr.message
	}
	return "Invalid request"
}
