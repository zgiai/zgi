package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/modules/contentparse/service"
	"github.com/zgiai/ginext/pkg/response"
)

type ArtifactHandler struct {
	service service.ArtifactService
}

func NewArtifactHandler(service service.ArtifactService) *ArtifactHandler {
	return &ArtifactHandler{service: service}
}

func (h *ArtifactHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/artifacts/:id", h.GetArtifact)
}

func (h *ArtifactHandler) GetArtifact(c *gin.Context) {
	id, err := parseRequiredUUID(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid artifact id")
		return
	}
	item, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "404001", "message": "artifact not found"})
		return
	}
	response.Success(c, item)
}
