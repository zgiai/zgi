package github

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
	defaultBaseURL       = "https://api.github.com"
	githubAPIVersion     = "2026-03-10"
	maxResponseBytes     = 2 << 20
	defaultMaxAttempts   = 3
	defaultClientTimeout = 20 * time.Second
)

type client struct {
	httpClient  *http.Client
	baseURL     *url.URL
	maxAttempts int
}

func newClient(httpClient *http.Client) (*client, error) {
	return newClientForBaseURL(httpClient, defaultBaseURL)
}

// newClientForBaseURL exists only for package tests. Production assembly uses
// newClient and therefore cannot redirect credentials to an arbitrary host.
func newClientForBaseURL(httpClient *http.Client, baseURL string) (*client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("initialize GitHub API endpoint")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultClientTimeout}
	}
	httpClientCopy := *httpClient
	httpClientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &client{httpClient: &httpClientCopy, baseURL: parsed, maxAttempts: defaultMaxAttempts}, nil
}

type responseMeta struct {
	RequestID   string
	Scopes      []string
	Attempts    int
	Diagnostics integrations.ProviderDiagnostics
}

func (c *client) getJSON(ctx context.Context, token, path string, query url.Values, target interface{}) (responseMeta, error) {
	return c.requestJSON(ctx, http.MethodGet, token, path, query, nil, target, true)
}

// postJSON deliberately never retries. GitHub issue and comment creation APIs
// do not accept an idempotency key, so retrying a response-loss or transient
// failure could create a duplicate external side effect.
func (c *client) postJSON(ctx context.Context, token, path string, body, target interface{}) (responseMeta, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "GitHub request could not be encoded", err)
	}
	return c.requestJSON(ctx, http.MethodPost, token, path, nil, payload, target, false)
}

func (c *client) requestJSON(
	ctx context.Context,
	method, token, path string,
	query url.Values,
	body []byte,
	target interface{},
	allowRetry bool,
) (responseMeta, error) {
	if c == nil || c.httpClient == nil || c.baseURL == nil {
		return responseMeta{}, integrations.NewError(integrations.ErrorCodeUpstream, "GitHub client is unavailable", nil)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return responseMeta{}, integrations.NewError(integrations.ErrorCodeAuthInvalid, "GitHub credentials are unavailable", nil)
	}
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + strings.TrimLeft(path, "/")
	endpoint.RawQuery = query.Encode()
	var lastErr error
	maxAttempts := c.maxAttempts
	if !allowRetry {
		maxAttempts = 1
	}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bytes.NewReader(body))
		if err != nil {
			return responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "GitHub request could not be created", err)
		}
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
		request.Header.Set("User-Agent", "ZGI-External-Integrations")
		if len(body) > 0 {
			request.Header.Set("Content-Type", "application/json")
		}
		response, err := c.httpClient.Do(request)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
				return responseMeta{Attempts: attempt}, integrations.NewError(integrations.ErrorCodeTimeout, "GitHub request timed out", ctx.Err())
			}
			lastErr = integrations.NewError(integrations.ErrorCodeUpstream, "GitHub is unavailable", err)
			if allowRetry && attempt < maxAttempts {
				if waitErr := waitForRetry(ctx, retryDelay(nil, attempt)); waitErr == nil {
					continue
				}
				return responseMeta{Attempts: attempt}, integrations.NewError(
					integrations.ErrorCodeTimeout,
					"GitHub request timed out",
					ctx.Err(),
				)
			}
			return responseMeta{Attempts: attempt}, lastErr
		}
		meta := responseMeta{
			RequestID: strings.TrimSpace(response.Header.Get("X-GitHub-Request-Id")),
			Scopes:    parseScopeHeader(response.Header.Get("X-OAuth-Scopes")),
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
				"GitHub response could not be read",
				readErr,
				meta.Diagnostics,
			)
		}
		if len(payload) > maxResponseBytes {
			return meta, integrations.NewProviderError(
				integrations.ErrorCodeResponseInvalid,
				"GitHub response exceeded the platform limit",
				nil,
				meta.Diagnostics,
			)
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			mapped, diagnostics := mapGitHubStatus(response.StatusCode, response.Header, payload, meta.RequestID)
			meta.Diagnostics = diagnostics
			if allowRetry && retryableGitHubError(response.StatusCode, mapped) && attempt < maxAttempts {
				lastErr = mapped
				if waitErr := waitForRetry(ctx, retryDelay(response.Header, attempt)); waitErr == nil {
					continue
				}
				return meta, integrations.NewProviderError(
					integrations.ErrorCodeTimeout,
					"GitHub request timed out",
					ctx.Err(),
					meta.Diagnostics,
				)
			}
			return meta, mapped
		}
		if err := json.Unmarshal(payload, target); err != nil {
			return meta, integrations.NewProviderError(
				integrations.ErrorCodeResponseInvalid,
				"GitHub returned an invalid response",
				err,
				meta.Diagnostics,
			)
		}
		return meta, nil
	}
	return responseMeta{Attempts: maxAttempts}, lastErr
}

func mapGitHubStatus(status int, header http.Header, payload []byte, requestID string) (error, integrations.ProviderDiagnostics) {
	message := githubErrorMessage(payload)
	secondaryRateLimit := status == http.StatusForbidden &&
		(strings.TrimSpace(header.Get("Retry-After")) != "" ||
			strings.Contains(strings.ToLower(message), "secondary rate limit") ||
			strings.Contains(strings.ToLower(message), "abuse detection"))
	primaryRateLimit := status == http.StatusForbidden &&
		strings.TrimSpace(header.Get("X-RateLimit-Remaining")) == "0"
	providerCode := fmt.Sprintf("http_%d", status)
	if secondaryRateLimit {
		providerCode = "secondary_rate_limit"
	} else if primaryRateLimit {
		providerCode = "primary_rate_limit"
	}
	diagnostics := integrations.ProviderDiagnostics{
		ErrorCode:    providerCode,
		RequestID:    requestID,
		HTTPStatus:   status,
		RetryAfterAt: githubRetryAfterAt(header),
	}
	code := ""
	safeMessage := ""
	switch status {
	case http.StatusUnauthorized:
		code = integrations.ErrorCodeAuthInvalid
		safeMessage = "GitHub credentials are invalid"
	case http.StatusPaymentRequired:
		code = integrations.ErrorCodeBudgetExceeded
		safeMessage = "GitHub billing prevents this request"
	case http.StatusForbidden:
		if primaryRateLimit || secondaryRateLimit {
			code = integrations.ErrorCodeRateLimited
			safeMessage = "GitHub rate limit was reached"
		} else {
			code = integrations.ErrorCodeAccessDenied
			safeMessage = "GitHub denied access to this resource"
		}
	case http.StatusNotFound:
		// GitHub intentionally uses 404 for some private resources that the token
		// cannot access. Treat it as authorization, not as credential invalidity.
		code = integrations.ErrorCodeAccessDenied
		safeMessage = "GitHub resource is unavailable to this connection"
	case http.StatusTooManyRequests:
		code = integrations.ErrorCodeRateLimited
		safeMessage = "GitHub rate limit was reached"
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		code = integrations.ErrorCodeInvalidInput
		safeMessage = "GitHub rejected the request parameters"
	default:
		if status >= http.StatusInternalServerError {
			code = integrations.ErrorCodeUpstream
			safeMessage = "GitHub is temporarily unavailable"
		} else {
			code = integrations.ErrorCodeUpstream
			safeMessage = "GitHub request failed"
		}
	}
	return integrations.NewProviderError(code, safeMessage, nil, diagnostics), diagnostics
}

func retryableGitHubError(status int, err error) bool {
	return integrations.ErrorCode(err) == integrations.ErrorCodeRateLimited ||
		status == http.StatusTooManyRequests || status == http.StatusInternalServerError ||
		status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func retryDelay(header http.Header, attempt int) time.Duration {
	if header != nil {
		if raw := strings.TrimSpace(header.Get("Retry-After")); raw != "" {
			if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
				return min(time.Duration(seconds)*time.Second, 5*time.Second)
			}
			if when, err := http.ParseTime(raw); err == nil {
				return min(max(time.Until(when), 0), 5*time.Second)
			}
		}
		if raw := strings.TrimSpace(header.Get("X-RateLimit-Reset")); raw != "" {
			if epoch, err := strconv.ParseInt(raw, 10, 64); err == nil {
				return min(max(time.Until(time.Unix(epoch, 0)), 0), 5*time.Second)
			}
		}
	}
	return time.Duration(attempt*attempt) * 100 * time.Millisecond
}

func githubRetryAfterAt(header http.Header) *time.Time {
	if raw := strings.TrimSpace(header.Get("Retry-After")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
			value := time.Now().Add(time.Duration(seconds) * time.Second).UTC()
			return &value
		}
		if value, err := http.ParseTime(raw); err == nil {
			value = value.UTC()
			return &value
		}
	}
	if raw := strings.TrimSpace(header.Get("X-RateLimit-Reset")); raw != "" {
		if epoch, err := strconv.ParseInt(raw, 10, 64); err == nil {
			value := time.Unix(epoch, 0).UTC()
			return &value
		}
	}
	return nil
}

func githubErrorMessage(payload []byte) string {
	var envelope struct {
		Message string `json:"message"`
	}
	if len(payload) == 0 || json.Unmarshal(payload, &envelope) != nil {
		return ""
	}
	message := strings.TrimSpace(envelope.Message)
	if len(message) > 512 {
		message = message[:512]
	}
	return message
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func parseScopeHeader(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		if _, exists := seen[part]; exists {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}
