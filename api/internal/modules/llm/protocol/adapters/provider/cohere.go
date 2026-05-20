package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

// CohereAdapter Cohere adapter
type CohereAdapter struct {
	config     *adapter.AdapterConfig
	httpClient *adapter.HTTPClient
	baseURL    string
}

// NewCohereAdapter creates a Cohere adapter
func NewCohereAdapter(config *adapter.AdapterConfig) (*CohereAdapter, error) {
	if err := validateCohereConfig(config); err != nil {
		return nil, err
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.cohere.com"
	}
	baseURL = normalizeCohereBaseURL(baseURL)

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	maxRetries := config.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	return &CohereAdapter{
		config:     config,
		httpClient: adapter.NewHTTPClient(timeout, maxRetries),
		baseURL:    baseURL,
	}, nil
}

func normalizeCohereBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	lowerBaseURL := strings.ToLower(baseURL)
	for _, suffix := range []string{"/v1", "/v2"} {
		if strings.HasSuffix(lowerBaseURL, suffix) {
			return baseURL[:len(baseURL)-len(suffix)]
		}
	}
	return baseURL
}

func (a *CohereAdapter) endpoint(path string) string {
	return a.baseURL + path
}

func validateCohereConfig(config *adapter.AdapterConfig) error {
	if config == nil {
		return adapter.ErrInvalidConfig
	}
	if config.APIKey == "" {
		return fmt.Errorf("%w: API key is required", adapter.ErrInvalidConfig)
	}
	return nil
}

// ChatCompletion executes chat completion request (Cohere v2 API)
func (a *CohereAdapter) ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	url := a.endpoint("/v2/chat")
	headers := a.buildHeaders()

	// Convert to Cohere request format
	cohereReq := a.convertToCohereRequest(request)

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, cohereReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if statusCode != 200 {
		return nil, a.handleError(statusCode, respBody)
	}

	// Parse Cohere response
	var cohereResp cohereResponse
	if err := json.Unmarshal(respBody, &cohereResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to standard format
	return a.convertFromCohereResponse(&cohereResp, request.Model), nil
}

// ChatCompletionStream executes streaming chat completion request
func (a *CohereAdapter) ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	request.Stream = true
	url := a.endpoint("/v2/chat")
	headers := a.buildHeaders()
	headers["Accept"] = "text/event-stream"

	// Convert to Cohere request format
	cohereReq := a.convertToCohereRequest(request)
	cohereReq["stream"] = true

	resp, err := a.httpClient.DoStreamRequest(ctx, "POST", url, headers, cohereReq)
	if err != nil {
		return nil, fmt.Errorf("stream request failed: %w", err)
	}

	respChan := make(chan adapter.StreamResponse, 10)

	go func() {
		defer close(respChan)
		defer func() {
			// Drain any remaining data from response body before closing
			// This prevents connection reuse issues with stale data
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()

		scanner := bufio.NewScanner(resp.Body)
		var messageID string
		var finalUsage *adapter.Usage

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				respChan <- adapter.StreamResponse{
					Error: ctx.Err(),
					Done:  true,
				}
				return
			default:
			}

			line := scanner.Text()

			// Skip empty lines and event lines
			if line == "" || strings.HasPrefix(line, "event:") {
				continue
			}

			// Parse data line
			if strings.HasPrefix(line, "data:") {
				data := strings.TrimPrefix(line, "data:")
				data = strings.TrimSpace(data)
				if data == "" {
					continue
				}

				var event cohereStreamEvent
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					continue
				}

				switch event.Type {
				case "message-start":
					messageID = event.ID
				case "content-delta":
					text := ""
					if event.Delta.Message.Content.Text != "" {
						text = event.Delta.Message.Content.Text
					}
					respChan <- adapter.StreamResponse{
						ID:      messageID,
						Object:  "chat.completion.chunk",
						Created: time.Now().Unix(),
						Model:   request.Model,
						Choices: []adapter.StreamChoice{
							{
								Index: 0,
								Delta: adapter.Message{
									Role:    "assistant",
									Content: text,
								},
							},
						},
					}
				case "message-end":
					if usage := event.Delta.Usage.Tokens; usage.InputTokens > 0 || usage.OutputTokens > 0 {
						finalUsage = &adapter.Usage{
							PromptTokens:     usage.InputTokens,
							CompletionTokens: usage.OutputTokens,
							TotalTokens:      usage.InputTokens + usage.OutputTokens,
						}
					}
					respChan <- adapter.StreamResponse{
						ID:      messageID,
						Object:  "chat.completion.chunk",
						Created: time.Now().Unix(),
						Model:   request.Model,
						Choices: []adapter.StreamChoice{
							{
								Index:        0,
								FinishReason: a.convertFinishReason(event.Delta.FinishReason),
							},
						},
						Usage: finalUsage,
						Done:  true,
					}
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			respChan <- adapter.StreamResponse{
				Error: err,
				Done:  true,
			}
		}
	}()

	return respChan, nil
}

// CreateResponse executes response creation request (not supported by Cohere)
func (a *CohereAdapter) CreateResponse(ctx context.Context, request *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, fmt.Errorf("CreateResponse is not supported by Cohere")
}

// CreateImage executes image generation request (not supported by Cohere)
func (a *CohereAdapter) CreateImage(ctx context.Context, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, fmt.Errorf("image generation is not supported by Cohere")
}

// CreateEmbeddings executes embeddings creation request (Cohere v2 API)
func (a *CohereAdapter) CreateEmbeddings(ctx context.Context, request *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	url := a.endpoint("/v2/embed")
	headers := a.buildHeaders()

	cohereReq, err := buildCohereEmbedRequest(request)
	if err != nil {
		return nil, err
	}

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, cohereReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if statusCode != 200 {
		return nil, a.handleError(statusCode, respBody)
	}

	var cohereResp cohereEmbedResponse
	if err := json.Unmarshal(respBody, &cohereResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to standard format
	embeddings := make([]adapter.Embedding, 0)
	if len(cohereResp.Embeddings.Float) > 0 {
		for i, emb := range cohereResp.Embeddings.Float {
			embedding := make([]float32, len(emb))
			for j, v := range emb {
				embedding[j] = float32(v)
			}
			embeddings = append(embeddings, adapter.Embedding{
				Object:    "embedding",
				Embedding: embedding,
				Index:     i,
			})
		}
	}

	return &adapter.EmbeddingsResponse{
		Object: "list",
		Data:   embeddings,
		Model:  request.Model,
		Usage: adapter.Usage{
			PromptTokens: cohereResp.Meta.BilledUnits.InputTokens,
			TotalTokens:  cohereResp.Meta.BilledUnits.InputTokens,
		},
	}, nil
}

func buildCohereEmbedRequest(request *adapter.EmbeddingsRequest) (map[string]interface{}, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: request is required", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, fmt.Errorf("%w: model is required", adapter.ErrInvalidRequest)
	}

	embeddingTypes, err := cohereEmbeddingTypes(request.EncodingFormat)
	if err != nil {
		return nil, err
	}

	payloadKey, payloadValue, inputType, err := cohereEmbedPayload(request.Input, request.InputType)
	if err != nil {
		return nil, err
	}

	cohereReq := map[string]interface{}{
		"model":           request.Model,
		"input_type":      inputType,
		"embedding_types": embeddingTypes,
		payloadKey:        payloadValue,
	}
	if request.Dimensions > 0 {
		cohereReq["output_dimension"] = request.Dimensions
	}
	truncate, err := normalizeCohereTruncate(request.Truncate)
	if err != nil {
		return nil, err
	}
	if truncate != "" {
		cohereReq["truncate"] = truncate
	}
	if request.MaxTokens > 0 {
		cohereReq["max_tokens"] = request.MaxTokens
	}

	return cohereReq, nil
}

func cohereEmbeddingTypes(encodingFormat string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(encodingFormat)) {
	case "", "float":
		return []string{"float"}, nil
	default:
		return nil, fmt.Errorf("%w: encoding_format %q is not supported by Cohere adapter", adapter.ErrCapabilityUnsupported, encodingFormat)
	}
}

func cohereEmbedPayload(input interface{}, requestedInputType string) (string, interface{}, string, error) {
	switch v := input.(type) {
	case string:
		inputType, err := normalizeCohereInputType("texts", requestedInputType)
		if err != nil {
			return "", nil, "", err
		}
		if inputType == "image" {
			return "images", []string{v}, inputType, nil
		}
		return "texts", []string{v}, inputType, nil
	case []string:
		inputType, err := normalizeCohereInputType("texts", requestedInputType)
		if err != nil {
			return "", nil, "", err
		}
		if inputType == "image" {
			return "images", v, inputType, nil
		}
		return "texts", v, inputType, nil
	case []interface{}:
		if texts, ok := interfaceSliceToStrings(v); ok {
			inputType, err := normalizeCohereInputType("texts", requestedInputType)
			if err != nil {
				return "", nil, "", err
			}
			if inputType == "image" {
				return "images", texts, inputType, nil
			}
			return "texts", texts, inputType, nil
		}
		if inputs, ok := interfaceSliceToMaps(v); ok {
			inputType, err := normalizeCohereInputType("inputs", requestedInputType)
			if err != nil {
				return "", nil, "", err
			}
			return "inputs", inputs, inputType, nil
		}
	case map[string]interface{}:
		inputType, err := normalizeCohereInputType("inputs", requestedInputType)
		if err != nil {
			return "", nil, "", err
		}
		return "inputs", []map[string]interface{}{v}, inputType, nil
	case []map[string]interface{}:
		inputType, err := normalizeCohereInputType("inputs", requestedInputType)
		if err != nil {
			return "", nil, "", err
		}
		return "inputs", v, inputType, nil
	}

	return "", nil, "", fmt.Errorf("%w: unsupported embeddings input type %T", adapter.ErrInvalidRequest, input)
}

func interfaceSliceToStrings(values []interface{}) ([]string, bool) {
	texts := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, false
		}
		texts = append(texts, text)
	}
	return texts, true
}

func interfaceSliceToMaps(values []interface{}) ([]map[string]interface{}, bool) {
	inputs := make([]map[string]interface{}, 0, len(values))
	for _, value := range values {
		item, ok := value.(map[string]interface{})
		if !ok {
			return nil, false
		}
		inputs = append(inputs, item)
	}
	return inputs, true
}

func normalizeCohereInputType(payloadKind, requested string) (string, error) {
	inputType := strings.ToLower(strings.TrimSpace(requested))
	if inputType == "" {
		if payloadKind == "images" {
			return "image", nil
		}
		return "search_document", nil
	}

	switch inputType {
	case "search_document", "search_query", "classification", "clustering":
		return inputType, nil
	case "image":
		if payloadKind == "inputs" {
			return "", fmt.Errorf("%w: input_type %q is not valid for mixed inputs payload", adapter.ErrInvalidRequest, inputType)
		}
		return inputType, nil
	default:
		return "", fmt.Errorf("%w: unsupported input_type %q", adapter.ErrInvalidRequest, requested)
	}
}

func normalizeCohereTruncate(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "NONE", "START", "END":
		return strings.ToUpper(strings.TrimSpace(value)), nil
	default:
		return "", fmt.Errorf("%w: unsupported truncate value %q", adapter.ErrInvalidRequest, value)
	}
}

// Rerank executes rerank request (Cohere v2 API)
func (a *CohereAdapter) Rerank(ctx context.Context, request *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	url := a.endpoint("/v2/rerank")
	headers := a.buildHeaders()

	// Convert to Cohere rerank request format
	cohereReq := map[string]interface{}{
		"model": request.Model,
		"query": request.Query,
	}

	// Handle documents - can be []string or []map[string]interface{}
	switch v := request.Documents.(type) {
	case []string:
		cohereReq["documents"] = v
	case []interface{}:
		cohereReq["documents"] = v
	default:
		return nil, fmt.Errorf("invalid documents type: must be []string or []interface{}")
	}

	// Optional parameters
	if request.TopN != nil {
		cohereReq["top_n"] = *request.TopN
	}

	// max_tokens_per_doc: prefer the new field, fallback to deprecated MaxChunksPerDoc
	if request.MaxTokensPerDoc != nil {
		cohereReq["max_tokens_per_doc"] = *request.MaxTokensPerDoc
	} else if request.MaxChunksPerDoc != nil {
		cohereReq["max_tokens_per_doc"] = *request.MaxChunksPerDoc
	}

	// priority: Cohere specific parameter
	if request.Priority != nil {
		cohereReq["priority"] = *request.Priority
	}

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, cohereReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if statusCode != 200 {
		return nil, a.handleError(statusCode, respBody)
	}

	var cohereResp cohereRerankResponse
	if err := json.Unmarshal(respBody, &cohereResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to standard format
	results := make([]adapter.RerankResult, 0, len(cohereResp.Results))
	for _, r := range cohereResp.Results {
		result := adapter.RerankResult{
			Index:          r.Index,
			RelevanceScore: r.RelevanceScore,
		}

		// Include document if returned
		if r.Document != nil {
			if docMap, ok := r.Document.(map[string]interface{}); ok {
				result.Document = docMap
				// Extract text field if available
				if text, ok := docMap["text"].(string); ok {
					result.Text = text
				}
			} else if docStr, ok := r.Document.(string); ok {
				result.Text = docStr
				result.Document = docStr
			}
		}

		results = append(results, result)
	}

	response := &adapter.RerankResponse{
		ID:      cohereResp.ID,
		Object:  "list",
		Model:   request.Model,
		Results: results,
	}

	// Add usage if available
	var promptTokens, completionTokens, totalTokens int

	// Use BilledUnits for accurate token counting
	if cohereResp.Meta.BilledUnits.InputTokens != nil {
		promptTokens = int(*cohereResp.Meta.BilledUnits.InputTokens)
	}
	if cohereResp.Meta.BilledUnits.OutputTokens != nil {
		completionTokens = int(*cohereResp.Meta.BilledUnits.OutputTokens)
	}
	if cohereResp.Meta.BilledUnits.SearchUnits != nil {
		// SearchUnits is the primary metric for rerank operations
		promptTokens = int(*cohereResp.Meta.BilledUnits.SearchUnits)
	}

	totalTokens = promptTokens + completionTokens

	if totalTokens > 0 || promptTokens > 0 {
		response.Usage = &adapter.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      totalTokens,
		}
	}

	return response, nil
}

// ListModels gets model list from Cohere
func (a *CohereAdapter) ListModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
	url := a.endpoint("/v1/models")
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", apiKey),
	}

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "GET", url, headers, nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if statusCode != 200 {
		return nil, a.handleError(statusCode, respBody)
	}

	var response cohereModelsResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	models := make([]adapter.Model, 0, len(response.Models))
	for _, m := range response.Models {
		capabilities := cohereCapabilitiesFromEndpoints(m.Endpoints)

		model := adapter.Model{
			ID:            m.Name,
			Name:          m.Name,
			ContextLength: m.ContextLength,
			Capabilities:  capabilities,
			Endpoints:     m.Endpoints, // Store endpoints for database
			IsFinetuned:   m.Finetuned, // Store finetuned status
			Architecture: &adapter.ModelArchitecture{
				Tokenizer: m.TokenizerURL,
			},
		}

		// Map features to capabilities and supported parameters
		if m.Features != nil {
			model.SupportedParameters = m.Features
			for _, feature := range m.Features {
				switch feature {
				case "vision":
					if model.Architecture == nil {
						model.Architecture = &adapter.ModelArchitecture{}
					}
					model.Architecture.InputModalities = append(model.Architecture.InputModalities, "image")
				}
			}
		}

		// Set modality based on endpoints
		if model.Architecture == nil {
			model.Architecture = &adapter.ModelArchitecture{}
		}
		for _, endpoint := range m.Endpoints {
			switch endpoint {
			case "chat", "generate":
				model.Architecture.Modality = "text"
				if !containsString(model.Architecture.InputModalities, "text") {
					model.Architecture.InputModalities = append(model.Architecture.InputModalities, "text")
				}
				if !containsString(model.Architecture.OutputModalities, "text") {
					model.Architecture.OutputModalities = append(model.Architecture.OutputModalities, "text")
				}
			case "embed", "embed_image":
				if model.Architecture.Modality == "" {
					model.Architecture.Modality = "embedding"
				}
			case "rerank":
				if model.Architecture.Modality == "" {
					model.Architecture.Modality = "rerank"
				}
			}
		}

		models = append(models, model)
	}

	return models, nil
}

// GetBalance gets balance information (Cohere doesn't have a balance API)
func (a *CohereAdapter) GetBalance(ctx context.Context, apiKey string) (*adapter.Balance, error) {
	// Cohere doesn't provide a balance API
	return &adapter.Balance{
		IsUnlimited: true,
		Currency:    "USD",
	}, nil
}

// ValidateConfig validates configuration
func (a *CohereAdapter) ValidateConfig(config *adapter.AdapterConfig) error {
	return validateCohereConfig(config)
}

// GetProviderInfo gets provider information
func (a *CohereAdapter) GetProviderInfo() *adapter.ProviderInfo {
	return &adapter.ProviderInfo{
		Name:         "cohere",
		Type:         "cohere",
		DisplayName:  "Cohere",
		Description:  "Cohere LLM and Embedding models",
		BaseURL:      a.baseURL,
		Capabilities: []string{"chat", "embedding", "rerank"},
		Version:      "v2",
	}
}

// buildHeaders builds request headers
func (a *CohereAdapter) buildHeaders() map[string]string {
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", a.config.APIKey),
		"Content-Type":  "application/json",
		"Accept":        "application/json",
	}

	// Add custom headers
	for k, v := range a.config.Headers {
		headers[k] = v
	}

	return headers
}

// handleError handles error response
func (a *CohereAdapter) handleError(statusCode int, body []byte) error {
	var errResp struct {
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil {
		// If body is not JSON (e.g., HTML error page), provide a clearer error message
		bodyStr := string(body)
		if len(bodyStr) > 200 {
			bodyStr = bodyStr[:200] + "..."
		}

		// Check if it's an HTML response
		if strings.HasPrefix(strings.TrimSpace(bodyStr), "<!") || strings.HasPrefix(strings.TrimSpace(bodyStr), "<html") {
			switch statusCode {
			case 403:
				return adapter.NewAdapterError("FORBIDDEN", "API request forbidden. Please check your API key and permissions.", statusCode, adapter.ErrAuthFailed)
			case 401:
				return adapter.NewAdapterError("UNAUTHORIZED", "Invalid API key or authentication failed.", statusCode, adapter.ErrAuthFailed)
			default:
				return adapter.NewAdapterError("UPSTREAM_ERROR", fmt.Sprintf("Received HTML error page (status %d). Please check API endpoint and credentials.", statusCode), statusCode, adapter.ErrUpstreamError)
			}
		}

		return adapter.NewAdapterError("PARSE_ERROR", fmt.Sprintf("Failed to parse error response: %s", bodyStr), statusCode, err)
	}

	switch statusCode {
	case 401:
		return adapter.NewAdapterError("UNAUTHORIZED", errResp.Message, statusCode, adapter.ErrAuthFailed)
	case 403:
		return adapter.NewAdapterError("FORBIDDEN", errResp.Message, statusCode, adapter.ErrAuthFailed)
	case 429:
		return adapter.NewAdapterError("RATE_LIMITED", errResp.Message, statusCode, adapter.ErrRateLimited)
	case 404:
		return adapter.NewAdapterError("NOT_FOUND", errResp.Message, statusCode, adapter.ErrModelNotFound)
	default:
		return adapter.NewAdapterError("UPSTREAM_ERROR", errResp.Message, statusCode, adapter.ErrUpstreamError)
	}
}

// convertToCohereRequest converts standard request to Cohere format
func (a *CohereAdapter) convertToCohereRequest(request *adapter.ChatRequest) map[string]interface{} {
	cohereReq := map[string]interface{}{
		"model": request.Model,
	}

	// Convert messages
	messages := make([]map[string]interface{}, 0, len(request.Messages))
	for _, msg := range request.Messages {
		cohereMsg := map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
		messages = append(messages, cohereMsg)
	}
	cohereReq["messages"] = messages

	// Optional parameters
	if request.Temperature != nil {
		cohereReq["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		cohereReq["p"] = *request.TopP
	}
	if request.MaxTokens != nil {
		cohereReq["max_tokens"] = *request.MaxTokens
	}
	if len(request.Stop) > 0 {
		cohereReq["stop_sequences"] = request.Stop
	}

	return cohereReq
}

// convertFromCohereResponse converts Cohere response to standard format
func (a *CohereAdapter) convertFromCohereResponse(resp *cohereResponse, model string) *adapter.ChatResponse {
	content := ""
	if len(resp.Message.Content) > 0 {
		content = resp.Message.Content[0].Text
	}

	return &adapter.ChatResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []adapter.Choice{
			{
				Index: 0,
				Message: adapter.Message{
					Role:    resp.Message.Role,
					Content: content,
				},
				FinishReason: a.convertFinishReason(resp.FinishReason),
			},
		},
		Usage: &adapter.Usage{
			PromptTokens:     resp.Usage.Tokens.InputTokens,
			CompletionTokens: resp.Usage.Tokens.OutputTokens,
			TotalTokens:      resp.Usage.Tokens.InputTokens + resp.Usage.Tokens.OutputTokens,
		},
	}
}

// convertFinishReason converts Cohere finish reason to standard format
func (a *CohereAdapter) convertFinishReason(reason string) string {
	switch reason {
	case "COMPLETE":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "ERROR":
		return "error"
	default:
		return reason
	}
}

// Cohere API response types
type cohereResponse struct {
	ID           string `json:"id"`
	FinishReason string `json:"finish_reason"`
	Message      struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
	Usage struct {
		BilledUnits struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"billed_units"`
		Tokens struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"tokens"`
	} `json:"usage"`
}

type cohereStreamEvent struct {
	Type  string `json:"type"`
	ID    string `json:"id,omitempty"`
	Index int    `json:"index,omitempty"`
	Delta struct {
		Message struct {
			Role    string `json:"role,omitempty"`
			Content struct {
				Type string `json:"type,omitempty"`
				Text string `json:"text,omitempty"`
			} `json:"content,omitempty"`
		} `json:"message,omitempty"`
		FinishReason string `json:"finish_reason,omitempty"`
		Usage        struct {
			BilledUnits struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"billed_units"`
			Tokens struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"tokens"`
		} `json:"usage,omitempty"`
	} `json:"delta,omitempty"`
}

type cohereModelsResponse struct {
	Models        []cohereModel `json:"models"`
	NextPageToken string        `json:"next_page_token,omitempty"`
}

type cohereModel struct {
	Name             string   `json:"name"`
	Endpoints        []string `json:"endpoints"`
	Finetuned        bool     `json:"finetuned"`
	ContextLength    int      `json:"context_length"`
	TokenizerURL     string   `json:"tokenizer_url,omitempty"`
	Features         []string `json:"features,omitempty"`
	DefaultEndpoints []string `json:"default_endpoints,omitempty"`
}

type cohereEmbedResponse struct {
	ID         string `json:"id"`
	Embeddings struct {
		Float [][]float64 `json:"float"`
	} `json:"embeddings"`
	Texts []string `json:"texts"`
	Meta  struct {
		APIVersion struct {
			Version        string `json:"version"`
			IsExperimental bool   `json:"is_experimental"`
		} `json:"api_version"`
		BilledUnits struct {
			InputTokens int `json:"input_tokens"`
		} `json:"billed_units"`
	} `json:"meta"`
}

type cohereRerankResponse struct {
	ID      string `json:"id"`
	Results []struct {
		Index          int         `json:"index"`
		RelevanceScore float64     `json:"relevance_score"`
		Document       interface{} `json:"document,omitempty"`
	} `json:"results"`
	Meta struct {
		APIVersion struct {
			Version        *string `json:"version,omitempty"`
			IsDeprecated   *bool   `json:"is_deprecated,omitempty"`
			IsExperimental *bool   `json:"is_experimental,omitempty"`
		} `json:"api_version,omitempty"`
		BilledUnits struct {
			Images          *float64 `json:"images,omitempty"`
			InputTokens     *float64 `json:"input_tokens,omitempty"`
			OutputTokens    *float64 `json:"output_tokens,omitempty"`
			SearchUnits     *float64 `json:"search_units,omitempty"`
			Classifications *float64 `json:"classifications,omitempty"`
		} `json:"billed_units,omitempty"`
		Tokens struct {
			InputTokens  *float64 `json:"input_tokens,omitempty"`
			OutputTokens *float64 `json:"output_tokens,omitempty"`
		} `json:"tokens,omitempty"`
		CachedTokens *float64  `json:"cached_tokens,omitempty"`
		Warnings     []*string `json:"warnings,omitempty"`
	} `json:"meta,omitempty"`
}

func cohereCapabilitiesFromEndpoints(endpoints []string) []string {
	capabilities := make([]string, 0, len(endpoints))
	seen := make(map[string]struct{}, len(endpoints))

	for _, endpoint := range endpoints {
		var capability string
		switch endpoint {
		case "chat", "generate":
			capability = "chat"
		case "embed", "embed_image":
			capability = "embedding"
		case "rerank":
			capability = "rerank"
		default:
			continue
		}

		if _, ok := seen[capability]; ok {
			continue
		}
		seen[capability] = struct{}{}
		capabilities = append(capabilities, capability)
	}

	return capabilities
}

// Helper function to check if a string is in a slice
func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
