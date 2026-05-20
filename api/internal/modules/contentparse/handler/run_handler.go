package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/modules/contentparse/service"
	"github.com/zgiai/ginext/pkg/response"
)

type RunHandler struct {
	service         service.RunQueryService
	artifactService service.ArtifactService
}

func NewRunHandler(service service.RunQueryService, artifactService service.ArtifactService) *RunHandler {
	return &RunHandler{service: service, artifactService: artifactService}
}

func (h *RunHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/runs/:id", h.GetRun)
	rg.GET("/runs", h.ListRuns)
	rg.GET("/runs/:id/chunking", h.ListChunkingRuns)
	rg.GET("/shadow/documents/:document_id", h.GetLatestDocumentShadow)
	rg.GET("/shadow/datasets/:dataset_id", h.GetLatestDatasetShadow)
}

func (h *RunHandler) GetRun(c *gin.Context) {
	id, err := parseRequiredUUID(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid run id")
		return
	}
	item, err := h.service.GetParseRunByID(c.Request.Context(), id)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "404001", "message": "run not found"})
		return
	}
	response.Success(c, item)
}

func (h *RunHandler) ListRuns(c *gin.Context) {
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if rawDocumentID := c.Query("document_id"); rawDocumentID != "" {
		documentID, err := parseRequiredUUID(rawDocumentID)
		if err != nil {
			response.FailWithMessage(c, response.ErrInvalidParam, "invalid document_id")
			return
		}
		items, err := h.service.ListParseRunsByDocumentID(c.Request.Context(), documentID, limit)
		if err != nil {
			response.FailWithMessage(c, response.ErrSystemError, err.Error())
			return
		}
		response.Success(c, items)
		return
	}
	if rawDatasetID := c.Query("dataset_id"); rawDatasetID != "" {
		datasetID, err := parseRequiredUUID(rawDatasetID)
		if err != nil {
			response.FailWithMessage(c, response.ErrInvalidParam, "invalid dataset_id")
			return
		}
		items, err := h.service.ListParseRunsByDatasetID(c.Request.Context(), datasetID, limit)
		if err != nil {
			response.FailWithMessage(c, response.ErrSystemError, err.Error())
			return
		}
		response.Success(c, items)
		return
	}
	response.FailWithMessage(c, response.ErrInvalidParam, "document_id or dataset_id is required")
}

func (h *RunHandler) ListChunkingRuns(c *gin.Context) {
	id, err := parseRequiredUUID(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid run id")
		return
	}
	items, err := h.service.ListChunkingRunsByParseRunID(c.Request.Context(), id)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if items == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "404001", "message": "chunking runs not found"})
		return
	}
	response.Success(c, items)
}

func (h *RunHandler) GetLatestDocumentShadow(c *gin.Context) {
	documentID, err := parseRequiredUUID(c.Param("document_id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid document_id")
		return
	}

	runs, err := h.service.ListParseRunsByDocumentID(c.Request.Context(), documentID, 1)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if len(runs) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": "404001", "message": "shadow run not found"})
		return
	}

	latestRun := runs[0]
	chunkingRuns, err := h.service.ListChunkingRunsByParseRunID(c.Request.Context(), latestRun.ID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	var artifact interface{}
	if latestRun.ArtifactID != nil && h.artifactService != nil {
		item, getErr := h.artifactService.GetByID(c.Request.Context(), *latestRun.ArtifactID)
		if getErr != nil {
			response.FailWithMessage(c, response.ErrSystemError, getErr.Error())
			return
		}
		artifact = item
	}

	response.Success(c, gin.H{
		"run":           latestRun,
		"artifact":      artifact,
		"chunking_runs": chunkingRuns,
	})
}

func (h *RunHandler) GetLatestDatasetShadow(c *gin.Context) {
	datasetID, err := parseRequiredUUID(c.Param("dataset_id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid dataset_id")
		return
	}
	limit := 200
	if raw := c.Query("limit"); raw != "" {
		if parsed, parseErr := strconv.Atoi(raw); parseErr == nil && parsed > 0 {
			limit = parsed
		}
	}
	summary, err := h.service.GetLatestDatasetShadowSummary(c.Request.Context(), datasetID, limit)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, summary)
}
