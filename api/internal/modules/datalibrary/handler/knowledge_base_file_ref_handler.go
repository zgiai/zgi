package handler

import (
	"context"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	datalibModel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	datalibService "github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
	datasetModel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspaceModel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
)

type KnowledgeBaseFileRefHandler struct {
	service        datalibService.KnowledgeBaseFileRefService
	dispatcher     datasetFileRefSyncEnqueuer
	accountService interfaces.AccountService
	documents      datasetFileRefDocumentManager
	datasets       datasetFileRefDatasetReader
	organization   datasetFileRefPermissionChecker
	processing     knowledgeBaseFileRefProcessingRequestService
}

const fileCandidateEmbeddingTaskType = "file_candidate_embedding"

type datasetFileRefDocumentManager interface {
	DeleteDocuments(ctx context.Context, datasetID string, documentIDs []string) error
}

type datasetFileRefDatasetReader interface {
	GetDatasetByID(ctx context.Context, datasetID string) (*datasetModel.Dataset, error)
}

type datasetFileRefPermissionChecker interface {
	CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode workspaceModel.WorkspacePermissionCode) (bool, error)
}

type datasetFileRefSyncEnqueuer interface {
	EnqueueDatasetRefSync(ctx context.Context, refID uuid.UUID, assetID uuid.UUID, datasetID string, generationNo int64, syncRunID uuid.UUID) error
	EnqueueFileCandidateEmbedding(ctx context.Context, req datalibService.KnowledgeBaseFileCandidateEmbeddingRequest) error
}

type knowledgeBaseFileRefProcessingRequestService interface {
	CreatePlannedRequest(ctx context.Context, req datalibService.ProcessingRequest) (*datalibService.ProcessingRequestView, error)
	GetRequest(ctx context.Context, organizationID string, id uuid.UUID) (*datalibService.ProcessingRequestView, error)
	QueueRequest(ctx context.Context, organizationID string, id uuid.UUID) (*datalibService.ProcessingRequestView, error)
	FailRequest(ctx context.Context, organizationID string, id uuid.UUID, errorCode string, errorMessage string, metadata map[string]any) (*datalibService.ProcessingRequestView, error)
}

func NewKnowledgeBaseFileRefHandler(service datalibService.KnowledgeBaseFileRefService, dispatcher datasetFileRefSyncEnqueuer, accountService interfaces.AccountService, documents datasetFileRefDocumentManager, datasets datasetFileRefDatasetReader, organization datasetFileRefPermissionChecker, processing ...knowledgeBaseFileRefProcessingRequestService) *KnowledgeBaseFileRefHandler {
	var processingService knowledgeBaseFileRefProcessingRequestService
	if len(processing) > 0 {
		processingService = processing[0]
	}
	return &KnowledgeBaseFileRefHandler{
		service:        service,
		dispatcher:     dispatcher,
		accountService: accountService,
		documents:      documents,
		datasets:       datasets,
		organization:   organization,
		processing:     processingService,
	}
}

func (h *KnowledgeBaseFileRefHandler) RegisterDatasetRoutes(router *gin.RouterGroup) {
	auth := router.Group("", middleware.JWTWithOrganizationAndService(h.accountService))
	group := auth.Group("/datasets/:dataset_id")
	group.GET("/file-candidates", h.ListFileCandidates)
	group.POST("/file-candidates/:asset_id/embeddings", h.GenerateFileCandidateEmbeddings)
	group.GET("/file-candidates/:asset_id/embedding-tasks/:request_id", h.GetFileCandidateEmbeddingTask)
	group.GET("/file-refs", h.ListFileRefs)
	group.POST("/file-refs", h.CreateFileRefs)
	group.DELETE("/file-refs/:ref_id", h.RemoveFileRef)
	group.POST("/file-refs/:ref_id/retry", h.RetryFileRef)
	group.POST("/file-refs/:ref_id/sync/retry", h.RetryFileRef)
}

func (h *KnowledgeBaseFileRefHandler) ListFileCandidates(c *gin.Context) {
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	if !h.requireKnowledgeBaseManage(c, organizationID, c.Param("dataset_id")) {
		return
	}
	limit, offset := parseLimitOffset(c, 20, 100)
	result, err := h.service.ListCandidates(c.Request.Context(), datalibService.KnowledgeBaseFileCandidateRequest{
		OrganizationID: organizationID,
		WorkspaceID:    optionalString(util.GetWorkspaceID(c)),
		DatasetID:      c.Param("dataset_id"),
		Filter:         c.DefaultQuery("filter", datalibService.FileCandidateFilterAddable),
		Keyword:        c.Query("keyword"),
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *KnowledgeBaseFileRefHandler) GenerateFileCandidateEmbeddings(c *gin.Context) {
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	if !h.requireKnowledgeBaseManage(c, organizationID, c.Param("dataset_id")) {
		return
	}
	assetID, err := uuid.Parse(c.Param("asset_id"))
	if err != nil || assetID == uuid.Nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	if h.dispatcher == nil {
		response.FailWithMessage(c, response.ErrSystemError, "file candidate embedding dispatcher is not configured")
		return
	}
	if h.processing == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing request service is not configured")
		return
	}
	processingRequest, err := h.processing.CreatePlannedRequest(c.Request.Context(), datalibService.ProcessingRequest{
		OrganizationID: organizationID,
		WorkspaceID:    optionalString(util.GetWorkspaceID(c)),
		AssetID:        assetID,
		TargetLevel:    datalibModel.DocumentProcessingLevelVectorize,
		RequestedBy:    util.GetAccountID(c),
		RequestMetadata: map[string]any{
			"task_type":  fileCandidateEmbeddingTaskType,
			"dataset_id": c.Param("dataset_id"),
		},
	})
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	queuedRequest, err := h.processing.QueueRequest(c.Request.Context(), organizationID, processingRequest.ID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	embeddingReq := datalibService.KnowledgeBaseFileCandidateEmbeddingRequest{
		OrganizationID:      organizationID,
		WorkspaceID:         optionalString(util.GetWorkspaceID(c)),
		DatasetID:           c.Param("dataset_id"),
		AssetID:             assetID,
		RequestedBy:         util.GetAccountID(c),
		ProcessingRequestID: queuedRequest.ID,
	}
	if err := h.dispatcher.EnqueueFileCandidateEmbedding(c.Request.Context(), embeddingReq); err != nil {
		_, _ = h.processing.FailRequest(c.Request.Context(), organizationID, queuedRequest.ID, "enqueue_failed", err.Error(), map[string]any{
			"task_type":  fileCandidateEmbeddingTaskType,
			"dataset_id": c.Param("dataset_id"),
			"asset_id":   assetID.String(),
		})
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, datalibService.KnowledgeBaseFileCandidateEmbeddingResult{
		AssetID:           assetID,
		Accepted:          true,
		ProcessingRequest: queuedRequest,
	})
}

func (h *KnowledgeBaseFileRefHandler) GetFileCandidateEmbeddingTask(c *gin.Context) {
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	if h.processing == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing request service is not configured")
		return
	}
	assetID, err := uuid.Parse(c.Param("asset_id"))
	if err != nil || assetID == uuid.Nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	requestID, err := uuid.Parse(c.Param("request_id"))
	if err != nil || requestID == uuid.Nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	item, err := h.processing.GetRequest(c.Request.Context(), organizationID, requestID)
	if err != nil {
		if errors.Is(err, datalibService.ErrProcessingRequestNotFound) {
			response.Fail(c, response.ErrNotFound)
			return
		}
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if item == nil || item.AssetID != assetID || item.TargetLevel != datalibModel.DocumentProcessingLevelVectorize {
		response.Fail(c, response.ErrNotFound)
		return
	}
	response.Success(c, item)
}

func (h *KnowledgeBaseFileRefHandler) ListFileRefs(c *gin.Context) {
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	if !h.requireKnowledgeBaseManage(c, organizationID, c.Param("dataset_id")) {
		return
	}
	limit, offset := parseLimitOffset(c, 20, 100)
	result, err := h.service.ListRefs(c.Request.Context(), datalibService.KnowledgeBaseFileRefListRequest{
		OrganizationID: organizationID,
		WorkspaceID:    optionalString(util.GetWorkspaceID(c)),
		DatasetID:      c.Param("dataset_id"),
		SyncStatus:     c.Query("sync_status"),
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, result)
}

type createFileRefsRequest struct {
	AssetIDs []string `json:"asset_ids"`
	FileIDs  []string `json:"file_ids"`
}

func (h *KnowledgeBaseFileRefHandler) CreateFileRefs(c *gin.Context) {
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	if !h.requireKnowledgeBaseManage(c, organizationID, c.Param("dataset_id")) {
		return
	}
	var req createFileRefsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	assetIDs, ok := parseAssetIDs(req.AssetIDs)
	if !ok {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	result, err := h.service.CreateRefs(c.Request.Context(), datalibService.KnowledgeBaseFileRefCreateRequest{
		OrganizationID: organizationID,
		WorkspaceID:    optionalString(util.GetWorkspaceID(c)),
		DatasetID:      c.Param("dataset_id"),
		AssetIDs:       assetIDs,
		CreatedBy:      util.GetAccountID(c),
	})
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	for _, item := range result.Items {
		if !item.Success || item.Ref == nil || item.SyncRunID == nil {
			continue
		}
		if err := h.enqueueDatasetRefSync(c.Request.Context(), organizationID, optionalString(util.GetWorkspaceID(c)), item.Ref.ID, item.AssetID, item.Ref.DatasetID, item.GenerationNo, *item.SyncRunID); err != nil {
			response.FailWithMessage(c, response.ErrSystemError, err.Error())
			return
		}
	}
	response.Success(c, result)
}

func (h *KnowledgeBaseFileRefHandler) RetryFileRef(c *gin.Context) {
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	if !h.requireKnowledgeBaseManage(c, organizationID, c.Param("dataset_id")) {
		return
	}
	refID, err := uuid.Parse(c.Param("ref_id"))
	if err != nil || refID == uuid.Nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	result, err := h.service.RetryRef(c.Request.Context(), datalibService.KnowledgeBaseFileRefRetryRequest{
		OrganizationID: organizationID,
		WorkspaceID:    optionalString(util.GetWorkspaceID(c)),
		DatasetID:      c.Param("dataset_id"),
		RefID:          refID,
	})
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if result.Success && result.Ref != nil && result.SyncRunID != nil {
		if err := h.enqueueDatasetRefSync(c.Request.Context(), organizationID, optionalString(util.GetWorkspaceID(c)), result.Ref.ID, result.AssetID, result.Ref.DatasetID, result.GenerationNo, *result.SyncRunID); err != nil {
			response.FailWithMessage(c, response.ErrSystemError, err.Error())
			return
		}
	}
	response.Success(c, result)
}

func (h *KnowledgeBaseFileRefHandler) RemoveFileRef(c *gin.Context) {
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	if !h.requireKnowledgeBaseManage(c, organizationID, c.Param("dataset_id")) {
		return
	}
	refID, err := uuid.Parse(c.Param("ref_id"))
	if err != nil || refID == uuid.Nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	req := datalibService.KnowledgeBaseFileRefGetRequest{
		OrganizationID: organizationID,
		WorkspaceID:    optionalString(util.GetWorkspaceID(c)),
		DatasetID:      c.Param("dataset_id"),
		RefID:          refID,
	}
	ref, err := h.service.GetRef(c.Request.Context(), req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if h.documents != nil && ref != nil && ref.DatasetDocumentID != nil {
		if err := h.documents.DeleteDocuments(c.Request.Context(), ref.DatasetID, []string{ref.DatasetDocumentID.String()}); err != nil {
			response.FailWithMessage(c, response.ErrSystemError, err.Error())
			return
		}
	}
	removed, err := h.service.RemoveRef(c.Request.Context(), req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, removed)
}

func (h *KnowledgeBaseFileRefHandler) requireKnowledgeBaseManage(c *gin.Context, organizationID string, datasetID string) bool {
	accountID := util.GetAccountID(c)
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return false
	}
	if h.datasets == nil || h.organization == nil {
		response.Fail(c, response.ErrSystemError)
		return false
	}
	dataset, err := h.datasets.GetDatasetByID(c.Request.Context(), datasetID)
	if err != nil || dataset == nil {
		response.Fail(c, response.ErrDatasetNotFound)
		return false
	}
	hasPermission, err := h.organization.CheckWorkspacePermission(
		c.Request.Context(),
		organizationID,
		dataset.WorkspaceID,
		accountID,
		workspaceModel.WorkspacePermissionKnowledgeBaseManage,
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

func (h *KnowledgeBaseFileRefHandler) enqueueDatasetRefSync(ctx context.Context, organizationID string, workspaceID *string, refID uuid.UUID, assetID uuid.UUID, datasetID string, generationNo int64, syncRunID uuid.UUID) error {
	if h.dispatcher == nil {
		return nil
	}
	if err := h.dispatcher.EnqueueDatasetRefSync(ctx, refID, assetID, datasetID, generationNo, syncRunID); err != nil {
		_, _ = h.service.MarkRefSyncFailed(ctx, datalibService.KnowledgeBaseFileRefSyncFailureRequest{
			OrganizationID: organizationID,
			WorkspaceID:    workspaceID,
			DatasetID:      datasetID,
			RefID:          refID,
			SyncRunID:      syncRunID,
			ErrorCode:      "enqueue_failed",
			ErrorMessage:   err.Error(),
		})
		return err
	}
	return nil
}

func parseAssetIDs(raw []string) ([]uuid.UUID, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	ids := make([]uuid.UUID, 0, len(raw))
	for _, item := range raw {
		id, err := uuid.Parse(item)
		if err != nil {
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}

func parseLimitOffset(c *gin.Context, defaultLimit int, maxLimit int) (int, int) {
	limit := defaultLimit
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	page := 1
	if raw := c.Query("page"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			page = parsed
		}
	}
	offset := (page - 1) * limit
	if raw := c.Query("offset"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	return limit, offset
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
