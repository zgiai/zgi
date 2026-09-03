package connections

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const encryptedCredentialSentinel = "v1.connector.TEST-SECRET-SENTINEL"

func TestGormRepositoryScopesConnectionGetAndListAndOmitsSecretsFromJSON(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	ctx := context.Background()
	organizationID, ownerID, connectionID := uuid.New(), uuid.New(), uuid.New()
	connection := readyConnection(organizationID, ownerID, connectionID)

	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_connections" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3 AND deleted_at IS NULL.*LIMIT \$4`).
		WithArgs(organizationID, ownerID, connectionID, 1).
		WillReturnRows(connectionRows(connection))
	got, err := repository.GetConnection(ctx, organizationID, ownerID, connectionID)
	if err != nil || got.ID != connectionID || got.EncryptedCredentials != encryptedCredentialSentinel {
		t.Fatalf("GetConnection() = %#v, %v", got, err)
	}
	assertJSONHasNoSecret(t, got)

	for name, scope := range map[string][2]uuid.UUID{
		"other organization": {uuid.New(), ownerID},
		"other owner":        {organizationID, uuid.New()},
	} {
		t.Run(name, func(t *testing.T) {
			mock.ExpectQuery(`(?s)SELECT .* FROM "connector_connections" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3 AND deleted_at IS NULL.*LIMIT \$4`).
				WithArgs(scope[0], scope[1], connectionID, 1).
				WillReturnRows(sqlmock.NewRows(connectionColumns()))
			_, err := repository.GetConnection(ctx, scope[0], scope[1], connectionID)
			if !core.IsErrorCode(err, core.ErrorCodeNotFound) {
				t.Fatalf("GetConnection() error = %v, want not found", err)
			}
		})
	}

	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_connections" WHERE \(organization_id = \$1 AND owner_account_id = \$2 AND deleted_at IS NULL\) AND connector_id = \$3 AND state IN \(\$4\) ORDER BY connector_id ASC, name ASC, created_at ASC, id ASC`).
		WithArgs(organizationID, ownerID, connection.ConnectorID.String(), core.ReadinessReady).
		WillReturnRows(connectionRows(connection))
	items, err := repository.ListConnections(ctx, organizationID, ownerID, ConnectionListFilter{
		ConnectorID: connection.ConnectorID,
		States:      []core.Readiness{core.ReadinessReady},
	})
	if err != nil || len(items) != 1 || items[0].ID != connectionID {
		t.Fatalf("ListConnections() = %#v, %v", items, err)
	}
	assertJSONHasNoSecret(t, items)
	assertMock(t, mock)
}

func TestGormRepositoryCreatesAndUpdatesWithOptimisticRevision(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	ctx := context.Background()
	organizationID, ownerID := uuid.New(), uuid.New()
	connection := readyConnection(organizationID, ownerID, uuid.New())

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "connector_connections"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repository.CreateConnection(ctx, connection); err != nil {
		t.Fatalf("CreateConnection() error = %v", err)
	}

	connection.Name = "Renamed GitHub"
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE "connector_connections" SET .* WHERE organization_id = \$[0-9]+ AND owner_account_id = \$[0-9]+ AND id = \$[0-9]+ AND connector_id = \$[0-9]+ AND provider_id = \$[0-9]+ AND driver_id = \$[0-9]+ AND auth_method_id = \$[0-9]+ AND revision = \$[0-9]+ AND deleted_at IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	updated, err := repository.UpdateConnection(ctx, organizationID, ownerID, connection, 1)
	if err != nil || updated.Revision != 2 || updated.Name != "Renamed GitHub" {
		t.Fatalf("UpdateConnection() = %#v, %v", updated, err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE "connector_connections" SET .*revision = \$[0-9]+ AND deleted_at IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "connector_connections" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3 AND deleted_at IS NULL`).
		WithArgs(organizationID, ownerID, connection.ID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	if _, err := repository.UpdateConnection(ctx, organizationID, ownerID, connection, 1); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("stale UpdateConnection() error = %v, want conflict", err)
	}
	if _, err := repository.UpdateConnection(ctx, organizationID, uuid.New(), connection, 1); !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
		t.Fatalf("cross-owner UpdateConnection() error = %v, want invalid input", err)
	}
	assertMock(t, mock)
}

func TestGormRepositoryPreservesTwoConnectionsForSameProviderAndExternalAccount(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	organizationID, ownerID := uuid.New(), uuid.New()
	first := readyConnection(organizationID, ownerID, uuid.New())
	second := first
	second.ID = uuid.New()
	second.Name = "Second GitHub key"
	for _, connection := range []core.PersonalConnection{first, second} {
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO "connector_connections"`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		if err := repository.CreateConnection(context.Background(), connection); err != nil {
			t.Fatalf("CreateConnection() error = %v", err)
		}
	}
	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_connections" WHERE \(organization_id = \$1 AND owner_account_id = \$2 AND deleted_at IS NULL\) AND connector_id = \$3 ORDER BY connector_id ASC, name ASC, created_at ASC, id ASC`).
		WithArgs(organizationID, ownerID, first.ConnectorID.String()).WillReturnRows(connectionRows(first, second))
	items, err := repository.ListConnections(context.Background(), organizationID, ownerID, ConnectionListFilter{ConnectorID: first.ConnectorID})
	if err != nil || len(items) != 2 || items[0].ID != first.ID || items[1].ID != second.ID ||
		items[0].ExternalAccountID == nil || items[1].ExternalAccountID == nil || *items[0].ExternalAccountID != *items[1].ExternalAccountID {
		t.Fatalf("same-account repository list = %#v, %v", items, err)
	}
	assertMock(t, mock)
}

func TestGormRepositoryClaimsIdempotentSetupByOwnerAndFingerprint(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	ctx := context.Background()
	organizationID, ownerID := uuid.New(), uuid.New()
	attempt := verifyingAttempt(organizationID, ownerID, uuid.New())

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)INSERT INTO "connector_key_setup_attempts" .*ON CONFLICT \("organization_id","owner_account_id","idempotency_key_hash"\) DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	claimed, created, err := repository.ClaimSetupAttempt(ctx, attempt)
	if err != nil || !created || claimed.ID != attempt.ID {
		t.Fatalf("ClaimSetupAttempt() = %#v, %v, %v", claimed, created, err)
	}
	assertJSONHasNoSecret(t, claimed)

	retry := attempt
	retry.ID = uuid.New()
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "connector_key_setup_attempts"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_key_setup_attempts" WHERE organization_id = \$1 AND owner_account_id = \$2 AND idempotency_key_hash = \$3.*LIMIT \$4 FOR UPDATE`).
		WithArgs(organizationID, ownerID, attempt.IdempotencyKeyHash, 1).
		WillReturnRows(setupAttemptRows(attempt))
	mock.ExpectCommit()
	claimed, created, err = repository.ClaimSetupAttempt(ctx, retry)
	if err != nil || created || claimed.ID != attempt.ID {
		t.Fatalf("idempotent ClaimSetupAttempt() = %#v, %v, %v", claimed, created, err)
	}

	changedRequest := retry
	changedRequest.RequestFingerprint = strings.Repeat("b", 64)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "connector_key_setup_attempts"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_key_setup_attempts" WHERE organization_id = \$1 AND owner_account_id = \$2 AND idempotency_key_hash = \$3.*FOR UPDATE`).
		WithArgs(organizationID, ownerID, attempt.IdempotencyKeyHash, 1).
		WillReturnRows(setupAttemptRows(attempt))
	mock.ExpectRollback()
	if _, _, err := repository.ClaimSetupAttempt(ctx, changedRequest); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("fingerprint mismatch error = %v, want conflict", err)
	}

	otherOwnerAttempt := attempt
	otherOwnerAttempt.ID = uuid.New()
	otherOwnerAttempt.OwnerAccountID = uuid.New()
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "connector_key_setup_attempts"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	claimed, created, err = repository.ClaimSetupAttempt(ctx, otherOwnerAttempt)
	if err != nil || !created || claimed.ID != otherOwnerAttempt.ID {
		t.Fatalf("other-owner ClaimSetupAttempt() = %#v, %v, %v", claimed, created, err)
	}
	assertMock(t, mock)
}

func TestGormRepositoryScopesSetupGetAndList(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	ctx := context.Background()
	organizationID, ownerID := uuid.New(), uuid.New()
	attempt := verifyingAttempt(organizationID, ownerID, uuid.New())
	otherOwnerID := uuid.New()

	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_key_setup_attempts" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3.*LIMIT \$4`).
		WithArgs(organizationID, otherOwnerID, attempt.ID, 1).
		WillReturnRows(sqlmock.NewRows(setupAttemptColumns()))
	if _, err := repository.GetSetupAttempt(ctx, organizationID, otherOwnerID, attempt.ID); !core.IsErrorCode(err, core.ErrorCodeNotFound) {
		t.Fatalf("cross-owner GetSetupAttempt() error = %v, want not found", err)
	}

	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_key_setup_attempts" WHERE \(organization_id = \$1 AND owner_account_id = \$2\) AND connector_id = \$3 AND state IN \(\$4\) ORDER BY created_at DESC, id DESC`).
		WithArgs(organizationID, ownerID, attempt.ConnectorID.String(), core.SetupStateVerifying).
		WillReturnRows(setupAttemptRows(attempt))
	items, err := repository.ListSetupAttempts(ctx, organizationID, ownerID, SetupAttemptListFilter{
		ConnectorID: attempt.ConnectorID,
		States:      []core.SetupState{core.SetupStateVerifying},
	})
	if err != nil || len(items) != 1 || items[0].ID != attempt.ID {
		t.Fatalf("ListSetupAttempts() = %#v, %v", items, err)
	}
	assertMock(t, mock)
}

func TestGormRepositoryCompletesReadySetupInOneTransaction(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	ctx := context.Background()
	organizationID, ownerID := uuid.New(), uuid.New()
	attempt := verifyingAttempt(organizationID, ownerID, uuid.New())
	connection := readyConnection(organizationID, ownerID, uuid.New())

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_key_setup_attempts" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3.*FOR UPDATE`).
		WithArgs(organizationID, ownerID, attempt.ID, 1).
		WillReturnRows(setupAttemptRows(attempt))
	mock.ExpectExec(`INSERT INTO "connector_connections"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "connector_key_setup_attempts" SET`).WillReturnError(errors.New("forced setup update failure"))
	mock.ExpectRollback()
	if _, err := repository.CompleteSetupReady(ctx, organizationID, ownerID, attempt.ID, connection); !core.IsErrorCode(err, core.ErrorCodeInternal) {
		t.Fatalf("CompleteSetupReady() rollback error = %v, want internal", err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_key_setup_attempts" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3.*FOR UPDATE`).
		WithArgs(organizationID, ownerID, attempt.ID, 1).
		WillReturnRows(setupAttemptRows(attempt))
	mock.ExpectExec(`INSERT INTO "connector_connections"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE "connector_key_setup_attempts" SET .* WHERE organization_id = \$[0-9]+ AND owner_account_id = \$[0-9]+ AND id = \$[0-9]+ AND state = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectKeySetupLifecycleAudit(mock, attempt, core.SetupStateReady)
	mock.ExpectCommit()
	completed, err := repository.CompleteSetupReady(ctx, organizationID, ownerID, attempt.ID, connection)
	if err != nil || completed.State != core.SetupStateReady || completed.ResultConnectionID != connection.ID {
		t.Fatalf("CompleteSetupReady() = %#v, %v", completed, err)
	}

	readyAttempt := attempt
	readyAttempt.State = core.SetupStateReady
	readyAttempt.ResultConnectionID = connection.ID
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_key_setup_attempts" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3.*FOR UPDATE`).
		WithArgs(organizationID, ownerID, attempt.ID, 1).
		WillReturnRows(setupAttemptRows(readyAttempt))
	mock.ExpectRollback()
	if _, err := repository.CompleteSetupReady(ctx, organizationID, ownerID, attempt.ID, readyConnection(organizationID, ownerID, uuid.New())); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("repeated CompleteSetupReady() error = %v, want conflict", err)
	}
	assertMock(t, mock)
}

func TestGormRepositoryReadyCompletionCannotCrossOwnerScope(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	ctx := context.Background()
	organizationID, ownerID, otherOwnerID := uuid.New(), uuid.New(), uuid.New()
	attempt := verifyingAttempt(organizationID, ownerID, uuid.New())
	connection := readyConnection(organizationID, otherOwnerID, uuid.New())

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_key_setup_attempts" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3.*FOR UPDATE`).
		WithArgs(organizationID, otherOwnerID, attempt.ID, 1).
		WillReturnRows(sqlmock.NewRows(setupAttemptColumns()))
	mock.ExpectRollback()
	if _, err := repository.CompleteSetupReady(ctx, organizationID, otherOwnerID, attempt.ID, connection); !core.IsErrorCode(err, core.ErrorCodeNotFound) {
		t.Fatalf("cross-owner CompleteSetupReady() error = %v, want not found", err)
	}
	assertMock(t, mock)
}

func TestGormRepositoryFailedAndCancelledSetupNeverInsertConnection(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	ctx := context.Background()
	organizationID, ownerID := uuid.New(), uuid.New()
	failedAttempt := verifyingAttempt(organizationID, ownerID, uuid.New())

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_key_setup_attempts" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3.*FOR UPDATE`).
		WithArgs(organizationID, ownerID, failedAttempt.ID, 1).
		WillReturnRows(setupAttemptRows(failedAttempt))
	mock.ExpectExec(`(?s)UPDATE "connector_key_setup_attempts" SET .* WHERE organization_id = \$[0-9]+ AND owner_account_id = \$[0-9]+ AND id = \$[0-9]+ AND state = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectKeySetupLifecycleAudit(mock, failedAttempt, core.SetupStateFailed)
	mock.ExpectCommit()
	failed, err := repository.CompleteSetupFailed(ctx, organizationID, ownerID, failedAttempt.ID, core.ErrorCodeInvalidKey)
	if err != nil || failed.State != core.SetupStateFailed || failed.ErrorCode != core.ErrorCodeInvalidKey || failed.ResultConnectionID != uuid.Nil {
		t.Fatalf("CompleteSetupFailed() = %#v, %v", failed, err)
	}

	cancelledAttempt := verifyingAttempt(organizationID, ownerID, uuid.New())
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_key_setup_attempts" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3.*FOR UPDATE`).
		WithArgs(organizationID, ownerID, cancelledAttempt.ID, 1).
		WillReturnRows(setupAttemptRows(cancelledAttempt))
	mock.ExpectExec(`UPDATE "connector_key_setup_attempts" SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	expectKeySetupLifecycleAudit(mock, cancelledAttempt, core.SetupStateCancelled)
	mock.ExpectCommit()
	cancelled, err := repository.CancelSetupAttempt(ctx, organizationID, ownerID, cancelledAttempt.ID)
	if err != nil || cancelled.State != core.SetupStateCancelled || cancelled.ErrorCode != "" || cancelled.ResultConnectionID != uuid.Nil {
		t.Fatalf("CancelSetupAttempt() = %#v, %v", cancelled, err)
	}
	// sqlmock rejects any unplanned Connection INSERT. The only inserts on both
	// terminal failure paths are the required non-secret lifecycle decisions.
	assertMock(t, mock)
}

func TestGormRepositoryRollsBackReadySetupWhenLifecycleAuditFails(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	organizationID, ownerID := uuid.New(), uuid.New()
	attempt := verifyingAttempt(organizationID, ownerID, uuid.New())
	connection := readyConnection(organizationID, ownerID, uuid.New())

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_key_setup_attempts" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3.*FOR UPDATE`).
		WithArgs(organizationID, ownerID, attempt.ID, 1).
		WillReturnRows(setupAttemptRows(attempt))
	mock.ExpectExec(`INSERT INTO "connector_connections"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "connector_key_setup_attempts" SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "connector_lifecycle_decisions"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`(?s)INSERT INTO "connector_lifecycle_decisions" .*ON CONFLICT DO NOTHING`).
		WillReturnError(errors.New("forced setup lifecycle audit failure"))
	mock.ExpectRollback()

	if _, err := repository.CompleteSetupReady(
		context.Background(), organizationID, ownerID, attempt.ID, connection,
	); !core.IsErrorCode(err, core.ErrorCodeInternal) {
		t.Fatalf("CompleteSetupReady(audit failure) error = %v, want internal", err)
	}
	assertMock(t, mock)
}

func TestGormRepositoryRoundTripsWorkflowSetupTarget(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	attempt := verifyingAttempt(uuid.New(), uuid.New(), uuid.New())
	attempt.Target = &core.TargetRef{Type: core.TargetTypeWorkflow, ID: "workflow-42", BindingInstanceID: "node-7"}

	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_key_setup_attempts" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3.*LIMIT \$4`).
		WithArgs(attempt.OrganizationID, attempt.OwnerAccountID, attempt.ID, 1).
		WillReturnRows(setupAttemptRows(attempt))
	got, err := repository.GetSetupAttempt(context.Background(), attempt.OrganizationID, attempt.OwnerAccountID, attempt.ID)
	if err != nil || got.Target == nil || *got.Target != *attempt.Target {
		t.Fatalf("GetSetupAttempt() = %#v, %v", got, err)
	}
	assertMock(t, mock)
}

func TestGormRepositoryForceRemoveSnapshotsAndMutatesNativeBindingsOnly(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	connection := readyConnection(uuid.New(), uuid.New(), uuid.New())
	connection.Revision = 3
	nativeBindingID := uuid.New()
	nativeRevision := 7

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_connections" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3 AND deleted_at IS NULL.*LIMIT \$4.*FOR UPDATE`).
		WithArgs(connection.OrganizationID, connection.OwnerAccountID, connection.ID, 1).
		WillReturnRows(connectionRows(connection))
	mock.ExpectQuery(`SELECT .* FROM "connector_target_bindings" WHERE organization_id = \$1 AND owner_account_id = \$2 AND connection_id = \$3 AND status = \$4 ORDER BY id ASC FOR UPDATE`).
		WithArgs(
			connection.OrganizationID,
			connection.OwnerAccountID,
			connection.ID,
			core.BindingStatusActive,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "revision"}).AddRow(nativeBindingID, nativeRevision))
	mock.ExpectExec(`UPDATE "connector_target_bindings" SET .* WHERE organization_id = \$[0-9]+ AND owner_account_id = \$[0-9]+ AND connection_id = \$[0-9]+ AND id IN \(\$[0-9]+\) AND status = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "connector_connections" SET .* WHERE organization_id = \$[0-9]+ AND owner_account_id = \$[0-9]+ AND id = \$[0-9]+ AND revision = \$[0-9]+ AND deleted_at IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "connector_chat_preferences" WHERE organization_id = \$1 AND owner_account_id = \$2 LIMIT \$3`).
		WithArgs(connection.OrganizationID, connection.OwnerAccountID, 1).WillReturnRows(sqlmock.NewRows([]string{"revision", "defaults"}))
	mock.ExpectCommit()

	err := repository.ForceRemoveConnection(
		context.Background(),
		connection.OrganizationID,
		connection.OwnerAccountID,
		connection.ID,
		connection.Revision,
		[]BindingRevisionSnapshot{{ID: nativeBindingID, Revision: nativeRevision}},
	)
	if err != nil {
		t.Fatalf("ForceRemoveConnection() error = %v", err)
	}
	// Both the locked snapshot and the update predicate carry the native-only
	// filter. A legacy_read_only fixture sharing this Connection is therefore
	// neither part of the impact token nor eligible for the status update.
	assertMock(t, mock)
}

func openRepositoryMock(t *testing.T) (*GormRepository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, DriverName: "postgres", PreferSimpleProtocol: true}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	return NewGormRepository(db), mock
}

func expectKeySetupLifecycleAudit(
	mock sqlmock.Sqlmock,
	attempt core.KeySetupAttempt,
	state core.SetupState,
) {
	status := core.DecisionStatusFailed
	if state == core.SetupStateReady {
		status = core.DecisionStatusSuccessful
	} else if state == core.SetupStateCancelled {
		status = core.DecisionStatusCancelled
	}
	correlationID := "key-setup:" + attempt.ID.String()
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "connector_lifecycle_decisions" WHERE \(organization_id = \$1 AND owner_account_id = \$2\) AND \(id = \$3 OR \(correlation_id = \$4 AND decision = \$5 AND status = \$6\)\)`).
		WithArgs(
			attempt.OrganizationID, attempt.OwnerAccountID, sqlmock.AnyArg(),
			correlationID, DecisionKeyCreation, status,
		).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`(?s)INSERT INTO "connector_lifecycle_decisions" .*ON CONFLICT DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func connectionColumns() []string {
	return []string{
		"id", "organization_id", "owner_account_id", "connector_id", "provider_id", "driver_id", "auth_method_id", "name",
		"external_account_id", "external_display_name", "encrypted_credentials", "credential_version", "state", "usable_operation_ids",
		"unavailable_operations", "diagnostic_code", "revision", "last_verified_at", "last_provider_success_at", "last_provider_failure_at",
		"created_at", "updated_at", "deleted_at",
	}
}

func connectionRows(connections ...core.PersonalConnection) *sqlmock.Rows {
	rows := sqlmock.NewRows(connectionColumns())
	for _, connection := range connections {
		usable := make([]string, 0, len(connection.UsableOperationIDs))
		for _, operationID := range connection.UsableOperationIDs {
			usable = append(usable, operationID.String())
		}
		unavailable := make(map[string]string, len(connection.UnavailableOperations))
		for operationID, code := range connection.UnavailableOperations {
			unavailable[operationID.String()] = code
		}
		usableJSON, _ := json.Marshal(usable)
		unavailableJSON, _ := json.Marshal(unavailable)
		rows.AddRow(
			connection.ID, connection.OrganizationID, connection.OwnerAccountID, connection.ConnectorID.String(), connection.ProviderID.String(), connection.DriverID.String(), connection.AuthMethodID.String(), connection.Name,
			pointerValue(connection.ExternalAccountID), pointerValue(connection.ExternalDisplayName), connection.EncryptedCredentials, connection.CredentialVersion, connection.State, usableJSON,
			unavailableJSON, pointerValue(connection.DiagnosticCode), connection.Revision, connection.LastVerifiedAt, pointerValue(connection.LastProviderSuccessAt), pointerValue(connection.LastProviderFailureAt),
			connection.CreatedAt, connection.UpdatedAt, pointerValue(connection.DeletedAt),
		)
	}
	return rows
}

func setupAttemptColumns() []string {
	return []string{
		"id", "organization_id", "owner_account_id", "connector_id", "connection_id", "target_type", "target_id", "binding_instance_id",
		"idempotency_key_hash", "request_fingerprint", "state", "error_code", "result_connection_id", "expires_at", "created_at", "updated_at",
	}
}

func setupAttemptRows(attempt core.KeySetupAttempt) *sqlmock.Rows {
	var targetType, targetID, bindingInstanceID any
	if attempt.Target != nil {
		targetType = attempt.Target.Type
		targetID = attempt.Target.ID
		if attempt.Target.BindingInstanceID != "" {
			bindingInstanceID = attempt.Target.BindingInstanceID
		}
	}
	return sqlmock.NewRows(setupAttemptColumns()).AddRow(
		attempt.ID, attempt.OrganizationID, attempt.OwnerAccountID, attempt.ConnectorID.String(), uuidDriverValue(attempt.ConnectionID), targetType, targetID, bindingInstanceID,
		attempt.IdempotencyKeyHash, attempt.RequestFingerprint, attempt.State, errorCodeDriverValue(attempt.ErrorCode), uuidDriverValue(attempt.ResultConnectionID), attempt.ExpiresAt, attempt.CreatedAt, attempt.UpdatedAt,
	)
}

func readyConnection(organizationID, ownerID, connectionID uuid.UUID) core.PersonalConnection {
	now := time.Now().UTC().Truncate(time.Microsecond)
	externalAccountID := "github-user-42"
	externalDisplayName := "octocat"
	return core.PersonalConnection{
		ID:                    connectionID,
		OrganizationID:        organizationID,
		OwnerAccountID:        ownerID,
		ConnectorID:           core.Identifier("github"),
		ProviderID:            core.Identifier("github"),
		DriverID:              core.Identifier("github-rest"),
		AuthMethodID:          core.Identifier("personal_access_token"),
		Name:                  "GitHub · octocat",
		ExternalAccountID:     &externalAccountID,
		ExternalDisplayName:   &externalDisplayName,
		EncryptedCredentials:  encryptedCredentialSentinel,
		CredentialVersion:     1,
		State:                 core.ReadinessReady,
		UsableOperationIDs:    []core.Identifier{"github.list_repositories"},
		UnavailableOperations: map[core.Identifier]string{},
		Revision:              1,
		LastVerifiedAt:        now,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
}

func verifyingAttempt(organizationID, ownerID, attemptID uuid.UUID) core.KeySetupAttempt {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return core.KeySetupAttempt{
		ID:                 attemptID,
		OrganizationID:     organizationID,
		OwnerAccountID:     ownerID,
		ConnectorID:        core.Identifier("github"),
		IdempotencyKeyHash: strings.Repeat("a", 64),
		RequestFingerprint: strings.Repeat("1", 64),
		State:              core.SetupStateVerifying,
		ExpiresAt:          now.Add(10 * time.Minute),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

func pointerValue[T any](value *T) any {
	if value == nil {
		return nil
	}
	return *value
}

func uuidDriverValue(value uuid.UUID) any {
	if value == uuid.Nil {
		return nil
	}
	return value
}

func errorCodeDriverValue(value core.ErrorCode) any {
	if value == "" {
		return nil
	}
	return value
}

func assertJSONHasNoSecret(t *testing.T, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	jsonText := string(encoded)
	for _, forbidden := range []string{encryptedCredentialSentinel, "encrypted_credentials", strings.Repeat("a", 64), strings.Repeat("1", 64)} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("JSON contains secret/internal value %q: %s", forbidden, jsonText)
		}
	}
}

func assertMock(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}
