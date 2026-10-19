package container

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/connectors/socialoauth"
	"gorm.io/gorm"
)

func (c *ServiceContainer) GetConnectorSocialOAuthService() *socialoauth.Service {
	if c.connectorSocialOAuth == nil {
		providers := make(map[string]socialoauth.ProviderConfig)
		for id, cfg := range c.config.Connectors.SocialOAuthProviders {
			providers[id] = socialoauth.ProviderConfig{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, RedirectURL: cfg.RedirectURL, Scopes: cfg.Scopes, APIVersion: cfg.APIVersion}
		}
		service, err := socialoauth.New(c.db, c.getConnectorCredentialCipher(), c.GetConnectorConnectionService(), socialoauth.Options{
			Providers: providers, FrontendURL: c.config.Connectors.SocialOAuthFrontendURL,
			LifecycleFactory: c.connectorSocialOAuthLifecycle,
			Reconnect: func(ctx context.Context, cmd connections.ReconnectCommand) (connections.ReconnectResult, error) {
				return c.GetConnectorMaintenanceService().Reconnect(ctx, cmd)
			},
		})
		if err != nil {
			panic(fmt.Sprintf("initialize social OAuth service: %v", err))
		}
		c.connectorSocialOAuth = service
	}
	return c.connectorSocialOAuth
}

// Resolve lazily: maintenance is also used by OAuth reconnect, so constructing
// either service must not recursively construct the other.
type connectorRefreshingCredentials struct{ container *ServiceContainer }

func (c *ServiceContainer) GetConnectorRefreshingCredentialResolver() *connectorRefreshingCredentials {
	return &connectorRefreshingCredentials{container: c}
}
func (r *connectorRefreshingCredentials) ResolveCredentials(ctx context.Context, request execution.CredentialResolutionRequest) (map[string]string, error) {
	if request.ConnectorID != "x-social" && request.ConnectorID != "linkedin-social" && request.ConnectorID != "facebook-social" {
		return r.container.GetConnectorCredentialResolver().ResolveCredentials(ctx, request)
	}
	return r.container.GetConnectorSocialOAuthService().ResolveCredentials(ctx, request)
}
func (r *connectorRefreshingCredentials) ResolveConnectionCredentials(ctx context.Context, c core.PersonalConnection) (map[string]string, error) {
	if c.ConnectorID != "x-social" && c.ConnectorID != "linkedin-social" && c.ConnectorID != "facebook-social" {
		return r.container.GetConnectorCredentialResolver().ResolveConnectionCredentials(ctx, c)
	}
	return r.container.GetConnectorSocialOAuthService().ResolveConnectionCredentials(ctx, c)
}

// Build transaction-scoped native persistence for callback completion. Both the
// native credential and its OAuth grant commit together; verifier calls still
// use the same fixed provider runtime as manual-token onboarding.
func (c *ServiceContainer) connectorSocialOAuthLifecycle(tx *gorm.DB) (socialoauth.Lifecycle, func(context.Context, connections.ReconnectCommand) (connections.ReconnectResult, error), error) {
	_ = c.GetConnectorMaintenanceService()
	repo := connections.NewGormRepository(tx)
	lifecycle, err := connections.NewService(c.GetConnectorPluginManager(), repo, c.GetConnectorCandidateVerifier(), c.GetConnectorCredentialSealer(), c.connectorCommandHasher)
	if err != nil {
		return nil, nil, err
	}
	maintenance, err := connections.NewMaintenanceService(c.GetConnectorPluginManager(), repo, bindings.NewGormRepository(tx), c.GetConnectorCandidateVerifier(), c.GetConnectorCredentialSealer(), c.GetConnectorCredentialResolver(), c.connectorCommandHasher, c.connectorImpactTokenCodec)
	if err != nil {
		return nil, nil, err
	}
	return lifecycle, maintenance.Reconnect, nil
}
