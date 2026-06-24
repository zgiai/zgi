package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	middleware "github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

// DatasetFolderHandler handles dataset folder-related HTTP requests
type DatasetFolderHandler struct {
	datasetService             service.DatasetService
	folderService              service.DatasetFolderService
	workspaceManagementService interfaces.WorkspaceManagementService
	accountService             interfaces.AccountService
	organizationService        interfaces.OrganizationService
	permissionService          interfaces.ResourcePermissionService
	authorizationService       interfaces.AuthorizationService
}

// NewDatasetFolderHandler creates a new DatasetFolderHandler instance
func NewDatasetFolderHandler(
	datasetService service.DatasetService,
	folderService service.DatasetFolderService,
	tenantService interfaces.WorkspaceManagementService,
	accountService interfaces.AccountService,
	enterpriseService interfaces.OrganizationService,
	permissionService interfaces.ResourcePermissionService,
	authorizationService interfaces.AuthorizationService,
) *DatasetFolderHandler {
	return &DatasetFolderHandler{
		datasetService:             datasetService,
		folderService:              folderService,
		workspaceManagementService: tenantService,
		accountService:             accountService,
		organizationService:        enterpriseService,
		permissionService:          permissionService,
		authorizationService:       authorizationService,
	}
}

func failDatasetFolderRead(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(err.Error(), "not found"):
		response.Fail(c, response.ErrDatasetNotFound)
	case errors.Is(err, service.ErrDatasetAccessDenied) || strings.Contains(err.Error(), "no permission"):
		response.Fail(c, response.ErrDatasetPermissionDenied)
	default:
		response.Fail(c, response.ErrSystemError)
	}
}

// GetFolders handles GET /dataset-folders
func (h *DatasetFolderHandler) GetFolders(c *gin.Context) {
	// Get account ID from context
	accountID := c.GetString("account_id")

	// Get query parameters
	var req dto.DatasetFolderListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Set default values
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}
	if req.Sort == "" || (req.Sort != "asc" && req.Sort != "desc") {
		req.Sort = "desc"
	}

	if req.FolderID != "" {
		if _, err := uuid.Parse(req.FolderID); err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
		parentFolder, ok := authorizeDatasetFolderViewAccess(c, h.folderService, h.authorizationService, req.FolderID)
		if !ok {
			return
		}
		if req.WorkspaceID != "" && req.WorkspaceID != parentFolder.WorkspaceID {
			response.Success(c, gin.H{
				"data":     []dto.DatasetFolderDetailResponse{},
				"has_more": false,
				"limit":    req.Limit,
				"total":    0,
				"page":     req.Page,
			})
			return
		}
		req.WorkspaceID = parentFolder.WorkspaceID
	}

	// Step 1: Get current organization (aligned with Agent logic)
	organizationID := c.GetString("tenant_id")
	userOrganizationRole, err := h.organizationService.GetUserOrganizationRole(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	organizationRole := string(userOrganizationRole)

	// Step 2: Get all departments in the organization
	orgDeptIDs, err := h.workspaceManagementService.GetWorkspaceIDsByOrganizationID(c.Request.Context(), organizationID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Step 3: Build tenant IDs list based on user role (aligned with Agent logic)
	var workspaceIDs []string
	isOrgAdmin := false
	if organizationRole == "owner" || organizationRole == "admin" {
		// Organization admin can see all departments
		workspaceIDs = orgDeptIDs
		isOrgAdmin = true
	} else {
		// For normal users, calculate intersection of organization departments and user departments
		userDepts, err := h.workspaceManagementService.GetUserWorkspaceMemberships(c.Request.Context(), accountID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}

		// Build organization department set
		orgDeptSet := make(map[string]bool)
		for _, deptID := range orgDeptIDs {
			orgDeptSet[deptID] = true
		}

		// Calculate intersection
		workspaceIDs = make([]string, 0)
		for _, dept := range userDepts {
			if orgDeptSet[dept.WorkspaceID] {
				workspaceIDs = append(workspaceIDs, dept.WorkspaceID)
			}
		}
	}

	allGroupWorkspaceIDs := make([]string, 0, len(orgDeptIDs))
	for _, id := range orgDeptIDs {
		allGroupWorkspaceIDs = append(allGroupWorkspaceIDs, id)
	}

	workspaceID := req.WorkspaceID
	if workspaceID != "" {
		filteredWorkspaceIDs := make([]string, 0, len(workspaceIDs))
		for _, id := range workspaceIDs {
			if id == workspaceID {
				filteredWorkspaceIDs = append(filteredWorkspaceIDs, id)
				break
			}
		}
		workspaceIDs = filteredWorkspaceIDs

		filteredAllGroupWorkspaceIDs := make([]string, 0, len(allGroupWorkspaceIDs))
		for _, id := range allGroupWorkspaceIDs {
			if id == workspaceID {
				filteredAllGroupWorkspaceIDs = append(filteredAllGroupWorkspaceIDs, id)
			}
		}
		allGroupWorkspaceIDs = filteredAllGroupWorkspaceIDs
	}

	if h.organizationService != nil {
		if workspaceID != "" {
			has, err := h.organizationService.CheckWorkspaceOrganizationAnyPermission(
				c.Request.Context(),
				organizationID,
				workspaceID,
				accountID,
				workspace_model.WorkspacePermissionKnowledgeBaseView,
				workspace_model.WorkspacePermissionKnowledgeBaseManage,
				workspace_model.WorkspacePermissionKnowledgeBaseFolderManage,
			)
			if err != nil {
				response.Fail(c, response.ErrSystemError)
				return
			}
			if !has {
				responseData := gin.H{
					"data":     []dto.DatasetFolderDetailResponse{},
					"has_more": false,
					"limit":    req.Limit,
					"total":    0,
					"page":     req.Page,
				}
				response.Success(c, responseData)
				return
			}
		} else {
			if !(organizationRole == "owner" || organizationRole == "admin") {
				filtered := make([]string, 0, len(workspaceIDs))
				for _, tid := range workspaceIDs {
					has, err := h.organizationService.CheckWorkspaceOrganizationAnyPermission(
						c.Request.Context(),
						organizationID,
						tid,
						accountID,
						workspace_model.WorkspacePermissionKnowledgeBaseView,
						workspace_model.WorkspacePermissionKnowledgeBaseManage,
						workspace_model.WorkspacePermissionKnowledgeBaseFolderManage,
					)
					if err != nil {
						response.Fail(c, response.ErrSystemError)
						return
					}
					if has {
						filtered = append(filtered, tid)
					}
				}
				workspaceIDs = filtered
				allGroupWorkspaceIDs = filtered
			}
		}
	}

	if len(workspaceIDs) == 0 {
		responseData := gin.H{
			"data":     []dto.DatasetFolderDetailResponse{},
			"has_more": false,
			"limit":    req.Limit,
			"total":    0,
			"page":     req.Page,
		}
		response.Success(c, responseData)
		return
	}

	// Use paginated version with permission filtering
	result, err := h.folderService.ListFoldersWithPaginationWithPermissions(
		c.Request.Context(),
		organizationID,
		workspaceIDs,
		req.FolderID,
		accountID,
		isOrgAdmin,
		allGroupWorkspaceIDs,
		req.Page,
		req.Limit,
		req.Sort,
		req.Keyword,
	)
	if err != nil {
		// For other errors, return a system error with custom message
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	// Prepare resources for batch permission check
	resources := make([]interfaces.ResourcePermissionInfo, len(result.Folders))
	for i, folder := range result.Folders {
		resources[i] = interfaces.ResourcePermissionInfo{
			ResourceID:  folder.ID,
			WorkspaceID: folder.WorkspaceID,
			CreatedBy:   folder.CreatedBy,
			GroupID:     nil, // Folders are workspace-scoped and have no organization compatibility override
		}
	}

	// Batch check permissions
	permissionMap, err := h.permissionService.CheckBatchResourceEditPermission(c.Request.Context(), interfaces.BatchResourcePermissionParams{
		AccountID: accountID,
		Resources: resources,
	})
	if err != nil {
		// On error, default to false for all
		permissionMap = make(map[string]bool)
	}

	// Convert folders to detail response format
	folderDetailResponses := make([]dto.DatasetFolderDetailResponse, 0, len(result.Folders))
	for _, folder := range result.Folders {
		// Convert DatasetFolderResponse to DatasetFolder model
		workspaceID := folder.WorkspaceID
		folderModel := &model.DatasetFolder{
			ID:             folder.ID,
			WorkspaceID:    workspaceID,
			Name:           folder.Name,
			Description:    folder.Description,
			ParentID:       folder.ParentID,
			CreatedBy:      folder.CreatedBy,
			CreatedAt:      folder.CreatedAt,
			UpdatedBy:      folder.UpdatedBy,
			UpdatedAt:      folder.UpdatedAt,
			Icon:           folder.Icon,
			IconType:       folder.IconType,
			IconBackground: folder.IconBackground,
			Position:       folder.Position,
			Permission:     folder.Permission,
		}
		// Convert to detail response
		detailResponse := h.convertFolderToDetailResponse(c.Request.Context(), folderModel)

		// Set can_edit from batch permission check
		canEdit, exists := permissionMap[folder.ID]
		if !exists {
			canEdit = false
		}
		detailResponse.CanEdit = canEdit

		folderDetailResponses = append(folderDetailResponses, detailResponse)
	}

	// Prepare response with parent folder info
	responseData := gin.H{
		"data":     folderDetailResponses,
		"has_more": result.HasMore,
		"limit":    result.Limit,
		"total":    result.Total,
		"page":     result.Page,
	}

	// Add parent folder info if exists
	if result.ParentFolder != nil {
		responseData["parent_folder"] = result.ParentFolder
	}

	response.Success(c, responseData)
}

// getFoldersWithPagination retrieves folders with pagination
func (h *DatasetFolderHandler) getFoldersWithPagination(ctx context.Context, tenantIDs []string, page, limit int) ([]*model.DatasetFolder, int64, error) {
	return h.folderService.GetFoldersByWorkspaceIDs(ctx, tenantIDs, page, limit)
}

// convertFolderToResponse converts a DatasetFolder model to DatasetFolderResponse DTO
func (h *DatasetFolderHandler) convertFolderToResponse(folder *model.DatasetFolder) dto.DatasetFolderResponse {
	return dto.DatasetFolderResponse{
		ID:             folder.ID,
		WorkspaceID:    folder.WorkspaceID,
		Name:           folder.Name,
		Description:    folder.Description,
		ParentID:       folder.ParentID,
		CreatedBy:      folder.CreatedBy,
		CreatedAt:      folder.CreatedAt,
		UpdatedBy:      folder.UpdatedBy,
		UpdatedAt:      folder.UpdatedAt,
		Icon:           folder.Icon,
		IconType:       folder.IconType,
		IconBackground: folder.IconBackground,
		Position:       folder.Position,
		Permission:     folder.Permission,
	}
}

// convertFolderToExResponse converts a DatasetFolder model to DatasetFolderDetailResponse DTO with tenant info
func (h *DatasetFolderHandler) convertFolderToDetailResponse(ctx context.Context, folder *model.DatasetFolder) dto.DatasetFolderDetailResponse {
	response := dto.DatasetFolderDetailResponse{
		ID:             folder.ID,
		WorkspaceID:    folder.WorkspaceID,
		Name:           folder.Name,
		Description:    folder.Description,
		ParentID:       folder.ParentID,
		CreatedBy:      folder.CreatedBy,
		CreatedAt:      folder.CreatedAt,
		UpdatedBy:      folder.UpdatedBy,
		UpdatedAt:      folder.UpdatedAt,
		Icon:           folder.Icon,
		IconType:       folder.IconType,
		IconBackground: folder.IconBackground,
		Position:       folder.Position,
		Permission:     folder.Permission,
		Tenant:         map[string]interface{}{},
	}

	if ctx != nil && h.workspaceManagementService != nil {
		if tenant, err := h.workspaceManagementService.GetWorkspaceByID(ctx, folder.WorkspaceID); err == nil && tenant != nil {
			response.Tenant = map[string]interface{}{
				"id":   tenant.ID,
				"name": tenant.Name,
			}
		} else {
			response.Tenant = map[string]interface{}{
				"id":   folder.WorkspaceID,
				"name": "Unknown Tenant",
			}
		}
	} else {
		response.Tenant = map[string]interface{}{
			"id":   folder.WorkspaceID,
			"name": "Unknown Tenant",
		}
	}

	return response
}

// PostFolder handles POST /dataset-folders
func (h *DatasetFolderHandler) PostFolder(c *gin.Context) {
	accountID := c.GetString("account_id")
	organizationID := c.GetString("tenant_id")

	var req dto.DatasetFolderCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate name
	if err := h.validateFolderName(req.Name); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate parent folder if provided
	var parentFolder *model.DatasetFolder
	if req.ParentID != nil {
		if *req.ParentID == "" {
			req.ParentID = nil
		} else {
			if _, err := uuid.Parse(*req.ParentID); err != nil {
				response.Fail(c, response.ErrInvalidParam)
				return
			}

			var ok bool
			parentFolder, ok = authorizeDatasetFolderManageAccess(c, h.folderService, h.authorizationService, *req.ParentID)
			if !ok {
				return
			}
		}
	}

	// Set default permission
	permission := "only_me"
	if req.Permission != nil && *req.Permission != "" {
		permission = *req.Permission
	}

	folderWorkspaceID := ""
	resolveWorkspaceID := ""
	if req.WorkspaceID != nil && *req.WorkspaceID != "" {
		resolveWorkspaceID = *req.WorkspaceID
	}
	workspaceID := resolveWorkspaceID

	if workspaceID != "" {
		folderWorkspaceID = workspaceID
	}
	if parentFolder != nil {
		if folderWorkspaceID != "" && folderWorkspaceID != parentFolder.WorkspaceID {
			response.Fail(c, response.ErrDatasetPermissionDenied)
			return
		}
		folderWorkspaceID = parentFolder.WorkspaceID
	}
	if folderWorkspaceID == "" {
		response.Fail(c, response.ErrDatasetPermissionDenied)
		return
	}

	if !authorizeDatasetWorkspaceFolderManageAccess(c, h.authorizationService, folderWorkspaceID) {
		return
	}

	// Create folder
	folder := &model.DatasetFolder{
		OrganizationID: organizationID,
		WorkspaceID:    folderWorkspaceID,
		Name:           req.Name,
		Description:    req.Description,
		ParentID:       req.ParentID,
		CreatedBy:      accountID,
		Permission:     permission,
		Icon:           req.Icon,
		IconType:       req.IconType,
		IconBackground: req.IconBackground,
		Position:       0, // Default position
	}

	if req.Position != nil {
		folder.Position = *req.Position
	}

	createdFolder, err := h.folderService.CreateFolder(c.Request.Context(), folder)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, createdFolder)
}

// GetFolder handles GET /dataset-folders/:id
func (h *DatasetFolderHandler) GetFolder(c *gin.Context) {
	folderID := c.Param("folder_id")
	if folderID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(folderID); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")

	folder, ok := authorizeDatasetFolderViewAccess(c, h.folderService, h.authorizationService, folderID)
	if !ok {
		return
	}

	// Check edit permission
	canEdit, err := h.permissionService.CheckSingleResourceEditPermission(c.Request.Context(), interfaces.SingleResourcePermissionParams{
		AccountID: accountID,
		TenantID:  folder.WorkspaceID,
		CreatedBy: folder.CreatedBy,
		GroupID:   nil, // Folders are workspace-scoped and have no organization compatibility override
	})
	if err != nil {
		// On error, default to false
		canEdit = false
	}

	responseData := h.convertFolderToDetailResponse(c.Request.Context(), folder)
	responseData.CanEdit = canEdit

	response.Success(c, responseData)
}

// PatchFolder handles PATCH /dataset-folders/:id
func (h *DatasetFolderHandler) PatchFolder(c *gin.Context) {
	folderID := c.Param("folder_id")
	if folderID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(folderID); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req dto.DatasetFolderUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate name if provided
	if req.Name != nil {
		if err := h.validateFolderName(*req.Name); err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
	}

	currentFolder, ok := authorizeDatasetFolderManageAccess(c, h.folderService, h.authorizationService, folderID)
	if !ok {
		return
	}

	// Validate parent folder if provided
	if req.ParentID != nil {
		if *req.ParentID == "" {
			req.ParentID = nil
		} else {
			if _, err := uuid.Parse(*req.ParentID); err != nil {
				response.Fail(c, response.ErrInvalidParam)
				return
			}

			parentFolder, ok := authorizeDatasetFolderManageAccess(c, h.folderService, h.authorizationService, *req.ParentID)
			if !ok {
				return
			}
			if parentFolder.WorkspaceID != currentFolder.WorkspaceID {
				response.Fail(c, response.ErrDatasetPermissionDenied)
				return
			}
		}
	}

	// Prepare update data
	updateData := map[string]interface{}{}
	if req.Name != nil {
		updateData["name"] = *req.Name
	}
	if req.Description != nil {
		updateData["description"] = req.Description
	}
	if req.ParentID != nil {
		updateData["parent_id"] = req.ParentID
	}
	if req.Icon != nil {
		updateData["icon"] = req.Icon
	}
	if req.IconType != nil {
		updateData["icon_type"] = req.IconType
	}
	if req.IconBackground != nil {
		updateData["icon_background"] = req.IconBackground
	}
	if req.Position != nil {
		updateData["position"] = *req.Position
	}
	if req.Permission != nil {
		updateData["permission"] = *req.Permission
	}
	workspaceID := req.TenantID
	if req.WorkspaceID != nil {
		workspaceID = req.WorkspaceID
	}
	if workspaceID != nil {
		if *workspaceID == "" {
			response.Fail(c, response.ErrDatasetPermissionDenied)
			return
		}
		if *workspaceID != currentFolder.WorkspaceID && !authorizeDatasetWorkspaceFolderManageAccess(c, h.authorizationService, *workspaceID) {
			return
		}
		updateData["tenant_id"] = *workspaceID
	}

	updatedFolder, err := h.folderService.UpdateFolder(c.Request.Context(), folderID, updateData)
	if err != nil {
		// TODO: distinguish between not found and other errors
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, updatedFolder)
}

// DeleteFolder handles DELETE /dataset-folders/:id
func (h *DatasetFolderHandler) DeleteFolder(c *gin.Context) {
	folderID := c.Param("folder_id")
	if folderID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(folderID); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if _, ok := authorizeDatasetFolderManageAccess(c, h.folderService, h.authorizationService, folderID); !ok {
		return
	}

	err := h.folderService.DeleteFolder(c.Request.Context(), folderID)
	if err != nil {
		// TODO: distinguish between not found and other errors
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

// MoveDatasetToFolder handles POST /dataset-folders/move-dataset
func (h *DatasetFolderHandler) MoveDatasetToFolder(c *gin.Context) {
	accountID := c.GetString("account_id")

	var req dto.MoveDatasetToFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate UUID formats
	if _, err := uuid.Parse(req.DatasetID); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	dataset, ok := authorizeDatasetManageAccess(c, h.datasetService, h.authorizationService, req.DatasetID)
	if !ok {
		return
	}

	// Validate FolderID only if it's not empty
	if req.FolderID != "" {
		if _, err := uuid.Parse(req.FolderID); err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}

		folder, ok := authorizeDatasetFolderManageAccess(c, h.folderService, h.authorizationService, req.FolderID)
		if !ok {
			return
		}
		if folder.WorkspaceID != dataset.WorkspaceID {
			response.Fail(c, response.ErrDatasetPermissionDenied)
			return
		}
	}

	// Move dataset to folder (or to root if FolderID is empty)
	err := h.folderService.MoveDatasetToFolder(c.Request.Context(), req.DatasetID, req.FolderID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

// GetDatasetsByFolder handles GET /dataset-folders/datasets
func (h *DatasetFolderHandler) GetDatasetsByFolder(c *gin.Context) {
	accountID := c.GetString("account_id")
	organizationID := c.GetString("tenant_id")
	ctx := c.Request.Context()

	var req dto.DatasetListWithFoldersRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}
	if req.Sort == "" || (req.Sort != "asc" && req.Sort != "desc") {
		req.Sort = "desc"
	}

	emptyResult := &dto.DatasetListResponse{
		Data:    []dto.DatasetResponse{},
		HasMore: false,
		Limit:   req.Limit,
		Total:   0,
		Page:    req.Page,
	}

	if req.FolderID != "" {
		if _, err := uuid.Parse(req.FolderID); err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
		folder, ok := authorizeDatasetFolderViewAccess(c, h.folderService, h.authorizationService, req.FolderID)
		if !ok {
			return
		}
		if req.WorkspaceID != "" && req.WorkspaceID != folder.WorkspaceID {
			response.Success(c, emptyResult)
			return
		}
		req.WorkspaceID = folder.WorkspaceID
	}

	// Step 1: Get user's organization role
	orgRole, err := h.organizationService.GetUserOrganizationRole(ctx, organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	isOrgAdmin := orgRole == "owner" || orgRole == "admin"

	// Step 2: Get all workspace IDs in the organization
	orgWorkspaceIDs, err := h.workspaceManagementService.GetWorkspaceIDsByOrganizationID(ctx, organizationID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Step 3: Determine queryable workspace IDs based on permission
	var queryWorkspaceIDs []string

	if req.WorkspaceID != "" {
		// 3a: Specific workspace requested — verify it belongs to the organization
		belongs := false
		for _, id := range orgWorkspaceIDs {
			if id == req.WorkspaceID {
				belongs = true
				break
			}
		}
		if !belongs {
			response.Success(c, emptyResult)
			return
		}

		// Check knowledge_base.view permission (org admin/owner auto-passes inside)
		has, err := h.organizationService.CheckWorkspaceOrganizationAnyPermission(
			ctx, organizationID, req.WorkspaceID, accountID,
			workspace_model.WorkspacePermissionKnowledgeBaseView,
			workspace_model.WorkspacePermissionKnowledgeBaseManage,
			workspace_model.WorkspacePermissionKnowledgeBaseFolderManage,
		)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !has {
			response.Success(c, emptyResult)
			return
		}
		queryWorkspaceIDs = []string{req.WorkspaceID}
	} else {
		// 3b: No specific workspace — resolve all accessible workspaces
		if isOrgAdmin {
			queryWorkspaceIDs = orgWorkspaceIDs
		} else {
			// Get user's workspace memberships intersected with organization workspaces
			userMemberships, err := h.workspaceManagementService.GetUserWorkspaceMemberships(ctx, accountID)
			if err != nil {
				response.Fail(c, response.ErrSystemError)
				return
			}

			orgWorkspaceSet := make(map[string]bool, len(orgWorkspaceIDs))
			for _, id := range orgWorkspaceIDs {
				orgWorkspaceSet[id] = true
			}

			candidates := make([]string, 0, len(userMemberships))
			for _, m := range userMemberships {
				if orgWorkspaceSet[m.WorkspaceID] {
					candidates = append(candidates, m.WorkspaceID)
				}
			}

			// Filter by knowledge_base.view permission
			for _, tid := range candidates {
				has, err := h.organizationService.CheckWorkspaceOrganizationAnyPermission(
					ctx, organizationID, tid, accountID,
					workspace_model.WorkspacePermissionKnowledgeBaseView,
					workspace_model.WorkspacePermissionKnowledgeBaseManage,
					workspace_model.WorkspacePermissionKnowledgeBaseFolderManage,
				)
				if err != nil {
					response.Fail(c, response.ErrSystemError)
					return
				}
				if has {
					queryWorkspaceIDs = append(queryWorkspaceIDs, tid)
				}
			}
		}
	}

	if len(queryWorkspaceIDs) == 0 {
		response.Success(c, emptyResult)
		return
	}

	// Step 4: Query datasets
	result, err := h.folderService.ListDatasetsWithPaginationWithPermissions(
		ctx,
		organizationID,
		queryWorkspaceIDs,
		req.FolderID,
		accountID,
		isOrgAdmin,
		orgWorkspaceIDs,
		req.Page,
		req.Limit,
		req.Keyword,
		req.Sort,
	)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	// Step 5: Batch check edit permissions
	resources := make([]interfaces.ResourcePermissionInfo, len(result.Data))
	for i, dataset := range result.Data {
		resources[i] = interfaces.ResourcePermissionInfo{
			ResourceID:  dataset.ID,
			WorkspaceID: dataset.WorkspaceID,
			CreatedBy:   dataset.CreatedBy,
			GroupID:     nil, // Datasets are workspace-scoped and have no organization compatibility override
		}
	}

	permissionMap, err := h.permissionService.CheckBatchResourceEditPermission(ctx, interfaces.BatchResourcePermissionParams{
		AccountID: accountID,
		Resources: resources,
	})
	if err != nil {
		permissionMap = make(map[string]bool)
	}

	for i := range result.Data {
		canEdit := permissionMap[result.Data[i].ID]
		result.Data[i].CanEdit = canEdit
		result.Data[i].IsEditor = canEdit
	}

	response.Success(c, result)
}

// RegisterRoutes registers all dataset folder routes
func (h *DatasetFolderHandler) RegisterRoutes(router *gin.RouterGroup) {
	authWithTenant := router.Group("", middleware.JWTWithOrganizationAndService(h.accountService))

	// Dataset folder routes
	authWithTenant.GET("/dataset-folders", h.GetFolders)
	authWithTenant.POST("/dataset-folders", h.PostFolder)
	authWithTenant.GET("/dataset-folders/:folder_id", h.GetFolder)
	authWithTenant.PATCH("/dataset-folders/:folder_id", h.PatchFolder)
	authWithTenant.DELETE("/dataset-folders/:folder_id", h.DeleteFolder)
	authWithTenant.GET("/dataset-folders/datasets", h.GetDatasetsByFolder)
	authWithTenant.POST("/dataset-folders/move-dataset", h.MoveDatasetToFolder)
}

// Helper methods
func (h *DatasetFolderHandler) validateFolderName(name string) error {
	if len(name) < 1 || len(name) > 40 {
		return fmt.Errorf("Name must be between 1 to 40 characters")
	}
	return nil
}
