package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

type ProviderSettingsHandler struct {
	service service.ProviderSettingsService
}

func NewProviderSettingsHandler(service service.ProviderSettingsService) *ProviderSettingsHandler {
	return &ProviderSettingsHandler{service: service}
}

func (h *ProviderSettingsHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/provider-settings", h.List)
	rg.PUT("/provider-settings/:provider_key", h.Upsert)
	rg.POST("/provider-settings/:provider_key/check", h.Check)
}

func (h *ProviderSettingsHandler) List(c *gin.Context) {
	organizationID, ok := parserSettingsOrganizationID(c)
	if !ok {
		response.FailWithMessage(c, response.ErrUnauthorized, "organization context missing")
		return
	}
	items, err := h.service.List(c.Request.Context(), organizationID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, items)
}

func (h *ProviderSettingsHandler) Upsert(c *gin.Context) {
	organizationID, ok := parserSettingsOrganizationID(c)
	if !ok {
		response.FailWithMessage(c, response.ErrUnauthorized, "organization context missing")
		return
	}
	var req service.ParserSettingsInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	item, err := h.service.Upsert(c.Request.Context(), organizationID, parserSettingsActorID(c), c.Param("provider_key"), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUnsupportedParserProvider), errors.Is(err, service.ErrParserConfigInvalid), errors.Is(err, service.ErrParserValidationFailed):
			response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		default:
			response.FailWithMessage(c, response.ErrSystemError, err.Error())
		}
		return
	}
	response.Success(c, item)
}

func (h *ProviderSettingsHandler) Check(c *gin.Context) {
	organizationID, ok := parserSettingsOrganizationID(c)
	if !ok {
		response.FailWithMessage(c, response.ErrUnauthorized, "organization context missing")
		return
	}
	item, err := h.service.Check(c.Request.Context(), organizationID, parserSettingsActorID(c), c.Param("provider_key"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUnsupportedParserProvider), errors.Is(err, service.ErrParserConfigInvalid), errors.Is(err, service.ErrParserValidationFailed):
			response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		default:
			response.FailWithMessage(c, response.ErrSystemError, err.Error())
		}
		return
	}
	response.Success(c, item)
}

func parserSettingsOrganizationID(c *gin.Context) (uuid.UUID, bool) {
	raw := c.GetString("organization_id")
	if raw == "" {
		raw = c.GetString("tenant_id")
	}
	parsed, err := uuid.Parse(raw)
	return parsed, err == nil
}

func parserSettingsActorID(c *gin.Context) *uuid.UUID {
	raw := c.GetString("account_id")
	if raw == "" {
		return nil
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return &parsed
}
