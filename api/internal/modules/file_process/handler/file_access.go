package handler

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/zgiai/zgi/api/internal/dto"
	file_model "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

type fileWorkspacePermissionChecker interface {
	CheckWorkspaceOrganizationAnyPermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCodes ...workspace_model.WorkspacePermissionCode) (bool, error)
	IsOrganizationAdminOrOwner(ctx context.Context, organizationID, accountID string) (bool, error)
}

type fileFolderPermissionReader interface {
	GetFolderByID(ctx context.Context, id string) (*file_model.FileFolder, error)
	GetFolderPermissionTenants(ctx context.Context, folderID string) ([]string, error)
}

func authorizeFileDownloadAccess(c *gin.Context, fileService interfaces.FileService, permissionChecker fileWorkspacePermissionChecker, fileID string) (*dto.UploadFile, bool) {
	return authorizeFileAccess(
		c,
		fileService,
		permissionChecker,
		fileID,
		workspace_model.WorkspacePermissionFileDownload,
	)
}

func authorizeFileViewAccess(c *gin.Context, fileService interfaces.FileService, permissionChecker fileWorkspacePermissionChecker, fileID string) (*dto.UploadFile, bool) {
	return authorizeFileAccess(
		c,
		fileService,
		permissionChecker,
		fileID,
		workspace_model.WorkspacePermissionFileView,
		workspace_model.WorkspacePermissionFileDownload,
	)
}

func authorizeFileManageAccess(c *gin.Context, fileService interfaces.FileService, permissionChecker fileWorkspacePermissionChecker, fileID string) (*dto.UploadFile, bool) {
	return authorizeFileAccess(
		c,
		fileService,
		permissionChecker,
		fileID,
		workspace_model.WorkspacePermissionFileManage,
	)
}

func authorizeFileFolderViewAccess(c *gin.Context, folderService fileFolderPermissionReader, permissionChecker fileWorkspacePermissionChecker, folderID string) (*file_model.FileFolder, bool) {
	return authorizeFileFolderAccess(c, folderService, permissionChecker, folderID, false)
}

func authorizeFileFolderManageAccess(c *gin.Context, folderService fileFolderPermissionReader, permissionChecker fileWorkspacePermissionChecker, folderID string) (*file_model.FileFolder, bool) {
	return authorizeFileFolderAccess(c, folderService, permissionChecker, folderID, true)
}

func authorizeFileAccess(c *gin.Context, fileService interfaces.FileService, permissionChecker fileWorkspacePermissionChecker, fileID string, permissions ...workspace_model.WorkspacePermissionCode) (*dto.UploadFile, bool) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return nil, false
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return nil, false
	}

	uploadFile, err := fileService.GetFileByID(c.Request.Context(), fileID)
	if err != nil || uploadFile == nil {
		response.Fail(c, response.ErrFileNotFound)
		return nil, false
	}
	if uploadFile.IsTemporary {
		if uploadFile.CreatedBy != accountID {
			response.Fail(c, response.ErrPermissionDenied)
			return nil, false
		}
		return uploadFile, true
	}

	if uploadFile.OrganizationID != organizationID {
		response.Fail(c, response.ErrFileNotFound)
		return nil, false
	}

	workspaceID := getUploadFileWorkspaceID(uploadFile)
	if workspaceID == "" {
		if requiresFileManagePermission(permissions) && uploadFile.CreatedBy != accountID {
			if permissionChecker == nil {
				response.Fail(c, response.ErrSystemError)
				return nil, false
			}
			isAdmin, err := permissionChecker.IsOrganizationAdminOrOwner(c.Request.Context(), organizationID, accountID)
			if err != nil {
				response.Fail(c, response.ErrSystemError)
				return nil, false
			}
			if !isAdmin {
				response.Fail(c, response.ErrPermissionDenied)
				return nil, false
			}
		}
		return uploadFile, true
	}
	if !checkWorkspaceFilePermission(c, permissionChecker, organizationID, accountID, workspaceID, permissions...) {
		return nil, false
	}

	return uploadFile, true
}

func authorizeFileFolderAccess(c *gin.Context, folderService fileFolderPermissionReader, permissionChecker fileWorkspacePermissionChecker, folderID string, requireManage bool) (*file_model.FileFolder, bool) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return nil, false
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return nil, false
	}
	if folderService == nil {
		response.Fail(c, response.ErrSystemError)
		return nil, false
	}

	folder, err := folderService.GetFolderByID(c.Request.Context(), folderID)
	if err != nil || folder == nil {
		response.Fail(c, response.ErrFileNotFound)
		return nil, false
	}
	if folder.OrganizationID != organizationID {
		response.Fail(c, response.ErrFileNotFound)
		return nil, false
	}
	if folder.CreatedBy == accountID {
		return folder, true
	}

	if permissionChecker == nil {
		response.Fail(c, response.ErrSystemError)
		return nil, false
	}
	isAdmin, err := permissionChecker.IsOrganizationAdminOrOwner(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return nil, false
	}
	if isAdmin {
		return folder, true
	}

	workspaceID := getFileFolderWorkspaceID(folder)
	if requireManage {
		if workspaceID != "" {
			hasPermission, err := hasWorkspaceFilePermission(c.Request.Context(), permissionChecker, organizationID, accountID, workspaceID, workspace_model.WorkspacePermissionFileManage)
			if err != nil {
				response.Fail(c, response.ErrSystemError)
				return nil, false
			}
			if hasPermission {
				return folder, true
			}
		}
		response.Fail(c, response.ErrPermissionDenied)
		return nil, false
	}

	if !fileFolderAllowsSharedView(folder) {
		response.Fail(c, response.ErrPermissionDenied)
		return nil, false
	}

	if workspaceID != "" {
		hasPermission, err := hasWorkspaceFilePermission(c.Request.Context(), permissionChecker, organizationID, accountID, workspaceID, fileViewPermissions()...)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return nil, false
		}
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return nil, false
		}
	}

	if file_model.FileFolderPermissionType(folder.Permission) == file_model.FileFolderPermissionPartialTeam {
		hasPermission, err := hasAnyPartialWorkspaceFilePermission(c.Request.Context(), folderService, permissionChecker, organizationID, accountID, folder.ID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return nil, false
		}
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return nil, false
		}
	}

	return folder, true
}

func requiresFileManagePermission(permissions []workspace_model.WorkspacePermissionCode) bool {
	for _, permission := range permissions {
		if permission == workspace_model.WorkspacePermissionFileManage {
			return true
		}
	}
	return false
}

func getFileFolderWorkspaceID(folder *file_model.FileFolder) string {
	if folder.WorkspaceID == nil {
		return ""
	}
	return *folder.WorkspaceID
}

func fileFolderAllowsSharedView(folder *file_model.FileFolder) bool {
	switch file_model.FileFolderPermissionType(folder.Permission) {
	case file_model.FileFolderPermissionAllTeam, file_model.FileFolderPermissionPartialTeam:
		return true
	default:
		return false
	}
}

func fileViewPermissions() []workspace_model.WorkspacePermissionCode {
	return []workspace_model.WorkspacePermissionCode{
		workspace_model.WorkspacePermissionFileView,
		workspace_model.WorkspacePermissionFileManage,
		workspace_model.WorkspacePermissionFileDownload,
	}
}

func hasAnyPartialWorkspaceFilePermission(ctx context.Context, folderService fileFolderPermissionReader, permissionChecker fileWorkspacePermissionChecker, organizationID, accountID, folderID string) (bool, error) {
	workspaceIDs, err := folderService.GetFolderPermissionTenants(ctx, folderID)
	if err != nil {
		return false, err
	}
	for _, workspaceID := range workspaceIDs {
		hasPermission, err := hasWorkspaceFilePermission(ctx, permissionChecker, organizationID, accountID, workspaceID, fileViewPermissions()...)
		if err != nil {
			return false, err
		}
		if hasPermission {
			return true, nil
		}
	}
	return false, nil
}

func checkWorkspaceFilePermission(c *gin.Context, permissionChecker fileWorkspacePermissionChecker, organizationID, accountID, workspaceID string, permissions ...workspace_model.WorkspacePermissionCode) bool {
	if permissionChecker == nil {
		response.Fail(c, response.ErrSystemError)
		return false
	}

	hasPermission, err := hasWorkspaceFilePermission(c.Request.Context(), permissionChecker, organizationID, accountID, workspaceID, permissions...)
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

func hasWorkspaceFilePermission(ctx context.Context, permissionChecker fileWorkspacePermissionChecker, organizationID, accountID, workspaceID string, permissions ...workspace_model.WorkspacePermissionCode) (bool, error) {
	return permissionChecker.CheckWorkspaceOrganizationAnyPermission(
		ctx,
		organizationID,
		workspaceID,
		accountID,
		permissions...,
	)
}
