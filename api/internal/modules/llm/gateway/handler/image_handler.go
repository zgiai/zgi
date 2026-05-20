package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	apikeymodel "github.com/zgiai/ginext/internal/modules/llm/apikey/model"
	"github.com/zgiai/ginext/internal/modules/llm/gateway/types"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/ginext/pkg/response"
)

// CreateImage handles POST /v1/images/generations
func (h *LLMHandler) CreateImage(c *gin.Context) {
	// 1. Get API key from context (set by middleware)
	apiKeyInterface, exists := c.Get("llm_api_key")
	if !exists {
		response.Fail(c, response.ErrorCode{
			Code:        types.ErrCodeInvalidAPIKey.Code,
			Message:     "API key not found in context",
			UserVisible: true,
		})
		return
	}

	apiKey, ok := apiKeyInterface.(*apikeymodel.TenantAPIKey)
	if !ok {
		response.Fail(c, response.ErrorCode{
			Code:        types.ErrCodeInvalidAPIKey.Code,
			Message:     "Invalid API key type",
			UserVisible: true,
		})
		return
	}

	// 2. Parse request
	var req adapter.ImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrorCode{
			Code:        types.ErrCodeInvalidRequest.Code,
			Message:     fmt.Sprintf("Invalid request: %v", err),
			UserVisible: true,
		})
		return
	}

	resp, err := h.gatewayService.CreateImage(c.Request.Context(), apiKey, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// 4. Return response
	c.JSON(http.StatusOK, resp)
}
