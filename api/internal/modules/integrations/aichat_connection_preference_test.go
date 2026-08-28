package integrations

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

type memoryAIChatPreferenceRepository struct {
	preferences   []AIChatIntegrationPreference
	repairCalls   int
	repairApplied bool
	beforeRepair  func()
}

func (repository *memoryAIChatPreferenceRepository) List(_ context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID) ([]AIChatIntegrationPreference, error) {
	result := make([]AIChatIntegrationPreference, 0)
	for _, preference := range repository.preferences {
		if preference.OrganizationID == organizationID && preference.AccountID == accountID && sameOptionalUUID(preference.WorkspaceID, workspaceID) {
			result = append(result, preference)
		}
	}
	return result, nil
}

func (repository *memoryAIChatPreferenceRepository) Replace(_ context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID, preferences []AIChatIntegrationPreference) error {
	retained := make([]AIChatIntegrationPreference, 0, len(repository.preferences)+len(preferences))
	for _, preference := range repository.preferences {
		if preference.OrganizationID != organizationID || preference.AccountID != accountID || !sameOptionalUUID(preference.WorkspaceID, workspaceID) {
			retained = append(retained, preference)
		}
	}
	for _, preference := range preferences {
		preference.OrganizationID = organizationID
		preference.AccountID = accountID
		preference.WorkspaceID = cloneUUIDPointer(workspaceID)
		retained = append(retained, preference)
	}
	repository.preferences = retained
	return nil
}

func (repository *memoryAIChatPreferenceRepository) RepairIfUnchanged(_ context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID, observed, repaired []AIChatIntegrationPreference) (bool, error) {
	repository.repairCalls++
	if repository.beforeRepair != nil {
		repository.beforeRepair()
		repository.beforeRepair = nil
	}
	current, _ := repository.List(context.Background(), organizationID, accountID, workspaceID)
	if !sameAIChatIntegrationPreferenceSnapshot(current, observed) {
		return false, nil
	}
	if err := repository.Replace(context.Background(), organizationID, accountID, workspaceID, repaired); err != nil {
		return false, err
	}
	repository.repairApplied = true
	return true, nil
}

func TestAIChatPreferenceRepairDoesNotOverwriteConcurrentReplacement(t *testing.T) {
	connections := newMemoryConnectionRepository()
	organizationID := uuid.New()
	accountID := uuid.New()
	staleID := uuid.New()
	firstID := uuid.New()
	newID := uuid.New()
	for _, connectionID := range []uuid.UUID{firstID, newID} {
		if err := connections.Create(context.Background(), &IntegrationConnection{
			ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: connectionID.String(),
			CredentialSource: ConnectionCredentialSourceAccount, OwnerAccountID: &accountID, AuthType: ConnectionAuthTypeAPIKey,
			Status: ConnectionStatusActive, CredentialVersion: 1, Revision: 1, HealthRevision: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	repository := &memoryAIChatPreferenceRepository{preferences: []AIChatIntegrationPreference{{
		ID: uuid.New(), OrganizationID: organizationID, AccountID: accountID, IntegrationID: "github",
		SelectedConnectionIDs: []string{staleID.String(), firstID.String()}, PreferredConnectionID: &staleID, Revision: 1,
	}}}
	repository.beforeRepair = func() {
		repository.preferences = []AIChatIntegrationPreference{{
			ID: uuid.New(), OrganizationID: organizationID, AccountID: accountID, IntegrationID: "github",
			SelectedConnectionIDs: []string{newID.String()}, PreferredConnectionID: &newID, Revision: 1,
		}}
	}
	service := NewAIChatIntegrationPreferenceService(repository, connections, NewConnectionAccessService(connections, &memoryConnectionGrantRepository{}))

	firstSnapshot, err := service.List(context.Background(), organizationID, accountID, nil)
	if err != nil || len(firstSnapshot) != 1 || firstSnapshot[0].PreferredConnectionID == nil || *firstSnapshot[0].PreferredConnectionID != firstID {
		t.Fatalf("first sanitized snapshot = %#v, %v", firstSnapshot, err)
	}
	if repository.repairApplied || len(repository.preferences) != 1 || repository.preferences[0].PreferredConnectionID == nil || *repository.preferences[0].PreferredConnectionID != newID {
		t.Fatalf("repair overwrote concurrent preference: %#v applied=%v", repository.preferences, repository.repairApplied)
	}
	secondSnapshot, err := service.List(context.Background(), organizationID, accountID, nil)
	if err != nil || len(secondSnapshot) != 1 || secondSnapshot[0].PreferredConnectionID == nil || *secondSnapshot[0].PreferredConnectionID != newID {
		t.Fatalf("second current snapshot = %#v, %v", secondSnapshot, err)
	}
}

func sameOptionalUUID(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func TestAIChatPreferenceListRemovesRevokedGrantFromRuntimeAndStorage(t *testing.T) {
	connections := newMemoryConnectionRepository()
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	connectionID := uuid.New()
	if err := connections.Create(context.Background(), &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "Shared",
		CredentialSource: ConnectionCredentialSourceOrganization, AuthType: ConnectionAuthTypeAPIKey,
		Status: ConnectionStatusActive, CredentialVersion: 1, Revision: 1, HealthRevision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	grants := &memoryConnectionGrantRepository{grants: []IntegrationConnectionGrant{{
		ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID,
		PrincipalType: ConnectionGrantPrincipalAccount, PrincipalID: &accountID,
		AccessMode: ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"}, ResourceConstraints: map[string]any{},
	}}}
	access := NewConnectionAccessService(connections, grants)
	repository := &memoryAIChatPreferenceRepository{}
	service := NewAIChatIntegrationPreferenceService(repository, connections, access)
	preferences, err := service.Replace(context.Background(), organizationID, accountID, &workspaceID, []AIChatIntegrationPreferenceInput{{
		IntegrationID: "github", SelectedConnectionIDs: []uuid.UUID{connectionID}, PreferredConnectionID: &connectionID,
	}})
	if err != nil || len(preferences) != 1 || preferences[0].PreferredConnectionID == nil || *preferences[0].PreferredConnectionID != connectionID {
		t.Fatalf("Replace() = %#v, %v", preferences, err)
	}

	// Revoking the grant must remove the stale routing hint before the next
	// runtime snapshot as well as continuing to fail the execution-time check.
	grants.grants = nil
	err = access.AuthorizeConnectionUse(context.Background(), ConnectionAccessRequest{
		OrganizationID: organizationID, WorkspaceID: &workspaceID, AccountID: accountID,
		ConnectionID: connectionID, IntegrationID: "github", ActionID: "github.issue.list", Effect: toolgovernance.EffectRead,
	})
	if ErrorCode(err) != ErrorCodeAccessDenied {
		t.Fatalf("revoked runtime grant error = %v", err)
	}
	stored, err := service.List(context.Background(), organizationID, accountID, &workspaceID)
	if err != nil || len(stored) != 0 {
		t.Fatalf("revoked preference snapshot = %#v, %v, want empty", stored, err)
	}
	if len(repository.preferences) != 0 || repository.repairCalls != 1 || !repository.repairApplied {
		t.Fatalf("revoked preference was not persistently repaired: %#v calls=%d applied=%v", repository.preferences, repository.repairCalls, repository.repairApplied)
	}
}

func TestAIChatPreferenceListDropsDeletedAndDisabledSelectionsAndFallsBackPreferred(t *testing.T) {
	connections := newMemoryConnectionRepository()
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	deletedID := uuid.New()
	disabledID := uuid.New()
	availableID := uuid.New()
	for _, connection := range []*IntegrationConnection{
		{
			ID: disabledID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "Disabled",
			CredentialSource: ConnectionCredentialSourceOrganization, AuthType: ConnectionAuthTypeAPIKey,
			Status: ConnectionStatusDisabled, CredentialVersion: 1, Revision: 1, HealthRevision: 1,
		},
		{
			ID: availableID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "Available",
			CredentialSource: ConnectionCredentialSourceOrganization, AuthType: ConnectionAuthTypeAPIKey,
			Status: ConnectionStatusActive, CredentialVersion: 1, Revision: 1, HealthRevision: 1,
		},
	} {
		if err := connections.Create(context.Background(), connection); err != nil {
			t.Fatal(err)
		}
	}
	grants := &memoryConnectionGrantRepository{grants: []IntegrationConnectionGrant{
		{ID: uuid.New(), OrganizationID: organizationID, ConnectionID: disabledID, PrincipalType: ConnectionGrantPrincipalAccount, PrincipalID: &accountID, AccessMode: ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"}, ResourceConstraints: map[string]any{}},
		{ID: uuid.New(), OrganizationID: organizationID, ConnectionID: availableID, PrincipalType: ConnectionGrantPrincipalAccount, PrincipalID: &accountID, AccessMode: ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"}, ResourceConstraints: map[string]any{}},
	}}
	preferenceID := uuid.New()
	repository := &memoryAIChatPreferenceRepository{preferences: []AIChatIntegrationPreference{{
		ID: preferenceID, OrganizationID: organizationID, AccountID: accountID, WorkspaceID: &workspaceID,
		IntegrationID: "github", SelectedConnectionIDs: []string{deletedID.String(), disabledID.String(), availableID.String()},
		PreferredConnectionID: &deletedID, Revision: 3,
	}}}
	service := NewAIChatIntegrationPreferenceService(repository, connections, NewConnectionAccessService(connections, grants))

	items, err := service.List(context.Background(), organizationID, accountID, &workspaceID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 || len(items[0].SelectedConnectionIDs) != 1 || items[0].SelectedConnectionIDs[0] != availableID.String() {
		t.Fatalf("sanitized selections = %#v", items)
	}
	if items[0].PreferredConnectionID == nil || *items[0].PreferredConnectionID != availableID {
		t.Fatalf("preferred fallback = %v, want %s", items[0].PreferredConnectionID, availableID)
	}
	if len(repository.preferences) != 1 || repository.preferences[0].PreferredConnectionID == nil || *repository.preferences[0].PreferredConnectionID != availableID || len(repository.preferences[0].SelectedConnectionIDs) != 1 {
		t.Fatalf("persisted repaired preference = %#v", repository.preferences)
	}
}

func TestAIChatPreferenceListDoesNotExposeAnotherAccountsPersonalConnection(t *testing.T) {
	connections := newMemoryConnectionRepository()
	organizationID := uuid.New()
	accountID := uuid.New()
	otherAccountID := uuid.New()
	connectionID := uuid.New()
	if err := connections.Create(context.Background(), &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "private metadata",
		CredentialSource: ConnectionCredentialSourceAccount, OwnerAccountID: &otherAccountID, AuthType: ConnectionAuthTypeAPIKey,
		Status: ConnectionStatusActive, CredentialVersion: 1, Revision: 1, HealthRevision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	repository := &memoryAIChatPreferenceRepository{preferences: []AIChatIntegrationPreference{{
		ID: uuid.New(), OrganizationID: organizationID, AccountID: accountID, IntegrationID: "github",
		SelectedConnectionIDs: []string{connectionID.String()}, PreferredConnectionID: &connectionID, Revision: 1,
	}}}
	service := NewAIChatIntegrationPreferenceService(repository, connections, NewConnectionAccessService(connections, &memoryConnectionGrantRepository{}))

	items, err := service.List(context.Background(), organizationID, accountID, nil)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 0 || len(repository.preferences) != 0 {
		t.Fatalf("inaccessible personal connection leaked through preferences: returned=%#v stored=%#v", items, repository.preferences)
	}
}
