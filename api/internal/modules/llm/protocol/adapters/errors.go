package adapter

import (
	"errors"
	"fmt"
	"strings"
)

const ErrorCodePlatformChannelUnavailable = "platform_channel_unavailable"

var (
	// ErrInvalidConfig configuration error
	ErrInvalidConfig = errors.New("invalid adapter configuration")

	// ErrCapabilityUnsupported indicates the provider does not support the requested capability.
	ErrCapabilityUnsupported = errors.New("adapter capability unsupported")

	// ErrAuthFailed authentication failed
	ErrAuthFailed = errors.New("authentication failed")

	// ErrRateLimited rate limit exceeded
	ErrRateLimited = errors.New("rate limit exceeded")

	// ErrInsufficientBalance insufficient balance
	ErrInsufficientBalance = errors.New("insufficient balance")

	// ErrQuotaExhausted indicates a provider quota that cannot currently be consumed.
	ErrQuotaExhausted = errors.New("provider quota exhausted")

	// ErrBillingUnavailable indicates that provider billing is not in a spendable state.
	ErrBillingUnavailable = errors.New("provider billing unavailable")

	// ErrPlatformChannelUnavailable indicates an official platform channel cannot currently serve requests.
	ErrPlatformChannelUnavailable = errors.New("platform channel unavailable")

	// ErrModelNotFound model not found
	ErrModelNotFound = errors.New("model not found")

	// ErrTimeout request timeout
	ErrTimeout = errors.New("request timeout")

	// ErrUpstreamError upstream service error
	ErrUpstreamError = errors.New("upstream service error")

	// ErrInvalidRequest invalid request
	ErrInvalidRequest = errors.New("invalid request")

	// ErrStreamClosed stream closed
	ErrStreamClosed = errors.New("stream closed")

	// ErrProxyError proxy/gateway error (nginx, CDN, etc.)
	ErrProxyError = errors.New("proxy or gateway error")

	// ErrContentPolicyViolation content policy violation
	ErrContentPolicyViolation = errors.New("content policy violation")
)

// IsCapabilityUnsupported reports whether the error indicates an unsupported adapter capability.
func IsCapabilityUnsupported(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrCapabilityUnsupported) {
		return true
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not implemented") ||
		strings.Contains(msg, "not supported") ||
		strings.Contains(msg, "unsupported")
}

// AdapterError adapter error
type AdapterError struct {
	Code       string
	Message    string
	StatusCode int
	Err        error
}

// HTTPStatusError preserves a non-success streaming response for the provider
// adapter that owns the error format. Its Error method intentionally omits the
// response body so callers do not accidentally log provider payloads.
type HTTPStatusError struct {
	StatusCode int
	Body       []byte
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("stream request failed with status %d", e.StatusCode)
}

func NewHTTPStatusError(statusCode int, body []byte) *HTTPStatusError {
	return &HTTPStatusError{StatusCode: statusCode, Body: append([]byte(nil), body...)}
}

func (e *AdapterError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *AdapterError) Unwrap() error {
	return e.Err
}

// NewAdapterError creates a new adapter error
func NewAdapterError(code, message string, statusCode int, err error) *AdapterError {
	return &AdapterError{
		Code:       code,
		Message:    message,
		StatusCode: statusCode,
		Err:        err,
	}
}

// HandleNonJSONError handles error responses that are not valid JSON (e.g. HTML
// error pages from nginx/reverse proxies). All provider adapters should call
// this inside their handleError when json.Unmarshal fails.
func HandleNonJSONError(statusCode int, body []byte) *AdapterError {
	bodyStr := string(body)
	if len(bodyStr) > 200 {
		bodyStr = bodyStr[:200] + "..."
	}

	// Detect HTML responses from proxy/nginx
	trimmed := strings.TrimSpace(bodyStr)
	if strings.HasPrefix(trimmed, "<") || strings.HasPrefix(trimmed, "<!") || strings.HasPrefix(trimmed, "<html") {
		switch statusCode {
		case 400:
			return NewAdapterError("PROXY_BAD_REQUEST",
				fmt.Sprintf("Proxy returned HTML 400 error. Request may be too large or malformed (status %d).", statusCode),
				statusCode, ErrProxyError)
		case 401:
			return NewAdapterError("UNAUTHORIZED",
				"Invalid API key or authentication failed.", statusCode, ErrAuthFailed)
		case 403:
			return NewAdapterError("FORBIDDEN",
				"API request forbidden. Please check your API key and permissions.", statusCode, ErrAuthFailed)
		case 502, 503, 504:
			return NewAdapterError("PROXY_UPSTREAM_ERROR",
				fmt.Sprintf("Proxy upstream error (status %d). The API endpoint may be temporarily unavailable.", statusCode),
				statusCode, ErrProxyError)
		default:
			return NewAdapterError("PROXY_ERROR",
				fmt.Sprintf("Received HTML error page from proxy (status %d). Please check API endpoint configuration.", statusCode),
				statusCode, ErrProxyError)
		}
	}

	return NewAdapterError("PARSE_ERROR",
		fmt.Sprintf("Failed to parse error response (status %d): %s", statusCode, bodyStr),
		statusCode, ErrUpstreamError)
}
