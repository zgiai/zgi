package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/datasource/model"
	"github.com/zgiai/zgi/api/pkg/sql_base/audit"
	"github.com/zgiai/zgi/api/pkg/sql_base/guard"
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
		guard.Result{},
		false,
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

func TestAuditSQLOperationWarnRecordsGuardAndExecutes(t *testing.T) {
	dataSource := &model.DataSource{
		ID:             "datasource-1",
		OrganizationID: "organization-1",
		Name:           "main",
		GuardPolicy:    guard.DefaultPolicyJSON(),
	}
	repo := &fakeSQLOperationRepository{}
	service := &dataSourceService{
		repo:             &fakeDataSourceRepo{ds: dataSource},
		sqlOperationRepo: repo,
	}
	executed := false

	err := service.auditSQLOperation(
		context.Background(),
		"organization-1",
		"datasource-1",
		"table-1",
		"main",
		"Users",
		"account-1",
		string(model.OperationTypeDelete),
		`DROP TABLE "users" CASCADE`,
		func() error {
			executed = true
			return nil
		},
	)
	if err != nil {
		t.Fatalf("auditSQLOperation returned error: %v", err)
	}
	if !executed {
		t.Fatal("execute should run in warn mode")
	}
	if len(repo.logs) != 1 {
		t.Fatalf("log count = %d, want 1", len(repo.logs))
	}
	got := repo.logs[0]
	if got.Status != string(model.OperationStatusSuccess) {
		t.Fatalf("status = %s, want success", got.Status)
	}
	if got.GuardVerdict == nil || *got.GuardVerdict != string(guard.VerdictDeny) {
		t.Fatalf("guard_verdict = %#v, want deny", got.GuardVerdict)
	}
	var reasons []guard.Reason
	if err := json.Unmarshal(got.GuardReasons, &reasons); err != nil {
		t.Fatalf("guard_reasons: %v", err)
	}
	if len(reasons) == 0 || reasons[0].Code != guard.ReasonBlockStatement {
		t.Fatalf("guard reasons = %#v, want block_statement", reasons)
	}
}

func TestAuditSQLOperationEnforceBlocksAndAuditsFailure(t *testing.T) {
	policy := guard.DefaultPolicy()
	policy.Mode = guard.ModeEnforce
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	dataSource := &model.DataSource{
		ID:             "datasource-1",
		OrganizationID: "organization-1",
		Name:           "main",
		GuardPolicy:    policyJSON,
	}
	repo := &fakeSQLOperationRepository{}
	service := &dataSourceService{
		repo:             &fakeDataSourceRepo{ds: dataSource},
		sqlOperationRepo: repo,
	}
	executed := false

	err = service.auditSQLOperation(
		context.Background(),
		"organization-1",
		"datasource-1",
		"table-1",
		"main",
		"Users",
		"account-1",
		string(model.OperationTypeDelete),
		`DROP TABLE "users" CASCADE`,
		func() error {
			executed = true
			return nil
		},
	)
	if err == nil {
		t.Fatal("auditSQLOperation should return guard denial")
	}
	var denied *guard.DeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("error = %T, want guard.DeniedError", err)
	}
	if executed {
		t.Fatal("execute should not run in enforce mode")
	}
	if len(repo.logs) != 1 {
		t.Fatalf("log count = %d, want 1", len(repo.logs))
	}
	got := repo.logs[0]
	if got.Status != string(model.OperationStatusFailed) {
		t.Fatalf("status = %s, want failed", got.Status)
	}
	if got.GuardVerdict == nil || *got.GuardVerdict != string(guard.VerdictDeny) {
		t.Fatalf("guard_verdict = %#v, want deny", got.GuardVerdict)
	}
	if got.ErrorMessage == nil || *got.ErrorMessage == "" {
		t.Fatal("error_message should be recorded")
	}
}

func TestAuditSQLOperationEnforceBlocksDDL(t *testing.T) {
	policy := guard.DefaultPolicy()
	policy.Mode = guard.ModeEnforce
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	dataSource := &model.DataSource{
		ID:             "datasource-1",
		OrganizationID: "organization-1",
		Name:           "main",
		GuardPolicy:    policyJSON,
	}
	repo := &fakeSQLOperationRepository{}
	service := &dataSourceService{
		repo:             &fakeDataSourceRepo{ds: dataSource},
		sqlOperationRepo: repo,
	}
	executed := false

	err = service.auditSQLOperation(
		context.Background(),
		"organization-1",
		"datasource-1",
		"",
		"main",
		"users",
		"account-1",
		string(model.OperationTypeCreate),
		`CREATE TABLE "users" ()`,
		func() error {
			executed = true
			return nil
		},
	)
	if err == nil {
		t.Fatal("auditSQLOperation should return guard denial")
	}
	if executed {
		t.Fatal("execute should not run for blocked DDL in enforce mode")
	}
	if len(repo.logs) != 1 {
		t.Fatalf("log count = %d, want 1", len(repo.logs))
	}
	got := repo.logs[0]
	if got.GuardVerdict == nil || *got.GuardVerdict != string(guard.VerdictDeny) {
		t.Fatalf("guard_verdict = %#v, want deny", got.GuardVerdict)
	}
}

func TestAuditSQLOperationAllowedSQLRecordsGuardAllow(t *testing.T) {
	dataSource := &model.DataSource{
		ID:             "datasource-1",
		OrganizationID: "organization-1",
		Name:           "main",
		GuardPolicy:    guard.DefaultPolicyJSON(),
	}
	repo := &fakeSQLOperationRepository{}
	service := &dataSourceService{
		repo:             &fakeDataSourceRepo{ds: dataSource},
		sqlOperationRepo: repo,
	}

	err := service.auditSQLOperation(
		context.Background(),
		"organization-1",
		"datasource-1",
		"table-1",
		"main",
		"Users",
		"account-1",
		string(model.OperationTypeQuery),
		`SELECT * FROM "users"`,
		func() error { return nil },
	)
	if err != nil {
		t.Fatalf("auditSQLOperation returned error: %v", err)
	}
	if len(repo.logs) != 1 {
		t.Fatalf("log count = %d, want 1", len(repo.logs))
	}
	got := repo.logs[0]
	if got.GuardVerdict == nil || *got.GuardVerdict != string(guard.VerdictAllow) {
		t.Fatalf("guard_verdict = %#v, want allow", got.GuardVerdict)
	}
	if len(got.GuardPolicy) == 0 {
		t.Fatal("guard_policy should be recorded")
	}
}

func TestSQLAuditContextIncludesGuardPolicy(t *testing.T) {
	policy := guard.DefaultPolicy()
	policy.Mode = guard.ModeEnforce
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	dataSource := &model.DataSource{
		ID:             "datasource-1",
		OrganizationID: "organization-1",
		Name:           "main",
		GuardPolicy:    policyJSON,
	}

	auditCtx := sqlAuditContext("organization-1", dataSource, nil, "account-1", "query")
	if auditCtx == nil || auditCtx.GuardPolicy == nil {
		t.Fatal("guard policy should be attached to audit context")
	}
	if auditCtx.GuardPolicy.Mode != guard.ModeEnforce {
		t.Fatalf("guard mode = %s, want enforce", auditCtx.GuardPolicy.Mode)
	}
}

func TestPreviewGuardUsesUpdatedPolicyImmediately(t *testing.T) {
	repo := &fakeDataSourceRepo{
		ds: &model.DataSource{
			ID:             "datasource-1",
			OrganizationID: "organization-1",
			Name:           "main",
			GuardPolicy:    guard.DefaultPolicyJSON(),
		},
	}
	svc := &dataSourceService{repo: repo}

	policy := guard.DefaultPolicy()
	policy.Mode = guard.ModeEnforce
	policy.Readonly = true
	policy.RequireWhere = false
	updated, err := svc.UpdateGuardPolicy(context.Background(), "organization-1", "datasource-1", policy)
	if err != nil {
		t.Fatalf("update policy: %v", err)
	}
	if updated.Mode != guard.ModeEnforce {
		t.Fatalf("updated mode = %s, want enforce", updated.Mode)
	}

	result, err := svc.PreviewGuard(context.Background(), "organization-1", "datasource-1", "INSERT INTO users(id) VALUES (1)", nil)
	if err != nil {
		t.Fatalf("preview guard: %v", err)
	}
	if result.Action != guard.ActionDeny {
		t.Fatalf("preview action = %s, want deny", result.Action)
	}
}

type fakeDataSourceRepo struct {
	ds *model.DataSource
}

func (r *fakeDataSourceRepo) Create(ctx context.Context, ds *model.DataSource) error {
	r.ds = ds
	return nil
}

func (r *fakeDataSourceRepo) FindByID(ctx context.Context, id string) (*model.DataSource, error) {
	if r.ds != nil && r.ds.ID == id {
		return r.ds, nil
	}
	return nil, nil
}

func (r *fakeDataSourceRepo) FindByOrganizationAndName(ctx context.Context, organizationID, name string) (*model.DataSource, error) {
	return nil, nil
}

func (r *fakeDataSourceRepo) ListByOrganization(ctx context.Context, organizationID string) ([]*model.DataSource, error) {
	return nil, nil
}

func (r *fakeDataSourceRepo) ListByOrganizationWithPermissionFilter(ctx context.Context, organizationID, accountID string, isAdmin bool, filterWorkspaceIDs []string) ([]*model.DataSource, error) {
	return nil, nil
}

func (r *fakeDataSourceRepo) Update(ctx context.Context, ds *model.DataSource) error {
	r.ds = ds
	return nil
}

func (r *fakeDataSourceRepo) UpdateGuardPolicy(ctx context.Context, id string, policy []byte) error {
	if r.ds != nil && r.ds.ID == id {
		r.ds.GuardPolicy = policy
	}
	return nil
}

func (r *fakeDataSourceRepo) UpdateStatus(ctx context.Context, id, status string) error {
	return nil
}

func (r *fakeDataSourceRepo) Delete(ctx context.Context, id string) error {
	r.ds = nil
	return nil
}
