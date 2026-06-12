package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/datasource/model"
	"github.com/zgiai/zgi/api/pkg/sql_base/audit"
)

func TestSQLAuditContextFallsBackToOrganizationWorkspace(t *testing.T) {
	dataSource := &model.DataSource{
		ID:             "datasource-1",
		OrganizationID: "organization-1",
		Name:           "main",
	}
	table := &model.Table{
		ID:   "table-1",
		Name: "Users",
	}

	auditCtx := sqlAuditContext("organization-1", dataSource, table, "account-1", "query")
	if auditCtx == nil {
		t.Fatal("audit context is nil")
	}
	if auditCtx.OrganizationID != "organization-1" {
		t.Fatalf("organization_id = %s, want organization-1", auditCtx.OrganizationID)
	}
	if auditCtx.WorkspaceID != "organization-1" {
		t.Fatalf("workspace_id = %s, want organization-1", auditCtx.WorkspaceID)
	}
	if auditCtx.TableID != "table-1" {
		t.Fatalf("table_id = %s, want table-1", auditCtx.TableID)
	}
}

func TestSQLAuditContextUsesDataSourceWorkspace(t *testing.T) {
	workspaceID := "workspace-1"
	dataSource := &model.DataSource{
		ID:             "datasource-1",
		OrganizationID: "organization-1",
		WorkspaceID:    &workspaceID,
		Name:           "main",
	}

	auditCtx := sqlAuditContext("organization-1", dataSource, nil, "account-1", "query")
	if auditCtx == nil {
		t.Fatal("audit context is nil")
	}
	if auditCtx.WorkspaceID != "workspace-1" {
		t.Fatalf("workspace_id = %s, want workspace-1", auditCtx.WorkspaceID)
	}
}

type fakeSQLOperationRepository struct {
	logs []*model.DataSourceSQLOperation
}

func (r *fakeSQLOperationRepository) Create(ctx context.Context, log *model.DataSourceSQLOperation) error {
	r.logs = append(r.logs, log)
	return nil
}

func (r *fakeSQLOperationRepository) Insert(ctx context.Context, records []audit.Record) error {
	return nil
}

func (r *fakeSQLOperationRepository) ListByTableID(ctx context.Context, tableID string, limit, offset int) ([]*model.DataSourceSQLOperation, error) {
	return nil, nil
}

func (r *fakeSQLOperationRepository) ListByOrganizationID(ctx context.Context, organizationID string, limit, offset int) ([]*model.DataSourceSQLOperation, error) {
	return nil, nil
}

func (r *fakeSQLOperationRepository) ListByDataSourceID(ctx context.Context, dataSourceID string, limit, offset int) ([]*model.DataSourceSQLOperation, error) {
	return nil, nil
}

func (r *fakeSQLOperationRepository) CountByDataSourceID(ctx context.Context, dataSourceID string) (int64, error) {
	return 0, nil
}

func (r *fakeSQLOperationRepository) ListByDataSourceIDWithFilters(ctx context.Context, dataSourceID string, filters dto.SQLOperationFilter, limit, offset int) ([]*model.DataSourceSQLOperation, error) {
	return nil, nil
}

func (r *fakeSQLOperationRepository) CountByDataSourceIDWithFilters(ctx context.Context, dataSourceID string, filters dto.SQLOperationFilter) (int64, error) {
	return 0, nil
}

func (r *fakeSQLOperationRepository) ListAuditByWorkspace(ctx context.Context, organizationID, workspaceID string, filters dto.SQLAuditFilter, limit, offset int) ([]*model.DataSourceSQLOperation, error) {
	return nil, nil
}

func (r *fakeSQLOperationRepository) CountAuditByWorkspace(ctx context.Context, organizationID, workspaceID string, filters dto.SQLAuditFilter) (int64, error) {
	return 0, nil
}

func (r *fakeSQLOperationRepository) FindAuditByWorkspaceAndID(ctx context.Context, organizationID, workspaceID, operationID string) (*model.DataSourceSQLOperation, error) {
	return nil, nil
}

func TestLogSQLOperationWithResultRecordsFailure(t *testing.T) {
	repo := &fakeSQLOperationRepository{}
	service := &dataSourceService{sqlOperationRepo: repo}
	start := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	end := start.Add(25 * time.Millisecond)
	execErr := errors.New("permission denied")

	err := service.logSQLOperationWithResult(
		context.Background(),
		"organization-1",
		"datasource-1",
		"table-1",
		"main",
		"Users",
		"account-1",
		string(model.OperationTypeDelete),
		`DROP TABLE "users" CASCADE`,
		start,
		end,
		execErr,
	)
	if err != nil {
		t.Fatalf("logSQLOperationWithResult returned error: %v", err)
	}
	if len(repo.logs) != 1 {
		t.Fatalf("log count = %d, want 1", len(repo.logs))
	}
	got := repo.logs[0]
	if got.Status != string(model.OperationStatusFailed) {
		t.Fatalf("status = %s, want failed", got.Status)
	}
	if got.ErrorMessage == nil || *got.ErrorMessage != "permission denied" {
		t.Fatalf("error_message = %#v, want permission denied", got.ErrorMessage)
	}
	if got.DurationMS == nil || *got.DurationMS != 25 {
		t.Fatalf("duration_ms = %#v, want 25", got.DurationMS)
	}
}
