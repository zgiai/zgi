package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

type DocumentAssetHandler struct {
	service                  service.DocumentAssetService
	syncService              service.FileAssetSyncService
	processingRequestService service.ProcessingRequestService
	knowledgeBaseRefService  service.KnowledgeBaseAssetRefService
	databaseRefService       service.DatabaseAssetRefService
}

func NewDocumentAssetHandler(assetService service.DocumentAssetService, syncService service.FileAssetSyncService, processingRequestService service.ProcessingRequestService, knowledgeBaseRefService service.KnowledgeBaseAssetRefService, databaseRefServices ...service.DatabaseAssetRefService) *DocumentAssetHandler {
	var databaseRefService service.DatabaseAssetRefService
	if len(databaseRefServices) > 0 {
		databaseRefService = databaseRefServices[0]
	}
	return &DocumentAssetHandler{
		service:                  assetService,
		syncService:              syncService,
		processingRequestService: processingRequestService,
		knowledgeBaseRefService:  knowledgeBaseRefService,
		databaseRefService:       databaseRefService,
	}
}

func (h *DocumentAssetHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/data-library/assets")
	group.GET("", h.ListAssets)
	group.POST("/sync-file", h.SyncFileAsset)
	group.POST("/sync-files", h.SyncFileAssets)
	group.POST("/:asset_id/processing-plan", h.PlanAssetProcessing)
	group.POST("/:asset_id/processing-requests", h.CreateAssetProcessingRequest)
	group.GET("/:asset_id/processing-requests", h.ListAssetProcessingRequests)
	group.GET("/:asset_id/knowledge-base-refs", h.ListAssetKnowledgeBaseRefs)
	group.GET("/:asset_id/database-refs", h.ListAssetDatabaseRefs)
	group.GET("/:asset_id/reuse-events", h.ListAssetReuseEvents)
	group.GET("/:asset_id", h.GetAsset)

	requestGroup := router.Group("/data-library/processing-requests")
	requestGroup.GET("", h.ListProcessingRequests)
	requestGroup.GET("/summary", h.SummarizeProcessingRequests)
	requestGroup.POST("/claim", h.ClaimProcessingRequest)
	requestGroup.POST("/:request_id/queue", h.QueueProcessingRequest)
	requestGroup.POST("/:request_id/start", h.StartProcessingRequest)
	requestGroup.POST("/:request_id/complete", h.CompleteProcessingRequest)
	requestGroup.POST("/:request_id/fail", h.FailProcessingRequest)
	requestGroup.POST("/:request_id/cancel", h.CancelProcessingRequest)
	requestGroup.POST("/:request_id/retry", h.RetryProcessingRequest)

	refGroup := router.Group("/data-library/knowledge-base-asset-refs")
	refGroup.GET("", h.ListKnowledgeBaseAssetRefs)
	refGroup.POST("", h.CreateKnowledgeBaseAssetRef)
	refGroup.POST("/:ref_id/disable", h.DisableKnowledgeBaseAssetRef)

	dbRefGroup := router.Group("/data-library/database-asset-refs")
	dbRefGroup.GET("", h.ListDatabaseAssetRefs)
	dbRefGroup.POST("", h.CreateDatabaseAssetRef)
	dbRefGroup.POST("/:ref_id/disable", h.DisableDatabaseAssetRef)
}

type syncFileAssetRequest struct {
	FileID string `json:"file_id"`
}

type syncFileAssetsRequest struct {
	FileIDs []string `json:"file_ids"`
}

type processingPlanRequest struct {
	TargetLevel     string         `json:"target_level"`
	Force           bool           `json:"force"`
	RequestMetadata map[string]any `json:"request_metadata,omitempty"`
}

type processingRequestCancelRequest struct {
	Reason string `json:"reason,omitempty"`
}

type processingRequestRetryRequest struct {
	Force           *bool          `json:"force,omitempty"`
	RequestMetadata map[string]any `json:"request_metadata,omitempty"`
}

type processingRequestClaimRequest struct {
	ExecutorKey string `json:"executor_key"`
}

type processingRequestStartRequest struct {
	ExecutorKey string `json:"executor_key"`
}

type processingRequestCompleteRequest struct {
	ExecutionMetadata map[string]any `json:"execution_metadata,omitempty"`
}

type processingRequestFailRequest struct {
	ErrorCode         string         `json:"error_code,omitempty"`
	ErrorMessage      string         `json:"error_message,omitempty"`
	ExecutionMetadata map[string]any `json:"execution_metadata,omitempty"`
}

type knowledgeBaseAssetRefRequest struct {
	DatasetID          string         `json:"dataset_id"`
	AssetID            string         `json:"asset_id"`
	VersionID          string         `json:"version_id"`
	ChunkArtifactSetID *string        `json:"chunk_artifact_set_id,omitempty"`
	VectorArtifactID   *string        `json:"vector_artifact_id,omitempty"`
	MetadataJSON       map[string]any `json:"metadata_json,omitempty"`
}

type databaseAssetRefRequest struct {
	DataSourceID         string         `json:"data_source_id"`
	TableID              *string        `json:"table_id,omitempty"`
	AssetID              string         `json:"asset_id"`
	VersionID            string         `json:"version_id"`
	ParseArtifactID      *string        `json:"parse_artifact_id,omitempty"`
	ExtractionArtifactID *string        `json:"extraction_artifact_id,omitempty"`
	MetadataJSON         map[string]any `json:"metadata_json,omitempty"`
}

func (h *DocumentAssetHandler) ListAssets(c *gin.Context) {
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	filter := repository.DocumentAssetListFilter{
		OrganizationID: organizationID,
		Status:         c.Query("status"),
		Limit:          parseIntQuery(c, "limit", 20),
		Offset:         parseIntQuery(c, "offset", 0),
	}
	if workspaceID := c.Query("workspace_id"); workspaceID != "" {
		filter.WorkspaceID = &workspaceID
	} else if workspaceID := util.GetWorkspaceID(c); workspaceID != "" {
		filter.WorkspaceID = &workspaceID
	}

	items, total, err := h.service.ListAssetViews(c.Request.Context(), filter)
	if err != nil {
		logger.Error("Failed to list data library assets", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to list data library assets")
		return
	}

	response.Success(c, gin.H{
		"items":  items,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

func (h *DocumentAssetHandler) SyncFileAsset(c *gin.Context) {
	if h.syncService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library file sync service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req syncFileAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	if req.FileID == "" {
		response.Fail(c, response.ErrFileIdRequired)
		return
	}

	view, created, err := h.syncService.SyncFileAsArchivedAsset(c.Request.Context(), organizationID, req.FileID, util.GetAccountID(c))
	if err != nil {
		if errors.Is(err, service.ErrFileIDRequired) {
			response.Fail(c, response.ErrFileIdRequired)
			return
		}
		if errors.Is(err, service.ErrSourceFileMissing) {
			response.Fail(c, response.ErrNotFound)
			return
		}
		logger.Error("Failed to sync file into data library asset", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to sync file into data library asset")
		return
	}

	response.Success(c, gin.H{
		"asset":   view,
		"created": created,
	})
}

func (h *DocumentAssetHandler) SyncFileAssets(c *gin.Context) {
	if h.syncService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library file sync service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req syncFileAssetsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	if len(req.FileIDs) == 0 {
		response.Fail(c, response.ErrFileIdRequired)
		return
	}

	result, err := h.syncService.SyncFilesAsArchivedAssets(c.Request.Context(), organizationID, req.FileIDs, util.GetAccountID(c))
	if err != nil {
		if errors.Is(err, service.ErrFileIDsRequired) {
			response.Fail(c, response.ErrFileIdRequired)
			return
		}
		logger.Error("Failed to sync files into data library assets", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to sync files into data library assets")
		return
	}

	response.Success(c, result)
}

func (h *DocumentAssetHandler) GetAsset(c *gin.Context) {
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	assetID, err := uuid.Parse(c.Param("asset_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	view, err := h.service.GetAssetViewByID(c.Request.Context(), assetID)
	if err != nil {
		if errors.Is(err, service.ErrAssetIDRequired) {
			response.Fail(c, response.ErrInvalidUuid)
			return
		}
		logger.Error("Failed to get data library asset", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to get data library asset")
		return
	}
	if view == nil || view.OrganizationID != organizationID {
		response.Fail(c, response.ErrNotFound)
		return
	}

	response.Success(c, view)
}

func (h *DocumentAssetHandler) PlanAssetProcessing(c *gin.Context) {
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	assetID, err := uuid.Parse(c.Param("asset_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	view, err := h.service.GetAssetViewByID(c.Request.Context(), assetID)
	if err != nil {
		logger.Error("Failed to get data library asset for processing plan", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to get data library asset")
		return
	}
	if view == nil || view.OrganizationID != organizationID {
		response.Fail(c, response.ErrNotFound)
		return
	}

	var req processingPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	plan, err := service.PlanProcessingRequest(service.ProcessingRequest{
		OrganizationID:  organizationID,
		WorkspaceID:     view.WorkspaceID,
		AssetID:         assetID,
		TargetLevel:     req.TargetLevel,
		RequestedBy:     util.GetAccountID(c),
		Force:           req.Force,
		RequestMetadata: req.RequestMetadata,
	})
	if err != nil {
		if errors.Is(err, service.ErrProcessingLevelRequired) || errors.Is(err, service.ErrProcessingLevelInvalid) {
			response.Fail(c, response.ErrInvalidParams)
			return
		}
		logger.Error("Failed to plan data library processing request", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to plan data library processing request")
		return
	}

	response.Success(c, gin.H{
		"asset": view,
		"plan":  plan,
	})
}

func (h *DocumentAssetHandler) CreateAssetProcessingRequest(c *gin.Context) {
	if h.processingRequestService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing request service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	assetID, err := uuid.Parse(c.Param("asset_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	view, err := h.service.GetAssetViewByID(c.Request.Context(), assetID)
	if err != nil {
		logger.Error("Failed to get data library asset for processing request", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to get data library asset")
		return
	}
	if view == nil || view.OrganizationID != organizationID {
		response.Fail(c, response.ErrNotFound)
		return
	}

	var req processingPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	processingRequest, err := h.processingRequestService.CreatePlannedRequest(c.Request.Context(), service.ProcessingRequest{
		OrganizationID:  organizationID,
		WorkspaceID:     view.WorkspaceID,
		AssetID:         assetID,
		TargetLevel:     req.TargetLevel,
		RequestedBy:     util.GetAccountID(c),
		Force:           req.Force,
		RequestMetadata: req.RequestMetadata,
	})
	if err != nil {
		if errors.Is(err, service.ErrProcessingLevelRequired) || errors.Is(err, service.ErrProcessingLevelInvalid) {
			response.Fail(c, response.ErrInvalidParams)
			return
		}
		logger.Error("Failed to create data library processing request", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to create data library processing request")
		return
	}

	response.Success(c, gin.H{
		"asset":              view,
		"processing_request": processingRequest,
	})
}

func (h *DocumentAssetHandler) ListAssetProcessingRequests(c *gin.Context) {
	if h.processingRequestService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing request service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	assetID, err := uuid.Parse(c.Param("asset_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	view, err := h.service.GetAssetViewByID(c.Request.Context(), assetID)
	if err != nil {
		logger.Error("Failed to get data library asset for processing request list", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to get data library asset")
		return
	}
	if view == nil || view.OrganizationID != organizationID {
		response.Fail(c, response.ErrNotFound)
		return
	}

	filter := repository.ProcessingRequestListFilter{
		OrganizationID: organizationID,
		AssetID:        assetID,
		Status:         c.Query("status"),
		ExecutorKey:    c.Query("executor_key"),
		Limit:          parseIntQuery(c, "limit", 20),
		Offset:         parseIntQuery(c, "offset", 0),
	}
	items, total, err := h.processingRequestService.ListRequests(c.Request.Context(), filter)
	if err != nil {
		logger.Error("Failed to list data library processing requests", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to list data library processing requests")
		return
	}

	response.Success(c, gin.H{
		"items":  items,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

func (h *DocumentAssetHandler) ListProcessingRequests(c *gin.Context) {
	if h.processingRequestService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing request service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	filter := repository.ProcessingRequestListFilter{
		OrganizationID: organizationID,
		TargetLevel:    c.Query("target_level"),
		Status:         c.Query("status"),
		ExecutorKey:    c.Query("executor_key"),
		Limit:          parseIntQuery(c, "limit", 20),
		Offset:         parseIntQuery(c, "offset", 0),
	}
	if assetIDParam := c.Query("asset_id"); assetIDParam != "" {
		assetID, err := uuid.Parse(assetIDParam)
		if err != nil {
			response.Fail(c, response.ErrInvalidUuid)
			return
		}
		filter.AssetID = assetID
	}

	items, total, err := h.processingRequestService.ListRequests(c.Request.Context(), filter)
	if err != nil {
		if errors.Is(err, service.ErrProcessingLevelInvalid) {
			response.Fail(c, response.ErrInvalidParams)
			return
		}
		logger.Error("Failed to list data library processing requests", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to list data library processing requests")
		return
	}

	response.Success(c, gin.H{
		"items":  items,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

func (h *DocumentAssetHandler) SummarizeProcessingRequests(c *gin.Context) {
	if h.processingRequestService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing request service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	filter := repository.ProcessingRequestQueueSummaryFilter{
		OrganizationID: organizationID,
		TargetLevel:    c.Query("target_level"),
		Status:         c.Query("status"),
		ExecutorKey:    c.Query("executor_key"),
	}
	items, err := h.processingRequestService.QueueSummary(c.Request.Context(), filter)
	if err != nil {
		if errors.Is(err, service.ErrProcessingLevelInvalid) {
			response.Fail(c, response.ErrInvalidParams)
			return
		}
		logger.Error("Failed to summarize data library processing requests", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to summarize data library processing requests")
		return
	}

	response.Success(c, gin.H{
		"items": items,
		"total": len(items),
	})
}

func (h *DocumentAssetHandler) QueueProcessingRequest(c *gin.Context) {
	if h.processingRequestService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing request service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	requestID, err := uuid.Parse(c.Param("request_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	view, err := h.processingRequestService.QueueRequest(c.Request.Context(), organizationID, requestID)
	if err != nil {
		h.handleProcessingRequestControlError(c, err, "failed to queue data library processing request")
		return
	}
	response.Success(c, view)
}

func (h *DocumentAssetHandler) ClaimProcessingRequest(c *gin.Context) {
	if h.processingRequestService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing request service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req processingRequestClaimRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	if req.ExecutorKey == "" {
		response.Fail(c, response.ErrInvalidParams)
		return
	}

	view, err := h.processingRequestService.ClaimNextQueuedRequest(c.Request.Context(), organizationID, req.ExecutorKey)
	if err != nil {
		h.handleProcessingRequestControlError(c, err, "failed to claim data library processing request")
		return
	}
	response.Success(c, view)
}

func (h *DocumentAssetHandler) StartProcessingRequest(c *gin.Context) {
	if h.processingRequestService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing request service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	requestID, err := uuid.Parse(c.Param("request_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	var req processingRequestStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	if req.ExecutorKey == "" {
		response.Fail(c, response.ErrInvalidParams)
		return
	}

	view, err := h.processingRequestService.StartRequest(c.Request.Context(), organizationID, requestID, req.ExecutorKey)
	if err != nil {
		h.handleProcessingRequestControlError(c, err, "failed to start data library processing request")
		return
	}
	response.Success(c, view)
}

func (h *DocumentAssetHandler) CompleteProcessingRequest(c *gin.Context) {
	if h.processingRequestService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing request service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	requestID, err := uuid.Parse(c.Param("request_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	var req processingRequestCompleteRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, response.ErrInvalidParams)
			return
		}
	}

	view, err := h.processingRequestService.CompleteRequest(c.Request.Context(), organizationID, requestID, req.ExecutionMetadata)
	if err != nil {
		h.handleProcessingRequestControlError(c, err, "failed to complete data library processing request")
		return
	}
	response.Success(c, view)
}

func (h *DocumentAssetHandler) FailProcessingRequest(c *gin.Context) {
	if h.processingRequestService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing request service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	requestID, err := uuid.Parse(c.Param("request_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	var req processingRequestFailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}

	view, err := h.processingRequestService.FailRequest(c.Request.Context(), organizationID, requestID, req.ErrorCode, req.ErrorMessage, req.ExecutionMetadata)
	if err != nil {
		h.handleProcessingRequestControlError(c, err, "failed to fail data library processing request")
		return
	}
	response.Success(c, view)
}

func (h *DocumentAssetHandler) CancelProcessingRequest(c *gin.Context) {
	if h.processingRequestService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing request service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	requestID, err := uuid.Parse(c.Param("request_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	var req processingRequestCancelRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, response.ErrInvalidParams)
			return
		}
	}

	view, err := h.processingRequestService.CancelRequest(c.Request.Context(), organizationID, requestID, req.Reason)
	if err != nil {
		h.handleProcessingRequestControlError(c, err, "failed to cancel data library processing request")
		return
	}
	response.Success(c, view)
}

func (h *DocumentAssetHandler) RetryProcessingRequest(c *gin.Context) {
	if h.processingRequestService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing request service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	requestID, err := uuid.Parse(c.Param("request_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	var req processingRequestRetryRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, response.ErrInvalidParams)
			return
		}
	}

	view, err := h.processingRequestService.RetryRequest(c.Request.Context(), organizationID, requestID, util.GetAccountID(c), req.Force, req.RequestMetadata)
	if err != nil {
		h.handleProcessingRequestControlError(c, err, "failed to retry data library processing request")
		return
	}
	response.Success(c, view)
}

func (h *DocumentAssetHandler) handleProcessingRequestControlError(c *gin.Context, err error, message string) {
	switch {
	case errors.Is(err, service.ErrProcessingRequestIDRequired):
		response.Fail(c, response.ErrInvalidUuid)
	case errors.Is(err, service.ErrProcessingRequestNotFound):
		response.Fail(c, response.ErrNotFound)
	case errors.Is(err, service.ErrProcessingRequestTransitionInvalid):
		response.Fail(c, response.ErrInvalidParams)
	case errors.Is(err, service.ErrProcessingExecutorKeyRequired):
		response.Fail(c, response.ErrInvalidParams)
	case errors.Is(err, service.ErrOrganizationIDRequired):
		response.Fail(c, response.ErrUnauthorized)
	default:
		logger.Error(message, err)
		response.FailWithMessage(c, response.ErrSystemError, message)
	}
}

func (h *DocumentAssetHandler) ListAssetReuseEvents(c *gin.Context) {
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	assetID, err := uuid.Parse(c.Param("asset_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	view, err := h.service.GetAssetViewByID(c.Request.Context(), assetID)
	if err != nil {
		logger.Error("Failed to get data library asset for reuse events", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to get data library asset")
		return
	}
	if view == nil || view.OrganizationID != organizationID {
		response.Fail(c, response.ErrNotFound)
		return
	}

	filter := repository.ReuseEventListFilter{
		OrganizationID: organizationID,
		AssetID:        &assetID,
		ConsumerType:   c.Query("consumer_type"),
		ConsumerID:     c.Query("consumer_id"),
		Limit:          parseIntQuery(c, "limit", 20),
		Offset:         parseIntQuery(c, "offset", 0),
	}
	items, total, err := h.service.ListReuseEvents(c.Request.Context(), filter)
	if err != nil {
		logger.Error("Failed to list data library reuse events", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to list data library reuse events")
		return
	}

	response.Success(c, gin.H{
		"items":  items,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

func (h *DocumentAssetHandler) ListAssetKnowledgeBaseRefs(c *gin.Context) {
	if h.knowledgeBaseRefService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library knowledge base ref service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	assetID, err := uuid.Parse(c.Param("asset_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	view, err := h.service.GetAssetViewByID(c.Request.Context(), assetID)
	if err != nil {
		logger.Error("Failed to get data library asset for knowledge base refs", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to get data library asset")
		return
	}
	if view == nil || view.OrganizationID != organizationID {
		response.Fail(c, response.ErrNotFound)
		return
	}

	filter, ok := h.knowledgeBaseRefListFilter(c, organizationID)
	if !ok {
		return
	}
	filter.AssetID = assetID

	items, total, err := h.knowledgeBaseRefService.ListRefViews(c.Request.Context(), filter)
	if err != nil {
		logger.Error("Failed to list data library knowledge base refs", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to list data library knowledge base refs")
		return
	}

	response.Success(c, gin.H{
		"items":  items,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

func (h *DocumentAssetHandler) ListKnowledgeBaseAssetRefs(c *gin.Context) {
	if h.knowledgeBaseRefService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library knowledge base ref service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	filter, ok := h.knowledgeBaseRefListFilter(c, organizationID)
	if !ok {
		return
	}

	items, total, err := h.knowledgeBaseRefService.ListRefViews(c.Request.Context(), filter)
	if err != nil {
		logger.Error("Failed to list data library knowledge base refs", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to list data library knowledge base refs")
		return
	}

	response.Success(c, gin.H{
		"items":  items,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

func (h *DocumentAssetHandler) CreateKnowledgeBaseAssetRef(c *gin.Context) {
	if h.knowledgeBaseRefService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library knowledge base ref service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req knowledgeBaseAssetRefRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	assetID, versionID, chunkSetID, vectorID, ok := parseKnowledgeBaseAssetRefRequest(c, req)
	if !ok {
		return
	}

	asset, err := h.service.GetAssetViewByID(c.Request.Context(), assetID)
	if err != nil {
		logger.Error("Failed to get data library asset for knowledge base ref", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to get data library asset")
		return
	}
	if asset == nil || asset.OrganizationID != organizationID {
		response.Fail(c, response.ErrNotFound)
		return
	}
	version, err := h.service.GetVersionByID(c.Request.Context(), versionID)
	if err != nil {
		logger.Error("Failed to get data library version for knowledge base ref", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to get data library version")
		return
	}
	if version == nil || version.AssetID != assetID {
		response.Fail(c, response.ErrNotFound)
		return
	}

	view, err := h.knowledgeBaseRefService.CreateRef(c.Request.Context(), &model.KnowledgeBaseAssetRef{
		OrganizationID:     organizationID,
		WorkspaceID:        asset.WorkspaceID,
		DatasetID:          req.DatasetID,
		AssetID:            assetID,
		VersionID:          versionID,
		ChunkArtifactSetID: chunkSetID,
		VectorArtifactID:   vectorID,
		MetadataJSON:       req.MetadataJSON,
		CreatedBy:          util.GetAccountID(c),
	})
	if err != nil {
		if errors.Is(err, service.ErrDatasetIDRequired) ||
			errors.Is(err, service.ErrAssetIDRequired) ||
			errors.Is(err, service.ErrVersionIDRequired) {
			response.Fail(c, response.ErrInvalidParams)
			return
		}
		logger.Error("Failed to create data library knowledge base ref", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to create data library knowledge base ref")
		return
	}

	response.Success(c, view)
}

func (h *DocumentAssetHandler) DisableKnowledgeBaseAssetRef(c *gin.Context) {
	if h.knowledgeBaseRefService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library knowledge base ref service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	refID, err := uuid.Parse(c.Param("ref_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	view, err := h.knowledgeBaseRefService.DisableRef(c.Request.Context(), organizationID, refID)
	if err != nil {
		if errors.Is(err, service.ErrKnowledgeBaseAssetRefNotFound) {
			response.Fail(c, response.ErrNotFound)
			return
		}
		logger.Error("Failed to disable data library knowledge base ref", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to disable data library knowledge base ref")
		return
	}

	response.Success(c, view)
}

func (h *DocumentAssetHandler) ListAssetDatabaseRefs(c *gin.Context) {
	if h.databaseRefService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library database ref service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	assetID, err := uuid.Parse(c.Param("asset_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	view, err := h.service.GetAssetViewByID(c.Request.Context(), assetID)
	if err != nil {
		logger.Error("Failed to get data library asset for database refs", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to get data library asset")
		return
	}
	if view == nil || view.OrganizationID != organizationID {
		response.Fail(c, response.ErrNotFound)
		return
	}

	filter, ok := h.databaseRefListFilter(c, organizationID)
	if !ok {
		return
	}
	filter.AssetID = assetID

	items, total, err := h.databaseRefService.ListRefViews(c.Request.Context(), filter)
	if err != nil {
		logger.Error("Failed to list data library database refs", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to list data library database refs")
		return
	}

	response.Success(c, gin.H{
		"items":  items,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

func (h *DocumentAssetHandler) ListDatabaseAssetRefs(c *gin.Context) {
	if h.databaseRefService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library database ref service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	filter, ok := h.databaseRefListFilter(c, organizationID)
	if !ok {
		return
	}

	items, total, err := h.databaseRefService.ListRefViews(c.Request.Context(), filter)
	if err != nil {
		logger.Error("Failed to list data library database refs", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to list data library database refs")
		return
	}

	response.Success(c, gin.H{
		"items":  items,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

func (h *DocumentAssetHandler) CreateDatabaseAssetRef(c *gin.Context) {
	if h.databaseRefService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library database ref service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req databaseAssetRefRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	assetID, versionID, parseArtifactID, extractionArtifactID, ok := parseDatabaseAssetRefRequest(c, req)
	if !ok {
		return
	}

	asset, err := h.service.GetAssetViewByID(c.Request.Context(), assetID)
	if err != nil {
		logger.Error("Failed to get data library asset for database ref", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to get data library asset")
		return
	}
	if asset == nil || asset.OrganizationID != organizationID {
		response.Fail(c, response.ErrNotFound)
		return
	}
	version, err := h.service.GetVersionByID(c.Request.Context(), versionID)
	if err != nil {
		logger.Error("Failed to get data library version for database ref", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to get data library version")
		return
	}
	if version == nil || version.AssetID != assetID {
		response.Fail(c, response.ErrNotFound)
		return
	}
	if parseArtifactID != nil && version.ParseArtifactID != nil && *parseArtifactID != *version.ParseArtifactID {
		response.Fail(c, response.ErrInvalidParams)
		return
	}

	view, err := h.databaseRefService.CreateRef(c.Request.Context(), &model.DatabaseAssetRef{
		OrganizationID:       organizationID,
		WorkspaceID:          asset.WorkspaceID,
		DataSourceID:         req.DataSourceID,
		TableID:              req.TableID,
		AssetID:              assetID,
		VersionID:            versionID,
		ParseArtifactID:      parseArtifactID,
		ExtractionArtifactID: extractionArtifactID,
		MetadataJSON:         req.MetadataJSON,
		CreatedBy:            util.GetAccountID(c),
	})
	if err != nil {
		if errors.Is(err, service.ErrDataSourceIDRequired) ||
			errors.Is(err, service.ErrAssetIDRequired) ||
			errors.Is(err, service.ErrVersionIDRequired) {
			response.Fail(c, response.ErrInvalidParams)
			return
		}
		logger.Error("Failed to create data library database ref", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to create data library database ref")
		return
	}

	response.Success(c, view)
}

func (h *DocumentAssetHandler) DisableDatabaseAssetRef(c *gin.Context) {
	if h.databaseRefService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library database ref service is not available")
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	refID, err := uuid.Parse(c.Param("ref_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	view, err := h.databaseRefService.DisableRef(c.Request.Context(), organizationID, refID)
	if err != nil {
		if errors.Is(err, service.ErrDatabaseAssetRefNotFound) {
			response.Fail(c, response.ErrNotFound)
			return
		}
		logger.Error("Failed to disable data library database ref", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to disable data library database ref")
		return
	}

	response.Success(c, view)
}

func parseIntQuery(c *gin.Context, key string, fallback int) int {
	value := c.Query(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseKnowledgeBaseAssetRefRequest(c *gin.Context, req knowledgeBaseAssetRefRequest) (uuid.UUID, uuid.UUID, *uuid.UUID, *uuid.UUID, bool) {
	assetID, err := uuid.Parse(req.AssetID)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return uuid.Nil, uuid.Nil, nil, nil, false
	}
	versionID, err := uuid.Parse(req.VersionID)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return uuid.Nil, uuid.Nil, nil, nil, false
	}
	chunkSetID, ok := parseOptionalUUID(c, req.ChunkArtifactSetID)
	if !ok {
		return uuid.Nil, uuid.Nil, nil, nil, false
	}
	vectorID, ok := parseOptionalUUID(c, req.VectorArtifactID)
	if !ok {
		return uuid.Nil, uuid.Nil, nil, nil, false
	}
	return assetID, versionID, chunkSetID, vectorID, true
}

func parseDatabaseAssetRefRequest(c *gin.Context, req databaseAssetRefRequest) (uuid.UUID, uuid.UUID, *uuid.UUID, *uuid.UUID, bool) {
	assetID, err := uuid.Parse(req.AssetID)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return uuid.Nil, uuid.Nil, nil, nil, false
	}
	versionID, err := uuid.Parse(req.VersionID)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return uuid.Nil, uuid.Nil, nil, nil, false
	}
	parseArtifactID, ok := parseOptionalUUID(c, req.ParseArtifactID)
	if !ok {
		return uuid.Nil, uuid.Nil, nil, nil, false
	}
	extractionArtifactID, ok := parseOptionalUUID(c, req.ExtractionArtifactID)
	if !ok {
		return uuid.Nil, uuid.Nil, nil, nil, false
	}
	return assetID, versionID, parseArtifactID, extractionArtifactID, true
}

func parseOptionalUUID(c *gin.Context, value *string) (*uuid.UUID, bool) {
	if value == nil || *value == "" {
		return nil, true
	}
	parsed, err := uuid.Parse(*value)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return nil, false
	}
	return &parsed, true
}

func (h *DocumentAssetHandler) knowledgeBaseRefListFilter(c *gin.Context, organizationID string) (repository.KnowledgeBaseAssetRefListFilter, bool) {
	filter := repository.KnowledgeBaseAssetRefListFilter{
		OrganizationID: organizationID,
		DatasetID:      c.Query("dataset_id"),
		Status:         c.Query("status"),
		Limit:          parseIntQuery(c, "limit", 20),
		Offset:         parseIntQuery(c, "offset", 0),
	}
	if assetID := c.Query("asset_id"); assetID != "" {
		parsed, err := uuid.Parse(assetID)
		if err != nil {
			response.Fail(c, response.ErrInvalidUuid)
			return filter, false
		}
		filter.AssetID = parsed
	}
	if versionID := c.Query("version_id"); versionID != "" {
		parsed, err := uuid.Parse(versionID)
		if err != nil {
			response.Fail(c, response.ErrInvalidUuid)
			return filter, false
		}
		filter.VersionID = parsed
	}
	return filter, true
}

func (h *DocumentAssetHandler) databaseRefListFilter(c *gin.Context, organizationID string) (repository.DatabaseAssetRefListFilter, bool) {
	filter := repository.DatabaseAssetRefListFilter{
		OrganizationID: organizationID,
		DataSourceID:   c.Query("data_source_id"),
		TableID:        c.Query("table_id"),
		Status:         c.Query("status"),
		Limit:          parseIntQuery(c, "limit", 20),
		Offset:         parseIntQuery(c, "offset", 0),
	}
	if assetID := c.Query("asset_id"); assetID != "" {
		parsed, err := uuid.Parse(assetID)
		if err != nil {
			response.Fail(c, response.ErrInvalidUuid)
			return filter, false
		}
		filter.AssetID = parsed
	}
	if versionID := c.Query("version_id"); versionID != "" {
		parsed, err := uuid.Parse(versionID)
		if err != nil {
			response.Fail(c, response.ErrInvalidUuid)
			return filter, false
		}
		filter.VersionID = parsed
	}
	return filter, true
}
