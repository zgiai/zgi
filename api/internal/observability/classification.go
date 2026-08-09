package observability

import "strconv"

// ErrorCategory describes what kind of failure occurred. Values must stay
// low-cardinality because adapters may index them as tags.
type ErrorCategory string

const (
	ErrorCategoryApplication   ErrorCategory = "application"
	ErrorCategoryConfiguration ErrorCategory = "configuration"
	ErrorCategoryDatabase      ErrorCategory = "database"
	ErrorCategoryDependency    ErrorCategory = "dependency"
	ErrorCategoryTimeout       ErrorCategory = "timeout"
)

// ErrorSource identifies the party that can act on a failure.
type ErrorSource string

const (
	ErrorSourceInfrastructure ErrorSource = "infrastructure"
	ErrorSourceProvider       ErrorSource = "provider"
	ErrorSourceTenant         ErrorSource = "tenant"
	ErrorSourceZGI            ErrorSource = "zgi"
)

// ErrorClassification is provider-neutral diagnostic metadata shared by
// Sentry, OpenTelemetry, and future Reporter adapters.
type ErrorClassification struct {
	Category  ErrorCategory
	Source    ErrorSource
	Code      string
	Retryable bool
}

// FailureReportHint carries provider-neutral reporting intent from a handler
// to outer HTTP middleware. It avoids teaching generic middleware about LLM
// provider or billing error types.
type FailureReportHint struct {
	EventName      string
	Classification ErrorClassification
	Suppress       bool
}

// WithErrorClassification adds stable diagnostic tags without exposing a
// vendor SDK or high-cardinality request data to product code.
func WithErrorClassification(classification ErrorClassification) EventOption {
	return func(event *Event) {
		if event.Tags == nil {
			event.Tags = make(map[string]string, 4)
		}
		if classification.Category != "" {
			event.Tags["error.category"] = string(classification.Category)
		}
		if classification.Source != "" {
			event.Tags["error.source"] = string(classification.Source)
		}
		if classification.Code != "" {
			event.Tags["error.code"] = classification.Code
		}
		event.Tags["error.retryable"] = strconv.FormatBool(classification.Retryable)
	}
}
