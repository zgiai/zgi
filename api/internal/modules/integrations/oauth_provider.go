package integrations

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

const OAuthPKCEChallengeMethodS256 = "S256"

// OAuthClient contains request-scoped OAuth application credentials. It must
// never be serialized, logged, cached, or returned to a browser.
type OAuthClient struct {
	ClientID     string
	ClientSecret string
	Config       map[string]any
	Source       string
}

func (client *OAuthClient) Destroy() {
	if client == nil {
		return
	}
	client.ClientID = ""
	client.ClientSecret = ""
	for key := range client.Config {
		delete(client.Config, key)
	}
	client.Config = nil
}

type OAuthAuthorizationRequest struct {
	Client              OAuthClient
	RedirectURI         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Scopes              []string
	Config              map[string]any
}

type OAuthCodeExchangeRequest struct {
	Client       OAuthClient
	Code         string
	RedirectURI  string
	CodeVerifier string
	Scopes       []string
	Config       map[string]any
}

type OAuthRefreshRequest struct {
	Client       OAuthClient
	RefreshToken string
	Scopes       []string
	Config       map[string]any
}

type OAuthRevokeRequest struct {
	Client        OAuthClient
	Token         string
	TokenTypeHint string
	Config        map[string]any
}

type OAuthProfileRequest struct {
	AccessToken string
	TokenType   string
	Config      map[string]any
}

// OAuthTokenSet is secret request-scoped material. Destroy it after the
// encrypted connection envelope has been persisted.
type OAuthTokenSet struct {
	AccessToken           string
	RefreshToken          string
	TokenType             string
	Scopes                []string
	ExpiresAt             *time.Time
	RefreshTokenExpiresAt *time.Time
}

func (tokens *OAuthTokenSet) Destroy() {
	if tokens == nil {
		return
	}
	tokens.AccessToken = ""
	tokens.RefreshToken = ""
	tokens.TokenType = ""
	for index := range tokens.Scopes {
		tokens.Scopes[index] = ""
	}
	tokens.Scopes = nil
	tokens.ExpiresAt = nil
	tokens.RefreshTokenExpiresAt = nil
}

func (tokens OAuthTokenSet) credentialMap() map[string]string {
	credentials := map[string]string{"access_token": strings.TrimSpace(tokens.AccessToken)}
	if refreshToken := strings.TrimSpace(tokens.RefreshToken); refreshToken != "" {
		credentials["refresh_token"] = refreshToken
	}
	if tokenType := strings.TrimSpace(tokens.TokenType); tokenType != "" {
		credentials["token_type"] = tokenType
	}
	return credentials
}

type OAuthProfile struct {
	AccountID   string
	DisplayName string
	Email       string
}

// OAuth2Provider owns fixed provider endpoints and protocol-specific parsing.
// Browser input cannot override authorization, token, profile, or revocation
// endpoints.
type OAuth2Provider interface {
	AuthorizationURL(OAuthAuthorizationRequest) (string, error)
	ExchangeCode(context.Context, OAuthCodeExchangeRequest) (OAuthTokenSet, error)
	RefreshToken(context.Context, OAuthRefreshRequest) (OAuthTokenSet, error)
	RevokeToken(context.Context, OAuthRevokeRequest) error
	ResolveProfile(context.Context, OAuthProfileRequest) (OAuthProfile, error)
}

// OAuthRevocationCapability is explicit because not every provider publishes a
// standards-compatible revocation endpoint. The flow service only attempts
// compensating revocation when a provider positively declares support.
type OAuthRevocationCapability interface {
	SupportsTokenRevocation() bool
}

type OAuthClientResolveRequest struct {
	OrganizationID uuid.UUID
	IntegrationID  string
	DriverID       string
	AuthMethodID   string
}

type OAuthClientResolver interface {
	ResolveOAuthClient(context.Context, OAuthClientResolveRequest) (OAuthClient, error)
	OAuthClientConfigured(context.Context, OAuthClientResolveRequest) bool
}
