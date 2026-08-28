package integrations

import (
	"testing"

	"github.com/google/uuid"
)

func TestCompleteConnectionSetupRequiresUsableSharedRule(t *testing.T) {
	repository := newMemoryConnectionRepository()
	grants := &memoryConnectionGrantRepository{}
	organizationID := uuid.New()
	connectionID := uuid.New()
	actorID := uuid.New()
	if err := repository.Create(t.Context(), &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "Shared",
		CredentialSource: ConnectionCredentialSourceOrganization, AuthType: ConnectionAuthTypeAPIKey,
		Status: ConnectionStatusActive, HealthStatus: ConnectionHealthHealthy, AuthStatus: ConnectionAuthValid,
		CredentialVersion: 1, Revision: 1, HealthRevision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	input := CompleteConnectionSetupInput{
		OrganizationID: organizationID, ConnectionID: connectionID, ActorID: actorID,
		UsableActionIDs: []string{"github.repository.list"},
	}
	if err := CompleteConnectionSetup(t.Context(), repository, grants, input); ErrorCode(err) != ErrorCodeAccessDenied {
		t.Fatalf("missing usage rule error = %v, want %s", err, ErrorCodeAccessDenied)
	}
	grants.grants = append(grants.grants, IntegrationConnectionGrant{
		ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID,
		PrincipalType: ConnectionGrantPrincipalOrganization, AccessMode: ConnectionGrantAccessRead,
		AllowedActionIDs: []string{"github.repository.list"}, Revision: 1,
	})
	if err := CompleteConnectionSetup(t.Context(), repository, grants, input); err != nil {
		t.Fatalf("complete setup error = %v", err)
	}
	stored, err := repository.GetByID(t.Context(), organizationID, connectionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SetupVersion != ConnectionSetupVersion || stored.SetupCompletedAt == nil || stored.SetupCompletedBy == nil || *stored.SetupCompletedBy != actorID {
		t.Fatalf("setup completion was not persisted: %#v", stored)
	}
}

func TestCompletePersonalConnectionSetupRequiresOwnerAndHealthyConnection(t *testing.T) {
	repository := newMemoryConnectionRepository()
	grants := &memoryConnectionGrantRepository{}
	organizationID := uuid.New()
	connectionID := uuid.New()
	ownerID := uuid.New()
	if err := repository.Create(t.Context(), &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "Mine",
		CredentialSource: ConnectionCredentialSourceAccount, OwnerAccountID: &ownerID, AuthType: ConnectionAuthTypeAPIKey,
		Status: ConnectionStatusPending, HealthStatus: ConnectionHealthUnknown, AuthStatus: ConnectionAuthUnknown,
		CredentialVersion: 1, Revision: 1, HealthRevision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	input := CompleteConnectionSetupInput{
		OrganizationID: organizationID, ConnectionID: connectionID, ActorID: ownerID,
		Personal: true, UsableActionIDs: []string{"github.user.get"},
	}
	if err := CompleteConnectionSetup(t.Context(), repository, grants, input); ErrorCode(err) != ErrorCodeConnectionInvalid {
		t.Fatalf("unverified connection error = %v, want %s", err, ErrorCodeConnectionInvalid)
	}
	connection, _ := repository.GetByID(t.Context(), organizationID, connectionID)
	connection.Status = ConnectionStatusActive
	connection.HealthStatus = ConnectionHealthHealthy
	connection.AuthStatus = ConnectionAuthValid
	if err := repository.Update(t.Context(), connection); err != nil {
		t.Fatal(err)
	}
	input.ActorID = uuid.New()
	if err := CompleteConnectionSetup(t.Context(), repository, grants, input); ErrorCode(err) != ErrorCodeAccessDenied {
		t.Fatalf("wrong owner error = %v, want %s", err, ErrorCodeAccessDenied)
	}
	input.ActorID = ownerID
	if err := CompleteConnectionSetup(t.Context(), repository, grants, input); err != nil {
		t.Fatalf("personal setup error = %v", err)
	}
}
