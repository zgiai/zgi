package integrations

import "time"

// CatalogWithConnectionHealth returns a cloned catalog enriched from
// caller-visible connection views. Authorization is intentionally outside this
// pure aggregation function so routes cannot accidentally aggregate records
// before applying their audience-specific visibility rules.
func CatalogWithConnectionHealth(items []ProviderCatalogItem, connections []ConnectionView, now time.Time) []ProviderCatalogItem {
	out := make([]ProviderCatalogItem, len(items))
	copy(out, items)
	byIntegration := make(map[string][]ConnectionView, len(out))
	for _, connection := range connections {
		if !supportedConnectionCredentialSource(connection.CredentialSource) {
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

func summarizeProviderConnections(item ProviderCatalogItem, connections []ConnectionView, now time.Time) (ProviderConnectionSummary, ProviderHealthState) {
	summary := ProviderConnectionSummary{Total: len(connections)}
	hasReady := false
	hasFailure := false
	for _, connection := range connections {
		switch connection.Status {
		case ConnectionStatusActive:
			summary.Active++
		case ConnectionStatusInvalid:
			summary.Invalid++
			hasFailure = true
		case ConnectionStatusDisabled:
			summary.Disabled++
		}
		switch connection.HealthStatus {
		case ConnectionHealthHealthy:
			summary.Healthy++
		case ConnectionHealthDegraded:
			summary.Degraded++
			hasFailure = true
		case ConnectionHealthUnhealthy:
			summary.Unhealthy++
			hasFailure = true
		default:
			summary.Unknown++
		}
		if connection.AuthStatus == ConnectionAuthReconnectRequired ||
			connection.AuthStatus == ConnectionAuthExpired ||
			refreshTokenExpiredForCatalog(connection, now) {
			summary.AuthRequired++
			hasFailure = true
		}
		if connection.ScopeStatus == ConnectionScopeDrifted {
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
		return summary, ProviderHealthStateUnavailable
	}
	if hasFailure {
		return summary, ProviderHealthStateDegraded
	}
	if hasReady {
		return summary, ProviderHealthStateReady
	}
	if len(connections) > 0 {
		return summary, ProviderHealthStateConfigured
	}
	return summary, ProviderHealthStateSetupRequired
}

func providerConnectionIsReady(connection ConnectionView, now time.Time) bool {
	return supportedConnectionCredentialSource(connection.CredentialSource) &&
		connection.Status == ConnectionStatusActive &&
		connection.HealthStatus == ConnectionHealthHealthy &&
		connection.AuthStatus == ConnectionAuthValid &&
		connection.ScopeStatus != ConnectionScopeDrifted &&
		connection.AttentionCode == nil &&
		!connectionExpiredForCatalog(connection, now)
}

func connectionExpiredForCatalog(connection ConnectionView, now time.Time) bool {
	return (connection.ExpiresAt != nil && !connection.ExpiresAt.After(now)) ||
		(connection.TokenExpiresAt != nil && !connection.TokenExpiresAt.After(now)) ||
		refreshTokenExpiredForCatalog(connection, now)
}

func refreshTokenExpiredForCatalog(connection ConnectionView, now time.Time) bool {
	return connection.RefreshTokenExpiresAt != nil && !connection.RefreshTokenExpiresAt.After(now)
}
