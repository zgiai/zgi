package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

// AvailableModelsHandler handles available models API requests
type AvailableModelsHandler struct {
	service service.AvailableModelsService
}

type availableModelsJSONService interface {
	ListAvailableJSON(ctx context.Context, organizationID uuid.UUID, provider string, useCase string) ([]byte, error)
}

// NewAvailableModelsHandler creates a new available models handler
func NewAvailableModelsHandler(svc service.AvailableModelsService) *AvailableModelsHandler {
	return &AvailableModelsHandler{
		service: svc,
	}
}

// ListAvailableRequest represents the request for listing available models
type ListAvailableRequest struct {
	Provider string `form:"provider"`
	UseCase  string `form:"use_case"` // Filter by use case (e.g., vision, embedding, text-chat)
}

// ListAvailableResponse represents the response for listing available models
type ListAvailableResponse struct {
	Items []*service.AvailableModel `json:"items"`
	Total int                       `json:"total"`
}

// ListAvailable handles GET /llm/models/available
// @Summary List available models for business use
// @Description Returns a simplified list of available models optimized for business scenarios like workflow, agent, knowledge base
// @Tags LLM Models
// @Accept json
// @Produce json
// @Param provider query string false "Filter by provider name"
// @Param use_case query string false "Filter by use case (vision, embedding, text-chat, function-calling, etc.)"
// @Success 200 {object} ListAvailableResponse
// @Router /llm/models/available [get]
func (h *AvailableModelsHandler) ListAvailable(c *gin.Context) {
	// Get tenant ID from context
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}

	// Strict: type-based filtering is deprecated and not supported by this endpoint.
	if c.Query("type") != "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "query param 'type' is deprecated; use 'use_case' instead")
		return
	}

	// Parse request
	var req ListAvailableRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	if req.UseCase != "" && !isValidUseCase(req.UseCase) {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid use_case")
		return
	}

	if jsonSvc, ok := h.service.(availableModelsJSONService); ok {
		body, err := jsonSvc.ListAvailableJSON(c.Request.Context(), organizationID, req.Provider, req.UseCase)
		if err != nil {
			response.FailWithMessage(c, response.ErrSystemError, err.Error())
			return
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", body)
		return
	}

	// Get available models (now with use_case support)
	models, err := h.service.ListAvailable(c.Request.Context(), organizationID, req.Provider, req.UseCase)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, &ListAvailableResponse{
		Items: models,
		Total: len(models),
	})
}

func isValidUseCase(v string) bool {
	for _, uc := range llmmodel.ValidUseCases() {
		if string(uc) == v {
			return true
		}
	}
	return false
}

// RefreshCache handles POST /llm/models/available/refresh
// @Summary Refresh available models cache
// @Description Forces a cache refresh for the current tenant
// @Tags LLM Models
// @Accept json
// @Produce json
// @Success 200 {object} response.Response
// @Router /llm/models/available/refresh [post]
func (h *AvailableModelsHandler) RefreshCache(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}

	if err := h.service.RefreshCache(c.Request.Context(), organizationID); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, gin.H{"message": "cache refreshed"})
}

// Note: getOrganizationID is defined in model_handler.go and shared across handlers in this package
