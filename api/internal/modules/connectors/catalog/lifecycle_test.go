package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	connectoraudit "github.com/zgiai/zgi/api/internal/modules/connectors/audit"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPluginLifecyclePinsImmutableVersionAcrossCompatibleUpdate(t *testing.T) {
	t.Parallel()
	current := pluginContractDefinitionFixture()
	lifecycle, err := NewPluginLifecycle([]Definition{current}, nil)
	if err != nil {
		t.Fatal(err)
	}
	oldPin, err := lifecycle.Pin(current.ID, current.Operations[0].ID)
	if err != nil {
		t.Fatal(err)
	}

	candidate := cloneDefinition(current)
	candidate.PluginVersion = "1.1.0"
	candidate.PluginDigest = strings.Repeat("b", 64)
	candidate.Name = "Updated display"
	candidate.Operations = append(candidate.Operations, additiveOperationFixture())
	if err := lifecycle.Activate(candidate); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	newPin, err := lifecycle.Pin(candidate.ID, candidate.Operations[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if oldPin.PluginVersion != "1.0.0" || oldPin.PluginDigest != strings.Repeat("a", 64) ||
		newPin.PluginVersion != "1.1.0" || newPin.PluginDigest != strings.Repeat("b", 64) {
		t.Fatalf("old/new pins = %#v / %#v", oldPin, newPin)
	}
	if oldPin.ContractDigest == "" || newPin.ContractDigest == "" || oldPin.ContractDigest == newPin.ContractDigest {
		t.Fatalf("old/new contract digests = %q / %q", oldPin.ContractDigest, newPin.ContractDigest)
	}

	// Mutating a caller-owned pin cannot rewrite either the active version or a
	// previously pinned operation snapshot.
	oldPin.Operation.InputSchema["mutated"] = true
	again, err := lifecycle.Pin(current.ID, current.Operations[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, leaked := again.Operation.InputSchema["mutated"]; leaked {
		t.Fatal("caller mutation leaked into the active version")
	}
}

func TestPluginLifecycleBlocksByDefaultAndForcedDisablePreservesOldPin(t *testing.T) {
	t.Parallel()
	definition := pluginContractDefinitionFixture()
	connectionID := uuid.New()
	bindingID := uuid.New()
	store := &pluginLifecycleStoreSpy{impact: PluginImpact{
		PluginID:    definition.PluginID,
		Definitions: []DefinitionImpact{{ConnectorID: definition.ID, Status: PluginDefinitionStatusActive}},
		Connections: []ConnectionImpact{{ID: connectionID, ConnectorID: definition.ID, State: "ready"}},
		Bindings:    []BindingImpact{{ID: bindingID, ConnectionID: connectionID, ConnectorID: definition.ID, Status: "active"}},
	}}
	lifecycle, err := NewPluginLifecycle([]Definition{definition}, store)
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := lifecycle.Pin(definition.ID, definition.Operations[0].ID)
	if err != nil {
		t.Fatal(err)
	}

	impact, err := lifecycle.Disable(context.Background(), definition.PluginID, false)
	if !errors.Is(err, ErrPluginLifecycleBlocked) || len(impact.Connections) != 1 || len(impact.Bindings) != 1 {
		t.Fatalf("Disable(default) = %#v, %v", impact, err)
	}
	if _, err := lifecycle.Pin(definition.ID, definition.Operations[0].ID); err != nil {
		t.Fatalf("blocked transition changed active definition: %v", err)
	}

	impact, err = lifecycle.Disable(context.Background(), definition.PluginID, true)
	if err != nil || len(impact.Connections) != 1 || store.lastStatus != PluginDefinitionStatusDisabled || !store.lastForce {
		t.Fatalf("Disable(force) = %#v, %v, store = %#v", impact, err, store)
	}
	if _, err := lifecycle.Pin(definition.ID, definition.Operations[0].ID); !errors.Is(err, ErrPluginDefinitionInactive) {
		t.Fatalf("Pin(after disable) error = %v", err)
	}
	if pinned.PluginVersion != definition.PluginVersion || pinned.Operation.ID != definition.Operations[0].ID {
		t.Fatalf("in-flight pin switched after disable: %#v", pinned)
	}
}

func TestPluginLifecycleRemoveMarksUnavailableWithoutDeletingDefinitions(t *testing.T) {
	t.Parallel()
	definition := pluginContractDefinitionFixture()
	store := &pluginLifecycleStoreSpy{impact: PluginImpact{
		PluginID:    definition.PluginID,
		Definitions: []DefinitionImpact{{ConnectorID: definition.ID, Status: PluginDefinitionStatusActive}},
	}}
	lifecycle, err := NewPluginLifecycle([]Definition{definition}, store)
	if err != nil {
		t.Fatal(err)
	}
	impact, err := lifecycle.Remove(context.Background(), definition.PluginID, false)
	if err != nil || len(impact.Definitions) != 1 || store.lastStatus != PluginDefinitionStatusUnavailable {
		t.Fatalf("Remove() = %#v, %v", impact, err)
	}
	if _, err := lifecycle.Pin(definition.ID, definition.Operations[0].ID); !errors.Is(err, ErrPluginDefinitionInactive) {
		t.Fatalf("Pin(after remove) error = %v", err)
	}
}

func TestPluginLifecycleInactivePluginCannotActivateNewConnectorID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		transition func(*PluginLifecycle, string) (PluginImpact, error)
	}{
		{
			name: "disabled",
			transition: func(lifecycle *PluginLifecycle, pluginID string) (PluginImpact, error) {
				return lifecycle.Disable(context.Background(), pluginID, false)
			},
		},
		{
			name: "unavailable",
			transition: func(lifecycle *PluginLifecycle, pluginID string) (PluginImpact, error) {
				return lifecycle.Remove(context.Background(), pluginID, false)
			},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			current := pluginContractDefinitionFixture()
			lifecycle, err := NewPluginLifecycle([]Definition{current}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := testCase.transition(lifecycle, current.PluginID); err != nil {
				t.Fatal(err)
			}
			candidate := cloneDefinition(current)
			candidate.ID = "second-connector"
			candidate.PluginVersion = "1.1.0"
			candidate.PluginDigest = strings.Repeat("b", 64)
			if err := lifecycle.Activate(candidate); !errors.Is(err, ErrPluginDefinitionInactive) {
				t.Fatalf("Activate(new Connector ID) error = %v", err)
			}
		})
	}
}

func TestPluginLifecycleStoreFailureLeavesActiveVersionAvailable(t *testing.T) {
	t.Parallel()
	definition := pluginContractDefinitionFixture()
	store := &pluginLifecycleStoreSpy{
		impact: PluginImpact{PluginID: definition.PluginID, Definitions: []DefinitionImpact{{ConnectorID: definition.ID}}},
		err:    errors.New("database unavailable"),
	}
	lifecycle, err := NewPluginLifecycle([]Definition{definition}, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycle.Disable(context.Background(), definition.PluginID, true); !errors.Is(err, store.err) {
		t.Fatalf("Disable() error = %v", err)
	}
	if _, err := lifecycle.Pin(definition.ID, definition.Operations[0].ID); err != nil {
		t.Fatalf("failed transition changed the active version: %v", err)
	}
}

func TestGormPluginLifecycleStoreReportsImpactAndForcedDisableRetainsRecords(t *testing.T) {
	organizationID := uuid.New()
	ownerID := uuid.New()
	connectionID := uuid.New()
	bindingID := uuid.New()
	db, mock := openPluginLifecycleStoreMock(t)
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WithArgs("connector-plugin:zgi.example").WillReturnResult(sqlmock.NewResult(0, 1))
	expectPluginImpactQueries(mock, organizationID, ownerID, connectionID, bindingID)
	mock.ExpectRollback()
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WithArgs("connector-plugin:zgi.example").WillReturnResult(sqlmock.NewResult(0, 1))
	expectPluginImpactQueries(mock, organizationID, ownerID, connectionID, bindingID)
	mock.ExpectQuery(`(?s)SELECT plugin_version, plugin_digest FROM "connector_plugin_versions" WHERE plugin_id = \$1 AND status = \$2 ORDER BY accepted_at DESC, plugin_version DESC.*LIMIT \$3`).
		WithArgs("zgi.example", PluginDefinitionStatusActive, 1).
		WillReturnRows(sqlmock.NewRows([]string{"plugin_version", "plugin_digest"}).
			AddRow("1.0.0", strings.Repeat("a", 64)))
	mock.ExpectExec(`UPDATE "connector_definitions" SET .* WHERE plugin_id = \$[0-9]+`).
		WithArgs(PluginDefinitionStatusDisabled, sqlmock.AnyArg(), "zgi.example").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "connector_plugin_versions" SET .* WHERE plugin_id = \$[0-9]+ AND status IN`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "connector_definition_versions" SET .* WHERE plugin_id = \$[0-9]+ AND status IN`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "connector_connections" SET .* WHERE connector_id IN \(SELECT .*connector_id.* FROM "connector_definitions" WHERE plugin_id = \$[0-9]+\) AND deleted_at IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "connector_target_bindings" SET .* WHERE connector_id IN \(SELECT .*connector_id.* FROM "connector_definitions" WHERE plugin_id = \$[0-9]+\) AND status <> \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO "connector_platform_lifecycle_events" .*ON CONFLICT DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	store := NewGormPluginLifecycleStore(db)
	ctx := maintainerPluginAuditContext()
	impact, err := store.TransitionPlugin(ctx, "zgi.example", PluginDefinitionStatusDisabled, false)
	if !errors.Is(err, ErrPluginLifecycleBlocked) || len(impact.Connections) != 1 || len(impact.Bindings) != 1 {
		t.Fatalf("TransitionPlugin(default) = %#v, %v", impact, err)
	}
	impact, err = store.TransitionPlugin(ctx, "zgi.example", PluginDefinitionStatusDisabled, true)
	if err != nil || len(impact.Connections) != 1 || len(impact.Bindings) != 1 {
		t.Fatalf("TransitionPlugin(force) = %#v, %v", impact, err)
	}
	// Only UPDATE statements are expected. Accepted plugin and definition
	// versions retain their history while their current entries become inactive.
	// A DELETE of a Connection, binding, or credential envelope would be an
	// unexpected SQL operation and fail the test, while the scanned impact proves
	// every affected item was returned.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGormPluginLifecycleStoreRepeatTransitionIsNoOpAfterSerializedImpactRead(t *testing.T) {
	db, mock := openPluginLifecycleStoreMock(t)
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WithArgs("connector-plugin:zgi.example").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT connector_id, status FROM "connector_definitions" WHERE plugin_id = \$1 ORDER BY connector_id ASC`).
		WithArgs("zgi.example").
		WillReturnRows(sqlmock.NewRows([]string{"connector_id", "status"}).AddRow("example", "disabled"))
	mock.ExpectQuery(`SELECT .* FROM "connector_connections"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "owner_account_id", "connector_id", "state"}))
	mock.ExpectQuery(`SELECT .* FROM "connector_target_bindings"`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "organization_id", "owner_account_id", "connection_id", "connector_id",
			"target_type", "target_id", "binding_instance_id", "status",
		}))
	mock.ExpectCommit()

	impact, err := NewGormPluginLifecycleStore(db).TransitionPlugin(
		maintainerPluginAuditContext(), "zgi.example", PluginDefinitionStatusDisabled, true,
	)
	if err != nil || len(impact.Definitions) != 1 {
		t.Fatalf("TransitionPlugin(replay) = %#v, %v", impact, err)
	}
	// No version read, canonical UPDATE, or audit INSERT is expected: the first
	// serialized transaction already committed the immutable transition event.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGormPluginLifecycleStoreRollsBackStateWhenPlatformAuditFails(t *testing.T) {
	db, mock := openPluginLifecycleStoreMock(t)
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 1))
	expectPluginImpactQueries(mock, uuid.New(), uuid.New(), uuid.New(), uuid.New())
	mock.ExpectQuery(`(?s)SELECT plugin_version, plugin_digest FROM "connector_plugin_versions"`).
		WillReturnRows(sqlmock.NewRows([]string{"plugin_version", "plugin_digest"}).
			AddRow("1.0.0", strings.Repeat("a", 64)))
	mock.ExpectExec(`UPDATE "connector_definitions" SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "connector_plugin_versions" SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "connector_definition_versions" SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "connector_connections" SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "connector_target_bindings" SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO "connector_platform_lifecycle_events" .*ON CONFLICT DO NOTHING`).
		WillReturnError(errors.New("forced plugin lifecycle audit failure"))
	mock.ExpectRollback()

	if _, err := NewGormPluginLifecycleStore(db).TransitionPlugin(
		maintainerPluginAuditContext(), "zgi.example", PluginDefinitionStatusUnavailable, true,
	); err == nil {
		t.Fatal("TransitionPlugin() succeeded without platform lifecycle audit")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func maintainerPluginAuditContext() context.Context {
	return connectoraudit.WithPlatformActor(context.Background(), connectoraudit.PlatformActor{
		Kind: connectoraudit.PlatformActorPlatformMaintainer,
		ID:   uuid.NewString(),
	})
}

func openPluginLifecycleStoreMock(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB, DriverName: "postgres", PreferSimpleProtocol: true,
	}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

func expectPluginImpactQueries(
	mock sqlmock.Sqlmock,
	organizationID uuid.UUID,
	ownerID uuid.UUID,
	connectionID uuid.UUID,
	bindingID uuid.UUID,
) {
	mock.ExpectQuery(`SELECT connector_id, status FROM "connector_definitions" WHERE plugin_id = \$1 ORDER BY connector_id ASC`).
		WithArgs("zgi.example").
		WillReturnRows(sqlmock.NewRows([]string{"connector_id", "status"}).AddRow("example", "active"))
	mock.ExpectQuery(`SELECT .* FROM "connector_connections" WHERE connector_id IN \(SELECT .*connector_id.* FROM "connector_definitions" WHERE plugin_id = \$1\) AND deleted_at IS NULL ORDER BY`).
		WithArgs("zgi.example").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "organization_id", "owner_account_id", "connector_id", "state",
		}).AddRow(connectionID, organizationID, ownerID, "example", "ready"))
	mock.ExpectQuery(`SELECT .* FROM "connector_target_bindings" WHERE connector_id IN \(SELECT .*connector_id.* FROM "connector_definitions" WHERE plugin_id = \$1\) AND status <> \$2 ORDER BY`).
		WithArgs("zgi.example", "disabled").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "organization_id", "owner_account_id", "connection_id", "connector_id",
			"target_type", "target_id", "binding_instance_id", "status",
		}).AddRow(bindingID, organizationID, ownerID, connectionID, "example", "agent", "agent-1", "", "active"))
}

type pluginLifecycleStoreSpy struct {
	impact     PluginImpact
	err        error
	lastStatus PluginDefinitionStatus
	lastForce  bool
}

func (store *pluginLifecycleStoreSpy) InspectPluginImpact(_ context.Context, pluginID string) (PluginImpact, error) {
	if pluginID != store.impact.PluginID {
		return PluginImpact{}, fmt.Errorf("unexpected plugin %s", pluginID)
	}
	return store.impact, store.err
}

func (store *pluginLifecycleStoreSpy) TransitionPlugin(
	_ context.Context,
	pluginID string,
	status PluginDefinitionStatus,
	force bool,
) (PluginImpact, error) {
	store.lastStatus = status
	store.lastForce = force
	if pluginID != store.impact.PluginID {
		return PluginImpact{}, fmt.Errorf("unexpected plugin %s", pluginID)
	}
	if store.err != nil {
		return store.impact, store.err
	}
	if store.impact.HasReferences() && !force {
		return store.impact, ErrPluginLifecycleBlocked
	}
	return store.impact, nil
}
