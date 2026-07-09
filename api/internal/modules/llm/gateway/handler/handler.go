// Package handler provides HTTP handlers for the LLM gateway.
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/types"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	llmshared "github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"github.com/zgiai/zgi/api/pkg/response"
)

// LLMHandler handles LLM API requests (OpenAI compatible)
type LLMHandler struct {
	gatewayService types.GatewayService
}

// NewLLMHandler creates a new LLM handler
func NewLLMHandler(gatewayService types.GatewayService) *LLMHandler {
	return &LLMHandler{
		gatewayService: gatewayService,
	}
}

// ChatCompletions handles POST /v1/chat/completions
func (h *LLMHandler) ChatCompletions(c *gin.Context) {
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
	var req adapter.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrorCode{
			Code:        types.ErrCodeInvalidRequest.Code,
			Message:     fmt.Sprintf("Invalid request: %v", err),
			UserVisible: true,
		})
		return
	}

	// 3. Validate required parameters
	if req.Model == "" {
		response.Fail(c, llmerrors.ErrMissingModel)
		return
	}

	// 4. Check if streaming is requested
	if req.Stream {
		h.handleStreamingRequest(c, apiKey, &req)
		return
	}

	resp, err := h.gatewayService.ChatCompletion(c.Request.Context(), apiKey, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// 5. Return response
	c.JSON(http.StatusOK, resp)
}

// Embeddings handles POST /v1/embeddings
func (h *LLMHandler) Embeddings(c *gin.Context) {
	// 1. Get API key from context
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
	var req adapter.EmbeddingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrorCode{
			Code:        types.ErrCodeInvalidRequest.Code,
			Message:     fmt.Sprintf("Invalid request: %v", err),
			UserVisible: true,
		})
		return
	}

	resp, err := h.gatewayService.CreateEmbeddings(c.Request.Context(), apiKey, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// 4. Return response
	c.JSON(http.StatusOK, resp)
}

// CreateResponse handles POST /v1/responses
func (h *LLMHandler) CreateResponse(c *gin.Context) {
	apiKey, ok := apiKeyFromContext(c)
	if !ok {
		return
	}

	req, err := parseRawResponseRequest(c)
	if err != nil {
		writeOpenAIProtocolError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if req.Model == "" {
		writeOpenAIProtocolError(c, http.StatusBadRequest, "invalid_request", "model field is required")
		return
	}

	if req.Stream {
		h.handleResponseStream(c, apiKey, req)
		return
	}

	resp, err := h.gatewayService.CreateResponseRaw(c.Request.Context(), apiKey, req)
	if err != nil {
		writeOpenAIProtocolError(c, protocolStatusFromError(err), "gateway_error", err.Error())
		return
	}

	c.Data(http.StatusOK, "application/json", resp.Body)
}

// CreateAnthropicMessage handles Anthropic Messages requests.
func (h *LLMHandler) CreateAnthropicMessage(c *gin.Context) {
	apiKey, ok := apiKeyFromContext(c)
	if !ok {
		return
	}

	req, err := parseAnthropicMessageRequest(c)
	if err != nil {
		writeAnthropicProtocolError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if req.Model == "" {
		writeAnthropicProtocolError(c, http.StatusBadRequest, "invalid_request_error", "model field is required")
		return
	}

	if req.Stream {
		h.handleAnthropicMessageStream(c, apiKey, req)
		return
	}

	resp, err := h.gatewayService.CreateAnthropicMessage(c.Request.Context(), apiKey, req)
	if err != nil {
		writeAnthropicProtocolError(c, protocolStatusFromError(err), "api_error", err.Error())
		return
	}

	c.Data(http.StatusOK, "application/json", resp.Body)
}

// handleStreamingRequest handles streaming chat completions
func (h *LLMHandler) handleStreamingRequest(c *gin.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.ChatRequest) {
	streamChan, err := h.gatewayService.ChatCompletionStream(c.Request.Context(), apiKey, req)
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

func (h *LLMHandler) handleResponseStream(c *gin.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.RawResponseRequest) {
	streamChan, err := h.gatewayService.CreateResponseStream(c.Request.Context(), apiKey, req)
	if err != nil {
		writeOpenAIProtocolError(c, protocolStatusFromError(err), "gateway_error", err.Error())
		return
	}

	streamWriter, err := llmshared.NewRawEventStreamWriter(c)
	if err != nil {
		writeOpenAIProtocolError(c, http.StatusInternalServerError, "gateway_error", err.Error())
		return
	}

	for event := range streamChan {
		if event.Error != nil {
			_ = streamWriter.WriteRawError(openAIStreamError(event.Error))
			return
		}
		if event.Done {
			return
		}
		if err := streamWriter.WriteEvent(event); err != nil {
			return
		}
	}
}

func (h *LLMHandler) handleAnthropicMessageStream(c *gin.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.AnthropicMessageRequest) {
	streamChan, err := h.gatewayService.CreateAnthropicMessageStream(c.Request.Context(), apiKey, req)
	if err != nil {
		writeAnthropicProtocolError(c, protocolStatusFromError(err), "api_error", err.Error())
		return
	}

	streamWriter, err := llmshared.NewRawEventStreamWriter(c)
	if err != nil {
		writeAnthropicProtocolError(c, http.StatusInternalServerError, "api_error", err.Error())
		return
	}

	for event := range streamChan {
		if event.Error != nil {
			_ = streamWriter.WriteRawError(anthropicStreamError(event.Error))
			return
		}
		if event.Done {
			return
		}
		if err := streamWriter.WriteEvent(event); err != nil {
			return
		}
	}
}

// ListModels handles GET /v1/models
func (h *LLMHandler) ListModels(c *gin.Context) {
	// 1. Get API key from context
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

	// 2. Get available models
	models, err := h.gatewayService.ListAvailableModels(c.Request.Context(), apiKey)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// 3. Return OpenAI-compatible response
	modelList := make([]gin.H, 0, len(models))
	for _, m := range models {
		modelList = append(modelList, gin.H{
			"id":       m.ID,
			"object":   "model",
			"created":  m.Created,
			"owned_by": m.OwnedBy,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   modelList,
	})
}

// handleError converts service layer errors to HTTP responses using standardized error codes
func (h *LLMHandler) handleError(c *gin.Context, err error) {
	// Use the centralized error handler from llmerrors package
	// This automatically maps domain errors to correct HTTP status codes
	llmerrors.HandleServiceError(c, err)
}

func apiKeyFromContext(c *gin.Context) (*apikeymodel.TenantAPIKey, bool) {
	apiKeyInterface, exists := c.Get("llm_api_key")
	if !exists {
		response.Fail(c, response.ErrorCode{
			Code:        types.ErrCodeInvalidAPIKey.Code,
			Message:     "API key not found in context",
			UserVisible: true,
		})
		return nil, false
	}

	apiKey, ok := apiKeyInterface.(*apikeymodel.TenantAPIKey)
	if !ok {
		response.Fail(c, response.ErrorCode{
			Code:        types.ErrCodeInvalidAPIKey.Code,
			Message:     "Invalid API key type",
			UserVisible: true,
		})
		return nil, false
	}
	return apiKey, true
}

func parseRawResponseRequest(c *gin.Context) (*adapter.RawResponseRequest, error) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	var meta struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return &adapter.RawResponseRequest{
		Model:  strings.TrimSpace(meta.Model),
		Stream: meta.Stream,
		Body:   body,
	}, nil
}

func parseAnthropicMessageRequest(c *gin.Context) (*adapter.AnthropicMessageRequest, error) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	var meta struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	headers := map[string]string{}
	for _, key := range []string{"anthropic-version", "anthropic-beta"} {
		if value := strings.TrimSpace(c.GetHeader(key)); value != "" {
			headers[key] = value
		}
	}
	return &adapter.AnthropicMessageRequest{
		Model:   strings.TrimSpace(meta.Model),
		Stream:  meta.Stream,
		Body:    body,
		Headers: headers,
	}, nil
}

func protocolStatusFromError(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case adapter.IsCapabilityUnsupported(err):
		return http.StatusBadRequest
	case errors.Is(err, gateway.ErrModelNotAuthorized):
		return http.StatusForbidden
	case errors.Is(err, gateway.ErrMissingModel), errors.Is(err, adapter.ErrInvalidRequest):
		return http.StatusBadRequest
	case errors.Is(err, gateway.ErrNoProviderAvailable), strings.Contains(err.Error(), "no provider available"):
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadGateway
	}
}

func writeOpenAIProtocolError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "invalid_request_error",
			"code":    code,
		},
	})
}

func writeAnthropicProtocolError(c *gin.Context, status int, errorType string, message string) {
	c.JSON(status, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errorType,
			"message": message,
		},
	})
}

func openAIStreamError(err error) []byte {
	data, _ := json.Marshal(gin.H{
		"type": "error",
		"error": gin.H{
			"message": err.Error(),
			"type":    "gateway_error",
			"code":    "gateway_error",
		},
	})
	return data
}

func anthropicStreamError(err error) []byte {
	data, _ := json.Marshal(gin.H{
		"type": "error",
		"error": gin.H{
			"type":    "api_error",
			"message": err.Error(),
		},
	})
	return data
}
