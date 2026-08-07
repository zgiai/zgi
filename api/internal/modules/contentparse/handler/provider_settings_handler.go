package handler

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
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

	writes := rg.Group("/provider-settings")
	writes.Use(parserSettingsWriteRequired())
	writes.PUT("/:provider_key", h.Upsert)
	writes.POST("/:provider_key/check", h.Check)
}

// parserSettingsWriteRequired permits organization owners and administrators to
// manage shared provider settings. It also permits the account that is currently
// in the personal workbench to finish parser setup without first selecting an
// unrelated workspace.
func parserSettingsWriteRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if middleware.IsOrganizationAdminOrOwner(c) || isPersonalWorkbenchParserSettingsRequest(c) {
			c.Next()
			return
		}
		response.Fail(c, response.ErrPermissionDenied)
		c.Abort()
	}
}

func isPersonalWorkbenchParserSettingsRequest(c *gin.Context) bool {
	organizationID, ok := parserSettingsOrganizationID(c)
	if !ok {
		return false
	}
	accountID := strings.TrimSpace(c.GetString("account_id"))
	if accountID == "" {
		return false
	}
	accountServiceRaw, exists := c.Get("account_service")
	if !exists {
		return false
	}
	accountService, ok := accountServiceRaw.(interfaces.AccountService)
	if !ok {
		return false
	}
	accountContext, err := accountService.GetAccountContext(c.Request.Context(), accountID)
	if err != nil || accountContext == nil || accountContext.CurrentOrganizationID == nil {
		return false
	}
	return accountContext.CurrentWorkspaceID == nil &&
		strings.TrimSpace(*accountContext.CurrentOrganizationID) == organizationID.String()
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
