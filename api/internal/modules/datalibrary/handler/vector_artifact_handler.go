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

type VectorArtifactHandler struct {
	service      service.VectorArtifactService
	assetService service.DocumentAssetService
}

func NewVectorArtifactHandler(vectorService service.VectorArtifactService, assetService service.DocumentAssetService) *VectorArtifactHandler {
	return &VectorArtifactHandler{
		service:      vectorService,
		assetService: assetService,
	}
}

func (h *VectorArtifactHandler) RegisterRoutes(router *gin.RouterGroup) {
	assetGroup := router.Group("/data-library/assets")
	assetGroup.GET("/:asset_id/vector-artifacts", h.ListAssetVectorArtifacts)

	group := router.Group("/data-library/vector-artifacts")
	group.GET("", h.ListVectorArtifacts)
	group.GET("/:artifact_id", h.GetVectorArtifact)
}

func (h *VectorArtifactHandler) ListAssetVectorArtifacts(c *gin.Context) {
	if h.service == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library vector artifact service is not available")
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
			logger.Error("Failed to load data library asset for vector artifacts", err)
			response.FailWithMessage(c, response.ErrSystemError, "failed to load data library asset")
			return
		}
		if asset == nil || asset.OrganizationID != organizationID {
			response.Fail(c, response.ErrNotFound)
			return
		}
	}

	filter, ok := h.vectorArtifactListFilter(c, organizationID)
	if !ok {
		return
	}
	filter.AssetID = assetID
	h.listVectorArtifacts(c, filter)
}

func (h *VectorArtifactHandler) ListVectorArtifacts(c *gin.Context) {
	if h.service == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library vector artifact service is not available")
		return
	}
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	filter, ok := h.vectorArtifactListFilter(c, organizationID)
	if !ok {
		return
	}
	h.listVectorArtifacts(c, filter)
}

func (h *VectorArtifactHandler) GetVectorArtifact(c *gin.Context) {
	if h.service == nil {
		response.FailWithMessage(c, response.ErrSystemError, "data library vector artifact service is not available")
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
		if errors.Is(err, service.ErrVectorArtifactNotFound) {
			response.Fail(c, response.ErrNotFound)
			return
		}
		logger.Error("Failed to get data library vector artifact", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to get data library vector artifact")
		return
	}
	if view.OrganizationID != organizationID {
		response.Fail(c, response.ErrNotFound)
		return
	}
	response.Success(c, gin.H{"vector_artifact": view})
}

func (h *VectorArtifactHandler) listVectorArtifacts(c *gin.Context, filter repository.VectorArtifactListFilter) {
	items, total, err := h.service.ListArtifactViews(c.Request.Context(), filter)
	if err != nil {
		logger.Error("Failed to list data library vector artifacts", err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to list data library vector artifacts")
		return
	}
	response.Success(c, gin.H{
		"items":  items,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

func (h *VectorArtifactHandler) vectorArtifactListFilter(c *gin.Context, organizationID string) (repository.VectorArtifactListFilter, bool) {
	filter := repository.VectorArtifactListFilter{
		OrganizationID: organizationID,
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
	if chunkArtifactSetID := c.Query("chunk_artifact_set_id"); chunkArtifactSetID != "" {
		parsed, err := uuid.Parse(chunkArtifactSetID)
		if err != nil {
			response.Fail(c, response.ErrInvalidUuid)
			return filter, false
		}
		filter.ChunkArtifactSetID = parsed
	}
	return filter, true
}
