package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/datalibrary/repository"
	"github.com/zgiai/ginext/internal/modules/datalibrary/service"
	"github.com/zgiai/ginext/internal/util"
	"github.com/zgiai/ginext/pkg/logger"
	"github.com/zgiai/ginext/pkg/response"
)

type ExtractionArtifactHandler struct {
	service      service.ExtractionArtifactService
	assetService service.DocumentAssetService
}

func NewExtractionArtifactHandler(extractionService service.ExtractionArtifactService, assetService service.DocumentAssetService) *ExtractionArtifactHandler {
	return &ExtractionArtifactHandler{
		service:      extractionService,
		assetService: assetService,
	}
}

func (h *ExtractionArtifactHandler) RegisterRoutes(router *gin.RouterGroup) {
	assetGroup := router.Group("/data-library/assets")
	assetGroup.GET("/:asset_id/extraction-artifacts", h.ListAssetExtractionArtifacts)

	group := router.Group("/data-library/extraction-artifacts")
	group.GET("", h.ListExtractionArtifacts)
	group.GET("/:artifact_id", h.GetExtractionArtifact)
}

func (h *ExtractionArtifactHandler) ListAssetExtractionArtifacts(c *gin.Context) {
	if h.service == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library extraction artifact service is not available")
		return
	}
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	assetID, err := uuid.Parse(c.Param("asset_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	if h.assetService != nil {
		asset, err := h.assetService.GetAssetByID(c.Request.Context(), assetID)
		if err != nil {
			logger.Error("Failed to load data library asset for extraction artifacts", err)
			response.FailWithMessage(c, response.ErrSystemError, "failed to load data library asset")
			return
		}
		if asset == nil || asset.OrganizationID != organizationID {
			response.Fail(c, response.ErrNotFound)
			return
		}
	}

	filter, ok := h.extractionArtifactListFilter(c, organizationID)
	if !ok {
		return
	}
	filter.AssetID = assetID
	h.listExtractionArtifacts(c, filter)
}

func (h *ExtractionArtifactHandler) ListExtractionArtifacts(c *gin.Context) {
	if h.service == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library extraction artifact service is not available")
		return
	}
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	filter, ok := h.extractionArtifactListFilter(c, organizationID)
	if !ok {
		return
	}
	h.listExtractionArtifacts(c, filter)
}

func (h *ExtractionArtifactHandler) GetExtractionArtifact(c *gin.Context) {
	if h.service == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library extraction artifact service is not available")
		return
	}
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	artifactID, err := uuid.Parse(c.Param("artifact_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	view, err := h.service.GetArtifactViewByID(c.Request.Context(), artifactID)
	if err != nil {
		if errors.Is(err, service.ErrExtractionArtifactNotFound) {
			response.Fail(c, response.ErrNotFound)
			return
		}
		logger.Error("Failed to get data library extraction artifact", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to get data library extraction artifact")
		return
	}
	if view.OrganizationID != organizationID {
		response.Fail(c, response.ErrNotFound)
		return
	}
	response.Success(c, gin.H{"extraction_artifact": view})
}

func (h *ExtractionArtifactHandler) listExtractionArtifacts(c *gin.Context, filter repository.ExtractionArtifactListFilter) {
	items, total, err := h.service.ListArtifactViews(c.Request.Context(), filter)
	if err != nil {
		logger.Error("Failed to list data library extraction artifacts", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to list data library extraction artifacts")
		return
	}
	response.Success(c, gin.H{
		"items":  items,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

func (h *ExtractionArtifactHandler) extractionArtifactListFilter(c *gin.Context, organizationID string) (repository.ExtractionArtifactListFilter, bool) {
	filter := repository.ExtractionArtifactListFilter{
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
	if parseArtifactID := c.Query("parse_artifact_id"); parseArtifactID != "" {
		parsed, err := uuid.Parse(parseArtifactID)
		if err != nil {
			response.Fail(c, response.ErrInvalidUuid)
			return filter, false
		}
		filter.ParseArtifactID = parsed
	}
	return filter, true
}
