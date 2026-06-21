package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

func (h *DataSourceHandler) AnalyzeExcelImport(c *gin.Context) {
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
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	if !h.ensureDatabasePermission(c, organizationID, dataSourceID, accountID, model.WorkspacePermissionDatabaseManage) {
		return
	}
	var req dto.AnalyzeExcelImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	result, err := h.service.AnalyzeExcelImport(c.Request.Context(), organizationID, dataSourceID, accountID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *DataSourceHandler) ConfirmExcelImport(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}
	dataSourceID := c.Param("id")
	jobID := c.Param("job_id")
	if dataSourceID == "" || jobID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id and job id are required")
		return
	}
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	if !h.ensureDatabasePermission(c, organizationID, dataSourceID, accountID, model.WorkspacePermissionDatabaseManage) {
		return
	}
	var req dto.ConfirmExcelImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	result, err := h.service.ConfirmExcelImport(c.Request.Context(), organizationID, dataSourceID, accountID, jobID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *DataSourceHandler) RecognizeExcelImportFields(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}
	dataSourceID := c.Param("id")
	jobID := c.Param("job_id")
	if dataSourceID == "" || jobID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id and job id are required")
		return
	}
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	if !h.ensureDatabasePermission(c, organizationID, dataSourceID, accountID, model.WorkspacePermissionDatabaseManage) {
		return
	}
	var req dto.RecognizeExcelImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	result, err := h.service.RecognizeExcelImportFields(c.Request.Context(), organizationID, dataSourceID, accountID, jobID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *DataSourceHandler) GetExcelImportJob(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}
	dataSourceID := c.Param("id")
	jobID := c.Param("job_id")
	if dataSourceID == "" || jobID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id and job id are required")
		return
	}
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	if !h.ensureDatabasePermission(c, organizationID, dataSourceID, accountID, model.WorkspacePermissionDatabaseView) {
		return
	}
	result, err := h.service.GetExcelImportJob(c.Request.Context(), organizationID, dataSourceID, jobID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if result == nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "import job not found")
		return
	}
	response.Success(c, result)
}

func (h *DataSourceHandler) ListExcelImportErrors(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}
	dataSourceID := c.Param("id")
	jobID := c.Param("job_id")
	if dataSourceID == "" || jobID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source id and job id are required")
		return
	}
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	if !h.ensureDatabasePermission(c, organizationID, dataSourceID, accountID, model.WorkspacePermissionDatabaseManage) {
		return
	}
	limit := clampQueryInt(c.DefaultQuery("limit", "20"), 20, 1, 100)
	offset := clampQueryInt(c.DefaultQuery("offset", "0"), 0, 0, 1000000)
	result, err := h.service.ListExcelImportErrors(c.Request.Context(), organizationID, dataSourceID, jobID, limit, offset)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *DataSourceHandler) ensureDatabasePermission(c *gin.Context, organizationID, dataSourceID, accountID string, permissions ...model.WorkspacePermissionCode) bool {
	if h.organizationService == nil {
		return true
	}
	dataSource, err := h.service.GetDataSourceByID(c.Request.Context(), organizationID, dataSourceID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return false
	}
	if dataSource == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data source not found")
		return false
	}
	if dataSource.WorkspaceID == nil || strings.TrimSpace(*dataSource.WorkspaceID) == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "data source has no workspace scope")
		return false
	}
	dataSourceWorkspaceID := strings.TrimSpace(*dataSource.WorkspaceID)
	hasPermission, err := h.organizationService.CheckWorkspaceOrganizationAnyPermission(
		c.Request.Context(),
		organizationID,
		dataSourceWorkspaceID,
		accountID,
		permissions...,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return false
	}
	if !hasPermission {
		response.Fail(c, response.ErrPermissionDenied)
		return false
	}
	return true
}

func (h *DataSourceHandler) ensureDatabaseWorkspacePermission(c *gin.Context, organizationID, workspaceID, accountID string, permissions ...model.WorkspacePermissionCode) bool {
	if h.organizationService == nil {
		return true
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "workspace_id is required")
		return false
	}
	hasPermission, err := h.organizationService.CheckWorkspaceOrganizationAnyPermission(
		c.Request.Context(),
		organizationID,
		workspaceID,
		accountID,
		permissions...,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return false
	}
	if !hasPermission {
		response.Fail(c, response.ErrPermissionDenied)
		return false
	}
	return true
}

func (h *DataSourceHandler) ensureDatabaseTablePermission(c *gin.Context, organizationID, tableID, accountID string, permissions ...model.WorkspacePermissionCode) bool {
	tableID = strings.TrimSpace(tableID)
	if tableID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "table id is required")
		return false
	}
	dataSourceID, err := h.service.ResolveTableDataSourceID(c.Request.Context(), organizationID, tableID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return false
	}
	return h.ensureDatabasePermission(c, organizationID, dataSourceID, accountID, permissions...)
}

func clampQueryInt(raw string, fallback, minValue, maxValue int) int {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
