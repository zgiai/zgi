package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

func authorizeFileDownloadAccess(c *gin.Context, fileService interfaces.FileService, enterpriseService interfaces.OrganizationService, fileID string) (*dto.UploadFile, bool) {
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
	if err != nil {
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
		return uploadFile, true
	}
	if !checkWorkspaceFileDownloadPermission(c, enterpriseService, organizationID, accountID, workspaceID) {
		return nil, false
	}

	return uploadFile, true
}

func checkWorkspaceFileDownloadPermission(c *gin.Context, enterpriseService interfaces.OrganizationService, organizationID, accountID, workspaceID string) bool {
	if enterpriseService == nil {
		response.Fail(c, response.ErrSystemError)
		return false
	}

	hasPermission, err := enterpriseService.CheckWorkspacePermission(
		c.Request.Context(),
		organizationID,
		workspaceID,
		accountID,
		workspace_model.WorkspacePermissionFileDownload,
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
