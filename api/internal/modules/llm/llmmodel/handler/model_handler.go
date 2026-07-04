package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

type ModelHandler struct {
	service                 service.ModelService
	platformCatalogReadOnly bool
}

func NewModelHandler(svc service.ModelService) *ModelHandler {
	return &ModelHandler{service: svc}
}

func (h *ModelHandler) SetPlatformCatalogReadOnly(readOnly bool) {
	h.platformCatalogReadOnly = readOnly
}

func (h *ModelHandler) denyPlatformCatalogWrite(c *gin.Context) bool {
	if !h.platformCatalogReadOnly {
		return false
	}

	response.FailWithMessage(c, response.ErrActionNotAllowed, "当前模式下平台模型目录不可写")
	return true
}

func (h *ModelHandler) CreateGlobal(c *gin.Context) {
	if h.denyPlatformCatalogWrite(c) {
		return
	}
	var req dto.CreateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	model, err := h.service.CreateGlobal(c.Request.Context(), &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, model)
}

func (h *ModelHandler) GetGlobal(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid model id")
		return
	}
	model, err := h.service.GetGlobal(c.Request.Context(), id)
	if err != nil {
		response.FailWithMessage(c, response.ErrNotFound, err.Error())
		return
	}
	response.Success(c, model)
}

func (h *ModelHandler) ListGlobal(c *gin.Context) {
	var req dto.ListModelRequest
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
	if req.PageSize > 1000 {
		req.PageSize = 1000
	}
	models, total, err := h.service.ListGlobal(c.Request.Context(), &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, gin.H{"list": models, "total": total, "page": req.Page, "page_size": req.PageSize})
}

func (h *ModelHandler) UpdateGlobal(c *gin.Context) {
	if h.denyPlatformCatalogWrite(c) {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid model id")
		return
	}
	var req dto.UpdateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	model, err := h.service.UpdateGlobal(c.Request.Context(), id, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, model)
}

func (h *ModelHandler) DeleteGlobal(c *gin.Context) {
	if h.denyPlatformCatalogWrite(c) {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid model id")
		return
	}
	if err := h.service.DeleteGlobal(c.Request.Context(), id); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, nil)
}

func (h *ModelHandler) ConfigureModel(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	var req dto.ConfigureModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	config, err := h.service.ConfigureModel(c.Request.Context(), organizationID, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, config)
}

func (h *ModelHandler) CreateCustom(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	var req dto.CreateCustomModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	model, err := h.service.CreateCustom(c.Request.Context(), organizationID, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, model)
}

func (h *ModelHandler) ListTenantModels(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}

	if c.Query("type") != "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "query param 'type' is deprecated; use 'use_case' instead")
		return
	}

	// Parse query parameters
	useCase := c.Query("use_case")
	provider := c.Query("provider")
	status := c.DefaultQuery("status", "active")
	if status != "active" && status != "deprecated" {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid status")
		return
	}
	isEnabledStr := c.Query("is_enabled")

	// Parse pagination parameters
	page := 1
	pageSize := 1000
	if p := c.Query("page"); p != "" {
		if pInt, err := strconv.Atoi(p); err == nil && pInt > 0 {
			page = pInt
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if psInt, err := strconv.Atoi(ps); err == nil && psInt > 0 && psInt <= 1000 {
			pageSize = psInt
		}
	}

	models, err := h.service.ListTenantModels(c.Request.Context(), organizationID, useCase, provider, status)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	// Filter by is_enabled if specified
	if isEnabledStr != "" {
		isEnabled := isEnabledStr == "true"
		filtered := make([]*model.ModelView, 0)
		for _, m := range models {
			if m.IsEnabled == isEnabled {
				filtered = append(filtered, m)
			}
		}
		models = filtered
	}

	// Calculate pagination
	total := int64(len(models))
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	start := (page - 1) * pageSize
	end := start + pageSize
	if start > len(models) {
		start = len(models)
	}
	if end > len(models) {
		end = len(models)
	}
	pagedModels := models[start:end]

	response.Success(c, &dto.TenantModelListResponse{
		Items:      pagedModels,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}

func (h *ModelHandler) GetCustom(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid model id")
		return
	}
	model, err := h.service.GetCustom(c.Request.Context(), organizationID, id)
	if err != nil {
		response.FailWithMessage(c, response.ErrNotFound, err.Error())
		return
	}
	response.Success(c, model)
}

func (h *ModelHandler) UpdateCustom(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid model id")
		return
	}
	var req dto.UpdateCustomModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	model, err := h.service.UpdateCustom(c.Request.Context(), organizationID, id, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, model)
}

func (h *ModelHandler) DeleteCustom(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid model id")
		return
	}
	if err := h.service.DeleteCustom(c.Request.Context(), organizationID, id); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, nil)
}

// GetModelConfig gets a tenant's configuration for a specific model
func (h *ModelHandler) GetModelConfig(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	modelID, err := uuid.Parse(c.Param("model_id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid model id")
		return
	}
	config, err := h.service.GetModelConfig(c.Request.Context(), organizationID, modelID)
	if err != nil {
		response.FailWithMessage(c, response.ErrNotFound, err.Error())
		return
	}
	response.Success(c, config)
}

// ListModelConfigs lists all model configurations for a tenant
func (h *ModelHandler) ListModelConfigs(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	var req dto.ListModelRequest
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
	if req.PageSize > 1000 {
		req.PageSize = 1000
	}
	configs, total, err := h.service.ListModelConfigs(c.Request.Context(), organizationID, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, gin.H{"list": configs, "total": total, "page": req.Page, "page_size": req.PageSize})
}

// ListCustomModels lists all custom models for a tenant
func (h *ModelHandler) ListCustomModels(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	var req dto.ListModelRequest
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
	if req.PageSize > 1000 {
		req.PageSize = 1000
	}
	models, total, err := h.service.ListCustom(c.Request.Context(), organizationID, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, gin.H{"list": models, "total": total, "page": req.Page, "page_size": req.PageSize})
}

// GetModelParameters handles GET /llm/models/parameters - get model parameters
func (h *ModelHandler) GetModelParameters(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}

	var req dto.LLMModelParametersRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	parameters, err := h.service.GetModelParameters(c.Request.Context(), organizationID, req.Provider, req.Model)
	if err != nil {
		if err == service.ErrModelNotFound {
			response.FailWithMessage(c, response.ErrNotFound, err.Error())
			return
		}
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, parameters)
}

// ToggleProviderModels handles POST /llm/models/provider/toggle - toggle all models for a provider
func (h *ModelHandler) ToggleProviderModels(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}

	var req dto.ToggleProviderModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	if err := h.service.ToggleProviderModels(c.Request.Context(), organizationID, req.Provider, req.IsEnabled); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, nil)
}

// BatchToggleModels handles POST /llm/models/batch/toggle - batch toggle specific models
func (h *ModelHandler) BatchToggleModels(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}

	var req dto.BatchToggleModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	// Convert model names to IDs
	var modelIDs []uuid.UUID
	for _, modelName := range req.Models {
		// Query model by provider and name
		models, _, err := h.service.ListGlobal(c.Request.Context(), &dto.ListModelRequest{
			Provider: req.Provider,
			Page:     1,
			PageSize: 1000,
		})
		if err != nil {
			response.FailWithMessage(c, response.ErrSystemError, err.Error())
			return
		}

		for _, m := range models {
			if m.Model == modelName {
				modelIDs = append(modelIDs, m.ID)
				break
			}
		}
	}

	if err := h.service.BatchToggleModels(c.Request.Context(), organizationID, modelIDs, req.IsEnabled); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, nil)
}

func getOrganizationID(c *gin.Context) (uuid.UUID, error) {
	tenantIDStr := c.GetString("organization_id")
	if tenantIDStr == "" {
		tenantIDStr = c.GetHeader("X-Organization-ID")
	}
	if tenantIDStr == "" {
		return uuid.Nil, service.ErrModelNotFound
	}
	return uuid.Parse(tenantIDStr)
}

// ListOfficialModels returns models provided by active system channels
func (h *ModelHandler) ListOfficialModels(c *gin.Context) {
	models, err := h.service.ListOfficialModels(c.Request.Context())
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, gin.H{
		"items": models,
		"total": len(models),
	})
}

// CheckAvailability handles GET /llm/models/:id/availability
func (h *ModelHandler) CheckAvailability(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrUnauthorized, "tenant context missing")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid model id")
		return
	}

	result, err := h.service.CheckAvailability(c.Request.Context(), organizationID, id)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// BatchCheckAvailability handles POST /llm/models/availability/batch
func (h *ModelHandler) BatchCheckAvailability(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrUnauthorized, "tenant context missing")
		return
	}

	var req dto.BatchModelAvailabilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	result, err := h.service.BatchCheckAvailability(c.Request.Context(), organizationID, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}
