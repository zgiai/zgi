package runtimeauth_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestInitialChatAvailabilityRechecksCatalogAndOperationConstraints(t *testing.T) {
	for _, test := range []struct {
		name      string
		wantReady bool
	}{
		{"ready", true},
		{"plugin disabled", false},
		{"chat surface removed", false},
		{"zero usable operations", false},
		{"all usable operations unavailable", false},
		{"operation removed from catalog", false},
		{"binding constraint denies", false},
		{"connection constraint denies", false},
		{"driver changed", false},
		{"non-ready", false},
		{"malformed constraint", false},
		{"second connection missing", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newChatResolverHarness(t)
			definition := harness.definition
			definitions := definitionMap{definition.ID: definition}
			if test.name == "plugin disabled" {
				delete(definitions, definition.ID)
			}
			if test.name == "chat surface removed" {
				definition.Surfaces = []catalog.Surface{catalog.SurfaceAgent}
				definitions[definition.ID] = definition
			}
			resolver, err := runtimeauth.NewChatResolver(harness.bindings, harness.connections, definitions)
			if err != nil {
				t.Fatal(err)
			}
			operationID := definition.Operations[0].ID
			operations := []core.Identifier{core.Identifier(operationID)}
			if test.name == "zero usable operations" {
				operations = []core.Identifier{}
			}
			if test.name == "operation removed from catalog" {
				operations = []core.Identifier{"removed_operation"}
			}
			unavailable := map[string]string{}
			if test.name == "all usable operations unavailable" {
				unavailable[operationID] = "provider_scope_missing"
			}
			bindingConstraint := core.OwnerConstraint(harness.ownerID, operations)
			connectionConstraint := core.OwnerConstraint(harness.ownerID, operations)
			if test.name == "binding constraint denies" {
				bindingConstraint.DenyReasons = []string{"disabled"}
			}
			if test.name == "connection constraint denies" {
				connectionConstraint.DenyReasons = []string{"disabled"}
			}
			bindingJSON, _ := json.Marshal(bindingConstraint)
			connectionJSON, _ := json.Marshal(connectionConstraint)
			if test.name == "malformed constraint" {
				connectionJSON = []byte(`{}`)
			}
			operationJSON, _ := json.Marshal(operations)
			unavailableJSON, _ := json.Marshal(unavailable)
			driverID := definition.DriverID
			if test.name == "driver changed" {
				driverID = "different_driver"
			}
			state := core.ReadinessReady
			if test.name == "non-ready" {
				state = core.ReadinessReconnectRequired
			}
			selections := []bindings.InitialChatConnection{{ConnectorID: core.Identifier(definition.ID), ConnectionID: harness.connection.ID}}
			if test.name == "second connection missing" {
				selections = append(selections, bindings.InitialChatConnection{ConnectorID: "web-search", ConnectionID: uuid.New()})
			}
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			mock.ExpectBegin()
			args := []driver.Value{harness.organizationID, harness.ownerID, "chat", harness.conversationID, "live", "", "active"}
			for _, selection := range selections {
				args = append(args, selection.ConnectionID)
			}
			// The projection must remain credential-free (no SELECT * or encrypted_credentials).
			mock.ExpectQuery(`SELECT b.connection_id, b.connector_id, b.binding_constraint, c.connection_constraint, c.state, c.driver_id, c.auth_method_id, c.usable_operation_ids, c.unavailable_operations FROM connector_target_bindings AS b JOIN connector_connections AS c ON.*`).
				WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"connection_id", "connector_id", "binding_constraint", "connection_constraint", "state", "driver_id", "auth_method_id", "usable_operation_ids", "unavailable_operations"}).
				AddRow(harness.connection.ID, definition.ID, string(bindingJSON), string(connectionJSON), state, driverID, definition.AuthMethodID, string(operationJSON), string(unavailableJSON)))
			if test.wantReady {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			err = db.Transaction(func(tx *gorm.DB) error {
				return resolver.ValidateInitialChatConnections(context.Background(), tx, runtimeauth.ChatCapabilityRequest{
					OrganizationID: harness.organizationID, OwnerAccountID: harness.ownerID, ConversationID: harness.conversationID,
				}, selections)
			})
			if test.wantReady && err != nil {
				t.Fatal(err)
			}
			if !test.wantReady && !core.IsErrorCode(err, core.ErrorCodeConnectionNotReady) {
				t.Fatalf("unavailable selection did not fail closed: %v", err)
			}
			if harness.connections.calls != 0 || harness.bindings.calls != 0 {
				t.Fatal("initial validation escaped the host transaction")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
