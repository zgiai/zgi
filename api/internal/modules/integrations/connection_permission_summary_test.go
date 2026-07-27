package integrations_test

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/exa"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/github"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/gmail"
)

func TestConnectionPermissionSummarySeparatesGitHubCapabilitiesAndProviderScopes(t *testing.T) {
	connection := &integrations.IntegrationConnection{
		IntegrationID: github.IntegrationID,
		AuthType:      integrations.ConnectionAuthTypeAPIKey,
		AuthMethodID:  github.AccountPATAuthMethodID,
		GrantedScopes: []string{"repo", "read:org", "metadata:read", "issues:read"},
	}
	summary := integrations.BuildConnectionPermissionSummary(connection, github.ProviderDefinition())
	if summary == nil {
		t.Fatal("summary is nil")
	}
	if len(summary.AdaptedCapabilities) != 3 {
		t.Fatalf("adapted capabilities = %d, want 3", len(summary.AdaptedCapabilities))
	}
	for _, capability := range summary.AdaptedCapabilities {
		if !capability.ScopeSatisfied {
			t.Fatalf("capability %s unexpectedly missing scopes: %#v", capability.ActionID, capability.MissingScopeIDs)
		}
	}
	if !summary.HasBroadPermissions {
		t.Fatal("classic repo scope was not marked broad")
	}
	if len(summary.ProviderPermissions) != 2 {
		t.Fatalf("provider permissions = %#v, want repo and read:org only", summary.ProviderPermissions)
	}
	for _, permission := range summary.ProviderPermissions {
		if permission.ID == "metadata:read" || permission.ID == "issues:read" {
			t.Fatalf("internal derived scope leaked into provider permissions: %#v", permission)
		}
	}
}

func TestConnectionPermissionSummaryKeepsUnknownProviderScopeIdentifiable(t *testing.T) {
	connection := &integrations.IntegrationConnection{
		IntegrationID: github.IntegrationID,
		AuthType:      integrations.ConnectionAuthTypeAPIKey,
		AuthMethodID:  github.AccountPATAuthMethodID,
		GrantedScopes: []string{"future:scope"},
	}
	summary := integrations.BuildConnectionPermissionSummary(connection, github.ProviderDefinition())
	if len(summary.UnknownPermissions) != 1 {
		t.Fatalf("unknown permissions = %#v", summary.UnknownPermissions)
	}
	permission := summary.UnknownPermissions[0]
	if permission.ID != "future:scope" || permission.Label != "future:scope" || permission.Known {
		t.Fatalf("unknown permission = %#v", permission)
	}
}

func TestConnectionPermissionSummaryGroupsOAuthIdentityAndMissingActionScope(t *testing.T) {
	connection := &integrations.IntegrationConnection{
		IntegrationID:         gmail.IntegrationID,
		AuthType:              integrations.ConnectionAuthTypeOAuth2,
		AuthMethodID:          gmail.AccountOAuthAuthMethodID,
		GrantedScopes:         []string{gmail.ScopeOpenID, gmail.ScopeEmail},
		MissingRequiredScopes: []string{gmail.ScopeMailSend},
	}
	summary := integrations.BuildConnectionPermissionSummary(connection, gmail.ProviderDefinition())
	if len(summary.IdentityPermissions) != 2 {
		t.Fatalf("identity permissions = %#v", summary.IdentityPermissions)
	}
	if len(summary.MissingPermissions) != 1 || summary.MissingPermissions[0].ID != gmail.ScopeMailSend {
		t.Fatalf("missing permissions = %#v", summary.MissingPermissions)
	}
	var sendSatisfied bool
	for _, capability := range summary.AdaptedCapabilities {
		if capability.ActionID == gmail.ActionSendMail {
			sendSatisfied = capability.ScopeSatisfied
		}
	}
	if sendSatisfied {
		t.Fatal("Gmail send capability was reported available without gmail.send")
	}
}

func TestConnectionPermissionSummaryDoesNotClaimAPIKeyScopeList(t *testing.T) {
	connection := &integrations.IntegrationConnection{
		IntegrationID: integrations.IntegrationWebSearch,
		AuthType:      integrations.ConnectionAuthTypeAPIKey,
		AuthMethodID:  exa.AccountAPIKeyAuthMethodID,
	}
	summary := integrations.BuildConnectionPermissionSummary(connection, exa.ProviderDefinition("auto"))
	if summary.ProviderScopesReported {
		t.Fatal("API key connection unexpectedly reports provider scopes")
	}
	for _, capability := range summary.AdaptedCapabilities {
		if !capability.ScopeSatisfied {
			t.Fatalf("API key capability %s should be verified at action runtime", capability.ActionID)
		}
	}
}
