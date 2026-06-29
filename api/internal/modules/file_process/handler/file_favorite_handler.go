package handler

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/file_process/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

// FileFavoriteHandler handles file favorite-related HTTP requests
type FileFavoriteHandler struct {
	fileFavoriteService service.FileFavoriteService
	fileService         interfaces.FileService
	accountService      interfaces.AccountService
	enterpriseService   fileWorkspacePermissionChecker
}

// NewFileFavoriteHandler creates a new FileFavoriteHandler instance
func NewFileFavoriteHandler(
	fileFavoriteService service.FileFavoriteService,
	fileService interfaces.FileService,
	accountService interfaces.AccountService,
	enterpriseService interfaces.OrganizationService,
) *FileFavoriteHandler {
	return &FileFavoriteHandler{
		fileFavoriteService: fileFavoriteService,
		fileService:         fileService,
		accountService:      accountService,
		enterpriseService:   enterpriseService,
	}
}

// businessError is a helper function for business errors
func (h *FileFavoriteHandler) businessError(c *gin.Context, errorCode response.ErrorCode) {
	response.Fail(c, errorCode)
}

// businessErrorWithMessage is a helper function for business errors with custom message
func (h *FileFavoriteHandler) businessErrorWithMessage(c *gin.Context, errorCode response.ErrorCode, message string) {
	response.FailWithMessage(c, errorCode, message)
}

// FavoriteFile handles POST /file-favorites - favorite a file
func (h *FileFavoriteHandler) FavoriteFile(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		h.businessError(c, response.ErrUnauthorized)
		return
	}

	// Parse request
	var req dto.FileFavoriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.businessError(c, response.ErrInvalidParam)
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(req.FileID); err != nil {
		h.businessError(c, response.ErrInvalidParam)
		return
	}

	if _, ok := authorizeFileViewAccess(c, h.fileService, h.enterpriseService, req.FileID); !ok {
		return
	}

	// Favorite the file
	err := h.fileFavoriteService.FavoriteFile(c.Request.Context(), req.FileID, accountID)
	if err != nil {
		h.businessErrorWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

// UnfavoriteFile handles DELETE /file-favorites/:file_id - unfavorite a file
func (h *FileFavoriteHandler) UnfavoriteFile(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		h.businessError(c, response.ErrUnauthorized)
		return
	}

	fileID := c.Param("file_id")
	if fileID == "" {
		h.businessError(c, response.ErrInvalidParam)
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(fileID); err != nil {
		h.businessError(c, response.ErrInvalidParam)
		return
	}

	if _, ok := authorizeFileViewAccess(c, h.fileService, h.enterpriseService, fileID); !ok {
		return
	}

	// Unfavorite the file
	err := h.fileFavoriteService.UnfavoriteFile(c.Request.Context(), fileID, accountID)
	if err != nil {
		h.businessErrorWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

// BatchFavoriteFiles handles POST /file-favorites/batch - batch favorite files
func (h *FileFavoriteHandler) BatchFavoriteFiles(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		h.businessError(c, response.ErrUnauthorized)
		return
	}

	// Parse request
	var req dto.BatchFileFavoriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.businessError(c, response.ErrInvalidParam)
		return
	}

	// Validate UUIDs
	for _, fileID := range req.FileIDs {
		if _, err := uuid.Parse(fileID); err != nil {
			h.businessError(c, response.ErrInvalidParam)
			return
		}
	}

	for _, fileID := range req.FileIDs {
		if _, ok := authorizeFileViewAccess(c, h.fileService, h.enterpriseService, fileID); !ok {
			return
		}
	}

	// Batch favorite files
	err := h.fileFavoriteService.BatchFavoriteFiles(c.Request.Context(), req.FileIDs, accountID)
	if err != nil {
		// Check if it's a file not found error
		if fmt.Sprintf("%s", err.Error()) == "file not found" {
			h.businessError(c, response.ErrFileNotFound)
			return
		}
		h.businessError(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

// BatchUnfavoriteFiles handles POST /file-favorites/batch-unfavorite - batch unfavorite files
func (h *FileFavoriteHandler) BatchUnfavoriteFiles(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		h.businessError(c, response.ErrUnauthorized)
		return
	}

	// Parse request
	var req dto.BatchFileFavoriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.businessError(c, response.ErrInvalidParam)
		return
	}

	// Validate UUIDs
	for _, fileID := range req.FileIDs {
		if _, err := uuid.Parse(fileID); err != nil {
			h.businessError(c, response.ErrInvalidParam)
			return
		}
	}

	for _, fileID := range req.FileIDs {
		if _, ok := authorizeFileViewAccess(c, h.fileService, h.enterpriseService, fileID); !ok {
			return
		}
	}

	// Batch unfavorite files
	err := h.fileFavoriteService.BatchUnfavoriteFiles(c.Request.Context(), req.FileIDs, accountID)
	if err != nil {
		// Check if it's a file not found error
		if fmt.Sprintf("%s", err.Error()) == "file not found" {
			h.businessError(c, response.ErrFileNotFound)
			return
		}
		h.businessError(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

// ListFavorites handles GET /file-favorites - list favorite files
func (h *FileFavoriteHandler) ListFavorites(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		h.businessError(c, response.ErrUnauthorized)
		return
	}

	// Get canonical organization scope
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		h.businessError(c, response.ErrInvalidTenantId)
		return
	}

	// Parse request parameters
	var req dto.FileFavoriteListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.businessError(c, response.ErrInvalidParam)
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

	// List favorites
	favorites, total, err := h.fileFavoriteService.ListFavorites(c.Request.Context(), accountID, req.Page, req.Limit)
	if err != nil {
		h.businessError(c, response.ErrSystemError)
		return
	}

	// Convert to response DTOs and hide favorites whose file is no longer visible
	// in the current organization/workspace scope.
	favoriteResponses := make([]dto.FileFavoriteResponse, 0, len(favorites))
	for _, favorite := range favorites {
		allowed, err := h.canListFavoriteFile(c.Request.Context(), organizationID, accountID, favorite.FileID)
		if err != nil {
			h.businessError(c, response.ErrSystemError)
			return
		}
		if !allowed {
			continue
		}
		favoriteResponses = append(favoriteResponses, dto.FileFavoriteResponse{
			ID:        favorite.ID,
			FileID:    favorite.FileID,
			AccountID: favorite.AccountID,
			CreatedAt: favorite.CreatedAt,
		})
	}

	// Calculate has more
	filteredTotal := int64(len(favoriteResponses))
	hasMore := int64(req.Page*req.Limit) < total && filteredTotal == int64(len(favorites))

	response.Success(c, &dto.FileFavoriteListResponse{
		Data:    favoriteResponses,
		HasMore: hasMore,
		Limit:   req.Limit,
		Total:   filteredTotal,
		Page:    req.Page,
	})
}

func (h *FileFavoriteHandler) canListFavoriteFile(ctx context.Context, organizationID, accountID, fileID string) (bool, error) {
	uploadFile, err := h.fileService.GetFileByID(ctx, fileID)
	if err != nil || uploadFile == nil {
		return false, nil
	}
	if uploadFile.IsTemporary {
		return uploadFile.CreatedBy == accountID, nil
	}
	if uploadFile.OrganizationID != organizationID {
		return false, nil
	}

	workspaceID := getUploadFileWorkspaceID(uploadFile)
	if workspaceID == "" {
		return true, nil
	}
	if h.enterpriseService == nil {
		return false, nil
	}

	return hasWorkspaceFilePermission(ctx, h.enterpriseService, organizationID, accountID, workspaceID, fileReadablePermissionCodes()...)
}
