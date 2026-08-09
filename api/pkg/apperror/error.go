package apperror

import (
	"errors"
	"strings"
)

// Error is a transport-neutral application error. It is immutable after
// construction and safe for concurrent reads.
type Error struct {
	code      Code
	cause     error
	operation string
	params    map[string]any
}

// Option adds diagnostic context while an Error is being constructed. Its
// fields are private so callers cannot reapply an option to a constructed
// Error and violate its immutability. The zero value is a no-op.
type Option struct {
	kind      optionKind
	operation string
	params    []Param
}

type optionKind uint8

const (
	operationOption optionKind = iota + 1
	paramsOption
)

// WithOperation records a stable internal operation name such as
// "gateway.chat_completion". It is diagnostic context, not user-facing text.
func WithOperation(operation string) Option {
	return Option{kind: operationOption, operation: strings.TrimSpace(operation)}
}

// WithParams adds immutable scalar parameters. Empty parameter names are
// ignored and later values replace earlier values with the same name.
func WithParams(params ...Param) Option {
	copied := make([]Param, len(params))
	copy(copied, params)
	return Option{kind: paramsOption, params: copied}
}

// New creates an application error without an underlying cause.
func New(code Code, options ...Option) error {
	return build(code, nil, options...)
}

// Wrap creates an application error that preserves cause. A nil cause returns
// nil, matching common Go wrapping helpers and avoiding typed-nil errors.
func Wrap(cause error, code Code, options ...Option) error {
	if cause == nil {
		return nil
	}
	return build(code, cause, options...)
}

func build(code Code, cause error, options ...Option) *Error {
	if code.value == "" {
		panic("application error code is empty")
	}
	operation := ""
	var params map[string]any
	for _, option := range options {
		switch option.kind {
		case operationOption:
			operation = option.operation
		case paramsOption:
			for _, param := range option.params {
				if param.name == "" {
					continue
				}
				if params == nil {
					params = make(map[string]any, len(option.params))
				}
				params[param.name] = param.value
			}
		}
	}
	return &Error{
		code:      code,
		cause:     cause,
		operation: operation,
		params:    params,
	}
}

// Error returns diagnostic text. It is not a localized or safe public message
// and must not be copied directly into an API response.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	identity := e.code.String()
	if e.operation != "" {
		identity = e.operation + ": " + identity
	}
	if e.cause != nil {
		if identity == "" {
			return e.cause.Error()
		}
		return identity + ": " + e.cause.Error()
	}
	return identity
}

// Unwrap exposes the underlying cause to errors.Is and errors.As.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Is allows errors.Is to match an application error by Code. Package callers
// should normally use IsCode rather than constructing a matcher directly.
func (e *Error) Is(target error) bool {
	matcher, ok := target.(codeMatcher)
	return ok && e != nil && e.code == matcher.code
}

// Code returns the stable application error identity.
func (e *Error) Code() Code {
	if e == nil {
		return Code{}
	}
	return e.code
}

// Operation returns diagnostic operation context.
func (e *Error) Operation() string {
	if e == nil {
		return ""
	}
	return e.operation
}

// Params returns a copy of the scalar parameters.
func (e *Error) Params() map[string]any {
	if e == nil {
		return nil
	}
	return copyParams(e.params)
}

// As finds the first application error in err's error tree.
func As(err error) (*Error, bool) {
	var appErr *Error
	if !errors.As(err, &appErr) || appErr == nil {
		return nil, false
	}
	return appErr, true
}

// CodeOf returns the first application error code in err's error tree.
func CodeOf(err error) (Code, bool) {
	appErr, ok := As(err)
	if !ok {
		return Code{}, false
	}
	return appErr.code, true
}

// IsCode reports whether any error in err's error tree carries code. It works
// with ordinary wrapping and errors.Join.
func IsCode(err error, code Code) bool {
	return err != nil && errors.Is(err, codeMatcher{code: code})
}

type codeMatcher struct {
	code Code
}

func (m codeMatcher) Error() string {
	return m.code.String()
}
