package handler

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	shared_visibility "github.com/zgiai/zgi/api/internal/modules/shared/visibility"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/response"
)

func resolveVisibleWorkspaceIDs(
	ctx context.Context,
	orgService interfaces.OrganizationService,
	organizationID string,
	accountID string,
	filterWorkspaceID string,
	permissionCodes ...workspace_model.WorkspacePermissionCode,
) ([]string, error) {
	if orgService == nil {
		return nil, fmt.Errorf("organization service is nil")
	}

	return shared_visibility.ResolveVisibleWorkspaceIDs(
		ctx,
		orgService,
		organizationID,
		accountID,
		filterWorkspaceID,
		permissionCodes...,
	)
}

func respondEmptyFileFolderList(c *gin.Context, page, limit int) {
	response.Success(c, &dto.FileFolderListResponse{
		Data:    []dto.FileFolderResponse{},
		HasMore: false,
		Limit:   limit,
		Total:   0,
		Page:    page,
	})
}

func respondEmptyFileList(c *gin.Context, page, limit int) {
	response.Success(c, &dto.FileListResponse{
		Data:    []dto.UploadFile{},
		HasMore: false,
		Limit:   limit,
		Total:   0,
		Page:    page,
	})
}
