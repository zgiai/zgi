package dingtalk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providertransport"

	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"strings"
	"sync"
	"time"
)

const (
	defaultAPIBaseURL    = "https://api.dingtalk.com"
	defaultLegacyBaseURL = "https://oapi.dingtalk.com"
)

type Adapter struct {
	client        *http.Client
	apiBaseURL    string
	legacyBaseURL string
	mu            sync.Mutex
	tokens        map[string]cachedToken
}
type cachedToken struct {
	value          string
	expiresAt      time.Time
	credentialHash [32]byte
}
type credentials struct {
	AppKey, AppSecret, AgentID, ConnectionID string
}

type apiErrorEnvelope struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestid"`
}

func New(httpClient *http.Client) (*Adapter, error) {
	return newForBaseURLs(httpClient, defaultAPIBaseURL, defaultLegacyBaseURL)
}
func newForBaseURLs(httpClient *http.Client, apiBaseURL, legacyBaseURL string) (*Adapter, error) {
	for _, raw := range []string{apiBaseURL, legacyBaseURL} {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("invalid DingTalk base URL")
		}
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	} else {
		clone := *httpClient
		httpClient = &clone
		if httpClient.Timeout <= 0 {
			httpClient.Timeout = 20 * time.Second
		}
	}
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &Adapter{client: httpClient, apiBaseURL: strings.TrimRight(apiBaseURL, "/"), legacyBaseURL: strings.TrimRight(legacyBaseURL, "/"), tokens: map[string]cachedToken{}}, nil
}

func (adapter *Adapter) DriverID() string { return DriverID }

func (adapter *Adapter) accessToken(ctx context.Context, creds credentials) (string, error) {
	hash := sha256.Sum256([]byte(creds.AppKey + "\x00" + creds.AppSecret + "\x00" + creds.AgentID))
	adapter.mu.Lock()
	cached, ok := adapter.tokens[creds.ConnectionID]
	adapter.mu.Unlock()
	if ok && cached.credentialHash == hash && time.Now().Before(cached.expiresAt) {
		return cached.value, nil
	}
	status, raw, err := adapter.doJSON(ctx, http.MethodPost, adapter.apiBaseURL+"/v1.0/oauth2/accessToken", nil, nil, map[string]string{"appKey": creds.AppKey, "appSecret": creds.AppSecret})
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		var envelope apiErrorEnvelope
		_ = json.Unmarshal(raw, &envelope)
		return "", mapAPIError(status, envelope)
	}
	var response struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int    `json:"expireIn"`
	}
	if err := json.Unmarshal(raw, &response); err != nil || strings.TrimSpace(response.AccessToken) == "" {
		return "", dingError(providertransport.ErrorCodeResponseInvalid, "DingTalk access token response is incomplete", err)
	}
	ttl := time.Duration(response.ExpireIn)*time.Second - 5*time.Minute
	if ttl < time.Minute {
		ttl = time.Minute
	}
	adapter.mu.Lock()
	adapter.tokens[creds.ConnectionID] = cachedToken{value: response.AccessToken, expiresAt: time.Now().Add(ttl), credentialHash: hash}
	adapter.mu.Unlock()
	return response.AccessToken, nil
}

func (adapter *Adapter) evictToken(creds credentials) {
	adapter.mu.Lock()
	delete(adapter.tokens, creds.ConnectionID)
	adapter.mu.Unlock()
}

func (adapter *Adapter) doJSON(ctx context.Context, method, target string, query url.Values, headers map[string]string, body interface{}) (int, []byte, error) {
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, dingError(providertransport.ErrorCodeInvalidInput, "DingTalk request could not be encoded", err)
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return 0, nil, dingError(providertransport.ErrorCodeInvalidInput, "DingTalk request is invalid", err)
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := adapter.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, dingError(providertransport.ErrorCodeTimeout, "DingTalk request timed out", ctx.Err())
		}
		return 0, nil, dingError(providertransport.ErrorCodeUpstream, "DingTalk service is unavailable", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return response.StatusCode, nil, dingError(providertransport.ErrorCodeResponseInvalid, "DingTalk response could not be read", err)
	}
	return response.StatusCode, raw, nil
}

func isExpiredToken(status int, code string) bool {
	lower := strings.ToLower(strings.TrimSpace(code))
	return status == http.StatusUnauthorized || strings.Contains(lower, "token") && (strings.Contains(lower, "invalid") || strings.Contains(lower, "expire"))
}

func mapAPIError(status int, envelope apiErrorEnvelope) error {
	diagnostics := providertransport.ProviderDiagnostics{
		ErrorCode:  envelope.Code,
		RequestID:  envelope.RequestID,
		HTTPStatus: status,
	}
	providerError := func(code, message string) error {
		return providertransport.NewProviderError(code, message, nil, diagnostics)
	}
	switch status {
	case http.StatusUnauthorized:
		return providerError(providertransport.ErrorCodeAuthInvalid, "DingTalk authentication failed")
	case http.StatusForbidden:
		if isExplicitDingTalkScopeDenial(envelope.Code) {
			return providerError(providertransport.ErrorCodeInsufficientScope, "DingTalk application has not granted this operation")
		}
		return providerError(providertransport.ErrorCodeAccessDenied, "DingTalk application does not have the required permission")
	case http.StatusTooManyRequests:
		return providerError(providertransport.ErrorCodeRateLimited, "DingTalk rate limit was reached")
	}
	if status >= 500 {
		return providerError(providertransport.ErrorCodeUpstream, "DingTalk service is unavailable")
	}
	if isExplicitDingTalkScopeDenial(envelope.Code) {
		return providerError(providertransport.ErrorCodeInsufficientScope, "DingTalk application has not granted this operation")
	}
	if strings.Contains(strings.ToLower(envelope.Code), "permission") {
		return providerError(providertransport.ErrorCodeAccessDenied, "DingTalk application does not have the required permission")
	}
	return providerError(providertransport.ErrorCodeProviderRejected, "DingTalk rejected the operation")
}

func isExplicitDingTalkScopeDenial(code string) bool {
	normalized := strings.NewReplacer(".", "", "_", "", "-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(code)))
	return strings.Contains(normalized, "accesstokenpermissiondenied") ||
		(strings.Contains(normalized, "scope") &&
			(strings.Contains(normalized, "missing") || strings.Contains(normalized, "denied")))
}

func dingError(code, message string, err error) error {
	return providertransport.NewError(code, message, err)
}
