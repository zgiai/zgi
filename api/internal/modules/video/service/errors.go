package service

import "errors"

var (
	ErrPromptRequired          = errors.New("PROMPT_REQUIRED")
	ErrPromptTooLong           = errors.New("PROMPT_TOO_LONG")
	ErrModelNotAvailable       = errors.New("MODEL_NOT_AVAILABLE")
	ErrTaskNotFound            = errors.New("VIDEO_TASK_NOT_FOUND")
	ErrBillingContextRequired  = errors.New("BILLING_CONTEXT_REQUIRED")
	ErrUpstreamFailed          = errors.New("UPSTREAM_FAILED")
	ErrVideoRuntimeUnavailable = errors.New("VIDEO_RUNTIME_UNAVAILABLE")
)

func ErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrPromptRequired):
		return "PROMPT_REQUIRED"
	case errors.Is(err, ErrPromptTooLong):
		return "PROMPT_TOO_LONG"
	case errors.Is(err, ErrModelNotAvailable):
		return "MODEL_NOT_AVAILABLE"
	case errors.Is(err, ErrTaskNotFound):
		return "VIDEO_TASK_NOT_FOUND"
	case errors.Is(err, ErrBillingContextRequired):
		return "BILLING_CONTEXT_REQUIRED"
	case errors.Is(err, ErrUpstreamFailed):
		return "UPSTREAM_FAILED"
	case errors.Is(err, ErrVideoRuntimeUnavailable):
		return "VIDEO_RUNTIME_UNAVAILABLE"
	default:
		return "VIDEO_RUNTIME_FAILED"
	}
}
