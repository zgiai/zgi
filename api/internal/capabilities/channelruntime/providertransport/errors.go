package providertransport

import (
	"errors"
)

const (
	ErrorCodeDisabled                = "integration_disabled"
	ErrorCodeInvalidInput            = "integration_invalid_input"
	ErrorCodeSensitiveInput          = "integration_sensitive_input_blocked"
	ErrorCodeQuotaExceeded           = "integration_quota_exceeded"
	ErrorCodeAuthInvalid             = "integration_auth_invalid"
	ErrorCodeBudgetExceeded          = "integration_budget_exceeded"
	ErrorCodeAccessDenied            = "integration_access_denied"
	ErrorCodeRateLimited             = "integration_rate_limited"
	ErrorCodeTimeout                 = "integration_timeout"
	ErrorCodeUpstream                = "integration_upstream_unavailable"
	ErrorCodeProviderRejected        = "integration_provider_rejected"
	ErrorCodeResponseInvalid         = "integration_response_invalid"
	ErrorCodeAuditFailed             = "integration_audit_failed"
	ErrorCodePolicyConflict          = "integration_policy_conflict"
	ErrorCodeReconnectRequired       = "integration_reconnect_required"
	ErrorCodeConnectionExpired       = "integration_connection_expired"
	ErrorCodeInsufficientScope       = "integration_insufficient_scope"
	ErrorCodeActionAuthMethod        = "integration_action_auth_incompatible"
	ErrorCodeOperationInProgress     = "integration_operation_in_progress"
	ErrorCodeOperationOutcomeUnknown = "integration_operation_outcome_unknown"
)

type Error struct {
	Code                string
	ReasonCode          string `json:"-"`
	Message             string
	Err                 error
	ProviderDiagnostics ProviderDiagnostics `json:"-"`
}

func (e *Error) Error() string {
	if e == nil {
		return "integration error"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	return "integration error"
}

func (e *Error) Unwrap() error { return e.Err }

// PublicErrorCode exposes the stable, non-sensitive integration error code to
// presentation layers. Callers must not surface Message or Err as a localized
// user-facing contract because either may contain provider-specific details.
func (e *Error) PublicErrorCode() string {
	if e == nil {
		return ""
	}
	return e.Code
}

// PublicErrorRecovery exposes only stable, structural provider diagnostics.
// Provider messages, descriptions, values, payloads, URLs, credentials, and
// connection identifiers never enter this contract.

func NewError(code, message string, err error) error {
	return &Error{Code: code, Message: message, Err: err}
}

// NewErrorWithReason attaches a stable, non-sensitive reason code that can be
// surfaced to models and UIs without exposing internal messages or identities.
func NewErrorWithReason(code, reasonCode, message string, err error) error {
	return &Error{Code: code, ReasonCode: reasonCode, Message: message, Err: err}
}

// NewProviderError creates an integration error with bounded, structured
// provider diagnostics. Provider response messages and bodies deliberately
// have no field in this contract and must remain in the wrapped operator-only
// error, if they are retained at all.
func NewProviderError(code, message string, err error, diagnostics ProviderDiagnostics) error {
	return &Error{
		Code:                code,
		Message:             message,
		Err:                 err,
		ProviderDiagnostics: normalizeProviderDiagnostics(diagnostics),
	}
}

// NewProviderErrorWithReason adds a stable recovery reason while preserving
// bounded provider diagnostics. Presentation layers may use the reason code
// to select an actionable, localized message without exposing provider text.
func NewProviderErrorWithReason(code, reasonCode, message string, err error, diagnostics ProviderDiagnostics) error {
	return &Error{
		Code:                code,
		ReasonCode:          reasonCode,
		Message:             message,
		Err:                 err,
		ProviderDiagnostics: normalizeProviderDiagnostics(diagnostics),
	}
}

func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	var integrationError *Error
	if errors.As(err, &integrationError) && integrationError.Code != "" {
		return integrationError.Code
	}
	return ErrorCodeUpstream
}

// ErrorReasonCode returns the stable, non-sensitive recovery reason attached
// by a provider adapter. Unknown errors deliberately return an empty value.
func ErrorReasonCode(err error) string {
	if err == nil {
		return ""
	}
	var integrationError *Error
	if errors.As(err, &integrationError) {
		return integrationError.ReasonCode
	}
	return ""
}
