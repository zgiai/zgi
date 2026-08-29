package catalog

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
)

func TestCatalogWithConnectionHealthRequiresObservedUsableConnection(t *testing.T) {
	now := time.Date(2026, 7, 22, 8, 0, 0, 0, time.UTC)
	base := integrations.ProviderCatalogItem{ID: "github", IntegrationID: "github", Enabled: true}
	tests := []struct {
		name        string
		item        integrations.ProviderCatalogItem
		connections []integrations.ConnectionView
		want        integrations.ProviderHealthState
	}{
		{name: "not configured", item: base, want: integrations.ProviderHealthStateSetupRequired},
		{name: "active unknown is configured", item: base, connections: []integrations.ConnectionView{{ID: uuid.New(), IntegrationID: "github", CredentialSource: integrations.ConnectionCredentialSourceOrganization, Status: integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthUnknown, AuthStatus: integrations.ConnectionAuthUnknown}}, want: integrations.ProviderHealthStateConfigured},
		{name: "healthy with unknown auth is configured", item: base, connections: []integrations.ConnectionView{{ID: uuid.New(), IntegrationID: "github", CredentialSource: integrations.ConnectionCredentialSourceOrganization, Status: integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthHealthy, AuthStatus: integrations.ConnectionAuthUnknown}}, want: integrations.ProviderHealthStateConfigured},
		{name: "observed healthy and usable is ready", item: base, connections: []integrations.ConnectionView{{ID: uuid.New(), IntegrationID: "github", CredentialSource: integrations.ConnectionCredentialSourceOrganization, Status: integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthHealthy, AuthStatus: integrations.ConnectionAuthValid, ScopeStatus: integrations.ConnectionScopeVerified}}, want: integrations.ProviderHealthStateReady},
		{name: "scope drift is degraded", item: base, connections: []integrations.ConnectionView{{ID: uuid.New(), IntegrationID: "github", CredentialSource: integrations.ConnectionCredentialSourceOrganization, Status: integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthHealthy, AuthStatus: integrations.ConnectionAuthValid, ScopeStatus: integrations.ConnectionScopeDrifted}}, want: integrations.ProviderHealthStateDegraded},
		{name: "expired auth is degraded", item: base, connections: []integrations.ConnectionView{{ID: uuid.New(), IntegrationID: "github", CredentialSource: integrations.ConnectionCredentialSourceOrganization, Status: integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthHealthy, AuthStatus: integrations.ConnectionAuthExpired}}, want: integrations.ProviderHealthStateDegraded},
		{name: "legacy platform is ignored", item: base, connections: []integrations.ConnectionView{{ID: uuid.New(), IntegrationID: "github", CredentialSource: integrations.ConnectionCredentialSourcePlatform, Status: integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthHealthy, AuthStatus: integrations.ConnectionAuthValid}}, want: integrations.ProviderHealthStateSetupRequired},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			items := WithConnectionHealth([]integrations.ProviderCatalogItem{testCase.item}, testCase.connections, now)
			if len(items) != 1 || items[0].HealthState != testCase.want {
				t.Fatalf("health state = %q, want %q; item=%#v", items[0].HealthState, testCase.want, items[0])
			}
		})
	}
}

func TestCatalogWithConnectionHealthReportsMixedHealthyAndFailingConnectionsAsDegraded(t *testing.T) {
	now := time.Now().UTC()
	defaultID := uuid.New()
	items := WithConnectionHealth([]integrations.ProviderCatalogItem{{ID: "github", IntegrationID: "github", Enabled: true}}, []integrations.ConnectionView{
		{ID: defaultID, IntegrationID: "github", CredentialSource: integrations.ConnectionCredentialSourceOrganization, Name: "secret personal label", Status: integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthHealthy, AuthStatus: integrations.ConnectionAuthValid, ScopeStatus: integrations.ConnectionScopeVerified, IsDefault: true},
		{ID: uuid.New(), IntegrationID: "github", CredentialSource: integrations.ConnectionCredentialSourceOrganization, Status: integrations.ConnectionStatusInvalid, HealthStatus: integrations.ConnectionHealthUnhealthy, AuthStatus: integrations.ConnectionAuthReconnectRequired},
	}, now)
	item := items[0]
	if item.HealthState != integrations.ProviderHealthStateDegraded {
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
	items := WithConnectionHealth(
		[]integrations.ProviderCatalogItem{{ID: "gmail", IntegrationID: "gmail", Enabled: true}},
		[]integrations.ConnectionView{{
			ID: uuid.New(), IntegrationID: "gmail",
			CredentialSource: integrations.ConnectionCredentialSourceAccount,
			Status:           integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthHealthy,
			AuthStatus: integrations.ConnectionAuthValid, ScopeStatus: integrations.ConnectionScopeVerified,
			RefreshTokenExpiresAt: &refreshExpiry,
		}},
		now,
	)
	if len(items) != 1 || items[0].HealthState != integrations.ProviderHealthStateDegraded ||
		items[0].ConnectionSummary == nil || items[0].ConnectionSummary.AuthRequired != 1 {
		t.Fatalf("expired refresh token catalog state = %#v", items)
	}
}
