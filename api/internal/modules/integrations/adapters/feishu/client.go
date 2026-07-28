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
	RequestID   string
	Attempts    int
	Diagnostics integrations.ProviderDiagnostics
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

func (c *client) searchUsers(ctx context.Context, region, accessToken string, query url.Values, target interface{}) (responseMeta, error) {
	return c.doEnvelope(ctx, region, http.MethodGet, "/open-apis/search/v1/user", query, nil, accessToken, true, target)
}

func (c *client) listChats(ctx context.Context, region, accessToken string, query url.Values, target interface{}) (responseMeta, error) {
	return c.doEnvelope(ctx, region, http.MethodGet, "/open-apis/im/v1/chats", query, nil, accessToken, true, target)
}

func (c *client) listCalendars(ctx context.Context, region, accessToken string, query url.Values, target interface{}) (responseMeta, error) {
	return c.doEnvelope(ctx, region, http.MethodGet, "/open-apis/calendar/v4/calendars", query, nil, accessToken, true, target)
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
		meta.Diagnostics.ErrorCode = strconv.Itoa(response.Code)
		return "", meta, withFeishuDiagnostics(mapFeishuBusinessCode(response.Code), meta)
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
		meta.Diagnostics.ErrorCode = strconv.Itoa(envelope.Code)
		return meta, withFeishuDiagnostics(mapFeishuBusinessCode(envelope.Code), meta)
	}
	if target != nil && len(bytes.TrimSpace(envelope.Data)) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, target); err != nil {
			return meta, withFeishuDiagnostics(
				integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu returned an invalid response", err),
				meta,
			)
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
				meta := responseMeta{Attempts: attempt}
				return meta, withFeishuDiagnostics(
					integrations.NewError(integrations.ErrorCodeTimeout, "Feishu request timed out", ctx.Err()),
					meta,
				)
			}
			lastErr = integrations.NewError(integrations.ErrorCodeUpstream, "Feishu is unavailable", requestErr)
			if retryable && attempt < attemptLimit {
				if waitErr := waitForRetry(ctx, time.Duration(attempt*attempt)*100*time.Millisecond); waitErr == nil {
					continue
				} else {
					meta := responseMeta{Attempts: attempt}
					return meta, withFeishuDiagnostics(
						integrations.NewError(integrations.ErrorCodeTimeout, "Feishu request timed out", waitErr),
						meta,
					)
				}
			}
			meta := responseMeta{Attempts: attempt}
			return meta, withFeishuDiagnostics(lastErr, meta)
		}
		retryAfterAt := feishuRetryAfterAt(response.Header)
		meta := responseMeta{
			RequestID: firstNonEmpty(response.Header.Get("X-Tt-Logid"), response.Header.Get("X-Request-Id")),
			Attempts:  attempt,
		}
		meta.Diagnostics = integrations.ProviderDiagnostics{
			RequestID:    meta.RequestID,
			HTTPStatus:   response.StatusCode,
			RetryAfterAt: retryAfterAt,
		}
		payload, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		_ = response.Body.Close()
		if readErr != nil {
			return meta, withFeishuDiagnostics(
				integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu response could not be read", readErr),
				meta,
			)
		}
		if len(payload) > maxResponseBytes {
			return meta, withFeishuDiagnostics(
				integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu response exceeded the platform limit", nil),
				meta,
			)
		}
		businessCode, hasBusinessCode := parseFeishuBusinessCode(payload)
		if hasBusinessCode && businessCode != 0 {
			meta.Diagnostics.ErrorCode = strconv.Itoa(businessCode)
			mapped := mapFeishuBusinessCode(businessCode)
			if retryable && retryableFeishuBusinessCode(businessCode) && attempt < attemptLimit {
				lastErr = withFeishuDiagnostics(mapped, meta)
				if waitErr := waitForRetry(ctx, feishuRetryDelay(response.Header, attempt)); waitErr == nil {
					continue
				} else {
					return meta, withFeishuDiagnostics(
						integrations.NewError(integrations.ErrorCodeTimeout, "Feishu request timed out", waitErr),
						meta,
					)
				}
			}
			return meta, withFeishuDiagnostics(mapped, meta)
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			meta.Diagnostics.ErrorCode = "http_" + strconv.Itoa(response.StatusCode)
			mapped := mapFeishuStatus(response.StatusCode)
			if retryable && retryableFeishuStatus(response.StatusCode) && attempt < attemptLimit {
				lastErr = withFeishuDiagnostics(mapped, meta)
				if waitErr := waitForRetry(ctx, feishuRetryDelay(response.Header, attempt)); waitErr == nil {
					continue
				} else {
					return meta, withFeishuDiagnostics(
						integrations.NewError(integrations.ErrorCodeTimeout, "Feishu request timed out", waitErr),
						meta,
					)
				}
			}
			return meta, withFeishuDiagnostics(mapped, meta)
		}
		if target != nil {
			if err := json.Unmarshal(payload, target); err != nil {
				return meta, withFeishuDiagnostics(
					integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu returned an invalid response", err),
					meta,
				)
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
		if status >= http.StatusBadRequest {
			return integrations.NewError(integrations.ErrorCodeProviderRejected, "Feishu rejected the request", nil)
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
	case 230020, 232019:
		return integrations.NewError(integrations.ErrorCodeRateLimited, "Feishu rate limit was reached", nil)
	case 230001, 232006, 1061002, 1770001, 1770002, 1770003:
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu rejected the request parameters", nil)
	case 230002, 230006, 230013, 230027, 230035, 230050,
		232010, 232011, 232033, 232034, 1770032:
		return integrations.NewError(integrations.ErrorCodeAccessDenied, "Feishu denied the requested scope or resource", nil)
	case 232025, 190007:
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu application bot capability is not enabled", nil)
	case 1770033:
		return integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu response exceeded the platform limit", nil)
	case 1061001:
		return integrations.NewError(integrations.ErrorCodeUpstream, "Feishu is temporarily unavailable", nil)
	case 190002, 190008, 190009, 191001:
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu rejected the request parameters", nil)
	case 190003:
		return integrations.NewError(integrations.ErrorCodeUpstream, "Feishu is temporarily unavailable", nil)
	case 190004, 190005, 190010:
		return integrations.NewError(integrations.ErrorCodeRateLimited, "Feishu rate limit was reached", nil)
	case 190006:
		return integrations.NewError(integrations.ErrorCodeAuthInvalid, "Feishu tenant application credentials are invalid", nil)
	case 191000, 191003, 191004:
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu calendar is missing, deleted, or incompatible with this action", nil)
	case 191002:
		return integrations.NewError(integrations.ErrorCodeAccessDenied, "Feishu denied the requested scope or resource", nil)
	case 195100:
		return integrations.NewError(integrations.ErrorCodeAccessDenied, "Feishu user is not available in the connected tenant", nil)
	case 99991663, 99991664, 99991668:
		return integrations.NewError(integrations.ErrorCodeAuthInvalid, "Feishu credentials are invalid or expired", nil)
	case 99991661, 99991672:
		return integrations.NewError(integrations.ErrorCodeAccessDenied, "Feishu denied the requested scope or resource", nil)
	case 99991400, 99991401:
		return integrations.NewError(integrations.ErrorCodeRateLimited, "Feishu rate limit was reached", nil)
	default:
		return integrations.NewError(integrations.ErrorCodeProviderRejected, "Feishu rejected the request", nil)
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
	return code == 20050 || code == 20072 || code == 230020 || code == 232019 ||
		code == 1061001 || code == 190003 || code == 190004 || code == 190005 || code == 190010
}

func retryableFeishuStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusInternalServerError ||
		status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func feishuRetryDelay(header http.Header, attempt int) time.Duration {
	if raw := strings.TrimSpace(header.Get("Retry-After")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
			return time.Duration(seconds) * time.Second
		}
		if retryAt, err := http.ParseTime(raw); err == nil {
			return max(time.Until(retryAt), 0)
		}
	}
	return time.Duration(attempt*attempt) * 100 * time.Millisecond
}

func feishuRetryAfterAt(header http.Header) *time.Time {
	raw := strings.TrimSpace(header.Get("Retry-After"))
	if raw == "" {
		return nil
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
		retryAt := time.Now().UTC().Add(time.Duration(seconds) * time.Second)
		return &retryAt
	}
	if retryAt, err := http.ParseTime(raw); err == nil {
		normalized := retryAt.UTC()
		return &normalized
	}
	return nil
}

func withFeishuDiagnostics(err error, meta responseMeta) error {
	if err == nil {
		return nil
	}
	return integrations.NewProviderError(
		integrations.ErrorCode(err),
		err.Error(),
		err,
		meta.Diagnostics,
	)
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
