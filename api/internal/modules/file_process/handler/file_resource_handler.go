package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/zgiai/zgi/api/internal/dto"
	datalibraryservice "github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/internal/modules/file_process/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"go.uber.org/zap"
)

const folderDuplicateNameMessage = "同一级目录下已存在同名文件夹，请更换名称。"

// FileResourceHandler handles file and file resource-related HTTP requests
type FileResourceHandler struct {
	fileFolderService   service.FileFolderService
	fileService         interfaces.FileService
	accountService      interfaces.AccountService
	enterpriseService   interfaces.OrganizationService
	fileFavoriteService service.FileFavoriteService
	assetSummaryService datalibraryservice.FileAssetSummaryService
}

// NewFileResourceHandler creates a new FileResourceHandler instance
func NewFileResourceHandler(
	fileFolderService service.FileFolderService,
	fileService interfaces.FileService,
	accountService interfaces.AccountService,
	enterpriseService interfaces.OrganizationService,
	fileFavoriteService service.FileFavoriteService,
	assetSummaryServices ...datalibraryservice.FileAssetSummaryService,
) *FileResourceHandler {
	var assetSummaryService datalibraryservice.FileAssetSummaryService
	if len(assetSummaryServices) > 0 {
		assetSummaryService = assetSummaryServices[0]
	}
	return &FileResourceHandler{
		fileFolderService:   fileFolderService,
		fileService:         fileService,
		accountService:      accountService,
		fileFavoriteService: fileFavoriteService,
		enterpriseService:   enterpriseService,
		assetSummaryService: assetSummaryService,
	}
}

// GetFolders handles GET /file-folders
func (h *FileResourceHandler) GetFolders(c *gin.Context) {
	organizationID := util.GetOrganizationID(c)
	accountID := c.GetString("account_id")

	// Get query parameters
	var req dto.FileFolderListRequest
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

	visibleWorkspaceIDs, err := resolveVisibleWorkspaceIDs(
		c.Request.Context(),
		h.enterpriseService,
		organizationID,
		accountID,
		req.WorkspaceID,
		workspace_model.WorkspacePermissionFileView,
		workspace_model.WorkspacePermissionFileManage,
		workspace_model.WorkspacePermissionFileDownload,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if len(visibleWorkspaceIDs) == 0 {
		respondEmptyFileFolderList(c, req.Page, req.Limit)
		return
	}

	// List folders with permission filtering
	folders, total, err := h.fileFolderService.ListFoldersWithPermissionFilter(
		c.Request.Context(),
		organizationID,
		accountID,
		req.Page,
		req.Limit,
		req.Keyword,
		req.Sort,
		req.ParentID,
		req.WorkspaceID,
		visibleWorkspaceIDs,
	)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	// Convert to response DTOs
	folderResponses := make([]dto.FileFolderResponse, len(folders))
	for i, folder := range folders {
		folderResponse := h.convertFolderToResponse(folder)

		// Get file count for the folder
		fileCount, err := h.fileFolderService.GetFolderFileCount(c.Request.Context(), folder.ID)
		if err == nil {
			folderResponse.FileCount = fileCount
		}

		folderResponses[i] = folderResponse
	}

	// Calculate has more
	hasMore := int64(req.Page*req.Limit) < total

	response.Success(c, &dto.FileFolderListResponse{
		Data:    folderResponses,
		HasMore: hasMore,
		Limit:   req.Limit,
		Total:   total,
		Page:    req.Page,
	})
}

// PostFolder handles POST /file-folders
func (h *FileResourceHandler) PostFolder(c *gin.Context) {
	accountID := c.GetString("account_id")
	organizationID := util.GetOrganizationID(c)

	var req dto.FileFolderCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	// Validate name
	if err := h.validateFolderName(req.Name); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate parent folder if provided
	if req.ParentID != nil {
		// Handle empty string case by setting to nil
		if *req.ParentID == "" {
			req.ParentID = nil
		} else {
			if _, err := uuid.Parse(*req.ParentID); err != nil {
				response.Fail(c, response.ErrInvalidParam)
				return
			}

			// Check parent folder exists and belongs to the same tenant
			parentFolder, err := h.fileFolderService.GetFolderByID(c.Request.Context(), *req.ParentID)
			if err != nil {
				response.Fail(c, response.ErrInvalidParam)
				return
			}

			if parentFolder.OrganizationID != organizationID {
				response.Fail(c, response.ErrInvalidParam)
				return
			}
		}
	}

	// Set default permission
	workspaceID := req.TeamTenantID
	if req.WorkspaceID != nil && *req.WorkspaceID != "" {
		workspaceID = req.WorkspaceID
	}
	if workspaceID != nil && *workspaceID != "" {
		if _, err := uuid.Parse(*workspaceID); err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}

		role, err := h.accountService.GetOrganizationRoleByWorkspaceID(c.Request.Context(), accountID, *workspaceID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}

		if role != "owner" && role != "admin" {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	permission := model.FileFolderPermissionOnlyMe
	if req.Permission != nil && *req.Permission != "" {
		permission = model.FileFolderPermissionType(*req.Permission)
	}

	// Create folder
	folder := &model.FileFolder{
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
		Name:           req.Name,
		Description:    req.Description,
		ParentID:       req.ParentID,
		CreatedBy:      accountID,
		Permission:     string(permission),
		Icon:           req.Icon,
		IconType:       req.IconType,
		IconBackground: req.IconBackground,
		Position:       0, // Default position
	}

	if req.Position != nil {
		folder.Position = *req.Position
	}

	createdFolder, err := h.fileFolderService.CreateFolder(c.Request.Context(), folder)
	if err != nil {
		if errors.Is(err, service.ErrFolderNameConflict) {
			response.FailWithMessage(c, response.ErrFileFolderExists, folderDuplicateNameMessage)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Handle partial_team permission
	if permission == model.FileFolderPermissionPartialTeam && len(req.PartialWorkspaceList) > 0 {
		if err := h.fileFolderService.UpdatePartialWorkspaceList(c.Request.Context(), createdFolder.ID, req.PartialWorkspaceList, accountID); err != nil {
			response.FailWithMessage(c, response.ErrSystemError, "Failed to update partial workspace list: "+err.Error())
			return
		}
	}

	response.Success(c, h.convertFolderToResponse(createdFolder))
}

// GetFolder handles GET /file-folders/:id
func (h *FileResourceHandler) GetFolder(c *gin.Context) {
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

	// Get account and tenant IDs from context
	accountID := c.GetString("account_id")
	organizationID := util.GetOrganizationID(c)

	visibleWorkspaceIDs, err := resolveVisibleWorkspaceIDs(
		c.Request.Context(),
		h.enterpriseService,
		organizationID,
		accountID,
		"",
		workspace_model.WorkspacePermissionFileView,
		workspace_model.WorkspacePermissionFileManage,
		workspace_model.WorkspacePermissionFileDownload,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	folder, err := h.fileFolderService.GetFolderWithViewPermissionCheck(c.Request.Context(), folderID, accountID, organizationID, visibleWorkspaceIDs)
	if err != nil {
		// Check if the error is because the folder was not found
		if strings.Contains(err.Error(), "not found") {
			response.Fail(c, response.ErrFileNotFound)
			return
		}
		if strings.Contains(err.Error(), "permission") {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, h.convertFolderToResponse(folder))
}

// PatchFolder handles PATCH /file-folders/:id
func (h *FileResourceHandler) PatchFolder(c *gin.Context) {
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
	organizationID := util.GetOrganizationID(c)

	var req dto.FileFolderUpdateRequest
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

	// Validate parent folder if provided
	if req.ParentID != nil {
		// Handle empty string case by setting to nil
		if *req.ParentID == "" {
			req.ParentID = nil
		} else {
			if _, err := uuid.Parse(*req.ParentID); err != nil {
				response.Fail(c, response.ErrInvalidParam)
				return
			}

			// Check parent folder exists and belongs to the same tenant
			parentFolder, err := h.fileFolderService.GetFolderByID(c.Request.Context(), *req.ParentID)
			if err != nil {
				response.Fail(c, response.ErrInvalidParam)
				return
			}

			if parentFolder.OrganizationID != organizationID {
				response.Fail(c, response.ErrInvalidParam)
				return
			}
		}
	}

	// Check permission
	hasPermission, err := h.fileFolderService.CheckFolderEditorPermission(c.Request.Context(), folderID, accountID, organizationID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			response.Fail(c, response.ErrFileNotFound)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	if !hasPermission {
		response.Fail(c, response.ErrPermissionDenied)
		return
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

	updatedFolder, err := h.fileFolderService.UpdateFolder(c.Request.Context(), folderID, updateData)
	if err != nil {
		if errors.Is(err, service.ErrFolderNameConflict) {
			response.FailWithMessage(c, response.ErrFileFolderExists, folderDuplicateNameMessage)
			return
		}
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	// Handle partial_team permission
	if req.Permission != nil && model.FileFolderPermissionType(*req.Permission) == model.FileFolderPermissionPartialTeam {
		if len(req.PartialWorkspaceList) > 0 {
			if err := h.fileFolderService.UpdatePartialWorkspaceList(c.Request.Context(), folderID, req.PartialWorkspaceList, accountID); err != nil {
				response.FailWithMessage(c, response.ErrSystemError, "Failed to update partial team list: "+err.Error())
				return
			}
		}
	} else if req.Permission != nil && model.FileFolderPermissionType(*req.Permission) != model.FileFolderPermissionPartialTeam {
		// Clear partial team list if permission is changed from partial_team to something else
		if err := h.fileFolderService.ClearPartialWorkspaceList(c.Request.Context(), folderID); err != nil {
			// Log error but don't fail the request
			logger.WarnContext(c.Request.Context(), "failed to clear partial team list", "folder_id", folderID, err)
		}
	}

	response.Success(c, h.convertFolderToResponse(updatedFolder))
}

// DeleteFolder handles DELETE /file-folders/:id
func (h *FileResourceHandler) DeleteFolder(c *gin.Context) {
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
	organizationID := util.GetOrganizationID(c)

	// Check permission
	hasPermission, err := h.fileFolderService.CheckFolderEditorPermission(c.Request.Context(), folderID, accountID, organizationID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			response.Fail(c, response.ErrFileNotFound)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	if !hasPermission {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	err = h.fileFolderService.DeleteFolder(c.Request.Context(), folderID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

// GetFilesInFolder handles GET /file-folders/files
func (h *FileResourceHandler) GetFilesInFolder(c *gin.Context) {
	// Get folder ID from query parameter instead of path parameter
	folderID := c.Query("folder_id")

	// If folder ID is empty, it means we want files in the root folder
	// In this case, we'll pass an empty string to the service

	// Validate UUID format only if folderID is not empty
	if folderID != "" {
		if _, err := uuid.Parse(folderID); err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
	}

	// Parse request parameters
	var req dto.FileListInFolderRequest
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
	} else if req.Limit > 100 {
		req.Limit = 100
	}

	// Get organization and account IDs from context
	organizationID := util.GetOrganizationID(c)
	accountID := c.GetString("account_id")
	visibleWorkspaceIDs, err := resolveVisibleWorkspaceIDs(
		c.Request.Context(),
		h.enterpriseService,
		organizationID,
		accountID,
		req.WorkspaceID,
		workspace_model.WorkspacePermissionFileView,
		workspace_model.WorkspacePermissionFileManage,
		workspace_model.WorkspacePermissionFileDownload,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if len(visibleWorkspaceIDs) == 0 {
		respondEmptyFileList(c, req.Page, req.Limit)
		return
	}

	if folderID != "" {
		hasPermission, err := h.fileFolderService.CheckFolderViewPermission(
			c.Request.Context(),
			folderID,
			accountID,
			organizationID,
			visibleWorkspaceIDs,
		)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !hasPermission {
			respondEmptyFileList(c, req.Page, req.Limit)
			return
		}
	}

	// Call service to get files
	files, total, err := h.fileFolderService.ListFilesInFolderWithFilters(
		c.Request.Context(),
		folderID,
		req.Page,
		req.Limit,
		req.Keyword,
		req.Sort,
		req.Extension,
		&req.StartTime,
		&req.EndTime,
		organizationID,
		visibleWorkspaceIDs,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Convert to response format
	// Collect file IDs for batch query
	fileIDs := make([]string, len(files))
	for i, file := range files {
		fileIDs[i] = file.ID
	}

	// Batch check favorite status
	favoriteMap := make(map[string]bool)
	if accountID != "" {
		favoriteMap, err = h.fileFavoriteService.BatchCheckFavorites(c.Request.Context(), fileIDs, accountID)
		if err != nil {
			logger.WarnContext(c.Request.Context(), "failed to batch check file favorites",
				err,
				zap.String("account_id", accountID),
				zap.String("tenant_id", organizationID),
				zap.Int("file_count", len(fileIDs)),
			)
			// Don't fail the request, just log the error and continue without favorite info
		}
	}

	assetSummaries := map[string]datalibraryservice.FileAssetSummaryView{}
	if h.assetSummaryService != nil {
		assetSummaries, err = h.assetSummaryService.ListCurrentFileAssetSummaries(c.Request.Context(), datalibraryservice.FileAssetSummaryListInput{
			OrganizationID: organizationID,
			SourceFileIDs:  fileIDs,
		})
		if err != nil {
			logger.ErrorContext(c.Request.Context(), "failed to batch get file asset processing summaries",
				err,
				zap.String("account_id", accountID),
				zap.String("tenant_id", organizationID),
				zap.Int("file_count", len(fileIDs)),
			)
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	fileResponses := make([]dto.UploadFile, len(files))
	for i, file := range files {
		// Convert model to DTO
		fileResponse := dto.UploadFile{
			ID:            file.ID,
			TenantID:      file.OrganizationID,
			StorageType:   file.StorageType,
			Key:           file.Key,
			Name:          file.Name,
			Size:          file.Size,
			Extension:     file.Extension,
			MimeType:      file.MimeType,
			CreatedByRole: dto.CreatedByRole(file.CreatedByRole),
			CreatedBy:     file.CreatedBy,
			CreatedAt:     file.CreatedAt,
			Used:          file.Used,
			UsedBy:        file.UsedBy,
			UsedAt:        file.UsedAt,
			Hash:          file.Hash,
			SourceURL:     file.SourceURL,
			ContentText:   file.ContentText,
			IsFavorite:    favoriteMap[file.ID],
		}

		// Get related dataset count (distinct datasets through documents)
		relatedDatasetCount, err := h.fileFolderService.GetRelatedDatasetCount(c.Request.Context(), file.ID)
		if err == nil {
			fileResponse.RelatedDatasetCount = relatedDatasetCount
			// Set the generic related count field, currently equal to dataset count
			fileResponse.RelatedCount = relatedDatasetCount
		}

		if summary, exists := assetSummaries[file.ID]; exists {
			applyFileAssetSummaryToUploadFile(&fileResponse, summary)
		}

		fileResponses[i] = fileResponse
	}

	// Calculate has more
	hasMore := int64(req.Page*req.Limit) < total

	response.Success(c, &dto.FileListResponse{
		Data:    fileResponses,
		HasMore: hasMore,
		Limit:   req.Limit,
		Total:   total,
		Page:    req.Page,
	})
}

// MoveFilesToFolder handles POST /file-folders/move-files
func (h *FileResourceHandler) MoveFilesToFolder(c *gin.Context) {
	accountID := c.GetString("account_id")
	organizationID := util.GetOrganizationID(c)

	var req dto.MoveFilesToFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate UUID formats for all file IDs
	for _, fileID := range req.FileIDs {
		if _, err := uuid.Parse(fileID); err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
	}

	// Validate FolderID only if it's not empty
	if req.FolderID != "" {
		if _, err := uuid.Parse(req.FolderID); err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}

		// Check if folder exists and user has permission to access it
		_, err := h.fileFolderService.GetFolderWithPermissionCheck(c.Request.Context(), req.FolderID, accountID, organizationID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				response.Fail(c, response.ErrFileNotFound)
				return
			}
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	// Check if all files exist
	for _, fileID := range req.FileIDs {
		_, err := h.fileService.GetFileByID(c.Request.Context(), fileID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				response.Fail(c, response.ErrFileNotFound)
				return
			}
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	// Move files to folder (or to root if FolderID is empty)
	err := h.fileFolderService.MoveFilesToFolder(c.Request.Context(), req.FileIDs, req.FolderID, accountID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

// MoveFolderToFolder handles POST /file-folders/move-folder
func (h *FileResourceHandler) MoveFolderToFolder(c *gin.Context) {
	accountID := c.GetString("account_id")
	organizationID := util.GetOrganizationID(c)

	var req dto.MoveFolderToFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate UUID formats
	if _, err := uuid.Parse(req.FolderID); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Only validate TargetID if it's not empty (empty means moving to root)
	if req.TargetID != "" {
		if _, err := uuid.Parse(req.TargetID); err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
	}

	// Check if folder exists and user has permission to access it
	_, err := h.fileFolderService.GetFolderWithPermissionCheck(c.Request.Context(), req.FolderID, accountID, organizationID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			response.Fail(c, response.ErrFileNotFound)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Check if target folder exists and user has permission to access it (if not moving to root)
	if req.TargetID != "" {
		_, err := h.fileFolderService.GetFolderWithPermissionCheck(c.Request.Context(), req.TargetID, accountID, organizationID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				response.Fail(c, response.ErrFileNotFound)
				return
			}
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	// Move folder to target folder
	err = h.fileFolderService.MoveFolderToFolder(c.Request.Context(), req.FolderID, req.TargetID, accountID, organizationID)
	if err != nil {
		if errors.Is(err, service.ErrFolderNameConflict) {
			response.FailWithMessage(c, response.ErrFileFolderExists, folderDuplicateNameMessage)
			return
		}
		// Check for specific error messages
		if strings.Contains(err.Error(), "cannot move folder to itself") {
			response.FailWithMessage(c, response.ErrInvalidParam, "Cannot move folder to itself")
			return
		}
		if strings.Contains(err.Error(), "folder is already in the target folder") {
			response.FailWithMessage(c, response.ErrInvalidParam, "Folder is already in the target folder")
			return
		}
		if strings.Contains(err.Error(), "cannot move folder to its own descendant") {
			response.FailWithMessage(c, response.ErrInvalidParam, "Cannot move folder to its own descendant")
			return
		}
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

// GetRelatedDocuments handles GET /files/:file_id/related-documents
func (h *FileResourceHandler) GetRelatedDocuments(c *gin.Context) {
	fileID := c.Param("file_id")
	if fileID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(fileID); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Get related documents
	documents, err := h.fileFolderService.GetRelatedDocuments(c.Request.Context(), fileID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, gin.H{
		"file_id": fileID,
		"data":    documents,
	})
}

// GetRelatedDatasets handles GET /files/:file_id/related-datasets
func (h *FileResourceHandler) GetRelatedDatasets(c *gin.Context) {
	fileID := c.Param("file_id")
	if fileID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(fileID); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Get related datasets
	datasets, err := h.fileFolderService.GetRelatedDatasets(c.Request.Context(), fileID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, gin.H{
		"file_id": fileID,
		"data":    datasets,
	})
}

// GetRelatedResources handles GET /files/:file_id/related-resources
// This is a generic endpoint to get all related resources for a file
func (h *FileResourceHandler) GetRelatedResources(c *gin.Context) {
	fileID := c.Param("file_id")
	if fileID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(fileID); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Get related documents count
	relatedDocumentCount, err := h.fileFolderService.GetRelatedDocumentCount(c.Request.Context(), fileID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	// Get related datasets with full content
	relatedDatasets, err := h.fileFolderService.GetRelatedDatasets(c.Request.Context(), fileID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, gin.H{
		"file_id": fileID,
		"data": map[string]interface{}{
			"documents": gin.H{
				"count": relatedDocumentCount,
			},
			"datasets": gin.H{
				"count": len(relatedDatasets),
				"items": relatedDatasets,
			},
			// Can be extended with other resource types
		},
	})
}

// ListAllFiles handles GET /file-folders/all-files
// This endpoint lists all files regardless of folder structure with pagination support
func (h *FileResourceHandler) ListAllFiles(c *gin.Context) {
	// Get organization and account IDs from context
	organizationID := util.GetOrganizationID(c)
	accountID := c.GetString("account_id")

	// Parse request parameters
	var req dto.FileListInFolderRequest
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
	} else if req.Limit > 100 {
		req.Limit = 100
	}

	visibleWorkspaceIDs, err := resolveVisibleWorkspaceIDs(
		c.Request.Context(),
		h.enterpriseService,
		organizationID,
		accountID,
		req.WorkspaceID,
		workspace_model.WorkspacePermissionFileView,
		workspace_model.WorkspacePermissionFileManage,
		workspace_model.WorkspacePermissionFileDownload,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if len(visibleWorkspaceIDs) == 0 {
		respondEmptyFileList(c, req.Page, req.Limit)
		return
	}

	// Call service to get all files
	files, total, err := h.fileFolderService.ListAllFilesWithFilters(
		c.Request.Context(),
		req.Page,
		req.Limit,
		req.Keyword,
		req.Sort,
		req.Extension,
		req.ProcessingStatus,
		&req.StartTime,
		&req.EndTime,
		organizationID,
		accountID,
		visibleWorkspaceIDs,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Convert to response format
	// Collect file IDs for batch query
	fileIDs := make([]string, len(files))
	for i, file := range files {
		fileIDs[i] = file.ID
	}

	// Batch get related dataset counts
	relatedDatasetCounts, err := h.fileFolderService.BatchGetRelatedDatasetCount(c.Request.Context(), fileIDs)
	if err != nil {
		logger.ErrorContext(c.Request.Context(), "failed to batch get related dataset counts",
			err,
			zap.String("account_id", accountID),
			zap.String("tenant_id", organizationID),
			zap.Int("file_count", len(fileIDs)),
		)
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Batch check favorite status
	favoriteMap := make(map[string]bool)
	if accountID != "" {
		favoriteMap, err = h.fileFavoriteService.BatchCheckFavorites(c.Request.Context(), fileIDs, accountID)
		if err != nil {
			logger.WarnContext(c.Request.Context(), "failed to batch check file favorites",
				err,
				zap.String("account_id", accountID),
				zap.String("tenant_id", organizationID),
				zap.Int("file_count", len(fileIDs)),
			)
			// Don't fail the request, just log the error and continue without favorite info
		}
	}

	assetSummaries := map[string]datalibraryservice.FileAssetSummaryView{}
	if h.assetSummaryService != nil {
		assetSummaries, err = h.assetSummaryService.ListCurrentFileAssetSummaries(c.Request.Context(), datalibraryservice.FileAssetSummaryListInput{
			OrganizationID: organizationID,
			SourceFileIDs:  fileIDs,
		})
		if err != nil {
			logger.ErrorContext(c.Request.Context(), "failed to batch get file asset processing summaries",
				err,
				zap.String("account_id", accountID),
				zap.String("tenant_id", organizationID),
				zap.Int("file_count", len(fileIDs)),
			)
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	fileResponses := make([]dto.UploadFile, len(files))
	for i, file := range files {
		// Convert model to DTO
		fileResponse := dto.UploadFile{
			ID:            file.ID,
			TenantID:      file.OrganizationID,
			TeamTenantID:  file.WorkspaceID,
			WorkspaceID:   file.WorkspaceID,
			StorageType:   file.StorageType,
			Key:           file.Key,
			Name:          file.Name,
			Size:          file.Size,
			Extension:     file.Extension,
			MimeType:      file.MimeType,
			CreatedByRole: dto.CreatedByRole(file.CreatedByRole),
			CreatedBy:     file.CreatedBy,
			CreatedAt:     file.CreatedAt,
			Used:          file.Used,
			UsedBy:        file.UsedBy,
			UsedAt:        file.UsedAt,
			Hash:          file.Hash,
			SourceURL:     file.SourceURL,
			IsFavorite:    favoriteMap[file.ID],
			// ContentText:   file.ContentText,
		}

		// Get related dataset count from batch result
		if relatedDatasetCount, exists := relatedDatasetCounts[file.ID]; exists {
			fileResponse.RelatedDatasetCount = relatedDatasetCount
			// Set the generic related count field, currently equal to dataset count
			fileResponse.RelatedCount = relatedDatasetCount
		}

		if summary, exists := assetSummaries[file.ID]; exists {
			applyFileAssetSummaryToUploadFile(&fileResponse, summary)
		}

		fileResponses[i] = fileResponse
	}

	// Calculate has more
	hasMore := int64(req.Page*req.Limit) < total

	response.Success(c, &dto.FileListResponse{
		Data:    fileResponses,
		HasMore: hasMore,
		Limit:   req.Limit,
		Total:   total,
		Page:    req.Page,
	})
}

func applyFileAssetSummaryToUploadFile(file *dto.UploadFile, summary datalibraryservice.FileAssetSummaryView) {
	if file == nil {
		return
	}
	file.AssetID = summary.AssetID.String()
	file.ProcessingStatus = summary.ProductStatus
	file.ProcessingStage = summary.ProcessingStage
	file.ProcessingProgress = summary.ProcessingProgress
	if summary.ActiveProcessingRequestID != nil {
		file.ProcessingRequestID = summary.ActiveProcessingRequestID.String()
	}
	if summary.ProcessingRunID != nil {
		file.ProcessingRunID = summary.ProcessingRunID.String()
	}
	file.GenerationNo = summary.GenerationNo
	file.PendingConfirmCount = summary.PendingConfirmationCount
	file.ChunkCount = summary.ChunkCount
	file.EmbeddingCount = summary.EmbeddingCount
	file.VectorStatus = summary.VectorStatus
	file.LastErrorCode = summary.LastErrorCode
	file.LastErrorMessage = summary.LastErrorMessage
}

// ListRecentFiles handles GET /file-folders/recent-files
// This endpoint lists recent files (within last 3 months, max 20 items)
func (h *FileResourceHandler) ListRecentFiles(c *gin.Context) {
	// Get organization and account IDs from context
	organizationID := util.GetOrganizationID(c)
	accountID := c.GetString("account_id")

	// Parse request parameters
	var req dto.FileListInFolderRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Set default values
	if req.Page <= 0 {
		req.Page = 1
	}

	// Set default limit to RecentFilesLimit if not specified or greater than RecentFilesLimit
	if req.Limit <= 0 || req.Limit > service.RecentFilesLimit {
		req.Limit = service.RecentFilesLimit
	}

	// Set default start time to 3 months ago if not specified
	if req.StartTime.IsZero() {
		threeMonthsAgo := time.Now().AddDate(0, -3, 0)
		req.StartTime = threeMonthsAgo
	}

	visibleWorkspaceIDs, err := resolveVisibleWorkspaceIDs(
		c.Request.Context(),
		h.enterpriseService,
		organizationID,
		accountID,
		req.WorkspaceID,
		workspace_model.WorkspacePermissionFileView,
		workspace_model.WorkspacePermissionFileManage,
		workspace_model.WorkspacePermissionFileDownload,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if len(visibleWorkspaceIDs) == 0 {
		respondEmptyFileList(c, req.Page, req.Limit)
		return
	}

	// Call service to get recent files
	files, total, err := h.fileFolderService.ListAllFilesWithFilters(
		c.Request.Context(),
		req.Page,
		req.Limit,
		req.Keyword,
		req.Sort,
		req.Extension,
		req.ProcessingStatus,
		&req.StartTime,
		&req.EndTime,
		organizationID,
		accountID,
		visibleWorkspaceIDs,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Convert to response format
	// Collect file IDs for batch query
	fileIDs := make([]string, len(files))
	for i, file := range files {
		fileIDs[i] = file.ID
	}

	// Batch get related dataset counts
	relatedDatasetCounts, err := h.fileFolderService.BatchGetRelatedDatasetCount(c.Request.Context(), fileIDs)
	if err != nil {
		logger.ErrorContext(c.Request.Context(), "failed to batch get related dataset counts for recent files",
			err,
			zap.String("account_id", accountID),
			zap.String("tenant_id", organizationID),
			zap.Int("file_count", len(fileIDs)),
		)
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Batch check favorite status
	favoriteMap := make(map[string]bool)
	if accountID != "" {
		favoriteMap, err = h.fileFavoriteService.BatchCheckFavorites(c.Request.Context(), fileIDs, accountID)
		if err != nil {
			logger.WarnContext(c.Request.Context(), "failed to batch check favorite recent files",
				err,
				zap.String("account_id", accountID),
				zap.String("tenant_id", organizationID),
				zap.Int("file_count", len(fileIDs)),
			)
			// Don't fail the request, just log the error and continue without favorite info
		}
	}

	assetSummaries := map[string]datalibraryservice.FileAssetSummaryView{}
	if h.assetSummaryService != nil {
		assetSummaries, err = h.assetSummaryService.ListCurrentFileAssetSummaries(c.Request.Context(), datalibraryservice.FileAssetSummaryListInput{
			OrganizationID: organizationID,
			SourceFileIDs:  fileIDs,
		})
		if err != nil {
			logger.ErrorContext(c.Request.Context(), "failed to batch get recent file asset processing summaries",
				err,
				zap.String("account_id", accountID),
				zap.String("tenant_id", organizationID),
				zap.Int("file_count", len(fileIDs)),
			)
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	fileResponses := make([]dto.UploadFile, len(files))
	for i, file := range files {
		// Convert model to DTO
		fileResponse := dto.UploadFile{
			ID:            file.ID,
			TenantID:      file.OrganizationID,
			StorageType:   file.StorageType,
			Key:           file.Key,
			Name:          file.Name,
			Size:          file.Size,
			Extension:     file.Extension,
			MimeType:      file.MimeType,
			CreatedByRole: dto.CreatedByRole(file.CreatedByRole),
			CreatedBy:     file.CreatedBy,
			CreatedAt:     file.CreatedAt,
			Used:          file.Used,
			UsedBy:        file.UsedBy,
			UsedAt:        file.UsedAt,
			Hash:          file.Hash,
			SourceURL:     file.SourceURL,
			ContentText:   file.ContentText,
			IsFavorite:    favoriteMap[file.ID],
		}

		// Get related dataset count from batch result
		if relatedDatasetCount, exists := relatedDatasetCounts[file.ID]; exists {
			fileResponse.RelatedDatasetCount = relatedDatasetCount
			// Set the generic related count field, currently equal to dataset count
			fileResponse.RelatedCount = relatedDatasetCount
		}

		if summary, exists := assetSummaries[file.ID]; exists {
			applyFileAssetSummaryToUploadFile(&fileResponse, summary)
		}

		fileResponses[i] = fileResponse
	}

	// Calculate has more
	hasMore := int64(req.Page*req.Limit) < total

	response.Success(c, &dto.FileListResponse{
		Data:    fileResponses,
		HasMore: hasMore,
		Limit:   req.Limit,
		Total:   total,
		Page:    req.Page,
	})
}

// ListFavoriteFiles handles GET /file-folders/favorite-files
// This endpoint lists favorite files with full file details
func (h *FileResourceHandler) ListFavoriteFiles(c *gin.Context) {
	// Get account and organization IDs from context
	accountID := c.GetString("account_id")
	organizationID := util.GetOrganizationID(c)

	// Parse request parameters
	var req dto.FileListInFolderRequest
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
	} else if req.Limit > 100 {
		req.Limit = 100
	}

	visibleWorkspaceIDs, err := resolveVisibleWorkspaceIDs(
		c.Request.Context(),
		h.enterpriseService,
		organizationID,
		accountID,
		req.WorkspaceID,
		workspace_model.WorkspacePermissionFileView,
		workspace_model.WorkspacePermissionFileManage,
		workspace_model.WorkspacePermissionFileDownload,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if len(visibleWorkspaceIDs) == 0 {
		respondEmptyFileList(c, req.Page, req.Limit)
		return
	}

	// Call service to get favorite files
	files, total, err := h.fileFolderService.ListFavoriteFiles(
		c.Request.Context(),
		accountID,
		req.Page,
		req.Limit,
		req.Keyword,
		req.Sort,
		req.Extension,
		&req.StartTime,
		&req.EndTime,
		organizationID,
		visibleWorkspaceIDs,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Convert to response format
	// Collect file IDs for batch query
	fileIDs := make([]string, len(files))
	for i, file := range files {
		fileIDs[i] = file.ID
	}

	// Batch get related dataset counts
	relatedDatasetCounts, err := h.fileFolderService.BatchGetRelatedDatasetCount(c.Request.Context(), fileIDs)
	if err != nil {
		logger.ErrorContext(c.Request.Context(), "failed to batch get related dataset counts for favorite files",
			err,
			zap.String("account_id", accountID),
			zap.String("tenant_id", organizationID),
			zap.Int("file_count", len(fileIDs)),
		)
		response.Fail(c, response.ErrSystemError)
		return
	}

	assetSummaries := map[string]datalibraryservice.FileAssetSummaryView{}
	if h.assetSummaryService != nil {
		assetSummaries, err = h.assetSummaryService.ListCurrentFileAssetSummaries(c.Request.Context(), datalibraryservice.FileAssetSummaryListInput{
			OrganizationID: organizationID,
			SourceFileIDs:  fileIDs,
		})
		if err != nil {
			logger.ErrorContext(c.Request.Context(), "failed to batch get favorite file asset processing summaries",
				err,
				zap.String("account_id", accountID),
				zap.String("tenant_id", organizationID),
				zap.Int("file_count", len(fileIDs)),
			)
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	fileResponses := make([]dto.UploadFile, len(files))
	for i, file := range files {
		// Convert model to DTO
		// All files in favorite list are favorited, so IsFavorite is always true
		fileResponse := dto.UploadFile{
			ID:            file.ID,
			TenantID:      file.OrganizationID,
			StorageType:   file.StorageType,
			Key:           file.Key,
			Name:          file.Name,
			Size:          file.Size,
			Extension:     file.Extension,
			MimeType:      file.MimeType,
			CreatedByRole: dto.CreatedByRole(file.CreatedByRole),
			CreatedBy:     file.CreatedBy,
			CreatedAt:     file.CreatedAt,
			Used:          file.Used,
			UsedBy:        file.UsedBy,
			UsedAt:        file.UsedAt,
			Hash:          file.Hash,
			SourceURL:     file.SourceURL,
			ContentText:   file.ContentText,
			IsFavorite:    true,
		}

		// Get related dataset count from batch result
		if relatedDatasetCount, exists := relatedDatasetCounts[file.ID]; exists {
			fileResponse.RelatedDatasetCount = relatedDatasetCount
			// Set the generic related count field, currently equal to dataset count
			fileResponse.RelatedCount = relatedDatasetCount
		}

		if summary, exists := assetSummaries[file.ID]; exists {
			applyFileAssetSummaryToUploadFile(&fileResponse, summary)
		}

		fileResponses[i] = fileResponse
	}

	// Calculate has more
	hasMore := int64(req.Page*req.Limit) < total

	response.Success(c, &dto.FileListResponse{
		Data:    fileResponses,
		HasMore: hasMore,
		Limit:   req.Limit,
		Total:   total,
		Page:    req.Page,
	})
}

// ArchiveFiles handles POST /files/archive
func (h *FileResourceHandler) ArchiveFiles(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Parse request
	var req dto.ArchiveFilesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate UUID formats
	for _, fileID := range req.FileIDs {
		if _, err := uuid.Parse(fileID); err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
	}

	// Archive the files
	err := h.fileFolderService.ArchiveFiles(c.Request.Context(), req.FileIDs, accountID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

// UnarchiveFiles handles POST /files/unarchive
func (h *FileResourceHandler) UnarchiveFiles(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Parse request
	var req dto.ArchiveFilesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate UUID formats
	for _, fileID := range req.FileIDs {
		if _, err := uuid.Parse(fileID); err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
	}

	// Unarchive the files
	err := h.fileFolderService.UnarchiveFiles(c.Request.Context(), req.FileIDs, accountID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

// Helper methods
func (h *FileResourceHandler) validateFolderName(name string) error {
	if len(name) < 1 || len(name) > 40 {
		return fmt.Errorf("name must be between 1 to 40 characters")
	}
	return nil
}

func (h *FileResourceHandler) convertFolderToResponse(folder *model.FileFolder) dto.FileFolderResponse {
	response := dto.FileFolderResponse{
		ID:             folder.ID,
		TenantID:       folder.OrganizationID,
		OrganizationID: folder.OrganizationID,
		TeamTenantID:   folder.WorkspaceID,
		WorkspaceID:    folder.WorkspaceID,
		Name:           folder.Name,
		Description:    folder.Description,
		ParentID:       folder.ParentID,
		CreatedBy:      folder.CreatedBy,
		CreatedAt:      folder.CreatedAt,
		UpdatedBy:      folder.UpdatedBy,
		UpdatedAt:      folder.UpdatedAt,
		IconType:       folder.IconType,
		Icon:           folder.Icon,
		IconBackground: folder.IconBackground,
		Position:       folder.Position,
		Permission:     folder.Permission,
		FileCount:      0, // Default value, will be updated when needed
	}

	// If the folder has partial_team permission, get the tenant list
	if model.FileFolderPermissionType(folder.Permission) == model.FileFolderPermissionPartialTeam {
		if tenantIDs, err := h.fileFolderService.GetFolderPermissionTenants(context.Background(), folder.ID); err == nil {
			response.PartialTeamList = tenantIDs
		}
	}

	return response
}

// GetFileStatistics handles GET /file-statistics
// This endpoint returns various file statistics for the tenant
func (h *FileResourceHandler) GetFileStatistics(c *gin.Context) {
	// Get account and organization IDs from context
	accountID := c.GetString("account_id")
	organizationID := util.GetOrganizationID(c)

	// Get total file count
	totalCount, err := h.fileFolderService.GetTotalFileCount(c.Request.Context(), organizationID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	// Get recent file count (within last 3 months) with limit
	recentCount, err := h.fileFolderService.GetRecentFileCountWithLimit(c.Request.Context(), organizationID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	// Get favorite file count
	favoriteCount, err := h.fileFolderService.GetFavoriteFileCount(c.Request.Context(), accountID, organizationID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	// Get root folder file count
	rootFolderCount, err := h.fileFolderService.GetRootFolderFileCount(c.Request.Context(), organizationID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	// Get archived file count
	archivedCount, err := h.fileFolderService.GetArchivedFileCount(c.Request.Context(), organizationID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	// Build response
	response.Success(c, &dto.FileStatisticsResponse{
		TotalCount:      totalCount,
		RecentCount:     recentCount,
		FavoriteCount:   favoriteCount,
		RootFolderCount: rootFolderCount,
		ArchivedCount:   archivedCount,
	})
}

// GetFolderPermissionTenants handles GET /file-folders/:folder_id/permission-tenants
// This endpoint returns the list of tenant IDs that have permission to access a folder
func (h *FileResourceHandler) GetFolderPermissionTenants(c *gin.Context) {
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

	// Get permission tenants
	tenantIDs, err := h.fileFolderService.GetFolderPermissionTenants(c.Request.Context(), folderID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, &dto.FileFolderPermissionTenantListResponse{
		Data: tenantIDs,
	})
}

// GetFolderPermissionTenantDetails handles GET /file-folders/:folder_id/permission-tenant-details
// This endpoint returns the list of tenants with details that have permission to access a folder
func (h *FileResourceHandler) GetFolderPermissionTenantDetails(c *gin.Context) {
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

	// Get permission tenant details
	tenantDetails, err := h.fileFolderService.GetFolderPermissionTenantDetails(c.Request.Context(), folderID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	// Convert to the correct type to avoid type mismatch
	var typedTenantDetails []dto.FileFolderPermissionTenantDetail
	for _, detail := range tenantDetails {
		typedTenantDetails = append(typedTenantDetails, dto.FileFolderPermissionTenantDetail{
			TenantID:      detail.TenantID,
			TenantName:    detail.TenantName,
			WorkspaceID:   detail.WorkspaceID,
			WorkspaceName: detail.WorkspaceName,
		})
	}

	response.Success(c, &dto.FileFolderPermissionTenantDetailListResponse{
		Data: typedTenantDetails,
	})
}
