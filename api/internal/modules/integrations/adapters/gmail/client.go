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
	RequestID   string
	Attempts    int
	Diagnostics integrations.ProviderDiagnostics
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
	return c.sendMessageInThread(ctx, accessToken, rawMessage, "", target)
}

func (c *client) listMessages(
	ctx context.Context,
	accessToken, searchQuery, pageToken string,
	maxResults int,
	includeSpamTrash bool,
	target interface{},
) (responseMeta, error) {
	query := url.Values{}
	query.Set("q", searchQuery)
	query.Set("maxResults", strconv.Itoa(maxResults))
	query.Set("includeSpamTrash", strconv.FormatBool(includeSpamTrash))
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	return c.doJSON(ctx, c.apiBaseURL, http.MethodGet, "/gmail/v1/users/me/messages", query, nil, accessToken, true, target)
}

func (c *client) getMessage(
	ctx context.Context,
	accessToken, messageID, format string,
	metadataHeaders []string,
	retryable bool,
	target interface{},
) (responseMeta, error) {
	query := url.Values{}
	query.Set("format", format)
	for _, header := range metadataHeaders {
		query.Add("metadataHeaders", header)
	}
	path := "/gmail/v1/users/me/messages/" + url.PathEscape(messageID)
	return c.doJSON(ctx, c.apiBaseURL, http.MethodGet, path, query, nil, accessToken, retryable, target)
}

func (c *client) sendMessageInThread(
	ctx context.Context,
	accessToken, rawMessage, threadID string,
	target interface{},
) (responseMeta, error) {
	body := map[string]string{"raw": rawMessage}
	if threadID != "" {
		body["threadId"] = threadID
	}
	return c.doJSON(ctx, c.apiBaseURL, http.MethodPost, "/gmail/v1/users/me/messages/send", nil, body, accessToken, false, target)
}

func (c *client) createDraft(ctx context.Context, accessToken, rawMessage string, target interface{}) (responseMeta, error) {
	body := map[string]interface{}{
		"message": map[string]string{"raw": rawMessage},
	}
	return c.doJSON(ctx, c.apiBaseURL, http.MethodPost, "/gmail/v1/users/me/drafts", nil, body, accessToken, false, target)
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
			if retryable && attempt < attemptLimit {
				if waitErr := waitForRetry(ctx, time.Duration(attempt*attempt)*100*time.Millisecond); waitErr == nil {
					continue
				} else {
					meta := responseMeta{Attempts: attempt}
					return meta, integrations.NewProviderError(
						integrations.ErrorCodeTimeout,
						"Google request timed out",
						waitErr,
						meta.Diagnostics,
					)
				}
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
		meta.Diagnostics = integrations.ProviderDiagnostics{
			RequestID:  meta.RequestID,
			HTTPStatus: response.StatusCode,
		}
		payload, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		_ = response.Body.Close()
		if readErr != nil {
			return meta, integrations.NewProviderError(
				integrations.ErrorCodeResponseInvalid,
				"Google response could not be read",
				readErr,
				meta.Diagnostics,
			)
		}
		if len(payload) > maxResponseBytes {
			return meta, integrations.NewProviderError(
				integrations.ErrorCodeResponseInvalid,
				"Google response exceeded the platform limit",
				nil,
				meta.Diagnostics,
			)
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			mapped, diagnostics := mapGoogleStatus(response.StatusCode, response.Header, payload, meta.RequestID)
			meta.Diagnostics = diagnostics
			if retryable && retryableGoogleError(response.StatusCode, mapped) && attempt < attemptLimit {
				lastErr = mapped
				if waitErr := waitForRetry(ctx, googleRetryDelay(response.Header, attempt)); waitErr == nil {
					continue
				} else {
					return meta, integrations.NewProviderError(
						integrations.ErrorCodeTimeout,
						"Google request timed out",
						waitErr,
						meta.Diagnostics,
					)
				}
			}
			return meta, mapped
		}
		if target != nil && len(bytes.TrimSpace(payload)) > 0 {
			if err := json.Unmarshal(payload, target); err != nil {
				return meta, integrations.NewProviderError(
					integrations.ErrorCodeResponseInvalid,
					"Google returned an invalid response",
					err,
					meta.Diagnostics,
				)
			}
		}
		return meta, nil
	}
	return responseMeta{Attempts: attemptLimit}, lastErr
}

func mapGoogleStatus(status int, header http.Header, payload []byte, requestID string) (error, integrations.ProviderDiagnostics) {
	reason, providerStatus := googleErrorReason(payload)
	diagnosticCode := reason
	if diagnosticCode == "" {
		diagnosticCode = providerStatus
	}
	diagnostics := integrations.ProviderDiagnostics{
		ErrorCode:    diagnosticCode,
		RequestID:    requestID,
		HTTPStatus:   status,
		RetryAfterAt: retryAfterAt(header),
	}
	code := ""
	message := ""
	switch reason {
	case "rateLimitExceeded", "userRateLimitExceeded":
		code = integrations.ErrorCodeRateLimited
		message = "Google rate limit was reached"
	case "dailyLimitExceeded":
		code = integrations.ErrorCodeBudgetExceeded
		message = "Google daily usage limit was reached"
	case "domainPolicy", "insufficientPermissions":
		code = integrations.ErrorCodeAccessDenied
		message = "Google denied the requested operation or scope"
	}
	if code != "" {
		return integrations.NewProviderError(code, message, nil, diagnostics), diagnostics
	}
	switch status {
	case http.StatusUnauthorized:
		code = integrations.ErrorCodeAuthInvalid
		message = "Google credentials are invalid or expired"
	case http.StatusForbidden:
		code = integrations.ErrorCodeAccessDenied
		message = "Google denied the requested operation or scope"
	case http.StatusNotFound:
		code = integrations.ErrorCodeAccessDenied
		message = "Google resource is unavailable to this connection"
	case http.StatusTooManyRequests:
		code = integrations.ErrorCodeRateLimited
		message = "Google rate limit was reached"
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		code = integrations.ErrorCodeInvalidInput
		message = "Google rejected the request parameters"
	default:
		if status >= http.StatusInternalServerError {
			code = integrations.ErrorCodeUpstream
			message = "Google is temporarily unavailable"
		} else {
			code = integrations.ErrorCodeUpstream
			message = "Google request failed"
		}
	}
	return integrations.NewProviderError(code, message, nil, diagnostics), diagnostics
}

func retryableGoogleError(status int, err error) bool {
	return integrations.ErrorCode(err) == integrations.ErrorCodeRateLimited ||
		status == http.StatusTooManyRequests || status == http.StatusInternalServerError ||
		status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func googleErrorReason(payload []byte) (string, string) {
	var envelope struct {
		Error struct {
			Status string `json:"status"`
			Errors []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	if len(bytes.TrimSpace(payload)) == 0 || json.Unmarshal(payload, &envelope) != nil {
		return "", ""
	}
	status := strings.TrimSpace(envelope.Error.Status)
	for _, item := range envelope.Error.Errors {
		switch reason := strings.TrimSpace(item.Reason); reason {
		case "rateLimitExceeded", "userRateLimitExceeded", "dailyLimitExceeded", "domainPolicy", "insufficientPermissions":
			return reason, status
		}
	}
	return "", status
}

func retryAfterAt(header http.Header) *time.Time {
	raw := strings.TrimSpace(header.Get("Retry-After"))
	if raw == "" {
		return nil
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
		value := time.Now().Add(time.Duration(seconds) * time.Second).UTC()
		return &value
	}
	if value, err := http.ParseTime(raw); err == nil {
		value = value.UTC()
		return &value
	}
	return nil
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
