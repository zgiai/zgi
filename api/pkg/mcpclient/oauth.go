package mcpclient

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

func newOAuthHandler(ctx context.Context, config *OAuthConfig, httpClient *http.Client) (auth.OAuthHandler, error) {
	if config == nil {
		return nil, &MCPError{Kind: ErrConfig, Operation: "OAuth", Cause: errors.New("OAuth config is required")}
	}
	if config.Handler != nil {
		return config.Handler, nil
	}
	if config.ScopeKey == "" || config.Store == nil || config.OnAuthorize == nil || config.RedirectURI == "" || (config.ClientID == "" && config.ClientIDMetadataURL == "" && config.DynamicRegistration == nil) {
		return nil, &MCPError{Kind: ErrConfig, Operation: "OAuth", Cause: errors.New("OAuth store, scope, callback, redirect and client registration are required")}
	}
	settings := &auth.AuthorizationCodeHandlerConfig{
		RedirectURL: config.RedirectURI, Client: httpClient, RequestRefreshToken: true,
		AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			result, err := config.OnAuthorize(ctx, args.URL)
			if err != nil {
				return nil, &MCPError{Kind: ErrAuthFailed, Operation: "OAuth authorization", Cause: err}
			}
			return &auth.AuthorizationResult{Code: result.Code, State: result.State, Iss: result.Issuer}, nil
		},
		NewTokenSource: func(ctx context.Context, cfg *oauth2.Config, token *oauth2.Token) (oauth2.TokenSource, error) {
			if err := config.Store.Save(ctx, config.ScopeKey, OAuthTokens{Config: *cfg, Token: *token}); err != nil {
				return nil, &MCPError{Kind: ErrAuthFailed, Operation: "OAuth token save", Cause: err}
			}
			return &savingTokenSource{source: cfg.TokenSource(ctx, token), cfg: *cfg, store: config.Store, scope: config.ScopeKey, last: *token}, nil
		},
	}
	if config.ClientIDMetadataURL != "" {
		settings.ClientIDMetadataDocumentConfig = &auth.ClientIDMetadataDocumentConfig{URL: config.ClientIDMetadataURL}
	}
	if config.ClientID != "" {
		credentials := &oauthex.ClientCredentials{ClientID: config.ClientID}
		if config.ClientSecret != "" {
			credentials.ClientSecretAuth = &oauthex.ClientSecretAuth{ClientSecret: config.ClientSecret}
		}
		settings.PreregisteredClient = credentials
	}
	if config.DynamicRegistration != nil {
		settings.DynamicClientRegistrationConfig = &auth.DynamicClientRegistrationConfig{Metadata: config.DynamicRegistration}
	}
	stored, found, err := config.Store.Load(ctx, config.ScopeKey)
	if err != nil {
		return nil, &MCPError{Kind: ErrAuthFailed, Operation: "OAuth load", Cause: err}
	}
	if found && stored.Config.Endpoint.TokenURL != "" && stored.Token.AccessToken != "" {
		refreshCtx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)
		settings.InitialTokenSource = &savingTokenSource{source: stored.Config.TokenSource(refreshCtx, &stored.Token), cfg: stored.Config, store: config.Store, scope: config.ScopeKey, last: stored.Token}
	}
	handler, err := auth.NewAuthorizationCodeHandler(settings)
	if err != nil {
		return nil, &MCPError{Kind: ErrConfig, Operation: "OAuth", Cause: err}
	}
	return handler, nil
}

type savingTokenSource struct {
	source oauth2.TokenSource
	cfg    oauth2.Config
	store  TokenStore
	scope  string
	mu     sync.Mutex
	last   oauth2.Token
}

func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, err := s.source.Token()
	if err != nil {
		return nil, err
	}
	if token.AccessToken != s.last.AccessToken || token.RefreshToken != s.last.RefreshToken || !token.Expiry.Equal(s.last.Expiry) {
		if err := s.store.Save(context.Background(), s.scope, OAuthTokens{Config: s.cfg, Token: *token}); err != nil {
			return nil, err
		}
		s.last = *token
	}
	return token, nil
}
