package feishu

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

const (
	feishuAuthorizationEndpoint = "https://accounts.feishu.cn/open-apis/authen/v1/authorize"
	larkAuthorizationEndpoint   = "https://accounts.larksuite.com/open-apis/authen/v1/authorize"
	feishuOAuthTokenEndpoint    = "https://open.feishu.cn/open-apis/authen/v2/oauth/token"
	larkOAuthTokenEndpoint      = "https://open.larksuite.com/open-apis/authen/v2/oauth/token"
	maxOAuthResponseBytes       = 1 << 20
)

func (adapter *Adapter) AuthorizationURL(request integrations.OAuthAuthorizationRequest) (string, error) {
	region, err := oauthRegion(request.Client.Config, request.Config)
	if err != nil {
		return "", err
	}
	if err := validateOAuthAuthorizationRequest(request); err != nil {
		return "", err
	}
	rawEndpoint := feishuAuthorizationEndpoint
	if region == RegionGlobal {
		rawEndpoint = larkAuthorizationEndpoint
	}
	endpoint, _ := url.Parse(rawEndpoint)
	query := endpoint.Query()
	query.Set("client_id", strings.TrimSpace(request.Client.ClientID))
	query.Set("redirect_uri", strings.TrimSpace(request.RedirectURI))
	query.Set("response_type", "code")
	query.Set("state", strings.TrimSpace(request.State))
	query.Set("scope", strings.Join(withOfflineAccess(request.Scopes), " "))
	query.Set("code_challenge", strings.TrimSpace(request.CodeChallenge))
	query.Set("code_challenge_method", integrations.OAuthPKCEChallengeMethodS256)
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func (adapter *Adapter) ExchangeCode(ctx context.Context, request integrations.OAuthCodeExchangeRequest) (integrations.OAuthTokenSet, error) {
	region, err := oauthRegion(request.Client.Config, request.Config)
	if err != nil {
		return integrations.OAuthTokenSet{}, err
	}
	body := map[string]string{
		"grant_type": "authorization_code", "client_id": strings.TrimSpace(request.Client.ClientID),
		"client_secret": strings.TrimSpace(request.Client.ClientSecret), "code": strings.TrimSpace(request.Code),
		"redirect_uri": strings.TrimSpace(request.RedirectURI), "code_verifier": strings.TrimSpace(request.CodeVerifier),
	}
	if body["client_id"] == "" || body["client_secret"] == "" || body["code"] == "" ||
		body["redirect_uri"] == "" || body["code_verifier"] == "" {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu OAuth code exchange is incomplete", nil)
	}
	return adapter.feishuOAuthTokenRequest(ctx, region, body, normalizeScopes(request.Scopes), true)
}

func (adapter *Adapter) RefreshToken(ctx context.Context, request integrations.OAuthRefreshRequest) (integrations.OAuthTokenSet, error) {
	region, err := oauthRegion(request.Client.Config, request.Config)
	if err != nil {
		return integrations.OAuthTokenSet{}, err
	}
	body := map[string]string{
		"grant_type": "refresh_token", "client_id": strings.TrimSpace(request.Client.ClientID),
		"client_secret": strings.TrimSpace(request.Client.ClientSecret), "refresh_token": strings.TrimSpace(request.RefreshToken),
	}
	if body["client_id"] == "" || body["client_secret"] == "" || body["refresh_token"] == "" {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu OAuth refresh request is incomplete", nil)
	}
	// Feishu rotates refresh tokens. A refresh response without a replacement
	// must fail closed so concurrent or future refreshes never reuse an already
	// consumed token.
	return adapter.feishuOAuthTokenRequest(ctx, region, body, normalizeScopes(request.Scopes), true)
}

func (adapter *Adapter) RevokeToken(_ context.Context, _ integrations.OAuthRevokeRequest) error {
	// Feishu/Lark does not publish a general OAuth token revocation endpoint for
	// this web flow. Local encrypted credentials are deleted by the connection
	// service; users can also revoke the application from Feishu account
	// settings. Never invent or accept a browser-supplied revocation URL.
	return nil
}

func (*Adapter) SupportsTokenRevocation() bool { return false }

func (adapter *Adapter) ResolveProfile(ctx context.Context, request integrations.OAuthProfileRequest) (integrations.OAuthProfile, error) {
	region, err := oauthRegion(nil, request.Config)
	if err != nil {
		return integrations.OAuthProfile{}, err
	}
	var user feishuUserData
	_, err = adapter.client.getUserInfo(ctx, region, request.AccessToken, &user)
	if err != nil {
		return integrations.OAuthProfile{}, err
	}
	accountID := firstNonEmpty(user.OpenID, user.UserID, user.UnionID)
	if accountID == "" {
		return integrations.OAuthProfile{}, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu identity response is incomplete", nil)
	}
	return integrations.OAuthProfile{
		AccountID: bounded(accountID, 255), DisplayName: bounded(firstNonEmpty(user.Name, user.EnName, user.Email, accountID), 255),
		Email: bounded(user.Email, 320),
	}, nil
}

func (adapter *Adapter) feishuOAuthTokenRequest(
	ctx context.Context,
	region string,
	body map[string]string,
	fallbackScopes []string,
	requireRefreshToken bool,
) (integrations.OAuthTokenSet, error) {
	if adapter == nil || adapter.client == nil || adapter.client.httpClient == nil {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeUpstream, "Feishu OAuth client is unavailable", nil)
	}
	rawEndpoint := feishuOAuthTokenEndpoint
	if region == RegionGlobal {
		rawEndpoint = larkOAuthTokenEndpoint
	}
	endpoint, err := url.Parse(rawEndpoint)
	if err != nil {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeUpstream, "Feishu OAuth token endpoint is invalid", err)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu OAuth token request could not be encoded", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(string(encoded)))
	if err != nil {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu OAuth token request could not be created", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json; charset=utf-8")
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("User-Agent", "ZGI-External-Integrations/1.0")
	response, err := adapter.client.httpClient.Do(httpRequest)
	if err != nil {
		if ctx.Err() != nil {
			return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeTimeout, "Feishu OAuth token request timed out", ctx.Err())
		}
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeUpstream, "Feishu OAuth token request failed", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxOAuthResponseBytes+1))
	if err != nil {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu OAuth token response could not be read", err)
	}
	if len(payload) > maxOAuthResponseBytes {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu OAuth token response exceeded the platform limit", nil)
	}
	var tokenResponse struct {
		Code             json.RawMessage `json:"code"`
		Error            string          `json:"error"`
		AccessToken      string          `json:"access_token"`
		RefreshToken     string          `json:"refresh_token"`
		TokenType        string          `json:"token_type"`
		Scope            string          `json:"scope"`
		ExpiresIn        int64           `json:"expires_in"`
		RefreshExpiresIn int64           `json:"refresh_token_expires_in"`
	}
	if err := json.Unmarshal(payload, &tokenResponse); err != nil {
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return integrations.OAuthTokenSet{}, mapFeishuStatus(response.StatusCode)
		}
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu OAuth token response is invalid", err)
	}
	if code, exists, codeErr := parseOAuthCode(tokenResponse.Code); codeErr != nil {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu OAuth token response is invalid", codeErr)
	} else if exists && code != 0 {
		return integrations.OAuthTokenSet{}, mapFeishuBusinessCode(code)
	}
	if tokenResponse.Error != "" {
		return integrations.OAuthTokenSet{}, mapFeishuOAuthError(tokenResponse.Error)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return integrations.OAuthTokenSet{}, mapFeishuStatus(response.StatusCode)
	}
	if strings.TrimSpace(tokenResponse.AccessToken) == "" || (requireRefreshToken && strings.TrimSpace(tokenResponse.RefreshToken) == "") {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu OAuth token response is incomplete", nil)
	}
	scopes := normalizeScopes(strings.Fields(tokenResponse.Scope))
	if len(scopes) == 0 {
		scopes = append([]string(nil), fallbackScopes...)
	}
	var expiresAt *time.Time
	if tokenResponse.ExpiresIn > 0 {
		value := time.Now().UTC().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second)
		expiresAt = &value
	}
	var refreshTokenExpiresAt *time.Time
	if tokenResponse.RefreshExpiresIn > 0 {
		value := time.Now().UTC().Add(time.Duration(tokenResponse.RefreshExpiresIn) * time.Second)
		refreshTokenExpiresAt = &value
	}
	return integrations.OAuthTokenSet{
		AccessToken: strings.TrimSpace(tokenResponse.AccessToken), RefreshToken: strings.TrimSpace(tokenResponse.RefreshToken),
		TokenType: firstNonEmpty(tokenResponse.TokenType, "Bearer"), Scopes: scopes, ExpiresAt: expiresAt,
		RefreshTokenExpiresAt: refreshTokenExpiresAt,
	}, nil
}

func validateOAuthAuthorizationRequest(request integrations.OAuthAuthorizationRequest) error {
	redirect, err := url.Parse(strings.TrimSpace(request.RedirectURI))
	if strings.TrimSpace(request.Client.ClientID) == "" || err != nil || redirect.Scheme == "" || redirect.Host == "" ||
		strings.TrimSpace(request.State) == "" || strings.TrimSpace(request.CodeChallenge) == "" ||
		request.CodeChallengeMethod != integrations.OAuthPKCEChallengeMethodS256 || len(withOfflineAccess(request.Scopes)) == 0 {
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu OAuth authorization request is incomplete", err)
	}
	return nil
}

func parseOAuthCode(raw json.RawMessage) (int, bool, error) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return 0, false, nil
	}
	value = strings.Trim(value, `"`)
	code, err := strconv.Atoi(value)
	if err != nil {
		return 0, true, err
	}
	return code, true, nil
}

func mapFeishuOAuthError(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "invalid_client", "invalid_grant", "unauthorized_client":
		return integrations.NewError(integrations.ErrorCodeAuthInvalid, "Feishu OAuth authorization is invalid or expired", nil)
	case "access_denied", "invalid_scope", "insufficient_scope":
		return integrations.NewError(integrations.ErrorCodeAccessDenied, "Feishu OAuth authorization was denied", nil)
	case "invalid_request", "unsupported_grant_type":
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu OAuth request is invalid", nil)
	case "server_error", "temporarily_unavailable":
		return integrations.NewError(integrations.ErrorCodeUpstream, "Feishu OAuth is temporarily unavailable", nil)
	default:
		return integrations.NewError(integrations.ErrorCodeAuthInvalid, "Feishu OAuth request was rejected", nil)
	}
}

func oauthRegion(configs ...map[string]any) (string, error) {
	_ = configs
	return RegionCN, nil
}

func withOfflineAccess(scopes []string) []string {
	result := normalizeScopes(scopes)
	for _, scope := range result {
		if scope == "offline_access" {
			return result
		}
	}
	return append(result, "offline_access")
}

func normalizeScopes(scopes []string) []string {
	result := make([]string, 0, len(scopes))
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" || len(scope) > 256 {
			continue
		}
		if _, exists := seen[scope]; exists {
			continue
		}
		seen[scope] = struct{}{}
		result = append(result, scope)
	}
	return result
}

var _ integrations.OAuth2Provider = (*Adapter)(nil)
var _ integrations.OAuthRevocationCapability = (*Adapter)(nil)
