package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/datasource/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
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
	logs           []*model.DataSourceSQLOperation
	auditLogs      []*model.DataSourceSQLOperation
	auditDetail    *model.DataSourceSQLOperation
	listAuditCalls int
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
	return r.logs, nil
}

func (r *fakeSQLOperationRepository) CountByDataSourceID(ctx context.Context, dataSourceID string) (int64, error) {
	return int64(len(r.logs)), nil
}

func (r *fakeSQLOperationRepository) ListByDataSourceIDWithFilters(ctx context.Context, dataSourceID string, filters dto.SQLOperationFilter, limit, offset int) ([]*model.DataSourceSQLOperation, error) {
	return r.logs, nil
}

func (r *fakeSQLOperationRepository) CountByDataSourceIDWithFilters(ctx context.Context, dataSourceID string, filters dto.SQLOperationFilter) (int64, error) {
	return int64(len(r.logs)), nil
}

func (r *fakeSQLOperationRepository) ListAuditByWorkspace(ctx context.Context, organizationID, workspaceID string, filters dto.SQLAuditFilter, limit, offset int) ([]*model.DataSourceSQLOperation, error) {
	r.listAuditCalls++
	return r.auditLogs, nil
}

func (r *fakeSQLOperationRepository) CountAuditByWorkspace(ctx context.Context, organizationID, workspaceID string, filters dto.SQLAuditFilter) (int64, error) {
	return int64(len(r.auditLogs)), nil
}

func (r *fakeSQLOperationRepository) FindAuditByWorkspaceAndID(ctx context.Context, organizationID, workspaceID, operationID string) (*model.DataSourceSQLOperation, error) {
	return r.auditDetail, nil
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

type fakeDataSourceRepository struct {
	items map[string]*model.DataSource
}

func (r *fakeDataSourceRepository) Create(ctx context.Context, ds *model.DataSource) error {
	if r.items == nil {
		r.items = make(map[string]*model.DataSource)
	}
	if ds.ID == "" {
		ds.ID = "datasource-created"
	}
	copied := *ds
	r.items[ds.ID] = &copied
	return nil
}

func (r *fakeDataSourceRepository) FindByID(ctx context.Context, id string) (*model.DataSource, error) {
	if r.items == nil {
		return nil, nil
	}
	item := r.items[id]
	if item == nil {
		return nil, nil
	}
	copied := *item
	return &copied, nil
}

func (r *fakeDataSourceRepository) FindByOrganizationAndName(ctx context.Context, organizationID, name string) (*model.DataSource, error) {
	for _, item := range r.items {
		if item.OrganizationID == organizationID && item.Name == name {
			copied := *item
			return &copied, nil
		}
	}
	return nil, nil
}

func (r *fakeDataSourceRepository) ListByOrganization(ctx context.Context, organizationID string) ([]*model.DataSource, error) {
	var result []*model.DataSource
	for _, item := range r.items {
		if item.OrganizationID != organizationID {
			continue
		}
		copied := *item
		result = append(result, &copied)
	}
	return result, nil
}

func (r *fakeDataSourceRepository) ListByOrganizationWithPermissionFilter(ctx context.Context, organizationID, accountID string, isAdmin bool, filterWorkspaceIDs []string) ([]*model.DataSource, error) {
	return r.ListByOrganization(ctx, organizationID)
}

func (r *fakeDataSourceRepository) Update(ctx context.Context, ds *model.DataSource) error {
	if r.items == nil {
		r.items = make(map[string]*model.DataSource)
	}
	copied := *ds
	r.items[ds.ID] = &copied
	return nil
}

func (r *fakeDataSourceRepository) UpdateGuardPolicy(ctx context.Context, id string, policy []byte) error {
	if r.items != nil && r.items[id] != nil {
		r.items[id].GuardPolicy = policy
	}
	return nil
}

func (r *fakeDataSourceRepository) UpdateStatus(ctx context.Context, id, status string) error {
	if r.items != nil && r.items[id] != nil {
		r.items[id].Status = status
	}
	return nil
}

func (r *fakeDataSourceRepository) Delete(ctx context.Context, id string) error {
	delete(r.items, id)
	return nil
}

type fakeTableRepository struct {
	items map[string]*model.Table
}

func (r *fakeTableRepository) Create(ctx context.Context, table *model.Table) error {
	if r.items == nil {
		r.items = make(map[string]*model.Table)
	}
	if table.ID == "" {
		table.ID = "table-created"
	}
	copied := *table
	r.items[table.ID] = &copied
	return nil
}

func (r *fakeTableRepository) FindByID(ctx context.Context, id string) (*model.Table, error) {
	if r.items == nil {
		return nil, nil
	}
	item := r.items[id]
	if item == nil {
		return nil, nil
	}
	copied := *item
	return &copied, nil
}

func (r *fakeTableRepository) FindByOrganizationAndName(ctx context.Context, organizationID, name string) (*model.Table, error) {
	for _, item := range r.items {
		if item.OrganizationID == organizationID && item.Name == name {
			copied := *item
			return &copied, nil
		}
	}
	return nil, nil
}

func (r *fakeTableRepository) FindByDataSourceAndTableName(ctx context.Context, dataSourceID, tableName string) (*model.Table, error) {
	for _, item := range r.items {
		if item.DataSourceID == dataSourceID && item.PhysicalTableName == tableName {
			copied := *item
			return &copied, nil
		}
	}
	return nil, nil
}

func (r *fakeTableRepository) FindByDataSourceAndName(ctx context.Context, dataSourceID, name string) (*model.Table, error) {
	for _, item := range r.items {
		if item.DataSourceID == dataSourceID && item.Name == name {
			copied := *item
			return &copied, nil
		}
	}
	return nil, nil
}

func (r *fakeTableRepository) ListByDataSource(ctx context.Context, dataSourceID string) ([]*model.Table, error) {
	var result []*model.Table
	for _, item := range r.items {
		if item.DataSourceID != dataSourceID {
			continue
		}
		copied := *item
		result = append(result, &copied)
	}
	return result, nil
}

func (r *fakeTableRepository) ListByOrganization(ctx context.Context, organizationID string) ([]*model.Table, error) {
	var result []*model.Table
	for _, item := range r.items {
		if item.OrganizationID != organizationID {
			continue
		}
		copied := *item
		result = append(result, &copied)
	}
	return result, nil
}

func (r *fakeTableRepository) Update(ctx context.Context, table *model.Table) error {
	if r.items == nil {
		r.items = make(map[string]*model.Table)
	}
	copied := *table
	r.items[table.ID] = &copied
	return nil
}

func (r *fakeTableRepository) Delete(ctx context.Context, id string) error {
	delete(r.items, id)
	return nil
}

type fakePromptRepository struct {
	items map[string]*model.TablePrompt
}

func (r *fakePromptRepository) Create(ctx context.Context, prompt *model.TablePrompt) error {
	if r.items == nil {
		r.items = make(map[string]*model.TablePrompt)
	}
	if prompt.ID == "" {
		prompt.ID = "prompt-created"
	}
	copied := *prompt
	r.items[prompt.TableID] = &copied
	return nil
}

func (r *fakePromptRepository) FindByTableID(ctx context.Context, tableID string) (*model.TablePrompt, error) {
	if r.items == nil {
		return nil, nil
	}
	prompt := r.items[tableID]
	if prompt == nil {
		return nil, nil
	}
	copied := *prompt
	return &copied, nil
}

func (r *fakePromptRepository) Update(ctx context.Context, prompt *model.TablePrompt) error {
	if r.items == nil {
		r.items = make(map[string]*model.TablePrompt)
	}
	copied := *prompt
	r.items[prompt.TableID] = &copied
	return nil
}

func (r *fakePromptRepository) Delete(ctx context.Context, id string) error {
	for tableID, prompt := range r.items {
		if prompt.ID == id {
			delete(r.items, tableID)
			return nil
		}
	}
	return nil
}

func (r *fakePromptRepository) DeleteByTableID(ctx context.Context, tableID string) error {
	delete(r.items, tableID)
	return nil
}

type fakeAuthorizationService struct {
	allow             bool
	err               error
	workspaceRequests []interfaces.WorkspaceScopeRequest
}

func (s *fakeAuthorizationService) RequireOrganizationMember(ctx context.Context, req interfaces.OrganizationScopeRequest) (*interfaces.OrganizationScope, error) {
	if s.err != nil {
		return nil, s.err
	}
	if !s.allow {
		return nil, errors.New("authorization denied")
	}
	return &interfaces.OrganizationScope{
		OrganizationID: req.OrganizationID,
		AccountID:      req.AccountID,
		Role:           workspace_model.OrganizationRoleNormal,
	}, nil
}

func (s *fakeAuthorizationService) CanUseOrganizationFeature(ctx context.Context, req interfaces.OrganizationFeatureRequest) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.allow, nil
}

func (s *fakeAuthorizationService) RequireWorkspacePermission(ctx context.Context, req interfaces.WorkspaceScopeRequest) (*interfaces.WorkspaceScope, error) {
	s.workspaceRequests = append(s.workspaceRequests, req)
	if s.err != nil {
		return nil, s.err
	}
	if !s.allow {
		return nil, errors.New("authorization denied")
	}
	return &interfaces.WorkspaceScope{
		OrganizationScope: interfaces.OrganizationScope{
			OrganizationID: req.OrganizationID,
			AccountID:      req.AccountID,
			Role:           workspace_model.OrganizationRoleNormal,
		},
		WorkspaceID:     req.WorkspaceID,
		PermissionCodes: req.PermissionCodes,
	}, nil
}

func (s *fakeAuthorizationService) ListWorkspaceIDsByPermission(ctx context.Context, req interfaces.WorkspaceListPermissionRequest) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	if !s.allow {
		return []string{}, nil
	}
	return []string{"workspace-1"}, nil
}

type fakeResourcePermissionService struct{}

func (s *fakeResourcePermissionService) CheckSingleResourceEditPermission(ctx context.Context, params interfaces.SingleResourcePermissionParams) (bool, error) {
	return params.AccountID == params.CreatedBy, nil
}

func (s *fakeResourcePermissionService) CheckBatchResourceEditPermission(ctx context.Context, params interfaces.BatchResourcePermissionParams) (map[string]bool, error) {
	result := make(map[string]bool, len(params.Resources))
	for _, resource := range params.Resources {
		result[resource.ResourceID] = params.AccountID == resource.CreatedBy
	}
	return result, nil
}

func newScopedDataSourceService(dataSources map[string]*model.DataSource, tables map[string]*model.Table, authorization *fakeAuthorizationService) *dataSourceService {
	if authorization == nil {
		authorization = &fakeAuthorizationService{allow: true}
	}
	return &dataSourceService{
		repo:                      &fakeDataSourceRepository{items: dataSources},
		tableRepo:                 &fakeTableRepository{items: tables},
		promptRepo:                &fakePromptRepository{items: make(map[string]*model.TablePrompt)},
		sqlOperationRepo:          &fakeSQLOperationRepository{},
		authorizationService:      authorization,
		resourcePermissionService: &fakeResourcePermissionService{},
	}
}

func TestGetDataSourceByIDRejectsCrossOrganizationAsset(t *testing.T) {
	workspaceID := "workspace-1"
	svc := newScopedDataSourceService(map[string]*model.DataSource{
		"datasource-1": {
			ID:             "datasource-1",
			OrganizationID: "organization-2",
			WorkspaceID:    &workspaceID,
			Name:           "external",
			CreatedBy:      "account-1",
		},
	}, nil, nil)

	got, err := svc.GetDataSourceByID(context.Background(), "organization-1", "datasource-1", "account-1")
	if err != nil {
		t.Fatalf("GetDataSourceByID error = %v", err)
	}
	if got != nil {
		t.Fatalf("GetDataSourceByID returned datasource from another organization: %#v", got)
	}
}

func TestCreateDataSourceRequiresWorkspaceScopeAndCreatePermission(t *testing.T) {
	authorization := &fakeAuthorizationService{allow: true}
	svc := newScopedDataSourceService(map[string]*model.DataSource{}, nil, authorization)
	workspaceID := " workspace-1 "

	got, err := svc.CreateDataSource(context.Background(), "organization-1", "account-1", dto.CreateDataSourceRequest{
		Name:        "main",
		WorkspaceID: &workspaceID,
		Permission:  string(model.DataSourcePermissionAllGroup),
	})
	if err != nil {
		t.Fatalf("CreateDataSource error = %v", err)
	}
	if got.WorkspaceID == nil || *got.WorkspaceID != "workspace-1" {
		t.Fatalf("workspace_id = %#v, want workspace-1", got.WorkspaceID)
	}
	if len(authorization.workspaceRequests) != 1 {
		t.Fatalf("workspace permission checks = %d, want 1", len(authorization.workspaceRequests))
	}
	req := authorization.workspaceRequests[0]
	if req.OrganizationID != "organization-1" || req.WorkspaceID != "workspace-1" || req.AccountID != "account-1" {
		t.Fatalf("workspace permission request = %#v, want organization/workspace/account scope", req)
	}
	if len(req.PermissionCodes) != 1 || req.PermissionCodes[0] != workspace_model.WorkspacePermissionDatabaseCreate {
		t.Fatalf("permission codes = %#v, want database.create", req.PermissionCodes)
	}
}

func TestCreateDataSourceRejectsMissingWorkspace(t *testing.T) {
	authorization := &fakeAuthorizationService{allow: true}
	svc := newScopedDataSourceService(map[string]*model.DataSource{}, nil, authorization)

	_, err := svc.CreateDataSource(context.Background(), "organization-1", "account-1", dto.CreateDataSourceRequest{Name: "main"})
	if err == nil || !strings.Contains(err.Error(), "workspace_id is required") {
		t.Fatalf("CreateDataSource error = %v, want workspace_id is required", err)
	}
	if len(authorization.workspaceRequests) != 0 {
		t.Fatalf("workspace permission checks = %d, want 0", len(authorization.workspaceRequests))
	}
}

func TestGetTableRejectsTableOutsideDataSource(t *testing.T) {
	workspaceID := "workspace-1"
	svc := newScopedDataSourceService(map[string]*model.DataSource{
		"datasource-1": {
			ID:             "datasource-1",
			OrganizationID: "organization-1",
			WorkspaceID:    &workspaceID,
		},
	}, map[string]*model.Table{
		"table-1": {
			ID:             "table-1",
			OrganizationID: "organization-1",
			DataSourceID:   "datasource-2",
			Name:           "Users",
		},
	}, nil)

	_, err := svc.GetTable(context.Background(), "organization-1", "datasource-1", "table-1", "account-1")
	if err == nil || !strings.Contains(err.Error(), "table with id 'table-1' not found") {
		t.Fatalf("GetTable error = %v, want table not found", err)
	}
}

func TestUpdateDataSourceUsesWorkspaceUpdatePermission(t *testing.T) {
	workspaceID := "workspace-1"
	authorization := &fakeAuthorizationService{allow: true}
	svc := newScopedDataSourceService(map[string]*model.DataSource{
		"datasource-1": {
			ID:             "datasource-1",
			OrganizationID: "organization-1",
			WorkspaceID:    &workspaceID,
			Name:           "main",
			CreatedBy:      "creator-account",
		},
	}, nil, authorization)

	newName := "main-renamed"
	got, err := svc.UpdateDataSource(context.Background(), "organization-1", "datasource-1", "workspace-manager", dto.UpdateDataSourceRequest{Name: &newName})
	if err != nil {
		t.Fatalf("UpdateDataSource error = %v", err)
	}
	if got.Name != newName {
		t.Fatalf("name = %s, want %s", got.Name, newName)
	}
	if len(authorization.workspaceRequests) != 1 {
		t.Fatalf("workspace permission checks = %d, want 1", len(authorization.workspaceRequests))
	}
	if authorization.workspaceRequests[0].PermissionCodes[0] != workspace_model.WorkspacePermissionDatabaseUpdate {
		t.Fatalf("permission code = %s, want database.update", authorization.workspaceRequests[0].PermissionCodes[0])
	}
}

func TestUpdateDataSourceMoveUsesWorkspaceMovePermission(t *testing.T) {
	workspaceID := "workspace-1"
	targetWorkspaceID := "workspace-2"
	authorization := &fakeAuthorizationService{allow: true}
	svc := newScopedDataSourceService(map[string]*model.DataSource{
		"datasource-1": {
			ID:             "datasource-1",
			OrganizationID: "organization-1",
			WorkspaceID:    &workspaceID,
			Name:           "main",
			CreatedBy:      "creator-account",
		},
	}, nil, authorization)

	got, err := svc.UpdateDataSource(context.Background(), "organization-1", "datasource-1", "workspace-manager", dto.UpdateDataSourceRequest{WorkspaceID: &targetWorkspaceID})
	if err != nil {
		t.Fatalf("UpdateDataSource error = %v", err)
	}
	if got.WorkspaceID == nil || *got.WorkspaceID != targetWorkspaceID {
		t.Fatalf("workspace_id = %#v, want %s", got.WorkspaceID, targetWorkspaceID)
	}
	if len(authorization.workspaceRequests) != 2 {
		t.Fatalf("workspace permission checks = %d, want 2", len(authorization.workspaceRequests))
	}
	updateReq := authorization.workspaceRequests[0]
	if updateReq.WorkspaceID != workspaceID || len(updateReq.PermissionCodes) != 1 || updateReq.PermissionCodes[0] != workspace_model.WorkspacePermissionDatabaseUpdate {
		t.Fatalf("first permission request = %#v, want database.update on source workspace", updateReq)
	}
	moveReq := authorization.workspaceRequests[1]
	if moveReq.WorkspaceID != targetWorkspaceID || len(moveReq.PermissionCodes) != 1 || moveReq.PermissionCodes[0] != workspace_model.WorkspacePermissionDatabaseMove {
		t.Fatalf("second permission request = %#v, want database.move on target workspace", moveReq)
	}
}

func TestDeleteDataSourceUsesWorkspaceDeletePermission(t *testing.T) {
	workspaceID := "workspace-1"
	authorization := &fakeAuthorizationService{allow: true}
	svc := newScopedDataSourceService(map[string]*model.DataSource{
		"datasource-1": {
			ID:             "datasource-1",
			OrganizationID: "organization-1",
			WorkspaceID:    &workspaceID,
			Name:           "main",
			CreatedBy:      "creator-account",
		},
	}, nil, authorization)

	if err := svc.DeleteDataSourceByID(context.Background(), "organization-1", "datasource-1", "workspace-manager", "", ""); err != nil {
		t.Fatalf("DeleteDataSourceByID error = %v", err)
	}
	if len(authorization.workspaceRequests) != 1 {
		t.Fatalf("workspace permission checks = %d, want 1", len(authorization.workspaceRequests))
	}
	req := authorization.workspaceRequests[0]
	if req.WorkspaceID != workspaceID || len(req.PermissionCodes) != 1 || req.PermissionCodes[0] != workspace_model.WorkspacePermissionDatabaseDelete {
		t.Fatalf("permission request = %#v, want database.delete on source workspace", req)
	}
}

func TestUpdateTableUsesDatabaseSchemaManagePermission(t *testing.T) {
	workspaceID := "workspace-1"
	authorization := &fakeAuthorizationService{allow: true}
	svc := newScopedDataSourceService(map[string]*model.DataSource{
		"datasource-1": {
			ID:             "datasource-1",
			OrganizationID: "organization-1",
			WorkspaceID:    &workspaceID,
		},
	}, map[string]*model.Table{
		"table-1": {
			ID:             "table-1",
			OrganizationID: "organization-1",
			DataSourceID:   "datasource-1",
			Name:           "Users",
		},
	}, authorization)

	newName := "Customers"
	got, err := svc.UpdateTable(context.Background(), "organization-1", "datasource-1", "table-1", "schema-editor", dto.UpdateTableRequest{Name: &newName})
	if err != nil {
		t.Fatalf("UpdateTable error = %v", err)
	}
	if got.Name != newName {
		t.Fatalf("table name = %s, want %s", got.Name, newName)
	}
	if len(authorization.workspaceRequests) != 1 {
		t.Fatalf("workspace permission checks = %d, want 1", len(authorization.workspaceRequests))
	}
	req := authorization.workspaceRequests[0]
	if req.WorkspaceID != "workspace-1" {
		t.Fatalf("workspace_id = %s, want workspace-1", req.WorkspaceID)
	}
	if len(req.PermissionCodes) != 1 || req.PermissionCodes[0] != workspace_model.WorkspacePermissionDatabaseSchemaManage {
		t.Fatalf("permission codes = %#v, want database.schema.manage", req.PermissionCodes)
	}
}

func TestRecordAndImportServicesRequireFineDatabasePermissions(t *testing.T) {
	tests := []struct {
		name           string
		call           func(context.Context, *dataSourceService) error
		wantPermission workspace_model.WorkspacePermissionCode
	}{
		{
			name: "add records",
			call: func(ctx context.Context, svc *dataSourceService) error {
				_, err := svc.AddTableRecords(ctx, "organization-1", "datasource-1", "table-1", "account-1", dto.AddRecordRequest{})
				return err
			},
			wantPermission: workspace_model.WorkspacePermissionDatabaseRecordCreate,
		},
		{
			name: "query records",
			call: func(ctx context.Context, svc *dataSourceService) error {
				_, err := svc.QueryTableRecords(ctx, "organization-1", "datasource-1", "table-1", "account-1", 20, 0, "")
				return err
			},
			wantPermission: workspace_model.WorkspacePermissionDatabaseRecordView,
		},
		{
			name: "update records",
			call: func(ctx context.Context, svc *dataSourceService) error {
				_, err := svc.UpdateTableRecords(ctx, "organization-1", "datasource-1", "table-1", "account-1", dto.UpdateRecordRequest{})
				return err
			},
			wantPermission: workspace_model.WorkspacePermissionDatabaseRecordUpdate,
		},
		{
			name: "delete records",
			call: func(ctx context.Context, svc *dataSourceService) error {
				_, err := svc.DeleteTableRecords(ctx, "organization-1", "datasource-1", "table-1", "account-1", dto.DeleteRecordRequest{})
				return err
			},
			wantPermission: workspace_model.WorkspacePermissionDatabaseRecordDelete,
		},
		{
			name: "import records",
			call: func(ctx context.Context, svc *dataSourceService) error {
				_, err := svc.ImportTableRecords(ctx, "organization-1", "datasource-1", "table-1", "account-1", strings.NewReader(""), "records.xlsx", false)
				return err
			},
			wantPermission: workspace_model.WorkspacePermissionDatabaseImportExecute,
		},
		{
			name: "import records from upload file",
			call: func(ctx context.Context, svc *dataSourceService) error {
				_, err := svc.ImportTableRecordsFromUploadFile(ctx, "organization-1", "datasource-1", "table-1", "account-1", "file-1", false)
				return err
			},
			wantPermission: workspace_model.WorkspacePermissionDatabaseImportExecute,
		},
		{
			name: "analyze file for table",
			call: func(ctx context.Context, svc *dataSourceService) error {
				_, err := svc.AnalyzeFileForTable(ctx, "datasource-1", "account-1", "", nil, nil)
				return err
			},
			wantPermission: workspace_model.WorkspacePermissionDatabaseImportAnalyze,
		},
		{
			name: "analyze excel import",
			call: func(ctx context.Context, svc *dataSourceService) error {
				_, err := svc.AnalyzeExcelImport(ctx, "organization-1", "datasource-1", "account-1", dto.AnalyzeExcelImportRequest{UploadFileID: "file-1"})
				return err
			},
			wantPermission: workspace_model.WorkspacePermissionDatabaseImportAnalyze,
		},
		{
			name: "parse file for table ingest",
			call: func(ctx context.Context, svc *dataSourceService) error {
				_, err := svc.ParseFileForTableIngest(ctx, "organization-1", "account-1", dto.ParseFileForTableIngestRequest{TableID: "table-1"})
				return err
			},
			wantPermission: workspace_model.WorkspacePermissionDatabaseImportAnalyze,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspaceID := "workspace-1"
			authorization := &fakeAuthorizationService{allow: false}
			svc := newScopedDataSourceService(map[string]*model.DataSource{
				"datasource-1": {
					ID:             "datasource-1",
					OrganizationID: "organization-1",
					WorkspaceID:    &workspaceID,
				},
			}, map[string]*model.Table{
				"table-1": {
					ID:             "table-1",
					TableID:        1,
					OrganizationID: "organization-1",
					DataSourceID:   "datasource-1",
					Name:           "Users",
				},
			}, authorization)

			err := tt.call(context.Background(), svc)

			if err == nil || !strings.Contains(err.Error(), "workspace permission check failed") {
				t.Fatalf("%s error = %v, want workspace permission check failure", tt.name, err)
			}
			if len(authorization.workspaceRequests) != 1 {
				t.Fatalf("workspace permission checks = %d, want 1", len(authorization.workspaceRequests))
			}
			req := authorization.workspaceRequests[0]
			if req.OrganizationID != "organization-1" || req.WorkspaceID != "workspace-1" || req.AccountID != "account-1" {
				t.Fatalf("workspace permission request = %#v, want organization/workspace/account scope", req)
			}
			if len(req.PermissionCodes) != 1 || req.PermissionCodes[0] != tt.wantPermission {
				t.Fatalf("permission codes = %#v, want %s", req.PermissionCodes, tt.wantPermission)
			}
		})
	}
}

func TestGetTablePromptRejectsTableOutsideDataSource(t *testing.T) {
	workspaceID := "workspace-1"
	svc := newScopedDataSourceService(map[string]*model.DataSource{
		"datasource-1": {
			ID:             "datasource-1",
			OrganizationID: "organization-1",
			WorkspaceID:    &workspaceID,
		},
	}, map[string]*model.Table{
		"table-1": {
			ID:             "table-1",
			OrganizationID: "organization-1",
			DataSourceID:   "datasource-2",
			Name:           "Users",
		},
	}, nil)

	_, err := svc.GetTablePrompt(context.Background(), "organization-1", "datasource-1", "table-1", "account-1", "en")
	if !IsDataSourceTableNotFound(err) {
		t.Fatalf("GetTablePrompt error = %v, want data source table not found", err)
	}
}

func TestUpsertTablePromptRequiresDatabaseSchemaManagePermission(t *testing.T) {
	workspaceID := "workspace-1"
	authorization := &fakeAuthorizationService{allow: true}
	svc := newScopedDataSourceService(map[string]*model.DataSource{
		"datasource-1": {
			ID:             "datasource-1",
			OrganizationID: "organization-1",
			WorkspaceID:    &workspaceID,
		},
	}, map[string]*model.Table{
		"table-1": {
			ID:             "table-1",
			OrganizationID: "organization-1",
			DataSourceID:   "datasource-1",
			Name:           "Users",
		},
	}, authorization)

	got, err := svc.UpsertTablePrompt(context.Background(), "organization-1", "datasource-1", "table-1", "account-1", dto.UpdateTablePromptRequest{
		Prompt:    "ingest carefully",
		UpdatedBy: "account-1",
	})
	if err != nil {
		t.Fatalf("UpsertTablePrompt error = %v", err)
	}
	if got.Prompt != "ingest carefully" {
		t.Fatalf("prompt = %s, want ingest carefully", got.Prompt)
	}
	if len(authorization.workspaceRequests) != 1 {
		t.Fatalf("workspace permission checks = %d, want 1", len(authorization.workspaceRequests))
	}
	req := authorization.workspaceRequests[0]
	if req.WorkspaceID != "workspace-1" {
		t.Fatalf("workspace_id = %s, want workspace-1", req.WorkspaceID)
	}
	if len(req.PermissionCodes) != 1 || req.PermissionCodes[0] != workspace_model.WorkspacePermissionDatabaseSchemaManage {
		t.Fatalf("permission codes = %#v, want database.schema.manage", req.PermissionCodes)
	}
}

func TestListOperationLogsRequiresDatabaseOperationLogsViewPermission(t *testing.T) {
	workspaceID := "workspace-1"
	authorization := &fakeAuthorizationService{allow: true}
	svc := newScopedDataSourceService(map[string]*model.DataSource{
		"datasource-1": {
			ID:             "datasource-1",
			OrganizationID: "organization-1",
			WorkspaceID:    &workspaceID,
		},
	}, nil, authorization)
	sqlRepo := svc.sqlOperationRepo.(*fakeSQLOperationRepository)
	sqlRepo.logs = []*model.DataSourceSQLOperation{{ID: "operation-1"}}

	got, err := svc.ListOperationLogsByDataSourceIDWithFilters(context.Background(), "organization-1", "datasource-1", "auditor-1", dto.SQLOperationFilter{}, 20, 0)
	if err != nil {
		t.Fatalf("ListOperationLogsByDataSourceIDWithFilters error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "operation-1" {
		t.Fatalf("operation logs = %#v, want operation-1", got)
	}
	if len(authorization.workspaceRequests) != 1 {
		t.Fatalf("workspace permission checks = %d, want 1", len(authorization.workspaceRequests))
	}
	req := authorization.workspaceRequests[0]
	if req.WorkspaceID != "workspace-1" {
		t.Fatalf("workspace_id = %s, want workspace-1", req.WorkspaceID)
	}
	if len(req.PermissionCodes) != 1 || req.PermissionCodes[0] != workspace_model.WorkspacePermissionDatabaseOperationLogsView {
		t.Fatalf("permission codes = %#v, want database.operation_logs.view", req.PermissionCodes)
	}
}

func TestListOperationLogsRejectsTableFilterOutsideDataSource(t *testing.T) {
	workspaceID := "workspace-1"
	svc := newScopedDataSourceService(map[string]*model.DataSource{
		"datasource-1": {
			ID:             "datasource-1",
			OrganizationID: "organization-1",
			WorkspaceID:    &workspaceID,
		},
	}, map[string]*model.Table{
		"table-1": {
			ID:             "table-1",
			OrganizationID: "organization-1",
			DataSourceID:   "datasource-2",
			Name:           "Users",
		},
	}, nil)
	tableID := "table-1"

	_, err := svc.ListOperationLogsByDataSourceIDWithFilters(context.Background(), "organization-1", "datasource-1", "account-1", dto.SQLOperationFilter{TableID: &tableID}, 20, 0)
	if err == nil || !strings.Contains(err.Error(), "table with id 'table-1' not found") {
		t.Fatalf("ListOperationLogsByDataSourceIDWithFilters error = %v, want table not found", err)
	}
}

func TestListSQLAuditByWorkspaceRequiresDatabaseSQLAuditViewPermission(t *testing.T) {
	authorization := &fakeAuthorizationService{allow: true}
	svc := newScopedDataSourceService(nil, nil, authorization)
	sqlRepo := svc.sqlOperationRepo.(*fakeSQLOperationRepository)
	sqlRepo.auditLogs = []*model.DataSourceSQLOperation{{ID: "operation-1"}}

	got, err := svc.ListSQLAuditByWorkspace(context.Background(), "organization-1", "workspace-1", "auditor-1", dto.SQLAuditFilter{}, 20, 0)
	if err != nil {
		t.Fatalf("ListSQLAuditByWorkspace error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "operation-1" {
		t.Fatalf("audit logs = %#v, want operation-1", got)
	}
	if sqlRepo.listAuditCalls != 1 {
		t.Fatalf("list audit calls = %d, want 1", sqlRepo.listAuditCalls)
	}
	if len(authorization.workspaceRequests) != 1 {
		t.Fatalf("workspace permission checks = %d, want 1", len(authorization.workspaceRequests))
	}
	req := authorization.workspaceRequests[0]
	if req.OrganizationID != "organization-1" || req.WorkspaceID != "workspace-1" || req.AccountID != "auditor-1" {
		t.Fatalf("workspace permission request = %#v, want organization/workspace/account scope", req)
	}
	if len(req.PermissionCodes) != 1 || req.PermissionCodes[0] != workspace_model.WorkspacePermissionDatabaseSQLAuditView {
		t.Fatalf("permission codes = %#v, want database.sql_audit.view", req.PermissionCodes)
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
	if got.GuardAction == nil || *got.GuardAction != string(guard.ActionAllow) {
		t.Fatalf("guard_action = %#v, want allow", got.GuardAction)
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
	if got.GuardAction == nil || *got.GuardAction != string(guard.ActionDeny) {
		t.Fatalf("guard_action = %#v, want deny", got.GuardAction)
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
	if got.GuardAction == nil || *got.GuardAction != string(guard.ActionAllow) {
		t.Fatalf("guard_action = %#v, want allow", got.GuardAction)
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

func TestUpdateGuardPolicyRejectsInvalidMode(t *testing.T) {
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
	policy.Mode = "enfore"
	if _, err := svc.UpdateGuardPolicy(context.Background(), "organization-1", "datasource-1", policy); err == nil {
		t.Fatal("expected invalid policy error")
	}
}

func TestPreviewGuardRejectsInvalidMode(t *testing.T) {
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
	policy.Mode = "ENFORCE"
	if _, err := svc.PreviewGuard(context.Background(), "organization-1", "datasource-1", "SELECT 1", &policy); err == nil {
		t.Fatal("expected invalid policy error")
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
