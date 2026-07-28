package gmail

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
	googleAuthorizationEndpoint = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenEndpoint         = "https://oauth2.googleapis.com/token"
	googleRevokeEndpoint        = "https://oauth2.googleapis.com/revoke"
	maxOAuthResponseBytes       = 1 << 20
)

func (adapter *Adapter) AuthorizationURL(request integrations.OAuthAuthorizationRequest) (string, error) {
	if err := validateOAuthAuthorizationRequest(request); err != nil {
		return "", err
	}
	endpoint, _ := url.Parse(googleAuthorizationEndpoint)
	query := endpoint.Query()
	query.Set("client_id", strings.TrimSpace(request.Client.ClientID))
	query.Set("redirect_uri", strings.TrimSpace(request.RedirectURI))
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(normalizeScopes(request.Scopes), " "))
	query.Set("state", strings.TrimSpace(request.State))
	query.Set("code_challenge", strings.TrimSpace(request.CodeChallenge))
	query.Set("code_challenge_method", integrations.OAuthPKCEChallengeMethodS256)
	query.Set("access_type", "offline")
	query.Set("include_granted_scopes", "true")
	// Google only guarantees a new refresh token when consent is explicitly
	// requested. The platform stores it encrypted and subsequent access is
	// refreshed without asking for the user's password.
	query.Set("prompt", "consent")
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func (adapter *Adapter) ExchangeCode(ctx context.Context, request integrations.OAuthCodeExchangeRequest) (integrations.OAuthTokenSet, error) {
	if adapter == nil || adapter.client == nil {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeUpstream, "Google OAuth client is unavailable", nil)
	}
	form := url.Values{
		"client_id":     []string{strings.TrimSpace(request.Client.ClientID)},
		"client_secret": []string{strings.TrimSpace(request.Client.ClientSecret)},
		"code":          []string{strings.TrimSpace(request.Code)},
		"code_verifier": []string{strings.TrimSpace(request.CodeVerifier)},
		"grant_type":    []string{"authorization_code"},
		"redirect_uri":  []string{strings.TrimSpace(request.RedirectURI)},
	}
	if form.Get("client_id") == "" || form.Get("code") == "" || form.Get("code_verifier") == "" || form.Get("redirect_uri") == "" {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Google OAuth code exchange is incomplete", nil)
	}
	return adapter.googleTokenRequest(ctx, form, normalizeScopes(request.Scopes))
}

func (adapter *Adapter) RefreshToken(ctx context.Context, request integrations.OAuthRefreshRequest) (integrations.OAuthTokenSet, error) {
	if adapter == nil || adapter.client == nil {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeUpstream, "Google OAuth client is unavailable", nil)
	}
	form := url.Values{
		"client_id":     []string{strings.TrimSpace(request.Client.ClientID)},
		"client_secret": []string{strings.TrimSpace(request.Client.ClientSecret)},
		"refresh_token": []string{strings.TrimSpace(request.RefreshToken)},
		"grant_type":    []string{"refresh_token"},
	}
	if form.Get("client_id") == "" || form.Get("refresh_token") == "" {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Google OAuth refresh request is incomplete", nil)
	}
	return adapter.googleTokenRequest(ctx, form, normalizeScopes(request.Scopes))
}

func (adapter *Adapter) RevokeToken(ctx context.Context, request integrations.OAuthRevokeRequest) error {
	if adapter == nil || adapter.client == nil || adapter.client.httpClient == nil {
		return integrations.NewError(integrations.ErrorCodeUpstream, "Google OAuth client is unavailable", nil)
	}
	token := strings.TrimSpace(request.Token)
	if token == "" {
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "Google OAuth revoke token is required", nil)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, googleRevokeEndpoint, strings.NewReader(url.Values{"token": []string{token}}.Encode()))
	if err != nil {
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "Google OAuth revoke request could not be created", err)
	}
	httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("User-Agent", "ZGI-External-Integrations/1.0")
	response, err := adapter.client.httpClient.Do(httpRequest)
	if err != nil {
		if ctx.Err() != nil {
			return integrations.NewError(integrations.ErrorCodeTimeout, "Google OAuth revoke request timed out", ctx.Err())
		}
		return integrations.NewError(integrations.ErrorCodeUpstream, "Google OAuth revoke request failed", err)
	}
	defer response.Body.Close()
	if _, err := io.Copy(io.Discard, io.LimitReader(response.Body, maxOAuthResponseBytes)); err != nil {
		return integrations.NewError(integrations.ErrorCodeResponseInvalid, "Google OAuth revoke response could not be read", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		mapped, _ := mapGoogleStatus(
			response.StatusCode,
			response.Header,
			nil,
			firstNonEmpty(response.Header.Get("X-GUploader-UploadID"), response.Header.Get("X-Google-Request-ID")),
		)
		return mapped
	}
	return nil
}

func (*Adapter) SupportsTokenRevocation() bool { return true }

func (adapter *Adapter) ResolveProfile(ctx context.Context, request integrations.OAuthProfileRequest) (integrations.OAuthProfile, error) {
	if adapter == nil || adapter.client == nil {
		return integrations.OAuthProfile{}, integrations.NewError(integrations.ErrorCodeUpstream, "Google OAuth client is unavailable", nil)
	}
	var identity googleIdentity
	_, err := adapter.client.getIdentity(ctx, request.AccessToken, &identity)
	if err != nil {
		return integrations.OAuthProfile{}, err
	}
	if strings.TrimSpace(identity.Subject) == "" || strings.TrimSpace(identity.Email) == "" {
		return integrations.OAuthProfile{}, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Google identity response is incomplete", nil)
	}
	return integrations.OAuthProfile{
		AccountID: bounded(identity.Subject, 255), DisplayName: bounded(firstNonEmpty(identity.Name, identity.Email), 255),
		Email: bounded(identity.Email, 320),
	}, nil
}

func (adapter *Adapter) googleTokenRequest(ctx context.Context, form url.Values, fallbackScopes []string) (integrations.OAuthTokenSet, error) {
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Google OAuth token request could not be created", err)
	}
	httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("User-Agent", "ZGI-External-Integrations/1.0")
	response, err := adapter.client.httpClient.Do(httpRequest)
	if err != nil {
		if ctx.Err() != nil {
			return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeTimeout, "Google OAuth token request timed out", ctx.Err())
		}
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeUpstream, "Google OAuth token request failed", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxOAuthResponseBytes+1))
	if err != nil {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Google OAuth token response could not be read", err)
	}
	if len(payload) > maxOAuthResponseBytes {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Google OAuth token response exceeded the platform limit", nil)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return integrations.OAuthTokenSet{}, mapGoogleOAuthError(response.StatusCode, payload)
	}
	var tokenResponse struct {
		AccessToken           string `json:"access_token"`
		RefreshToken          string `json:"refresh_token"`
		TokenType             string `json:"token_type"`
		Scope                 string `json:"scope"`
		ExpiresIn             int64  `json:"expires_in"`
		RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in"`
	}
	if err := json.Unmarshal(payload, &tokenResponse); err != nil || strings.TrimSpace(tokenResponse.AccessToken) == "" {
		return integrations.OAuthTokenSet{}, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Google OAuth token response is invalid", err)
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
	if tokenResponse.RefreshTokenExpiresIn > 0 {
		value := time.Now().UTC().Add(time.Duration(tokenResponse.RefreshTokenExpiresIn) * time.Second)
		refreshTokenExpiresAt = &value
	}
	return integrations.OAuthTokenSet{
		AccessToken: strings.TrimSpace(tokenResponse.AccessToken), RefreshToken: strings.TrimSpace(tokenResponse.RefreshToken),
		TokenType: firstNonEmpty(tokenResponse.TokenType, "Bearer"), Scopes: scopes, ExpiresAt: expiresAt,
		RefreshTokenExpiresAt: refreshTokenExpiresAt,
	}, nil
}

func mapGoogleOAuthError(status int, payload []byte) error {
	var response struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(payload, &response)
	switch strings.ToLower(strings.TrimSpace(response.Error)) {
	case "invalid_grant", "invalid_client", "unauthorized_client":
		return integrations.NewError(integrations.ErrorCodeAuthInvalid, "Google OAuth authorization is invalid or expired", nil)
	case "access_denied", "insufficient_scope":
		return integrations.NewError(integrations.ErrorCodeAccessDenied, "Google OAuth authorization was denied", nil)
	}
	if status == http.StatusTooManyRequests {
		return integrations.NewError(integrations.ErrorCodeRateLimited, "Google OAuth rate limit was reached", nil)
	}
	if status >= http.StatusInternalServerError {
		return integrations.NewError(integrations.ErrorCodeUpstream, "Google OAuth is temporarily unavailable", nil)
	}
	return integrations.NewError(integrations.ErrorCodeAuthInvalid, "Google OAuth request was rejected", nil)
}

func validateOAuthAuthorizationRequest(request integrations.OAuthAuthorizationRequest) error {
	redirect, err := url.Parse(strings.TrimSpace(request.RedirectURI))
	if strings.TrimSpace(request.Client.ClientID) == "" || err != nil || redirect.Scheme == "" || redirect.Host == "" ||
		strings.TrimSpace(request.State) == "" || strings.TrimSpace(request.CodeChallenge) == "" ||
		request.CodeChallengeMethod != integrations.OAuthPKCEChallengeMethodS256 || len(normalizeScopes(request.Scopes)) == 0 {
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "Google OAuth authorization request is incomplete", err)
	}
	return nil
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
