package integrations_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/dingtalk"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/exa"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/feishu"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/github"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/gmail"
)

func TestConnectionPermissionSummaryDoesNotClaimConnectorDeclaredScopesWereProviderReported(t *testing.T) {
	connection := &integrations.IntegrationConnection{
		IntegrationID: dingtalk.IntegrationID,
		AuthType:      integrations.ConnectionAuthTypeCustomCredential,
		AuthMethodID:  dingtalk.AuthMethodID,
		GrantedScopes: []string{dingtalk.ScopeContacts, dingtalk.ScopeAttendance, dingtalk.ScopeSend},
		ScopeStatus:   integrations.ConnectionScopeVerified,
	}
	summary := integrations.BuildConnectionPermissionSummary(connection, dingtalk.ProviderDefinition())
	if summary.ScopeEvidence != integrations.AuthScopeEvidenceConnectorDeclared {
		t.Fatalf("scope evidence = %q", summary.ScopeEvidence)
	}
	if summary.ProviderScopesReported {
		t.Fatal("connector-declared DingTalk scope groups were presented as provider-reported")
	}
	if len(summary.AdaptedCapabilities) != 12 {
		t.Fatalf("adapted capabilities = %d, want 12", len(summary.AdaptedCapabilities))
	}
	for _, capability := range summary.AdaptedCapabilities {
		if !capability.ScopeSatisfied {
			t.Fatalf("declared capability %s should remain executable", capability.ActionID)
		}
		if capability.ScopeVerified {
			t.Fatalf("declared capability %s was presented as provider-verified", capability.ActionID)
		}
		if capability.Availability != integrations.CapabilityAvailabilityRuntimeVerificationRequired {
			t.Fatalf("declared capability %s availability = %q, want runtime verification", capability.ActionID, capability.Availability)
		}
	}
}

func TestConnectionPermissionSummaryUsesExactConnectorActionEvidence(t *testing.T) {
	connection := &integrations.IntegrationConnection{
		IntegrationID:     dingtalk.IntegrationID,
		AuthType:          integrations.ConnectionAuthTypeCustomCredential,
		AuthMethodID:      dingtalk.AuthMethodID,
		VerifiedActionIDs: []string{dingtalk.ActionDepartmentList},
		DeniedActionIDs:   []string{dingtalk.ActionContactSearch},
	}
	summary := integrations.BuildConnectionPermissionSummary(connection, dingtalk.ProviderDefinition())
	var departments, contacts *integrations.ConnectionCapabilityPermission
	for index := range summary.AdaptedCapabilities {
		capability := &summary.AdaptedCapabilities[index]
		switch capability.ActionID {
		case dingtalk.ActionDepartmentList:
			departments = capability
		case dingtalk.ActionContactSearch:
			contacts = capability
		}
	}
	if departments == nil || !departments.ScopeSatisfied || !departments.ScopeVerified {
		t.Fatalf("department capability = %#v", departments)
	}
	if departments.Availability != integrations.CapabilityAvailabilityReady {
		t.Fatalf("department availability = %q, want ready", departments.Availability)
	}
	if contacts == nil || contacts.ScopeSatisfied || contacts.ScopeVerified ||
		contacts.Availability != integrations.CapabilityAvailabilityPermissionMissing {
		t.Fatalf("contact capability = %#v", contacts)
	}
}

func TestConnectionPermissionSummarySeparatesGitHubCapabilitiesAndProviderScopes(t *testing.T) {
	connection := &integrations.IntegrationConnection{
		IntegrationID: github.IntegrationID,
		AuthType:      integrations.ConnectionAuthTypeAPIKey,
		AuthMethodID:  github.AccountPATAuthMethodID,
		GrantedScopes: []string{"repo", "read:org", "metadata:read", "issues:read", "issues:write"},
	}
	summary := integrations.BuildConnectionPermissionSummary(connection, github.ProviderDefinition())
	if summary == nil {
		t.Fatal("summary is nil")
	}
	if len(summary.AdaptedCapabilities) != 8 {
		t.Fatalf("adapted capabilities = %d, want 8", len(summary.AdaptedCapabilities))
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
		if permission.ID == "metadata:read" || permission.ID == "issues:read" || permission.ID == "issues:write" {
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

func TestConnectionPermissionSummaryOffersLeastPrivilegeFeishuScopeUpgrade(t *testing.T) {
	connection := &integrations.IntegrationConnection{
		IntegrationID: feishu.IntegrationID,
		AuthType:      integrations.ConnectionAuthTypeOAuth2,
		AuthMethodID:  feishu.UserOAuthAuthMethodID,
		GrantedScopes: []string{"auth:user.id:read", feishu.ScopeOfflineAccess},
	}
	summary := integrations.BuildConnectionPermissionSummary(connection, feishu.ProviderDefinition())
	if summary == nil {
		t.Fatal("summary is nil")
	}
	var account, send *integrations.ConnectionCapabilityPermission
	for index := range summary.AdaptedCapabilities {
		capability := &summary.AdaptedCapabilities[index]
		switch capability.ActionID {
		case feishu.ActionGetAccount:
			account = capability
		case feishu.ActionSendUserMessage:
			send = capability
		}
	}
	if account == nil || !account.ScopeSatisfied ||
		account.Availability != integrations.CapabilityAvailabilityReady || account.CanUpgrade {
		t.Fatalf("account capability = %#v", account)
	}
	if send == nil || send.ScopeSatisfied || !send.CanUpgrade ||
		send.Availability != integrations.CapabilityAvailabilityScopeUpgradeRequired {
		t.Fatalf("send capability = %#v", send)
	}
	if !slices.Equal(send.MissingScopeIDs, []string{feishu.ScopeMessage, feishu.ScopeSendAsUser}) {
		t.Fatalf("send missing scopes = %#v", send.MissingScopeIDs)
	}
}

func TestConnectionPermissionSummaryHandlesAlternativeScopes(t *testing.T) {
	definition := integrations.ProviderDefinition{
		ID: "chat",
		AuthMethods: []integrations.AuthMethodDefinition{{
			ID: "oauth", Type: integrations.AuthMethodTypeOAuth2,
			OAuth: &integrations.OAuthMethodMetadata{ScopeUpgradeEnabled: true},
		}},
		Actions: []integrations.ActionDefinition{{
			ID: "chat.message.list", Name: "List messages",
			RequiredScopes:    []string{"chat:read"},
			RequiredAnyScopes: []string{"messages:read", "messages:history"},
			PreferredScopes:   []string{"messages:read"},
		}},
	}
	tests := []struct {
		name        string
		granted     []string
		satisfied   bool
		wantMissing []string
	}{
		{
			name:      "non-preferred alternative satisfies action",
			granted:   []string{"chat:read", "messages:history"},
			satisfied: true,
		},
		{
			name:        "missing alternative recommends preferred scope only",
			granted:     []string{"chat:read"},
			wantMissing: []string{"messages:read"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			connection := &integrations.IntegrationConnection{
				IntegrationID: "chat", AuthType: integrations.ConnectionAuthTypeOAuth2,
				AuthMethodID: "oauth", GrantedScopes: test.granted,
			}
			summary := integrations.BuildConnectionPermissionSummary(connection, definition)
			if len(summary.AdaptedCapabilities) != 1 {
				t.Fatalf("capabilities = %#v", summary.AdaptedCapabilities)
			}
			capability := summary.AdaptedCapabilities[0]
			if capability.ScopeSatisfied != test.satisfied {
				t.Fatalf("ScopeSatisfied = %v, want %v (%#v)", capability.ScopeSatisfied, test.satisfied, capability)
			}
			if !reflect.DeepEqual(capability.MissingScopeIDs, test.wantMissing) {
				t.Fatalf("MissingScopeIDs = %#v, want %#v", capability.MissingScopeIDs, test.wantMissing)
			}
			if !reflect.DeepEqual(capability.RequiredAnyScopes, []string{"messages:read", "messages:history"}) ||
				!reflect.DeepEqual(capability.PreferredScopes, []string{"messages:read"}) {
				t.Fatalf("alternative scope contract was not propagated: %#v", capability)
			}
		})
	}
}
