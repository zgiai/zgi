package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/service"
	helper "github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
)

// DepartmentHandler handles department-related HTTP requests
type DepartmentHandler struct {
	departmentService service.DepartmentService
	accountService    interfaces.AccountService
	enterpriseService interfaces.OrganizationService
	consoleWebURL     string
}

// NewDepartmentHandler creates a new department handler
func NewDepartmentHandler(
	departmentService service.DepartmentService,
	accountService interfaces.AccountService,
	enterpriseService interfaces.OrganizationService,
	consoleWebURL string,
) *DepartmentHandler {
	return &DepartmentHandler{
		departmentService: departmentService,
		accountService:    accountService,
		enterpriseService: enterpriseService,
		consoleWebURL:     consoleWebURL,
	}
}

// CreateDepartmentRequest request body for creating a department
type CreateDepartmentRequest struct {
	Name      string  `json:"name" binding:"required"`
	ParentID  *string `json:"parent_id"`
	SortOrder int     `json:"sort_order"`
}

// UpdateDepartmentRequest request body for updating a department
type UpdateDepartmentRequest struct {
	Name      string                  `json:"name"`
	ParentID  *string                 `json:"parent_id"`
	SortOrder *int                    `json:"sort_order"`
	Status    *model.DepartmentStatus `json:"status"`
}

// AddMemberRequest request body for adding a member to a department
type AddMemberRequest struct {
	AccountID string `json:"account_id" binding:"required"`
}

// ChangeMemberDepartmentRequest request body for changing a member's department
type ChangeMemberDepartmentRequest struct {
	DepartmentID string `json:"department_id"`
}

// DepartmentResponse response for a department
type DepartmentResponse struct {
	ID             string                 `json:"id"`
	OrganizationID string                 `json:"organization_id"`
	ParentID       *string                `json:"parent_id"`
	Name           string                 `json:"name"`
	SortOrder      int                    `json:"sort_order"`
	Status         model.DepartmentStatus `json:"status"`
	MemberCount    int64                  `json:"member_count,omitempty"`
	CreatedAt      string                 `json:"created_at"`
	UpdatedAt      string                 `json:"updated_at"`
}

// DepartmentTreeResponse response for department tree
type DepartmentTreeResponse struct {
	ID             string                    `json:"id"`
	OrganizationID string                    `json:"organization_id"`
	ParentID       *string                   `json:"parent_id"`
	Name           string                    `json:"name"`
	SortOrder      int                       `json:"sort_order"`
	Status         model.DepartmentStatus    `json:"status"`
	MemberCount    int64                     `json:"member_count"`
	Children       []*DepartmentTreeResponse `json:"children"`
}

// MemberResponse response for a department member
type MemberResponse struct {
	ID           string `json:"id"`
	DepartmentID string `json:"department_id"`
	AccountID    string `json:"account_id"`
	CreatedAt    string `json:"created_at"`
}

// getOrganizationID gets organization_id from URL param or context (for current routes)
func (h *DepartmentHandler) getOrganizationID(c *gin.Context) string {
	organizationID := c.Param("organization_id")
	if organizationID == "" || organizationID == "current" {
		organizationID = helper.GetOrganizationID(c)
	}
	return organizationID
}

func (h *DepartmentHandler) requireOrganizationAdminOrOwner(c *gin.Context, organizationID string) (string, bool) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return "", false
	}
	if h.enterpriseService == nil {
		response.Fail(c, response.ErrSystemError)
		return "", false
	}

	allowed, err := h.enterpriseService.IsOrganizationAdminOrOwner(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return "", false
	}
	if !allowed {
		response.Fail(c, response.ErrPermissionDenied)
		return "", false
	}

	return accountID, true
}

func (h *DepartmentHandler) requireOrganizationReadAccess(c *gin.Context, organizationID string) (string, bool, bool) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return "", false, false
	}
	if h.enterpriseService == nil {
		response.Fail(c, response.ErrSystemError)
		return "", false, false
	}

	isAdmin, err := h.enterpriseService.IsOrganizationAdminOrOwner(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return "", false, false
	}
	if isAdmin {
		return accountID, true, true
	}

	isMember, err := h.enterpriseService.IsOrganizationMember(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return "", false, false
	}
	hasManagedWorkspace := false
	if !isMember {
		hasManagedWorkspace, err = h.enterpriseService.CheckAnyManagedWorkspacePermission(c.Request.Context(), organizationID, accountID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return "", false, false
		}
	}
	if !isMember && !hasManagedWorkspace {
		response.Fail(c, response.ErrPermissionDenied)
		return "", false, false
	}

	return accountID, false, true
}

func (h *DepartmentHandler) requireDepartmentInOrganization(c *gin.Context, organizationID, deptID string) (*model.Department, bool) {
	if deptID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return nil, false
	}

	dept, err := h.departmentService.GetDepartment(c.Request.Context(), deptID)
	if err != nil {
		if errors.Is(err, service.ErrDepartmentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": "DepartmentNotFound", "message": err.Error()})
			return nil, false
		}
		response.Fail(c, response.ErrSystemError)
		return nil, false
	}
	if dept.OrganizationID != organizationID {
		response.Fail(c, response.ErrPermissionDenied)
		return nil, false
	}

	return dept, true
}

func (h *DepartmentHandler) visibleDepartmentIDs(c *gin.Context, organizationID, accountID string) (map[string]struct{}, error) {
	dept, err := h.departmentService.GetMemberDepartment(c.Request.Context(), organizationID, accountID)
	if err != nil {
		if errors.Is(err, service.ErrMemberNotInDept) {
			return map[string]struct{}{}, nil
		}
		return nil, err
	}

	tree, err := h.departmentService.GetDepartmentTree(c.Request.Context(), organizationID)
	if err != nil {
		return nil, err
	}

	visible := make(map[string]struct{})
	var collect func(nodes []*service.DepartmentTreeNode) bool
	collect = func(nodes []*service.DepartmentTreeNode) bool {
		for _, node := range nodes {
			if node.ID == dept.ID {
				var collectSubtree func(*service.DepartmentTreeNode)
				collectSubtree = func(current *service.DepartmentTreeNode) {
					visible[current.ID] = struct{}{}
					for _, child := range current.Children {
						collectSubtree(child)
					}
				}
				collectSubtree(node)
				return true
			}
			if collect(node.Children) {
				return true
			}
		}
		return false
	}
	collect(tree)

	return visible, nil
}

func filterDepartmentTreeByIDs(nodes []*service.DepartmentTreeNode, visible map[string]struct{}) []*service.DepartmentTreeNode {
	filtered := make([]*service.DepartmentTreeNode, 0)
	for _, node := range nodes {
		children := filterDepartmentTreeByIDs(node.Children, visible)
		if _, ok := visible[node.ID]; ok {
			nodeCopy := *node
			nodeCopy.Children = children
			filtered = append(filtered, &nodeCopy)
			continue
		}
		filtered = append(filtered, children...)
	}
	return filtered
}

func hasVisibleDepartment(visible map[string]struct{}, departmentID string) bool {
	_, ok := visible[departmentID]
	return ok
}

// CreateDepartment handles POST /organizations/:organization_id/departments
func (h *DepartmentHandler) CreateDepartment(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID, ok := h.requireOrganizationAdminOrOwner(c, organizationID)
	if !ok {
		return
	}

	var req CreateDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	dept, err := h.departmentService.CreateDepartment(c.Request.Context(), organizationID, req.Name, req.ParentID, req.SortOrder, accountID)
	if err != nil {
		if errors.Is(err, service.ErrDepartmentNameExists) {
			c.JSON(http.StatusBadRequest, gin.H{"code": "DepartmentNameExists", "message": err.Error()})
			return
		}
		if errors.Is(err, service.ErrDepartmentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": "ParentDepartmentNotFound", "message": "Parent department not found"})
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, toDepartmentResponse(dept))
}

// GetDepartment handles GET /organizations/:organization_id/departments/:dept_id
func (h *DepartmentHandler) GetDepartment(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	deptID := c.Param("dept_id")

	accountID, isAdmin, ok := h.requireOrganizationReadAccess(c, organizationID)
	if !ok {
		return
	}

	dept, ok := h.requireDepartmentInOrganization(c, organizationID, deptID)
	if !ok {
		return
	}

	if !isAdmin {
		visible, err := h.visibleDepartmentIDs(c, organizationID, accountID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !hasVisibleDepartment(visible, dept.ID) {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	response.Success(c, toDepartmentResponse(dept))
}

// UpdateDepartment handles PUT /organizations/:organization_id/departments/:dept_id
func (h *DepartmentHandler) UpdateDepartment(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	deptID := c.Param("dept_id")

	if _, ok := h.requireOrganizationAdminOrOwner(c, organizationID); !ok {
		return
	}
	if _, ok := h.requireDepartmentInOrganization(c, organizationID, deptID); !ok {
		return
	}

	var req UpdateDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	dept, err := h.departmentService.UpdateDepartment(c.Request.Context(), deptID, req.Name, req.ParentID, req.SortOrder, req.Status)
	if err != nil {
		if errors.Is(err, service.ErrDepartmentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": "DepartmentNotFound", "message": err.Error()})
			return
		}
		if errors.Is(err, service.ErrDepartmentNameExists) {
			c.JSON(http.StatusBadRequest, gin.H{"code": "DepartmentNameExists", "message": err.Error()})
			return
		}
		if errors.Is(err, service.ErrCircularReference) {
			c.JSON(http.StatusBadRequest, gin.H{"code": "CircularReference", "message": err.Error()})
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, toDepartmentResponse(dept))
}

// DeleteDepartment handles DELETE /organizations/:organization_id/departments/:dept_id
func (h *DepartmentHandler) DeleteDepartment(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	deptID := c.Param("dept_id")

	if _, ok := h.requireOrganizationAdminOrOwner(c, organizationID); !ok {
		return
	}
	if _, ok := h.requireDepartmentInOrganization(c, organizationID, deptID); !ok {
		return
	}

	err := h.departmentService.DeleteDepartment(c.Request.Context(), deptID)
	if err != nil {
		if errors.Is(err, service.ErrDepartmentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": "DepartmentNotFound", "message": err.Error()})
			return
		}
		if errors.Is(err, service.ErrCannotDeleteNonEmptyDept) {
			c.JSON(http.StatusBadRequest, gin.H{"code": "DepartmentNotEmpty", "message": err.Error()})
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Department deleted successfully"})
}

// GetDepartmentTree handles GET /organizations/:organization_id/departments
func (h *DepartmentHandler) GetDepartmentTree(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID, isAdmin, ok := h.requireOrganizationReadAccess(c, organizationID)
	if !ok {
		return
	}

	tree, err := h.departmentService.GetDepartmentTree(c.Request.Context(), organizationID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if !isAdmin {
		visible, err := h.visibleDepartmentIDs(c, organizationID, accountID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		tree = filterDepartmentTreeByIDs(tree, visible)
	}

	response.Success(c, gin.H{"departments": toTreeResponse(tree)})
}

// AddMemberToDepartment handles POST /organizations/:organization_id/departments/:dept_id/members
func (h *DepartmentHandler) AddMemberToDepartment(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	deptID := c.Param("dept_id")

	if _, ok := h.requireOrganizationAdminOrOwner(c, organizationID); !ok {
		return
	}
	if _, ok := h.requireDepartmentInOrganization(c, organizationID, deptID); !ok {
		return
	}

	var req AddMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	member, err := h.departmentService.AddMemberToDepartment(c.Request.Context(), organizationID, deptID, req.AccountID)
	if err != nil {
		if errors.Is(err, service.ErrDepartmentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": "DepartmentNotFound", "message": err.Error()})
			return
		}
		if errors.Is(err, service.ErrMemberAlreadyInDept) {
			c.JSON(http.StatusBadRequest, gin.H{"code": "MemberAlreadyInDepartment", "message": err.Error()})
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, toMemberResponse(member))
}

// RemoveMemberFromDepartment handles DELETE /organizations/:organization_id/departments/:dept_id/members/:account_id
func (h *DepartmentHandler) RemoveMemberFromDepartment(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	deptID := c.Param("dept_id")
	accountID := c.Param("account_id")

	if _, ok := h.requireOrganizationAdminOrOwner(c, organizationID); !ok {
		return
	}
	if _, ok := h.requireDepartmentInOrganization(c, organizationID, deptID); !ok {
		return
	}

	err := h.departmentService.RemoveMemberFromDepartment(c.Request.Context(), deptID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Member removed from department successfully"})
}

// GetDepartmentMembers handles GET /organizations/:organization_id/departments/:dept_id/members
func (h *DepartmentHandler) GetDepartmentMembers(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	deptID := c.Param("dept_id")

	accountID, isAdmin, ok := h.requireOrganizationReadAccess(c, organizationID)
	if !ok {
		return
	}
	if _, ok := h.requireDepartmentInOrganization(c, organizationID, deptID); !ok {
		return
	}
	if !isAdmin {
		visible, err := h.visibleDepartmentIDs(c, organizationID, accountID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !hasVisibleDepartment(visible, deptID) {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	// Parse query parameters (reuse ListMembers semantics)
	keyword := c.Query("keyword")
	includeSubDept := c.Query("include_sub_depts") == "true"

	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := 20
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	deptIDCopy := deptID
	params := &service.MemberListParams{
		OrganizationID: organizationID,
		Keyword:        keyword,
		DepartmentID:   &deptIDCopy,
		IncludeSubDept: includeSubDept,
		Page:           page,
		Limit:          limit,
	}

	result, err := h.departmentService.ListMembers(c.Request.Context(), params)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{
		"members":  result.Data,
		"total":    result.Total,
		"page":     result.Page,
		"limit":    result.Limit,
		"has_more": result.HasMore,
	})
}

// GetMemberDepartment handles GET /organizations/:organization_id/departments/member/:account_id
func (h *DepartmentHandler) GetMemberDepartment(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	accountID := c.Param("account_id")

	currentAccountID, isAdmin, ok := h.requireOrganizationReadAccess(c, organizationID)
	if !ok {
		return
	}
	if !isAdmin && accountID != currentAccountID {
		detail, err := h.departmentService.GetMemberDetailByAccountID(c.Request.Context(), organizationID, accountID)
		if err != nil || detail == nil {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
		visible, err := h.visibleDepartmentIDs(c, organizationID, currentAccountID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !hasVisibleDepartment(visible, detail.DepartmentID) {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	dept, err := h.departmentService.GetMemberDepartment(c.Request.Context(), organizationID, accountID)
	if err != nil {
		if errors.Is(err, service.ErrMemberNotInDept) {
			response.Success(c, nil)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, toDepartmentResponse(dept))
}

// ChangeMemberDepartment handles PUT /organizations/:organization_id/departments/member/:account_id
func (h *DepartmentHandler) ChangeMemberDepartment(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	accountID := c.Param("account_id")

	if _, ok := h.requireOrganizationAdminOrOwner(c, organizationID); !ok {
		return
	}

	var req ChangeMemberDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	member, err := h.departmentService.ChangeMemberDepartment(c.Request.Context(), organizationID, accountID, req.DepartmentID)
	if err != nil {
		if errors.Is(err, service.ErrDepartmentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": "DepartmentNotFound", "message": err.Error()})
			return
		}
		if errors.Is(err, service.ErrMemberNotInDept) {
			c.JSON(http.StatusNotFound, gin.H{"code": "MemberNotInDepartment", "message": err.Error()})
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	if member == nil {
		response.Success(c, nil)
		return
	}

	response.Success(c, toMemberResponse(member))
}

// ListMembers handles GET /organizations/:organization_id/departments/members
// Supports listing all members or searching with query parameters
func (h *DepartmentHandler) ListMembers(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID, isAdmin, ok := h.requireOrganizationReadAccess(c, organizationID)
	if !ok {
		return
	}

	// Parse query parameters
	keyword := c.Query("keyword")

	var departmentID *string
	if deptID := c.Query("department_id"); deptID != "" {
		departmentID = &deptID
	}

	includeSubDept := c.Query("include_sub_depts") == "true"

	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := 20
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	if !isAdmin {
		visible, err := h.visibleDepartmentIDs(c, organizationID, accountID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if departmentID != nil {
			if !hasVisibleDepartment(visible, *departmentID) {
				response.Fail(c, response.ErrPermissionDenied)
				return
			}
		} else {
			dept, err := h.departmentService.GetMemberDepartment(c.Request.Context(), organizationID, accountID)
			if err != nil {
				if !errors.Is(err, service.ErrMemberNotInDept) {
					response.Fail(c, response.ErrSystemError)
					return
				}
				response.Success(c, &service.MemberListResponse{
					Data:    []*service.DepartmentMemberDetail{},
					Total:   0,
					Page:    page,
					Limit:   limit,
					HasMore: false,
				})
				return
			}
			departmentID = &dept.ID
			includeSubDept = true
		}
	}

	params := &service.MemberListParams{
		OrganizationID: organizationID,
		Keyword:        keyword,
		DepartmentID:   departmentID,
		IncludeSubDept: includeSubDept,
		Page:           page,
		Limit:          limit,
	}

	result, err := h.departmentService.ListMembers(c.Request.Context(), params)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}

func (h *DepartmentHandler) GetMemberDetailByAccountID(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	memberID := c.Param("member_id")
	if memberID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID, isAdmin, ok := h.requireOrganizationReadAccess(c, organizationID)
	if !ok {
		return
	}

	result, err := h.departmentService.GetMemberDetailByAccountID(c.Request.Context(), organizationID, memberID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	if result == nil {
		response.Fail(c, response.ErrMemberNotFound)
		return
	}

	if !isAdmin && memberID != accountID {
		visible, err := h.visibleDepartmentIDs(c, organizationID, accountID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !hasVisibleDepartment(visible, result.DepartmentID) {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	response.Success(c, result)
}

// ListMembersWithoutDepartment handles GET /organizations/:organization_id/departments/members-without-department
// Returns members who have joined the group but are not in any department
func (h *DepartmentHandler) ListMembersWithoutDepartment(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if _, ok := h.requireOrganizationAdminOrOwner(c, organizationID); !ok {
		return
	}

	keyword := c.Query("keyword")

	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := 20
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	params := &service.MemberListParams{
		OrganizationID:        organizationID,
		Keyword:               keyword,
		DepartmentID:          nil,
		IncludeSubDept:        false,
		Page:                  page,
		Limit:                 limit,
		OnlyWithoutDepartment: true,
	}

	result, err := h.departmentService.ListMembers(c.Request.Context(), params)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}

// Helper functions

func toDepartmentResponse(dept *model.Department) *DepartmentResponse {
	return &DepartmentResponse{
		ID:             dept.ID,
		OrganizationID: dept.OrganizationID,
		ParentID:       dept.ParentID,
		Name:           dept.Name,
		SortOrder:      dept.SortOrder,
		Status:         dept.Status,
		CreatedAt:      dept.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      dept.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func toTreeResponse(nodes []*service.DepartmentTreeNode) []*DepartmentTreeResponse {
	if nodes == nil {
		return []*DepartmentTreeResponse{}
	}

	result := make([]*DepartmentTreeResponse, len(nodes))
	for i, node := range nodes {
		result[i] = &DepartmentTreeResponse{
			ID:             node.ID,
			OrganizationID: node.OrganizationID,
			ParentID:       node.ParentID,
			Name:           node.Name,
			SortOrder:      node.SortOrder,
			Status:         node.Status,
			MemberCount:    node.MemberCount,
			Children:       toTreeResponse(node.Children),
		}
	}
	return result
}

func toMemberResponse(member *model.DepartmentMember) *MemberResponse {
	return &MemberResponse{
		ID:           member.ID,
		DepartmentID: member.DepartmentID,
		AccountID:    member.AccountID,
		CreatedAt:    member.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func toMembersResponse(members []*model.DepartmentMember) []*MemberResponse {
	result := make([]*MemberResponse, len(members))
	for i, member := range members {
		result[i] = toMemberResponse(member)
	}
	return result
}

// RegisterRoutes registers department routes
// Use "current" as organization_id to read the canonical organization scope from context.
func (h *DepartmentHandler) RegisterRoutes(router *gin.RouterGroup) {

	organization := router.Group("/organizations", middleware.JWTWithOrganizationAndService(h.accountService))
	{
		// Department management
		organization.GET("/:organization_id/departments", h.GetDepartmentTree)
		organization.POST("/:organization_id/departments", h.CreateDepartment)
		organization.GET("/:organization_id/departments/:dept_id", h.GetDepartment)
		organization.PUT("/:organization_id/departments/:dept_id", h.UpdateDepartment)
		organization.DELETE("/:organization_id/departments/:dept_id", h.DeleteDepartment)

		// Department member management
		organization.GET("/:organization_id/departments/:dept_id/members", h.GetDepartmentMembers)
		organization.POST("/:organization_id/departments/:dept_id/members", h.AddMemberToDepartment)
		organization.DELETE("/:organization_id/departments/:dept_id/members/:account_id", h.RemoveMemberFromDepartment)

		// Member's department (single department mode, under /departments to avoid conflict)
		organization.GET("/:organization_id/departments/member/:account_id", h.GetMemberDepartment)
		organization.PUT("/:organization_id/departments/member/:account_id", h.ChangeMemberDepartment)

		// Member list with search (under /departments to avoid conflict with EnterpriseHandler)
		organization.GET("/:organization_id/departments/members-without-department", h.ListMembersWithoutDepartment)
		organization.GET("/:organization_id/departments/members", h.ListMembers)
		organization.GET("/:organization_id/departments/members/:member_id", h.GetMemberDetailByAccountID)
	}

}
