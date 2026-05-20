package service

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zgiai/ginext/config"
	shareddto "github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/internal/observability"
)

const casdoorDiscoveryPath = "/.well-known/openid-configuration"

const (
	casdoorHostEnvKey          = "CASDOOR_HOST"
	casdoorClientIDEnvKey      = "CASDOOR_CLIENT_ID"
	casdoorClientSecretEnvKey  = "CASDOOR_CLIENT_SECRET"
	casdoorRedirectURIEnvKey   = "CASDOOR_REDIRECT_URI"
	casdoorScopesEnvKey        = "CASDOOR_SCOPES"
	defaultCasdoorScopes       = "openid profile email"
	casdoorScopeListSeparator  = ","
	oidcScopeSeparator         = " "
	oidcGrantTypeKey           = "grant_type"
	oidcGrantTypeAuthCode      = "authorization_code"
	oidcCodeParamKey           = "code"
	oidcClientIDParamKey       = "client_id"
	oidcClientSecretParamKey   = "client_secret"
	oidcRedirectURIParamKey    = "redirect_uri"
	httpHeaderAuthorization    = "Authorization"
	httpHeaderContentType      = "Content-Type"
	httpBearerTokenPrefix      = "Bearer "
	httpFormContentType        = "application/x-www-form-urlencoded"
	oidcTokenHeaderAlgorithm   = "alg"
	oidcTokenHeaderKeyID       = "kid"
	oidcJWKKeyTypeRSA          = "RSA"
	casdoorHTTPTimeout         = 10 * time.Second
	httpSuccessStatusMin       = http.StatusOK
	httpRedirectStatusBoundary = http.StatusMultipleChoices
)

type CasdoorOIDCClient struct {
	host       string
	config     casdoorOIDCConfig
	httpClient *http.Client
}

type oidcTokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
}

type oidcJWKS struct {
	Keys []oidcJWK `json:"keys"`
}

type oidcJWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func NewCasdoorOIDCClientFromEnv() (*CasdoorOIDCClient, error) {
	casdoor := config.Current().Auth.Casdoor
	host := strings.TrimRight(strings.TrimSpace(casdoor.Host), "/")
	clientID := strings.TrimSpace(casdoor.ClientID)
	clientSecret := strings.TrimSpace(casdoor.ClientSecret)
	redirectURI := strings.TrimSpace(casdoor.RedirectURI)
	scopes := casdoor.Scopes
	if len(scopes) == 0 {
		scopes = strings.Fields(strings.ReplaceAll(defaultCasdoorScopes, casdoorScopeListSeparator, oidcScopeSeparator))
	}

	switch {
	case host == "":
		return nil, fmt.Errorf("%s is required", casdoorHostEnvKey)
	case clientID == "":
		return nil, fmt.Errorf("%s is required", casdoorClientIDEnvKey)
	case clientSecret == "":
		return nil, fmt.Errorf("%s is required", casdoorClientSecretEnvKey)
	case redirectURI == "":
		return nil, fmt.Errorf("%s is required", casdoorRedirectURIEnvKey)
	}

	return &CasdoorOIDCClient{
		host: host,
		config: casdoorOIDCConfig{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURI:  redirectURI,
			Scopes:       scopes,
		},
		httpClient: observability.HTTPClient(&http.Client{Timeout: casdoorHTTPTimeout}),
	}, nil
}

func (c *CasdoorOIDCClient) AuthorizationURL(ctx context.Context, state string) (string, error) {
	discovery, err := c.fetchDiscovery(ctx)
	if err != nil {
		return "", err
	}
	return buildCasdoorAuthorizationURL(discovery, c.config, state)
}

func (c *CasdoorOIDCClient) ExchangeCode(ctx context.Context, code string) (*shareddto.SSOExchangeResult, error) {
	discovery, err := c.fetchDiscovery(ctx)
	if err != nil {
		return nil, err
	}

	tokenResponse, err := c.exchangeAuthorizationCode(ctx, discovery, code)
	if err != nil {
		return nil, err
	}

	claims := map[string]any{}
	if tokenResponse.IDToken != "" {
		claims, err = c.validateIDToken(ctx, discovery, tokenResponse.IDToken)
		if err != nil {
			return nil, err
		}
	}

	if tokenResponse.AccessToken != "" && discovery.UserinfoEndpoint != "" {
		userinfoClaims, err := c.fetchUserinfo(ctx, discovery.UserinfoEndpoint, tokenResponse.AccessToken)
		if err != nil {
			return nil, err
		}
		mapsCopy(claims, userinfoClaims)
	}

	identity, err := identityFromClaims(claims)
	if err != nil {
		return nil, err
	}

	result := &shareddto.SSOExchangeResult{
		Identity: &shareddto.SSOIdentity{
			Subject:     identity.Subject,
			Email:       identity.Email,
			Name:        identity.Name,
			PhoneNumber: identity.PhoneNumber,
			CountryCode: identity.CountryCode,
		},
	}
	if tokenResponse.IDToken != "" {
		result.Token = &shareddto.SSOProviderToken{
			Provider: shareddto.SSOProviderCasdoor,
			IDToken:  tokenResponse.IDToken,
		}
	}

	return result, nil
}

func (c *CasdoorOIDCClient) fetchDiscovery(ctx context.Context) (oidcDiscoveryDocument, error) {
	var discovery oidcDiscoveryDocument
	if err := c.getJSON(ctx, c.host+casdoorDiscoveryPath, &discovery, nil); err != nil {
		return oidcDiscoveryDocument{}, fmt.Errorf("fetch discovery document: %w", err)
	}
	discovery = normalizeCasdoorDiscoveryEndpoints(c.host, discovery)
	return discovery, nil
}

func (c *CasdoorOIDCClient) exchangeAuthorizationCode(ctx context.Context, discovery oidcDiscoveryDocument, code string) (*oidcTokenResponse, error) {
	form := url.Values{}
	form.Set(oidcGrantTypeKey, oidcGrantTypeAuthCode)
	form.Set(oidcCodeParamKey, code)
	form.Set(oidcClientIDParamKey, c.config.ClientID)
	form.Set(oidcClientSecretParamKey, c.config.ClientSecret)
	form.Set(oidcRedirectURIParamKey, c.config.RedirectURI)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, discovery.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	request.Header.Set(httpHeaderContentType, httpFormContentType)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("exchange authorization code: %w", err)
	}
	defer response.Body.Close()

	if !isHTTPSuccess(response.StatusCode) {
		return nil, fmt.Errorf("exchange authorization code: unexpected status %d", response.StatusCode)
	}

	var tokenResponse oidcTokenResponse
	if err := json.NewDecoder(response.Body).Decode(&tokenResponse); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}

	return &tokenResponse, nil
}

func (c *CasdoorOIDCClient) fetchUserinfo(ctx context.Context, endpoint, accessToken string) (map[string]any, error) {
	var claims map[string]any
	headers := map[string]string{
		httpHeaderAuthorization: httpBearerTokenPrefix + accessToken,
	}
	if err := c.getJSON(ctx, endpoint, &claims, headers); err != nil {
		return nil, fmt.Errorf("fetch userinfo: %w", err)
	}
	return claims, nil
}

func (c *CasdoorOIDCClient) validateIDToken(ctx context.Context, discovery oidcDiscoveryDocument, idToken string) (map[string]any, error) {
	keySet, err := c.fetchJWKS(ctx, discovery.JWKSURI)
	if err != nil {
		return nil, err
	}

	token, err := jwt.Parse(idToken, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", token.Header[oidcTokenHeaderAlgorithm])
		}

		kid, _ := token.Header[oidcTokenHeaderKeyID].(string)
		return keySet.lookupKey(kid)
	})
	if err != nil {
		return nil, fmt.Errorf("validate id token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid id token claims")
	}

	if err := validateIssuer(claims, discovery.Issuer); err != nil {
		return nil, err
	}
	if err := validateAudience(claims, c.config.ClientID); err != nil {
		return nil, err
	}

	return map[string]any(claims), nil
}

func (c *CasdoorOIDCClient) fetchJWKS(ctx context.Context, endpoint string) (*oidcJWKS, error) {
	var keySet oidcJWKS
	if err := c.getJSON(ctx, endpoint, &keySet, nil); err != nil {
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}
	return &keySet, nil
}

func (c *CasdoorOIDCClient) getJSON(ctx context.Context, endpoint string, target any, headers map[string]string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer response.Body.Close()

	if !isHTTPSuccess(response.StatusCode) {
		return fmt.Errorf("unexpected status %d", response.StatusCode)
	}

	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func normalizeCasdoorDiscoveryEndpoints(baseHost string, discovery oidcDiscoveryDocument) oidcDiscoveryDocument {
	discovery.AuthorizationEndpoint = normalizeCasdoorEndpoint(baseHost, discovery.AuthorizationEndpoint)
	discovery.TokenEndpoint = normalizeCasdoorEndpoint(baseHost, discovery.TokenEndpoint)
	discovery.UserinfoEndpoint = normalizeCasdoorEndpoint(baseHost, discovery.UserinfoEndpoint)
	discovery.JWKSURI = normalizeCasdoorEndpoint(baseHost, discovery.JWKSURI)
	return discovery
}

func normalizeCasdoorEndpoint(baseHost, endpoint string) string {
	baseURL, err := url.Parse(strings.TrimSpace(baseHost))
	if err != nil || baseURL.Host == "" {
		return endpoint
	}
	if baseURL.Port() == "" {
		return endpoint
	}

	endpointURL, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || endpointURL.Host == "" {
		return endpoint
	}

	baseHostname := baseURL.Hostname()
	endpointHostname := endpointURL.Hostname()
	if baseHostname == "" || endpointHostname == "" || !strings.EqualFold(baseHostname, endpointHostname) {
		return endpoint
	}
	if endpointURL.Port() != "" {
		return endpoint
	}

	endpointURL.Scheme = baseURL.Scheme
	endpointURL.Host = baseURL.Host
	return endpointURL.String()
}

func (k *oidcJWKS) lookupKey(kid string) (*rsa.PublicKey, error) {
	for _, key := range k.Keys {
		if kid != "" && key.Kid != kid {
			continue
		}
		publicKey, err := key.publicKey()
		if err != nil {
			return nil, err
		}
		return publicKey, nil
	}
	return nil, fmt.Errorf("jwk key not found for kid %q", kid)
}

func (k oidcJWK) publicKey() (*rsa.PublicKey, error) {
	if k.Kty != oidcJWKKeyTypeRSA {
		return nil, fmt.Errorf("unsupported jwk key type %q", k.Kty)
	}

	modulus, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decode modulus: %w", err)
	}
	exponentBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decode exponent: %w", err)
	}

	exponent := 0
	for _, b := range exponentBytes {
		exponent = exponent<<8 + int(b)
	}
	if exponent == 0 {
		return nil, errors.New("invalid rsa exponent")
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(modulus),
		E: exponent,
	}, nil
}

func isHTTPSuccess(statusCode int) bool {
	return statusCode >= httpSuccessStatusMin && statusCode < httpRedirectStatusBoundary
}

func validateIssuer(claims map[string]any, expectedIssuer string) error {
	if expectedIssuer == "" {
		return nil
	}
	issuer, _ := stringClaim(claims, oidcClaimIssuer)
	if issuer != expectedIssuer {
		return fmt.Errorf("unexpected issuer %q", issuer)
	}
	return nil
}

func validateAudience(claims map[string]any, clientID string) error {
	rawAudience, ok := claims[oidcClaimAudience]
	if !ok {
		return errors.New("audience claim is required")
	}

	switch audience := rawAudience.(type) {
	case string:
		if audience == clientID {
			return nil
		}
	case []any:
		for _, item := range audience {
			if value, ok := item.(string); ok && value == clientID {
				return nil
			}
		}
	}

	return fmt.Errorf("audience does not include client id %q", clientID)
}

func mapsCopy(dst, src map[string]any) {
	for key, value := range src {
		if _, exists := dst[key]; exists && dst[key] != nil {
			continue
		}
		dst[key] = value
	}
}
