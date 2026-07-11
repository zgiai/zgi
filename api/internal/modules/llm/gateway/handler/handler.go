// Package handler provides HTTP handlers for the LLM gateway.
package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/types"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	llmshared "github.com/zgiai/zgi/api/internal/modules/llm/shared"
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
	apiKey, ok := apiKeyFromContext(c)
	if !ok {
		return
	}

	// 2. Parse request
	var req adapter.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeOpenAIProtocolError(c, invalidRequestProtocolError(fmt.Sprintf("Invalid request: %v", err)))
		return
	}

	// 3. Validate required parameters
	if req.Model == "" {
		writeOpenAIProtocolError(c, invalidRequestProtocolError("Model field is required"))
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
	apiKey, ok := apiKeyFromContext(c)
	if !ok {
		return
	}

	// 2. Parse request
	var req adapter.EmbeddingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeOpenAIProtocolError(c, invalidRequestProtocolError(fmt.Sprintf("Invalid request: %v", err)))
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
		writeOpenAIProtocolError(c, invalidRequestProtocolError(err.Error()))
		return
	}

	if req.Model == "" {
		writeOpenAIProtocolError(c, invalidRequestProtocolError("Model field is required"))
		return
	}

	if req.Stream {
		h.handleResponseStream(c, apiKey, req)
		return
	}

	resp, err := h.gatewayService.CreateResponseRaw(c.Request.Context(), apiKey, req)
	if err != nil {
		writeOpenAIProtocolError(c, classifyProtocolError(err))
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
		writeAnthropicProtocolError(c, invalidRequestProtocolError(err.Error()))
		return
	}
	if req.Model == "" {
		writeAnthropicProtocolError(c, invalidRequestProtocolError("Model field is required"))
		return
	}

	if req.Stream {
		h.handleAnthropicMessageStream(c, apiKey, req)
		return
	}

	resp, err := h.gatewayService.CreateAnthropicMessage(c.Request.Context(), apiKey, req)
	if err != nil {
		writeAnthropicProtocolError(c, classifyProtocolError(err))
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
		writeOpenAIProtocolError(c, classifyProtocolError(err))
		return
	}

	streamWriter, err := llmshared.NewRawEventStreamWriter(c)
	if err != nil {
		writeOpenAIProtocolError(c, internalProtocolError())
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
		writeAnthropicProtocolError(c, classifyProtocolError(err))
		return
	}

	streamWriter, err := llmshared.NewRawEventStreamWriter(c)
	if err != nil {
		writeAnthropicProtocolError(c, internalProtocolError())
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
	apiKey, ok := apiKeyFromContext(c)
	if !ok {
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

// handleError converts service errors to the public OpenAI-compatible contract.
func (h *LLMHandler) handleError(c *gin.Context, err error) {
	writeOpenAIProtocolError(c, classifyProtocolError(err))
}

func apiKeyFromContext(c *gin.Context) (*apikeymodel.TenantAPIKey, bool) {
	apiKeyInterface, exists := c.Get("llm_api_key")
	if !exists {
		writeProtocolError(c, invalidAPIKeyProtocolError("API key not found in context"))
		return nil, false
	}

	apiKey, ok := apiKeyInterface.(*apikeymodel.TenantAPIKey)
	if !ok {
		writeProtocolError(c, invalidAPIKeyProtocolError("Invalid API key"))
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

func openAIStreamError(err error) []byte {
	protocolErr := classifyProtocolError(err)
	data, _ := json.Marshal(gin.H{
		"type":    "error",
		"message": protocolErr.message,
		"code":    protocolErr.openAICode,
		"param":   nil,
	})
	return data
}

func anthropicStreamError(err error) []byte {
	protocolErr := classifyProtocolError(err)
	data, _ := json.Marshal(gin.H{
		"type": "error",
		"error": gin.H{
			"type":    protocolErr.anthropicType,
			"message": protocolErr.message,
		},
	})
	return data
}
