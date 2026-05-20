package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

type ProviderHandler struct {
	service service.ProviderAdminService
}

func NewProviderHandler(service service.ProviderAdminService) *ProviderHandler {
	return &ProviderHandler{service: service}
}

func (h *ProviderHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/providers", h.ListProviders)
	rg.GET("/providers/:id", h.GetProvider)
	rg.POST("/providers", h.CreateProvider)
	rg.PUT("/providers/:id", h.UpdateProvider)
	rg.DELETE("/providers/:id", h.DeleteProvider)
}

func (h *ProviderHandler) ListProviders(c *gin.Context) {
	scope := defaultString(c.Query("scope"), "system")
	workspaceID, err := parseOptionalUUID(c.Query("workspace_id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid workspace_id")
		return
	}
	items, err := h.service.ListByScope(c.Request.Context(), scope, workspaceID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, items)
}

func (h *ProviderHandler) GetProvider(c *gin.Context) {
	id, err := parseRequiredUUID(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid provider id")
		return
	}
	item, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "404001", "message": "provider not found"})
		return
	}
	response.Success(c, item)
}

func (h *ProviderHandler) CreateProvider(c *gin.Context) {
	var item model.ProviderConfig
	if err := c.ShouldBindJSON(&item); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	if err := h.service.Create(c.Request.Context(), &item); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, item)
}

func (h *ProviderHandler) UpdateProvider(c *gin.Context) {
	id, err := parseRequiredUUID(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid provider id")
		return
	}
	var item model.ProviderConfig
	if err := c.ShouldBindJSON(&item); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	item.ID = id
	if err := h.service.Update(c.Request.Context(), &item); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, item)
}

func (h *ProviderHandler) DeleteProvider(c *gin.Context) {
	id, err := parseRequiredUUID(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid provider id")
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, gin.H{"id": id})
}

func parseRequiredUUID(raw string) (uuid.UUID, error) {
	return uuid.Parse(raw)
}

func parseOptionalUUID(raw string) (*uuid.UUID, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
