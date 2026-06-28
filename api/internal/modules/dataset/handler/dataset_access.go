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

func knowledgeBaseViewPermissionCodes() []workspace_model.WorkspacePermissionCode {
	return []workspace_model.WorkspacePermissionCode{
		workspace_model.WorkspacePermissionKnowledgeBaseCreate,
		workspace_model.WorkspacePermissionKnowledgeBaseFolderView,
		workspace_model.WorkspacePermissionKnowledgeBaseDocumentView,
		workspace_model.WorkspacePermissionKnowledgeBaseSegmentView,
		workspace_model.WorkspacePermissionKnowledgeBaseGraphView,
		workspace_model.WorkspacePermissionKnowledgeBaseRetrievalTest,
		workspace_model.WorkspacePermissionKnowledgeBaseUpdate,
		workspace_model.WorkspacePermissionKnowledgeBaseDelete,
		workspace_model.WorkspacePermissionKnowledgeBaseMove,
		workspace_model.WorkspacePermissionKnowledgeBaseDocumentCreate,
		workspace_model.WorkspacePermissionKnowledgeBaseDocumentUpdate,
		workspace_model.WorkspacePermissionKnowledgeBaseDocumentDelete,
		workspace_model.WorkspacePermissionKnowledgeBaseSegmentUpdate,
		workspace_model.WorkspacePermissionKnowledgeBaseSegmentDelete,
		workspace_model.WorkspacePermissionKnowledgeBaseIndexManage,
		workspace_model.WorkspacePermissionKnowledgeBaseGraphManage,
		workspace_model.WorkspacePermissionKnowledgeBaseFolderManage,
	}
}

func knowledgeBaseDocumentViewPermissionCodes() []workspace_model.WorkspacePermissionCode {
	return []workspace_model.WorkspacePermissionCode{
		workspace_model.WorkspacePermissionKnowledgeBaseDocumentView,
		workspace_model.WorkspacePermissionKnowledgeBaseDocumentUpdate,
		workspace_model.WorkspacePermissionKnowledgeBaseDocumentDelete,
		workspace_model.WorkspacePermissionKnowledgeBaseIndexManage,
	}
}

func knowledgeBaseSegmentViewPermissionCodes() []workspace_model.WorkspacePermissionCode {
	return []workspace_model.WorkspacePermissionCode{
		workspace_model.WorkspacePermissionKnowledgeBaseSegmentView,
		workspace_model.WorkspacePermissionKnowledgeBaseSegmentUpdate,
		workspace_model.WorkspacePermissionKnowledgeBaseSegmentDelete,
	}
}

func knowledgeBaseFolderViewPermissionCodes() []workspace_model.WorkspacePermissionCode {
	return []workspace_model.WorkspacePermissionCode{
		workspace_model.WorkspacePermissionKnowledgeBaseFolderView,
		workspace_model.WorkspacePermissionKnowledgeBaseFolderManage,
	}
}

func authorizeDatasetViewAccess(c *gin.Context, datasetService datasetAccessDatasetReader, authService datasetAccessAuthorizer, datasetID string) (*dataset_model.Dataset, bool) {
	return authorizeDatasetAccess(
		c,
		datasetService,
		authService,
		datasetID,
		knowledgeBaseViewPermissionCodes()...,
	)
}

func authorizeDatasetUpdateAccess(c *gin.Context, datasetService datasetAccessDatasetReader, authService datasetAccessAuthorizer, datasetID string) (*dataset_model.Dataset, bool) {
	return authorizeDatasetAccess(
		c,
		datasetService,
		authService,
		datasetID,
		workspace_model.WorkspacePermissionKnowledgeBaseUpdate,
	)
}

func authorizeDatasetDeleteAccess(c *gin.Context, datasetService datasetAccessDatasetReader, authService datasetAccessAuthorizer, datasetID string) (*dataset_model.Dataset, bool) {
	return authorizeDatasetAccess(
		c,
		datasetService,
		authService,
		datasetID,
		workspace_model.WorkspacePermissionKnowledgeBaseDelete,
	)
}

func authorizeDatasetSegmentDeleteAccessByDataset(c *gin.Context, datasetService datasetAccessDatasetReader, authService datasetAccessAuthorizer, datasetID string) (*dataset_model.Dataset, bool) {
	return authorizeDatasetAccess(
		c,
		datasetService,
		authService,
		datasetID,
		workspace_model.WorkspacePermissionKnowledgeBaseSegmentDelete,
	)
}

func authorizeDatasetMoveAccess(c *gin.Context, datasetService datasetAccessDatasetReader, authService datasetAccessAuthorizer, datasetID string) (*dataset_model.Dataset, bool) {
	return authorizeDatasetAccess(
		c,
		datasetService,
		authService,
		datasetID,
		workspace_model.WorkspacePermissionKnowledgeBaseMove,
	)
}

func authorizeDatasetDocumentCreateAccess(c *gin.Context, datasetService datasetAccessDatasetReader, authService datasetAccessAuthorizer, datasetID string) (*dataset_model.Dataset, bool) {
	return authorizeDatasetAccess(
		c,
		datasetService,
		authService,
		datasetID,
		workspace_model.WorkspacePermissionKnowledgeBaseDocumentCreate,
	)
}

func authorizeDatasetDocumentBatchUpdateAccess(c *gin.Context, datasetService datasetAccessDatasetReader, authService datasetAccessAuthorizer, datasetID string) (*dataset_model.Dataset, bool) {
	return authorizeDatasetAccess(
		c,
		datasetService,
		authService,
		datasetID,
		workspace_model.WorkspacePermissionKnowledgeBaseDocumentUpdate,
	)
}

func authorizeDatasetIndexManageAccess(c *gin.Context, datasetService datasetAccessDatasetReader, authService datasetAccessAuthorizer, datasetID string) (*dataset_model.Dataset, bool) {
	return authorizeDatasetAccess(
		c,
		datasetService,
		authService,
		datasetID,
		workspace_model.WorkspacePermissionKnowledgeBaseIndexManage,
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
		knowledgeBaseViewPermissionCodes()...,
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
		knowledgeBaseFolderViewPermissionCodes()...,
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
		knowledgeBaseDocumentViewPermissionCodes()...,
	)
}

func authorizeDatasetDocumentSegmentUpdateAccess(c *gin.Context, datasetService datasetAccessDatasetReader, documentService datasetAccessDocumentReader, authService datasetAccessAuthorizer, datasetID, documentID string) (*dataset_model.Dataset, *dataset_model.Document, bool) {
	return authorizeDatasetDocumentAccess(c, datasetService, documentService, authService, datasetID, documentID,
		workspace_model.WorkspacePermissionKnowledgeBaseSegmentUpdate,
	)
}

func authorizeDatasetDocumentSegmentDeleteAccess(c *gin.Context, datasetService datasetAccessDatasetReader, documentService datasetAccessDocumentReader, authService datasetAccessAuthorizer, datasetID, documentID string) (*dataset_model.Dataset, *dataset_model.Document, bool) {
	return authorizeDatasetDocumentAccess(c, datasetService, documentService, authService, datasetID, documentID,
		workspace_model.WorkspacePermissionKnowledgeBaseSegmentDelete,
	)
}

func authorizeDatasetDocumentUpdateAccess(c *gin.Context, datasetService datasetAccessDatasetReader, documentService datasetAccessDocumentReader, authService datasetAccessAuthorizer, datasetID, documentID string) (*dataset_model.Dataset, *dataset_model.Document, bool) {
	return authorizeDatasetDocumentAccess(c, datasetService, documentService, authService, datasetID, documentID,
		workspace_model.WorkspacePermissionKnowledgeBaseDocumentUpdate,
	)
}

func authorizeDatasetDocumentDeleteAccess(c *gin.Context, datasetService datasetAccessDatasetReader, documentService datasetAccessDocumentReader, authService datasetAccessAuthorizer, datasetID, documentID string) (*dataset_model.Dataset, *dataset_model.Document, bool) {
	return authorizeDatasetDocumentAccess(c, datasetService, documentService, authService, datasetID, documentID,
		workspace_model.WorkspacePermissionKnowledgeBaseDocumentDelete,
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
		knowledgeBaseSegmentViewPermissionCodes()...,
	)
}

func authorizeDatasetSegmentUpdateAccess(c *gin.Context, datasetService datasetAccessDatasetReader, documentService datasetAccessDocumentReader, segmentService datasetAccessSegmentReader, authService datasetAccessAuthorizer, datasetID, documentID, segmentID string) (*dataset_model.Dataset, *dataset_model.Document, *dataset_model.DocumentSegment, bool) {
	return authorizeDatasetSegmentAccess(c, datasetService, documentService, segmentService, authService, datasetID, documentID, segmentID,
		workspace_model.WorkspacePermissionKnowledgeBaseSegmentUpdate,
	)
}

func authorizeDatasetSegmentDeleteAccess(c *gin.Context, datasetService datasetAccessDatasetReader, documentService datasetAccessDocumentReader, segmentService datasetAccessSegmentReader, authService datasetAccessAuthorizer, datasetID, documentID, segmentID string) (*dataset_model.Dataset, *dataset_model.Document, *dataset_model.DocumentSegment, bool) {
	return authorizeDatasetSegmentAccess(c, datasetService, documentService, segmentService, authService, datasetID, documentID, segmentID,
		workspace_model.WorkspacePermissionKnowledgeBaseSegmentDelete,
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
