package x

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

const (
	defaultAPIBaseURL    = "https://api.x.com"
	defaultClientTimeout = 20 * time.Second
	maxResponseBytes     = 2 << 20
	maxReadAttempts      = 3
)

type client struct {
	httpClient *http.Client
	baseURL    *url.URL
}

type responseMeta struct {
	RequestID   string
	Attempts    int
	Diagnostics integrations.ProviderDiagnostics
}

func newClient(httpClient *http.Client) (*client, error) {
	return newClientForBaseURL(httpClient, defaultAPIBaseURL)
}

// newClientForBaseURL exists only for package-local httptest servers.
// Production construction pins credentials to api.x.com.
func newClientForBaseURL(httpClient *http.Client, baseURL string) (*client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("initialize X API endpoint")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultClientTimeout}
	}
	httpClientCopy := *httpClient
	httpClientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &client{httpClient: &httpClientCopy, baseURL: parsed}, nil
}

func (c *client) getJSON(ctx context.Context, accessToken, path string, query url.Values, target interface{}) (responseMeta, error) {
	return c.doJSON(ctx, http.MethodGet, path, query, nil, accessToken, true, target)
}

func (c *client) postJSON(ctx context.Context, accessToken, path string, body interface{}, target interface{}) (responseMeta, error) {
	// Creating a post is non-idempotent. Never retry it automatically.
	return c.doJSON(ctx, http.MethodPost, path, nil, body, accessToken, false, target)
}

func (c *client) doJSON(
	ctx context.Context,
	method, path string,
	query url.Values,
	body interface{},
	accessToken string,
	retryable bool,
	target interface{},
) (responseMeta, error) {
	if c == nil || c.httpClient == nil || c.baseURL == nil {
		return responseMeta{}, integrations.NewError(integrations.ErrorCodeUpstream, "X client is unavailable", nil)
	}
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return responseMeta{}, integrations.NewError(integrations.ErrorCodeAuthInvalid, "X credentials are unavailable", nil)
	}
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + strings.TrimLeft(path, "/")
	if query != nil {
		endpoint.RawQuery = query.Encode()
	}
	var encodedBody []byte
	var err error
	if body != nil {
		encodedBody, err = json.Marshal(body)
		if err != nil {
			return responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "X request could not be encoded", err)
		}
	}
	attemptLimit := 1
	if retryable {
		attemptLimit = maxReadAttempts
	}
	var lastErr error
	for attempt := 1; attempt <= attemptLimit; attempt++ {
		var reader io.Reader
		if encodedBody != nil {
			reader = bytes.NewReader(encodedBody)
		}
		request, requestErr := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
		if requestErr != nil {
			return responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "X request could not be created", requestErr)
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("Authorization", "Bearer "+accessToken)
		request.Header.Set("User-Agent", "ZGI-External-Integrations/1.0")
		if encodedBody != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		response, requestErr := c.httpClient.Do(request)
		if requestErr != nil {
			if ctx.Err() != nil || errors.Is(requestErr, context.DeadlineExceeded) {
				return responseMeta{Attempts: attempt}, integrations.NewError(integrations.ErrorCodeTimeout, "X request timed out", ctx.Err())
			}
			lastErr = integrations.NewError(integrations.ErrorCodeUpstream, "X is unavailable", requestErr)
			if retryable && attempt < attemptLimit {
				if waitErr := waitForRetry(ctx, time.Duration(attempt*attempt)*100*time.Millisecond); waitErr == nil {
					continue
				} else {
					meta := responseMeta{Attempts: attempt}
					return meta, integrations.NewProviderError(
						integrations.ErrorCodeTimeout,
						"X request timed out",
						waitErr,
						meta.Diagnostics,
					)
				}
			}
			return responseMeta{Attempts: attempt}, lastErr
		}
		meta := responseMeta{
			RequestID: firstNonEmpty(response.Header.Get("X-Transaction-Id"), response.Header.Get("X-Request-Id")),
			Attempts:  attempt,
		}
		meta.Diagnostics = integrations.ProviderDiagnostics{
			RequestID:  meta.RequestID,
			HTTPStatus: response.StatusCode,
		}
		payload, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		_ = response.Body.Close()
		if readErr != nil {
			return meta, integrations.NewProviderError(
				integrations.ErrorCodeResponseInvalid,
				"X response could not be read",
				readErr,
				meta.Diagnostics,
			)
		}
		if len(payload) > maxResponseBytes {
			return meta, integrations.NewProviderError(
				integrations.ErrorCodeResponseInvalid,
				"X response exceeded the platform limit",
				nil,
				meta.Diagnostics,
			)
		}
		problem, hasProblem := parseXProblem(payload)
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			mapped, diagnostics := mapXProblem(response.StatusCode, response.Header, meta.RequestID, problem)
			meta.Diagnostics = diagnostics
			if retryable && retryableXError(response.StatusCode, mapped) && attempt < attemptLimit {
				lastErr = mapped
				if waitErr := waitForRetry(ctx, xRetryDelay(response.Header, attempt)); waitErr == nil {
					continue
				} else {
					return meta, integrations.NewProviderError(
						integrations.ErrorCodeTimeout,
						"X request timed out",
						waitErr,
						meta.Diagnostics,
					)
				}
			}
			return meta, mapped
		}
		if hasProblem {
			mapped, diagnostics := mapXProblem(response.StatusCode, response.Header, meta.RequestID, problem)
			meta.Diagnostics = diagnostics
			return meta, mapped
		}
		if target != nil && len(bytes.TrimSpace(payload)) > 0 {
			if err := json.Unmarshal(payload, target); err != nil {
				return meta, integrations.NewProviderError(
					integrations.ErrorCodeResponseInvalid,
					"X returned an invalid response",
					err,
					meta.Diagnostics,
				)
			}
		}
		return meta, nil
	}
	return responseMeta{Attempts: attemptLimit}, lastErr
}

func mapXProblem(
	status int,
	header http.Header,
	requestID string,
	problem xProblem,
) (error, integrations.ProviderDiagnostics) {
	providerCode := xProviderErrorCode(problem)
	if providerCode == "" {
		providerCode = fmt.Sprintf("http_%d", status)
	}
	diagnostics := integrations.ProviderDiagnostics{
		ErrorCode:    providerCode,
		RequestID:    requestID,
		HTTPStatus:   status,
		RetryAfterAt: xRetryAfterAt(header),
	}
	problemKind := strings.ToLower(strings.Join([]string{problem.Type, problem.Title}, " "))
	code := ""
	message := ""
	switch {
	case strings.Contains(problemKind, "usage-capped"),
		strings.Contains(problemKind, "usage cap"),
		strings.Contains(problemKind, "usagecap"):
		code = integrations.ErrorCodeBudgetExceeded
		message = "X usage plan limit was reached"
	case strings.Contains(problemKind, "rate-limit"),
		strings.Contains(problemKind, "rate limit"),
		strings.Contains(problemKind, "too many requests"):
		code = integrations.ErrorCodeRateLimited
		message = "X rate limit was reached"
	case strings.Contains(problemKind, "invalid"):
		code = integrations.ErrorCodeInvalidInput
		message = "X rejected the request parameters"
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
		code = integrations.ErrorCodeResponseInvalid
		message = "X returned a partial response with errors"
	}
	if code != "" {
		return integrations.NewProviderError(code, message, nil, diagnostics), diagnostics
	}
	switch status {
	case http.StatusUnauthorized:
		code = integrations.ErrorCodeAuthInvalid
		message = "X credentials are invalid or expired"
	case http.StatusPaymentRequired:
		code = integrations.ErrorCodeBudgetExceeded
		message = "X plan does not permit this operation"
	case http.StatusForbidden, http.StatusNotFound:
		code = integrations.ErrorCodeAccessDenied
		message = "X denied the requested operation or resource"
	case http.StatusTooManyRequests:
		code = integrations.ErrorCodeRateLimited
		message = "X rate limit was reached"
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		code = integrations.ErrorCodeInvalidInput
		message = "X rejected the request parameters"
	default:
		if status >= http.StatusInternalServerError {
			code = integrations.ErrorCodeUpstream
			message = "X is temporarily unavailable"
		} else {
			code = integrations.ErrorCodeUpstream
			message = "X request failed"
		}
	}
	return integrations.NewProviderError(code, message, nil, diagnostics), diagnostics
}

func retryableXError(status int, err error) bool {
	return integrations.ErrorCode(err) == integrations.ErrorCodeRateLimited ||
		status == http.StatusTooManyRequests || status == http.StatusInternalServerError ||
		status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func xRetryDelay(header http.Header, attempt int) time.Duration {
	if raw := strings.TrimSpace(header.Get("Retry-After")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
			return min(time.Duration(seconds)*time.Second, 5*time.Second)
		}
	}
	if raw := strings.TrimSpace(header.Get("X-Rate-Limit-Reset")); raw != "" {
		if epoch, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return min(max(time.Until(time.Unix(epoch, 0)), 0), 5*time.Second)
		}
	}
	return time.Duration(attempt*attempt) * 100 * time.Millisecond
}

func xRetryAfterAt(header http.Header) *time.Time {
	if raw := strings.TrimSpace(header.Get("Retry-After")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
			value := time.Now().Add(time.Duration(seconds) * time.Second).UTC()
			return &value
		}
	}
	if raw := strings.TrimSpace(header.Get("X-Rate-Limit-Reset")); raw != "" {
		if epoch, err := strconv.ParseInt(raw, 10, 64); err == nil {
			value := time.Unix(epoch, 0).UTC()
			return &value
		}
	}
	return nil
}

type xProblem struct {
	Type      string
	Title     string
	HasErrors bool
}

func parseXProblem(payload []byte) (xProblem, bool) {
	var envelope struct {
		Type   string            `json:"type"`
		Title  string            `json:"title"`
		Errors []json.RawMessage `json:"errors"`
	}
	if len(bytes.TrimSpace(payload)) == 0 || json.Unmarshal(payload, &envelope) != nil {
		return xProblem{}, false
	}
	problem := xProblem{
		Type:      strings.TrimSpace(envelope.Type),
		Title:     strings.TrimSpace(envelope.Title),
		HasErrors: len(envelope.Errors) > 0,
	}
	return problem, problem.Type != "" || problem.Title != "" || problem.HasErrors
}

func xProviderErrorCode(problem xProblem) string {
	// X problem titles are human-readable, provider-controlled strings and
	// must never become persisted diagnostics. Map only documented problem
	// families to fixed identifiers; unknown problems fall back to the HTTP
	// status in mapXProblem.
	problemKind := strings.ToLower(strings.Join([]string{problem.Type, problem.Title}, " "))
	switch {
	case strings.Contains(problemKind, "usage-capped"),
		strings.Contains(problemKind, "usage cap"),
		strings.Contains(problemKind, "usagecap"):
		return "usage_capped"
	case strings.Contains(problemKind, "rate-limit"),
		strings.Contains(problemKind, "rate limit"),
		strings.Contains(problemKind, "too many requests"):
		return "rate_limit_exceeded"
	case strings.Contains(problemKind, "client-forbidden"),
		strings.Contains(problemKind, "client forbidden"):
		return "client_forbidden"
	case strings.Contains(problemKind, "invalid"):
		return "invalid_request"
	default:
		return ""
	}
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}
