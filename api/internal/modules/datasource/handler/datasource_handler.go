package handler

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/datasource/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	shared_visibility "github.com/zgiai/zgi/api/internal/modules/shared/visibility"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
	"github.com/zgiai/zgi/api/pkg/sql_base/guard"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

// DataSourceHandler handles HTTP requests for data sources
type DataSourceHandler struct {
	service             service.DataSourceService
	accountService      interfaces.AccountService
	organizationService interfaces.OrganizationService
}

var databaseExistingAssetVisibilityPermissions = []workspace_model.WorkspacePermissionCode{
	workspace_model.WorkspacePermissionDatabaseUpdate,
	workspace_model.WorkspacePermissionDatabaseDelete,
	workspace_model.WorkspacePermissionDatabaseMove,
	workspace_model.WorkspacePermissionDatabaseSchemaView,
	workspace_model.WorkspacePermissionDatabaseSchemaManage,
	workspace_model.WorkspacePermissionDatabaseRecordView,
	workspace_model.WorkspacePermissionDatabaseRecordCreate,
	workspace_model.WorkspacePermissionDatabaseRecordUpdate,
	workspace_model.WorkspacePermissionDatabaseRecordDelete,
	workspace_model.WorkspacePermissionDatabaseImportAnalyze,
	workspace_model.WorkspacePermissionDatabaseImportExecute,
	workspace_model.WorkspacePermissionDatabaseImportErrorsView,
	workspace_model.WorkspacePermissionDatabaseGuardPolicyManage,
	workspace_model.WorkspacePermissionDatabaseTablePromptView,
	workspace_model.WorkspacePermissionDatabaseTablePromptManage,
	workspace_model.WorkspacePermissionDatabaseOperationLogsView,
	workspace_model.WorkspacePermissionDatabaseSQLAuditView,
	workspace_model.WorkspacePermissionDatabaseAIQueryRead,
	workspace_model.WorkspacePermissionDatabaseAIQueryWrite,
}

var databaseWorkspaceVisibilityPermissions = append(
	[]workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionDatabaseCreate},
	databaseExistingAssetVisibilityPermissions...,
)

// NewDataSourceHandler creates a new DataSourceHandler
func NewDataSourceHandler(service service.DataSourceService, accountService interfaces.AccountService, enterpriseService interfaces.OrganizationService) *DataSourceHandler {
	return &DataSourceHandler{
		service:             service,
		accountService:      accountService,
		organizationService: enterpriseService,
	}
}

func sanitizeDownloadFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "table"
	}

	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(name)
}

// UpdateDataSource updates an existing data source
// @Summary Update data source
// @Description Update an existing data source's name, icon, permission, and workspace scope
// @Tags Data Source
// @Accept json
// @Produce json
// @Param id path string true "Data source ID"
// @Param dataSource body dto.UpdateDataSourceRequest true "Data source update request"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id} [put]
func (h *DataSourceHandler) UpdateDataSource(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	id := c.Param("id")
	if id == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(c, organizationID, id, accountID, workspace_model.WorkspacePermissionDatabaseUpdate) {
		return
	}

	var req dto.UpdateDataSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	// Update data source
	dataSource, err := h.service.UpdateDataSource(c.Request.Context(), organizationID, id, accountID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, dataSource)
}

// CreateDataSource creates a new data source
// @Summary Create a new data source
// @Description Create a new data source for the tenant
// @Tags Data Source
// @Accept json
// @Produce json
// @Param dataSource body dto.CreateDataSourceRequest true "Data source to create"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs [post]
func (h *DataSourceHandler) CreateDataSource(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	var workspaceReq struct {
		WorkspaceID *string `json:"workspace_id"`
	}
	if err := c.ShouldBindBodyWith(&workspaceReq, binding.JSON); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	if workspaceReq.WorkspaceID == nil || strings.TrimSpace(*workspaceReq.WorkspaceID) == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "workspace_id is required")
		return
	}
	workspaceID := strings.TrimSpace(*workspaceReq.WorkspaceID)

	if !h.ensureDatabaseWorkspacePermission(c, organizationID, workspaceID, accountID, workspace_model.WorkspacePermissionDatabaseCreate) {
		return
	}

	var req dto.CreateDataSourceRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}
	req.WorkspaceID = &workspaceID

	// Create data source
	dataSource, err := h.service.CreateDataSource(c.Request.Context(), organizationID, accountID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, dataSource)
}

// ListDataSources lists all data sources for a tenant
// @Summary List data sources
// @Description List all data sources for the tenant with permission filtering
// @Tags Data Source
// @Produce json
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs [get]
func (h *DataSourceHandler) ListDataSources(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}
	accountID := c.GetString("account_id")

	filterWorkspaceID := c.Query("workspace_id")

	var filteredWorkspaceIDs []string
	if h.organizationService != nil {
		var err error
		filteredWorkspaceIDs, err = shared_visibility.ResolveVisibleWorkspaceIDs(
			c.Request.Context(),
			h.organizationService,
			organizationID,
			accountID,
			filterWorkspaceID,
			databaseWorkspaceVisibilityPermissions...,
		)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if len(filteredWorkspaceIDs) == 0 {
			response.Success(c, []*dto.DataSourceResponse{})
			return
		}
	}

	// List data sources with permission filtering at query time
	dataSources, err := h.service.ListDataSources(c.Request.Context(), organizationID, accountID, filteredWorkspaceIDs)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, dataSources)
}

// GetDataSourceByID gets a specific data source by ID
// @Summary Get data source by ID
// @Description Get a specific data source by ID
// @Tags Data Source
// @Produce json
// @Param id path string true "Data source ID"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id} [get]
func (h *DataSourceHandler) GetDataSourceByID(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	id := c.Param("id")
	if id == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(
		c,
		organizationID,
		id,
		accountID,
		databaseExistingAssetVisibilityPermissions...,
	) {
		return
	}

	// Get data source
	dataSource, err := h.service.GetDataSourceByID(c.Request.Context(), organizationID, id, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	if dataSource == nil {
		response.Fail(c, response.ErrNotFound)
		return
	}

	response.Success(c, dataSource)
}

// DeleteDataSourceByID deletes a data source by ID
// @Summary Delete data source by ID
// @Description Delete a specific data source by ID
// @Tags Data Source
// @Produce json
// @Param id path string true "Data source ID"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id} [delete]
func (h *DataSourceHandler) DeleteDataSourceByID(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	id := c.Param("id")
	if id == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(c, organizationID, id, accountID, workspace_model.WorkspacePermissionDatabaseDelete) {
		return
	}

	// Delete data source
	err := h.service.DeleteDataSourceByID(c.Request.Context(), organizationID, id, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"message": "data source deleted successfully"})
}

func (h *DataSourceHandler) GetGuardPolicy(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}
	id := c.Param("id")
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	if !h.ensureDatabaseManage(c, organizationID, id, accountID) {
		return
	}
	policy, err := h.service.GetGuardPolicy(c.Request.Context(), organizationID, id)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, dto.GuardPolicyResponse{Policy: policy})
}

func (h *DataSourceHandler) UpdateGuardPolicy(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}
	id := c.Param("id")
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	var req dto.UpdateGuardPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}
	if !h.ensureDatabaseManage(c, organizationID, id, accountID) {
		return
	}
	if h.organizationService != nil {
		currentPolicy, err := h.service.GetGuardPolicy(c.Request.Context(), organizationID, id)
		if err != nil {
			response.FailWithMessage(c, response.ErrSystemError, err.Error())
			return
		}
		if req.Policy.Mode == "enforce" || currentPolicy.Mode == "enforce" {
			if !h.ensureDatabasePermission(c, organizationID, id, accountID, workspace_model.WorkspacePermissionDatabaseGuardPolicyManage) {
				return
			}
		}
	}
	policy, err := h.service.UpdateGuardPolicy(c.Request.Context(), organizationID, id, req.Policy)
	if err != nil {
		if errors.Is(err, guard.ErrInvalidPolicy) {
			response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
			return
		}
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, dto.GuardPolicyResponse{Policy: policy})
}

func (h *DataSourceHandler) PreviewGuard(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}
	id := c.Param("id")
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	var req dto.PreviewGuardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}
	if !h.ensureDatabaseManage(c, organizationID, id, accountID) {
		return
	}
	result, err := h.service.PreviewGuard(c.Request.Context(), organizationID, id, req.SQL, req.Policy)
	if err != nil {
		if errors.Is(err, guard.ErrInvalidPolicy) {
			response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
			return
		}
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, dto.PreviewGuardResponse{Result: result})
}

func (h *DataSourceHandler) ensureDatabaseManage(c *gin.Context, organizationID, dataSourceID, accountID string) bool {
	return h.ensureDatabasePermission(c, organizationID, dataSourceID, accountID, workspace_model.WorkspacePermissionDatabaseGuardPolicyManage)
}

// CreateTable creates a new table in a data source
// @Summary Create a new table
// @Description Create a new table in the specified data source
// @Tags Data Source Tables
// @Accept json
// @Produce json
// @Param id path string true "Data source ID"
// @Param table body dto.CreateTableRequest true "Table to create"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id}/tables [post]
func (h *DataSourceHandler) CreateTable(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	id := c.Param("id")
	if id == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(c, organizationID, id, accountID, workspace_model.WorkspacePermissionDatabaseSchemaManage) {
		return
	}

	var req dto.CreateTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	// Create table in data source
	table, err := h.service.CreateTable(c.Request.Context(), organizationID, id, accountID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, table)
}

// ListTables lists all tables in a data source
// @Summary List tables
// @Description List all tables in the specified data source
// @Tags Data Source Tables
// @Produce json
// @Param id path string true "Data source ID"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id}/tables [get]
func (h *DataSourceHandler) ListTables(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	id := c.Param("id")
	if id == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(c, organizationID, id, accountID, workspace_model.WorkspacePermissionDatabaseSchemaView) {
		return
	}

	// List tables in data source
	tables, err := h.service.ListTables(c.Request.Context(), organizationID, id, accountID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, tables)
}

// GetTable gets a specific table in a data source
// @Summary Get table
// @Description Get a specific table in the specified data source
// @Tags Data Source Tables
// @Produce json
// @Param id path string true "Data source ID"
// @Param table_id path string true "Table ID"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id}/tables/{table_id} [get]
func (h *DataSourceHandler) GetTable(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	id := c.Param("id")
	if id == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id is required")
		return
	}

	tableID := c.Param("table_id")
	if tableID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "table id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(c, organizationID, id, accountID, workspace_model.WorkspacePermissionDatabaseSchemaView) {
		return
	}

	// Get table in data source
	table, err := h.service.GetTable(c.Request.Context(), organizationID, id, tableID, accountID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	if table == nil {
		response.Fail(c, response.ErrNotFound)
		return
	}

	response.Success(c, table)
}

// DeleteTable deletes a table in a data source
// @Summary Delete table
// @Description Delete a specific table in the specified data source
// @Tags Data Source Tables
// @Produce json
// @Param id path string true "Data source ID"
// @Param table_id path string true "Table ID"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id}/tables/{table_id} [delete]
func (h *DataSourceHandler) DeleteTable(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	id := c.Param("id")
	if id == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id is required")
		return
	}

	tableID := c.Param("table_id")
	if tableID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "table id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(c, organizationID, id, accountID, workspace_model.WorkspacePermissionDatabaseSchemaManage) {
		return
	}

	// Delete table in data source
	err := h.service.DeleteTable(c.Request.Context(), organizationID, id, tableID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"message": "table deleted successfully"})
}

// UpdateTable updates a table's metadata (name and/or description)
// @Summary Update table metadata
// @Description Update a table's name and/or description
// @Tags Data Source Tables
// @Accept json
// @Produce json
// @Param id path string true "Data source ID"
// @Param table_id path string true "Table ID"
// @Param table body dto.UpdateTableRequest true "Table metadata to update"
// @Success 200 {object} response.Response{data=model.Table}
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id}/tables/{table_id} [put]
func (h *DataSourceHandler) UpdateTable(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	dataSourceID := c.Param("id")
	if dataSourceID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id is required")
		return
	}

	tableID := c.Param("table_id")
	if tableID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "table id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(c, organizationID, dataSourceID, accountID, workspace_model.WorkspacePermissionDatabaseSchemaManage) {
		return
	}

	var req dto.UpdateTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	// Update table metadata
	table, err := h.service.UpdateTable(c.Request.Context(), organizationID, dataSourceID, tableID, accountID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	if table == nil {
		response.Fail(c, response.ErrNotFound)
		return
	}

	response.Success(c, table)
}

// UpdateTableColumns updates the columns of a table
// @Summary Update table columns
// @Description Update the columns of a specific table in the specified data source (full replacement)
// @Tags Data Source Tables
// @Accept json
// @Produce json
// @Param id path string true "Data source ID"
// @Param table_id path string true "Table ID"
// @Param columns body dto.UpdateTableColumnsRequest true "Columns to update"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id}/tables/{table_id}/columns [put]
func (h *DataSourceHandler) UpdateTableColumns(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	id := c.Param("id")
	if id == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id is required")
		return
	}

	tableID := c.Param("table_id")
	if tableID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "table id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(c, organizationID, id, accountID, workspace_model.WorkspacePermissionDatabaseSchemaManage) {
		return
	}

	var req dto.UpdateTableColumnsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	// Update table columns
	err := h.service.UpdateTableColumns(c.Request.Context(), organizationID, id, tableID, accountID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, gin.H{"message": "table columns updated successfully"})
}

// GetTableColumns retrieves the columns of a specific table
// @Summary Get table columns
// @Description Retrieve the columns of the specified table
// @Tags Data Source Tables
// @Produce json
// @Param id path string true "Data source ID"
// @Param table_id path string true "Table ID"
// @Success 200 {object} response.Response{data=dto.GetTableColumnsResponse}
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id}/tables/{table_id}/columns [get]
func (h *DataSourceHandler) GetTableColumns(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	dataSourceID := c.Param("id")
	if dataSourceID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id is required")
		return
	}

	tableID := c.Param("table_id")
	if tableID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "table id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(c, organizationID, dataSourceID, accountID, workspace_model.WorkspacePermissionDatabaseSchemaView) {
		return
	}

	// Check if system fields should be included
	includeSystemFields := c.Query("include_system_fields") == "true"

	// Get table columns
	columns, err := h.service.GetTableColumns(c.Request.Context(), organizationID, dataSourceID, tableID, includeSystemFields)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, columns)
}

// AddTableRecords adds records to a table
// @Summary Add records to table
// @Description Add one or more records to the specified table
// @Tags Data Source Tables
// @Accept json
// @Produce json
// @Param id path string true "Data source ID"
// @Param table_id path string true "Table ID"
// @Param records body dto.AddRecordRequest true "Records to add"
// @Success 200 {object} response.Response{data=dto.AddRecordResponse}
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id}/tables/{table_id}/records [post]
func (h *DataSourceHandler) AddTableRecords(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	dataSourceID := c.Param("id")
	if dataSourceID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id is required")
		return
	}

	tableID := c.Param("table_id")
	if tableID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "table id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(
		c,
		organizationID,
		dataSourceID,
		accountID,
		workspace_model.WorkspacePermissionDatabaseRecordCreate,
	) {
		return
	}

	var req dto.AddRecordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	// Add records to table
	result, err := h.service.AddTableRecords(c.Request.Context(), organizationID, dataSourceID, tableID, accountID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// QueryTableRecords queries data from a table
// @Summary Query table data
// @Description Query data from the specified table with pagination
// @Tags Data Source Tables
// @Produce json
// @Param id path string true "Data source ID"
// @Param table_id path string true "Table ID"
// @Param limit query int false "Number of records to return (default: 20)"
// @Param offset query int false "Offset for pagination (default: 0)"
// @Param order query string false "Order by clause (default: id DESC)"
// @Success 200 {object} response.Response{data=dto.QueryRecordResponse}
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id}/tables/{table_id}/records [get]
func (h *DataSourceHandler) QueryTableRecords(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	dataSourceID := c.Param("id")
	if dataSourceID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id is required")
		return
	}

	tableID := c.Param("table_id")
	if tableID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "table id is required")
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	order := c.DefaultQuery("order", "id DESC")

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(c, organizationID, dataSourceID, accountID, workspace_model.WorkspacePermissionDatabaseRecordView) {
		return
	}

	data, err := h.service.QueryTableRecords(c.Request.Context(), organizationID, dataSourceID, tableID, accountID, limit, offset, order)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, data)
}

// UpdateTableRecords updates existing records in a table
// @Summary Update table records
// @Description Update one or more existing records in the specified table
// @Tags Data Source Tables
// @Accept json
// @Produce json
// @Param id path string true "Data source ID"
// @Param table_id path string true "Table ID"
// @Param records body dto.UpdateRecordRequest true "Records to update"
// @Success 200 {object} response.Response{data=dto.UpdateRecordResponse}
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id}/tables/{table_id}/records [put]
func (h *DataSourceHandler) UpdateTableRecords(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	dataSourceID := c.Param("id")
	if dataSourceID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id is required")
		return
	}

	tableID := c.Param("table_id")
	if tableID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "table id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(
		c,
		organizationID,
		dataSourceID,
		accountID,
		workspace_model.WorkspacePermissionDatabaseRecordUpdate,
	) {
		return
	}

	var req dto.UpdateRecordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	// Update records in table
	result, err := h.service.UpdateTableRecords(c.Request.Context(), organizationID, dataSourceID, tableID, accountID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// DeleteTableRecords deletes records from a table
// @Summary Delete table records
// @Description Delete one or more records from the specified table
// @Tags Data Source Tables
// @Produce json
// @Param id path string true "Data source ID"
// @Param table_id path string true "Table ID"
// @Param ids query []string true "Record IDs to delete"
// @Success 200 {object} response.Response{data=dto.DeleteRecordResponse}
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id}/tables/{table_id}/records [delete]
func (h *DataSourceHandler) DeleteTableRecords(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	dataSourceID := c.Param("id")
	if dataSourceID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id is required")
		return
	}

	tableID := c.Param("table_id")
	if tableID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "table id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(
		c,
		organizationID,
		dataSourceID,
		accountID,
		workspace_model.WorkspacePermissionDatabaseRecordDelete,
	) {
		return
	}

	// Get IDs from query parameters
	ids := c.QueryArray("ids")
	if len(ids) == 0 {
		response.FailWithMessage(c, response.ErrInvalidParam, "at least one record id is required")
		return
	}

	// Construct DeleteRecordRequest from IDs
	var req dto.DeleteRecordRequest
	for _, id := range ids {
		req.Records = append(req.Records, map[string]interface{}{"id": id})
	}

	// Delete records from table
	result, err := h.service.DeleteTableRecords(c.Request.Context(), organizationID, dataSourceID, tableID, accountID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// AnalyzeFileForTable analyzes a file and returns inferred table structure
// @Summary Analyze file for table structure
// @Description Analyze a file and return inferred table structure
// @Tags Data Source Tables
// @Accept json
// @Produce json
// @Param request body dto.AnalyzeFileForTableRequest true "File analysis request"
// @Success 200 {object} response.Response{data=dto.AnalyzeFileForTableResponse}
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/analyze-file-for-table [post]
func (h *DataSourceHandler) AnalyzeFileForTable(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var scopeReq struct {
		DataSourceID string `json:"data_source_id" binding:"required"`
	}
	if err := c.ShouldBindBodyWith(&scopeReq, binding.JSON); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	if !h.ensureDatabasePermission(c, organizationID, scopeReq.DataSourceID, accountID, workspace_model.WorkspacePermissionDatabaseImportAnalyze) {
		return
	}

	var req dto.AnalyzeFileForTableRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	// Extract fileID if provided
	var fileID string
	if req.FileID != nil {
		fileID = *req.FileID
	}

	// Analyze file for table structure using dataSourceID to resolve organization scope
	result, err := h.service.AnalyzeFileForTable(c.Request.Context(), req.DataSourceID, accountID, fileID, req.Prompt, req.Model)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// DownloadTableTemplate downloads an Excel template for the specified table
// @Summary Download table template
// @Description Download an Excel template with column headers matching the table structure for data import
// @Tags Data Source Tables
// @Produce application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param id path string true "Data source ID"
// @Param table_id path string true "Table ID"
// @Success 200 {file} file "Excel template file"
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id}/tables/{table_id}/template [get]
func (h *DataSourceHandler) DownloadTableTemplate(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	dataSourceID := c.Param("id")
	if dataSourceID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id is required")
		return
	}

	tableID := c.Param("table_id")
	if tableID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "table id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(c, organizationID, dataSourceID, accountID, workspace_model.WorkspacePermissionDatabaseSchemaView) {
		return
	}

	table, err := h.service.GetTable(c.Request.Context(), organizationID, dataSourceID, tableID, accountID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, "failed to get table: "+err.Error())
		return
	}

	// Generate Excel template through service
	excelData, err := h.service.GenerateTableTemplateExcel(c.Request.Context(), organizationID, dataSourceID, tableID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, "failed to generate table template: "+err.Error())
		return
	}

	// Set file name
	fileName := fmt.Sprintf("%s_import_template.xlsx", sanitizeDownloadFileName(table.Name))

	// Set response headers
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))

	// Write file data to response
	c.Data(200, "application/octet-stream", excelData)

	c.AbortWithStatus(200)
}

// ImportTableRecords imports records from an Excel file
// @Summary Import records from Excel
// @Description Import records from an uploaded Excel file to the specified table
// @Tags Data Source Tables
// @Accept json
// @Accept multipart/form-data
// @Produce json
// @Param id path string true "Data source ID"
// @Param table_id path string true "Table ID"
// @Param request body dto.ImportRecordRequest false "Internal uploaded file ID"
// @Param file formData file false "Legacy Excel file upload"
// @Success 200 {object} response.Response{data=dto.ImportRecordResponse}
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id}/tables/{table_id}/records/import [post]
func (h *DataSourceHandler) ImportTableRecords(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	dataSourceID := c.Param("id")
	if dataSourceID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id is required")
		return
	}

	tableID := c.Param("table_id")
	if tableID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "table id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(
		c,
		organizationID,
		dataSourceID,
		accountID,
		workspace_model.WorkspacePermissionDatabaseImportExecute,
	) {
		return
	}

	contentType := c.GetHeader("Content-Type")
	if strings.Contains(contentType, "application/json") {
		var req dto.ImportRecordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			response.FailWithMessage(c, response.ErrInvalidParam, "invalid request: "+err.Error())
			return
		}
		result, err := h.service.ImportTableRecordsFromUploadFile(c.Request.Context(), organizationID, dataSourceID, tableID, accountID, req.UploadFileID, req.SkipUnmatchedColumns)
		if err != nil {
			response.FailWithMessage(c, response.ErrSystemError, "failed to import records: "+err.Error())
			return
		}
		response.Success(c, result)
		return
	}

	// Legacy multipart upload path.
	file, err := c.FormFile("file")
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "failed to get file: "+err.Error())
		return
	}

	// Open the file
	fileContent, err := file.Open()
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, "failed to open file: "+err.Error())
		return
	}
	defer fileContent.Close()

	// Import records through service
	result, err := h.service.ImportTableRecords(c.Request.Context(), organizationID, dataSourceID, tableID, accountID, fileContent, file.Filename, false)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, "failed to import records: "+err.Error())
		return
	}

	response.Success(c, result)
}

// IngestFileToTable ingests file content into a table
// @Summary Ingest file content into table
// @Description Ingest file content into the specified table and return parsed records for review
// @Tags Data Source Tables
// @Accept json
// @Produce json
// @Param request body dto.IngestFileToTableRequest true "File ingestion request"
// @Success 200 {object} response.Response{data=dto.IngestFileToTableResponse}
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/ingest-file-to-table [post]
func (h *DataSourceHandler) IngestFileToTable(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var scopeReq struct {
		TableID string `json:"table_id" binding:"required"`
	}
	if err := c.ShouldBindBodyWith(&scopeReq, binding.JSON); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}
	if !h.ensureDatabaseTablePermission(
		c,
		organizationID,
		scopeReq.TableID,
		accountID,
		workspace_model.WorkspacePermissionDatabaseImportAnalyze,
	) {
		return
	}

	var req dto.IngestFileToTableRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	// Ingest file content into table
	result, err := h.service.IngestFileToTable(c.Request.Context(), organizationID, accountID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// ParseFileForTableIngest parses file content for table ingestion review.
func (h *DataSourceHandler) ParseFileForTableIngest(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var scopeReq struct {
		TableID string `json:"table_id" binding:"required"`
	}
	if err := c.ShouldBindBodyWith(&scopeReq, binding.JSON); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}
	if !h.ensureDatabaseTablePermission(
		c,
		organizationID,
		scopeReq.TableID,
		accountID,
		workspace_model.WorkspacePermissionDatabaseImportAnalyze,
	) {
		return
	}

	var req dto.ParseFileForTableIngestRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	result, err := h.service.ParseFileForTableIngest(c.Request.Context(), organizationID, accountID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// ExtractTextToTableRecords recognizes table records from parsed text content.
func (h *DataSourceHandler) ExtractTextToTableRecords(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var scopeReq struct {
		TableID string `json:"table_id" binding:"required"`
	}
	if err := c.ShouldBindBodyWith(&scopeReq, binding.JSON); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}
	if !h.ensureDatabaseTablePermission(
		c,
		organizationID,
		scopeReq.TableID,
		accountID,
		workspace_model.WorkspacePermissionDatabaseImportAnalyze,
	) {
		return
	}

	var req dto.ExtractTextToTableRecordsRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	result, err := h.service.ExtractTextToTableRecords(c.Request.Context(), organizationID, accountID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// BatchIngestFileToTable handles the batch ingestion of multiple files into a table
func (h *DataSourceHandler) BatchIngestFileToTable(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var scopeReq struct {
		TableID string `json:"table_id" binding:"required"`
	}
	if err := c.ShouldBindBodyWith(&scopeReq, binding.JSON); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}
	if !h.ensureDatabaseTablePermission(
		c,
		organizationID,
		scopeReq.TableID,
		accountID,
		workspace_model.WorkspacePermissionDatabaseImportAnalyze,
	) {
		return
	}

	var req dto.BatchIngestFileToTableRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	// Batch ingest file contents into table
	result, err := h.service.BatchIngestFileToTable(c.Request.Context(), organizationID, accountID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// GetTablePrompt gets the prompt for a table or returns a default one if it doesn't exist
// @Summary Get table prompt
// @Description Get the prompt for a table or returns a default one if it doesn't exist
// @Tags Data Source Tables
// @Produce json
// @Param id path string true "Data source ID"
// @Param table_id path string true "Table ID"
// @Success 200 {object} response.Response{data=model.TablePrompt}
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id}/tables/{table_id}/prompt [get]
func (h *DataSourceHandler) GetTablePrompt(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	dataSourceID := c.Param("id")
	if dataSourceID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id is required")
		return
	}

	tableID := c.Param("table_id")
	if tableID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "table id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(c, organizationID, dataSourceID, accountID, workspace_model.WorkspacePermissionDatabaseTablePromptView) {
		return
	}

	// Determine language - prioritize user's interface language setting
	lang := "en" // default language
	if accountID != "" {
		// Get user's profile to check interface language setting
		profile, err := h.accountService.GetAccountProfile(c.Request.Context(), accountID)
		if err == nil && profile != nil && profile.InterfaceLanguage != "" {
			// Simplify complex language codes to basic language identifiers (zh-CN → zh)
			if strings.HasPrefix(strings.ToLower(profile.InterfaceLanguage), "zh") {
				lang = "zh"
			} else {
				lang = "en"
			}
		} else {
			// Fallback to Accept-Language header if user language not set or error occurred
			acceptLanguage := c.GetHeader("Accept-Language")
			lang = parseAcceptLanguage(acceptLanguage)
		}
	} else {
		// Fallback to Accept-Language header if no account ID
		acceptLanguage := c.GetHeader("Accept-Language")
		lang = parseAcceptLanguage(acceptLanguage)
	}

	// Get prompt for table
	prompt, err := h.service.GetTablePrompt(c.Request.Context(), organizationID, dataSourceID, tableID, accountID, lang)
	if err != nil {
		if service.IsDataSourceTableNotFound(err) {
			response.Fail(c, response.ErrNotFound)
			return
		}
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, prompt)
}

// parseAcceptLanguage parses the Accept-Language header and returns the preferred language
// Returns "zh" for Chinese, "en" for English (default)
func parseAcceptLanguage(acceptLanguage string) string {
	if acceptLanguage == "" {
		return "en"
	}

	// Split by comma to get language-qvalue pairs
	langs := strings.Split(acceptLanguage, ",")

	// Find the language with the highest q-value
	highestQ := 0.0
	preferredLang := "en"

	for _, lang := range langs {
		// Split language and q-value
		parts := strings.Split(strings.TrimSpace(lang), ";")
		language := strings.ToLower(strings.TrimSpace(parts[0]))

		// Default q-value is 1.0
		q := 1.0
		if len(parts) > 1 {
			qPart := strings.TrimSpace(parts[1])
			if strings.HasPrefix(qPart, "q=") {
				if qVal, err := strconv.ParseFloat(qPart[2:], 64); err == nil {
					q = qVal
				}
			}
		}

		// Check if this language has a higher q-value
		if q > highestQ {
			highestQ = q
			// Check if it's Chinese (zh, zh-CN, zh-TW, etc.)
			if strings.HasPrefix(language, "zh") {
				preferredLang = "zh"
			} else if strings.HasPrefix(language, "en") {
				preferredLang = "en"
			}
		}
	}

	return preferredLang
}

// UpsertTablePrompt updates the prompt for a table, or creates it if it doesn't exist
// @Summary Update or create table prompt
// @Description Update the prompt for a table, or create it if it doesn't exist
// @Tags Data Source Tables
// @Accept json
// @Produce json
// @Param id path string true "Data source ID"
// @Param table_id path string true "Table ID"
// @Param prompt body dto.UpdateTablePromptRequest true "Prompt to update or create"
// @Success 200 {object} response.Response{data=model.TablePrompt}
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id}/tables/{table_id}/prompt [put]
func (h *DataSourceHandler) UpsertTablePrompt(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	dataSourceID := c.Param("id")
	if dataSourceID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id is required")
		return
	}

	tableID := c.Param("table_id")
	if tableID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "table id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(c, organizationID, dataSourceID, accountID, workspace_model.WorkspacePermissionDatabaseTablePromptManage) {
		return
	}

	var req dto.UpdateTablePromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	req.UpdatedBy = accountID

	// Update or create prompt for table
	prompt, err := h.service.UpsertTablePrompt(c.Request.Context(), organizationID, dataSourceID, tableID, accountID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, prompt)
}

// DeleteTablePrompt deletes the prompt for a table, resetting it to default
// @Summary Delete table prompt
// @Description Delete the prompt for a table, resetting it to default
// @Tags Data Source Tables
// @Produce json
// @Param id path string true "Data source ID"
// @Param table_id path string true "Table ID"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{id}/tables/{table_id}/prompt [delete]
func (h *DataSourceHandler) DeleteTablePrompt(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	dataSourceID := c.Param("id")
	if dataSourceID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id is required")
		return
	}

	tableID := c.Param("table_id")
	if tableID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "table id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(c, organizationID, dataSourceID, accountID, workspace_model.WorkspacePermissionDatabaseTablePromptManage) {
		return
	}

	err := h.service.DeleteTablePrompt(c.Request.Context(), organizationID, dataSourceID, tableID, accountID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, gin.H{"message": "table prompt deleted successfully"})
}

// ListOperationLogsByDataSourceID lists operation logs for a specific data source
// @Summary List operation logs by data source ID
// @Description List operation logs for a specific data source with pagination
// @Tags Data Source
// @Produce json
// @Param data_source_id path string true "Data Source ID"
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Page size" default(20)
// @Param table_id query string false "Table ID to filter by"
// @Param created_by query string false "Created by user ID to filter by"
// @Param operation_type query string false "Operation type to filter by"
// @Param status query string false "Status to filter by"
// @Param created_at_gte query string false "Created at greater than or equal (RFC3339 format)"
// @Param created_at_lte query string false "Created at less than or equal (RFC3339 format)"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /data-dbs/{data_source_id}/sql-operations [get]
func (h *DataSourceHandler) ListOperationLogsByDataSourceID(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	dataSourceID := c.Param("id")
	if dataSourceID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data_source_id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabasePermission(c, organizationID, dataSourceID, accountID, workspace_model.WorkspacePermissionDatabaseOperationLogsView) {
		return
	}

	// Parse query parameters
	var req dto.ListSQLOperationsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid query parameters: "+err.Error())
		return
	}

	// Set default values
	page := 1
	limit := 20
	if req.Page > 0 {
		page = req.Page
	}
	if req.Limit > 0 {
		limit = req.Limit
	}

	// Calculate offset from page and limit
	offset := (page - 1) * limit

	// Prepare filters
	filters := dto.SQLOperationFilter{
		TableID:       req.TableID,
		CreatedBy:     req.CreatedBy,
		OperationType: req.OperationType,
		Status:        req.Status,
		CreatedAtGTE:  req.StartTime,
		CreatedAtLTE:  req.EndTime,
	}

	// List operation logs with filters
	operations, err := h.service.ListOperationLogsByDataSourceIDWithFilters(c.Request.Context(), organizationID, dataSourceID, accountID, filters, limit, offset)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	// Get total count for pagination info
	total, err := h.service.CountOperationLogsByDataSourceIDWithFilters(c.Request.Context(), organizationID, dataSourceID, accountID, filters)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	uniqueCreatorIDs := make([]string, 0)
	creatorIDMap := make(map[string]bool)
	for _, op := range operations {
		if op.CreatedBy != "" && !creatorIDMap[op.CreatedBy] {
			uniqueCreatorIDs = append(uniqueCreatorIDs, op.CreatedBy)
			creatorIDMap[op.CreatedBy] = true
		}
	}

	creatorMap := make(map[string]*auth_model.Account)
	if len(uniqueCreatorIDs) > 0 {
		creators, err := h.accountService.GetAccountsByIDs(c.Request.Context(), uniqueCreatorIDs)
		if err == nil {
			creatorMap = creators
		}
	}

	// Convert to response DTOs
	var operationResponses []dto.SQLOperationResponse
	for _, op := range operations {
		responseDTO := dto.ConvertSQLOperationModelToResponse(op)

		if op.CreatedBy != "" {
			if account, exists := creatorMap[op.CreatedBy]; exists && account != nil {
				responseDTO.CreatedByName = &account.Name
			}
		}

		operationResponses = append(operationResponses, *responseDTO)
	}

	hasMore := int64(page*limit) < total

	response.Success(c, dto.ListSQLOperationsByDataSourceIDResponse{
		Data:    operationResponses,
		HasMore: hasMore,
		Limit:   limit,
		Total:   total,
		Page:    page,
	})
}

func (h *DataSourceHandler) ListSQLAuditByWorkspace(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	workspaceID := c.Param("workspace_id")
	if workspaceID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "workspace_id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabaseWorkspacePermission(c, organizationID, workspaceID, accountID, workspace_model.WorkspacePermissionDatabaseSQLAuditView) {
		return
	}

	var req dto.ListSQLAuditRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid query parameters: "+err.Error())
		return
	}

	page := 1
	limit := 20
	if req.Page > 0 {
		page = req.Page
	}
	if req.Limit > 0 {
		limit = req.Limit
	}
	offset := (page - 1) * limit

	filters := dto.SQLAuditFilter{
		DataSourceID:  req.DataSourceID,
		TableID:       req.TableID,
		ClientType:    req.ClientType,
		WorkflowRunID: req.WorkflowRunID,
		NodeID:        req.NodeID,
		CreatedBy:     req.CreatedBy,
		OperationType: req.OperationType,
		Status:        req.Status,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
	}

	records, err := h.service.ListSQLAuditByWorkspace(c.Request.Context(), organizationID, workspaceID, accountID, filters, limit, offset)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	total, err := h.service.CountSQLAuditByWorkspace(c.Request.Context(), organizationID, workspaceID, accountID, filters)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	items := make([]dto.SQLAuditListItem, 0, len(records))
	for _, record := range records {
		items = append(items, dto.ConvertSQLOperationModelToAuditListItem(record))
	}

	response.Success(c, dto.ListSQLAuditResponse{
		Data:    items,
		HasMore: int64(page*limit) < total,
		Limit:   limit,
		Total:   total,
		Page:    page,
	})
}

func (h *DataSourceHandler) GetSQLAuditDetail(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	workspaceID := c.Param("workspace_id")
	if workspaceID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "workspace_id is required")
		return
	}

	operationID := c.Param("operation_id")
	if operationID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "operation_id is required")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.ensureDatabaseWorkspacePermission(c, organizationID, workspaceID, accountID, workspace_model.WorkspacePermissionDatabaseSQLAuditView) {
		return
	}

	record, err := h.service.GetSQLAuditDetail(c.Request.Context(), organizationID, workspaceID, operationID, accountID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if record == nil {
		response.Fail(c, response.ErrNotFound)
		return
	}

	response.Success(c, dto.ConvertSQLOperationModelToAuditDetail(record))
}

// RegisterRoutes registers all data source routes
func (h *DataSourceHandler) RegisterRoutes(router *gin.RouterGroup) {
	authWithTenant := router.Group("", middleware.JWTWithOrganizationAndService(h.accountService))

	// Data source routes with authentication
	authWithTenant.POST("/data-dbs", h.CreateDataSource)
	authWithTenant.GET("/data-dbs", h.ListDataSources)
	authWithTenant.GET("/data-dbs/:id", h.GetDataSourceByID)
	authWithTenant.PUT("/data-dbs/:id", h.UpdateDataSource)
	authWithTenant.DELETE("/data-dbs/:id", h.DeleteDataSourceByID)
	authWithTenant.GET("/data-dbs/:id/guard/policy", h.GetGuardPolicy)
	authWithTenant.PUT("/data-dbs/:id/guard/policy", h.UpdateGuardPolicy)
	authWithTenant.POST("/data-dbs/:id/guard/preview", h.PreviewGuard)

	// Table operations within a data source
	authWithTenant.POST("/data-dbs/:id/tables", h.CreateTable)
	authWithTenant.GET("/data-dbs/:id/tables", h.ListTables)
	authWithTenant.GET("/data-dbs/:id/tables/:table_id", h.GetTable)
	authWithTenant.PUT("/data-dbs/:id/tables/:table_id", h.UpdateTable)
	authWithTenant.DELETE("/data-dbs/:id/tables/:table_id", h.DeleteTable)
	authWithTenant.GET("/data-dbs/:id/tables/:table_id/prompt", h.GetTablePrompt)
	authWithTenant.PUT("/data-dbs/:id/tables/:table_id/prompt", h.UpsertTablePrompt)
	authWithTenant.DELETE("/data-dbs/:id/tables/:table_id/prompt", h.DeleteTablePrompt)
	authWithTenant.PUT("/data-dbs/:id/tables/:table_id/columns", h.UpdateTableColumns)
	authWithTenant.GET("/data-dbs/:id/tables/:table_id/columns", h.GetTableColumns)
	authWithTenant.GET("/data-dbs/:id/sql-operations", h.ListOperationLogsByDataSourceID)
	authWithTenant.GET("/workspaces/:workspace_id/sql-audit", h.ListSQLAuditByWorkspace)
	authWithTenant.GET("/workspaces/:workspace_id/sql-audit/:operation_id", h.GetSQLAuditDetail)
	authWithTenant.POST("/data-dbs/analyze-file-for-table", h.AnalyzeFileForTable)
	authWithTenant.POST("/data-dbs/:id/excel-import/analyze", h.AnalyzeExcelImport)
	authWithTenant.GET("/data-dbs/:id/excel-import/jobs/:job_id", h.GetExcelImportJob)
	authWithTenant.POST("/data-dbs/:id/excel-import/jobs/:job_id/recognize", h.RecognizeExcelImportFields)
	authWithTenant.POST("/data-dbs/:id/excel-import/jobs/:job_id/import", h.ConfirmExcelImport)
	authWithTenant.GET("/data-dbs/:id/excel-import/jobs/:job_id/errors", h.ListExcelImportErrors)

	// Table data operations
	authWithTenant.POST("/data-dbs/:id/tables/:table_id/records", h.AddTableRecords)
	authWithTenant.PUT("/data-dbs/:id/tables/:table_id/records", h.UpdateTableRecords)
	authWithTenant.DELETE("/data-dbs/:id/tables/:table_id/records", h.DeleteTableRecords)
	authWithTenant.GET("/data-dbs/:id/tables/:table_id/records", h.QueryTableRecords)
	authWithTenant.GET("/data-dbs/:id/tables/:table_id/template", h.DownloadTableTemplate)
	authWithTenant.POST("/data-dbs/:id/tables/:table_id/records/import", h.ImportTableRecords)

	// File ingestion operations
	authWithTenant.POST("/data-dbs/parse-file-for-table-ingest", h.ParseFileForTableIngest)
	authWithTenant.POST("/data-dbs/extract-text-to-table-records", h.ExtractTextToTableRecords)
	authWithTenant.POST("/data-dbs/ingest-file-to-table", h.IngestFileToTable)
	authWithTenant.POST("/data-dbs/batch-ingest-file-to-table", h.BatchIngestFileToTable)
}
