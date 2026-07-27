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
	RequestID string
	Attempts  int
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
			if retryable && attempt < attemptLimit && waitForRetry(ctx, time.Duration(attempt*attempt)*100*time.Millisecond) {
				continue
			}
			return responseMeta{Attempts: attempt}, lastErr
		}
		meta := responseMeta{
			RequestID: firstNonEmpty(response.Header.Get("X-Transaction-Id"), response.Header.Get("X-Request-Id")),
			Attempts:  attempt,
		}
		payload, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		_ = response.Body.Close()
		if readErr != nil {
			return meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "X response could not be read", readErr)
		}
		if len(payload) > maxResponseBytes {
			return meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "X response exceeded the platform limit", nil)
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			mapped := mapXStatus(response.StatusCode)
			if retryable && retryableXStatus(response.StatusCode) && attempt < attemptLimit &&
				waitForRetry(ctx, xRetryDelay(response.Header, attempt)) {
				lastErr = mapped
				continue
			}
			return meta, mapped
		}
		if target != nil && len(bytes.TrimSpace(payload)) > 0 {
			if err := json.Unmarshal(payload, target); err != nil {
				return meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "X returned an invalid response", err)
			}
		}
		return meta, nil
	}
	return responseMeta{Attempts: attemptLimit}, lastErr
}

func mapXStatus(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return integrations.NewError(integrations.ErrorCodeAuthInvalid, "X credentials are invalid or expired", nil)
	case http.StatusPaymentRequired:
		return integrations.NewError(integrations.ErrorCodeBudgetExceeded, "X plan does not permit this operation", nil)
	case http.StatusForbidden, http.StatusNotFound:
		return integrations.NewError(integrations.ErrorCodeAccessDenied, "X denied the requested operation or resource", nil)
	case http.StatusTooManyRequests:
		return integrations.NewError(integrations.ErrorCodeRateLimited, "X rate limit was reached", nil)
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "X rejected the request parameters", nil)
	default:
		if status >= http.StatusInternalServerError {
			return integrations.NewError(integrations.ErrorCodeUpstream, "X is temporarily unavailable", nil)
		}
		return integrations.NewError(integrations.ErrorCodeUpstream, "X request failed", nil)
	}
}

func retryableXStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusBadGateway ||
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

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
