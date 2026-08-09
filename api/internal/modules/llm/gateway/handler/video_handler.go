package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// CreateVideo handles POST /v1/videos/generations.
func (h *LLMHandler) CreateVideo(c *gin.Context) {
	apiKey, ok := apiKeyFromContext(c)
	if !ok {
		return
	}
	var req adapter.VideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeOpenAIProtocolError(c, invalidRequestProtocolError(fmt.Sprintf("Invalid request: %v", err)))
		return
	}
	videoGateway, ok := h.gatewayService.(interface {
		CreateVideo(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.VideoRequest) (*adapter.VideoResponse, error)
	})
	if !ok {
		h.handleError(c, fmt.Errorf("%w: video generation is not enabled", adapter.ErrCapabilityUnsupported))
		return
	}
	resp, err := videoGateway.CreateVideo(c.Request.Context(), apiKey, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// GetVideoTask handles GET /v1/videos/generations/:task_id.
func (h *LLMHandler) GetVideoTask(c *gin.Context) {
	apiKey, ok := apiKeyFromContext(c)
	if !ok {
		return
	}
	taskID := strings.TrimPrefix(c.Param("task_id"), "/")
	req := adapter.VideoTaskRequest{
		TaskID: strings.TrimSpace(taskID),
		Model:  strings.TrimSpace(c.Query("model")),
	}
	if req.Model == "" {
		writeOpenAIProtocolError(c, invalidRequestProtocolError("model is required"))
		return
	}
	videoGateway, ok := h.gatewayService.(interface {
		GetVideoTask(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.VideoTaskRequest) (*adapter.VideoResponse, error)
	})
	if !ok {
		h.handleError(c, fmt.Errorf("%w: video task query is not enabled", adapter.ErrCapabilityUnsupported))
		return
	}
	resp, err := videoGateway.GetVideoTask(c.Request.Context(), apiKey, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}
