package integrations

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type memoryConnectionGrantRepository struct {
	grants []IntegrationConnectionGrant
}

func (repository *memoryConnectionGrantRepository) ListApplicable(_ context.Context, organizationID, connectionID, accountID uuid.UUID, workspaceID *uuid.UUID) ([]IntegrationConnectionGrant, error) {
	result := make([]IntegrationConnectionGrant, 0)
	for _, grant := range repository.grants {
		if grant.OrganizationID != organizationID || grant.ConnectionID != connectionID {
			continue
		}
		switch grant.PrincipalType {
		case ConnectionGrantPrincipalOrganization:
			if grant.PrincipalID == nil {
				result = append(result, grant)
			}
		case ConnectionGrantPrincipalAccount:
			if grant.PrincipalID != nil && *grant.PrincipalID == accountID {
				result = append(result, grant)
			}
		case ConnectionGrantPrincipalWorkspace:
			if workspaceID != nil && grant.PrincipalID != nil && *grant.PrincipalID == *workspaceID {
				result = append(result, grant)
			}
		}
	}
	return result, nil
}

func (repository *memoryConnectionGrantRepository) List(ctx context.Context, organizationID, connectionID uuid.UUID) ([]IntegrationConnectionGrant, error) {
	return repository.ListApplicable(ctx, organizationID, connectionID, uuid.Nil, nil)
}

func (repository *memoryConnectionGrantRepository) Save(_ context.Context, grant *IntegrationConnectionGrant, _ int) error {
	repository.grants = append(repository.grants, *grant)
	return nil
}

func (repository *memoryConnectionGrantRepository) Delete(_ context.Context, organizationID, connectionID, grantID uuid.UUID) error {
	for index, grant := range repository.grants {
		if grant.OrganizationID == organizationID && grant.ConnectionID == connectionID && grant.ID == grantID {
			repository.grants = append(repository.grants[:index], repository.grants[index+1:]...)
			return nil
		}
	}
	return ErrConnectionNotFound
}

func TestConnectionAccessEnforcesPrincipalActionEffectAndResources(t *testing.T) {
	connections := newMemoryConnectionRepository()
	organizationID := uuid.New()
	connectionID := uuid.New()
	if err := connections.Create(context.Background(), &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "Shared",
		CredentialSource: ConnectionCredentialSourceOrganization, AuthType: ConnectionAuthTypeAPIKey,
		Status: ConnectionStatusActive, CredentialVersion: 1, Revision: 1, HealthRevision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	workspaceID := uuid.New()
	accountID := uuid.New()
	grants := &memoryConnectionGrantRepository{grants: []IntegrationConnectionGrant{
		{
			ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID,
			PrincipalType: ConnectionGrantPrincipalWorkspace, PrincipalID: &workspaceID,
			AccessMode: ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"},
			ResourceConstraints: map[string]any{"resource_ids": []string{"repo-a", "repo-b"}},
		},
	}}
	service := NewConnectionAccessService(connections, grants)
	request := ConnectionAccessRequest{
		OrganizationID: organizationID, WorkspaceID: &workspaceID, AccountID: accountID,
		ConnectionID: connectionID, IntegrationID: "github", ActionID: "github.issue.list",
		Effect: toolgovernance.EffectRead, ResourceIDs: []string{"repo-a"}, ResourcesRequired: true,
	}
	if err := service.AuthorizeConnectionUse(context.Background(), request); err != nil {
		t.Fatalf("authorized request error = %v", err)
	}
	request.ResourceIDs = nil
	request.ResourcesRequired = false
	if ErrorCode(service.AuthorizeConnectionUse(context.Background(), request)) != ErrorCodeAccessDenied {
		t.Fatal("resource-constrained grant became unrestricted when resource extraction was absent")
	}
	request.ResourceIDs = []string{"repo-a"}
	request.ResourcesRequired = true
	request.ActionID = "github.issue.create"
	if ErrorCode(service.AuthorizeConnectionUse(context.Background(), request)) != ErrorCodeAccessDenied {
		t.Fatal("grant authorized an unlisted action")
	}
	request.ActionID = "github.issue.list"
	request.Effect = toolgovernance.EffectCreate
	if ErrorCode(service.AuthorizeConnectionUse(context.Background(), request)) != ErrorCodeAccessDenied {
		t.Fatal("read grant authorized a write action")
	}
	request.Effect = toolgovernance.EffectRead
	request.ResourceIDs = []string{"repo-private"}
	if ErrorCode(service.AuthorizeConnectionUse(context.Background(), request)) != ErrorCodeAccessDenied {
		t.Fatal("resource-constrained grant authorized an unlisted resource")
	}
	request.ResourceIDs = []string{"repo-a"}
	otherWorkspace := uuid.New()
	request.WorkspaceID = &otherWorkspace
	if ErrorCode(service.AuthorizeConnectionUse(context.Background(), request)) != ErrorCodeAccessDenied {
		t.Fatal("workspace grant authorized another workspace")
	}
}

func TestAccountOwnedConnectionIsPrivateToOwner(t *testing.T) {
	connections := newMemoryConnectionRepository()
	organizationID := uuid.New()
	ownerID := uuid.New()
	connectionID := uuid.New()
	if err := connections.Create(context.Background(), &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "Private",
		CredentialSource: ConnectionCredentialSourceAccount, OwnerAccountID: &ownerID,
		AuthType: ConnectionAuthTypeOAuth2, Status: ConnectionStatusActive, CredentialVersion: 1, Revision: 1, HealthRevision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	otherAccountID := uuid.New()
	service := NewConnectionAccessService(connections, &memoryConnectionGrantRepository{grants: []IntegrationConnectionGrant{{
		OrganizationID: organizationID, ConnectionID: connectionID,
		PrincipalType: ConnectionGrantPrincipalAccount, PrincipalID: &otherAccountID,
		AccessMode: ConnectionGrantAccessWrite, AllowedActionIDs: []string{"github.issue.list"}, ResourceConstraints: map[string]any{},
	}}})
	request := ConnectionAccessRequest{
		OrganizationID: organizationID, AccountID: ownerID, ConnectionID: connectionID,
		IntegrationID: "github", ActionID: "github.issue.list", Effect: toolgovernance.EffectRead,
	}
	if err := service.AuthorizeConnectionUse(context.Background(), request); err != nil {
		t.Fatalf("owner authorization = %v", err)
	}
	request.AccountID = otherAccountID
	if ErrorCode(service.AuthorizeConnectionUse(context.Background(), request)) != ErrorCodeAccessDenied {
		t.Fatal("private connection was exposed to another account through a grant")
	}
	if ErrorCode(service.AuthorizeConnectionPreference(context.Background(), organizationID, otherAccountID, nil, connectionID)) != ErrorCodeAccessDenied {
		t.Fatal("private connection metadata was exposed to another account through a grant")
	}
}

func TestAgentConnectionAccessUsesOnlyOrganizationOrWorkspaceGrants(t *testing.T) {
	connections := newMemoryConnectionRepository()
	organizationID, workspaceID, accountID := uuid.New(), uuid.New(), uuid.New()
	connectionID := uuid.New()
	if err := connections.Create(context.Background(), &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "Shared",
		CredentialSource: ConnectionCredentialSourceOrganization, AuthType: ConnectionAuthTypeAPIKey,
		Status: ConnectionStatusActive, CredentialVersion: 1, Revision: 1, HealthRevision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	accountGrant := IntegrationConnectionGrant{
		ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID,
		PrincipalType: ConnectionGrantPrincipalAccount, PrincipalID: &accountID,
		AccessMode: ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"}, ResourceConstraints: map[string]any{},
	}
	grants := &memoryConnectionGrantRepository{grants: []IntegrationConnectionGrant{accountGrant}}
	service := NewConnectionAccessService(connections, grants)
	request := ConnectionAccessRequest{
		OrganizationID: organizationID, WorkspaceID: &workspaceID, AccountID: accountID,
		ConnectionID: connectionID, IntegrationID: "github", ActionID: "github.issue.list", Effect: toolgovernance.EffectRead,
	}
	if err := service.AuthorizeConnectionUse(context.Background(), request); err != nil {
		t.Fatalf("ordinary account grant should remain valid for AIChat: %v", err)
	}
	if ErrorCode(service.AuthorizeAgentConnectionUse(context.Background(), request)) != ErrorCodeAccessDenied {
		t.Fatal("account-only grant authorized an Agent invocation")
	}
	workspaceGrant := accountGrant
	workspaceGrant.ID = uuid.New()
	workspaceGrant.PrincipalType = ConnectionGrantPrincipalWorkspace
	workspaceGrant.PrincipalID = &workspaceID
	grants.grants = append(grants.grants, workspaceGrant)
	if err := service.AuthorizeAgentConnectionUse(context.Background(), request); err != nil {
		t.Fatalf("workspace grant did not authorize Agent invocation: %v", err)
	}
	grants.grants = []IntegrationConnectionGrant{{
		ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID,
		PrincipalType: ConnectionGrantPrincipalOrganization,
		AccessMode:    ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"}, ResourceConstraints: map[string]any{},
	}}
	if err := service.AuthorizeAgentConnectionUse(context.Background(), request); err != nil {
		t.Fatalf("organization grant did not authorize Agent invocation: %v", err)
	}
}

func TestAgentConnectionAccessRejectsPersonalCredentialEvenForOwner(t *testing.T) {
	connections := newMemoryConnectionRepository()
	organizationID, workspaceID, ownerID, connectionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := connections.Create(context.Background(), &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "Personal",
		CredentialSource: ConnectionCredentialSourceAccount, OwnerAccountID: &ownerID, AuthType: ConnectionAuthTypeAPIKey,
		Status: ConnectionStatusActive, CredentialVersion: 1, Revision: 1, HealthRevision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	grants := &memoryConnectionGrantRepository{grants: []IntegrationConnectionGrant{{
		ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID,
		PrincipalType: ConnectionGrantPrincipalOrganization,
		AccessMode:    ConnectionGrantAccessWrite, AllowedActionIDs: []string{"github.issue.list"}, ResourceConstraints: map[string]any{},
	}}}
	service := NewConnectionAccessService(connections, grants)
	if ErrorCode(service.AuthorizeAgentConnectionUse(context.Background(), ConnectionAccessRequest{
		OrganizationID: organizationID, WorkspaceID: &workspaceID, AccountID: ownerID,
		ConnectionID: connectionID, IntegrationID: "github", ActionID: "github.issue.list", Effect: toolgovernance.EffectRead,
	})) != ErrorCodeAccessDenied {
		t.Fatal("personal credential authorized Agent execution for its owner")
	}
}

func TestValidateConnectionGrantRejectsFutureActionWildcard(t *testing.T) {
	grant := &IntegrationConnectionGrant{
		OrganizationID: uuid.New(), ConnectionID: uuid.New(),
		PrincipalType:    ConnectionGrantPrincipalOrganization,
		AccessMode:       ConnectionGrantAccessWrite,
		AllowedActionIDs: []string{"*"}, ResourceConstraints: map[string]any{},
	}
	if ErrorCode(validateConnectionGrant(grant)) != ErrorCodeInvalidInput {
		t.Fatal("connection grant wildcard must not authorize actions added in the future")
	}
	if containsAccessAction([]string{"*"}, "github.issue.list") {
		t.Fatal("legacy wildcard must fail closed at the execution boundary")
	}
}

func TestConnectionAccessRejectsUnhealthyConnectionForSelectionAndExecution(t *testing.T) {
	connections := newMemoryConnectionRepository()
	organizationID, accountID, connectionID := uuid.New(), uuid.New(), uuid.New()
	if err := connections.Create(context.Background(), &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "Broken",
		CredentialSource: ConnectionCredentialSourceOrganization, AuthType: ConnectionAuthTypeAPIKey,
		Status: ConnectionStatusActive, HealthStatus: ConnectionHealthUnhealthy, AuthStatus: ConnectionAuthReconnectRequired,
		CredentialVersion: 1, Revision: 1, HealthRevision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	grants := &memoryConnectionGrantRepository{grants: []IntegrationConnectionGrant{{
		OrganizationID: organizationID, ConnectionID: connectionID, PrincipalType: ConnectionGrantPrincipalOrganization,
		AccessMode: ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"}, ResourceConstraints: map[string]any{},
	}}}
	service := NewConnectionAccessService(connections, grants)
	if err := service.AuthorizeConnectionVisibility(context.Background(), organizationID, accountID, nil, connectionID); err != nil {
		t.Fatalf("authorized unhealthy connection should remain visible for diagnostics: %v", err)
	}
	if ErrorCode(service.AuthorizeConnectionPreference(context.Background(), organizationID, accountID, nil, connectionID)) != ErrorCodeAccessDenied {
		t.Fatal("unhealthy connection was selectable")
	}
	if ErrorCode(service.AuthorizeConnectionUse(context.Background(), ConnectionAccessRequest{
		OrganizationID: organizationID, AccountID: accountID, ConnectionID: connectionID,
		IntegrationID: "github", ActionID: "github.issue.list", Effect: toolgovernance.EffectRead,
	})) != ErrorCodeAccessDenied {
		t.Fatal("unhealthy connection was executable")
	}
}

func TestConnectionGrantRepositoryRefusesToEraseExistingResourceConstraints(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	organizationID, connectionID, grantID := uuid.New(), uuid.New(), uuid.New()
	mock.ExpectQuery(`SELECT "resource_constraints","revision" FROM "integration_connection_grants"`).
		WithArgs(organizationID, connectionID, grantID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"resource_constraints", "revision"}).AddRow(`{"resource_ids":["repo-private"]}`, 2))
	repository := NewGormConnectionGrantRepository(db)
	err = repository.Save(t.Context(), &IntegrationConnectionGrant{
		ID: grantID, OrganizationID: organizationID, ConnectionID: connectionID,
		PrincipalType: ConnectionGrantPrincipalOrganization,
		AccessMode:    ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"},
		ResourceConstraints: map[string]any{}, Revision: 2,
	}, 2)
	if ErrorCode(err) != ErrorCodeInvalidInput {
		t.Fatalf("Save() error = %v, want %s", err, ErrorCodeInvalidInput)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestAuthorizeConnectionScopesRequiresAllAndAtLeastOneAlternative(t *testing.T) {
	err := AuthorizeConnectionScopes([]string{"repo:read", "issues:write"}, ConnectionScopeRequirement{
		AllOf: []string{"repo:read"}, AnyOf: []string{"admin", "issues:write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ErrorCode(AuthorizeConnectionScopes([]string{"repo:read"}, ConnectionScopeRequirement{
		AllOf: []string{"repo:read", "issues:write"},
	})) != ErrorCodeInsufficientScope {
		t.Fatal("missing required scope was authorized")
	}
}
