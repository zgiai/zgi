package gmail

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
	defaultAPIBaseURL      = "https://gmail.googleapis.com"
	defaultIdentityBaseURL = "https://openidconnect.googleapis.com"
	defaultClientTimeout   = 20 * time.Second
	maxResponseBytes       = 2 << 20
	maxReadAttempts        = 3
)

type client struct {
	httpClient      *http.Client
	apiBaseURL      *url.URL
	identityBaseURL *url.URL
}

type responseMeta struct {
	RequestID string
	Attempts  int
}

func newClient(httpClient *http.Client) (*client, error) {
	return newClientForBaseURLs(httpClient, defaultAPIBaseURL, defaultIdentityBaseURL)
}

// newClientForBaseURLs is package-private and exists only for local httptest
// servers. Production construction fixes both credential-bearing destinations.
func newClientForBaseURLs(httpClient *http.Client, apiBaseURL, identityBaseURL string) (*client, error) {
	apiBase, err := parseBaseURL(apiBaseURL)
	if err != nil {
		return nil, fmt.Errorf("initialize Gmail API endpoint: %w", err)
	}
	identityBase, err := parseBaseURL(identityBaseURL)
	if err != nil {
		return nil, fmt.Errorf("initialize Google identity endpoint: %w", err)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultClientTimeout}
	}
	httpClientCopy := *httpClient
	httpClientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &client{httpClient: &httpClientCopy, apiBaseURL: apiBase, identityBaseURL: identityBase}, nil
}

func parseBaseURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("endpoint is invalid")
	}
	return parsed, nil
}

func (c *client) getIdentity(ctx context.Context, accessToken string, target interface{}) (responseMeta, error) {
	return c.doJSON(ctx, c.identityBaseURL, http.MethodGet, "/v1/userinfo", nil, nil, accessToken, true, target)
}

func (c *client) sendMessage(ctx context.Context, accessToken, rawMessage string, target interface{}) (responseMeta, error) {
	body := map[string]string{"raw": rawMessage}
	return c.doJSON(ctx, c.apiBaseURL, http.MethodPost, "/gmail/v1/users/me/messages/send", nil, body, accessToken, false, target)
}

func (c *client) doJSON(
	ctx context.Context,
	baseURL *url.URL,
	method, path string,
	query url.Values,
	body interface{},
	accessToken string,
	retryable bool,
	target interface{},
) (responseMeta, error) {
	if c == nil || c.httpClient == nil || baseURL == nil {
		return responseMeta{}, integrations.NewError(integrations.ErrorCodeUpstream, "Google client is unavailable", nil)
	}
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return responseMeta{}, integrations.NewError(integrations.ErrorCodeAuthInvalid, "Google credentials are unavailable", nil)
	}
	endpoint := *baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + strings.TrimLeft(path, "/")
	if query != nil {
		endpoint.RawQuery = query.Encode()
	}
	var encodedBody []byte
	var err error
	if body != nil {
		encodedBody, err = json.Marshal(body)
		if err != nil {
			return responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Google request could not be encoded", err)
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
			return responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Google request could not be created", requestErr)
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
				return responseMeta{Attempts: attempt}, integrations.NewError(integrations.ErrorCodeTimeout, "Google request timed out", ctx.Err())
			}
			lastErr = integrations.NewError(integrations.ErrorCodeUpstream, "Google is unavailable", requestErr)
			if retryable && attempt < attemptLimit && waitForRetry(ctx, time.Duration(attempt*attempt)*100*time.Millisecond) {
				continue
			}
			return responseMeta{Attempts: attempt}, lastErr
		}
		meta := responseMeta{
			RequestID: firstNonEmpty(
				response.Header.Get("X-GUploader-UploadID"),
				response.Header.Get("X-Google-Request-ID"),
			),
			Attempts: attempt,
		}
		payload, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		_ = response.Body.Close()
		if readErr != nil {
			return meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Google response could not be read", readErr)
		}
		if len(payload) > maxResponseBytes {
			return meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Google response exceeded the platform limit", nil)
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			mapped := mapGoogleStatus(response.StatusCode, response.Header)
			if retryable && retryableGoogleStatus(response.StatusCode) && attempt < attemptLimit &&
				waitForRetry(ctx, googleRetryDelay(response.Header, attempt)) {
				lastErr = mapped
				continue
			}
			return meta, mapped
		}
		if target != nil && len(bytes.TrimSpace(payload)) > 0 {
			if err := json.Unmarshal(payload, target); err != nil {
				return meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Google returned an invalid response", err)
			}
		}
		return meta, nil
	}
	return responseMeta{Attempts: attemptLimit}, lastErr
}

func mapGoogleStatus(status int, header http.Header) error {
	switch status {
	case http.StatusUnauthorized:
		return integrations.NewError(integrations.ErrorCodeAuthInvalid, "Google credentials are invalid or expired", nil)
	case http.StatusForbidden:
		return integrations.NewError(integrations.ErrorCodeAccessDenied, "Google denied the requested operation or scope", nil)
	case http.StatusNotFound:
		return integrations.NewError(integrations.ErrorCodeAccessDenied, "Google resource is unavailable to this connection", nil)
	case http.StatusTooManyRequests:
		return integrations.NewError(integrations.ErrorCodeRateLimited, "Google rate limit was reached", nil)
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "Google rejected the request parameters", nil)
	default:
		if status >= http.StatusInternalServerError {
			return integrations.NewError(integrations.ErrorCodeUpstream, "Google is temporarily unavailable", nil)
		}
		_ = header
		return integrations.NewError(integrations.ErrorCodeUpstream, "Google request failed", nil)
	}
}

func retryableGoogleStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func googleRetryDelay(header http.Header, attempt int) time.Duration {
	if raw := strings.TrimSpace(header.Get("Retry-After")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
			return min(time.Duration(seconds)*time.Second, 5*time.Second)
		}
		if when, err := http.ParseTime(raw); err == nil {
			return min(max(time.Until(when), 0), 5*time.Second)
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
