package agents

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type explicitAgentIntegrationActionCatalog struct {
	integrationID string
	actions       []integrations.ActionDefinition
}

func (catalog explicitAgentIntegrationActionCatalog) HasAction(integrationID, actionID string) bool {
	_, ok := catalog.ActionDetail(integrationID, actionID)
	return ok
}

func (catalog explicitAgentIntegrationActionCatalog) ActionDetail(integrationID, actionID string) (integrations.ActionDefinition, bool) {
	if !strings.EqualFold(catalog.integrationID, integrationID) {
		return integrations.ActionDefinition{}, false
	}
	for _, action := range catalog.actions {
		if strings.EqualFold(action.ID, actionID) {
			return action, true
		}
	}
	return integrations.ActionDefinition{}, false
}

func (catalog explicitAgentIntegrationActionCatalog) Actions(integrationID string) []integrations.ActionDefinition {
	if !strings.EqualFold(catalog.integrationID, integrationID) {
		return nil
	}
	return append([]integrations.ActionDefinition(nil), catalog.actions...)
}

type agentIntegrationConnectionRepository struct {
	items map[uuid.UUID]*integrations.IntegrationConnection
}

func (repository *agentIntegrationConnectionRepository) Create(_ context.Context, connection *integrations.IntegrationConnection) error {
	if repository.items == nil {
		repository.items = map[uuid.UUID]*integrations.IntegrationConnection{}
	}
	copyValue := *connection
	repository.items[connection.ID] = &copyValue
	return nil
}

func (repository *agentIntegrationConnectionRepository) GetByID(_ context.Context, organizationID, connectionID uuid.UUID) (*integrations.IntegrationConnection, error) {
	connection := repository.items[connectionID]
	if connection == nil || connection.OrganizationID != organizationID {
		return nil, integrations.ErrConnectionNotFound
	}
	copyValue := *connection
	return &copyValue, nil
}

func (repository *agentIntegrationConnectionRepository) List(_ context.Context, organizationID uuid.UUID, filter integrations.ConnectionListFilter) ([]*integrations.IntegrationConnection, error) {
	items := make([]*integrations.IntegrationConnection, 0, len(repository.items))
	for _, connection := range repository.items {
		if connection.OrganizationID != organizationID || (filter.IntegrationID != "" && !strings.EqualFold(connection.IntegrationID, filter.IntegrationID)) {
			continue
		}
		copyValue := *connection
		items = append(items, &copyValue)
	}
	return items, nil
}

func (repository *agentIntegrationConnectionRepository) Count(ctx context.Context, organizationID uuid.UUID, filter integrations.ConnectionListFilter) (int64, error) {
	items, err := repository.List(ctx, organizationID, filter)
	return int64(len(items)), err
}

func (*agentIntegrationConnectionRepository) GetDefault(context.Context, uuid.UUID, string, string) (*integrations.IntegrationConnection, error) {
	return nil, integrations.ErrConnectionNotFound
}

func (repository *agentIntegrationConnectionRepository) Update(_ context.Context, connection *integrations.IntegrationConnection) error {
	if repository.items[connection.ID] == nil {
		return integrations.ErrConnectionNotFound
	}
	copyValue := *connection
	repository.items[connection.ID] = &copyValue
	return nil
}

func (*agentIntegrationConnectionRepository) SetDefault(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (repository *agentIntegrationConnectionRepository) Delete(_ context.Context, organizationID, connectionID uuid.UUID) error {
	connection := repository.items[connectionID]
	if connection == nil || connection.OrganizationID != organizationID {
		return integrations.ErrConnectionNotFound
	}
	delete(repository.items, connectionID)
	return nil
}

type agentIntegrationGrantRepository struct {
	grants []integrations.IntegrationConnectionGrant
}

func (repository *agentIntegrationGrantRepository) ListApplicable(_ context.Context, organizationID, connectionID, accountID uuid.UUID, workspaceID *uuid.UUID) ([]integrations.IntegrationConnectionGrant, error) {
	items := make([]integrations.IntegrationConnectionGrant, 0)
	for _, grant := range repository.grants {
		if grant.OrganizationID != organizationID || grant.ConnectionID != connectionID {
			continue
		}
		switch grant.PrincipalType {
		case integrations.ConnectionGrantPrincipalOrganization:
			if grant.PrincipalID == nil {
				items = append(items, grant)
			}
		case integrations.ConnectionGrantPrincipalWorkspace:
			if workspaceID != nil && grant.PrincipalID != nil && *grant.PrincipalID == *workspaceID {
				items = append(items, grant)
			}
		case integrations.ConnectionGrantPrincipalAccount:
			if grant.PrincipalID != nil && *grant.PrincipalID == accountID {
				items = append(items, grant)
			}
		}
	}
	return items, nil
}

func (repository *agentIntegrationGrantRepository) List(ctx context.Context, organizationID, connectionID uuid.UUID) ([]integrations.IntegrationConnectionGrant, error) {
	return repository.ListApplicable(ctx, organizationID, connectionID, uuid.Nil, nil)
}

func (repository *agentIntegrationGrantRepository) Save(_ context.Context, grant *integrations.IntegrationConnectionGrant, _ int) error {
	repository.grants = append(repository.grants, *grant)
	return nil
}

func (*agentIntegrationGrantRepository) Delete(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

func agentIntegrationACLService(connections *agentIntegrationConnectionRepository, grants *agentIntegrationGrantRepository) *integrations.DefaultConnectionAccessService {
	return integrations.NewConnectionAccessService(connections, grants)
}

func TestListAgentIntegrationConnectionCandidatesEnforcesACLAndKeepsOnlyAuthorizedInvalidSelection(t *testing.T) {
	organizationID, workspaceID, accountID := uuid.New(), uuid.New(), uuid.New()
	otherAccountID := uuid.New()
	sharedWorkspaceID, sharedAccountID := uuid.New(), uuid.New()
	sharedDeniedID, platformID, personalID, otherPersonalID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	selectedUnhealthyID, expiredID := uuid.New(), uuid.New()
	now := time.Now().UTC()
	connections := &agentIntegrationConnectionRepository{items: map[uuid.UUID]*integrations.IntegrationConnection{}}
	add := func(id uuid.UUID, name string, source integrations.ConnectionCredentialSource, owner *uuid.UUID, health integrations.ConnectionHealthStatus, auth integrations.ConnectionAuthStatus) {
		connections.items[id] = &integrations.IntegrationConnection{
			ID: id, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: name,
			CredentialSource: source, OwnerAccountID: owner, Status: integrations.ConnectionStatusActive,
			HealthStatus: health, AuthStatus: auth, UpdatedAt: now,
		}
	}
	add(sharedWorkspaceID, "Workspace shared", integrations.ConnectionCredentialSourceOrganization, nil, integrations.ConnectionHealthHealthy, integrations.ConnectionAuthValid)
	add(sharedAccountID, "Account shared", integrations.ConnectionCredentialSourceOrganization, nil, integrations.ConnectionHealthHealthy, integrations.ConnectionAuthValid)
	add(sharedDeniedID, "Unshared secret", integrations.ConnectionCredentialSourceOrganization, nil, integrations.ConnectionHealthHealthy, integrations.ConnectionAuthValid)
	add(platformID, "Platform managed", integrations.ConnectionCredentialSourcePlatform, nil, integrations.ConnectionHealthHealthy, integrations.ConnectionAuthValid)
	add(personalID, "My personal", integrations.ConnectionCredentialSourceAccount, &accountID, integrations.ConnectionHealthHealthy, integrations.ConnectionAuthValid)
	add(otherPersonalID, "Other personal secret", integrations.ConnectionCredentialSourceAccount, &otherAccountID, integrations.ConnectionHealthHealthy, integrations.ConnectionAuthValid)
	add(selectedUnhealthyID, "Needs repair", integrations.ConnectionCredentialSourceOrganization, nil, integrations.ConnectionHealthUnhealthy, integrations.ConnectionAuthReconnectRequired)
	add(expiredID, "Expired shared", integrations.ConnectionCredentialSourceOrganization, nil, integrations.ConnectionHealthUnhealthy, integrations.ConnectionAuthExpired)
	grants := &agentIntegrationGrantRepository{grants: []integrations.IntegrationConnectionGrant{
		{OrganizationID: organizationID, ConnectionID: sharedWorkspaceID, PrincipalType: integrations.ConnectionGrantPrincipalWorkspace, PrincipalID: &workspaceID, AccessMode: integrations.ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"}},
		{OrganizationID: organizationID, ConnectionID: sharedAccountID, PrincipalType: integrations.ConnectionGrantPrincipalAccount, PrincipalID: &accountID, AccessMode: integrations.ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"}},
		{OrganizationID: organizationID, ConnectionID: platformID, PrincipalType: integrations.ConnectionGrantPrincipalOrganization, AccessMode: integrations.ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"}},
		{OrganizationID: organizationID, ConnectionID: otherPersonalID, PrincipalType: integrations.ConnectionGrantPrincipalOrganization, AccessMode: integrations.ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"}},
		{OrganizationID: organizationID, ConnectionID: selectedUnhealthyID, PrincipalType: integrations.ConnectionGrantPrincipalWorkspace, PrincipalID: &workspaceID, AccessMode: integrations.ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"}},
		{OrganizationID: organizationID, ConnectionID: expiredID, PrincipalType: integrations.ConnectionGrantPrincipalWorkspace, PrincipalID: &workspaceID, AccessMode: integrations.ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"}},
	}}
	agentID := uuid.New()
	cfg := &AgentsConfig{AgentsID: agentID}
	if _, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{IntegrationBindings: []dto.AgentIntegrationBinding{
		{ConnectionID: selectedUnhealthyID.String(), IntegrationID: "github", AccessMode: "read", AllowedActionIDs: []string{"github.issue.list"}},
		{ConnectionID: otherPersonalID.String(), IntegrationID: "other-personal", AccessMode: "read", AllowedActionIDs: []string{"github.issue.list"}},
	}}, accountID.String()); err != nil {
		t.Fatalf("apply config: %v", err)
	}
	access := agentIntegrationACLService(connections, grants)
	service := &agentsService{
		agentsRepo:                &stubWebAppStatusRepository{agent: &Agent{ID: agentID, TenantID: workspaceID, AgentsType: "AGENT"}, config: cfg},
		accountService:            &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService:         &stubWebAppStatusOrganizationService{allowed: true, organizationID: organizationID.String()},
		integrationActions:        stubIntegrationActionCatalog{"github/github.issue.list": {}},
		integrationActionPolicies: allowAgentIntegrationActionPolicies,
		integrationConnections:    connections,
		integrationAccess:         access,
	}

	response, err := service.ListAgentIntegrationConnectionCandidates(t.Context(), agentID.String(), accountID.String(), dto.AgentIntegrationConnectionCandidatesRequest{IntegrationID: "github", IncludeSelected: true, Limit: 20})
	if err != nil {
		t.Fatalf("ListAgentIntegrationConnectionCandidates() error = %v", err)
	}
	if response.Total != 2 || len(response.Data) != 2 {
		t.Fatalf("candidates = %#v, want two organization Agent items", response.Data)
	}
	byID := make(map[string]dto.AgentIntegrationConnectionCandidate, len(response.Data))
	for _, candidate := range response.Data {
		byID[candidate.ConnectionID] = candidate
	}
	for _, expected := range []uuid.UUID{sharedWorkspaceID, selectedUnhealthyID} {
		if _, ok := byID[expected.String()]; !ok {
			t.Fatalf("authorized candidate %s missing: %#v", expected, response.Data)
		}
	}
	for _, forbidden := range []uuid.UUID{sharedAccountID, sharedDeniedID, platformID, personalID, otherPersonalID, expiredID} {
		if _, ok := byID[forbidden.String()]; ok {
			t.Fatalf("unauthorized/unavailable candidate %s leaked: %#v", forbidden, response.Data)
		}
	}
	invalid := byID[selectedUnhealthyID.String()]
	if !invalid.Selected || invalid.Status != "invalid" || strings.Join(invalid.AllowedActionIDs, ",") != "github.issue.list" {
		t.Fatalf("selected invalid diagnostic = %#v", invalid)
	}
}

func TestListAgentIntegrationConnectionCandidatesNeverReturnsSelectedPersonalConnection(t *testing.T) {
	organizationID, workspaceID, accountID := uuid.New(), uuid.New(), uuid.New()
	connectionID, agentID := uuid.New(), uuid.New()
	connections := &agentIntegrationConnectionRepository{items: map[uuid.UUID]*integrations.IntegrationConnection{
		connectionID: {
			ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "My personal GitHub",
			CredentialSource: integrations.ConnectionCredentialSourceAccount, OwnerAccountID: &accountID,
			Status: integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthHealthy, AuthStatus: integrations.ConnectionAuthValid,
		},
	}}
	cfg := &AgentsConfig{AgentsID: agentID}
	if _, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{IntegrationBindings: []dto.AgentIntegrationBinding{{
		ConnectionID: connectionID.String(), IntegrationID: "github", AccessMode: "read", AllowedActionIDs: []string{"github.issue.list"},
	}}}, accountID.String()); err != nil {
		t.Fatalf("apply config: %v", err)
	}
	service := &agentsService{
		agentsRepo:                &stubWebAppStatusRepository{agent: &Agent{ID: agentID, TenantID: workspaceID, AgentsType: "AGENT"}, config: cfg},
		accountService:            &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService:         &stubWebAppStatusOrganizationService{allowed: true, organizationID: organizationID.String()},
		integrationActions:        stubIntegrationActionCatalog{"github/github.issue.list": {}},
		integrationActionPolicies: allowAgentIntegrationActionPolicies,
		integrationConnections:    connections,
		integrationAccess:         agentIntegrationACLService(connections, &agentIntegrationGrantRepository{}),
	}

	response, err := service.ListAgentIntegrationConnectionCandidates(t.Context(), agentID.String(), accountID.String(), dto.AgentIntegrationConnectionCandidatesRequest{
		IntegrationID: "github", IncludeSelected: true, Limit: 20,
	})
	if err != nil {
		t.Fatalf("ListAgentIntegrationConnectionCandidates() error = %v", err)
	}
	if response.Total != 0 || len(response.Data) != 0 {
		t.Fatalf("selected personal connection leaked into Agent candidates: %#v", response.Data)
	}
}

func TestListAgentIntegrationConnectionCandidatesReturnsOnlyGrantAndPolicyAllowedAgentActions(t *testing.T) {
	organizationID, workspaceID, accountID, connectionID, agentID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	connections := &agentIntegrationConnectionRepository{items: map[uuid.UUID]*integrations.IntegrationConnection{
		connectionID: {
			ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "Shared GitHub",
			AuthMethodID:     "tenant_app",
			CredentialSource: integrations.ConnectionCredentialSourceOrganization, Status: integrations.ConnectionStatusActive,
			HealthStatus: integrations.ConnectionHealthHealthy, AuthStatus: integrations.ConnectionAuthValid, UpdatedAt: time.Now().UTC(),
		},
	}}
	grants := &agentIntegrationGrantRepository{grants: []integrations.IntegrationConnectionGrant{{
		OrganizationID: organizationID, ConnectionID: connectionID,
		PrincipalType: integrations.ConnectionGrantPrincipalWorkspace, PrincipalID: &workspaceID,
		AccessMode:          integrations.ConnectionGrantAccessWrite,
		AllowedActionIDs:    []string{"github.issue.list", "github.issue.write", "github.aichat.only", "github.oauth.only"},
		ResourceConstraints: map[string]any{"resource_ids": []string{"repo-a"}},
	}}}
	catalog := explicitAgentIntegrationActionCatalog{integrationID: "github", actions: []integrations.ActionDefinition{
		{ID: "github.issue.list", Effect: toolgovernance.EffectRead, SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAgent}},
		{ID: "github.issue.write", Effect: toolgovernance.EffectUpdate, DataEgress: true, SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAgent}},
		{ID: "github.aichat.only", Effect: toolgovernance.EffectRead, SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat}},
		{
			ID: "github.oauth.only", Effect: toolgovernance.EffectRead,
			SupportedAuthMethodIDs: []string{"user_oauth"},
			SupportedCallers:       []tools.ToolInvokeFrom{tools.ToolInvokeFromAgent},
		},
	}}
	writePolicyAllowsEgress := false
	service := &agentsService{
		agentsRepo:             &stubWebAppStatusRepository{agent: &Agent{ID: agentID, TenantID: workspaceID, AgentsType: "AGENT"}, config: &AgentsConfig{AgentsID: agentID}},
		accountService:         &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService:      &stubWebAppStatusOrganizationService{allowed: true, organizationID: organizationID.String()},
		integrationActions:     catalog,
		integrationConnections: connections,
		integrationAccess:      agentIntegrationACLService(connections, grants),
	}
	service.integrationActionPolicies = agentIntegrationActionPolicyResolverFunc(func(
		_ context.Context,
		_, _ string,
		action integrations.ActionDefinition,
	) (integrations.ActionPolicyDecision, error) {
		return integrations.ActionPolicyDecision{
			Enabled:           true,
			DataEgressAllowed: action.ID != "github.issue.write" || writePolicyAllowsEgress,
		}, nil
	})

	list := func() dto.AgentIntegrationConnectionCandidate {
		t.Helper()
		response, err := service.ListAgentIntegrationConnectionCandidates(t.Context(), agentID.String(), accountID.String(), dto.AgentIntegrationConnectionCandidatesRequest{Limit: 20})
		if err != nil {
			t.Fatalf("ListAgentIntegrationConnectionCandidates() error = %v", err)
		}
		if len(response.Data) != 1 {
			t.Fatalf("candidates = %#v, want one", response.Data)
		}
		return response.Data[0]
	}

	candidate := list()
	if candidate.AuthMethodID != "tenant_app" {
		t.Fatalf("AuthMethodID = %q, want tenant_app", candidate.AuthMethodID)
	}
	if got := strings.Join(candidate.AvailableActionIDs, ","); got != "github.issue.list" {
		t.Fatalf("AvailableActionIDs with denied egress = %q, want github.issue.list", got)
	}
	if candidate.AvailableAccessMode != "read" {
		t.Fatalf("AvailableAccessMode = %q, want read", candidate.AvailableAccessMode)
	}

	writePolicyAllowsEgress = true
	candidate = list()
	if got := strings.Join(candidate.AvailableActionIDs, ","); got != "github.issue.list,github.issue.write" {
		t.Fatalf("AvailableActionIDs with write enabled = %q", got)
	}
	if candidate.AvailableAccessMode != "write" {
		t.Fatalf("AvailableAccessMode = %q, want write", candidate.AvailableAccessMode)
	}
}

func TestValidateIntegrationBindingGrantUsesRuntimeConnectionACL(t *testing.T) {
	organizationID, workspaceID, accountID := uuid.New(), uuid.New(), uuid.New()
	otherAccountID := uuid.New()
	sharedID, personalOwnerID, personalOtherID := uuid.New(), uuid.New(), uuid.New()
	connections := &agentIntegrationConnectionRepository{items: map[uuid.UUID]*integrations.IntegrationConnection{
		sharedID: {
			ID: sharedID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "Shared",
			CredentialSource: integrations.ConnectionCredentialSourceOrganization, Status: integrations.ConnectionStatusActive,
			HealthStatus: integrations.ConnectionHealthHealthy, AuthStatus: integrations.ConnectionAuthValid,
		},
		personalOwnerID: {
			ID: personalOwnerID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "Owner personal",
			CredentialSource: integrations.ConnectionCredentialSourceAccount, OwnerAccountID: &accountID, Status: integrations.ConnectionStatusActive,
			HealthStatus: integrations.ConnectionHealthHealthy, AuthStatus: integrations.ConnectionAuthValid,
		},
		personalOtherID: {
			ID: personalOtherID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "Other personal",
			CredentialSource: integrations.ConnectionCredentialSourceAccount, OwnerAccountID: &otherAccountID, Status: integrations.ConnectionStatusActive,
			HealthStatus: integrations.ConnectionHealthHealthy, AuthStatus: integrations.ConnectionAuthValid,
		},
	}}
	grants := &agentIntegrationGrantRepository{grants: []integrations.IntegrationConnectionGrant{
		{OrganizationID: organizationID, ConnectionID: sharedID, PrincipalType: integrations.ConnectionGrantPrincipalWorkspace, PrincipalID: &workspaceID, AccessMode: integrations.ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"}, ResourceConstraints: map[string]any{}},
		// Even an accidentally-created organization grant must never share a
		// personal credential owned by another account.
		{OrganizationID: organizationID, ConnectionID: personalOtherID, PrincipalType: integrations.ConnectionGrantPrincipalOrganization, AccessMode: integrations.ConnectionGrantAccessWrite, AllowedActionIDs: []string{"*"}, ResourceConstraints: map[string]any{}},
	}}
	service := &agentsService{
		integrationActions:     stubIntegrationActionCatalog{"github/github.issue.list": {}},
		integrationConnections: connections,
	}
	service.integrationAccess = agentIntegrationACLService(connections, grants)
	binding := dto.AgentIntegrationBinding{ConnectionID: sharedID.String(), IntegrationID: "github", AccessMode: "read", AllowedActionIDs: []string{"github.issue.list"}}
	if err := service.validateIntegrationBindingGrant(t.Context(), organizationID.String(), workspaceID.String(), accountID.String(), binding); err != nil {
		t.Fatalf("authorized binding error = %v", err)
	}
	connections.items[sharedID].AuthMethodID = "tenant_app"
	service.integrationActions = explicitAgentIntegrationActionCatalog{
		integrationID: "github",
		actions: []integrations.ActionDefinition{{
			ID: "github.issue.list", Effect: toolgovernance.EffectRead,
			SupportedAuthMethodIDs: []string{"user_oauth"},
		}},
	}
	if err := service.validateIntegrationBindingGrant(t.Context(), organizationID.String(), workspaceID.String(), accountID.String(), binding); err == nil {
		t.Fatal("authentication-incompatible action was accepted as an Agent binding")
	}
	service.integrationActions = stubIntegrationActionCatalog{"github/github.issue.list": {}}
	binding.ConnectionID = personalOtherID.String()
	if err := service.validateIntegrationBindingGrant(t.Context(), organizationID.String(), workspaceID.String(), accountID.String(), binding); err == nil {
		t.Fatal("another account's personal connection bypassed binding ACL")
	}
	binding.ConnectionID = personalOwnerID.String()
	if err := service.validateIntegrationBindingGrant(t.Context(), organizationID.String(), workspaceID.String(), accountID.String(), binding); err == nil {
		t.Fatal("connection owner's personal credential was accepted as an Agent binding")
	}
}

func TestValidateIncrementalAgentBindingChangesRechecksUnchangedCredentialSource(t *testing.T) {
	organizationID, workspaceID, accountID := uuid.New(), uuid.New(), uuid.New()
	organizationConnectionID, platformConnectionID, personalConnectionID := uuid.New(), uuid.New(), uuid.New()
	connections := &agentIntegrationConnectionRepository{items: map[uuid.UUID]*integrations.IntegrationConnection{
		organizationConnectionID: {
			ID: organizationConnectionID, OrganizationID: organizationID, IntegrationID: "github",
			CredentialSource: integrations.ConnectionCredentialSourceOrganization, Status: integrations.ConnectionStatusActive,
		},
		platformConnectionID: {
			ID: platformConnectionID, OrganizationID: organizationID, IntegrationID: "github",
			CredentialSource: integrations.ConnectionCredentialSourcePlatform, Status: integrations.ConnectionStatusActive,
		},
		personalConnectionID: {
			ID: personalConnectionID, OrganizationID: organizationID, IntegrationID: "github",
			CredentialSource: integrations.ConnectionCredentialSourceAccount, OwnerAccountID: &accountID, Status: integrations.ConnectionStatusActive,
		},
	}}
	grants := &agentIntegrationGrantRepository{grants: []integrations.IntegrationConnectionGrant{
		{
			OrganizationID: organizationID, ConnectionID: organizationConnectionID,
			PrincipalType: integrations.ConnectionGrantPrincipalWorkspace, PrincipalID: &workspaceID,
			AccessMode: integrations.ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"}, ResourceConstraints: map[string]any{},
		},
		{
			OrganizationID: organizationID, ConnectionID: platformConnectionID,
			PrincipalType: integrations.ConnectionGrantPrincipalOrganization,
			AccessMode:    integrations.ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"}, ResourceConstraints: map[string]any{},
		},
	}}
	service := &agentsService{
		enterpriseService:      &stubWebAppStatusOrganizationService{allowed: true, organizationID: organizationID.String()},
		integrationActions:     stubIntegrationActionCatalog{"github/github.issue.list": {}},
		integrationConnections: connections,
		integrationAccess:      agentIntegrationACLService(connections, grants),
	}
	agent := &Agent{TenantID: workspaceID}
	tests := []struct {
		name       string
		connection uuid.UUID
		wantError  bool
	}{
		{name: "organization", connection: organizationConnectionID},
		{name: "legacy platform", connection: platformConnectionID, wantError: true},
		{name: "personal account", connection: personalConnectionID, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binding := dto.AgentIntegrationBinding{
				ConnectionID: tt.connection.String(), IntegrationID: "github", AccessMode: "read", AllowedActionIDs: []string{"github.issue.list"},
			}
			err := service.validateIncrementalAgentBindingChanges(t.Context(), agent, accountID.String(),
				&dto.AgentConfigResponse{IntegrationBindings: []dto.AgentIntegrationBinding{binding}},
				dto.AgentConfigRequest{IntegrationBindings: []dto.AgentIntegrationBinding{binding}},
			)
			if (err != nil) != tt.wantError {
				t.Fatalf("validateIncrementalAgentBindingChanges() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}
