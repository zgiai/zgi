package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/modules/llm/client"
	llmerrors "github.com/zgiai/ginext/internal/modules/llm/errors"
	"github.com/zgiai/ginext/internal/modules/llm/gateway"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	llmshared "github.com/zgiai/ginext/internal/modules/llm/shared"
	"github.com/zgiai/ginext/pkg/response"
)

// InternalHandler handles internal LLM requests (authenticated via JWT)
type InternalHandler struct {
	client client.LLMClient
}

// NewInternalHandler creates a new internal LLM handler
func NewInternalHandler(llmClient client.LLMClient) *InternalHandler {
	return &InternalHandler{
		client: llmClient,
	}
}

// NewLLMInternalHandler is an alias for NewInternalHandler (deprecated)
func NewLLMInternalHandler(llmClient client.LLMClient) *InternalHandler {
	return NewInternalHandler(llmClient)
}

// ChatCompletions handles POST /chat/completions
func (h *InternalHandler) ChatCompletions(c *gin.Context) {
	// 1. Get Tenant ID from context (set by middleware)
	organizationID := c.GetString("organization_id")
	if organizationID == "" {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	// 2. Parse request
	var req adapter.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrorCode{
			Code:        gateway.ErrCodeInvalidRequest.Code,
			Message:     fmt.Sprintf("Invalid request: %v", err),
			UserVisible: true,
		})
		return
	}

	// 3. Check if streaming is requested
	if req.Stream {
		h.handleStreamingRequest(c, organizationID, &req)
		return
	}

	// 4. Handle non-streaming request
	resp, err := h.client.Chat(c.Request.Context(), organizationID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// 5. Return response
	c.JSON(http.StatusOK, resp)
}

// CreateResponse handles POST /responses
func (h *InternalHandler) CreateResponse(c *gin.Context) {
	// 1. Get Tenant ID from context
	organizationID := c.GetString("organization_id")
	if organizationID == "" {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	// 2. Parse request
	var req adapter.CreateResponseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrorCode{
			Code:        gateway.ErrCodeInvalidRequest.Code,
			Message:     fmt.Sprintf("Invalid request: %v", err),
			UserVisible: true,
		})
		return
	}

	// 3. Handle request
	resp, err := h.client.CreateResponse(c.Request.Context(), organizationID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// 4. Return response
	c.JSON(http.StatusOK, resp)
}

// Embeddings handles POST /embeddings
func (h *InternalHandler) Embeddings(c *gin.Context) {
	// 1. Get Tenant ID from context
	organizationID := c.GetString("organization_id")
	if organizationID == "" {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	// 2. Parse request
	var req adapter.EmbeddingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrorCode{
			Code:        gateway.ErrCodeInvalidRequest.Code,
			Message:     fmt.Sprintf("Invalid request: %v", err),
			UserVisible: true,
		})
		return
	}

	// 3. Handle request
	resp, err := h.client.Embed(c.Request.Context(), organizationID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// 4. Return response
	c.JSON(http.StatusOK, resp)
}

// Rerank handles POST /rerank
func (h *InternalHandler) Rerank(c *gin.Context) {
	// 1. Get Tenant ID from context
	organizationID := c.GetString("organization_id")
	if organizationID == "" {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	// 2. Parse request
	var req adapter.RerankRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrorCode{
			Code:        gateway.ErrCodeInvalidRequest.Code,
			Message:     fmt.Sprintf("Invalid request: %v", err),
			UserVisible: true,
		})
		return
	}

	// 3. Handle request
	resp, err := h.client.Rerank(c.Request.Context(), organizationID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// 4. Return response
	c.JSON(http.StatusOK, resp)
}

// handleStreamingRequest handles streaming chat completions
func (h *InternalHandler) handleStreamingRequest(c *gin.Context, organizationID string, req *adapter.ChatRequest) {
	// Get stream channel
	streamChan, err := h.client.ChatStream(c.Request.Context(), organizationID, req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	streamWriter, err := llmshared.NewStreamWriter(c)
	if err != nil {
		h.handleError(c, err)
		return
	}

	sentUsage := false
	for resp := range streamChan {
		// Check for errors
		if resp.Error != nil {
			streamWriter.WriteError(resp.Error)
			break
		}

		if resp.Done {
			if resp.Usage != nil && !sentUsage {
				streamWriter.WriteFinalUsage(resp, req.Model)
			}
			streamWriter.WriteDone()
			break
		}

		streamWriter.WriteObject(resp)
		if resp.Usage != nil {
			sentUsage = true
		}
	}
}

// handleError converts service layer errors to HTTP responses using standardized error codes
func (h *InternalHandler) handleError(c *gin.Context, err error) {
	// Use the centralized error handler from llmerrors package
	llmerrors.HandleServiceError(c, err)
}
