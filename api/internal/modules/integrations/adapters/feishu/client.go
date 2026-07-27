package feishu

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
	defaultCNBaseURL     = "https://open.feishu.cn"
	defaultGlobalBaseURL = "https://open.larksuite.com"
	defaultClientTimeout = 20 * time.Second
	maxResponseBytes     = 2 << 20
	maxReadAttempts      = 3
)

type client struct {
	httpClient    *http.Client
	cnBaseURL     *url.URL
	globalBaseURL *url.URL
}

type responseMeta struct {
	RequestID string
	Attempts  int
}

type apiEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func newClient(httpClient *http.Client) (*client, error) {
	return newClientForBaseURLs(httpClient, defaultCNBaseURL, defaultGlobalBaseURL)
}

// newClientForBaseURLs exists only for package-local httptest servers.
// Production construction pins both regional credential-bearing hosts.
func newClientForBaseURLs(httpClient *http.Client, cnBaseURL, globalBaseURL string) (*client, error) {
	cnBase, err := parseBaseURL(cnBaseURL)
	if err != nil {
		return nil, fmt.Errorf("initialize Feishu API endpoint: %w", err)
	}
	globalBase, err := parseBaseURL(globalBaseURL)
	if err != nil {
		return nil, fmt.Errorf("initialize Lark API endpoint: %w", err)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultClientTimeout}
	}
	httpClientCopy := *httpClient
	httpClientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &client{httpClient: &httpClientCopy, cnBaseURL: cnBase, globalBaseURL: globalBase}, nil
}

func parseBaseURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("endpoint is invalid")
	}
	return parsed, nil
}

func (c *client) getUserInfo(ctx context.Context, region, accessToken string, target interface{}) (responseMeta, error) {
	return c.doEnvelope(ctx, region, http.MethodGet, "/open-apis/authen/v1/user_info", nil, nil, accessToken, true, target)
}

func (c *client) listDriveFiles(ctx context.Context, region, accessToken string, query url.Values, target interface{}) (responseMeta, error) {
	return c.doEnvelope(ctx, region, http.MethodGet, "/open-apis/drive/v1/files", query, nil, accessToken, true, target)
}

func (c *client) getDocumentRawContent(ctx context.Context, region, accessToken, documentID string, target interface{}) (responseMeta, error) {
	path := "/open-apis/docx/v1/documents/" + url.PathEscape(documentID) + "/raw_content"
	return c.doEnvelope(ctx, region, http.MethodGet, path, nil, nil, accessToken, true, target)
}

func (c *client) sendMessage(ctx context.Context, region, accessToken, receiveIDType string, body interface{}, target interface{}) (responseMeta, error) {
	query := url.Values{"receive_id_type": []string{receiveIDType}}
	// Sending a message is non-idempotent. Never retry it automatically.
	return c.doEnvelope(ctx, region, http.MethodPost, "/open-apis/im/v1/messages", query, body, accessToken, false, target)
}

func (c *client) tenantAccessToken(ctx context.Context, region, appID, appSecret string) (string, responseMeta, error) {
	var response struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	meta, err := c.doRawJSON(ctx, region, http.MethodPost, "/open-apis/auth/v3/tenant_access_token/internal", nil, map[string]string{
		"app_id": appID, "app_secret": appSecret,
	}, "", true, &response)
	if err != nil {
		return "", meta, err
	}
	if response.Code != 0 {
		return "", meta, mapFeishuBusinessCode(response.Code)
	}
	token := strings.TrimSpace(response.TenantAccessToken)
	if token == "" {
		return "", meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu token response is incomplete", nil)
	}
	return token, meta, nil
}

func (c *client) getTenant(ctx context.Context, region, tenantToken string, target interface{}) (responseMeta, error) {
	return c.doEnvelope(ctx, region, http.MethodGet, "/open-apis/tenant/v2/tenant/query", nil, nil, tenantToken, true, target)
}

func (c *client) doEnvelope(
	ctx context.Context,
	region, method, path string,
	query url.Values,
	body interface{},
	token string,
	retryable bool,
	target interface{},
) (responseMeta, error) {
	var envelope apiEnvelope
	meta, err := c.doRawJSON(ctx, region, method, path, query, body, token, retryable, &envelope)
	if err != nil {
		return meta, err
	}
	if envelope.Code != 0 {
		return meta, mapFeishuBusinessCode(envelope.Code)
	}
	if target != nil && len(bytes.TrimSpace(envelope.Data)) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, target); err != nil {
			return meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu returned an invalid response", err)
		}
	}
	return meta, nil
}

func (c *client) doRawJSON(
	ctx context.Context,
	region, method, path string,
	query url.Values,
	body interface{},
	token string,
	retryable bool,
	target interface{},
) (responseMeta, error) {
	if c == nil || c.httpClient == nil {
		return responseMeta{}, integrations.NewError(integrations.ErrorCodeUpstream, "Feishu client is unavailable", nil)
	}
	baseURL, err := c.baseURL(region)
	if err != nil {
		return responseMeta{}, err
	}
	endpoint := *baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + strings.TrimLeft(path, "/")
	if query != nil {
		endpoint.RawQuery = query.Encode()
	}
	var encodedBody []byte
	if body != nil {
		encodedBody, err = json.Marshal(body)
		if err != nil {
			return responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu request could not be encoded", err)
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
			return responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu request could not be created", requestErr)
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("User-Agent", "ZGI-External-Integrations/1.0")
		if strings.TrimSpace(token) != "" {
			request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
		}
		if encodedBody != nil {
			request.Header.Set("Content-Type", "application/json; charset=utf-8")
		}
		response, requestErr := c.httpClient.Do(request)
		if requestErr != nil {
			if ctx.Err() != nil || errors.Is(requestErr, context.DeadlineExceeded) {
				return responseMeta{Attempts: attempt}, integrations.NewError(integrations.ErrorCodeTimeout, "Feishu request timed out", ctx.Err())
			}
			lastErr = integrations.NewError(integrations.ErrorCodeUpstream, "Feishu is unavailable", requestErr)
			if retryable && attempt < attemptLimit && waitForRetry(ctx, time.Duration(attempt*attempt)*100*time.Millisecond) {
				continue
			}
			return responseMeta{Attempts: attempt}, lastErr
		}
		meta := responseMeta{
			RequestID: firstNonEmpty(response.Header.Get("X-Tt-Logid"), response.Header.Get("X-Request-Id")),
			Attempts:  attempt,
		}
		payload, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		_ = response.Body.Close()
		if readErr != nil {
			return meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu response could not be read", readErr)
		}
		if len(payload) > maxResponseBytes {
			return meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu response exceeded the platform limit", nil)
		}
		businessCode, hasBusinessCode := parseFeishuBusinessCode(payload)
		if hasBusinessCode && businessCode != 0 {
			mapped := mapFeishuBusinessCode(businessCode)
			if retryable && retryableFeishuBusinessCode(businessCode) && attempt < attemptLimit &&
				waitForRetry(ctx, feishuRetryDelay(response.Header, attempt)) {
				lastErr = mapped
				continue
			}
			return meta, mapped
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			mapped := mapFeishuStatus(response.StatusCode)
			if retryable && retryableFeishuStatus(response.StatusCode) && attempt < attemptLimit &&
				waitForRetry(ctx, feishuRetryDelay(response.Header, attempt)) {
				lastErr = mapped
				continue
			}
			return meta, mapped
		}
		if target != nil {
			if err := json.Unmarshal(payload, target); err != nil {
				return meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu returned an invalid response", err)
			}
		}
		return meta, nil
	}
	return responseMeta{Attempts: attemptLimit}, lastErr
}

func (c *client) baseURL(region string) (*url.URL, error) {
	switch strings.ToLower(strings.TrimSpace(region)) {
	case "", RegionCN:
		return c.cnBaseURL, nil
	case RegionGlobal:
		return c.globalBaseURL, nil
	default:
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu region is invalid", nil)
	}
}

func mapFeishuStatus(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return integrations.NewError(integrations.ErrorCodeAuthInvalid, "Feishu credentials are invalid or expired", nil)
	case http.StatusForbidden, http.StatusNotFound:
		return integrations.NewError(integrations.ErrorCodeAccessDenied, "Feishu denied access to the requested resource", nil)
	case http.StatusTooManyRequests:
		return integrations.NewError(integrations.ErrorCodeRateLimited, "Feishu rate limit was reached", nil)
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu rejected the request parameters", nil)
	default:
		if status >= http.StatusInternalServerError {
			return integrations.NewError(integrations.ErrorCodeUpstream, "Feishu is temporarily unavailable", nil)
		}
		return integrations.NewError(integrations.ErrorCodeUpstream, "Feishu request failed", nil)
	}
}

func mapFeishuBusinessCode(code int) error {
	switch code {
	case 20002, 20026, 20037, 20049, 20064, 20073:
		return integrations.NewError(integrations.ErrorCodeAuthInvalid, "Feishu authorization is invalid or expired", nil)
	case 20050, 20072:
		return integrations.NewError(integrations.ErrorCodeUpstream, "Feishu authorization service is temporarily unavailable", nil)
	case 230020:
		return integrations.NewError(integrations.ErrorCodeRateLimited, "Feishu rate limit was reached", nil)
	case 230027, 230035:
		return integrations.NewError(integrations.ErrorCodeAccessDenied, "Feishu denied the requested scope or resource", nil)
	case 99991663, 99991664, 99991668:
		return integrations.NewError(integrations.ErrorCodeAuthInvalid, "Feishu credentials are invalid or expired", nil)
	case 99991661, 99991672:
		return integrations.NewError(integrations.ErrorCodeAccessDenied, "Feishu denied the requested scope or resource", nil)
	case 99991400, 99991401:
		return integrations.NewError(integrations.ErrorCodeRateLimited, "Feishu rate limit was reached", nil)
	default:
		return integrations.NewError(integrations.ErrorCodeUpstream, "Feishu rejected the request", nil)
	}
}

func parseFeishuBusinessCode(payload []byte) (int, bool) {
	var envelope struct {
		Code *int `json:"code"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.Code == nil {
		return 0, false
	}
	return *envelope.Code, true
}

func retryableFeishuBusinessCode(code int) bool {
	return code == 20050 || code == 20072 || code == 230020
}

func retryableFeishuStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func feishuRetryDelay(header http.Header, attempt int) time.Duration {
	if raw := strings.TrimSpace(header.Get("Retry-After")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
			return min(time.Duration(seconds)*time.Second, 5*time.Second)
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
