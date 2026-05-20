package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

type ProcessingExecutorHandler struct {
	registry                 *service.ProcessingExecutorRegistry
	processingRequestService service.ProcessingRequestService
}

func NewProcessingExecutorHandler(registry *service.ProcessingExecutorRegistry, processingRequestServices ...service.ProcessingRequestService) *ProcessingExecutorHandler {
	var processingRequestService service.ProcessingRequestService
	if len(processingRequestServices) > 0 {
		processingRequestService = processingRequestServices[0]
	}
	return &ProcessingExecutorHandler{
		registry:                 registry,
		processingRequestService: processingRequestService,
	}
}

func (h *ProcessingExecutorHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/data-library/processing-executors")
	group.GET("", h.ListProcessingExecutors)
	group.POST("/:executor_key/enqueue", h.EnqueueProcessingRequest)
	group.POST("/:executor_key/claim", h.ClaimProcessingRequest)
}

type processingExecutorEnqueueRequest struct {
	ProcessingRequestID string `json:"processing_request_id"`
}

func (h *ProcessingExecutorHandler) ListProcessingExecutors(c *gin.Context) {
	if h.registry == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing executor registry is not available")
		return
	}
	if util.GetOrganizationID(c) == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	items := h.registry.List()
	response.Success(c, gin.H{
		"items": items,
		"total": len(items),
	})
}

func (h *ProcessingExecutorHandler) EnqueueProcessingRequest(c *gin.Context) {
	if h.registry == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing executor registry is not available")
		return
	}
	if h.processingRequestService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing request service is not available")
		return
	}
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	executorKey := c.Param("executor_key")
	executor, err := h.registry.MustGet(executorKey)
	if err != nil {
		if errors.Is(err, service.ErrProcessingExecutorNotRegistered) {
			response.Fail(c, response.ErrNotFound)
			return
		}
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	if !executor.Info().Enabled {
		response.FailWithMessage(c, response.ErrInvalidParams, "data library processing executor is disabled")
		return
	}

	var req processingExecutorEnqueueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	requestID, err := uuid.Parse(req.ProcessingRequestID)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	view, err := h.processingRequestService.EnqueueRequest(c.Request.Context(), organizationID, requestID, executor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrProcessingRequestIDRequired):
			response.Fail(c, response.ErrInvalidUuid)
		case errors.Is(err, service.ErrProcessingRequestNotFound):
			response.Fail(c, response.ErrNotFound)
		case errors.Is(err, service.ErrProcessingRequestTransitionInvalid),
			errors.Is(err, service.ErrProcessingExecutorDisabled),
			errors.Is(err, service.ErrProcessingExecutorTargetUnsupported),
			errors.Is(err, service.ErrProcessingExecutorKeyRequired):
			response.Fail(c, response.ErrInvalidParams)
		case errors.Is(err, service.ErrOrganizationIDRequired):
			response.Fail(c, response.ErrUnauthorized)
		default:
			response.FailWithMessage(c, response.ErrSystemError, "failed to enqueue data library processing request")
		}
		return
	}
	response.Success(c, view)
}

func (h *ProcessingExecutorHandler) ClaimProcessingRequest(c *gin.Context) {
	executor, organizationID, ok := h.resolveEnabledExecutor(c)
	if !ok {
		return
	}
	if h.processingRequestService == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing request service is not available")
		return
	}

	view, err := h.processingRequestService.ClaimNextQueuedRequestForExecutor(c.Request.Context(), organizationID, executor)
	if err != nil {
		h.handleProcessingExecutorRequestError(c, err, "failed to claim data library processing request")
		return
	}
	response.Success(c, view)
}

func (h *ProcessingExecutorHandler) resolveEnabledExecutor(c *gin.Context) (service.RegisteredProcessingRequestExecutor, string, bool) {
	if h.registry == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library processing executor registry is not available")
		return nil, "", false
	}
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return nil, "", false
	}
	executor, err := h.registry.MustGet(c.Param("executor_key"))
	if err != nil {
		if errors.Is(err, service.ErrProcessingExecutorNotRegistered) {
			response.Fail(c, response.ErrNotFound)
			return nil, "", false
		}
		response.Fail(c, response.ErrInvalidParams)
		return nil, "", false
	}
	if !executor.Info().Enabled {
		response.FailWithMessage(c, response.ErrInvalidParams, "data library processing executor is disabled")
		return nil, "", false
	}
	return executor, organizationID, true
}

func (h *ProcessingExecutorHandler) handleProcessingExecutorRequestError(c *gin.Context, err error, message string) {
	switch {
	case errors.Is(err, service.ErrProcessingRequestIDRequired):
		response.Fail(c, response.ErrInvalidUuid)
	case errors.Is(err, service.ErrProcessingRequestNotFound):
		response.Fail(c, response.ErrNotFound)
	case errors.Is(err, service.ErrProcessingRequestTransitionInvalid),
		errors.Is(err, service.ErrProcessingExecutorDisabled),
		errors.Is(err, service.ErrProcessingExecutorTargetUnsupported),
		errors.Is(err, service.ErrProcessingExecutorRequired),
		errors.Is(err, service.ErrProcessingExecutorKeyRequired):
		response.Fail(c, response.ErrInvalidParams)
	case errors.Is(err, service.ErrOrganizationIDRequired):
		response.Fail(c, response.ErrUnauthorized)
	default:
		response.FailWithMessage(c, response.ErrSystemError, message)
	}
}
