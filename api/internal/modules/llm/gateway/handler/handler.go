// Package handler provides HTTP handlers for the LLM gateway.
package handler

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/zgiai/zgi/api/internal/observability"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/database"
)

// LLMHandler handles LLM API requests (OpenAI compatible)
type LLMHandler struct {
	gatewayService types.GatewayService
	errorProjector *apptransport.Projector
}

// geminiGatewayService is deliberately additive so existing GatewayService
// implementations and test doubles remain source-compatible.
type geminiGatewayService interface {
	CreateGeminiContent(context.Context, *apikeymodel.TenantAPIKey, *adapter.GeminiGenerateContentRequest) (*adapter.RawResponse, error)
	CreateGeminiContentStream(context.Context, *apikeymodel.TenantAPIKey, *adapter.GeminiGenerateContentRequest) (<-chan adapter.RawStreamEvent, error)
}

// NewLLMHandler creates a new LLM handler
func NewLLMHandler(gatewayService types.GatewayService, errorProjector *apptransport.Projector) *LLMHandler {
	if errorProjector == nil {
		panic("LLM handler requires application error projector")
	}
	return &LLMHandler{
		gatewayService: gatewayService,
		errorProjector: errorProjector,
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
		writeOpenAIProtocolError(c, invalidRequestProtocolError("Invalid JSON request body"))
		return
	}

	// 3. Validate required parameters
	if err := validatePublicChatRequest(&req); err != nil {
		writeOpenAIProtocolError(c, invalidRequestProtocolError(publicInputMessage(err)))
		return
	}

	// Publish the server-generated billing/audit ID without trusting or replacing
	// the caller's transport request ID. Set it before SSE headers are committed.
	c.Request = c.Request.WithContext(types.WithInvocationIDObserver(c.Request.Context(), func(id string) {
		c.Header(types.InvocationIDHeader, id)
	}))

	// 4. Check if streaming is requested
	if req.Stream {
		h.handleStreamingRequest(c, apiKey, &req)
		return
	}

	resp, err := h.gatewayService.ChatCompletion(gatewayRequestContext(c), apiKey, &req)
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
		writeOpenAIProtocolError(c, invalidRequestProtocolError("Invalid JSON request body"))
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
		writeOpenAIProtocolError(c, invalidRequestProtocolError(publicInputMessage(err)))
		return
	}

	if req.Model == "" {
		writeOpenAIProtocolError(c, invalidRequestProtocolError("Model field is required"))
		return
	}
	if err := validatePublicNativeBody(req.Body, "responses"); err != nil {
		writeOpenAIProtocolError(c, invalidRequestProtocolError(publicInputMessage(err)))
		return
	}

	if req.Stream {
		h.handleResponseStream(c, apiKey, req)
		return
	}

	resp, err := h.gatewayService.CreateResponseRaw(gatewayRequestContext(c), apiKey, req)
	if err != nil {
		recordServiceError(c, err)
		if writeRawProtocolError(c, err, req.Model, "openai") {
			return
		}
		writeOpenAIProtocolError(c, h.localizedProtocolError(c, err))
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
		writeAnthropicProtocolError(c, invalidRequestProtocolError(publicInputMessage(err)))
		return
	}
	if req.Model == "" {
		writeAnthropicProtocolError(c, invalidRequestProtocolError("Model field is required"))
		return
	}
	if err := validatePublicNativeBody(req.Body, "messages"); err != nil {
		writeAnthropicProtocolError(c, invalidRequestProtocolError(publicInputMessage(err)))
		return
	}

	if req.Stream {
		h.handleAnthropicMessageStream(c, apiKey, req)
		return
	}

	resp, err := h.gatewayService.CreateAnthropicMessage(gatewayRequestContext(c), apiKey, req)
	if err != nil {
		recordServiceError(c, err)
		if writeRawProtocolError(c, err, req.Model, "anthropic") {
			return
		}
		writeAnthropicProtocolError(c, h.localizedProtocolError(c, err))
		return
	}

	c.Data(http.StatusOK, "application/json", resp.Body)
}

// CreateGeminiContent handles both Gemini generateContent actions. The model
// and stream mode come from the native path; the JSON body is forwarded intact.
func (h *LLMHandler) CreateGeminiContent(c *gin.Context) {
	apiKey, ok := apiKeyFromContext(c)
	if !ok {
		return
	}
	req, err := parseGeminiGenerateContentRequest(c)
	if err != nil {
		writeGeminiProtocolError(c, invalidRequestProtocolError(publicInputMessage(err)))
		return
	}
	if err := validatePublicNativeBody(req.Body, "gemini"); err != nil {
		writeGeminiProtocolError(c, invalidRequestProtocolError(publicInputMessage(err)))
		return
	}
	if req.Stream {
		h.handleGeminiContentStream(c, apiKey, req)
		return
	}
	service, ok := h.gatewayService.(geminiGatewayService)
	if !ok {
		writeGeminiProtocolError(c, classifyProtocolError(adapter.ErrCapabilityUnsupported))
		return
	}
	resp, err := service.CreateGeminiContent(gatewayRequestContext(c), apiKey, req)
	if err != nil {
		recordServiceError(c, err)
		if writeRawProtocolError(c, err, req.Model, "gemini") {
			return
		}
		writeGeminiProtocolError(c, h.localizedProtocolError(c, err))
		return
	}
	c.Data(http.StatusOK, "application/json", resp.Body)
}

// handleStreamingRequest handles streaming chat completions
func (h *LLMHandler) handleStreamingRequest(c *gin.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.ChatRequest) {
	streamChan, err := h.gatewayService.ChatCompletionStream(gatewayRequestContext(c), apiKey, req)
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
			recordStreamServiceError(c, resp.Error)
			streamWriter.WriteErrorMessage(h.localizedChatStreamError(c, resp.Error))
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
	streamChan, err := h.gatewayService.CreateResponseStream(gatewayRequestContext(c), apiKey, req)
	if err != nil {
		recordServiceError(c, err)
		if writeRawProtocolError(c, err, req.Model, "openai") {
			return
		}
		writeOpenAIProtocolError(c, h.localizedProtocolError(c, err))
		return
	}

	streamWriter, err := llmshared.NewRawEventStreamWriter(c)
	if err != nil {
		recordServiceError(c, err)
		writeOpenAIProtocolError(c, internalProtocolError())
		return
	}

	for event := range streamChan {
		if event.Error != nil {
			recordStreamServiceError(c, event.Error)
			_ = streamWriter.WriteRawError(h.openAIStreamError(c, event.Error))
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
	streamChan, err := h.gatewayService.CreateAnthropicMessageStream(gatewayRequestContext(c), apiKey, req)
	if err != nil {
		recordServiceError(c, err)
		if writeRawProtocolError(c, err, req.Model, "anthropic") {
			return
		}
		writeAnthropicProtocolError(c, h.localizedProtocolError(c, err))
		return
	}

	streamWriter, err := llmshared.NewRawEventStreamWriter(c)
	if err != nil {
		recordServiceError(c, err)
		writeAnthropicProtocolError(c, internalProtocolError())
		return
	}

	for event := range streamChan {
		if event.Error != nil {
			recordStreamServiceError(c, event.Error)
			_ = streamWriter.WriteRawError(h.anthropicStreamError(c, event.Error))
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

func (h *LLMHandler) handleGeminiContentStream(c *gin.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.GeminiGenerateContentRequest) {
	service, ok := h.gatewayService.(geminiGatewayService)
	if !ok {
		writeGeminiProtocolError(c, classifyProtocolError(adapter.ErrCapabilityUnsupported))
		return
	}
	streamChan, err := service.CreateGeminiContentStream(gatewayRequestContext(c), apiKey, req)
	if err != nil {
		recordServiceError(c, err)
		if writeRawProtocolError(c, err, req.Model, "gemini") {
			return
		}
		writeGeminiProtocolError(c, h.localizedProtocolError(c, err))
		return
	}
	streamWriter, err := llmshared.NewRawEventStreamWriter(c)
	if err != nil {
		writeGeminiProtocolError(c, internalProtocolError())
		return
	}
	for event := range streamChan {
		if event.Error != nil {
			recordStreamServiceError(c, event.Error)
			_ = streamWriter.WriteEvent(adapter.RawStreamEvent{Data: geminiStreamErrorBody(h.localizedProtocolError(c, event.Error))})
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

	writeModelsCatalog(c, models)
}

// GetModel handles GET /v1/models/{model_id} for Anthropic and OpenAI clients.
func (h *LLMHandler) GetModel(c *gin.Context) {
	apiKey, ok := apiKeyFromContext(c)
	if !ok {
		return
	}
	models, err := h.gatewayService.ListAvailableModels(c.Request.Context(), apiKey)
	if err != nil {
		h.handleError(c, err)
		return
	}
	writeModelsDetail(c, models)
}

// handleError converts service errors to the public OpenAI-compatible contract.
func (h *LLMHandler) handleError(c *gin.Context, err error) {
	recordServiceError(c, err)
	writeOpenAIProtocolError(c, h.localizedProtocolError(c, err))
}

// recordServiceError preserves the concrete request failure for the outer
// reporter middleware. The reporter can then distinguish a later billing or
// gateway failure from an upstream error already reported during the request.
func recordServiceError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	err = normalizeGatewayApplicationError(err)
	ginErr := c.Error(err)
	ginErr.SetMeta(serviceFailureReportHint(err))
}

func serviceFailureReportHint(err error) observability.FailureReportHint {
	if gateway.IsClientIOError(err) {
		return observability.FailureReportHint{Suppress: true}
	}
	if gateway.IsProviderFailureReported(err) {
		return observability.FailureReportHint{Suppress: true}
	}
	if adapter.IsDeterministicRejection(err) {
		return observability.FailureReportHint{Suppress: true}
	}
	if gateway.IsPersistentChannelFailure(err) {
		// All Gateway provider paths emit the authoritative channel-aware event.
		// Suppress this owner-agnostic HTTP fallback for both tenant-managed and
		// platform-managed credential states.
		return observability.FailureReportHint{Suppress: true}
	}
	if isExpectedTenantLLMError(err) {
		return observability.FailureReportHint{Suppress: true}
	}

	if errors.Is(err, gateway.ErrNoProviderAvailable) || errors.Is(err, llmerrors.DomainErrNoProviderAvailable) {
		// Provider selection already emits a tenant/configuration diagnostic.
		// Suppress the status fallback so one request cannot produce a second,
		// conflicting provider-owned incident.
		return observability.FailureReportHint{Suppress: true}
	}
	if gateway.IsProviderSelectionPreparationError(err) {
		classification := database.ClassifyError(err)
		if classification.Source != observability.ErrorSourceInfrastructure {
			classification = observability.ErrorClassification{
				Category: observability.ErrorCategoryApplication,
				Source:   observability.ErrorSourceZGI,
				Code:     "provider_selection_preparation_failed",
			}
		}
		return observability.FailureReportHint{
			EventName:      "llm.request.failed",
			Classification: classification,
		}
	}
	if database.IsOperationError(err) {
		return observability.FailureReportHint{
			EventName:      "llm.request.failed",
			Classification: database.ClassifyError(err),
		}
	}
	protocolErr := classifyProtocolError(err)
	hint := observability.FailureReportHint{
		EventName: "llm.request.failed",
		Classification: observability.ErrorClassification{
			Category:  observability.ErrorCategoryDependency,
			Source:    observability.ErrorSourceProvider,
			Code:      "upstream_request_failed",
			Retryable: true,
		},
	}
	if protocolErr.openAIStatus == http.StatusInternalServerError {
		hint.Classification = observability.ErrorClassification{
			Category: observability.ErrorCategoryApplication,
			Source:   observability.ErrorSourceZGI,
			Code:     "request_processing_failed",
		}
	} else if protocolErr.openAIStatus == http.StatusGatewayTimeout {
		hint.Classification.Category = observability.ErrorCategoryTimeout
		hint.Classification.Code = "upstream_timeout"
	}
	return hint
}

func isExpectedTenantLLMError(err error) bool {
	var billingUserErr *gateway.BillingUserError
	if errors.As(err, &billingUserErr) && billingUserErr != nil {
		return true
	}
	return errors.Is(err, gateway.ErrInsufficientQuota) ||
		errors.Is(err, gateway.ErrInsufficientBalance) ||
		errors.Is(err, llmerrors.DomainErrInsufficientBalance) ||
		errors.Is(err, llmerrors.DomainErrRateLimitExceeded) ||
		errors.Is(err, adapter.ErrInsufficientBalance) ||
		errors.Is(err, adapter.ErrQuotaExhausted)
}

// recordStreamServiceError preserves both the concrete failure and an explicit
// reporting hint. Streaming responses have already flushed HTTP 200, so outer
// middleware cannot accurately infer whether the failure belongs to the
// provider, ZGI billing, or an expected deterministic rejection from status
// alone.
func recordStreamServiceError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	err = normalizeGatewayApplicationError(err)
	ginErr := c.Error(err)
	ginErr.SetMeta(streamFailureReportHint(err))
}

func streamFailureReportHint(err error) observability.FailureReportHint {
	if gateway.IsClientIOError(err) {
		return observability.FailureReportHint{Suppress: true}
	}
	if adapter.IsDeterministicRejection(err) {
		return observability.FailureReportHint{Suppress: true}
	}
	if gateway.IsPersistentChannelFailure(err) {
		return observability.FailureReportHint{Suppress: true}
	}
	if isExpectedTenantLLMError(err) {
		return observability.FailureReportHint{Suppress: true}
	}

	protocolErr := classifyProtocolError(err)
	hint := observability.FailureReportHint{
		EventName: "llm.stream.failed",
		Classification: observability.ErrorClassification{
			Category:  observability.ErrorCategoryDependency,
			Source:    observability.ErrorSourceProvider,
			Code:      "upstream_stream_failed",
			Retryable: true,
		},
	}
	if protocolErr.openAIStatus == http.StatusInternalServerError {
		hint.Classification = observability.ErrorClassification{
			Category:  observability.ErrorCategoryApplication,
			Source:    observability.ErrorSourceZGI,
			Code:      "stream_finalization_failed",
			Retryable: true,
		}
	} else if protocolErr.openAIStatus == http.StatusGatewayTimeout {
		hint.Classification.Category = observability.ErrorCategoryTimeout
		hint.Classification.Code = "upstream_stream_timeout"
	}
	return hint
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
		return nil, newPublicInputErrorf("invalid JSON request body")
	}
	var meta struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, newPublicInputErrorf("invalid JSON request body")
	}
	return &adapter.RawResponseRequest{
		Model:  strings.TrimSpace(meta.Model),
		Stream: meta.Stream,
		Body:   body,
	}, nil
}

func gatewayRequestContext(c *gin.Context) context.Context {
	ctx := c.Request.Context()
	if requestID := requestIDFromContext(c); requestID != "" {
		ctx = context.WithValue(ctx, llmshared.ContextKeyRequestID, requestID)
	}
	return ctx
}

func parseAnthropicMessageRequest(c *gin.Context) (*adapter.AnthropicMessageRequest, error) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, newPublicInputErrorf("invalid JSON request body")
	}
	var meta struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, newPublicInputErrorf("invalid JSON request body")
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

func parseGeminiGenerateContentRequest(c *gin.Context) (*adapter.GeminiGenerateContentRequest, error) {
	modelAction := strings.TrimPrefix(strings.TrimSpace(c.Param("modelAction")), "/")
	model, action, ok := strings.Cut(modelAction, ":")
	if !ok || strings.TrimSpace(model) == "" {
		return nil, newPublicInputErrorf("Gemini path must be /v1beta/models/{model}:generateContent")
	}
	stream := false
	switch action {
	case "generateContent":
	case "streamGenerateContent":
		stream = true
	default:
		return nil, newPublicInputErrorf("unsupported Gemini action %s", action)
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, newPublicInputErrorf("invalid JSON request body")
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		body = []byte(`{}`)
	}
	if !json.Valid(body) {
		return nil, newPublicInputErrorf("invalid JSON request body")
	}
	return &adapter.GeminiGenerateContentRequest{Model: strings.TrimSpace(model), Stream: stream, Body: body}, nil
}

func writeRawProtocolError(c *gin.Context, err error, logicalModel, protocol string) bool {
	var rawErr *adapter.RawProtocolError
	if !errors.As(err, &rawErr) || rawErr == nil {
		return false
	}
	status := publicRawProtocolStatus(rawErr.StatusCode)
	body := gateway.SanitizePublicNativeErrorBody(rawErr.Body, logicalModel)
	if len(body) == 0 {
		writeRawProtocolFallback(c, status, protocol)
		return true
	}
	contentType := strings.TrimSpace(rawErr.ContentType)
	if !strings.Contains(strings.ToLower(contentType), "json") {
		contentType = "application/json"
	}
	c.Data(status, contentType, body)
	return true
}

func publicRawProtocolStatus(status int) int {
	if status >= 400 && status <= 599 {
		return status
	}
	return http.StatusBadGateway
}

func writeRawProtocolFallback(c *gin.Context, status int, protocol string) {
	const message = "Upstream request failed"
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "anthropic":
		c.JSON(status, gin.H{"type": "error", "error": gin.H{"type": "api_error", "message": message}})
	case "gemini":
		c.JSON(status, gin.H{"error": gin.H{"code": status, "message": message, "status": geminiErrorStatus(status)}})
	default:
		c.JSON(status, gin.H{"error": gin.H{"message": message, "type": "api_error", "code": "upstream_error", "param": nil}})
	}
}

func geminiStreamErrorBody(protocolErr protocolError) json.RawMessage {
	body, _ := json.Marshal(gin.H{"error": gin.H{"code": protocolErr.openAIStatus, "message": protocolErr.message, "status": geminiErrorStatus(protocolErr.openAIStatus)}})
	return body
}

func (h *LLMHandler) openAIStreamError(c *gin.Context, err error) []byte {
	protocolErr := h.localizedProtocolError(c, err)
	data, _ := json.Marshal(gin.H{
		"type":    "error",
		"message": protocolErr.message,
		"code":    protocolErr.openAICode,
		"param":   nil,
	})
	return data
}

func (h *LLMHandler) anthropicStreamError(c *gin.Context, err error) []byte {
	protocolErr := h.localizedProtocolError(c, err)
	data, _ := json.Marshal(gin.H{
		"type": "error",
		"error": gin.H{
			"type":    protocolErr.anthropicType,
			"message": protocolErr.message,
		},
	})
	return data
}
