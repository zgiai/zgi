package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/provider/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/provider/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

var ErrInvalidOrganizationID = errors.New("invalid tenant id")

const platformCatalogReadOnlyMessage = "当前模式下平台模型目录不可写"

type ProviderHandler struct {
	service                 service.ProviderService
	platformCatalogReadOnly bool
}

func NewProviderHandler(svc service.ProviderService) *ProviderHandler {
	return &ProviderHandler{service: svc}
}

func (h *ProviderHandler) SetPlatformCatalogReadOnly(readOnly bool) {
	h.platformCatalogReadOnly = readOnly
}

func (h *ProviderHandler) denyPlatformCatalogWrite(c *gin.Context) bool {
	if !h.platformCatalogReadOnly {
		return false
	}

	response.FailWithMessage(c, response.ErrActionNotAllowed, platformCatalogReadOnlyMessage)
	return true
}

// Global provider operations (admin)
func (h *ProviderHandler) CreateGlobal(c *gin.Context) {
	if h.denyPlatformCatalogWrite(c) {
		return
	}
	var req dto.CreateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	provider, err := h.service.CreateGlobal(c.Request.Context(), &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, provider)
}

func (h *ProviderHandler) GetGlobal(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid provider id")
		return
	}
	provider, err := h.service.GetGlobal(c.Request.Context(), id)
	if err != nil {
		response.FailWithMessage(c, response.ErrNotFound, err.Error())
		return
	}
	response.Success(c, provider)
}

func (h *ProviderHandler) ListGlobal(c *gin.Context) {
	var req dto.ListProviderRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 1000
	}
	providers, total, err := h.service.ListGlobal(c.Request.Context(), &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, gin.H{"list": providers, "total": total, "page": req.Page, "page_size": req.PageSize})
}

func (h *ProviderHandler) UpdateGlobal(c *gin.Context) {
	if h.denyPlatformCatalogWrite(c) {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid provider id")
		return
	}
	var req dto.UpdateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	provider, err := h.service.UpdateGlobal(c.Request.Context(), id, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, provider)
}

func (h *ProviderHandler) DeleteGlobal(c *gin.Context) {
	if h.denyPlatformCatalogWrite(c) {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid provider id")
		return
	}
	if err := h.service.DeleteGlobal(c.Request.Context(), id); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, nil)
}

// Tenant provider operations
func (h *ProviderHandler) ConfigureProvider(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	var req dto.ConfigureProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	config, err := h.service.ConfigureProvider(c.Request.Context(), organizationID, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, config)
}

func (h *ProviderHandler) CreateCustom(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	var req dto.CreateCustomProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	provider, err := h.service.CreateCustom(c.Request.Context(), organizationID, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, provider)
}

func (h *ProviderHandler) ListTenantProviders(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}

	// Parse pagination parameters
	page := 1
	limit := 1000
	if p := c.Query("page"); p != "" {
		if pInt, err := strconv.Atoi(p); err == nil && pInt > 0 {
			page = pInt
		}
	}
	if l := c.Query("limit"); l != "" {
		if lInt, err := strconv.Atoi(l); err == nil && lInt > 0 && lInt <= 100 {
			limit = lInt
		}
	}

	providers, err := h.service.ListTenantProviders(c.Request.Context(), organizationID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	// Calculate pagination
	total := int64(len(providers))
	hasMore := int64(page*limit) < total

	// Return paginated response matching legacy format
	response.Success(c, &dto.TenantProviderListResponse{
		Items:   providers,
		Total:   total,
		Page:    page,
		Limit:   limit,
		HasMore: hasMore,
	})
}

func (h *ProviderHandler) GetTenantProvider(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}

	providerIdentifier := c.Param("id")
	if providerIdentifier == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "provider id or name is required")
		return
	}

	provider, err := h.service.GetTenantProvider(c.Request.Context(), organizationID, providerIdentifier)
	if err != nil {
		response.FailWithMessage(c, response.ErrNotFound, err.Error())
		return
	}

	response.Success(c, provider)
}

func (h *ProviderHandler) GetCustom(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid provider id")
		return
	}
	provider, err := h.service.GetCustom(c.Request.Context(), organizationID, id)
	if err != nil {
		response.FailWithMessage(c, response.ErrNotFound, err.Error())
		return
	}
	response.Success(c, provider)
}

func (h *ProviderHandler) UpdateCustom(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid provider id")
		return
	}
	var req dto.UpdateCustomProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	provider, err := h.service.UpdateCustom(c.Request.Context(), organizationID, id, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, provider)
}

func (h *ProviderHandler) DeleteCustom(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid provider id")
		return
	}
	if err := h.service.DeleteCustom(c.Request.Context(), organizationID, id); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, nil)
}

func (h *ProviderHandler) GetProviderConfig(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	providerID, err := uuid.Parse(c.Param("provider_id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid provider id")
		return
	}
	config, err := h.service.GetProviderConfig(c.Request.Context(), organizationID, providerID)
	if err != nil {
		response.FailWithMessage(c, response.ErrNotFound, err.Error())
		return
	}
	response.Success(c, config)
}

func (h *ProviderHandler) ListProviderConfigs(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	var req dto.ListProviderRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 1000
	}
	configs, total, err := h.service.ListProviderConfigs(c.Request.Context(), organizationID, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, gin.H{"list": configs, "total": total, "page": req.Page, "page_size": req.PageSize})
}

func (h *ProviderHandler) ListCustomProviders(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	var req dto.ListProviderRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 1000
	}
	providers, total, err := h.service.ListCustom(c.Request.Context(), organizationID, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, gin.H{"list": providers, "total": total, "page": req.Page, "page_size": req.PageSize})
}

func getOrganizationID(c *gin.Context) (uuid.UUID, error) {
	tenantIDStr := c.GetString("organization_id")
	if tenantIDStr == "" {
		tenantIDStr = c.GetHeader("X-Organization-ID")
	}
	if tenantIDStr == "" {
		return uuid.Nil, ErrInvalidOrganizationID
	}
	return uuid.Parse(tenantIDStr)
}

// ToggleProvider enables or disables a provider for a tenant
// POST /providers/toggle
func (h *ProviderHandler) ToggleProvider(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	var req dto.ToggleProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	if err := h.service.ToggleProvider(c.Request.Context(), organizationID, req.Provider, req.IsEnabled); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Provider toggled successfully", "provider": req.Provider, "is_enabled": req.IsEnabled})
}

// GetProviderDetail gets detailed provider information with models
// GET /providers/:provider
func (h *ProviderHandler) GetProviderDetail(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	provider := c.Param("provider")
	if provider == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "provider is required")
		return
	}
	detail, err := h.service.GetProviderDetail(c.Request.Context(), organizationID, provider)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, detail)
}

// ToggleModel enables or disables a model for a tenant under a provider
// POST /providers/:provider/models/toggle
func (h *ProviderHandler) ToggleModel(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	provider := c.Param("provider")
	if provider == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "provider is required")
		return
	}
	var req dto.ToggleModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	// Support both "model" and "model_name" for backward compatibility
	modelName := req.Model
	if modelName == "" {
		modelName = req.ModelName
	}
	if modelName == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "model or model_name is required")
		return
	}

	if err := h.service.ToggleModel(c.Request.Context(), organizationID, provider, modelName, req.IsEnabled); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Model toggled successfully", "provider": provider, "model": modelName, "is_enabled": req.IsEnabled})
}
