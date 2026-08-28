package exa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/observability"
)

const (
	officialBaseURL = "https://api.exa.ai"
	maxResponseSize = 5 << 20
)

type client struct {
	baseURL string
	http    *http.Client
}

func newClient(baseURL string, httpClient *http.Client) *client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = officialBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &client{baseURL: strings.TrimRight(baseURL, "/"), http: observability.HTTPClient(httpClient)}
}

func (c *client) post(ctx context.Context, apiKey, path string, input interface{}, output interface{}) (int, string, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return 0, "", integrations.NewError(integrations.ErrorCodeInvalidInput, "integration request could not be encoded", err)
	}
	var requestID string
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return attempt, requestID, integrations.NewError(integrations.ErrorCodeUpstream, "integration request could not be created", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("x-api-key", strings.TrimSpace(apiKey))

		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return attempt, requestID, integrations.NewError(integrations.ErrorCodeTimeout, "external integration request timed out", ctx.Err())
			}
			if attempt < 3 {
				if err := waitForRetry(ctx, retryDelay(attempt, "")); err != nil {
					return attempt, requestID, integrations.NewError(integrations.ErrorCodeTimeout, "external integration request timed out", err)
				}
				continue
			}
			return attempt, requestID, integrations.NewError(integrations.ErrorCodeUpstream, "external integration is unavailable", err)
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
		_ = resp.Body.Close()
		diagnostics := integrations.ProviderDiagnostics{
			RequestID:    requestID,
			HTTPStatus:   resp.StatusCode,
			RetryAfterAt: exaRetryAfterAt(resp.Header),
		}
		if readErr != nil {
			if ctx.Err() != nil {
				return attempt, requestID, integrations.NewProviderError(
					integrations.ErrorCodeTimeout,
					"external integration request timed out",
					ctx.Err(),
					diagnostics,
				)
			}
			return attempt, requestID, integrations.NewProviderError(
				integrations.ErrorCodeResponseInvalid,
				"integration response could not be read",
				readErr,
				diagnostics,
			)
		}
		if len(body) > maxResponseSize {
			return attempt, requestID, integrations.NewProviderError(
				integrations.ErrorCodeResponseInvalid,
				"integration response exceeded the platform limit",
				nil,
				diagnostics,
			)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if err := json.Unmarshal(body, output); err != nil {
				return attempt, requestID, integrations.NewProviderError(
					integrations.ErrorCodeResponseInvalid,
					"integration returned malformed JSON",
					err,
					diagnostics,
				)
			}
			requestID = responseRequestID(output)
			return attempt, requestID, nil
		}

		var upstreamError errorResponse
		_ = json.Unmarshal(body, &upstreamError)
		if strings.TrimSpace(upstreamError.RequestID) != "" {
			requestID = boundedRequestID(upstreamError.RequestID)
		}
		diagnostics.RequestID = requestID
		diagnostics.ErrorCode = boundedProviderCode(upstreamError.Tag)
		if diagnostics.ErrorCode == "" {
			diagnostics.ErrorCode = exaStatusErrorCode(upstreamError.Error)
		}
		if retryableStatus(resp.StatusCode) && attempt < 3 {
			if err := waitForRetry(ctx, retryDelay(attempt, resp.Header.Get("Retry-After"))); err != nil {
				return attempt, requestID, integrations.NewProviderError(
					integrations.ErrorCodeTimeout,
					"external integration request timed out",
					err,
					diagnostics,
				)
			}
			continue
		}
		return attempt, requestID, statusError(resp.StatusCode, diagnostics)
	}
	return 3, requestID, integrations.NewError(integrations.ErrorCodeUpstream, "external integration is unavailable", nil)
}

func responseRequestID(output interface{}) string {
	if typed, ok := output.(*response); ok && typed != nil {
		return boundedRequestID(typed.RequestID)
	}
	return ""
}

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusInternalServerError || status == http.StatusBadGateway || status == http.StatusServiceUnavailable
}

func statusError(status int, diagnostics integrations.ProviderDiagnostics) error {
	code := ""
	message := ""
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		code = integrations.ErrorCodeInvalidInput
		message = "external integration rejected the request"
	case http.StatusUnauthorized:
		code = integrations.ErrorCodeAuthInvalid
		message = "external integration authentication is invalid"
	case http.StatusPaymentRequired:
		code = integrations.ErrorCodeBudgetExceeded
		message = "external integration budget has been exhausted"
	case http.StatusForbidden:
		code = integrations.ErrorCodeAccessDenied
		message = "external integration denied access"
	case http.StatusTooManyRequests:
		code = integrations.ErrorCodeRateLimited
		message = "external integration rate limit was reached"
	default:
		code = integrations.ErrorCodeUpstream
		message = fmt.Sprintf("external integration returned HTTP %d", status)
	}
	return integrations.NewProviderError(code, message, nil, diagnostics)
}

func retryDelay(attempt int, retryAfter string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds >= 0 {
		if seconds > 86400 {
			seconds = 86400
		}
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(strings.TrimSpace(retryAfter)); err == nil {
		delay := time.Until(at)
		if delay > 24*time.Hour {
			delay = 24 * time.Hour
		}
		if delay > 0 {
			return delay
		}
	}
	return time.Duration(250*(1<<(attempt-1))) * time.Millisecond
}

func exaRetryAfterAt(header http.Header) *time.Time {
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

func boundedProviderCode(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9',
			char == '.', char == '_', char == ':':
			builder.WriteRune(char)
		default:
			builder.WriteByte('_')
		}
		if builder.Len() >= 128 {
			break
		}
	}
	return strings.Trim(builder.String(), "_")
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
