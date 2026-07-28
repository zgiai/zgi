package integrations

import (
	"regexp"
	"strings"
	"time"
)

const (
	maxProviderErrorCodeLength = 128
	maxProviderRequestIDLength = 128
)

var (
	safeProviderErrorCodePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)
	safeProviderRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/=+-]*$`)
)

// ProviderDiagnostics is the complete provider-owned metadata that may cross
// the adapter boundary into audit and health records. It intentionally does
// not contain provider messages, response bodies, headers, URLs, or request
// payloads.
type ProviderDiagnostics struct {
	ErrorCode    string
	RequestID    string
	HTTPStatus   int
	RetryAfterAt *time.Time
}

// ProviderDiagnosticsFromError returns normalized, non-sensitive provider
// diagnostics carried by an integration error. Errors without an explicit
// diagnostic contract return the zero value.
func ProviderDiagnosticsFromError(err error) ProviderDiagnostics {
	return providerDiagnosticsFromError(err, 0)
}

func providerDiagnosticsFromError(err error, depth int) ProviderDiagnostics {
	if err == nil || depth > 64 {
		return ProviderDiagnostics{}
	}
	diagnostics := ProviderDiagnostics{}
	if integrationError, ok := err.(*Error); ok {
		diagnostics = normalizeProviderDiagnostics(integrationError.ProviderDiagnostics)
	}
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		for _, nested := range wrapped.Unwrap() {
			diagnostics = mergeProviderDiagnostics(diagnostics, providerDiagnosticsFromError(nested, depth+1))
		}
	case interface{ Unwrap() error }:
		diagnostics = mergeProviderDiagnostics(diagnostics, providerDiagnosticsFromError(wrapped.Unwrap(), depth+1))
	}
	return diagnostics
}

func normalizeProviderDiagnostics(diagnostics ProviderDiagnostics) ProviderDiagnostics {
	diagnostics.ErrorCode = normalizeProviderDiagnosticValue(
		diagnostics.ErrorCode,
		maxProviderErrorCodeLength,
		safeProviderErrorCodePattern,
	)
	diagnostics.RequestID = normalizeProviderDiagnosticValue(
		diagnostics.RequestID,
		maxProviderRequestIDLength,
		safeProviderRequestIDPattern,
	)
	if diagnostics.HTTPStatus < 100 || diagnostics.HTTPStatus > 599 {
		diagnostics.HTTPStatus = 0
	}
	if diagnostics.RetryAfterAt != nil {
		if diagnostics.RetryAfterAt.IsZero() {
			diagnostics.RetryAfterAt = nil
		} else {
			normalized := diagnostics.RetryAfterAt.UTC()
			diagnostics.RetryAfterAt = &normalized
		}
	}
	return diagnostics
}

func mergeProviderDiagnostics(primary, fallback ProviderDiagnostics) ProviderDiagnostics {
	primary = normalizeProviderDiagnostics(primary)
	fallback = normalizeProviderDiagnostics(fallback)
	if primary.ErrorCode == "" {
		primary.ErrorCode = fallback.ErrorCode
	}
	if primary.RequestID == "" {
		primary.RequestID = fallback.RequestID
	}
	if primary.HTTPStatus == 0 {
		primary.HTTPStatus = fallback.HTTPStatus
	}
	if primary.RetryAfterAt == nil {
		primary.RetryAfterAt = cloneTimePointer(fallback.RetryAfterAt)
	}
	return primary
}

func normalizeProviderDiagnosticValue(value string, maxLength int, pattern *regexp.Regexp) string {
	value = strings.TrimSpace(value)
	if value == "" ||
		len(value) > maxLength ||
		!pattern.MatchString(value) ||
		containsSensitiveValue(value) {
		return ""
	}
	return value
}

func providerHTTPStatusValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func providerHTTPStatusPointer(status int) *int {
	if status < 100 || status > 599 {
		return nil
	}
	return &status
}
