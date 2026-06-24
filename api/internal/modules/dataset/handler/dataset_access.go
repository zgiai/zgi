package handler

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"

	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	shared_service "github.com/zgiai/zgi/api/internal/modules/shared/service"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

type datasetAccessDatasetReader interface {
	GetDatasetByID(ctx context.Context, id string) (*dataset_model.Dataset, error)
}

type datasetAccessDocumentReader interface {
	GetDocumentByID(ctx context.Context, documentID string) (*dataset_model.Document, error)
}

type datasetAccessSegmentReader interface {
	GetChunkByID(ctx context.Context, id string) (*dataset_model.DocumentSegment, error)
	GetChildChunkByID(ctx context.Context, childChunkID string) (*dataset_model.ChildChunk, error)
}

type datasetAccessFolderReader interface {
	GetFolderByID(ctx context.Context, folderID string) (*dataset_model.DatasetFolder, error)
}

type datasetAccessAuthorizer interface {
	RequireWorkspacePermission(ctx context.Context, req interfaces.WorkspaceScopeRequest) (*interfaces.WorkspaceScope, error)
}

func authorizeDatasetViewAccess(c *gin.Context, datasetService datasetAccessDatasetReader, authService datasetAccessAuthorizer, datasetID string) (*dataset_model.Dataset, bool) {
	return authorizeDatasetAccess(
		c,
		datasetService,
		authService,
		datasetID,
		workspace_model.WorkspacePermissionKnowledgeBaseView,
		workspace_model.WorkspacePermissionKnowledgeBaseManage,
		workspace_model.WorkspacePermissionKnowledgeBaseFolderManage,
	)
}

func authorizeDatasetManageAccess(c *gin.Context, datasetService datasetAccessDatasetReader, authService datasetAccessAuthorizer, datasetID string) (*dataset_model.Dataset, bool) {
	return authorizeDatasetAccess(
		c,
		datasetService,
		authService,
		datasetID,
		workspace_model.WorkspacePermissionKnowledgeBaseManage,
	)
}

func authorizeDatasetAccess(c *gin.Context, datasetService datasetAccessDatasetReader, authService datasetAccessAuthorizer, datasetID string, permissions ...workspace_model.WorkspacePermissionCode) (*dataset_model.Dataset, bool) {
	accountID := c.GetString("account_id")
	organizationID := util.GetOrganizationIDCompat(c)
	if accountID == "" || organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return nil, false
	}
	if datasetService == nil || authService == nil {
		response.Fail(c, response.ErrSystemError)
		return nil, false
	}

	dataset, err := datasetService.GetDatasetByID(c.Request.Context(), datasetID)
	if err != nil || dataset == nil {
		response.Fail(c, response.ErrDatasetNotFound)
		return nil, false
	}
	if dataset.OrganizationID != organizationID || dataset.WorkspaceID == "" {
		response.Fail(c, response.ErrDatasetPermissionDenied)
		return nil, false
	}

	if _, ok := authorizeDatasetWorkspacePermission(c, authService, dataset.WorkspaceID, permissions...); !ok {
		return nil, false
	}

	return dataset, true
}

func authorizeDatasetWorkspaceViewAccess(c *gin.Context, authService datasetAccessAuthorizer, workspaceID string) bool {
	_, ok := authorizeDatasetWorkspacePermission(
		c,
		authService,
		workspaceID,
		workspace_model.WorkspacePermissionKnowledgeBaseView,
		workspace_model.WorkspacePermissionKnowledgeBaseManage,
		workspace_model.WorkspacePermissionKnowledgeBaseFolderManage,
	)
	return ok
}

func authorizeDatasetWorkspaceFolderManageAccess(c *gin.Context, authService datasetAccessAuthorizer, workspaceID string) bool {
	_, ok := authorizeDatasetWorkspacePermission(
		c,
		authService,
		workspaceID,
		workspace_model.WorkspacePermissionKnowledgeBaseFolderManage,
	)
	return ok
}

func authorizeDatasetWorkspacePermission(c *gin.Context, authService datasetAccessAuthorizer, workspaceID string, permissions ...workspace_model.WorkspacePermissionCode) (*interfaces.WorkspaceScope, bool) {
	accountID := c.GetString("account_id")
	organizationID := util.GetOrganizationIDCompat(c)
	if accountID == "" || organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return nil, false
	}
	if workspaceID == "" {
		response.Fail(c, response.ErrDatasetPermissionDenied)
		return nil, false
	}
	if authService == nil {
		response.Fail(c, response.ErrSystemError)
		return nil, false
	}

	scope, err := authService.RequireWorkspacePermission(c.Request.Context(), interfaces.WorkspaceScopeRequest{
		OrganizationID:  organizationID,
		WorkspaceID:     workspaceID,
		AccountID:       accountID,
		PermissionCodes: permissions,
	})
	if err != nil {
		if errors.Is(err, shared_service.ErrAuthorizationDenied) {
			response.Fail(c, response.ErrDatasetPermissionDenied)
			return nil, false
		}
		response.Fail(c, response.ErrSystemError)
		return nil, false
	}

	return scope, true
}

func authorizeDatasetFolderViewAccess(c *gin.Context, folderService datasetAccessFolderReader, authService datasetAccessAuthorizer, folderID string) (*dataset_model.DatasetFolder, bool) {
	return authorizeDatasetFolderAccess(
		c,
		folderService,
		authService,
		folderID,
		false,
		workspace_model.WorkspacePermissionKnowledgeBaseView,
		workspace_model.WorkspacePermissionKnowledgeBaseManage,
		workspace_model.WorkspacePermissionKnowledgeBaseFolderManage,
	)
}

func authorizeDatasetFolderManageAccess(c *gin.Context, folderService datasetAccessFolderReader, authService datasetAccessAuthorizer, folderID string) (*dataset_model.DatasetFolder, bool) {
	return authorizeDatasetFolderAccess(
		c,
		folderService,
		authService,
		folderID,
		true,
		workspace_model.WorkspacePermissionKnowledgeBaseFolderManage,
	)
}

func authorizeDatasetFolderAccess(c *gin.Context, folderService datasetAccessFolderReader, authService datasetAccessAuthorizer, folderID string, requireManage bool, permissions ...workspace_model.WorkspacePermissionCode) (*dataset_model.DatasetFolder, bool) {
	accountID := c.GetString("account_id")
	organizationID := util.GetOrganizationIDCompat(c)
	if accountID == "" || organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return nil, false
	}
	if folderService == nil {
		response.Fail(c, response.ErrSystemError)
		return nil, false
	}

	folder, err := folderService.GetFolderByID(c.Request.Context(), folderID)
	if err != nil || folder == nil {
		response.Fail(c, response.ErrDatasetNotFound)
		return nil, false
	}
	if folder.OrganizationID != organizationID || folder.WorkspaceID == "" {
		response.Fail(c, response.ErrDatasetPermissionDenied)
		return nil, false
	}

	_, ok := authorizeDatasetWorkspacePermission(c, authService, folder.WorkspaceID, permissions...)
	if !ok {
		return nil, false
	}
	if requireManage {
		return folder, true
	}

	return folder, true
}

func authorizeDatasetDocumentViewAccess(c *gin.Context, datasetService datasetAccessDatasetReader, documentService datasetAccessDocumentReader, authService datasetAccessAuthorizer, datasetID, documentID string) (*dataset_model.Dataset, *dataset_model.Document, bool) {
	return authorizeDatasetDocumentAccess(c, datasetService, documentService, authService, datasetID, documentID,
		workspace_model.WorkspacePermissionKnowledgeBaseView,
		workspace_model.WorkspacePermissionKnowledgeBaseManage,
		workspace_model.WorkspacePermissionKnowledgeBaseFolderManage,
	)
}

func authorizeDatasetDocumentManageAccess(c *gin.Context, datasetService datasetAccessDatasetReader, documentService datasetAccessDocumentReader, authService datasetAccessAuthorizer, datasetID, documentID string) (*dataset_model.Dataset, *dataset_model.Document, bool) {
	return authorizeDatasetDocumentAccess(c, datasetService, documentService, authService, datasetID, documentID,
		workspace_model.WorkspacePermissionKnowledgeBaseManage,
	)
}

func authorizeDatasetDocumentAccess(c *gin.Context, datasetService datasetAccessDatasetReader, documentService datasetAccessDocumentReader, authService datasetAccessAuthorizer, datasetID, documentID string, permissions ...workspace_model.WorkspacePermissionCode) (*dataset_model.Dataset, *dataset_model.Document, bool) {
	dataset, ok := authorizeDatasetAccess(c, datasetService, authService, datasetID, permissions...)
	if !ok {
		return nil, nil, false
	}
	if documentService == nil {
		response.Fail(c, response.ErrSystemError)
		return nil, nil, false
	}

	document, err := documentService.GetDocumentByID(c.Request.Context(), documentID)
	if err != nil || document == nil {
		response.Fail(c, response.ErrDocumentNotFound)
		return nil, nil, false
	}
	if document.OrganizationID != dataset.OrganizationID || document.DatasetID != dataset.ID {
		response.Fail(c, response.ErrDocumentNotFound)
		return nil, nil, false
	}

	return dataset, document, true
}

func authorizeDatasetSegmentViewAccess(c *gin.Context, datasetService datasetAccessDatasetReader, documentService datasetAccessDocumentReader, segmentService datasetAccessSegmentReader, authService datasetAccessAuthorizer, datasetID, documentID, segmentID string) (*dataset_model.Dataset, *dataset_model.Document, *dataset_model.DocumentSegment, bool) {
	return authorizeDatasetSegmentAccess(c, datasetService, documentService, segmentService, authService, datasetID, documentID, segmentID,
		workspace_model.WorkspacePermissionKnowledgeBaseView,
		workspace_model.WorkspacePermissionKnowledgeBaseManage,
		workspace_model.WorkspacePermissionKnowledgeBaseFolderManage,
	)
}

func authorizeDatasetSegmentManageAccess(c *gin.Context, datasetService datasetAccessDatasetReader, documentService datasetAccessDocumentReader, segmentService datasetAccessSegmentReader, authService datasetAccessAuthorizer, datasetID, documentID, segmentID string) (*dataset_model.Dataset, *dataset_model.Document, *dataset_model.DocumentSegment, bool) {
	return authorizeDatasetSegmentAccess(c, datasetService, documentService, segmentService, authService, datasetID, documentID, segmentID,
		workspace_model.WorkspacePermissionKnowledgeBaseManage,
	)
}

func authorizeDatasetSegmentAccess(c *gin.Context, datasetService datasetAccessDatasetReader, documentService datasetAccessDocumentReader, segmentService datasetAccessSegmentReader, authService datasetAccessAuthorizer, datasetID, documentID, segmentID string, permissions ...workspace_model.WorkspacePermissionCode) (*dataset_model.Dataset, *dataset_model.Document, *dataset_model.DocumentSegment, bool) {
	dataset, document, ok := authorizeDatasetDocumentAccess(c, datasetService, documentService, authService, datasetID, documentID, permissions...)
	if !ok {
		return nil, nil, nil, false
	}
	if segmentService == nil {
		response.Fail(c, response.ErrSystemError)
		return nil, nil, nil, false
	}

	segment, err := segmentService.GetChunkByID(c.Request.Context(), segmentID)
	if err != nil || segment == nil {
		response.Fail(c, response.ErrSegmentNotFound)
		return nil, nil, nil, false
	}
	if segment.OrganizationID != dataset.OrganizationID || segment.DatasetID != dataset.ID || segment.DocumentID != document.ID {
		response.Fail(c, response.ErrSegmentNotFound)
		return nil, nil, nil, false
	}

	return dataset, document, segment, true
}

func authorizeDatasetChildChunkAccess(c *gin.Context, datasetService datasetAccessDatasetReader, documentService datasetAccessDocumentReader, segmentService datasetAccessSegmentReader, authService datasetAccessAuthorizer, datasetID, documentID, segmentID, childChunkID string, permissions ...workspace_model.WorkspacePermissionCode) (*dataset_model.ChildChunk, bool) {
	_, _, segment, ok := authorizeDatasetSegmentAccess(c, datasetService, documentService, segmentService, authService, datasetID, documentID, segmentID, permissions...)
	if !ok {
		return nil, false
	}

	childChunk, err := segmentService.GetChildChunkByID(c.Request.Context(), childChunkID)
	if err != nil || childChunk == nil {
		response.Fail(c, response.ErrChildChunkNotFound)
		return nil, false
	}
	if childChunk.OrganizationID != segment.OrganizationID || childChunk.DatasetID != segment.DatasetID || childChunk.DocumentID != segment.DocumentID || childChunk.SegmentID != segment.ID {
		response.Fail(c, response.ErrChildChunkNotFound)
		return nil, false
	}

	return childChunk, true
}
