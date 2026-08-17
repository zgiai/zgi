package agents

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

type stubIntegrationActionCatalog map[string]struct{}

func (catalog stubIntegrationActionCatalog) HasAction(integrationID, actionID string) bool {
	_, ok := catalog[strings.ToLower(strings.TrimSpace(integrationID))+"/"+strings.ToLower(strings.TrimSpace(actionID))]
	return ok
}

func (catalog stubIntegrationActionCatalog) ActionDetail(integrationID, actionID string) (integrations.ActionDefinition, bool) {
	if !catalog.HasAction(integrationID, actionID) {
		return integrations.ActionDefinition{}, false
	}
	effect := toolgovernance.EffectRead
	if strings.Contains(strings.ToLower(actionID), "write") {
		effect = toolgovernance.EffectUpdate
	}
	return integrations.ActionDefinition{ID: actionID, Effect: effect}, true
}

func (catalog stubIntegrationActionCatalog) Actions(integrationID string) []integrations.ActionDefinition {
	prefix := strings.ToLower(strings.TrimSpace(integrationID)) + "/"
	actions := make([]integrations.ActionDefinition, 0)
	for key := range catalog {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		actionID := strings.TrimPrefix(key, prefix)
		action, ok := catalog.ActionDetail(integrationID, actionID)
		if ok {
			actions = append(actions, action)
		}
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].ID < actions[j].ID })
	return actions
}

type agentIntegrationActionPolicyResolverFunc func(context.Context, string, string, integrations.ActionDefinition) (integrations.ActionPolicyDecision, error)

func (fn agentIntegrationActionPolicyResolverFunc) Resolve(
	ctx context.Context,
	organizationID string,
	integrationID string,
	action integrations.ActionDefinition,
) (integrations.ActionPolicyDecision, error) {
	return fn(ctx, organizationID, integrationID, action)
}

var allowAgentIntegrationActionPolicies = agentIntegrationActionPolicyResolverFunc(func(
	context.Context,
	string,
	string,
	integrations.ActionDefinition,
) (integrations.ActionPolicyDecision, error) {
	return integrations.ActionPolicyDecision{Enabled: true, DataEgressAllowed: true}, nil
})

func TestAgentRunConfigCarriesServerOwnedIntegrationConnectionAndActionAllowlist(t *testing.T) {
	runConfig := agentRunConfig("agent-1", "agent.draft", dto.AgentConfigResponse{
		IntegrationBindings: []dto.AgentIntegrationBinding{{
			ConnectionID:     "connection-1",
			IntegrationID:    "web-search",
			AccessMode:       "read",
			AllowedActionIDs: []string{"web.search"},
		}},
		BindingAuthorizations: []dto.AgentBindingAuthorization{{
			BindingType:      "integration_connection",
			ResourceID:       "connection-1",
			ParentResourceID: "web-search",
			AccessMode:       "read",
			AllowedActionIDs: []string{"web.search"},
			BoundByAccountID: "account-1",
			BoundAtUnix:      123,
		}},
	}, "account")
	if runConfig.IntegrationConnectionIDs["web-search"] != "connection-1" {
		t.Fatalf("IntegrationConnectionIDs = %#v", runConfig.IntegrationConnectionIDs)
	}
	if !reflect.DeepEqual(runConfig.IntegrationSelectedConnectionIDs, map[string][]string{"web-search": {"connection-1"}}) {
		t.Fatalf("IntegrationSelectedConnectionIDs = %#v", runConfig.IntegrationSelectedConnectionIDs)
	}
	if len(runConfig.BindingAuthorizations) != 1 || len(runConfig.BindingAuthorizations[0].AllowedActionIDs) != 1 || runConfig.BindingAuthorizations[0].AllowedActionIDs[0] != "web.search" {
		t.Fatalf("BindingAuthorizations = %#v", runConfig.BindingAuthorizations)
	}
}

func TestFilterAgentConfigByBindingHealthPreservesMatchingIntegrationAuthorization(t *testing.T) {
	config := dto.AgentConfigResponse{
		IntegrationBindings: []dto.AgentIntegrationBinding{{
			ConnectionID:     "connection-1",
			IntegrationID:    "web-search",
			AccessMode:       "read",
			AllowedActionIDs: []string{"web.search"},
		}},
		BindingAuthorizations: []dto.AgentBindingAuthorization{{
			BindingType:      "integration_connection",
			ResourceID:       "connection-1",
			ParentResourceID: "web-search",
			AccessMode:       "read",
			AllowedActionIDs: []string{"web.search"},
			BoundByAccountID: "account-1",
			BoundAtUnix:      123,
		}},
		BindingHealth: dto.AgentBindingHealth{
			Status:      agentBindingStatusActive,
			ActiveCount: 1,
			Items: []dto.AgentBindingHealthItem{{
				BindingType:      "integration_connection",
				ResourceID:       "connection-1",
				ParentResourceID: "web-search",
				AccessMode:       "read",
				AllowedActionIDs: []string{"web.search"},
				Status:           agentBindingStatusActive,
			}},
		},
	}

	filtered := filterAgentConfigByBindingHealth(config)
	if len(filtered.IntegrationBindings) != 1 || len(filtered.BindingAuthorizations) != 1 {
		t.Fatalf("filtered integration config = %#v", filtered)
	}

	config.BindingAuthorizations[0].AllowedActionIDs = []string{"web.fetch"}
	filtered = filterAgentConfigByBindingHealth(config)
	if len(filtered.BindingAuthorizations) != 0 {
		t.Fatalf("mismatched action authorization survived health filter: %#v", filtered.BindingAuthorizations)
	}
}

func TestValidateIntegrationBindingGrantRejectsUnknownAndOversizedActionAllowlists(t *testing.T) {
	connections := &agentIntegrationConnectionRepository{items: map[uuid.UUID]*integrations.IntegrationConnection{}}
	service := &agentsService{
		integrationActions: stubIntegrationActionCatalog{
			"web-search/web.search": {},
		},
		integrationConnections: connections,
		integrationAccess:      agentIntegrationACLService(connections, &agentIntegrationGrantRepository{}),
	}
	binding := dto.AgentIntegrationBinding{
		ConnectionID:     "33333333-3333-3333-3333-333333333333",
		IntegrationID:    "web-search",
		AccessMode:       "read",
		AllowedActionIDs: []string{"web.unknown"},
	}
	if err := service.validateIntegrationBindingGrant(t.Context(), "22222222-2222-2222-2222-222222222222", "44444444-4444-4444-4444-444444444444", "99999999-9999-9999-9999-999999999999", binding); err == nil {
		t.Fatal("unknown action should be rejected")
	}

	binding.AllowedActionIDs = make([]string, 0, maxAgentIntegrationAllowedActions+1)
	for index := 0; index <= maxAgentIntegrationAllowedActions; index++ {
		binding.AllowedActionIDs = append(binding.AllowedActionIDs, fmt.Sprintf("web.action.%d", index))
	}
	if err := service.validateIntegrationBindingGrant(t.Context(), "22222222-2222-2222-2222-222222222222", "44444444-4444-4444-4444-444444444444", "99999999-9999-9999-9999-999999999999", binding); err == nil {
		t.Fatal("oversized action allowlist should be rejected")
	}

	binding.AllowedActionIDs = []string{"web." + strings.Repeat("x", maxAgentIntegrationActionIDLength)}
	if err := service.validateIntegrationBindingGrant(t.Context(), "22222222-2222-2222-2222-222222222222", "44444444-4444-4444-4444-444444444444", "99999999-9999-9999-9999-999999999999", binding); err == nil {
		t.Fatal("oversized action id should be rejected")
	}
}

func TestValidateIntegrationBindingGrantAcceptsRegisteredActiveConnection(t *testing.T) {
	organizationID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	workspaceID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	accountID := uuid.MustParse("99999999-9999-9999-9999-999999999999")
	connectionID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	connections := &agentIntegrationConnectionRepository{items: map[uuid.UUID]*integrations.IntegrationConnection{
		connectionID: {
			ID: connectionID, OrganizationID: organizationID, IntegrationID: "web-search", DriverID: "exa", Name: "Search",
			CredentialSource: integrations.ConnectionCredentialSourceOrganization, Status: integrations.ConnectionStatusActive,
			HealthStatus: integrations.ConnectionHealthHealthy, AuthStatus: integrations.ConnectionAuthValid,
		},
	}}
	grants := &agentIntegrationGrantRepository{grants: []integrations.IntegrationConnectionGrant{{
		OrganizationID: organizationID, ConnectionID: connectionID,
		PrincipalType: integrations.ConnectionGrantPrincipalWorkspace, PrincipalID: &workspaceID,
		AccessMode: integrations.ConnectionGrantAccessRead, AllowedActionIDs: []string{"web.search"}, ResourceConstraints: map[string]any{},
	}}}
	service := &agentsService{
		integrationActions: stubIntegrationActionCatalog{
			"web-search/web.search": {},
		},
		integrationConnections: connections,
		integrationAccess:      agentIntegrationACLService(connections, grants),
	}
	err := service.validateIntegrationBindingGrant(t.Context(), organizationID.String(), workspaceID.String(), accountID.String(), dto.AgentIntegrationBinding{
		ConnectionID:     "33333333-3333-3333-3333-333333333333",
		IntegrationID:    "web-search",
		AccessMode:       "read",
		AllowedActionIDs: []string{"WEB.SEARCH", "web.search"},
	})
	if err != nil {
		t.Fatalf("validateIntegrationBindingGrant() error = %v", err)
	}
}

func TestValidateIntegrationBindingGrantRejectsWriteActionForReadOnlyBinding(t *testing.T) {
	organizationID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	workspaceID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	accountID := uuid.MustParse("99999999-9999-9999-9999-999999999999")
	connectionID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	connections := &agentIntegrationConnectionRepository{items: map[uuid.UUID]*integrations.IntegrationConnection{
		connectionID: {
			ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github", Name: "GitHub",
			CredentialSource: integrations.ConnectionCredentialSourceOrganization, Status: integrations.ConnectionStatusActive,
			HealthStatus: integrations.ConnectionHealthHealthy, AuthStatus: integrations.ConnectionAuthValid,
		},
	}}
	grants := &agentIntegrationGrantRepository{grants: []integrations.IntegrationConnectionGrant{{
		OrganizationID: organizationID, ConnectionID: connectionID,
		PrincipalType: integrations.ConnectionGrantPrincipalWorkspace, PrincipalID: &workspaceID,
		AccessMode: integrations.ConnectionGrantAccessWrite, AllowedActionIDs: []string{"github.issue.write"}, ResourceConstraints: map[string]any{},
	}}}
	service := &agentsService{
		integrationActions: stubIntegrationActionCatalog{
			"github/github.issue.write": {},
		},
		integrationConnections: connections,
		integrationAccess:      agentIntegrationACLService(connections, grants),
	}

	err := service.validateIntegrationBindingGrant(t.Context(), organizationID.String(), workspaceID.String(), accountID.String(), dto.AgentIntegrationBinding{
		ConnectionID:     connectionID.String(),
		IntegrationID:    "github",
		AccessMode:       "read",
		AllowedActionIDs: []string{"github.issue.write"},
	})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("read-only binding write action error = %v", err)
	}
}
