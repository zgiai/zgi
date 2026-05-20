package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/modules/workspace/model"
	workspace_service "github.com/zgiai/ginext/internal/modules/workspace/service"
	"github.com/zgiai/ginext/pkg/response"
)

// GetOrganizationWorkspaceAvailableMembers lists organization members that can be added to a workspace.
func (h *OrganizationHandler) GetOrganizationWorkspaceAvailableMembers(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	workspaceID := c.Param("workspace_id")
	if organizationID == "" || workspaceID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if h.organizationService == nil || h.departmentService == nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	organization, err := h.organizationService.GetOrganizationByWorkspaceID(c.Request.Context(), workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if organization == nil || organization.ID != organizationID {
		response.Fail(c, response.ErrWorkspaceNotInOrganization)
		return
	}

	hasPermission, err := h.organizationService.CheckWorkspacePermission(
		c.Request.Context(),
		organizationID,
		workspaceID,
		accountID,
		model.WorkspacePermissionWorkspaceManage,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if !hasPermission {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	keyword := c.Query("keyword")
	var departmentID *string
	if deptID := c.Query("department_id"); deptID != "" && deptID != "null" {
		departmentID = &deptID
	}

	page := parsePositiveIntQuery(c, "page", 1, 0)
	limit := parsePositiveIntQuery(c, "limit", 20, 100)
	includeSubDept := c.Query("include_sub_depts") == "true"

	params := &workspace_service.MemberListParams{
		OrganizationID:     organizationID,
		Keyword:            keyword,
		DepartmentID:       departmentID,
		IncludeSubDept:     includeSubDept,
		Page:               page,
		Limit:              limit,
		OnlyActive:         true,
		ExcludeWorkspaceID: &workspaceID,
	}

	result, err := h.departmentService.ListMembers(c.Request.Context(), params)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}

func parsePositiveIntQuery(c *gin.Context, key string, fallback, max int) int {
	value := fallback
	if raw := c.Query(key); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			value = parsed
		}
	}
	if max > 0 && value > max {
		return fallback
	}
	return value
}
