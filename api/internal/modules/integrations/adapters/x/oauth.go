package x

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

const (
	xAuthorizationEndpoint = "https://x.com/i/oauth2/authorize"
	xTokenEndpoint         = "https://api.x.com/2/oauth2/token"
	xRevokeEndpoint        = "https://api.x.com/2/oauth2/revoke"
	maxOAuthResponseBytes  = 1 << 20
)

func (adapter *Adapter) AuthorizationURL(request integrations.OAuthAuthorizationRequest) (string, error) {
	if err := validateOAuthAuthorizationRequest(request); err != nil {
		return "", err
	}
	endpoint, _ := url.Parse(xAuthorizationEndpoint)
	query := endpoint.Query()
	query.Set("client_id", strings.TrimSpace(request.Client.ClientID))
	query.Set("redirect_uri", strings.TrimSpace(request.RedirectURI))
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(withOfflineAccess(request.Scopes), " "))
	query.Set("state", strings.TrimSpace(request.State))
	query.Set("code_challenge", strings.TrimSpace(request.CodeChallenge))
	query.Set("code_challenge_method", integrations.OAuthPKCEChallengeMethodS256)
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func (adapter *Adapter) ExchangeCode(ctx context.Context, request integrations.OAuthCodeExchangeRequest) (integrations.OAuthTokenSet, error) {
	form := url.Values{
		"code":          []string{strings.TrimSpace(request.Code)},
		"grant_type":    []string{"authorization_code"},
		"redirect_uri":  []string{strings.TrimSpace(request.RedirectURI)},
		"code_verifier": []string{strings.TrimSpace(request.CodeVerifier)},
	}
	if strings.TrimSpace(request.Client.ClientID) == "" || form.Get("code") == "" ||
		form.Get("redirect_uri") == "" || form.Get("code_verifier") == "" {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "X OAuth code exchange is incomplete", nil)
	}
	return adapter.xTokenRequest(ctx, request.Client, form, withOfflineAccess(request.Scopes), "")
}

func (adapter *Adapter) RefreshToken(ctx context.Context, request integrations.OAuthRefreshRequest) (integrations.OAuthTokenSet, error) {
	refreshToken := strings.TrimSpace(request.RefreshToken)
	form := url.Values{
		"refresh_token": []string{refreshToken},
		"grant_type":    []string{"refresh_token"},
	}
	if strings.TrimSpace(request.Client.ClientID) == "" || refreshToken == "" {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "X OAuth refresh request is incomplete", nil)
	}
	return adapter.xTokenRequest(ctx, request.Client, form, normalizeScopes(request.Scopes), refreshToken)
}

func (adapter *Adapter) RevokeToken(ctx context.Context, request integrations.OAuthRevokeRequest) error {
	if adapter == nil || adapter.client == nil || adapter.client.httpClient == nil {
		return integrations.NewError(integrations.ErrorCodeUpstream, "X OAuth client is unavailable", nil)
	}
	token := strings.TrimSpace(request.Token)
	if token == "" || strings.TrimSpace(request.Client.ClientID) == "" {
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "X OAuth revoke request is incomplete", nil)
	}
	tokenTypeHint := strings.TrimSpace(request.TokenTypeHint)
	if tokenTypeHint != "refresh_token" {
		tokenTypeHint = "access_token"
	}
	form := url.Values{"token": []string{token}, "token_type_hint": []string{tokenTypeHint}}
	applyXOAuthClientAuthentication(form, request.Client)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, xRevokeEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "X OAuth revoke request could not be created", err)
	}
	applyXOAuthHeaders(httpRequest, request.Client)
	response, err := adapter.client.httpClient.Do(httpRequest)
	if err != nil {
		if ctx.Err() != nil {
			return integrations.NewError(integrations.ErrorCodeTimeout, "X OAuth revoke request timed out", ctx.Err())
		}
		return integrations.NewError(integrations.ErrorCodeUpstream, "X OAuth revoke request failed", err)
	}
	defer response.Body.Close()
	if _, err := io.Copy(io.Discard, io.LimitReader(response.Body, maxOAuthResponseBytes)); err != nil {
		return integrations.NewError(integrations.ErrorCodeResponseInvalid, "X OAuth revoke response could not be read", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		mapped, _ := mapXProblem(
			response.StatusCode,
			response.Header,
			firstNonEmpty(response.Header.Get("X-Transaction-Id"), response.Header.Get("X-Request-Id")),
			xProblem{},
		)
		return mapped
	}
	return nil
}

func (*Adapter) SupportsTokenRevocation() bool { return true }

func (adapter *Adapter) ResolveProfile(ctx context.Context, request integrations.OAuthProfileRequest) (integrations.OAuthProfile, error) {
	user, _, err := adapter.fetchCurrentUser(ctx, request.AccessToken)
	if err != nil {
		return integrations.OAuthProfile{}, err
	}
	return integrations.OAuthProfile{
		AccountID: bounded(user.ID, 255), DisplayName: bounded(firstNonEmpty(user.Name, "@"+user.Username), 255),
	}, nil
}

func (adapter *Adapter) xTokenRequest(
	ctx context.Context,
	oauthClient integrations.OAuthClient,
	form url.Values,
	fallbackScopes []string,
	fallbackRefreshToken string,
) (integrations.OAuthTokenSet, error) {
	if adapter == nil || adapter.client == nil || adapter.client.httpClient == nil {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeUpstream, "X OAuth client is unavailable", nil)
	}
	applyXOAuthClientAuthentication(form, oauthClient)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, xTokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "X OAuth token request could not be created", err)
	}
	applyXOAuthHeaders(httpRequest, oauthClient)
	response, err := adapter.client.httpClient.Do(httpRequest)
	if err != nil {
		if ctx.Err() != nil {
			return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeTimeout, "X OAuth token request timed out", ctx.Err())
		}
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeUpstream, "X OAuth token request failed", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxOAuthResponseBytes+1))
	if err != nil {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeResponseInvalid, "X OAuth token response could not be read", err)
	}
	if len(payload) > maxOAuthResponseBytes {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeResponseInvalid, "X OAuth token response exceeded the platform limit", nil)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return integrations.OAuthTokenSet{}, mapXOAuthError(response.StatusCode, payload)
	}
	var tokenResponse struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(payload, &tokenResponse); err != nil || strings.TrimSpace(tokenResponse.AccessToken) == "" {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeResponseInvalid, "X OAuth token response is invalid", err)
	}
	refreshToken := firstNonEmpty(tokenResponse.RefreshToken, fallbackRefreshToken)
	scopes := normalizeScopes(strings.Fields(tokenResponse.Scope))
	if len(scopes) == 0 {
		scopes = append([]string(nil), fallbackScopes...)
	}
	var expiresAt *time.Time
	if tokenResponse.ExpiresIn > 0 {
		value := time.Now().UTC().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second)
		expiresAt = &value
	}
	return integrations.OAuthTokenSet{
		AccessToken: strings.TrimSpace(tokenResponse.AccessToken), RefreshToken: refreshToken,
		TokenType: firstNonEmpty(tokenResponse.TokenType, "Bearer"), Scopes: scopes, ExpiresAt: expiresAt,
	}, nil
}

func applyXOAuthHeaders(request *http.Request, client integrations.OAuthClient) {
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "ZGI-External-Integrations/1.0")
	clientID := strings.TrimSpace(client.ClientID)
	clientSecret := strings.TrimSpace(client.ClientSecret)
	if clientSecret != "" {
		request.SetBasicAuth(clientID, clientSecret)
	}
}

func applyXOAuthClientAuthentication(form url.Values, client integrations.OAuthClient) {
	if strings.TrimSpace(client.ClientSecret) == "" {
		form.Set("client_id", strings.TrimSpace(client.ClientID))
	}
}

func mapXOAuthError(status int, payload []byte) error {
	var response struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(payload, &response)
	switch strings.ToLower(strings.TrimSpace(response.Error)) {
	case "invalid_grant", "invalid_client", "unauthorized_client":
		return integrations.NewError(integrations.ErrorCodeAuthInvalid, "X OAuth authorization is invalid or expired", nil)
	case "access_denied", "insufficient_scope":
		return integrations.NewError(integrations.ErrorCodeAccessDenied, "X OAuth authorization was denied", nil)
	}
	if status == http.StatusTooManyRequests {
		return integrations.NewError(integrations.ErrorCodeRateLimited, "X OAuth rate limit was reached", nil)
	}
	if status >= http.StatusInternalServerError {
		return integrations.NewError(integrations.ErrorCodeUpstream, "X OAuth is temporarily unavailable", nil)
	}
	return integrations.NewError(integrations.ErrorCodeAuthInvalid, "X OAuth request was rejected", nil)
}

func validateOAuthAuthorizationRequest(request integrations.OAuthAuthorizationRequest) error {
	redirect, err := url.Parse(strings.TrimSpace(request.RedirectURI))
	if strings.TrimSpace(request.Client.ClientID) == "" || err != nil || redirect.Scheme == "" || redirect.Host == "" ||
		strings.TrimSpace(request.State) == "" || strings.TrimSpace(request.CodeChallenge) == "" ||
		request.CodeChallengeMethod != integrations.OAuthPKCEChallengeMethodS256 || len(normalizeScopes(request.Scopes)) == 0 {
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "X OAuth authorization request is incomplete", err)
	}
	return nil
}

func withOfflineAccess(scopes []string) []string {
	result := normalizeScopes(scopes)
	for _, scope := range result {
		if scope == "offline.access" {
			return result
		}
	}
	return append(result, "offline.access")
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
