package preferences

import (
	"context"
	"testing"

	"github.com/google/uuid"
	integrations "github.com/zgiai/zgi/api/internal/modules/integrations/core"
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
		preference.WorkspaceID = clonePreferenceUUID(workspaceID)
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

type preferenceConnectionLookup struct {
	connections map[uuid.UUID]*integrations.IntegrationConnection
}

func (lookup *preferenceConnectionLookup) GetByID(_ context.Context, organizationID, connectionID uuid.UUID) (*integrations.IntegrationConnection, error) {
	connection := lookup.connections[connectionID]
	if connection == nil || connection.OrganizationID != organizationID {
		return nil, integrations.ErrConnectionNotFound
	}
	return connection, nil
}

type preferenceAccessChecker struct {
	allowed map[uuid.UUID]bool
}

func (checker *preferenceAccessChecker) AuthorizeConnectionPreference(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, connectionID uuid.UUID) error {
	if checker != nil && checker.allowed[connectionID] {
		return nil
	}
	return integrations.NewError(integrations.ErrorCodeAccessDenied, "integration connection is not available", nil)
}

func TestAIChatPreferenceRepairDoesNotOverwriteConcurrentReplacement(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	staleID := uuid.New()
	firstID := uuid.New()
	newID := uuid.New()
	connections := &preferenceConnectionLookup{connections: map[uuid.UUID]*integrations.IntegrationConnection{}}
	for _, connectionID := range []uuid.UUID{firstID, newID} {
		connections.connections[connectionID] = availablePreferenceConnection(organizationID, accountID, connectionID)
	}
	access := &preferenceAccessChecker{allowed: map[uuid.UUID]bool{firstID: true, newID: true}}
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
	service := NewAIChatIntegrationPreferenceService(repository, connections, access)

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

func TestAIChatPreferenceListRemovesRevokedSelectionFromRuntimeAndStorage(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	connectionID := uuid.New()
	connections := &preferenceConnectionLookup{connections: map[uuid.UUID]*integrations.IntegrationConnection{
		connectionID: availablePreferenceConnection(organizationID, accountID, connectionID),
	}}
	access := &preferenceAccessChecker{allowed: map[uuid.UUID]bool{connectionID: true}}
	repository := &memoryAIChatPreferenceRepository{}
	service := NewAIChatIntegrationPreferenceService(repository, connections, access)
	preferences, err := service.Replace(context.Background(), organizationID, accountID, &workspaceID, []AIChatIntegrationPreferenceInput{{
		IntegrationID: "github", SelectedConnectionIDs: []uuid.UUID{connectionID}, PreferredConnectionID: &connectionID,
	}})
	if err != nil || len(preferences) != 1 || preferences[0].PreferredConnectionID == nil || *preferences[0].PreferredConnectionID != connectionID {
		t.Fatalf("Replace() = %#v, %v", preferences, err)
	}

	delete(access.allowed, connectionID)
	stored, err := service.List(context.Background(), organizationID, accountID, &workspaceID)
	if err != nil || len(stored) != 0 {
		t.Fatalf("revoked preference snapshot = %#v, %v, want empty", stored, err)
	}
	if len(repository.preferences) != 0 || repository.repairCalls != 1 || !repository.repairApplied {
		t.Fatalf("revoked preference was not persistently repaired: %#v calls=%d applied=%v", repository.preferences, repository.repairCalls, repository.repairApplied)
	}
}

func TestAIChatPreferenceListDropsDeletedAndDisabledSelectionsAndFallsBackPreferred(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	deletedID := uuid.New()
	disabledID := uuid.New()
	availableID := uuid.New()
	disabled := availablePreferenceConnection(organizationID, accountID, disabledID)
	disabled.Status = integrations.ConnectionStatusDisabled
	connections := &preferenceConnectionLookup{connections: map[uuid.UUID]*integrations.IntegrationConnection{
		disabledID:  disabled,
		availableID: availablePreferenceConnection(organizationID, accountID, availableID),
	}}
	access := &preferenceAccessChecker{allowed: map[uuid.UUID]bool{disabledID: true, availableID: true}}
	preferenceID := uuid.New()
	repository := &memoryAIChatPreferenceRepository{preferences: []AIChatIntegrationPreference{{
		ID: preferenceID, OrganizationID: organizationID, AccountID: accountID, WorkspaceID: &workspaceID,
		IntegrationID: "github", SelectedConnectionIDs: []string{deletedID.String(), disabledID.String(), availableID.String()},
		PreferredConnectionID: &deletedID, Revision: 3,
	}}}
	service := NewAIChatIntegrationPreferenceService(repository, connections, access)

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

func TestAIChatPreferenceListDoesNotExposeInaccessiblePersonalConnection(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	otherAccountID := uuid.New()
	connectionID := uuid.New()
	connection := availablePreferenceConnection(organizationID, otherAccountID, connectionID)
	connections := &preferenceConnectionLookup{connections: map[uuid.UUID]*integrations.IntegrationConnection{connectionID: connection}}
	repository := &memoryAIChatPreferenceRepository{preferences: []AIChatIntegrationPreference{{
		ID: uuid.New(), OrganizationID: organizationID, AccountID: accountID, IntegrationID: "github",
		SelectedConnectionIDs: []string{connectionID.String()}, PreferredConnectionID: &connectionID, Revision: 1,
	}}}
	service := NewAIChatIntegrationPreferenceService(repository, connections, &preferenceAccessChecker{allowed: map[uuid.UUID]bool{}})

	items, err := service.List(context.Background(), organizationID, accountID, nil)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 0 || len(repository.preferences) != 0 {
		t.Fatalf("inaccessible personal connection leaked through preferences: returned=%#v stored=%#v", items, repository.preferences)
	}
}

func availablePreferenceConnection(organizationID, accountID, connectionID uuid.UUID) *integrations.IntegrationConnection {
	return &integrations.IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: connectionID.String(),
		CredentialSource: integrations.ConnectionCredentialSourceAccount, OwnerAccountID: &accountID,
		AuthType: integrations.ConnectionAuthTypeAPIKey, Status: integrations.ConnectionStatusActive,
		CredentialVersion: 1, Revision: 1, HealthRevision: 1,
	}
}

func sameOptionalUUID(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
