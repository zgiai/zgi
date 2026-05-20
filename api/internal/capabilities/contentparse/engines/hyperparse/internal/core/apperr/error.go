package apperr

import "fmt"

const (
	CodeBadInput            = "BAD_INPUT"
	CodeUnknownFormat       = "UNKNOWN_FORMAT"
	CodeNoAdapterRegistered = "NO_ADAPTER_REGISTERED"
	CodeAdapterParseFailed  = "ADAPTER_PARSE_FAILED"
)

// Error is a structured application error carrying a stable code.
type Error struct {
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return fmt.Sprintf("[%s] %s", e.Code, e.Message)
	}
	return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func New(code, msg string) error {
	return &Error{Code: code, Message: msg}
}

func Wrap(code, msg string, cause error) error {
	return &Error{Code: code, Message: msg, Cause: cause}
}
