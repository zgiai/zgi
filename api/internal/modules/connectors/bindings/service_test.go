package bindings

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestBindingServiceUpsertIsLogicalKeyIdempotentWithStableUUID(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	connection := readyServiceConnection(organizationID, ownerID, uuid.New())
	command := UpsertBindingCommand{
		OrganizationID:    organizationID,
		ActingPrincipalID: ownerID,
		ConnectionID:      connection.ID,
		IdempotencyKey:    "binding-idempotency-key-0001",
		Target: core.TargetRef{
			Type: core.TargetTypeChat,
			ID:   "workflow-1",
		},
	}

	firstRepository := newMemoryBindingRepository()
	firstAuthority := &recordingTargetAuthority{}
	firstService := mustBindingService(t, firstRepository, &recordingConnectionReader{connection: connection}, firstAuthority)
	firstService.now = func() time.Time { return time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC) }

	first, err := firstService.UpsertBinding(context.Background(), command)
	if err != nil {
		t.Fatalf("UpsertBinding() first error = %v", err)
	}
	if !first.Created || first.Binding.ID == uuid.Nil {
		t.Fatalf("UpsertBinding() first = %#v", first)
	}

	replay := command
	replay.IdempotencyKey = "binding-idempotency-key-0002"
	second, err := firstService.UpsertBinding(context.Background(), replay)
	if err != nil {
		t.Fatalf("UpsertBinding() replay error = %v", err)
	}
	if second.Created || second.Binding.ID != first.Binding.ID || second.Binding.Revision != first.Binding.Revision {
		t.Fatalf("UpsertBinding() replay = %#v, first = %#v", second, first)
	}
	if firstRepository.upsertCalls != 2 || len(firstRepository.bindings) != 1 {
		t.Fatalf("repository calls/rows = %d/%d", firstRepository.upsertCalls, len(firstRepository.bindings))
	}
	if len(firstRepository.mutations) != 1 ||
		firstRepository.mutations[0].MutationType != MutationBindingUpserted ||
		firstRepository.mutations[0].CorrelationID == command.IdempotencyKey {
		t.Fatalf("automatic upsert lifecycle mutations = %#v", firstRepository.mutations)
	}

	// A fresh service derives the same native UUID from the immutable logical
	// key. Existing mapped or legacy UUIDs still win inside the repository.
	secondRepository := newMemoryBindingRepository()
	secondService := mustBindingService(t, secondRepository, &recordingConnectionReader{connection: connection}, &recordingTargetAuthority{})
	third, err := secondService.UpsertBinding(context.Background(), command)
	if err != nil {
		t.Fatalf("UpsertBinding() fresh service error = %v", err)
	}
	if third.Binding.ID != first.Binding.ID {
		t.Fatalf("stable binding UUID = %s, want %s", third.Binding.ID, first.Binding.ID)
	}

	if len(firstAuthority.calls) != 2 {
		t.Fatalf("target authority calls = %#v", firstAuthority.calls)
	}
	for _, call := range firstAuthority.calls {
		if call.organizationID != organizationID || call.actingPrincipalID != ownerID || call.target != (TargetIdentity{Type: core.TargetTypeChat, ID: "workflow-1"}) {
			t.Fatalf("target authority call = %#v", call)
		}
	}
}

func TestBindingServiceUpdatesAuthorizationWithRevisionAndConnectionSnapshot(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	connection := readyServiceConnection(organizationID, ownerID, uuid.New())
	connection.Revision = 4
	connection.CredentialVersion = 3
	repository := newMemoryBindingRepository()
	service := mustBindingService(t, repository, &recordingConnectionReader{connection: connection}, &recordingTargetAuthority{})
	created, err := service.UpsertBinding(context.Background(), UpsertBindingCommand{
		OrganizationID: organizationID, ActingPrincipalID: ownerID, ConnectionID: connection.ID,
		Target:         core.TargetRef{Type: core.TargetTypeChat, ID: "chat-auth"},
		IdempotencyKey: "binding-auth-create-0001", AuthorizationMode: core.AuthorizationModeDirect,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Binding.Constraint.AuthorizationConnectionRevision != 4 || created.Binding.Constraint.AuthorizationCredentialVersion != 3 {
		t.Fatalf("direct authorization snapshot = %#v", created.Binding.Constraint)
	}
	updated, err := service.UpdateBindingAuthorization(context.Background(), UpdateBindingAuthorizationCommand{
		OrganizationID: organizationID, ActingPrincipalID: ownerID, BindingID: created.Binding.ID,
		ExpectedRevision: created.Binding.Revision, AuthorizationMode: core.AuthorizationModeAsk,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != created.Binding.Revision+1 || updated.Constraint.AuthorizationModeValue() != core.AuthorizationModeAsk ||
		updated.Constraint.AuthorizationConnectionRevision != 0 || updated.Constraint.AuthorizationCredentialVersion != 0 {
		t.Fatalf("authorization update = %#v", updated)
	}
	if _, err := service.UpdateBindingAuthorization(context.Background(), UpdateBindingAuthorizationCommand{
		OrganizationID: organizationID, ActingPrincipalID: ownerID, BindingID: created.Binding.ID,
		ExpectedRevision: created.Binding.Revision, AuthorizationMode: core.AuthorizationModeDirect,
	}); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("stale authorization update error = %v, want conflict", err)
	}
}

func TestBindingServiceReplacementKeepsBindingUUID(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	firstConnection := readyServiceConnection(organizationID, ownerID, uuid.New())
	secondConnection := readyServiceConnection(organizationID, ownerID, uuid.New())
	repository := newMemoryBindingRepository()
	reader := &recordingConnectionReader{connections: map[uuid.UUID]core.PersonalConnection{
		firstConnection.ID:  firstConnection,
		secondConnection.ID: secondConnection,
	}}
	service := mustBindingService(t, repository, reader, &recordingTargetAuthority{})
	command := UpsertBindingCommand{
		OrganizationID: organizationID, ActingPrincipalID: ownerID,
		ConnectionID: firstConnection.ID, IdempotencyKey: "binding-idempotency-key-0003",
		Target: core.TargetRef{Type: core.TargetTypeChat, ID: "agent-1"},
	}
	first, err := service.UpsertBinding(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	command.ConnectionID = secondConnection.ID
	command.IdempotencyKey = "binding-idempotency-key-0004"
	command.ExpectedRevision = first.Binding.Revision
	replaced, err := service.UpsertBinding(context.Background(), command)
	if err != nil {
		t.Fatalf("UpsertBinding() replacement error = %v", err)
	}
	if replaced.Created || replaced.Binding.ID != first.Binding.ID || replaced.Binding.ConnectionID != secondConnection.ID || replaced.Binding.Revision != first.Binding.Revision+1 {
		t.Fatalf("replacement = %#v, first = %#v", replaced, first)
	}
	if len(repository.mutations) != 2 ||
		repository.mutations[0].MutationType != MutationBindingUpserted ||
		repository.mutations[1].MutationType != MutationBindingUpserted ||
		repository.mutations[0].CorrelationID == repository.mutations[1].CorrelationID {
		t.Fatalf("service lifecycle mutations = %#v", repository.mutations)
	}
}

func TestBindingServiceChatReplacementRequiresExpectedRevisionAndIsAtomic(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	firstConnection := readyServiceConnection(organizationID, ownerID, uuid.New())
	secondConnection := readyServiceConnection(organizationID, ownerID, uuid.New())
	repository := newMemoryBindingRepository()
	service := mustBindingService(t, repository, &recordingConnectionReader{connections: map[uuid.UUID]core.PersonalConnection{
		firstConnection.ID: firstConnection, secondConnection.ID: secondConnection,
	}}, &recordingTargetAuthority{})
	command := UpsertBindingCommand{
		OrganizationID: organizationID, ActingPrincipalID: ownerID,
		ConnectionID: firstConnection.ID, IdempotencyKey: "chat-binding-create-0001",
		Target: core.TargetRef{Type: core.TargetTypeChat, ID: "chat-1"},
	}
	first, err := service.UpsertBinding(context.Background(), command)
	if err != nil {
		t.Fatalf("create Chat binding: %v", err)
	}

	command.ConnectionID = secondConnection.ID
	command.IdempotencyKey = "chat-binding-replace-0001"
	if _, err := service.UpsertBinding(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("replace Chat binding without expected revision error = %v, want conflict", err)
	}
	stored := repository.bindings[bindingLogicalKey(first.Binding)]
	if stored.ID != first.Binding.ID || stored.ConnectionID != firstConnection.ID || stored.Revision != first.Binding.Revision {
		t.Fatalf("failed replacement changed Chat binding: %#v", stored)
	}

	command.ExpectedRevision = first.Binding.Revision
	replaced, err := service.UpsertBinding(context.Background(), command)
	if err != nil {
		t.Fatalf("replace Chat binding with expected revision: %v", err)
	}
	if replaced.Created || replaced.Binding.ID != first.Binding.ID || replaced.Binding.ConnectionID != secondConnection.ID ||
		replaced.Binding.Revision != first.Binding.Revision+1 {
		t.Fatalf("Chat replacement = %#v, first = %#v", replaced, first)
	}
}

func TestBindingServiceNewConnectionDoesNotReplaceAnOccupiedTargetSlot(t *testing.T) {
	for _, target := range []core.TargetRef{
		{Type: core.TargetTypeChat, ID: "chat-1"},
		{Type: core.TargetTypeChat, ID: "agent-1"},
		{Type: core.TargetTypeChat, ID: "workflow-1"},
	} {
		t.Run(string(target.Type), func(t *testing.T) {
			organizationID, ownerID := uuid.New(), uuid.New()
			first := readyServiceConnection(organizationID, ownerID, uuid.New())
			second := readyServiceConnection(organizationID, ownerID, uuid.New())
			externalAccountID := "583231"
			first.ExternalAccountID, second.ExternalAccountID = &externalAccountID, &externalAccountID
			repository := newMemoryBindingRepository()
			service := mustBindingService(t, repository, &recordingConnectionReader{connections: map[uuid.UUID]core.PersonalConnection{
				first.ID: first, second.ID: second,
			}}, &recordingTargetAuthority{})
			created, err := service.UpsertBinding(context.Background(), UpsertBindingCommand{
				OrganizationID: organizationID, ActingPrincipalID: ownerID, ConnectionID: first.ID,
				Target: target, IdempotencyKey: "first-connection-slot-0001",
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.UpsertBinding(context.Background(), UpsertBindingCommand{
				OrganizationID: organizationID, ActingPrincipalID: ownerID, ConnectionID: second.ID,
				Target: target, IdempotencyKey: "second-connection-slot-0001",
			})
			stored := repository.bindings[bindingLogicalKey(created.Binding)]
			if !core.IsErrorCode(err, core.ErrorCodeConflict) || stored.ID != created.Binding.ID ||
				stored.ConnectionID != first.ID || stored.Revision != created.Binding.Revision || len(repository.mutations) != 1 {
				t.Fatalf("new connection replaced an occupied slot: stored=%#v, error=%v", stored, err)
			}
		})
	}
}

func TestBindingServiceRequiresExactOrganizationAndOwner(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	connection := readyServiceConnection(uuid.New(), ownerID, uuid.New())
	repository := newMemoryBindingRepository()
	reader := &recordingConnectionReader{connection: connection}
	authority := &recordingTargetAuthority{}
	service := mustBindingService(t, repository, reader, authority)

	_, err := service.UpsertBinding(context.Background(), UpsertBindingCommand{
		OrganizationID: organizationID, ActingPrincipalID: ownerID,
		ConnectionID: connection.ID, IdempotencyKey: "binding-idempotency-key-0005",
		Target: core.TargetRef{Type: core.TargetTypeChat, ID: "chat-1"},
	})
	if !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
		t.Fatalf("UpsertBinding() error = %v, want %s", err, core.ErrorCodeAccessDenied)
	}
	if len(reader.calls) != 1 || reader.calls[0].organizationID != organizationID || reader.calls[0].ownerID != ownerID {
		t.Fatalf("connection reader calls = %#v", reader.calls)
	}
	if len(authority.calls) != 0 || repository.upsertCalls != 0 {
		t.Fatalf("authority/repository were reached: %d/%d", len(authority.calls), repository.upsertCalls)
	}
}

func TestBindingServiceRequiresTargetEditAuthority(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	connection := readyServiceConnection(organizationID, ownerID, uuid.New())
	repository := newMemoryBindingRepository()
	authority := &recordingTargetAuthority{err: core.NewError(core.ErrorCodeAccessDenied, false, nil)}
	service := mustBindingService(t, repository, &recordingConnectionReader{connection: connection}, authority)

	_, err := service.UpsertBinding(context.Background(), UpsertBindingCommand{
		OrganizationID: organizationID, ActingPrincipalID: ownerID,
		ConnectionID: connection.ID, IdempotencyKey: "binding-idempotency-key-0006",
		Target: core.TargetRef{Type: core.TargetTypeChat, ID: "agent-1"},
	})
	if !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
		t.Fatalf("UpsertBinding() error = %v, want %s", err, core.ErrorCodeAccessDenied)
	}
	if len(authority.calls) != 1 || repository.upsertCalls != 0 {
		t.Fatalf("authority/repository calls = %d/%d", len(authority.calls), repository.upsertCalls)
	}

	_, err = service.ListTargetBindings(
		context.Background(), organizationID, ownerID,
		TargetIdentity{Type: core.TargetTypeChat, ID: "agent-1"},
	)
	if !core.IsErrorCode(err, core.ErrorCodeAccessDenied) || repository.listTargetCalls != 0 {
		t.Fatalf("ListTargetBindings() error/calls = %v/%d", err, repository.listTargetCalls)
	}
}

func TestBindingServiceRequiresReadyConnectionBeforeTargetOrWrite(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	connection := readyServiceConnection(organizationID, ownerID, uuid.New())
	connection.State = core.ReadinessReconnectRequired
	repository := newMemoryBindingRepository()
	authority := &recordingTargetAuthority{}
	service := mustBindingService(t, repository, &recordingConnectionReader{connection: connection}, authority)

	_, err := service.UpsertBinding(context.Background(), UpsertBindingCommand{
		OrganizationID: organizationID, ActingPrincipalID: ownerID,
		ConnectionID: connection.ID, IdempotencyKey: "binding-idempotency-key-0007",
		Target: core.TargetRef{Type: core.TargetTypeChat, ID: "chat-1"},
	})
	if !core.IsErrorCode(err, core.ErrorCodeConnectionNotReady) {
		t.Fatalf("UpsertBinding() error = %v, want %s", err, core.ErrorCodeConnectionNotReady)
	}
	if len(authority.calls) != 0 || repository.upsertCalls != 0 {
		t.Fatalf("authority/repository were reached: %d/%d", len(authority.calls), repository.upsertCalls)
	}
}

func TestBindingServiceDetachIsOwnerAuthorizedRevisionedAndIdempotent(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	connection := readyServiceConnection(organizationID, ownerID, uuid.New())
	repository := newMemoryBindingRepository()
	binding := serviceBinding(
		organizationID,
		ownerID,
		connection,
		core.TargetRef{Type: core.TargetTypeChat, ID: "workflow-1"},
	)
	repository.bindings[bindingLogicalKey(binding)] = binding
	authority := &recordingTargetAuthority{}
	service := mustBindingService(t, repository, &recordingConnectionReader{connection: connection}, authority)

	detached, err := service.DetachBinding(context.Background(), DetachBindingCommand{
		OrganizationID: organizationID, ActingPrincipalID: ownerID,
		BindingID: binding.ID, ExpectedRevision: binding.Revision,
	})
	if err != nil {
		t.Fatalf("DetachBinding() error = %v", err)
	}
	if detached.ID != binding.ID || detached.ConnectionID != binding.ConnectionID || detached.Target != binding.Target ||
		detached.Status != core.BindingStatusDisabled || detached.Revision != binding.Revision+1 {
		t.Fatalf("DetachBinding() = %#v, before = %#v", detached, binding)
	}
	if repository.setStatusCalls != 1 || len(authority.calls) != 1 ||
		authority.calls[0].target != (TargetIdentity{Type: core.TargetTypeChat, ID: "workflow-1"}) {
		t.Fatalf("detach repository/authority calls = %d/%#v", repository.setStatusCalls, authority.calls)
	}

	// A transport retry with the revision it originally submitted returns the
	// already-disabled row and cannot increment the revision a second time.
	replayed, err := service.DetachBinding(context.Background(), DetachBindingCommand{
		OrganizationID: organizationID, ActingPrincipalID: ownerID,
		BindingID: binding.ID, ExpectedRevision: binding.Revision,
	})
	if err != nil || replayed.ID != detached.ID || replayed.Status != detached.Status ||
		replayed.Revision != detached.Revision || replayed.ConnectionID != detached.ConnectionID || replayed.Target != detached.Target {
		t.Fatalf("DetachBinding() replay = %#v, %v; want %#v", replayed, err, detached)
	}
	if repository.setStatusCalls != 1 || len(authority.calls) != 2 {
		t.Fatalf("detach replay repository/authority calls = %d/%d", repository.setStatusCalls, len(authority.calls))
	}
	if len(repository.mutations) != 1 || repository.mutations[0].MutationType != MutationBindingDetached {
		t.Fatalf("automatic detach lifecycle mutations = %#v", repository.mutations)
	}

	impact, err := service.GetRemovalImpact(context.Background(), organizationID, ownerID, connection.ID)
	if err != nil || !impact.CanRemove || impact.BlockingBindingCount != 0 {
		t.Fatalf("GetRemovalImpact() after detach = %#v, %v", impact, err)
	}
}

func TestBindingServiceDetachRejectsStaleAndUnauthorizedMutations(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	connection := readyServiceConnection(organizationID, ownerID, uuid.New())
	target := core.TargetRef{Type: core.TargetTypeChat, ID: "agent-1"}

	t.Run("stale revision", func(t *testing.T) {
		repository := newMemoryBindingRepository()
		binding := serviceBinding(organizationID, ownerID, connection, target)
		binding.Revision = 3
		repository.bindings[bindingLogicalKey(binding)] = binding
		authority := &recordingTargetAuthority{}
		service := mustBindingService(t, repository, &recordingConnectionReader{connection: connection}, authority)

		_, err := service.DetachBinding(context.Background(), DetachBindingCommand{
			OrganizationID: organizationID, ActingPrincipalID: ownerID,
			BindingID: binding.ID, ExpectedRevision: 1,
		})
		if !core.IsErrorCode(err, core.ErrorCodeConflict) || repository.setStatusCalls != 0 || len(authority.calls) != 1 {
			t.Fatalf("stale detach error/calls = %v/%d/%d", err, repository.setStatusCalls, len(authority.calls))
		}
	})

	t.Run("target authority", func(t *testing.T) {
		repository := newMemoryBindingRepository()
		binding := serviceBinding(organizationID, ownerID, connection, target)
		repository.bindings[bindingLogicalKey(binding)] = binding
		authority := &recordingTargetAuthority{err: core.NewError(core.ErrorCodeAccessDenied, false, nil)}
		service := mustBindingService(t, repository, &recordingConnectionReader{connection: connection}, authority)

		_, err := service.DetachBinding(context.Background(), DetachBindingCommand{
			OrganizationID: organizationID, ActingPrincipalID: ownerID,
			BindingID: binding.ID, ExpectedRevision: binding.Revision,
		})
		if !core.IsErrorCode(err, core.ErrorCodeAccessDenied) || repository.setStatusCalls != 0 || len(authority.calls) != 1 {
			t.Fatalf("unauthorized detach error/calls = %v/%d/%d", err, repository.setStatusCalls, len(authority.calls))
		}
	})
}

func TestBindingServiceRemovalImpactIsNativeOwnerScopedAndReadOnly(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	connection := readyServiceConnection(organizationID, ownerID, uuid.New())
	repository := newMemoryBindingRepository()
	active := serviceBinding(organizationID, ownerID, connection, core.TargetRef{Type: core.TargetTypeChat, ID: "agent-1"})
	disabled := serviceBinding(organizationID, ownerID, connection, core.TargetRef{Type: core.TargetTypeChat, ID: "workflow-1"})
	disabled.ID = uuid.New()
	disabled.Status = core.BindingStatusDisabled

	repository.bindings[bindingLogicalKey(active)] = active
	repository.bindings[bindingLogicalKey(disabled)] = disabled
	authority := &recordingTargetAuthority{}
	reader := &recordingConnectionReader{connection: connection}
	service := mustBindingService(t, repository, reader, authority)

	impact, err := service.GetRemovalImpact(context.Background(), organizationID, ownerID, connection.ID)
	if err != nil {
		t.Fatalf("GetRemovalImpact() error = %v", err)
	}
	if impact.ConnectionID != connection.ID || impact.CanRemove || impact.BlockingBindingCount != 1 || len(impact.AffectedBindings) != 2 {
		t.Fatalf("GetRemovalImpact() = %#v", impact)
	}
	if impact.AffectedBindings[0].ID != active.ID {
		t.Fatalf("native removal impact = %#v", impact.AffectedBindings)
	}
	storedDisabled, err := repository.GetBinding(context.Background(), organizationID, ownerID, disabled.ID)
	if err != nil || storedDisabled.ID != disabled.ID || storedDisabled.ConnectionID != disabled.ConnectionID ||
		storedDisabled.Status != disabled.Status || storedDisabled.Revision != disabled.Revision {
		t.Fatalf("disabled binding changed while inspecting native impact: %#v, %v", storedDisabled, err)
	}
	if len(authority.calls) != 0 {
		t.Fatalf("removal impact incorrectly required current target edit authority: %#v", authority.calls)
	}
	if len(reader.calls) != 1 || reader.calls[0].organizationID != organizationID || reader.calls[0].ownerID != ownerID {
		t.Fatalf("connection reader calls = %#v", reader.calls)
	}
}

func mustBindingService(t *testing.T, repository Repository, reader ConnectionReader, authority TargetAuthority) *Service {
	t.Helper()
	service, err := NewService(repository, reader, authority)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func readyServiceConnection(organizationID, ownerID, connectionID uuid.UUID) core.PersonalConnection {
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	return core.PersonalConnection{
		ID: connectionID, OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectorID: "github", ProviderID: "github", DriverID: "github-rest", AuthMethodID: "personal_access_token",
		Name: "octocat", EncryptedCredentials: "not-read-by-binding-service", CredentialVersion: 1,
		State: core.ReadinessReady, UsableOperationIDs: []core.Identifier{"github.list_repositories"},
		UnavailableOperations: map[core.Identifier]string{}, Revision: 1,
		LastVerifiedAt: now, CreatedAt: now, UpdatedAt: now,
	}
}

func serviceBinding(organizationID, ownerID uuid.UUID, connection core.PersonalConnection, target core.TargetRef) core.TargetBinding {
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	return core.TargetBinding{
		ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectionID: connection.ID, ConnectorID: connection.ConnectorID, Target: target,
		Status: core.BindingStatusActive, Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
}

type connectionReaderCall struct {
	organizationID uuid.UUID
	ownerID        uuid.UUID
	connectionID   uuid.UUID
}

type recordingConnectionReader struct {
	connection  core.PersonalConnection
	connections map[uuid.UUID]core.PersonalConnection
	err         error
	calls       []connectionReaderCall
}

func (reader *recordingConnectionReader) GetConnection(_ context.Context, organizationID, ownerID, connectionID uuid.UUID) (core.PersonalConnection, error) {
	reader.calls = append(reader.calls, connectionReaderCall{organizationID: organizationID, ownerID: ownerID, connectionID: connectionID})
	if reader.err != nil {
		return core.PersonalConnection{}, reader.err
	}
	if reader.connections != nil {
		if connection, ok := reader.connections[connectionID]; ok {
			return connection, nil
		}
		return core.PersonalConnection{}, core.NewError(core.ErrorCodeNotFound, false, nil)
	}
	return reader.connection, nil
}

type targetAuthorityCall struct {
	organizationID    uuid.UUID
	actingPrincipalID uuid.UUID
	target            TargetIdentity
}

type recordingTargetAuthority struct {
	err   error
	calls []targetAuthorityCall
}

func (authority *recordingTargetAuthority) RequireTargetEdit(_ context.Context, organizationID, actingPrincipalID uuid.UUID, target TargetIdentity) error {
	authority.calls = append(authority.calls, targetAuthorityCall{
		organizationID: organizationID, actingPrincipalID: actingPrincipalID, target: target,
	})
	return authority.err
}

type memoryBindingRepository struct {
	bindings        map[string]core.TargetBinding
	upsertCalls     int
	listTargetCalls int
	setStatusCalls  int
	mutations       []BindingLifecycleMutation
}

func newMemoryBindingRepository() *memoryBindingRepository {
	return &memoryBindingRepository{bindings: map[string]core.TargetBinding{}}
}

func (repository *memoryBindingRepository) UpsertBinding(_ context.Context, organizationID, ownerID uuid.UUID, binding core.TargetBinding, expectedRevision int) (core.TargetBinding, bool, error) {
	repository.upsertCalls++
	key := bindingLogicalKey(binding)
	existing, ok := repository.bindings[key]
	if !ok {
		repository.bindings[key] = binding
		return binding, true, nil
	}
	if existing.ConnectionID == binding.ConnectionID && existing.Status == binding.Status &&
		core.EquivalentConstraints(existing.Constraint, binding.Constraint) {
		return existing, false, nil
	}
	if expectedRevision < 1 || existing.Revision != expectedRevision || existing.OrganizationID != organizationID || existing.OwnerAccountID != ownerID {
		return core.TargetBinding{}, false, core.NewError(core.ErrorCodeConflict, false, nil)
	}
	existing.ConnectionID = binding.ConnectionID
	existing.Status = binding.Status
	existing.Constraint = binding.Constraint
	existing.Revision++
	existing.UpdatedAt = binding.UpdatedAt
	repository.bindings[key] = existing
	return existing, false, nil
}

func (repository *memoryBindingRepository) UpsertBindingWithLifecycle(
	ctx context.Context,
	organizationID, ownerID uuid.UUID,
	binding core.TargetBinding,
	expectedRevision int,
	mutation BindingLifecycleMutation,
) (core.TargetBinding, bool, error) {
	stored, created, err := repository.UpsertBinding(ctx, organizationID, ownerID, binding, expectedRevision)
	if err == nil && (created || stored.Revision != binding.Revision || stored.ConnectionID != binding.ConnectionID) {
		repository.mutations = append(repository.mutations, mutation)
	}
	return stored, created, err
}

func (repository *memoryBindingRepository) UpdateBindingAuthorizationWithLifecycle(
	_ context.Context,
	organizationID, ownerID, bindingID uuid.UUID,
	constraint *core.AccessConstraint,
	expectedRevision int,
	mutation BindingLifecycleMutation,
) (core.TargetBinding, error) {
	for key, binding := range repository.bindings {
		if binding.ID != bindingID || binding.OrganizationID != organizationID || binding.OwnerAccountID != ownerID {
			continue
		}
		if binding.Revision != expectedRevision || binding.Status != core.BindingStatusActive {
			return core.TargetBinding{}, core.NewError(core.ErrorCodeConflict, false, nil)
		}
		binding.Constraint = constraint
		binding.Revision++
		repository.bindings[key] = binding
		repository.mutations = append(repository.mutations, mutation)
		return binding, nil
	}
	return core.TargetBinding{}, core.NewError(core.ErrorCodeNotFound, false, nil)
}

func (repository *memoryBindingRepository) GetBinding(_ context.Context, organizationID, ownerID, bindingID uuid.UUID) (core.TargetBinding, error) {
	for _, binding := range repository.bindings {
		if binding.ID == bindingID && binding.OrganizationID == organizationID && binding.OwnerAccountID == ownerID {
			return binding, nil
		}
	}
	return core.TargetBinding{}, core.NewError(core.ErrorCodeNotFound, false, nil)
}

func (repository *memoryBindingRepository) ListBindingsByTarget(_ context.Context, organizationID, ownerID uuid.UUID, targetType core.TargetType, targetID string, options ListOptions) ([]core.TargetBinding, error) {
	repository.listTargetCalls++
	items := make([]core.TargetBinding, 0)
	for _, binding := range repository.bindings {
		if binding.OrganizationID == organizationID && binding.OwnerAccountID == ownerID && binding.Target.Type == targetType && binding.Target.ID == targetID && memoryBindingMatchesOptions(binding, options) {
			items = append(items, binding)
		}
	}
	return items, nil
}

func (repository *memoryBindingRepository) ListBindingsByConnection(_ context.Context, organizationID, ownerID, connectionID uuid.UUID, options ListOptions) ([]core.TargetBinding, error) {
	items := make([]core.TargetBinding, 0)
	for _, binding := range repository.bindings {
		if binding.OrganizationID == organizationID && binding.OwnerAccountID == ownerID && binding.ConnectionID == connectionID && memoryBindingMatchesOptions(binding, options) {
			items = append(items, binding)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Target.ID < items[j].Target.ID })
	return items, nil
}

func (repository *memoryBindingRepository) ListBindingsByConnector(_ context.Context, organizationID, ownerID uuid.UUID, connectorID core.Identifier, options ListOptions) ([]core.TargetBinding, error) {
	items := make([]core.TargetBinding, 0)
	for _, binding := range repository.bindings {
		if binding.OrganizationID == organizationID && binding.OwnerAccountID == ownerID && binding.ConnectorID == connectorID && memoryBindingMatchesOptions(binding, options) {
			items = append(items, binding)
		}
	}
	return items, nil
}

func (repository *memoryBindingRepository) SetBindingStatus(_ context.Context, organizationID, ownerID, bindingID uuid.UUID, status core.BindingStatus, expectedRevision int) (core.TargetBinding, error) {
	repository.setStatusCalls++
	for key, binding := range repository.bindings {
		if binding.ID != bindingID || binding.OrganizationID != organizationID || binding.OwnerAccountID != ownerID {
			continue
		}
		if binding.Revision != expectedRevision {
			return core.TargetBinding{}, core.NewError(core.ErrorCodeConflict, false, nil)
		}
		binding.Status = status
		binding.Revision++
		repository.bindings[key] = binding
		return binding, nil
	}
	return core.TargetBinding{}, core.NewError(core.ErrorCodeNotFound, false, nil)
}

func (repository *memoryBindingRepository) SetBindingStatusWithLifecycle(
	ctx context.Context,
	organizationID, ownerID, bindingID uuid.UUID,
	status core.BindingStatus,
	expectedRevision int,
	mutation BindingLifecycleMutation,
) (core.TargetBinding, error) {
	stored, err := repository.SetBindingStatus(ctx, organizationID, ownerID, bindingID, status, expectedRevision)
	if err == nil {
		repository.mutations = append(repository.mutations, mutation)
	}
	return stored, err
}

func bindingLogicalKey(binding core.TargetBinding) string {
	return binding.OrganizationID.String() + "\x00" + binding.OwnerAccountID.String() + "\x00" +
		string(binding.Target.Type) + "\x00" + binding.Target.ID + "\x00" + binding.ConnectorID.String() + "\x00" + binding.Target.BindingInstanceID
}

func memoryBindingMatchesOptions(binding core.TargetBinding, options ListOptions) bool {
	statusMatches := len(options.Statuses) == 0
	for _, status := range options.Statuses {
		statusMatches = statusMatches || binding.Status == status
	}
	return statusMatches
}
