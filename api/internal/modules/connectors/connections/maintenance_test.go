package connections

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestMaintenanceReconnectSameIdentityIsIdempotentAndKeepsConnection(t *testing.T) {
	service, repository, bindings, verifier, sealer, _ := newMaintenanceFixture(t)
	existing := maintenanceConnection(service.now(), uuid.New(), uuid.New())
	repository.connections[existing.ID] = existing
	sibling := maintenanceConnection(service.now(), existing.OrganizationID, existing.OwnerAccountID)
	sibling.Name = "Independent GitHub key"
	sibling.EncryptedCredentials = "independent-sealed-envelope"
	repository.connections[sibling.ID] = sibling
	bindings.items = []connectorcore.TargetBinding{maintenanceBinding(existing, connectorcore.BindingStatusActive)}
	values := map[string]string{"token": candidateKeySentinel}

	result, err := service.Reconnect(context.Background(), ReconnectCommand{
		OrganizationID: existing.OrganizationID, OwnerAccountID: existing.OwnerAccountID,
		ConnectionID: existing.ID, IdempotencyKey: "maintenance-reconnect-same-0001", Values: values,
	})
	if err != nil {
		t.Fatalf("Reconnect() error = %v", err)
	}
	if result.Connection == nil || !result.ReplacedExisting || result.Connection.ID != existing.ID ||
		result.Connection.CredentialVersion != existing.CredentialVersion+1 || result.Connection.Revision != existing.Revision+1 {
		t.Fatalf("same-identity reconnect = %#v", result)
	}
	stored := repository.connections[existing.ID]
	if stored.EncryptedCredentials != sealer.envelope || stored.CredentialVersion != existing.CredentialVersion+1 || stored.Name != existing.Name {
		t.Fatalf("stored same-identity reconnect = %#v", stored)
	}
	if len(values) != 0 || len(sealer.values) == 0 || verifier.calls != 1 {
		t.Fatalf("candidate lifecycle values=%#v sealed=%#v calls=%d", values, sealer.values, verifier.calls)
	}

	replay, err := service.Reconnect(context.Background(), ReconnectCommand{
		OrganizationID: existing.OrganizationID, OwnerAccountID: existing.OwnerAccountID,
		ConnectionID: existing.ID, IdempotencyKey: "maintenance-reconnect-same-0001",
		Values: map[string]string{"token": candidateKeySentinel},
	})
	if err != nil || replay.Connection == nil || replay.Connection.ID != existing.ID || verifier.calls != 1 || sealer.calls != 1 {
		t.Fatalf("idempotent reconnect = %#v, %v; calls=%d/%d", replay, err, verifier.calls, sealer.calls)
	}
	if len(bindings.items) != 1 || bindings.items[0].ConnectionID != existing.ID {
		t.Fatalf("same identity changed bindings: %#v", bindings.items)
	}
	unchanged := repository.connections[sibling.ID]
	if len(repository.connections) != 2 || unchanged.EncryptedCredentials != sibling.EncryptedCredentials ||
		unchanged.CredentialVersion != sibling.CredentialVersion || unchanged.Revision != sibling.Revision || unchanged.Name != sibling.Name {
		t.Fatalf("targeted reconnect changed a same-account sibling: %#v", unchanged)
	}
	if len(repository.lifecycle) != 1 || repository.lifecycle[0].Decision != DecisionConnectionReconnected ||
		len(repository.lifecycle[0].Changes) != 2 || repository.lifecycle[0].Changes[0].Type != MutationCredentialRotated ||
		repository.lifecycle[0].Changes[1].Type != MutationReadinessChanged {
		t.Fatalf("same-identity reconnect lifecycle = %#v", repository.lifecycle)
	}
}

func TestMaintenanceReconnectDifferentIdentityCreatesUnboundConnection(t *testing.T) {
	service, repository, bindings, verifier, _, _ := newMaintenanceFixture(t)
	existing := maintenanceConnection(service.now(), uuid.New(), uuid.New())
	repository.connections[existing.ID] = existing
	bindings.items = []connectorcore.TargetBinding{maintenanceBinding(existing, connectorcore.BindingStatusActive)}
	verifier.result.ExternalAccountID = "different-account"
	verifier.result.ExternalDisplayName = "different-user"

	result, err := service.Reconnect(context.Background(), ReconnectCommand{
		OrganizationID: existing.OrganizationID, OwnerAccountID: existing.OwnerAccountID,
		ConnectionID: existing.ID, IdempotencyKey: "maintenance-reconnect-different-1",
		Values: map[string]string{"token": candidateKeySentinel},
	})
	if err != nil {
		t.Fatalf("Reconnect() error = %v", err)
	}
	if result.Connection == nil || result.ReplacedExisting || result.Connection.ID == existing.ID || result.Connection.CredentialVersion != 1 {
		t.Fatalf("different identity result = %#v", result)
	}
	if repository.connections[existing.ID].EncryptedCredentials != existing.EncryptedCredentials || len(repository.connections) != 2 {
		t.Fatalf("different identity mutated existing: %#v", repository.connections)
	}
	for _, binding := range bindings.items {
		if binding.ConnectionID == result.Connection.ID {
			t.Fatalf("new identity inherited binding: %#v", binding)
		}
	}
	if len(repository.lifecycle) != 1 || len(repository.lifecycle[0].Changes) != 0 {
		t.Fatalf("different-identity reconnect changes = %#v", repository.lifecycle)
	}
}

func TestMaintenanceReconnectFailureAndUnstableIdentityNeverReplaceSilently(t *testing.T) {
	service, repository, _, verifier, sealer, _ := newMaintenanceFixture(t)
	existing := maintenanceConnection(service.now(), uuid.New(), uuid.New())
	repository.connections[existing.ID] = existing
	verifier.err = connectorcore.NewError(connectorcore.ErrorCodeInvalidKey, false, nil)

	_, err := service.Reconnect(context.Background(), ReconnectCommand{
		OrganizationID: existing.OrganizationID, OwnerAccountID: existing.OwnerAccountID,
		ConnectionID: existing.ID, IdempotencyKey: "maintenance-reconnect-invalid-1",
		Values: map[string]string{"token": candidateKeySentinel},
	})
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeInvalidKey) || repository.connections[existing.ID].EncryptedCredentials != existing.EncryptedCredentials || sealer.calls != 0 {
		t.Fatalf("failed reconnect error/stored/sealer = %v / %#v / %d", err, repository.connections[existing.ID], sealer.calls)
	}

	verifier.err = nil
	verifier.result.ExternalAccountID = ""
	verifier.result.ExternalDisplayName = ""
	_, err = service.Reconnect(context.Background(), ReconnectCommand{
		OrganizationID: existing.OrganizationID, OwnerAccountID: existing.OwnerAccountID,
		ConnectionID: existing.ID, IdempotencyKey: "maintenance-reconnect-unstable-1",
		Values: map[string]string{"token": candidateKeySentinel},
	})
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConflict) || repository.connections[existing.ID].EncryptedCredentials != existing.EncryptedCredentials || sealer.calls != 0 {
		t.Fatalf("unconfirmed unstable identity = %v / %#v / %d", err, repository.connections[existing.ID], sealer.calls)
	}

	confirmed, err := service.Reconnect(context.Background(), ReconnectCommand{
		OrganizationID: existing.OrganizationID, OwnerAccountID: existing.OwnerAccountID,
		ConnectionID: existing.ID, IdempotencyKey: "maintenance-reconnect-unstable-2",
		Values: map[string]string{"token": candidateKeySentinel}, ConfirmUnstableIdentity: true,
	})
	if err != nil || confirmed.Connection == nil || !confirmed.ReplacedExisting || confirmed.Connection.ID != existing.ID || sealer.calls != 1 {
		t.Fatalf("confirmed unstable identity = %#v, %v; sealer=%d", confirmed, err, sealer.calls)
	}
}

func TestMaintenanceDiagnosticsAreNonSecretAndUpdateReadiness(t *testing.T) {
	service, repository, _, verifier, _, resolver := newMaintenanceFixture(t)
	existing := maintenanceConnection(service.now(), uuid.New(), uuid.New())
	repository.connections[existing.ID] = existing

	ready, err := service.Diagnose(context.Background(), existing.OrganizationID, existing.OwnerAccountID, existing.ID)
	if err != nil || ready.State != connectorcore.ReadinessReady || ready.EncryptedCredentials != "" || len(resolver.returned) != 0 {
		t.Fatalf("ready diagnostics = %#v, %v; retained=%#v", ready, err, resolver.returned)
	}
	verifier.err = connectorcore.NewError(connectorcore.ErrorCodeInvalidKey, false, nil)
	reconnect, err := service.Diagnose(context.Background(), existing.OrganizationID, existing.OwnerAccountID, existing.ID)
	if err != nil || reconnect.State != connectorcore.ReadinessReconnectRequired || reconnect.DiagnosticCode == nil ||
		*reconnect.DiagnosticCode != string(connectorcore.ErrorCodeInvalidKey) || reconnect.EncryptedCredentials != "" {
		t.Fatalf("invalid-key diagnostics = %#v, %v", reconnect, err)
	}
	if repository.connections[existing.ID].EncryptedCredentials != existing.EncryptedCredentials {
		t.Fatal("diagnostics replaced stored credential")
	}
	if len(repository.lifecycle) != 2 || repository.lifecycle[0].Changes[0].Readiness != connectorcore.ReadinessReady ||
		repository.lifecycle[1].Changes[0].Readiness != connectorcore.ReadinessReconnectRequired {
		t.Fatalf("diagnostic lifecycle = %#v", repository.lifecycle)
	}
}

func TestMaintenanceImpactTokenGuardsForceRemovalAndDetectsDrift(t *testing.T) {
	service, repository, bindings, _, _, _ := newMaintenanceFixture(t)
	existing := maintenanceConnection(service.now(), uuid.New(), uuid.New())
	repository.connections[existing.ID] = existing
	active := maintenanceBinding(existing, connectorcore.BindingStatusActive)
	disabled := maintenanceBinding(existing, connectorcore.BindingStatusDisabled)
	replacement := maintenanceBinding(existing, connectorcore.BindingStatusNeedsReplacement)

	bindings.items = []connectorcore.TargetBinding{active, disabled, replacement}

	impact, err := service.GetRemovalImpact(context.Background(), existing.OrganizationID, existing.OwnerAccountID, existing.ID)
	if err != nil || impact.CanRemove || impact.ImpactToken == "" || impact.ExpiresAt.IsZero() || len(impact.AffectedBindings) != 3 {
		t.Fatalf("GetRemovalImpact() = %#v, %v", impact, err)
	}
	err = service.Remove(context.Background(), RemoveConnectionCommand{
		OrganizationID: existing.OrganizationID, OwnerAccountID: existing.OwnerAccountID, ConnectionID: existing.ID,
	})
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConflict) || repository.removed {
		t.Fatalf("normal removal error/removed = %v/%v", err, repository.removed)
	}

	bindings.items[0].Revision++
	err = service.Remove(context.Background(), RemoveConnectionCommand{
		OrganizationID: existing.OrganizationID, OwnerAccountID: existing.OwnerAccountID, ConnectionID: existing.ID,
		Force: true, ImpactToken: impact.ImpactToken,
	})
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConflict) || repository.removed {
		t.Fatalf("stale impact removal error/removed = %v/%v", err, repository.removed)
	}

	fresh, err := service.GetRemovalImpact(context.Background(), existing.OrganizationID, existing.OwnerAccountID, existing.ID)
	if err != nil {
		t.Fatal(err)
	}
	err = service.Remove(context.Background(), RemoveConnectionCommand{
		OrganizationID: existing.OrganizationID, OwnerAccountID: existing.OwnerAccountID, ConnectionID: existing.ID,
		Force: true, ImpactToken: fresh.ImpactToken,
	})
	if err != nil || !repository.removed || repository.lastRemovalConnection != existing.ID || len(repository.lastRemovalBindings) != 1 {
		t.Fatalf("force removal error/state = %v / %#v", err, repository)
	}
	if bindings.items[0].Status != connectorcore.BindingStatusNeedsReplacement || bindings.items[1].Status != connectorcore.BindingStatusDisabled {
		t.Fatalf("force removal binding states = %#v", bindings.items)
	}
	if bindings.items[2].Status != connectorcore.BindingStatusNeedsReplacement || bindings.items[2].Revision != replacement.Revision {
		t.Fatalf("force removal mutated an already unavailable binding: %#v", bindings.items[2])
	}
	if len(repository.lifecycle) != 1 || repository.lifecycle[0].Decision != MutationConnectionRemoved ||
		len(repository.lifecycle[0].Changes) != 1 || repository.lifecycle[0].Changes[0].Type != MutationConnectionRemoved {
		t.Fatalf("removal lifecycle = %#v", repository.lifecycle)
	}
}

func TestMaintenanceRenameUsesOwnerAndRevision(t *testing.T) {
	service, repository, _, _, _, _ := newMaintenanceFixture(t)
	existing := maintenanceConnection(service.now(), uuid.New(), uuid.New())
	repository.connections[existing.ID] = existing
	updated, err := service.Rename(context.Background(), existing.OrganizationID, existing.OwnerAccountID, existing.ID, existing.Revision, "  Work GitHub  ")
	if err != nil || updated.Name != "Work GitHub" || updated.EncryptedCredentials != "" || repository.connections[existing.ID].EncryptedCredentials != existing.EncryptedCredentials {
		t.Fatalf("Rename() = %#v, %v", updated, err)
	}
	if _, err := service.Rename(context.Background(), existing.OrganizationID, uuid.New(), existing.ID, updated.Revision, "Other"); !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeNotFound) {
		t.Fatalf("cross-owner rename error = %v", err)
	}
	if len(repository.lifecycle) != 1 || repository.lifecycle[0].Decision != MutationConnectionRenamed ||
		repository.lifecycle[0].Changes[0].Name != "Work GitHub" {
		t.Fatalf("rename lifecycle = %#v", repository.lifecycle)
	}
}

func newMaintenanceFixture(t *testing.T) (
	*MaintenanceService,
	*maintenanceMemoryRepository,
	*maintenanceBindingReader,
	*recordingCandidateVerifier,
	*recordingCredentialSealer,
	*maintenanceCredentialResolver,
) {
	t.Helper()
	baseService, verifier, sealer, memory := newKeyLifecycleTestService(t)
	bindings := &maintenanceBindingReader{}
	repository := &maintenanceMemoryRepository{memoryRepository: memory, bindings: bindings}
	resolver := &maintenanceCredentialResolver{}
	hasher, err := NewCommandHasher(strings.Repeat("m", 32))
	if err != nil {
		t.Fatal(err)
	}
	codec, err := NewImpactTokenCodec(strings.Repeat("m", 32))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewMaintenanceService(baseService.catalog, repository, bindings, verifier, sealer, resolver, hasher, codec)
	if err != nil {
		t.Fatal(err)
	}
	fixedNow := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedNow }
	return service, repository, bindings, verifier, sealer, resolver
}

func maintenanceConnection(now time.Time, organizationID, ownerID uuid.UUID) connectorcore.PersonalConnection {
	externalID, display := "583231", "octocat"
	return connectorcore.PersonalConnection{
		ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectorID: "github", ProviderID: "github", DriverID: "github-rest", AuthMethodID: "personal_access_token",
		Name: "My GitHub", ExternalAccountID: &externalID, ExternalDisplayName: &display,
		EncryptedCredentials: "old-sealed-envelope", CredentialVersion: 2, State: connectorcore.ReadinessReady,
		UsableOperationIDs: []connectorcore.Identifier{"github.list_repositories"}, UnavailableOperations: map[connectorcore.Identifier]string{},
		Revision: 3, LastVerifiedAt: now.Add(-time.Hour), LastProviderSuccessAt: timePointer(now.Add(-time.Hour)),
		CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-time.Hour),
	}
}

func maintenanceBinding(connection connectorcore.PersonalConnection, status connectorcore.BindingStatus) connectorcore.TargetBinding {
	now := connection.UpdatedAt
	binding := connectorcore.TargetBinding{
		ID: uuid.New(), OrganizationID: connection.OrganizationID, OwnerAccountID: connection.OwnerAccountID,
		ConnectionID: connection.ID, ConnectorID: connection.ConnectorID,
		Target: connectorcore.TargetRef{Type: connectorcore.TargetTypeAgent, ID: uuid.NewString()},
		Status: status, Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	if status != connectorcore.BindingStatusActive {
		binding.DisabledAt = timePointer(now)
	}
	return binding
}

type maintenanceCredentialResolver struct {
	returned map[string]string
	err      error
}

func (resolver *maintenanceCredentialResolver) ResolveConnectionCredentials(_ context.Context, _ connectorcore.PersonalConnection) (map[string]string, error) {
	if resolver.err != nil {
		return nil, resolver.err
	}
	resolver.returned = map[string]string{"token": candidateKeySentinel}
	return resolver.returned, nil
}

type maintenanceBindingReader struct {
	items []connectorcore.TargetBinding
	err   error
}

func (reader *maintenanceBindingReader) ListBindingsByConnection(
	_ context.Context,
	organizationID, ownerID, connectionID uuid.UUID,
	options connectorbindings.ListOptions,
) ([]connectorcore.TargetBinding, error) {
	if reader.err != nil {
		return nil, reader.err
	}
	result := make([]connectorcore.TargetBinding, 0, len(reader.items))
	for _, item := range reader.items {
		if item.OrganizationID == organizationID && item.OwnerAccountID == ownerID && item.ConnectionID == connectionID &&
			maintenanceBindingMatchesOptions(item, options) {
			result = append(result, item)
		}
	}
	return result, nil
}

type maintenanceMemoryRepository struct {
	*memoryRepository
	bindings              *maintenanceBindingReader
	removed               bool
	lastRemovalConnection uuid.UUID
	lastRemovalBindings   []BindingRevisionSnapshot
	lifecycle             []ConnectionLifecycleMutation
}

func (repository *maintenanceMemoryRepository) UpdateConnectionWithLifecycle(
	ctx context.Context,
	organizationID, ownerID uuid.UUID,
	connection connectorcore.PersonalConnection,
	expectedRevision int,
	mutation ConnectionLifecycleMutation,
) (connectorcore.PersonalConnection, error) {
	updated, err := repository.memoryRepository.UpdateConnection(
		ctx, organizationID, ownerID, connection, expectedRevision,
	)
	if err == nil {
		repository.lifecycle = append(repository.lifecycle, mutation)
	}
	return updated, err
}

func (repository *maintenanceMemoryRepository) CompleteReconnectReadyWithLifecycle(
	ctx context.Context,
	organizationID, ownerID, attemptID uuid.UUID,
	completion ReconnectCompletion,
	mutation ConnectionLifecycleMutation,
) (connectorcore.KeySetupAttempt, error) {
	completed, err := repository.CompleteReconnectReady(ctx, organizationID, ownerID, attemptID, completion)
	if err == nil {
		repository.lifecycle = append(repository.lifecycle, mutation)
	}
	return completed, err
}

func (repository *maintenanceMemoryRepository) ForceRemoveConnectionWithLifecycle(
	ctx context.Context,
	organizationID, ownerID, connectionID uuid.UUID,
	expectedRevision int,
	expectedBindings []BindingRevisionSnapshot,
	mutation ConnectionLifecycleMutation,
) error {
	err := repository.ForceRemoveConnection(
		ctx, organizationID, ownerID, connectionID, expectedRevision, expectedBindings,
	)
	if err == nil {
		repository.lifecycle = append(repository.lifecycle, mutation)
	}
	return err
}

func (repository *maintenanceMemoryRepository) CompleteReconnectReady(
	_ context.Context,
	organizationID, ownerID, attemptID uuid.UUID,
	completion ReconnectCompletion,
) (connectorcore.KeySetupAttempt, error) {
	attempt, err := repository.GetSetupAttempt(context.Background(), organizationID, ownerID, attemptID)
	if err != nil {
		return connectorcore.KeySetupAttempt{}, err
	}
	existing, err := repository.GetConnection(context.Background(), organizationID, ownerID, completion.ExistingConnectionID)
	if err != nil || existing.Revision != completion.ExpectedRevision || attempt.State != connectorcore.SetupStateVerifying {
		return connectorcore.KeySetupAttempt{}, connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	result := completion.Result
	if completion.ReplaceExisting {
		result.Revision = completion.ExpectedRevision + 1
	}
	repository.connections[result.ID] = result
	attempt.State = connectorcore.SetupStateReady
	attempt.ResultConnectionID = result.ID
	repository.attempts[attempt.ID] = attempt
	return attempt, nil
}

func (repository *maintenanceMemoryRepository) ForceRemoveConnection(
	_ context.Context,
	organizationID, ownerID, connectionID uuid.UUID,
	expectedRevision int,
	expectedBindings []BindingRevisionSnapshot,
) error {
	connection, err := repository.GetConnection(context.Background(), organizationID, ownerID, connectionID)
	if err != nil {
		return err
	}
	if connection.Revision != expectedRevision || !equalBindingSnapshots(activeBindingSnapshot(repository.bindings.items), expectedBindings) {
		return connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	for index := range repository.bindings.items {
		if repository.bindings.items[index].ConnectionID == connectionID &&
			repository.bindings.items[index].Status == connectorcore.BindingStatusActive {
			repository.bindings.items[index].Status = connectorcore.BindingStatusNeedsReplacement
			repository.bindings.items[index].Revision++
		}
	}
	repository.removed = true
	repository.lastRemovalConnection = connectionID
	repository.lastRemovalBindings = append([]BindingRevisionSnapshot(nil), expectedBindings...)
	delete(repository.connections, connectionID)
	return nil
}

func maintenanceBindingMatchesOptions(binding connectorcore.TargetBinding, options connectorbindings.ListOptions) bool {
	statusMatches := len(options.Statuses) == 0
	for _, status := range options.Statuses {
		statusMatches = statusMatches || binding.Status == status
	}
	return statusMatches
}
