package catalog

import (
	"time"

	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
)

// WithConnectionHealth returns a cloned catalog enriched from
// caller-visible connection views. Authorization is intentionally outside this
// pure aggregation function so routes cannot accidentally aggregate records
// before applying their audience-specific visibility rules.
func WithConnectionHealth(items []integrations.ProviderCatalogItem, connections []integrations.ConnectionView, now time.Time) []integrations.ProviderCatalogItem {
	out := make([]integrations.ProviderCatalogItem, len(items))
	copy(out, items)
	byIntegration := make(map[string][]integrations.ConnectionView, len(out))
	for _, connection := range connections {
		if !supportedCredentialSource(connection.CredentialSource) {
			continue
		}
		byIntegration[connection.IntegrationID] = append(byIntegration[connection.IntegrationID], connection)
	}
	for index := range out {
		summary, state := summarizeProviderConnections(out[index], byIntegration[out[index].IntegrationID], now)
		out[index].ConnectionSummary = &summary
		out[index].HealthState = state
	}
	return out
}

func summarizeProviderConnections(item integrations.ProviderCatalogItem, connections []integrations.ConnectionView, now time.Time) (integrations.ProviderConnectionSummary, integrations.ProviderHealthState) {
	summary := integrations.ProviderConnectionSummary{Total: len(connections)}
	hasReady := false
	hasFailure := false
	for _, connection := range connections {
		switch connection.Status {
		case integrations.ConnectionStatusActive:
			summary.Active++
		case integrations.ConnectionStatusInvalid:
			summary.Invalid++
			hasFailure = true
		case integrations.ConnectionStatusDisabled:
			summary.Disabled++
		}
		switch connection.HealthStatus {
		case integrations.ConnectionHealthHealthy:
			summary.Healthy++
		case integrations.ConnectionHealthDegraded:
			summary.Degraded++
			hasFailure = true
		case integrations.ConnectionHealthUnhealthy:
			summary.Unhealthy++
			hasFailure = true
		default:
			summary.Unknown++
		}
		if connection.AuthStatus == integrations.ConnectionAuthReconnectRequired ||
			connection.AuthStatus == integrations.ConnectionAuthExpired ||
			refreshTokenExpiredForCatalog(connection, now) {
			summary.AuthRequired++
			hasFailure = true
		}
		if connection.ScopeStatus == integrations.ConnectionScopeDrifted {
			summary.ScopeDrifted++
			hasFailure = true
		}
		if connection.AttentionCode != nil || connectionExpiredForCatalog(connection, now) {
			hasFailure = true
		}
		if connection.IsDefault {
			id := connection.ID.String()
			summary.DefaultConnectionID = &id
		}
		if providerConnectionIsReady(connection, now) {
			hasReady = true
		}
	}
	if !item.Enabled {
		return summary, integrations.ProviderHealthStateUnavailable
	}
	if hasFailure {
		return summary, integrations.ProviderHealthStateDegraded
	}
	if hasReady {
		return summary, integrations.ProviderHealthStateReady
	}
	if len(connections) > 0 {
		return summary, integrations.ProviderHealthStateConfigured
	}
	return summary, integrations.ProviderHealthStateSetupRequired
}

func providerConnectionIsReady(connection integrations.ConnectionView, now time.Time) bool {
	return supportedCredentialSource(connection.CredentialSource) &&
		connection.Status == integrations.ConnectionStatusActive &&
		connection.HealthStatus == integrations.ConnectionHealthHealthy &&
		connection.AuthStatus == integrations.ConnectionAuthValid &&
		connection.ScopeStatus != integrations.ConnectionScopeDrifted &&
		connection.AttentionCode == nil &&
		!connectionExpiredForCatalog(connection, now)
}

func connectionExpiredForCatalog(connection integrations.ConnectionView, now time.Time) bool {
	return (connection.ExpiresAt != nil && !connection.ExpiresAt.After(now)) ||
		(connection.TokenExpiresAt != nil && !connection.TokenExpiresAt.After(now)) ||
		refreshTokenExpiredForCatalog(connection, now)
}

func refreshTokenExpiredForCatalog(connection integrations.ConnectionView, now time.Time) bool {
	return connection.RefreshTokenExpiresAt != nil && !connection.RefreshTokenExpiresAt.After(now)
}

func supportedCredentialSource(source integrations.ConnectionCredentialSource) bool {
	return source == integrations.ConnectionCredentialSourceOrganization || source == integrations.ConnectionCredentialSourceAccount
}
