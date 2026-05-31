package database

import (
	"context"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
	datasourcemodel "github.com/zgiai/zgi/api/internal/modules/datasource/model"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestListAccessibleDatabasesRequiresReadPermissions(t *testing.T) {
	provider := NewProvider(newDatabaseFakeDataSourceService(), &databaseFakeOrganizationService{
		aiQuery: true,
		view:    false,
	})
	tool, err := provider.GetTool(ToolListAccessibleDatabases)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	runtimeTool := tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
		},
	})

	messages, err := runtimeTool.Invoke(context.Background(), "acct-1", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	payload := messages[0].Data
	databases, ok := payload["databases"].([]map[string]interface{})
	if !ok {
		t.Fatalf("databases payload type = %T", payload["databases"])
	}
	if len(databases) != 0 {
		t.Fatalf("databases = %#v, want no rows without database.view", databases)
	}
}

func TestAgentBindingBlocksUnboundTable(t *testing.T) {
	provider := NewProvider(newDatabaseFakeDataSourceService(), &databaseFakeOrganizationService{
		aiQuery:  true,
		view:     true,
		dataEdit: true,
	})
	tool, err := provider.GetTool(ToolDescribeDatabaseTable)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	runtimeTool := tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAgent,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"database_bindings": []map[string]interface{}{
				{"data_source_id": "db-1", "table_ids": []string{"table-1"}},
			},
		},
	})

	_, err = runtimeTool.Invoke(context.Background(), "acct-1", map[string]interface{}{
		"data_source_id": "db-1",
		"table_id":       "table-2",
	}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not bound") {
		t.Fatalf("Invoke() error = %v, want unbound table rejection", err)
	}
}

func TestAgentDatabaseUsesBindingActorAccount(t *testing.T) {
	dataSources := newDatabaseFakeDataSourceService()
	organization := &databaseFakeOrganizationService{
		aiQuery: true,
		view:    true,
	}
	provider := NewProvider(dataSources, organization)
	tool, err := provider.GetTool(ToolQueryTableRecords)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	runtimeTool := tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAgent,
		RuntimeParameters: map[string]interface{}{
			"organization_id":              "org-1",
			"database_bound_by_account_id": "binder-1",
			"database_bindings": []map[string]interface{}{
				{"data_source_id": "db-1", "table_ids": []string{"table-1"}},
			},
		},
	})

	_, err = runtimeTool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"data_source_id": "db-1",
		"table_id":       "table-1",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if dataSources.lastQueryAccountID != "binder-1" {
		t.Fatalf("QueryTableRecords account = %q, want binder-1", dataSources.lastQueryAccountID)
	}
	if organization.lastAccountID != "binder-1" {
		t.Fatalf("permission check account = %q, want binder-1", organization.lastAccountID)
	}
}

func TestAgentDatabaseGrantSkipsRuntimePermissionsAndUsesWritableTables(t *testing.T) {
	dataSources := newDatabaseFakeDataSourceService()
	provider := NewProvider(dataSources, &databaseFakeOrganizationService{})
	tool, err := provider.GetTool(ToolInsertTableRecords)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	runtimeTool := tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAgent,
		RuntimeParameters: map[string]interface{}{
			"organization_id":              "org-1",
			"database_binding_grant":       true,
			"database_bound_by_account_id": "binder-1",
			"database_bindings": []map[string]interface{}{
				{"data_source_id": "db-1", "table_ids": []string{"table-1", "table-2"}, "writable_table_ids": []string{"table-1"}},
			},
		},
	})

	_, err = runtimeTool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"data_source_id": "db-1",
		"table_id":       "table-2",
		"records":        []map[string]interface{}{{"name": "Ada"}},
	}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not writable") {
		t.Fatalf("Invoke() error = %v, want writable binding rejection", err)
	}

	_, err = runtimeTool.Invoke(context.Background(), "caller-1", map[string]interface{}{
		"data_source_id": "db-1",
		"table_id":       "table-1",
		"records":        []map[string]interface{}{{"name": "Ada"}},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if dataSources.addedRecords != 1 {
		t.Fatalf("addedRecords = %d, want 1", dataSources.addedRecords)
	}
}

func TestInsertTableRecordsRequiresWritePermission(t *testing.T) {
	provider := NewProvider(newDatabaseFakeDataSourceService(), &databaseFakeOrganizationService{
		aiQuery: true,
		view:    true,
	})
	tool, err := provider.GetTool(ToolInsertTableRecords)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	runtimeTool := tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
		},
	})

	_, err = runtimeTool.Invoke(context.Background(), "acct-1", map[string]interface{}{
		"data_source_id": "db-1",
		"table_id":       "table-1",
		"records":        []map[string]interface{}{{"name": "Ada"}},
	}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "data edit or manage") {
		t.Fatalf("Invoke() error = %v, want write permission rejection", err)
	}
}

func TestInsertTableRecordsAcceptsStructuredRecordsString(t *testing.T) {
	dataSources := newDatabaseFakeDataSourceService()
	provider := NewProvider(dataSources, &databaseFakeOrganizationService{
		aiQuery:  true,
		view:     true,
		dataEdit: true,
	})
	tool, err := provider.GetTool(ToolInsertTableRecords)
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	runtimeTool := tool.ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
		},
	})

	messages, err := runtimeTool.Invoke(context.Background(), "acct-1", map[string]interface{}{
		"data_source_id": "db-1",
		"table_id":       "table-1",
		"records":        `[{"name":"Ada"}]`,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if dataSources.addedRecords != 1 {
		t.Fatalf("addedRecords = %d, want 1", dataSources.addedRecords)
	}
	if got := messages[0].Data["affected_rows"]; got != int64(1) {
		t.Fatalf("affected_rows = %#v, want 1", got)
	}
}

type databaseFakeDataSourceService struct {
	dataSources        []*dto.DataSourceResponse
	tables             []*datasourcemodel.Table
	addedRecords       int
	lastQueryAccountID string
}

func newDatabaseFakeDataSourceService() *databaseFakeDataSourceService {
	workspaceID := "workspace-1"
	return &databaseFakeDataSourceService{
		dataSources: []*dto.DataSourceResponse{{
			ID:             "db-1",
			OrganizationID: "org-1",
			WorkspaceID:    &workspaceID,
			Name:           "Operations",
			Description:    "Operational records",
			SchemaName:     "public",
			Status:         "active",
		}},
		tables: []*datasourcemodel.Table{
			{ID: "table-1", OrganizationID: "org-1", DataSourceID: "db-1", Name: "Customers", PhysicalTableName: "customers"},
			{ID: "table-2", OrganizationID: "org-1", DataSourceID: "db-1", Name: "Orders", PhysicalTableName: "orders"},
		},
	}
}

func (s *databaseFakeDataSourceService) ListDataSources(context.Context, string, string, []string) ([]*dto.DataSourceResponse, error) {
	return s.dataSources, nil
}

func (s *databaseFakeDataSourceService) GetDataSourceByID(_ context.Context, _ string, id string, _ string) (*dto.DataSourceResponse, error) {
	for _, dataSource := range s.dataSources {
		if dataSource.ID == id {
			return dataSource, nil
		}
	}
	return nil, nil
}

func (s *databaseFakeDataSourceService) ListTables(context.Context, string, string, string) ([]*datasourcemodel.Table, error) {
	return s.tables, nil
}

func (s *databaseFakeDataSourceService) GetTable(_ context.Context, _ string, dataSourceID, tableID string, _ string) (*datasourcemodel.Table, error) {
	for _, table := range s.tables {
		if table.DataSourceID == dataSourceID && table.ID == tableID {
			return table, nil
		}
	}
	return nil, nil
}

func (s *databaseFakeDataSourceService) GetTableColumns(context.Context, string, string, string, bool) (dto.GetTableColumnsResponse, error) {
	return dto.GetTableColumnsResponse{Columns: []dto.TableColumn{{Name: "name", Type: "varchar"}}}, nil
}

func (s *databaseFakeDataSourceService) QueryTableRecords(_ context.Context, _, _, _ string, accountID string, _ int, _ int, _ string) (dto.QueryRecordResponse, error) {
	s.lastQueryAccountID = accountID
	return dto.QueryRecordResponse{Data: []map[string]interface{}{{"id": 1}}, TotalNum: 1}, nil
}

func (s *databaseFakeDataSourceService) AddTableRecords(_ context.Context, _, _, _ string, _ string, req dto.AddRecordRequest) (dto.AddRecordResponse, error) {
	s.addedRecords += len(req.Records)
	return dto.AddRecordResponse{AffectedRows: int64(len(req.Records))}, nil
}

func (s *databaseFakeDataSourceService) UpdateTableRecords(context.Context, string, string, string, string, dto.UpdateRecordRequest) (dto.UpdateRecordResponse, error) {
	return dto.UpdateRecordResponse{AffectedRows: 1}, nil
}

func (s *databaseFakeDataSourceService) DeleteTableRecords(context.Context, string, string, string, string, dto.DeleteRecordRequest) (dto.DeleteRecordResponse, error) {
	return dto.DeleteRecordResponse{AffectedRows: 1}, nil
}

type databaseFakeOrganizationService struct {
	aiQuery       bool
	view          bool
	dataEdit      bool
	manage        bool
	lastAccountID string
}

func (s *databaseFakeOrganizationService) CheckWorkspacePermission(_ context.Context, _, _, accountID string, permission workspacemodel.WorkspacePermissionCode) (bool, error) {
	s.lastAccountID = accountID
	switch permission {
	case workspacemodel.WorkspacePermissionDatabaseAIQuery:
		return s.aiQuery, nil
	case workspacemodel.WorkspacePermissionDatabaseView:
		return s.view, nil
	default:
		return false, nil
	}
}

func (s *databaseFakeOrganizationService) CheckWorkspaceOrganizationAnyPermission(_ context.Context, _, _, accountID string, permissions ...workspacemodel.WorkspacePermissionCode) (bool, error) {
	s.lastAccountID = accountID
	for _, permission := range permissions {
		switch permission {
		case workspacemodel.WorkspacePermissionDatabaseDataEdit:
			if s.dataEdit {
				return true, nil
			}
		case workspacemodel.WorkspacePermissionDatabaseManage:
			if s.manage {
				return true, nil
			}
		}
	}
	return false, nil
}
