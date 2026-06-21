package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/dto"
	datasource_model "github.com/zgiai/zgi/api/internal/modules/datasource/model"
	datasource_service "github.com/zgiai/zgi/api/internal/modules/datasource/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestExcelImportMutationsRequireDatabaseManageBeforeBindingRequest(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		target        string
		params        gin.Params
		call          func(*DataSourceHandler, *gin.Context)
		wantAnalyze   int
		wantConfirm   int
		wantRecognize int
	}{
		{
			name:   "analyze",
			method: http.MethodPost,
			target: "/data-dbs/datasource-1/excel-import/analyze",
			params: gin.Params{
				{Key: "id", Value: "datasource-1"},
			},
			call: (*DataSourceHandler).AnalyzeExcelImport,
		},
		{
			name:   "confirm",
			method: http.MethodPost,
			target: "/data-dbs/datasource-1/excel-import/jobs/job-1/import",
			params: gin.Params{
				{Key: "id", Value: "datasource-1"},
				{Key: "job_id", Value: "job-1"},
			},
			call: (*DataSourceHandler).ConfirmExcelImport,
		},
		{
			name:   "recognize",
			method: http.MethodPost,
			target: "/data-dbs/datasource-1/excel-import/jobs/job-1/recognize",
			params: gin.Params{
				{Key: "id", Value: "datasource-1"},
				{Key: "job_id", Value: "job-1"},
			},
			call: (*DataSourceHandler).RecognizeExcelImportFields,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
			permissionChecker := &excelImportPermissionChecker{allowed: false}
			handler := &DataSourceHandler{
				service:             dataSourceService,
				organizationService: permissionChecker,
			}
			ctx, recorder := newExcelImportPermissionContext(tt.method, tt.target, tt.params)

			tt.call(handler, ctx)

			require.Equal(t, http.StatusForbidden, recorder.Code)
			requireResponseCode(t, recorder, response.ErrPermissionDenied)
			require.Equal(t, 1, dataSourceService.getDataSourceCalls)
			require.Equal(t, 1, permissionChecker.calls)
			require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseManage}, permissionChecker.lastPermissions)
			require.Zero(t, dataSourceService.analyzeCalls)
			require.Zero(t, dataSourceService.confirmCalls)
			require.Zero(t, dataSourceService.recognizeCalls)
		})
	}
}

func TestExcelImportReadRoutesRequireDatabasePermissionBeforeJobLookup(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		target          string
		params          gin.Params
		call            func(*DataSourceHandler, *gin.Context)
		wantPermissions []workspace_model.WorkspacePermissionCode
	}{
		{
			name:   "get job",
			method: http.MethodGet,
			target: "/data-dbs/datasource-1/excel-import/jobs/job-1",
			params: gin.Params{
				{Key: "id", Value: "datasource-1"},
				{Key: "job_id", Value: "job-1"},
			},
			call:            (*DataSourceHandler).GetExcelImportJob,
			wantPermissions: []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseView},
		},
		{
			name:   "list errors",
			method: http.MethodGet,
			target: "/data-dbs/datasource-1/excel-import/jobs/job-1/errors",
			params: gin.Params{
				{Key: "id", Value: "datasource-1"},
				{Key: "job_id", Value: "job-1"},
			},
			call:            (*DataSourceHandler).ListExcelImportErrors,
			wantPermissions: []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseManage},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
			permissionChecker := &excelImportPermissionChecker{allowed: false}
			handler := &DataSourceHandler{
				service:             dataSourceService,
				organizationService: permissionChecker,
			}
			ctx, recorder := newExcelImportPermissionContext(tt.method, tt.target, tt.params)

			tt.call(handler, ctx)

			require.Equal(t, http.StatusForbidden, recorder.Code)
			requireResponseCode(t, recorder, response.ErrPermissionDenied)
			require.Equal(t, 1, dataSourceService.getDataSourceCalls)
			require.Equal(t, 1, permissionChecker.calls)
			require.Equal(t, tt.wantPermissions, permissionChecker.lastPermissions)
			require.Zero(t, dataSourceService.getExcelImportJobCalls)
			require.Zero(t, dataSourceService.listExcelImportErrorsCalls)
		})
	}
}

func TestUpdateDataSourceRequiresDatabaseManageBeforeBindingRequest(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContext(
		http.MethodPut,
		"/data-dbs/datasource-1",
		gin.Params{{Key: "id", Value: "datasource-1"}},
	)

	handler.UpdateDataSource(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, dataSourceService.getDataSourceCalls)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseManage}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.updateCalls)
}

func TestDeleteDataSourceRequiresDatabaseManageBeforeMutation(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContext(
		http.MethodDelete,
		"/data-dbs/datasource-1",
		gin.Params{{Key: "id", Value: "datasource-1"}},
	)

	handler.DeleteDataSourceByID(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, dataSourceService.getDataSourceCalls)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseManage}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.deleteDataSourceCalls)
}

func TestCreateDataSourceRequiresDatabaseManageBeforeServiceMutation(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContextWithBody(
		http.MethodPost,
		"/data-dbs",
		nil,
		`{"name":"main","workspace_id":"workspace-1"}`,
	)

	handler.CreateDataSource(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseManage}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.createDataSourceCalls)
}

func TestCreateDataSourceRequiresDatabaseManageBeforeNameValidation(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContextWithBody(
		http.MethodPost,
		"/data-dbs",
		nil,
		`{"workspace_id":"workspace-1"}`,
	)

	handler.CreateDataSource(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseManage}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.createDataSourceCalls)
}

func TestCreateTableRequiresDatabaseManageBeforeBindingRequest(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContext(
		http.MethodPost,
		"/data-dbs/datasource-1/tables",
		gin.Params{{Key: "id", Value: "datasource-1"}},
	)

	handler.CreateTable(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, dataSourceService.getDataSourceCalls)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseManage}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.createTableCalls)
}

func TestListTablesRequiresDatabaseViewBeforeTableListing(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContext(
		http.MethodGet,
		"/data-dbs/datasource-1/tables",
		gin.Params{{Key: "id", Value: "datasource-1"}},
	)

	handler.ListTables(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, dataSourceService.getDataSourceCalls)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseView}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.listTablesCalls)
}

func TestGetTableRequiresDatabaseViewBeforeTableLookup(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContext(
		http.MethodGet,
		"/data-dbs/datasource-1/tables/table-1",
		gin.Params{
			{Key: "id", Value: "datasource-1"},
			{Key: "table_id", Value: "table-1"},
		},
	)

	handler.GetTable(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, dataSourceService.getDataSourceCalls)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseView}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.getTableCalls)
}

func TestDeleteTableRequiresDatabaseManageBeforeTableMutation(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContext(
		http.MethodDelete,
		"/data-dbs/datasource-1/tables/table-1",
		gin.Params{
			{Key: "id", Value: "datasource-1"},
			{Key: "table_id", Value: "table-1"},
		},
	)

	handler.DeleteTable(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, dataSourceService.getDataSourceCalls)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseManage}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.deleteTableCalls)
}

func TestUpdateTableRequiresDatabaseManageBeforeBindingRequest(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContext(
		http.MethodPut,
		"/data-dbs/datasource-1/tables/table-1",
		gin.Params{
			{Key: "id", Value: "datasource-1"},
			{Key: "table_id", Value: "table-1"},
		},
	)

	handler.UpdateTable(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, dataSourceService.getDataSourceCalls)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseManage}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.updateTableCalls)
}

func TestUpdateTableColumnsRequiresDatabaseManageBeforeBindingRequest(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContext(
		http.MethodPut,
		"/data-dbs/datasource-1/tables/table-1/columns",
		gin.Params{
			{Key: "id", Value: "datasource-1"},
			{Key: "table_id", Value: "table-1"},
		},
	)

	handler.UpdateTableColumns(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, dataSourceService.getDataSourceCalls)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseManage}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.updateTableColumnsCalls)
}

func TestGetTableColumnsRequiresDatabaseViewBeforeColumnLookup(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContext(
		http.MethodGet,
		"/data-dbs/datasource-1/tables/table-1/columns",
		gin.Params{
			{Key: "id", Value: "datasource-1"},
			{Key: "table_id", Value: "table-1"},
		},
	)

	handler.GetTableColumns(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, dataSourceService.getDataSourceCalls)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseView}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.getTableColumnsCalls)
}

func TestAddTableRecordsRequiresDatabaseEditBeforeBindingRequest(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContext(
		http.MethodPost,
		"/data-dbs/datasource-1/tables/table-1/records",
		gin.Params{
			{Key: "id", Value: "datasource-1"},
			{Key: "table_id", Value: "table-1"},
		},
	)

	handler.AddTableRecords(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, dataSourceService.getDataSourceCalls)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{
		workspace_model.WorkspacePermissionDatabaseManage,
		workspace_model.WorkspacePermissionDatabaseDataEdit,
	}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.addTableRecordsCalls)
}

func TestQueryTableRecordsRequiresDatabaseViewBeforeRecordLookup(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContext(
		http.MethodGet,
		"/data-dbs/datasource-1/tables/table-1/records",
		gin.Params{
			{Key: "id", Value: "datasource-1"},
			{Key: "table_id", Value: "table-1"},
		},
	)

	handler.QueryTableRecords(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, dataSourceService.getDataSourceCalls)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseView}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.queryTableRecordsCalls)
}

func TestDownloadTableTemplateRequiresDatabaseViewBeforeTemplateLookup(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContext(
		http.MethodGet,
		"/data-dbs/datasource-1/tables/table-1/template",
		gin.Params{
			{Key: "id", Value: "datasource-1"},
			{Key: "table_id", Value: "table-1"},
		},
	)

	handler.DownloadTableTemplate(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, dataSourceService.getDataSourceCalls)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseView}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.getTableCalls)
	require.Zero(t, dataSourceService.generateTableTemplateCalls)
}

func TestUpdateTableRecordsRequiresDatabaseEditBeforeBindingRequest(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContext(
		http.MethodPut,
		"/data-dbs/datasource-1/tables/table-1/records",
		gin.Params{
			{Key: "id", Value: "datasource-1"},
			{Key: "table_id", Value: "table-1"},
		},
	)

	handler.UpdateTableRecords(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, dataSourceService.getDataSourceCalls)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{
		workspace_model.WorkspacePermissionDatabaseManage,
		workspace_model.WorkspacePermissionDatabaseDataEdit,
	}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.updateTableRecordsCalls)
}

func TestDeleteTableRecordsRequiresDatabaseEditBeforeIDValidation(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContext(
		http.MethodDelete,
		"/data-dbs/datasource-1/tables/table-1/records",
		gin.Params{
			{Key: "id", Value: "datasource-1"},
			{Key: "table_id", Value: "table-1"},
		},
	)

	handler.DeleteTableRecords(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, dataSourceService.getDataSourceCalls)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{
		workspace_model.WorkspacePermissionDatabaseManage,
		workspace_model.WorkspacePermissionDatabaseDataEdit,
	}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.deleteTableRecordsCalls)
}

func TestUpsertTablePromptRequiresDatabaseManageBeforeBindingRequest(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContext(
		http.MethodPut,
		"/data-dbs/datasource-1/tables/table-1/prompt",
		gin.Params{
			{Key: "id", Value: "datasource-1"},
			{Key: "table_id", Value: "table-1"},
		},
	)

	handler.UpsertTablePrompt(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, dataSourceService.getDataSourceCalls)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseManage}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.upsertTablePromptCalls)
}

func TestImportTableRecordsRequiresDatabaseEditBeforeBindingRequest(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContext(
		http.MethodPost,
		"/data-dbs/datasource-1/tables/table-1/records/import",
		gin.Params{
			{Key: "id", Value: "datasource-1"},
			{Key: "table_id", Value: "table-1"},
		},
	)

	handler.ImportTableRecords(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, dataSourceService.getDataSourceCalls)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{
		workspace_model.WorkspacePermissionDatabaseManage,
		workspace_model.WorkspacePermissionDatabaseDataEdit,
	}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.importTableRecordsFromUploadFileCalls)
}

func TestAnalyzeFileForTableRequiresDatabaseManageBeforeModelValidation(t *testing.T) {
	dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
	permissionChecker := &excelImportPermissionChecker{allowed: false}
	handler := &DataSourceHandler{
		service:             dataSourceService,
		organizationService: permissionChecker,
	}
	ctx, recorder := newExcelImportPermissionContextWithBody(
		http.MethodPost,
		"/data-dbs/analyze-file-for-table",
		nil,
		`{"data_source_id":"datasource-1","model":{}}`,
	)

	handler.AnalyzeFileForTable(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, 1, dataSourceService.getDataSourceCalls)
	require.Equal(t, 1, permissionChecker.calls)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseManage}, permissionChecker.lastPermissions)
	require.Zero(t, dataSourceService.analyzeFileForTableCalls)
}

func TestDatabaseIngestionHelpersRequireDatabaseEditBeforeBodyValidation(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		body       string
		call       func(*DataSourceHandler, *gin.Context)
		assertZero func(*testing.T, *excelImportPermissionDataSourceService)
	}{
		{
			name:   "parse file",
			target: "/data-dbs/parse-file-for-table-ingest",
			body:   `{"table_id":"table-1"}`,
			call:   (*DataSourceHandler).ParseFileForTableIngest,
			assertZero: func(t *testing.T, svc *excelImportPermissionDataSourceService) {
				require.Zero(t, svc.parseFileForTableIngestCalls)
			},
		},
		{
			name:   "extract records",
			target: "/data-dbs/extract-text-to-table-records",
			body:   `{"table_id":"table-1","model":{}}`,
			call:   (*DataSourceHandler).ExtractTextToTableRecords,
			assertZero: func(t *testing.T, svc *excelImportPermissionDataSourceService) {
				require.Zero(t, svc.extractTextToTableRecordsCalls)
			},
		},
		{
			name:   "ingest file",
			target: "/data-dbs/ingest-file-to-table",
			body:   `{"table_id":"table-1","model":{}}`,
			call:   (*DataSourceHandler).IngestFileToTable,
			assertZero: func(t *testing.T, svc *excelImportPermissionDataSourceService) {
				require.Zero(t, svc.ingestFileToTableCalls)
			},
		},
		{
			name:   "batch ingest",
			target: "/data-dbs/batch-ingest-file-to-table",
			body:   `{"table_id":"table-1"}`,
			call:   (*DataSourceHandler).BatchIngestFileToTable,
			assertZero: func(t *testing.T, svc *excelImportPermissionDataSourceService) {
				require.Zero(t, svc.batchIngestFileToTableCalls)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
			permissionChecker := &excelImportPermissionChecker{allowed: false}
			handler := &DataSourceHandler{
				service:             dataSourceService,
				organizationService: permissionChecker,
			}
			ctx, recorder := newExcelImportPermissionContextWithBody(http.MethodPost, tt.target, nil, tt.body)

			tt.call(handler, ctx)

			require.Equal(t, http.StatusForbidden, recorder.Code)
			requireResponseCode(t, recorder, response.ErrPermissionDenied)
			require.Equal(t, 1, dataSourceService.resolveTableDataSourceCalls)
			require.Equal(t, 1, dataSourceService.getDataSourceCalls)
			require.Equal(t, 1, permissionChecker.calls)
			require.Equal(t, []workspace_model.WorkspacePermissionCode{
				workspace_model.WorkspacePermissionDatabaseManage,
				workspace_model.WorkspacePermissionDatabaseDataEdit,
			}, permissionChecker.lastPermissions)
			tt.assertZero(t, dataSourceService)
		})
	}
}

func TestSQLAuditRoutesRequireDatabaseManageBeforeServiceLookup(t *testing.T) {
	tests := []struct {
		name   string
		target string
		params gin.Params
		call   func(*DataSourceHandler, *gin.Context)
	}{
		{
			name:   "list",
			target: "/workspaces/workspace-1/sql-audit?page=bad",
			params: gin.Params{{Key: "workspace_id", Value: "workspace-1"}},
			call:   (*DataSourceHandler).ListSQLAuditByWorkspace,
		},
		{
			name:   "detail",
			target: "/workspaces/workspace-1/sql-audit/operation-1",
			params: gin.Params{
				{Key: "workspace_id", Value: "workspace-1"},
				{Key: "operation_id", Value: "operation-1"},
			},
			call: (*DataSourceHandler).GetSQLAuditDetail,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataSourceService := &excelImportPermissionDataSourceService{workspaceID: "workspace-1"}
			permissionChecker := &excelImportPermissionChecker{allowed: false}
			handler := &DataSourceHandler{
				service:             dataSourceService,
				organizationService: permissionChecker,
			}
			ctx, recorder := newExcelImportPermissionContext(http.MethodGet, tt.target, tt.params)

			tt.call(handler, ctx)

			require.Equal(t, http.StatusForbidden, recorder.Code)
			requireResponseCode(t, recorder, response.ErrPermissionDenied)
			require.Equal(t, 1, permissionChecker.calls)
			require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseManage}, permissionChecker.lastPermissions)
			require.Zero(t, dataSourceService.listSQLAuditCalls)
			require.Zero(t, dataSourceService.countSQLAuditCalls)
			require.Zero(t, dataSourceService.getSQLAuditDetailCalls)
		})
	}
}

func newExcelImportPermissionContext(method, target string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	return newExcelImportPermissionContextWithBody(method, target, params, "{")
}

func newExcelImportPermissionContextWithBody(method, target string, params gin.Params, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = params
	ctx.Set("account_id", "account-1")
	util.SetOrganizationScopeCompat(ctx, "org-1")
	return ctx, recorder
}

func requireResponseCode(t *testing.T, recorder *httptest.ResponseRecorder, expected response.ErrorCode) {
	t.Helper()
	var body response.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, strconv.Itoa(expected.Code), body.Code)
}

type excelImportPermissionDataSourceService struct {
	datasource_service.DataSourceService

	workspaceID                           string
	resolveTableDataSourceCalls           int
	getDataSourceCalls                    int
	analyzeCalls                          int
	analyzeFileForTableCalls              int
	parseFileForTableIngestCalls          int
	extractTextToTableRecordsCalls        int
	ingestFileToTableCalls                int
	batchIngestFileToTableCalls           int
	confirmCalls                          int
	recognizeCalls                        int
	getExcelImportJobCalls                int
	listExcelImportErrorsCalls            int
	createDataSourceCalls                 int
	updateCalls                           int
	deleteDataSourceCalls                 int
	createTableCalls                      int
	listTablesCalls                       int
	getTableCalls                         int
	deleteTableCalls                      int
	updateTableCalls                      int
	updateTableColumnsCalls               int
	getTableColumnsCalls                  int
	addTableRecordsCalls                  int
	queryTableRecordsCalls                int
	generateTableTemplateCalls            int
	updateTableRecordsCalls               int
	deleteTableRecordsCalls               int
	upsertTablePromptCalls                int
	importTableRecordsFromUploadFileCalls int
	listSQLAuditCalls                     int
	countSQLAuditCalls                    int
	getSQLAuditDetailCalls                int
}

func (s *excelImportPermissionDataSourceService) GetDataSourceByID(_ context.Context, organizationID, id, _ string) (*dto.DataSourceResponse, error) {
	s.getDataSourceCalls++
	workspaceID := s.workspaceID
	return &dto.DataSourceResponse{
		ID:             id,
		OrganizationID: organizationID,
		WorkspaceID:    &workspaceID,
	}, nil
}

func (s *excelImportPermissionDataSourceService) ResolveTableDataSourceID(_ context.Context, _, _ string) (string, error) {
	s.resolveTableDataSourceCalls++
	return "datasource-1", nil
}

func (s *excelImportPermissionDataSourceService) AnalyzeExcelImport(_ context.Context, _, _, _ string, _ dto.AnalyzeExcelImportRequest) (dto.AnalyzeExcelImportData, error) {
	s.analyzeCalls++
	return dto.AnalyzeExcelImportData{}, nil
}

func (s *excelImportPermissionDataSourceService) AnalyzeFileForTable(_ context.Context, _, _, _ string, _ *string, _ *dto.ModelSpec) (dto.AnalyzeFileForTableResponse, error) {
	s.analyzeFileForTableCalls++
	return dto.AnalyzeFileForTableResponse{}, nil
}

func (s *excelImportPermissionDataSourceService) ParseFileForTableIngest(_ context.Context, _, _ string, _ dto.ParseFileForTableIngestRequest) (dto.ParseFileForTableIngestResponse, error) {
	s.parseFileForTableIngestCalls++
	return dto.ParseFileForTableIngestResponse{}, nil
}

func (s *excelImportPermissionDataSourceService) ExtractTextToTableRecords(_ context.Context, _, _ string, _ dto.ExtractTextToTableRecordsRequest) (dto.ExtractTextToTableRecordsResponse, error) {
	s.extractTextToTableRecordsCalls++
	return dto.ExtractTextToTableRecordsResponse{}, nil
}

func (s *excelImportPermissionDataSourceService) IngestFileToTable(_ context.Context, _, _ string, _ dto.IngestFileToTableRequest) (dto.IngestFileToTableResponse, error) {
	s.ingestFileToTableCalls++
	return dto.IngestFileToTableResponse{}, nil
}

func (s *excelImportPermissionDataSourceService) BatchIngestFileToTable(_ context.Context, _, _ string, _ dto.BatchIngestFileToTableRequest) (dto.BatchIngestFileToTableResponse, error) {
	s.batchIngestFileToTableCalls++
	return dto.BatchIngestFileToTableResponse{}, nil
}

func (s *excelImportPermissionDataSourceService) ConfirmExcelImport(_ context.Context, _, _, _, _ string, _ dto.ConfirmExcelImportRequest) (dto.ConfirmExcelImportData, error) {
	s.confirmCalls++
	return dto.ConfirmExcelImportData{}, nil
}

func (s *excelImportPermissionDataSourceService) RecognizeExcelImportFields(_ context.Context, _, _, _, _ string, _ dto.RecognizeExcelImportRequest) (dto.RecognizeExcelImportData, error) {
	s.recognizeCalls++
	return dto.RecognizeExcelImportData{}, nil
}

func (s *excelImportPermissionDataSourceService) GetExcelImportJob(_ context.Context, _, _, _ string) (*dto.ExcelImportJobResponse, error) {
	s.getExcelImportJobCalls++
	return &dto.ExcelImportJobResponse{}, nil
}

func (s *excelImportPermissionDataSourceService) ListExcelImportErrors(_ context.Context, _, _, _ string, _, _ int) (dto.ExcelImportErrorList, error) {
	s.listExcelImportErrorsCalls++
	return dto.ExcelImportErrorList{}, nil
}

func (s *excelImportPermissionDataSourceService) CreateDataSource(_ context.Context, organizationID string, _ string, req dto.CreateDataSourceRequest) (*dto.DataSourceResponse, error) {
	s.createDataSourceCalls++
	return &dto.DataSourceResponse{
		ID:             "datasource-1",
		OrganizationID: organizationID,
		WorkspaceID:    req.WorkspaceID,
	}, nil
}

func (s *excelImportPermissionDataSourceService) UpdateDataSource(_ context.Context, organizationID, id, _ string, _ dto.UpdateDataSourceRequest) (*dto.DataSourceResponse, error) {
	s.updateCalls++
	workspaceID := s.workspaceID
	return &dto.DataSourceResponse{
		ID:             id,
		OrganizationID: organizationID,
		WorkspaceID:    &workspaceID,
	}, nil
}

func (s *excelImportPermissionDataSourceService) DeleteDataSourceByID(_ context.Context, _, _ string, _ string) error {
	s.deleteDataSourceCalls++
	return nil
}

func (s *excelImportPermissionDataSourceService) CreateTable(_ context.Context, _, _ string, _ string, _ dto.CreateTableRequest) (*datasource_model.Table, error) {
	s.createTableCalls++
	return &datasource_model.Table{}, nil
}

func (s *excelImportPermissionDataSourceService) ListTables(_ context.Context, _, _ string, _ string) ([]*datasource_model.Table, error) {
	s.listTablesCalls++
	return []*datasource_model.Table{}, nil
}

func (s *excelImportPermissionDataSourceService) GetTable(_ context.Context, _, _, _ string, _ string) (*datasource_model.Table, error) {
	s.getTableCalls++
	return &datasource_model.Table{}, nil
}

func (s *excelImportPermissionDataSourceService) DeleteTable(_ context.Context, _, _, _, _ string) error {
	s.deleteTableCalls++
	return nil
}

func (s *excelImportPermissionDataSourceService) UpdateTable(_ context.Context, _, _, _ string, _ string, _ dto.UpdateTableRequest) (*datasource_model.Table, error) {
	s.updateTableCalls++
	return &datasource_model.Table{}, nil
}

func (s *excelImportPermissionDataSourceService) UpdateTableColumns(_ context.Context, _, _, _, _ string, _ dto.UpdateTableColumnsRequest) error {
	s.updateTableColumnsCalls++
	return nil
}

func (s *excelImportPermissionDataSourceService) GetTableColumns(_ context.Context, _, _, _ string, _ bool) (dto.GetTableColumnsResponse, error) {
	s.getTableColumnsCalls++
	return dto.GetTableColumnsResponse{}, nil
}

func (s *excelImportPermissionDataSourceService) AddTableRecords(_ context.Context, _, _, _, _ string, _ dto.AddRecordRequest) (dto.AddRecordResponse, error) {
	s.addTableRecordsCalls++
	return dto.AddRecordResponse{}, nil
}

func (s *excelImportPermissionDataSourceService) QueryTableRecords(_ context.Context, _, _, _, _ string, _ int, _ int, _ string) (dto.QueryRecordResponse, error) {
	s.queryTableRecordsCalls++
	return dto.QueryRecordResponse{}, nil
}

func (s *excelImportPermissionDataSourceService) GenerateTableTemplateExcel(_ context.Context, _, _, _ string) ([]byte, error) {
	s.generateTableTemplateCalls++
	return []byte("template"), nil
}

func (s *excelImportPermissionDataSourceService) UpdateTableRecords(_ context.Context, _, _, _, _ string, _ dto.UpdateRecordRequest) (dto.UpdateRecordResponse, error) {
	s.updateTableRecordsCalls++
	return dto.UpdateRecordResponse{}, nil
}

func (s *excelImportPermissionDataSourceService) DeleteTableRecords(_ context.Context, _, _, _, _ string, _ dto.DeleteRecordRequest) (dto.DeleteRecordResponse, error) {
	s.deleteTableRecordsCalls++
	return dto.DeleteRecordResponse{}, nil
}

func (s *excelImportPermissionDataSourceService) UpsertTablePrompt(_ context.Context, _, _, _, _ string, _ dto.UpdateTablePromptRequest) (*datasource_model.TablePrompt, error) {
	s.upsertTablePromptCalls++
	return &datasource_model.TablePrompt{}, nil
}

func (s *excelImportPermissionDataSourceService) ImportTableRecordsFromUploadFile(_ context.Context, _, _, _, _, _ string) (dto.ImportRecordResponse, error) {
	s.importTableRecordsFromUploadFileCalls++
	return dto.ImportRecordResponse{}, nil
}

func (s *excelImportPermissionDataSourceService) ListSQLAuditByWorkspace(_ context.Context, _, _, _ string, _ dto.SQLAuditFilter, _, _ int) ([]*datasource_model.DataSourceSQLOperation, error) {
	s.listSQLAuditCalls++
	return []*datasource_model.DataSourceSQLOperation{}, nil
}

func (s *excelImportPermissionDataSourceService) CountSQLAuditByWorkspace(_ context.Context, _, _, _ string, _ dto.SQLAuditFilter) (int64, error) {
	s.countSQLAuditCalls++
	return 0, nil
}

func (s *excelImportPermissionDataSourceService) GetSQLAuditDetail(_ context.Context, _, _, _, _ string) (*datasource_model.DataSourceSQLOperation, error) {
	s.getSQLAuditDetailCalls++
	return &datasource_model.DataSourceSQLOperation{}, nil
}

type excelImportPermissionChecker struct {
	interfaces.OrganizationService

	allowed         bool
	calls           int
	lastPermissions []workspace_model.WorkspacePermissionCode
}

func (c *excelImportPermissionChecker) CheckWorkspaceOrganizationAnyPermission(_ context.Context, _, _, _ string, permissionCodes ...workspace_model.WorkspacePermissionCode) (bool, error) {
	c.calls++
	c.lastPermissions = append([]workspace_model.WorkspacePermissionCode(nil), permissionCodes...)
	return c.allowed, nil
}
