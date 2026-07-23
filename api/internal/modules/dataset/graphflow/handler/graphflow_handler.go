package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	datasetmodel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

type datasetReader interface {
	GetDatasetByID(ctx context.Context, id string) (*datasetmodel.Dataset, error)
}

type graphPermissionChecker interface {
	CheckWorkspaceOrganizationAnyPermission(
		ctx context.Context,
		organizationID string,
		workspaceID string,
		accountID string,
		permissionCodes ...workspacemodel.WorkspacePermissionCode,
	) (bool, error)
}

type GraphFlowHandler struct {
	lifecycle   *graphflow.LifecycleService
	datasets    datasetReader
	permissions graphPermissionChecker
}

func NewGraphFlowHandler(
	lifecycle *graphflow.LifecycleService,
	datasets datasetReader,
	permissions graphPermissionChecker,
) *GraphFlowHandler {
	return &GraphFlowHandler{lifecycle: lifecycle, datasets: datasets, permissions: permissions}
}

func (h *GraphFlowHandler) GetStatus(c *gin.Context) {
	dataset, organizationID, ok := h.authorize(c, workspacemodel.WorkspacePermissionKnowledgeBaseGraphView, workspacemodel.WorkspacePermissionKnowledgeBaseGraphManage)
	if !ok {
		return
	}
	status, err := h.lifecycle.GetStatus(c.Request.Context(), organizationID, uuid.MustParse(dataset.ID))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, status)
}

func (h *GraphFlowHandler) Rebuild(c *gin.Context) {
	dataset, organizationID, ok := h.authorize(c, workspacemodel.WorkspacePermissionKnowledgeBaseGraphManage)
	if !ok {
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		key = "manual-rebuild:" + uuid.NewString()
	}
	run, _, err := h.lifecycle.RebuildDataset(c.Request.Context(), organizationID, uuid.MustParse(dataset.ID), key)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, response.Response{
		Code:    "0",
		Message: "success",
		Data: gin.H{
			"run_id":         run.ID,
			"mode":           run.Mode,
			"status":         run.Status,
			"graph_revision": run.GraphRevision,
		},
	})
}

func (h *GraphFlowHandler) RetryDocument(c *gin.Context) {
	dataset, organizationID, ok := h.authorize(c, workspacemodel.WorkspacePermissionKnowledgeBaseGraphManage)
	if !ok {
		return
	}
	documentID, err := uuid.Parse(c.Param("document_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_document_id", "message": "Document ID is invalid."})
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		key = "document-retry:" + documentID.String() + ":" + uuid.NewString()
	}
	run, _, err := h.lifecycle.RetryDocument(c.Request.Context(), organizationID, uuid.MustParse(dataset.ID), documentID, key)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, response.Response{
		Code:    "0",
		Message: "success",
		Data: gin.H{
			"run_id":              run.ID,
			"document_id":         run.DocumentID,
			"ref_id":              run.SourceRefID,
			"sync_run_id":         run.SyncRunID,
			"status":              run.Status,
			"asset_generation_no": run.AssetGenerationNo,
		},
	})
}

func (h *GraphFlowHandler) authorize(
	c *gin.Context,
	permissionCodes ...workspacemodel.WorkspacePermissionCode,
) (*datasetmodel.Dataset, uuid.UUID, bool) {
	datasetID, err := uuid.Parse(c.Param("dataset_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_dataset_id", "message": "Dataset ID is invalid."})
		return nil, uuid.Nil, false
	}
	accountID := c.GetString("account_id")
	organizationValue := util.GetOrganizationIDCompat(c)
	organizationID, err := uuid.Parse(organizationValue)
	if err != nil || accountID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "unauthorized", "message": "Authentication is required."})
		return nil, uuid.Nil, false
	}
	dataset, err := h.datasets.GetDatasetByID(c.Request.Context(), datasetID.String())
	if err != nil || dataset == nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || dataset == nil {
			c.JSON(http.StatusNotFound, gin.H{"code": "dataset_not_found", "message": "Dataset was not found."})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "dataset_read_failed", "message": "Failed to read dataset."})
		}
		return nil, uuid.Nil, false
	}
	if dataset.OrganizationID != organizationID.String() {
		c.JSON(http.StatusForbidden, gin.H{"code": "graph_permission_denied", "message": "Knowledge graph access is denied."})
		return nil, uuid.Nil, false
	}
	allowed, err := h.permissions.CheckWorkspaceOrganizationAnyPermission(
		c.Request.Context(), organizationID.String(), dataset.WorkspaceID, accountID, permissionCodes...,
	)
	if err != nil || !allowed {
		c.JSON(http.StatusForbidden, gin.H{"code": "graph_permission_denied", "message": "Knowledge graph access is denied."})
		return nil, uuid.Nil, false
	}
	return dataset, organizationID, true
}

func (h *GraphFlowHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, graphflow.ErrStaleDocumentSnapshot):
		c.JSON(http.StatusConflict, gin.H{"code": "stale_document_snapshot", "message": "The document snapshot is no longer current."})
	case errors.Is(err, graphflow.ErrGraphFlowDisabled):
		c.JSON(http.StatusConflict, gin.H{"code": "graph_not_enabled", "message": "Knowledge graph is not enabled."})
	case errors.Is(err, graphflow.ErrGraphFlowTenantScopeMismatch):
		c.JSON(http.StatusForbidden, gin.H{"code": "graph_permission_denied", "message": "Knowledge graph access is denied."})
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"code": "graph_resource_not_found", "message": "Knowledge graph resource was not found."})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"code": "graph_operation_failed", "message": "Knowledge graph operation failed."})
	}
}
