package bindings

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
)

func TestInitialChatConnectionsValidateBeforePersistence(t *testing.T) {
	valid := InitialChatConnection{ConnectorID: "github", ConnectionID: uuid.New()}
	for _, test := range []struct {
		name       string
		selections []InitialChatConnection
		valid      bool
	}{
		{"empty", nil, true},
		{"single", []InitialChatConnection{valid}, true},
		{"duplicate connector", []InitialChatConnection{valid, valid}, false},
		{"invalid connector", []InitialChatConnection{{ConnectorID: "not valid", ConnectionID: uuid.New()}}, false},
		{"nil connection", []InitialChatConnection{{ConnectorID: "github"}}, false},
		{"bounded input", make([]InitialChatConnection, MaxInitialChatConnections+1), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateInitialChatConnections(test.selections)
			if (err == nil) != test.valid {
				t.Fatalf("validation = %v, valid = %v", err, test.valid)
			}
		})
	}
}

// Every expected read is owner/organization/provider scoped; no credential
// column is read while converting the draft into an authoritative binding.
func expectInitialConnectionMetadata(mock sqlmock.Sqlmock, b core.TargetBinding, operations, constraint string) {
	expectBindingLifecycleAuthorities(mock, []bindingAuthorityExpectation{{binding: b}})
	mock.ExpectQuery(`SELECT connection_constraint, usable_operation_ids FROM "connector_connections" WHERE id = \$1 AND organization_id = \$2 AND owner_account_id = \$3 AND connector_id = \$4 AND state = \$5 AND deleted_at IS NULL.*LIMIT \$6`).
		WithArgs(b.ConnectionID, b.OrganizationID, b.OwnerAccountID, b.ConnectorID.String(), string(core.ReadinessReady), 1).
		WillReturnRows(sqlmock.NewRows([]string{"connection_constraint", "usable_operation_ids"}).AddRow(constraint, operations))
}

func expectInitialConnectionSaved(mock sqlmock.Sqlmock, b core.TargetBinding, auditErr error) {
	constraint, _ := json.Marshal(core.OwnerConstraint(b.OwnerAccountID, []core.Identifier{"read"}))
	expectInitialConnectionMetadata(mock, b, `["read"]`, string(constraint))
	expectLogicalBindingUnlocked(mock, b, bindingRows())
	expectBindingLifecycleAuthorities(mock, []bindingAuthorityExpectation{{binding: b}})
	expectLogicalBinding(mock, b, bindingRows())
	expectReadyConnection(mock, b)
	mock.ExpectExec(`INSERT INTO "connector_target_bindings".*ON CONFLICT DO NOTHING`).WillReturnResult(sqlmock.NewResult(0, 1))
	audit := mock.ExpectExec(`INSERT INTO "connector_lifecycle_decisions".*ON CONFLICT DO NOTHING`)
	if auditErr != nil {
		audit.WillReturnError(auditErr)
	} else {
		audit.WillReturnResult(sqlmock.NewResult(0, 1))
	}
}

func TestInitialChatConnectionsCommitAllBindingsAndAuditsOrRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "commit"
		if fail {
			name = "rollback second audit"
		}
		t.Run(name, func(t *testing.T) {
			repo, mock := openRepositoryMock(t)
			chatID := uuid.New()
			first := nativeBinding(core.TargetTypeChat, chatID.String(), "")
			first.ConnectionID = uuid.MustParse("10000000-0000-4000-8000-000000000001")
			second := first
			second.ConnectorID = "exa"
			second.ConnectionID = uuid.MustParse("20000000-0000-4000-8000-000000000002")
			mock.ExpectBegin()
			expectInitialConnectionSaved(mock, first, nil)
			var auditErr error
			if fail {
				auditErr = errors.New("unavailable")
			}
			expectInitialConnectionSaved(mock, second, auditErr)
			if fail {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			err := repo.db.Transaction(func(tx *gorm.DB) error {
				// Reversed input must still acquire connection locks in stable order.
				return InitializeChatConnections(context.Background(), tx, first.OrganizationID, first.OwnerAccountID, chatID, []InitialChatConnection{
					{ConnectorID: second.ConnectorID, ConnectionID: second.ConnectionID},
					{ConnectorID: first.ConnectorID, ConnectionID: first.ConnectionID},
				})
			})
			if (err != nil) != fail {
				t.Fatalf("initialize = %v, expectations = %v", err, mock.ExpectationsWereMet())
			}
			assertMock(t, mock)
		})
	}
}

func TestInitialChatConnectionsRejectUnusableMetadata(t *testing.T) {
	for _, input := range []struct{ name, operations, constraint string }{
		{"no usable operations", `[]`, `{}`},
		{"malformed operations", `{}`, `{}`},
		{"unsupported constraint", `["read"]`, `{"schema_version":99}`},
	} {
		t.Run(input.name, func(t *testing.T) {
			repo, mock := openRepositoryMock(t)
			chatID := uuid.New()
			b := nativeBinding(core.TargetTypeChat, chatID.String(), "")
			mock.ExpectBegin()
			expectInitialConnectionMetadata(mock, b, input.operations, input.constraint)
			mock.ExpectRollback()
			err := repo.db.Transaction(func(tx *gorm.DB) error {
				return InitializeChatConnections(context.Background(), tx, b.OrganizationID, b.OwnerAccountID, chatID,
					[]InitialChatConnection{{ConnectorID: b.ConnectorID, ConnectionID: b.ConnectionID}})
			})
			if err == nil {
				t.Fatal("invalid metadata allowed binding creation")
			}
			assertMock(t, mock)
		})
	}
}

func TestInitialChatConnectionsRejectUnavailableOrForeignConnection(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		name := "not ready or deleted"
		if foreign {
			name = "owner organization or provider mismatch"
		}
		t.Run(name, func(t *testing.T) {
			repo, mock := openRepositoryMock(t)
			chatID := uuid.New()
			b := nativeBinding(core.TargetTypeChat, chatID.String(), "")
			mock.ExpectBegin()
			if foreign {
				mock.ExpectQuery(`SELECT "id" FROM "connector_connections" WHERE id = \$1 AND organization_id = \$2 AND owner_account_id = \$3 AND connector_id = \$4.*LIMIT \$5 FOR UPDATE`).
					WithArgs(b.ConnectionID, b.OrganizationID, b.OwnerAccountID, b.ConnectorID.String(), 1).
					WillReturnRows(sqlmock.NewRows([]string{"id"}))
			} else {
				expectBindingLifecycleAuthorities(mock, []bindingAuthorityExpectation{{binding: b}})
				mock.ExpectQuery(`SELECT connection_constraint, usable_operation_ids FROM "connector_connections" WHERE id = \$1 AND organization_id = \$2 AND owner_account_id = \$3 AND connector_id = \$4 AND state = \$5 AND deleted_at IS NULL.*LIMIT \$6`).
					WithArgs(b.ConnectionID, b.OrganizationID, b.OwnerAccountID, b.ConnectorID.String(), string(core.ReadinessReady), 1).
					WillReturnRows(sqlmock.NewRows([]string{"connection_constraint", "usable_operation_ids"}))
			}
			mock.ExpectRollback()
			err := repo.db.Transaction(func(tx *gorm.DB) error {
				return InitializeChatConnections(context.Background(), tx, b.OrganizationID, b.OwnerAccountID, chatID,
					[]InitialChatConnection{{ConnectorID: b.ConnectorID, ConnectionID: b.ConnectionID}})
			})
			if err == nil {
				t.Fatal("unavailable connection allowed binding creation")
			}
			assertMock(t, mock)
		})
	}
}
