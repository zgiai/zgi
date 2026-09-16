package connections

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestUpdateConnectionLifecycleCommitsCanonicalAndAuditTogether(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	connection := readyConnection(uuid.New(), uuid.New(), uuid.New())
	connection.Name = "Renamed GitHub"
	createdAt := time.Date(2026, 9, 2, 2, 3, 4, 0, time.UTC)
	mutation := newConnectionLifecycleMutation(
		MutationConnectionRenamed,
		"connection-rename:"+connection.ID.String()+":1",
		createdAt,
		ConnectionChange{Type: MutationConnectionRenamed, Name: connection.Name},
	)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "connector_connections" SET .* WHERE organization_id = \$[0-9]+ AND owner_account_id = \$[0-9]+ AND id = \$[0-9]+ AND connector_id = \$[0-9]+ AND provider_id = \$[0-9]+ AND driver_id = \$[0-9]+ AND auth_method_id = \$[0-9]+ AND revision = \$[0-9]+ AND deleted_at IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectConnectionLifecycleDecision(mock, connection, mutation)
	mock.ExpectCommit()

	stored, err := repository.UpdateConnectionWithLifecycle(
		context.Background(), connection.OrganizationID, connection.OwnerAccountID, connection, connection.Revision, mutation,
	)
	if err != nil || stored.Name != connection.Name || stored.Revision != connection.Revision+1 {
		t.Fatalf("UpdateConnectionWithLifecycle() = %#v, %v", stored, err)
	}
	assertMock(t, mock)
}

func TestUpdateConnectionLifecycleRollsBackCanonicalWriteWhenAuditFails(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	connection := readyConnection(uuid.New(), uuid.New(), uuid.New())
	connection.Name = "Renamed GitHub"
	mutation := newConnectionLifecycleMutation(
		MutationConnectionRenamed,
		"connection-rename:"+connection.ID.String()+":1",
		time.Date(2026, 9, 2, 3, 4, 5, 0, time.UTC),
		ConnectionChange{Type: MutationConnectionRenamed, Name: connection.Name},
	)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "connector_connections" SET .* WHERE organization_id = \$[0-9]+ AND owner_account_id = \$[0-9]+ AND id = \$[0-9]+ AND connector_id = \$[0-9]+ AND provider_id = \$[0-9]+ AND driver_id = \$[0-9]+ AND auth_method_id = \$[0-9]+ AND revision = \$[0-9]+ AND deleted_at IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "connector_lifecycle_decisions"`).WillReturnError(errors.New("audit unavailable"))
	mock.ExpectRollback()

	_, err := repository.UpdateConnectionWithLifecycle(
		context.Background(), connection.OrganizationID, connection.OwnerAccountID, connection, connection.Revision, mutation,
	)
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeInternal) {
		t.Fatalf("UpdateConnectionWithLifecycle() error = %v, want internal", err)
	}
	assertMock(t, mock)
}

func TestConnectionLifecycleAuditReplayRejectsDifferentContentForSameCorrelation(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	createdAt := time.Date(2026, 9, 2, 9, 10, 11, 123456000, time.UTC)
	desired := connectionLifecycleDecisionRecord{
		ID: uuid.New(), OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ActingPrincipalID: uuid.New(),
		ConnectorID: "github", ConnectionID: uuid.New(), Decision: MutationConnectionRenamed,
		Status: connectionLifecycleSuccessful, CorrelationID: "connection-audit-replay-conflict",
		Details: datatypes.JSON(`{"name":"expected"}`), CreatedAt: createdAt,
	}
	existing := desired
	existing.ID = uuid.New()
	existing.Details = datatypes.JSON(`{"name":"different"}`)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "connector_lifecycle_decisions".*ON CONFLICT DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT \* FROM "connector_lifecycle_decisions" WHERE organization_id = \$1 AND correlation_id = \$2 AND decision = \$3 AND status = \$4.*LIMIT \$5`).
		WithArgs(desired.OrganizationID, desired.CorrelationID, desired.Decision, desired.Status, 1).
		WillReturnRows(connectionLifecycleDecisionRows(existing))
	mock.ExpectRollback()

	err := repository.db.Transaction(func(tx *gorm.DB) error {
		return createOrVerifyConnectionLifecycleDecision(context.Background(), tx, desired)
	})
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConflict) {
		t.Fatalf("createOrVerifyConnectionLifecycleDecision() error = %v, want conflict", err)
	}
	assertMock(t, mock)
}

func TestReconnectLifecycleRollsBackCredentialSwapAndAttemptWhenAuditFails(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	existing := readyConnection(uuid.New(), uuid.New(), uuid.New())
	attempt := verifyingAttempt(existing.OrganizationID, existing.OwnerAccountID, existing.ID)
	attempt.ConnectionID = existing.ID
	attempt.ConnectorID = existing.ConnectorID
	result := existing
	result.CredentialVersion++
	result.EncryptedCredentials = "v2.connector.NEW-PROTECTED-ENVELOPE"
	mutation := newConnectionLifecycleMutation(
		DecisionConnectionReconnected, "connection-reconnect:"+attempt.ID.String(), time.Now().UTC(),
		ConnectionChange{
			Type: MutationCredentialRotated, CredentialVersion: result.CredentialVersion,
		},
		ConnectionChange{Type: MutationReadinessChanged, Readiness: result.State},
	)

	mock.ExpectBegin()
	stored := result
	stored.Revision = existing.Revision + 1
	mock.ExpectQuery(`SELECT .* FROM "connector_key_setup_attempts" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3.*FOR UPDATE`).
		WithArgs(existing.OrganizationID, existing.OwnerAccountID, attempt.ID, 1).
		WillReturnRows(setupAttemptRows(attempt))
	mock.ExpectQuery(`SELECT .* FROM "connector_connections" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3 AND deleted_at IS NULL.*FOR UPDATE`).
		WithArgs(existing.OrganizationID, existing.OwnerAccountID, existing.ID, 1).
		WillReturnRows(connectionRows(existing))
	mock.ExpectExec(`UPDATE "connector_connections" SET .* WHERE organization_id = \$[0-9]+ AND owner_account_id = \$[0-9]+ AND id = \$[0-9]+ AND revision = \$[0-9]+ AND deleted_at IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "connector_key_setup_attempts" SET .* WHERE organization_id = \$[0-9]+ AND owner_account_id = \$[0-9]+ AND id = \$[0-9]+ AND state = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "connector_lifecycle_decisions"`).WillReturnError(errors.New("audit unavailable"))
	mock.ExpectRollback()

	_, err := repository.CompleteReconnectReadyWithLifecycle(
		context.Background(), existing.OrganizationID, existing.OwnerAccountID, attempt.ID,
		ReconnectCompletion{
			ExistingConnectionID: existing.ID, ExpectedRevision: existing.Revision,
			ReplaceExisting: true, Result: result,
		},
		mutation,
	)
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeInternal) {
		t.Fatalf("CompleteReconnectReadyWithLifecycle() error = %v, want internal", err)
	}
	assertMock(t, mock)
}

func TestRemovalLifecycleRollsBackConnectionTombstoneWhenAuditFails(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	connection := readyConnection(uuid.New(), uuid.New(), uuid.New())
	mutation := newConnectionLifecycleMutation(
		MutationConnectionRemoved, "connection-remove:"+connection.ID.String()+":1", time.Now().UTC(),
		ConnectionChange{Type: MutationConnectionRemoved},
	)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "connector_connections" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3 AND deleted_at IS NULL.*LIMIT \$4`).
		WithArgs(connection.OrganizationID, connection.OwnerAccountID, connection.ID, 1).
		WillReturnRows(connectionRows(connection))
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "connector_connections" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3 AND deleted_at IS NULL.*FOR UPDATE`).
		WithArgs(connection.OrganizationID, connection.OwnerAccountID, connection.ID, 1).
		WillReturnRows(connectionRows(connection))
	mock.ExpectQuery(`SELECT .* FROM "connector_target_bindings" WHERE organization_id = \$1 AND owner_account_id = \$2 AND connection_id = \$3 AND status = \$4 ORDER BY id ASC FOR UPDATE`).
		WithArgs(connection.OrganizationID, connection.OwnerAccountID, connection.ID, connectorcore.BindingStatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"id", "revision"}))
	mock.ExpectExec(`UPDATE "connector_connections" SET .* WHERE organization_id = \$[0-9]+ AND owner_account_id = \$[0-9]+ AND id = \$[0-9]+ AND revision = \$[0-9]+ AND deleted_at IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	removed := connection
	mock.ExpectQuery(`SELECT .* FROM "connector_chat_preferences" WHERE organization_id = \$1 AND owner_account_id = \$2 LIMIT \$3`).
		WithArgs(connection.OrganizationID, connection.OwnerAccountID, 1).WillReturnRows(sqlmock.NewRows([]string{"organization_id", "owner_account_id", "revision", "defaults"}).AddRow(connection.OrganizationID, connection.OwnerAccountID, 1, `{"github":{"mode":"selected","connection_id":"`+connection.ID.String()+`"}}`))
	mock.ExpectExec(`UPDATE "connector_chat_preferences"`).WithArgs(`{"github":{"mode":"none"}}`, int64(2), sqlmock.AnyArg(), connection.OrganizationID, connection.OwnerAccountID, int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	removed.State = connectorcore.ReadinessUnavailable
	removed.Revision++
	removed.DeletedAt = &mutation.CreatedAt
	mock.ExpectExec(`INSERT INTO "connector_lifecycle_decisions"`).WillReturnError(errors.New("audit unavailable"))
	mock.ExpectRollback()

	err := repository.ForceRemoveConnectionWithLifecycle(
		context.Background(), connection.OrganizationID, connection.OwnerAccountID, connection.ID,
		connection.Revision, nil, mutation,
	)
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeInternal) {
		t.Fatalf("ForceRemoveConnectionWithLifecycle() error = %v, want internal", err)
	}
	assertMock(t, mock)
}

func expectConnectionLifecycleDecision(
	mock sqlmock.Sqlmock,
	connection connectorcore.PersonalConnection,
	mutation ConnectionLifecycleMutation,
) {
	mock.ExpectExec(`INSERT INTO "connector_lifecycle_decisions"`).WithArgs(
		mutation.ID,
		connection.OrganizationID,
		connection.OwnerAccountID,
		connection.OwnerAccountID,
		connection.ConnectorID.String(),
		connection.ID,
		nil,
		nil,
		nil,
		nil,
		nil,
		mutation.Decision,
		connectionLifecycleSuccessful,
		mutation.CorrelationID,
		nil,
		sqlmock.AnyArg(),
		mutation.CreatedAt.UTC(),
	).WillReturnResult(sqlmock.NewResult(0, 1))
}

func connectionLifecycleDecisionRows(records ...connectionLifecycleDecisionRecord) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{
		"id", "organization_id", "owner_account_id", "acting_principal_id", "connector_id", "connection_id",
		"target_type", "target_id", "binding_instance_id", "operation_id", "logical_operation_id", "decision",
		"status", "correlation_id", "receipt_id", "details", "created_at",
	})
	for _, record := range records {
		rows.AddRow(
			record.ID, record.OrganizationID, record.OwnerAccountID, record.ActingPrincipalID, record.ConnectorID, record.ConnectionID,
			record.TargetType, record.TargetID, record.BindingInstanceID, record.OperationID, record.LogicalOperationID, record.Decision,
			record.Status, record.CorrelationID, record.ReceiptID, record.Details, record.CreatedAt,
		)
	}
	return rows
}
