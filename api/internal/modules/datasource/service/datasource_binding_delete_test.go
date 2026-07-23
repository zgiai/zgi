package service

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"github.com/zgiai/zgi/api/internal/modules/datasource/model"
	"github.com/zgiai/zgi/api/pkg/sql_base"
	"github.com/zgiai/zgi/api/pkg/sql_base/audit"
	"github.com/zgiai/zgi/api/pkg/sql_base/guard"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type bindingDeletionSQLBase struct {
	sql_base.SQLBase
	deleteCalls int
	deleteErr   error
}

func (s *bindingDeletionSQLBase) ExecuteSQL(context.Context, string, []interface{}, *audit.Context) (*sql_base.QueryResult, error) {
	return &sql_base.QueryResult{Rows: [][]any{{int64(0)}}}, nil
}

func (s *bindingDeletionSQLBase) DeleteTable(context.Context, int, bool) (*sql_base.Table, error) {
	s.deleteCalls++
	return nil, s.deleteErr
}

func TestDeleteTableRejectsStaleImpactBeforeMetadataOrPhysicalDeletion(t *testing.T) {
	svc, mock, sqlBase, tableRepo, organizationID, accountID := newBindingDeletionTestService(t, nil)
	boundAgentID := uuid.New()
	bindingRows := agentBindingRows().AddRow(
		uuid.New(), boundAgentID, agentbindings.ScopeDraft, organizationID, uuid.New(), nil,
		agentbindings.BindingTypeDatabaseTable, "table-1", "datasource-1", "table", "write", nil, nil, time.Now(), time.Now(),
	)
	mock.ExpectBegin()
	expectDatabaseTableBindingLocks(mock, organizationID, "datasource-1", "table-1")
	lockedWorkspaceID := expectLockedDatabaseTable(mock, organizationID, "datasource-1", "table-1")
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings".*binding_type.*resource_id.*parent_resource_id.*ORDER BY`).
		WithArgs(uuid.MustParse(organizationID), agentbindings.BindingTypeDatabaseTable, "table-1", "datasource-1").
		WillReturnRows(bindingRows)
	mock.ExpectQuery(`SELECT id, name, description, icon_type, icon FROM "agents" WHERE id IN \(\$1\) AND deleted_at IS NULL`).
		WithArgs(boundAgentID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "icon_type", "icon"}).
			AddRow(boundAgentID, "Orders assistant", "Uses the orders table", "text", `{"icon":"OA"}`))
	mock.ExpectRollback()

	err := svc.DeleteTable(context.Background(), organizationID, "datasource-1", "table-1", accountID, "unbind", "stale-token")
	var conflict *agentbindings.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("DeleteTable() error = %v, want ConflictError", err)
	}
	if sqlBase.deleteCalls != 0 {
		t.Fatalf("physical delete calls = %d, want 0", sqlBase.deleteCalls)
	}
	if table, _ := tableRepo.FindByID(context.Background(), "table-1"); table == nil {
		t.Fatal("table metadata was removed for stale impact token")
	}
	authorization := svc.authorizationService.(*fakeAuthorizationService)
	if len(authorization.workspaceRequests) != 2 || authorization.workspaceRequests[1].WorkspaceID != lockedWorkspaceID {
		t.Fatalf("workspace permission requests = %#v, want locked workspace %s rechecked", authorization.workspaceRequests, lockedWorkspaceID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestDeleteTableDropFailureKeepsCommittedLogicalDeletionSuccessful(t *testing.T) {
	dropErr := errors.New("postgres-meta unavailable")
	svc, mock, sqlBase, _, organizationID, accountID := newBindingDeletionTestService(t, dropErr)
	mock.ExpectBegin()
	expectDatabaseTableBindingLocks(mock, organizationID, "datasource-1", "table-1")
	_ = expectLockedDatabaseTable(mock, organizationID, "datasource-1", "table-1")
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings".*binding_type.*resource_id.*parent_resource_id.*ORDER BY`).
		WithArgs(uuid.MustParse(organizationID), agentbindings.BindingTypeDatabaseTable, "table-1", "datasource-1").
		WillReturnRows(agentBindingRows())
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM data_source_table_prompts WHERE table_id = $1`)).
		WithArgs("table-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM data_source_tables WHERE id = $1`)).
		WithArgs("table-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := svc.DeleteTable(context.Background(), organizationID, "datasource-1", "table-1", accountID, "", "")
	if err != nil {
		t.Fatalf("DeleteTable() error = %v, want committed logical deletion to remain successful", err)
	}
	if sqlBase.deleteCalls != 1 {
		t.Fatalf("physical delete calls = %d, want 1 after metadata commit", sqlBase.deleteCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestDeleteDataSourceReauthorizesLockedWorkspaceAfterLifecycleLock(t *testing.T) {
	svc, mock, _, _, organizationID, accountID := newBindingDeletionTestService(t, nil)
	svc.tableRepo = &fakeTableRepository{items: map[string]*model.Table{}}

	mock.ExpectBegin()
	lockSQL := regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)
	mock.ExpectExec(lockSQL).
		WithArgs("agent-binding-resource:" + organizationID + ":database::datasource-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	lockedWorkspaceID := uuid.NewString()
	mock.ExpectQuery(`SELECT \* FROM "data_sources" WHERE id = \$1 AND organization_id = \$2 LIMIT \$3 FOR UPDATE`).
		WithArgs("datasource-1", organizationID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "organization_id", "workspace_id", "name", "created_by", "guard_policy",
		}).AddRow("datasource-1", organizationID, lockedWorkspaceID, "database", accountID, guard.DefaultPolicyJSON()))
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings".*binding_type.*resource_id.*ORDER BY`).
		WithArgs(
			uuid.MustParse(organizationID),
			agentbindings.BindingTypeDatabase,
			"datasource-1",
			agentbindings.BindingTypeDatabaseTable,
			"datasource-1",
		).
		WillReturnRows(agentBindingRows())
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM data_sources WHERE id = $1`)).
		WithArgs("datasource-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := svc.DeleteDataSourceByID(context.Background(), organizationID, "datasource-1", accountID, "", ""); err != nil {
		t.Fatalf("DeleteDataSourceByID() error = %v", err)
	}
	authorization := svc.authorizationService.(*fakeAuthorizationService)
	if len(authorization.workspaceRequests) != 2 || authorization.workspaceRequests[1].WorkspaceID != lockedWorkspaceID {
		t.Fatalf("workspace permission requests = %#v, want locked workspace %s rechecked", authorization.workspaceRequests, lockedWorkspaceID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func expectDatabaseTableBindingLocks(mock sqlmock.Sqlmock, organizationID, dataSourceID, tableID string) {
	lockSQL := regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)
	mock.ExpectExec(lockSQL).
		WithArgs("agent-binding-resource:" + organizationID + ":database::" + dataSourceID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(lockSQL).
		WithArgs("agent-binding-resource:" + organizationID + ":database_table:" + dataSourceID + ":" + tableID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectLockedDatabaseTable(mock sqlmock.Sqlmock, organizationID, dataSourceID, tableID string) string {
	workspaceID := uuid.NewString()
	mock.ExpectQuery(`SELECT \* FROM "data_sources" WHERE id = \$1 AND organization_id = \$2 LIMIT \$3 FOR UPDATE`).
		WithArgs(dataSourceID, organizationID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "organization_id", "workspace_id", "name", "created_by", "guard_policy",
		}).AddRow(dataSourceID, organizationID, workspaceID, "database", uuid.NewString(), guard.DefaultPolicyJSON()))
	mock.ExpectQuery(`SELECT \* FROM "data_source_tables" WHERE id = \$1 AND data_source_id = \$2 AND organization_id = \$3 LIMIT \$4 FOR UPDATE`).
		WithArgs(tableID, dataSourceID, organizationID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "organization_id", "data_source_id", "name", "table_id", "table_name",
		}).AddRow(tableID, organizationID, dataSourceID, "orders", 42, "tbl_orders"))
	return workspaceID
}

func newBindingDeletionTestService(t *testing.T, deleteErr error) (*dataSourceService, sqlmock.Sqlmock, *bindingDeletionSQLBase, *fakeTableRepository, string, string) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	workspaceID := uuid.NewString()
	dataSource := &model.DataSource{
		ID:             "datasource-1",
		OrganizationID: organizationID,
		WorkspaceID:    &workspaceID,
		Name:           "database",
		CreatedBy:      accountID,
		GuardPolicy:    guard.DefaultPolicyJSON(),
	}
	tableRepo := &fakeTableRepository{items: map[string]*model.Table{
		"table-1": {
			ID:                "table-1",
			OrganizationID:    organizationID,
			DataSourceID:      dataSource.ID,
			Name:              "orders",
			TableID:           42,
			PhysicalTableName: "tbl_orders",
		},
	}}
	sqlBase := &bindingDeletionSQLBase{deleteErr: deleteErr}
	svc := &dataSourceService{
		repo:                 &fakeDataSourceRepository{items: map[string]*model.DataSource{dataSource.ID: dataSource}},
		tableRepo:            tableRepo,
		promptRepo:           &fakePromptRepository{items: map[string]*model.TablePrompt{}},
		sqlOperationRepo:     &fakeSQLOperationRepository{},
		sqlBase:              sqlBase,
		authorizationService: &fakeAuthorizationService{allow: true},
		db:                   db,
		agentBindings:        agentbindings.NewRepositoryWithTokenSecret(db, "shared-delete-secret"),
	}
	return svc, mock, sqlBase, tableRepo, organizationID, accountID
}

func agentBindingRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "agent_id", "binding_scope", "organization_id", "workspace_id", "published_version_uuid",
		"binding_type", "resource_id", "parent_resource_id", "display_name", "access_mode",
		"authorized_by", "authorized_at", "created_at", "updated_at",
	})
}
