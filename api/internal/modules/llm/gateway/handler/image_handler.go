package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// CreateImage handles POST /v1/images/generations
func (h *LLMHandler) CreateImage(c *gin.Context) {
	apiKey, ok := apiKeyFromContext(c)
	if !ok {
		return
	}

	// 2. Parse request
	var req adapter.ImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeOpenAIProtocolError(c, invalidRequestProtocolError(fmt.Sprintf("Invalid request: %v", err)))
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
