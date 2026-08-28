package integrations

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func openIntegrationRepositoryMock(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, DriverName: "postgres"}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	return db, mock
}

func TestGormConnectionRepositoryScopesLookupToOrganization(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	organizationID := uuid.New()
	connectionID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "integration_connections" WHERE (organization_id = $1 AND id = $2) AND "integration_connections"."deleted_at" IS NULL ORDER BY "integration_connections"."id" LIMIT $3`)).
		WithArgs(organizationID, connectionID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "integration_id", "driver_id", "status", "credential_version"}).
			AddRow(connectionID, organizationID, IntegrationWebSearch, DriverExa, ConnectionStatusActive, 1))
	connection, err := NewGormConnectionRepository(db).GetByID(context.Background(), organizationID, connectionID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if connection.ID != connectionID || connection.OrganizationID != organizationID {
		t.Fatalf("GetByID() = %#v", connection)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestGormConnectionRepositoryAppliesCredentialSourceAndOwnerFilters(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	repository := NewGormConnectionRepository(db)
	organizationID, ownerID := uuid.New(), uuid.New()

	mock.ExpectQuery(`SELECT \* FROM "integration_connections" WHERE organization_id = \$1 AND credential_source IN \(\$2,\$3\).*ORDER BY integration_id ASC, is_default DESC, name ASC, created_at ASC`).
		WithArgs(organizationID, ConnectionCredentialSourcePlatform, ConnectionCredentialSourceOrganization).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "credential_source"}))
	if _, err := repository.List(context.Background(), organizationID, ConnectionListFilter{
		CredentialSources: []ConnectionCredentialSource{ConnectionCredentialSourcePlatform, ConnectionCredentialSourceOrganization},
	}); err != nil {
		t.Fatalf("managed List() error = %v", err)
	}

	mock.ExpectQuery(`SELECT count\(\*\) FROM "integration_connections" WHERE organization_id = \$1 AND credential_source IN \(\$2\) AND owner_account_id = \$3`).
		WithArgs(organizationID, ConnectionCredentialSourceAccount, ownerID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	total, err := repository.Count(context.Background(), organizationID, ConnectionListFilter{
		CredentialSources: []ConnectionCredentialSource{ConnectionCredentialSourceAccount}, OwnerAccountID: &ownerID,
	})
	if err != nil || total != 1 {
		t.Fatalf("personal Count() = %d, %v", total, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestGormConnectionRepositorySerializesDefaultSelectionByIntegration(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	repository := NewGormConnectionRepository(db)
	organizationID := uuid.New()
	actorID := uuid.New()
	targetID := uuid.New()
	oldDefaultID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT "id","integration_id","credential_source" FROM "integration_connections" WHERE .*organization_id = \$1 AND id = \$2.*LIMIT \$3`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "integration_id", "credential_source"}).AddRow(targetID, IntegrationWebSearch, ConnectionCredentialSourceOrganization))
	mock.ExpectQuery(`SELECT \* FROM "integration_connections" WHERE .*organization_id = \$1 AND integration_id = \$2.*ORDER BY id ASC FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "integration_id", "driver_id", "status", "is_default", "credential_version"}).
			AddRow(oldDefaultID, organizationID, IntegrationWebSearch, DriverExa, ConnectionStatusActive, true, 1).
			AddRow(targetID, organizationID, IntegrationWebSearch, DriverExa, ConnectionStatusActive, false, 1))
	mock.ExpectExec(`UPDATE "integration_connections" SET .*"is_default"=\$[0-9]+.*"updated_by"=\$[0-9]+.*WHERE .*organization_id = \$[0-9]+ AND integration_id = \$[0-9]+ AND is_default = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "integration_connections" SET .*"is_default"=\$[0-9]+.*"updated_by"=\$[0-9]+.*WHERE .*organization_id = \$[0-9]+ AND id = \$[0-9]+ AND status = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repository.SetDefaultAs(context.Background(), organizationID, targetID, &actorID); err != nil {
		t.Fatalf("SetDefaultAs() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestGormConnectionRepositoryRejectsExpiredDefaultSelection(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	repository := NewGormConnectionRepository(db)
	organizationID := uuid.New()
	targetID := uuid.New()
	expiredAt := time.Now().UTC().Add(-time.Minute)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT "id","integration_id","credential_source" FROM "integration_connections" WHERE .*organization_id = \$1 AND id = \$2.*LIMIT \$3`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "integration_id", "credential_source"}).AddRow(targetID, IntegrationWebSearch, ConnectionCredentialSourceOrganization))
	mock.ExpectQuery(`SELECT \* FROM "integration_connections" WHERE .*organization_id = \$1 AND integration_id = \$2.*ORDER BY id ASC FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "integration_id", "driver_id", "status", "expires_at", "credential_version", "revision"}).
			AddRow(targetID, organizationID, IntegrationWebSearch, DriverExa, ConnectionStatusActive, expiredAt, 1, 1))
	mock.ExpectRollback()
	err := repository.SetDefaultAs(context.Background(), organizationID, targetID, nil)
	if err == nil || ErrorCode(err) != ErrorCodeConnectionInvalid {
		t.Fatalf("SetDefaultAs() error = %v, code = %q", err, ErrorCode(err))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestGormConnectionRepositoryDeleteIsOrganizationScopedAndSoft(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	repository := NewGormConnectionRepository(db)
	organizationID := uuid.New()
	connectionID := uuid.New()
	actorID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "integration_connections" WHERE .*organization_id = \$1 AND id = \$2.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "integration_id", "credential_source", "status"}).
			AddRow(connectionID, organizationID, IntegrationWebSearch, ConnectionCredentialSourceOrganization, ConnectionStatusActive))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "agent_resource_bindings" WHERE organization_id = \$1 AND binding_type = \$2 AND resource_id = \$3`).
		WithArgs(organizationID, "integration_connection", connectionID.String()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`UPDATE "integration_connections" SET .*"encrypted_credentials"=\$[0-9]+.*"updated_by"=\$[0-9]+.*WHERE .*organization_id = \$[0-9]+ AND id = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "integration_connections" SET "deleted_at"=\$1 WHERE .*organization_id = \$2 AND id = \$3.*"deleted_at" IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repository.DeleteAs(context.Background(), organizationID, connectionID, &actorID); err != nil {
		t.Fatalf("DeleteAs() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestGormConnectionRepositoryDeleteRejectsBoundAgentInsideTransaction(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	repository := NewGormConnectionRepository(db)
	organizationID := uuid.New()
	connectionID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "integration_connections" WHERE .*organization_id = \$1 AND id = \$2.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "integration_id", "credential_source", "status"}).
			AddRow(connectionID, organizationID, IntegrationWebSearch, ConnectionCredentialSourceOrganization, ConnectionStatusActive))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "agent_resource_bindings" WHERE organization_id = \$1 AND binding_type = \$2 AND resource_id = \$3`).
		WithArgs(organizationID, "integration_connection", connectionID.String()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()
	err := repository.DeleteAs(context.Background(), organizationID, connectionID, nil)
	if !errors.Is(err, ErrConnectionInUse) {
		t.Fatalf("DeleteAs() error = %v, want ErrConnectionInUse", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestGormConnectionRepositoryDeleteRejectsNonOwnerOfPersonalConnection(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	repository := NewGormConnectionRepository(db)
	organizationID, connectionID, ownerID, otherAccountID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "integration_connections" WHERE .*organization_id = \$1 AND id = \$2.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "integration_id", "credential_source", "owner_account_id", "status"}).
			AddRow(connectionID, organizationID, IntegrationWebSearch, ConnectionCredentialSourceAccount, ownerID, ConnectionStatusActive))
	mock.ExpectRollback()
	err := repository.DeleteAs(context.Background(), organizationID, connectionID, &otherAccountID)
	if !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("DeleteAs() error = %v, want ErrConnectionNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestGormConnectionRepositoryAtomicallyPersistsOAuthRevocationBeforeDelete(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	repository := NewGormConnectionRepository(db)
	organizationID, connectionID := uuid.New(), uuid.New()
	connectionEnvelope := "v2.test.connection-envelope"
	connection := &IntegrationConnection{
		ID:                   connectionID,
		OrganizationID:       organizationID,
		IntegrationID:        "github",
		DriverID:             "github-rest",
		CredentialSource:     ConnectionCredentialSourceOrganization,
		AuthType:             ConnectionAuthTypeOAuth2,
		AuthMethodID:         "github_oauth",
		EncryptedCredentials: &connectionEnvelope,
		CredentialVersion:    2,
		Revision:             1,
		Config:               map[string]any{},
	}
	task, err := newOAuthRevocationRecoveryTask(connection, "v2.test.client-envelope", 1, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "integration_connections" WHERE .*organization_id = \$1 AND id = \$2.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "organization_id", "integration_id", "driver_id", "credential_source",
			"auth_type", "auth_method_id", "encrypted_credentials", "credential_version", "status", "revision",
		}).AddRow(
			connectionID, organizationID, "github", "github-rest",
			ConnectionCredentialSourceOrganization, ConnectionAuthTypeOAuth2,
			"github_oauth", connectionEnvelope, 2, ConnectionStatusActive, 1,
		))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "agent_resource_bindings"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`INSERT INTO "integration_oauth_recovery_operations"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE "integration_connections" SET .*"encrypted_credentials"=`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "integration_connections" SET "deleted_at"=`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repository.DeleteWithOAuthRevocation(
		context.Background(),
		organizationID,
		connectionID,
		nil,
		task,
	); err != nil {
		t.Fatalf("DeleteWithOAuthRevocation() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestGormConnectionRepositoryOAuthOutboxFailureRollsBackDeletion(t *testing.T) {
	db, mock := openIntegrationRepositoryMock(t)
	repository := NewGormConnectionRepository(db)
	organizationID, connectionID := uuid.New(), uuid.New()
	connectionEnvelope := "v2.test.connection-envelope"
	connection := &IntegrationConnection{
		ID:                   connectionID,
		OrganizationID:       organizationID,
		IntegrationID:        "github",
		DriverID:             "github-rest",
		CredentialSource:     ConnectionCredentialSourceOrganization,
		AuthType:             ConnectionAuthTypeOAuth2,
		AuthMethodID:         "github_oauth",
		EncryptedCredentials: &connectionEnvelope,
		CredentialVersion:    2,
		Revision:             1,
		Config:               map[string]any{},
	}
	task, err := newOAuthRevocationRecoveryTask(connection, "v2.test.client-envelope", 1, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "integration_connections" WHERE .*organization_id = \$1 AND id = \$2.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "organization_id", "integration_id", "driver_id", "credential_source",
			"auth_type", "auth_method_id", "encrypted_credentials", "credential_version", "status", "revision",
		}).AddRow(
			connectionID, organizationID, "github", "github-rest",
			ConnectionCredentialSourceOrganization, ConnectionAuthTypeOAuth2,
			"github_oauth", connectionEnvelope, 2, ConnectionStatusActive, 1,
		))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "agent_resource_bindings"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`INSERT INTO "integration_oauth_recovery_operations"`).
		WillReturnError(errors.New("database unavailable"))
	mock.ExpectRollback()

	if err := repository.DeleteWithOAuthRevocation(
		context.Background(),
		organizationID,
		connectionID,
		nil,
		task,
	); err == nil {
		t.Fatal("DeleteWithOAuthRevocation() accepted an outbox persistence failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}
