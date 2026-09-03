package agents

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"github.com/zgiai/zgi/api/internal/dto"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAgentConnectorBindingsRoundTripDraftPublishAndHistorySnapshot(t *testing.T) {
	ownerID := uuid.New()
	connectionOne := uuid.New()
	connectionTwo := uuid.New()
	config := &AgentsConfig{}
	request := dto.AgentConfigRequest{
		ModelParameters: map[string]interface{}{},
		ConnectorBindings: []dto.AgentConnectorBinding{
			{ConnectorID: " GitHub ", ConnectionID: connectionOne.String()},
			{ConnectorID: "github", ConnectionID: connectionTwo.String()},
			{ConnectorID: "EXA", ConnectionID: uuid.NewString()},
		},
	}
	if _, err := applyAgentConfigRequestToDraft(config, request, ownerID.String()); err != nil {
		t.Fatalf("applyAgentConfigRequestToDraft() error = %v", err)
	}

	draft := agentConfigResponse(uuid.NewString(), config)
	if len(draft.ConnectorBindings) != 2 || draft.ConnectorBindings[0].ConnectorID != "exa" ||
		draft.ConnectorBindings[1] != (dto.AgentConnectorBinding{ConnectorID: "github", ConnectionID: connectionTwo.String()}) {
		t.Fatalf("draft Connector bindings = %#v", draft.ConnectorBindings)
	}
	snapshot := agentConfigSnapshot(draft.AgentID, config)
	historical := agentConfigResponseFromSnapshot(draft.AgentID, snapshot)
	if !connectorBindingsEqual(draft.ConnectorBindings, historical.ConnectorBindings) {
		t.Fatalf("historical Connector bindings = %#v, want %#v", historical.ConnectorBindings, draft.ConnectorBindings)
	}

	agent := &Agent{ID: uuid.New(), TenantID: uuid.New()}
	rows, err := (&agentsService{}).bindingRowsForConfig(
		context.Background(), agent, draft, agentbindings.ScopeDraft, nil, ownerID.String(), time.Now(),
	)
	if err != nil {
		t.Fatalf("bindingRowsForConfig() error = %v", err)
	}
	if len(rows) != 0 || len(agentBindingAuthorizationsFromRows(rows)) != 0 {
		t.Fatalf("Connector leaked into the ordinary resource ledger: %#v", rows)
	}

}

func TestPublishedAgentBindingRowsUseIndependentSnapshotIdentity(t *testing.T) {
	draftID := uuid.New()
	createdAt := time.Unix(1_800_000_000, 0).UTC()
	rows := []agentbindings.Binding{{
		ID: draftID, BindingType: agentbindings.BindingTypeSkill,
		ResourceID: "connection", ParentResourceID: "github",
		CreatedAt: createdAt, UpdatedAt: createdAt,
	}}

	published := publishedAgentBindingRows(rows)
	if len(published) != 1 || published[0].ID != uuid.Nil ||
		!published[0].CreatedAt.IsZero() || !published[0].UpdatedAt.IsZero() {
		t.Fatalf("published row retained draft ledger identity: %#v", published)
	}
	if rows[0].ID != draftID || !rows[0].CreatedAt.Equal(createdAt) {
		t.Fatalf("draft row was mutated: %#v", rows[0])
	}
}

func TestSyncAgentConnectorTargetBindingsReplaceDetachAndStableUUID(t *testing.T) {
	organizationID, ownerID, agentID := uuid.New(), uuid.New(), uuid.New()
	firstConnection, secondConnection := uuid.New(), uuid.New()
	target := connectorcore.TargetRef{Type: connectorcore.TargetTypeAgent, ID: agentID.String()}
	repository := newMemoryAgentConnectorBindingRepository()
	repository.connections[firstConnection] = memoryAgentConnectorConnection{organizationID: organizationID, ownerID: ownerID, connectorID: "github"}
	repository.connections[secondConnection] = memoryAgentConnectorConnection{organizationID: organizationID, ownerID: ownerID, connectorID: "github"}

	first := syncAgentConnectorBindingsWithRepository(t, repository, organizationID, ownerID, target, []dto.AgentConnectorBinding{{
		ConnectorID: "github", ConnectionID: firstConnection.String(),
	}})
	if len(first) != 1 || first[0].Revision != 1 || first[0].Status != connectorcore.BindingStatusActive {
		t.Fatalf("first Target Binding = %#v", first)
	}
	wantID := first[0].ID
	if wantID == uuid.Nil {
		t.Fatal("Target Binding ID is nil")
	}

	replaced := syncAgentConnectorBindingsWithRepository(t, repository, organizationID, ownerID, target, []dto.AgentConnectorBinding{{
		ConnectorID: "github", ConnectionID: secondConnection.String(),
	}})
	if len(replaced) != 1 || replaced[0].ID != wantID || replaced[0].ConnectionID != secondConnection || replaced[0].Revision != 2 {
		t.Fatalf("replaced Target Binding = %#v", replaced)
	}

	syncAgentConnectorBindingsWithRepository(t, repository, organizationID, ownerID, target, nil)
	stored, err := repository.GetBinding(t.Context(), organizationID, ownerID, wantID)
	if err != nil {
		t.Fatalf("GetBinding() error = %v", err)
	}
	if stored.Status != connectorcore.BindingStatusDisabled || stored.Revision != 3 {
		t.Fatalf("detached Target Binding = %#v", stored)
	}

	readded := syncAgentConnectorBindingsWithRepository(t, repository, organizationID, ownerID, target, []dto.AgentConnectorBinding{{
		ConnectorID: "github", ConnectionID: firstConnection.String(),
	}})
	if len(readded) != 1 || readded[0].ID != wantID || readded[0].Status != connectorcore.BindingStatusActive || readded[0].Revision != 4 {
		t.Fatalf("re-added Target Binding = %#v", readded)
	}
}

func TestSyncAgentConnectorTargetBindingsRejectsNonOwnerAndConnectorForgery(t *testing.T) {
	organizationID, ownerID, otherAccountID := uuid.New(), uuid.New(), uuid.New()
	connectionID := uuid.New()
	target := connectorcore.TargetRef{Type: connectorcore.TargetTypeAgent, ID: uuid.NewString()}
	repository := newMemoryAgentConnectorBindingRepository()
	repository.connections[connectionID] = memoryAgentConnectorConnection{organizationID: organizationID, ownerID: ownerID, connectorID: "github"}

	tests := []struct {
		name      string
		accountID uuid.UUID
		connector string
	}{
		{name: "non owner", accountID: otherAccountID, connector: "github"},
		{name: "forged Connector", accountID: ownerID, connector: "exa"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := syncAgentConnectorTargetBindingsWithRepository(t.Context(), repository, organizationID, test.accountID, target, []dto.AgentConnectorBinding{{
				ConnectorID: test.connector, ConnectionID: connectionID.String(),
			}})
			var apiErr *agentBindingAPIError
			if !errors.As(err, &apiErr) || apiErr.Code != agentBindingsInvalidCode {
				t.Fatalf("sync error = %v, want owner-scoped invalid binding", err)
			}
		})
	}
	if len(repository.bindings) != 0 {
		t.Fatalf("forged Target Bindings = %#v, want none", repository.bindings)
	}
}

func TestOwnedAgentConnectorBindingsProjectOnlyOwnerNativeUsableStates(t *testing.T) {
	organizationID, ownerID, foreignOwnerID := uuid.New(), uuid.New(), uuid.New()
	target := connectorcore.TargetRef{Type: connectorcore.TargetTypeAgent, ID: uuid.NewString()}
	repository := newMemoryAgentConnectorBindingRepository()
	bindings := []connectorcore.TargetBinding{
		{ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID, ConnectionID: uuid.New(), ConnectorID: "github", Target: target, Status: connectorcore.BindingStatusActive, Revision: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID, ConnectionID: uuid.New(), ConnectorID: "exa", Target: target, Status: connectorcore.BindingStatusNeedsReplacement, Revision: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID, ConnectionID: uuid.New(), ConnectorID: "disabled", Target: target, Status: connectorcore.BindingStatusDisabled, Revision: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},

		{ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: foreignOwnerID, ConnectionID: uuid.New(), ConnectorID: "foreign", Target: target, Status: connectorcore.BindingStatusActive, Revision: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	for _, binding := range bindings {
		repository.bindings[binding.ID] = binding
	}

	projected, err := ownedAgentConnectorBindingsWithRepository(t.Context(), repository, organizationID, ownerID, target)
	if err != nil {
		t.Fatalf("ownedAgentConnectorBindingsWithRepository() error = %v", err)
	}
	if len(projected) != 2 || projected[0].ConnectorID != "exa" || projected[1].ConnectorID != "github" {
		t.Fatalf("owner projection = %#v, want only native active/needs_replacement", projected)
	}
}

func TestHistoricalAndRollbackConnectorSnapshotFiltersForeignOwner(t *testing.T) {
	db, mock := newAgentConnectorBindingSQLMock(t)
	organizationID, ownerID := uuid.New(), uuid.New()
	ownedConnectionID, foreignConnectionID := uuid.New(), uuid.New()
	agent := &Agent{ID: uuid.New(), TenantID: organizationID}
	service := &agentsService{db: db}
	query := `SELECT connection\.id, connection\.organization_id, connection\.owner_account_id, connection\.connector_id, connection\.state, definition\.status AS definition_status FROM connector_connections AS connection JOIN connector_definitions AS definition ON definition\.connector_id = connection\.connector_id WHERE connection\.id = \$1 AND connection\.organization_id = \$2 AND connection\.owner_account_id = \$3 AND connection\.deleted_at IS NULL.*LIMIT \$4`
	mock.ExpectQuery(query).
		WithArgs(foreignConnectionID, organizationID, ownerID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "owner_account_id", "connector_id", "state", "definition_status"}))
	mock.ExpectQuery(query).
		WithArgs(ownedConnectionID, organizationID, ownerID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "owner_account_id", "connector_id", "state", "definition_status"}).
			AddRow(ownedConnectionID, organizationID, ownerID, "github", connectorcore.ReadinessReady, connectorcore.DefinitionStatusActive))

	filtered, err := service.filterOwnedAgentConnectorSnapshotBindings(t.Context(), db, agent, ownerID.String(), []dto.AgentConnectorBinding{
		{ConnectorID: "github", ConnectionID: ownedConnectionID.String()},
		{ConnectorID: "exa", ConnectionID: foreignConnectionID.String()},
	})
	if err != nil {
		t.Fatalf("filterOwnedAgentConnectorSnapshotBindings() error = %v", err)
	}
	if len(filtered) != 1 || filtered[0] != (dto.AgentConnectorBinding{ConnectorID: "github", ConnectionID: ownedConnectionID.String()}) {
		t.Fatalf("historical owner projection = %#v", filtered)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("historical authorization SQL expectations: %v", err)
	}
}

func TestSyncAgentConnectorTargetBindingsJoinsCallerTransactionRollback(t *testing.T) {
	db, mock := newAgentConnectorBindingSQLMock(t)
	organizationID, ownerID, connectionID := uuid.New(), uuid.New(), uuid.New()
	service := &agentsService{db: db}
	agent := &Agent{ID: uuid.New(), TenantID: organizationID}
	wantRollback := errors.New("force host save rollback")
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "connector_target_bindings" WHERE .*organization_id = \$1 AND owner_account_id = \$2 AND target_type = \$3 AND target_id = \$4.*target_scope = \$5 AND target_version_id = \$6.*ORDER BY`).
		WithArgs(organizationID, ownerID, string(connectorcore.TargetTypeAgent), agent.ID.String(), connectorcore.ScopeDraft, "").
		WillReturnRows(sqlmock.NewRows(agentConnectorBindingColumns()))
	mock.ExpectExec(`SAVEPOINT sp[0-9]+`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT "id" FROM "connector_connections" WHERE id = \$1 AND organization_id = \$2 AND owner_account_id = \$3 AND connector_id = \$4 AND state = \$5 AND deleted_at IS NULL.*LIMIT \$6`).
		WithArgs(connectionID, organizationID, ownerID, "github", string(connectorcore.ReadinessReady), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(connectionID))
	mock.ExpectExec(`INSERT INTO "connector_target_bindings".*ON CONFLICT DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	err := db.Transaction(func(tx *gorm.DB) error {
		if _, syncErr := service.syncAgentConnectorTargetBindings(t.Context(), tx, agent, ownerID.String(), []dto.AgentConnectorBinding{{
			ConnectorID: "github", ConnectionID: connectionID.String(),
		}}); syncErr != nil {
			return syncErr
		}
		return wantRollback
	})
	if !errors.Is(err, wantRollback) {
		t.Fatalf("transaction error = %v, want rollback sentinel", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("transaction SQL expectations: %v", err)
	}
}

func syncAgentConnectorBindingsWithRepository(
	t *testing.T,
	repository connectorbindings.Repository,
	organizationID, ownerID uuid.UUID,
	target connectorcore.TargetRef,
	desired []dto.AgentConnectorBinding,
) []connectorcore.TargetBinding {
	t.Helper()
	result, err := syncAgentConnectorTargetBindingsWithRepository(t.Context(), repository, organizationID, ownerID, target, desired)
	if err != nil {
		t.Fatalf("syncAgentConnectorTargetBindingsWithRepository() error = %v", err)
	}
	return result
}

type memoryAgentConnectorConnection struct {
	organizationID uuid.UUID
	ownerID        uuid.UUID
	connectorID    string
}

type memoryAgentConnectorBindingRepository struct {
	bindings    map[uuid.UUID]connectorcore.TargetBinding
	connections map[uuid.UUID]memoryAgentConnectorConnection
}

func newMemoryAgentConnectorBindingRepository() *memoryAgentConnectorBindingRepository {
	return &memoryAgentConnectorBindingRepository{
		bindings:    map[uuid.UUID]connectorcore.TargetBinding{},
		connections: map[uuid.UUID]memoryAgentConnectorConnection{},
	}
}

func (repository *memoryAgentConnectorBindingRepository) UpsertBinding(
	_ context.Context,
	organizationID, ownerID uuid.UUID,
	binding connectorcore.TargetBinding,
	expectedRevision int,
) (connectorcore.TargetBinding, bool, error) {
	connection, ok := repository.connections[binding.ConnectionID]
	if !ok || connection.organizationID != organizationID || connection.ownerID != ownerID || connection.connectorID != binding.ConnectorID.String() {
		return connectorcore.TargetBinding{}, false, connectorcore.NewError(connectorcore.ErrorCodeConnectionNotReady, false, nil)
	}
	var existing *connectorcore.TargetBinding
	for id, candidate := range repository.bindings {
		if candidate.OrganizationID == organizationID && candidate.OwnerAccountID == ownerID && candidate.Target == binding.Target && candidate.ConnectorID == binding.ConnectorID {
			copy := candidate
			existing = &copy
			_ = id
			break
		}
	}
	if existing == nil {
		if expectedRevision != 0 {
			return connectorcore.TargetBinding{}, false, connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
		}
		repository.bindings[binding.ID] = binding
		return binding, true, nil
	}
	if existing.ConnectionID == binding.ConnectionID && existing.Status == binding.Status {
		return *existing, false, nil
	}
	if expectedRevision < 1 || existing.Revision != expectedRevision {
		return connectorcore.TargetBinding{}, false, connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	updated := *existing
	updated.ConnectionID = binding.ConnectionID
	updated.Status = binding.Status
	updated.DisabledAt = nil
	updated.Revision++
	updated.UpdatedAt = time.Now().UTC()
	repository.bindings[updated.ID] = updated
	return updated, false, nil
}

func (repository *memoryAgentConnectorBindingRepository) GetBinding(
	_ context.Context, organizationID, ownerID, bindingID uuid.UUID,
) (connectorcore.TargetBinding, error) {
	binding, ok := repository.bindings[bindingID]
	if !ok || binding.OrganizationID != organizationID || binding.OwnerAccountID != ownerID {
		return connectorcore.TargetBinding{}, connectorcore.NewError(connectorcore.ErrorCodeNotFound, false, nil)
	}
	return binding, nil
}

func (repository *memoryAgentConnectorBindingRepository) ListBindingsByTarget(
	_ context.Context,
	organizationID, ownerID uuid.UUID,
	targetType connectorcore.TargetType,
	targetID string,
	options connectorbindings.ListOptions,
) ([]connectorcore.TargetBinding, error) {
	result := make([]connectorcore.TargetBinding, 0)
	for _, binding := range repository.bindings {
		if binding.OrganizationID == organizationID && binding.OwnerAccountID == ownerID && binding.Target.Type == targetType && binding.Target.ID == targetID &&
			containsAgentConnectorBindingStatus(options.Statuses, binding.Status) {
			result = append(result, binding)
		}
	}
	return result, nil
}

func containsAgentConnectorBindingStatus(options []connectorcore.BindingStatus, value connectorcore.BindingStatus) bool {
	if len(options) == 0 {
		return true
	}
	for _, option := range options {
		if option == value {
			return true
		}
	}
	return false
}

func (repository *memoryAgentConnectorBindingRepository) ListBindingsByConnection(
	_ context.Context, organizationID, ownerID, connectionID uuid.UUID, _ connectorbindings.ListOptions,
) ([]connectorcore.TargetBinding, error) {
	result := make([]connectorcore.TargetBinding, 0)
	for _, binding := range repository.bindings {
		if binding.OrganizationID == organizationID && binding.OwnerAccountID == ownerID && binding.ConnectionID == connectionID {
			result = append(result, binding)
		}
	}
	return result, nil
}

func (repository *memoryAgentConnectorBindingRepository) ListBindingsByConnector(
	_ context.Context, organizationID, ownerID uuid.UUID, connectorID connectorcore.Identifier, _ connectorbindings.ListOptions,
) ([]connectorcore.TargetBinding, error) {
	result := make([]connectorcore.TargetBinding, 0)
	for _, binding := range repository.bindings {
		if binding.OrganizationID == organizationID && binding.OwnerAccountID == ownerID && binding.ConnectorID == connectorID {
			result = append(result, binding)
		}
	}
	return result, nil
}

func (repository *memoryAgentConnectorBindingRepository) SetBindingStatus(
	_ context.Context,
	organizationID, ownerID, bindingID uuid.UUID,
	status connectorcore.BindingStatus,
	expectedRevision int,
) (connectorcore.TargetBinding, error) {
	binding, ok := repository.bindings[bindingID]
	if !ok || binding.OrganizationID != organizationID || binding.OwnerAccountID != ownerID {
		return connectorcore.TargetBinding{}, connectorcore.NewError(connectorcore.ErrorCodeNotFound, false, nil)
	}
	if binding.Revision != expectedRevision {
		return connectorcore.TargetBinding{}, connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	if binding.Status == status {
		return binding, nil
	}
	binding.Status = status
	binding.Revision++
	binding.UpdatedAt = time.Now().UTC()
	if status == connectorcore.BindingStatusActive {
		binding.DisabledAt = nil
	} else {
		disabledAt := binding.UpdatedAt
		binding.DisabledAt = &disabledAt
	}
	repository.bindings[bindingID] = binding
	return binding, nil
}

func newAgentConnectorBindingSQLMock(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	return db, mock
}

func agentConnectorBindingColumns() []string {
	return []string{
		"id", "organization_id", "owner_account_id", "connection_id", "connector_id",
		"target_type", "target_id", "binding_instance_id", "status",
		"revision", "disabled_at", "created_at", "updated_at",
	}
}
