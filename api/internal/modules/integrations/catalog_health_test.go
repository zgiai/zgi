package integrations

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCatalogWithConnectionHealthRequiresObservedUsableConnection(t *testing.T) {
	now := time.Date(2026, 7, 22, 8, 0, 0, 0, time.UTC)
	base := ProviderCatalogItem{ID: "github", IntegrationID: "github", Enabled: true}
	tests := []struct {
		name        string
		item        ProviderCatalogItem
		connections []ConnectionView
		want        ProviderHealthState
	}{
		{name: "not configured", item: base, want: ProviderHealthStateSetupRequired},
		{name: "active unknown is configured", item: base, connections: []ConnectionView{{ID: uuid.New(), IntegrationID: "github", CredentialSource: ConnectionCredentialSourceOrganization, Status: ConnectionStatusActive, HealthStatus: ConnectionHealthUnknown, AuthStatus: ConnectionAuthUnknown}}, want: ProviderHealthStateConfigured},
		{name: "healthy with unknown auth is configured", item: base, connections: []ConnectionView{{ID: uuid.New(), IntegrationID: "github", CredentialSource: ConnectionCredentialSourceOrganization, Status: ConnectionStatusActive, HealthStatus: ConnectionHealthHealthy, AuthStatus: ConnectionAuthUnknown}}, want: ProviderHealthStateConfigured},
		{name: "observed healthy and usable is ready", item: base, connections: []ConnectionView{{ID: uuid.New(), IntegrationID: "github", CredentialSource: ConnectionCredentialSourceOrganization, Status: ConnectionStatusActive, HealthStatus: ConnectionHealthHealthy, AuthStatus: ConnectionAuthValid, ScopeStatus: ConnectionScopeVerified}}, want: ProviderHealthStateReady},
		{name: "scope drift is degraded", item: base, connections: []ConnectionView{{ID: uuid.New(), IntegrationID: "github", CredentialSource: ConnectionCredentialSourceOrganization, Status: ConnectionStatusActive, HealthStatus: ConnectionHealthHealthy, AuthStatus: ConnectionAuthValid, ScopeStatus: ConnectionScopeDrifted}}, want: ProviderHealthStateDegraded},
		{name: "expired auth is degraded", item: base, connections: []ConnectionView{{ID: uuid.New(), IntegrationID: "github", CredentialSource: ConnectionCredentialSourceOrganization, Status: ConnectionStatusActive, HealthStatus: ConnectionHealthHealthy, AuthStatus: ConnectionAuthExpired}}, want: ProviderHealthStateDegraded},
		{name: "legacy platform is ignored", item: base, connections: []ConnectionView{{ID: uuid.New(), IntegrationID: "github", CredentialSource: ConnectionCredentialSourcePlatform, Status: ConnectionStatusActive, HealthStatus: ConnectionHealthHealthy, AuthStatus: ConnectionAuthValid}}, want: ProviderHealthStateSetupRequired},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			items := CatalogWithConnectionHealth([]ProviderCatalogItem{testCase.item}, testCase.connections, now)
			if len(items) != 1 || items[0].HealthState != testCase.want {
				t.Fatalf("health state = %q, want %q; item=%#v", items[0].HealthState, testCase.want, items[0])
			}
		})
	}
}

func TestCatalogWithConnectionHealthReportsMixedHealthyAndFailingConnectionsAsDegraded(t *testing.T) {
	now := time.Now().UTC()
	defaultID := uuid.New()
	items := CatalogWithConnectionHealth([]ProviderCatalogItem{{ID: "github", IntegrationID: "github", Enabled: true}}, []ConnectionView{
		{ID: defaultID, IntegrationID: "github", CredentialSource: ConnectionCredentialSourceOrganization, Name: "secret personal label", Status: ConnectionStatusActive, HealthStatus: ConnectionHealthHealthy, AuthStatus: ConnectionAuthValid, ScopeStatus: ConnectionScopeVerified, IsDefault: true},
		{ID: uuid.New(), IntegrationID: "github", CredentialSource: ConnectionCredentialSourceOrganization, Status: ConnectionStatusInvalid, HealthStatus: ConnectionHealthUnhealthy, AuthStatus: ConnectionAuthReconnectRequired},
	}, now)
	item := items[0]
	if item.HealthState != ProviderHealthStateDegraded {
		t.Fatalf("health state = %q, want degraded", item.HealthState)
	}
	if item.ConnectionSummary == nil || item.ConnectionSummary.Total != 2 || item.ConnectionSummary.Active != 1 || item.ConnectionSummary.Invalid != 1 || item.ConnectionSummary.Healthy != 1 || item.ConnectionSummary.Unhealthy != 1 || item.ConnectionSummary.AuthRequired != 1 {
		t.Fatalf("connection summary = %#v", item.ConnectionSummary)
	}
	if item.ConnectionSummary.DefaultConnectionID == nil || *item.ConnectionSummary.DefaultConnectionID != defaultID.String() {
		t.Fatalf("default connection id = %#v", item.ConnectionSummary.DefaultConnectionID)
	}
}

func TestCatalogWithConnectionHealthReportsExpiredRefreshTokenAsAuthenticationRequired(t *testing.T) {
	now := time.Date(2026, 7, 23, 8, 0, 0, 0, time.UTC)
	refreshExpiry := now.Add(-time.Minute)
	items := CatalogWithConnectionHealth(
		[]ProviderCatalogItem{{ID: "gmail", IntegrationID: "gmail", Enabled: true}},
		[]ConnectionView{{
			ID: uuid.New(), IntegrationID: "gmail",
			CredentialSource: ConnectionCredentialSourceAccount,
			Status:           ConnectionStatusActive, HealthStatus: ConnectionHealthHealthy,
			AuthStatus: ConnectionAuthValid, ScopeStatus: ConnectionScopeVerified,
			RefreshTokenExpiresAt: &refreshExpiry,
		}},
		now,
	)
	if len(items) != 1 || items[0].HealthState != ProviderHealthStateDegraded ||
		items[0].ConnectionSummary == nil || items[0].ConnectionSummary.AuthRequired != 1 {
		t.Fatalf("expired refresh token catalog state = %#v", items)
	}
}
