package gateway

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func openWorkspaceQuotaPostgresMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("open gorm: %v", err)
	}

	cleanup := func() {
		_ = sqlDB.Close()
	}

	return db, mock, cleanup
}

func TestBillingService_GetOrCreateWorkspaceQuotaForUpdate_DuplicateCreateFallsBackToExistingRow(t *testing.T) {
	db, mock, cleanup := openWorkspaceQuotaPostgresMockDB(t)
	defer cleanup()

	svc := &BillingService{db: db}
	workspaceID := "ws-race-1"
	orgID := uuid.New()

	selectForUpdate := regexp.QuoteMeta(`SELECT * FROM "llm_workspace_quotas" WHERE workspace_id = $1 ORDER BY "llm_workspace_quotas"."workspace_id" LIMIT $2 FOR UPDATE`)
	insertWithConflict := regexp.QuoteMeta(`INSERT INTO "llm_workspace_quotas" ("workspace_id","organization_id","used_quota","remain_quota","quota_limit","created_at","updated_at") VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT ("workspace_id") DO NOTHING`)

	mock.ExpectBegin()
	mock.ExpectQuery(selectForUpdate).
		WithArgs(workspaceID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"workspace_id",
			"organization_id",
			"used_quota",
			"remain_quota",
			"quota_limit",
			"created_at",
			"updated_at",
		}))
	mock.ExpectExec(insertWithConflict).
		WithArgs(workspaceID, sqlmock.AnyArg(), int64(0), int64(0), nil, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(selectForUpdate).
		WithArgs(workspaceID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"workspace_id",
			"organization_id",
			"used_quota",
			"remain_quota",
			"quota_limit",
			"created_at",
			"updated_at",
		}).AddRow(workspaceID, orgID.String(), int64(0), int64(0), nil, time.Now(), time.Now()))
	mock.ExpectCommit()

	var got *WorkspaceQuota
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		got, err = svc.getOrCreateWorkspaceQuotaForUpdate(context.Background(), tx, workspaceID, orgID)
		return err
	})
	if err != nil {
		t.Fatalf("getOrCreateWorkspaceQuotaForUpdate returned error: %v", err)
	}
	if got == nil {
		t.Fatal("expected workspace quota, got nil")
	}
	if got.WorkspaceID != workspaceID {
		t.Fatalf("workspace_id = %q, want %q", got.WorkspaceID, workspaceID)
	}
	if got.OrganizationID != orgID {
		t.Fatalf("organization_id = %s, want %s", got.OrganizationID, orgID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}
