package github

import (
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
	return &client{httpClient: httpClient, baseURL: parsed, maxAttempts: defaultMaxAttempts}, nil
}

type responseMeta struct {
	RequestID string
	Scopes    []string
	Attempts  int
}

func (c *client) getJSON(ctx context.Context, token, path string, query url.Values, target interface{}) (responseMeta, error) {
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
	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "GitHub request could not be created", err)
		}
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
		request.Header.Set("User-Agent", "ZGI-External-Integrations")
		response, err := c.httpClient.Do(request)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
				return responseMeta{Attempts: attempt}, integrations.NewError(integrations.ErrorCodeTimeout, "GitHub request timed out", ctx.Err())
			}
			lastErr = integrations.NewError(integrations.ErrorCodeUpstream, "GitHub is unavailable", err)
			if attempt < c.maxAttempts && waitForRetry(ctx, retryDelay(nil, attempt)) {
				continue
			}
			return responseMeta{Attempts: attempt}, lastErr
		}
		meta := responseMeta{
			RequestID: strings.TrimSpace(response.Header.Get("X-GitHub-Request-Id")),
			Scopes:    parseScopeHeader(response.Header.Get("X-OAuth-Scopes")),
			Attempts:  attempt,
		}
		payload, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		_ = response.Body.Close()
		if readErr != nil {
			return meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "GitHub response could not be read", readErr)
		}
		if len(payload) > maxResponseBytes {
			return meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "GitHub response exceeded the platform limit", nil)
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			mapped := mapGitHubStatus(response.StatusCode, response.Header)
			if retryableGitHubStatus(response.StatusCode) && attempt < c.maxAttempts && waitForRetry(ctx, retryDelay(response.Header, attempt)) {
				lastErr = mapped
				continue
			}
			return meta, mapped
		}
		if err := json.Unmarshal(payload, target); err != nil {
			return meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "GitHub returned an invalid response", err)
		}
		return meta, nil
	}
	return responseMeta{Attempts: c.maxAttempts}, lastErr
}

func mapGitHubStatus(status int, header http.Header) error {
	switch status {
	case http.StatusUnauthorized:
		return integrations.NewError(integrations.ErrorCodeAuthInvalid, "GitHub credentials are invalid", nil)
	case http.StatusPaymentRequired:
		return integrations.NewError(integrations.ErrorCodeBudgetExceeded, "GitHub billing prevents this request", nil)
	case http.StatusForbidden:
		if strings.TrimSpace(header.Get("X-RateLimit-Remaining")) == "0" {
			return integrations.NewError(integrations.ErrorCodeRateLimited, "GitHub rate limit was reached", nil)
		}
		return integrations.NewError(integrations.ErrorCodeAccessDenied, "GitHub denied access to this resource", nil)
	case http.StatusNotFound:
		// GitHub intentionally uses 404 for some private resources that the token
		// cannot access. Treat it as authorization, not as credential invalidity.
		return integrations.NewError(integrations.ErrorCodeAccessDenied, "GitHub resource is unavailable to this connection", nil)
	case http.StatusTooManyRequests:
		return integrations.NewError(integrations.ErrorCodeRateLimited, "GitHub rate limit was reached", nil)
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "GitHub rejected the request parameters", nil)
	default:
		if status >= http.StatusInternalServerError {
			return integrations.NewError(integrations.ErrorCodeUpstream, "GitHub is temporarily unavailable", nil)
		}
		return integrations.NewError(integrations.ErrorCodeUpstream, "GitHub request failed", nil)
	}
}

func retryableGitHubStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
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
	}
	return time.Duration(attempt*attempt) * 100 * time.Millisecond
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
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
