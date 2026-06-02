package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	datalibService "github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
	datalibWorker "github.com/zgiai/zgi/api/internal/modules/datalibrary/worker"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
)

type KnowledgeBaseFileRefHandler struct {
	service        datalibService.KnowledgeBaseFileRefService
	dispatcher     *datalibWorker.FileProcessTaskDispatcher
	accountService interfaces.AccountService
}

func NewKnowledgeBaseFileRefHandler(service datalibService.KnowledgeBaseFileRefService, dispatcher *datalibWorker.FileProcessTaskDispatcher, accountService interfaces.AccountService) *KnowledgeBaseFileRefHandler {
	return &KnowledgeBaseFileRefHandler{service: service, dispatcher: dispatcher, accountService: accountService}
}

func (h *KnowledgeBaseFileRefHandler) RegisterDatasetRoutes(router *gin.RouterGroup) {
	auth := router.Group("", middleware.JWTWithOrganizationAndService(h.accountService))
	group := auth.Group("/datasets/:dataset_id")
	group.GET("/file-candidates", h.ListFileCandidates)
	group.GET("/file-refs", h.ListFileRefs)
	group.POST("/file-refs", h.CreateFileRefs)
	group.POST("/file-refs/:ref_id/retry", h.RetryFileRef)
	group.POST("/file-refs/:ref_id/sync/retry", h.RetryFileRef)
}

func (h *KnowledgeBaseFileRefHandler) ListFileCandidates(c *gin.Context) {
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
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

func (h *KnowledgeBaseFileRefHandler) ListFileRefs(c *gin.Context) {
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
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
		if err := h.dispatcher.EnqueueDatasetRefSync(c.Request.Context(), item.Ref.ID, item.AssetID, item.Ref.DatasetID, item.GenerationNo, *item.SyncRunID); err != nil {
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
		if err := h.dispatcher.EnqueueDatasetRefSync(c.Request.Context(), result.Ref.ID, result.AssetID, result.Ref.DatasetID, result.GenerationNo, *result.SyncRunID); err != nil {
			response.FailWithMessage(c, response.ErrSystemError, err.Error())
			return
		}
	}
	response.Success(c, result)
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
