package core

import "errors"

// ErrorCode is a stable, non-sensitive failure identity shared by services and
// transports. Provider messages and submitted values are never valid codes.
type ErrorCode string

const (
	ErrorCodeInvalidInput          ErrorCode = "connector_invalid_input"
	ErrorCodeNotFound              ErrorCode = "connector_not_found"
	ErrorCodeAccessDenied          ErrorCode = "connector_access_denied"
	ErrorCodeConflict              ErrorCode = "connector_conflict"
	ErrorCodeInvalidKey            ErrorCode = "connector_key_invalid"
	ErrorCodeInsufficientKey       ErrorCode = "connector_key_insufficient"
	ErrorCodeProviderUnavailable   ErrorCode = "connector_provider_unavailable"
	ErrorCodeConnectorUnavailable  ErrorCode = "connector_unavailable"
	ErrorCodeConnectionNotReady    ErrorCode = "connector_connection_not_ready"
	ErrorCodeTargetNotReady        ErrorCode = "connector_target_not_ready"
	ErrorCodeConfirmationRequired  ErrorCode = "connector_confirmation_required"
	ErrorCodeOutcomeUnknown        ErrorCode = "connector_operation_outcome_unknown"
	ErrorCodeInternal              ErrorCode = "connector_internal"
	ErrorCodeInvalidAppCredentials ErrorCode = "connector_app_credentials_invalid"
	ErrorCodePermissionMissing     ErrorCode = "connector_provider_permission_missing"
	ErrorCodeResourceAccessDenied  ErrorCode = "connector_resource_access_denied"
	ErrorCodeApplicationNotReady   ErrorCode = "connector_application_not_ready"
	ErrorCodeRateLimited           ErrorCode = "connector_rate_limited"
	ErrorCodeFlowExpired           ErrorCode = "connector_flow_expired"
	// ErrorCodeCommandForbidden is a server command the organization's rules
	// forbid (ADR 0029).
	ErrorCodeCommandForbidden ErrorCode = "connector_command_forbidden"
	// ErrorCodeSessionGrantUnsupported is a confirmation that asked to be
	// remembered for the session on a surface that confirms each operation
	// once. It is returned before anything is routed, so the same receipt can
	// be confirmed again without the preference.
	ErrorCodeSessionGrantUnsupported ErrorCode = "connector_session_grant_unsupported"

	// Word-order aliases keep call sites readable without changing the stable
	// wire values.
	ErrorCodeKeyInvalid      = ErrorCodeInvalidKey
	ErrorCodeKeyInsufficient = ErrorCodeInsufficientKey
)

func (code ErrorCode) Valid() bool {
	switch code {
	case ErrorCodeInvalidInput,
		ErrorCodeNotFound,
		ErrorCodeAccessDenied,
		ErrorCodeConflict,
		ErrorCodeInvalidKey,
		ErrorCodeInsufficientKey,
		ErrorCodeProviderUnavailable,
		ErrorCodeConnectorUnavailable,
		ErrorCodeConnectionNotReady,
		ErrorCodeTargetNotReady,
		ErrorCodeConfirmationRequired,
		ErrorCodeOutcomeUnknown,
		ErrorCodeWeixinAlreadyBound,
		ErrorCodeWeixinRegionUnsupported,
		ErrorCodeInternal, ErrorCodeInvalidAppCredentials, ErrorCodePermissionMissing,
		ErrorCodeResourceAccessDenied, ErrorCodeApplicationNotReady, ErrorCodeRateLimited, ErrorCodeFlowExpired, ErrorCodeCommandForbidden,
		ErrorCodeSessionGrantUnsupported,
		ErrorCodeVerificationWaiting, ErrorCodeProviderResponseUnrecognized, ErrorCodeVerificationUncertain,
		ErrorCodeMailAuthenticationRejected, ErrorCodeMailAppPasswordRequired, ErrorCodeMailTLSRequired,
		ErrorCodeMailPolicyRestricted, ErrorCodeMailReferenceExpired, ErrorCodeMailSizeExceeded, ErrorCodeMailRecipientRejected, ErrorCodeMailSubmissionRejected,
		ErrorCodeMailClientIdentityRequired, ErrorCodeMailAuthMechanismUnsupported, ErrorCodeMailUidStateUnavailable:
		return true
	default:
		return false
	}
}

// Error deliberately contains only public, bounded fields. NewError accepts a
// cause so callers can use a uniform construction pattern, but does not retain
// it: provider failures can echo submitted credentials in their text.
type Error struct {
	Code       ErrorCode `json:"code"`
	Retryable  bool      `json:"retryable"`
	diagnostic *ProviderDiagnostic
}

func NewError(code ErrorCode, retryable bool, cause error) *Error {
	if !code.Valid() {
		code = ErrorCodeInternal
	}
	return &Error{Code: code, Retryable: retryable, diagnostic: DiagnosticOf(cause)}
}

func (err *Error) Error() string {
	if err == nil || !err.Code.Valid() {
		return string(ErrorCodeInternal)
	}
	return string(err.Code)
}

// ErrorCodeOf extracts a stable code and maps all unstructured failures to a
// non-sensitive internal code.
func ErrorCodeOf(err error) ErrorCode {
	if err == nil {
		return ""
	}
	var connectorError *Error
	if errors.As(err, &connectorError) && connectorError != nil && connectorError.Code.Valid() {
		return connectorError.Code
	}
	return ErrorCodeInternal
}

func IsErrorCode(err error, code ErrorCode) bool {
	return code.Valid() && ErrorCodeOf(err) == code
}

func invalidModel() error {
	return NewError(ErrorCodeInvalidInput, false, nil)
}

func (err *Error) PublicErrorCode() string { return err.Error() }
