package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/shopspring/decimal"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// OpenRouterAdapter OpenRouter
type OpenRouterAdapter struct {
	config     *adapter.AdapterConfig
	httpClient *adapter.HTTPClient
	baseURL    string
}

// NewOpenRouterAdapter creates an OpenRouter adapter
func NewOpenRouterAdapter(config *adapter.AdapterConfig) (*OpenRouterAdapter, error) {
	if err := validateOpenRouterConfig(config); err != nil {
		return nil, err
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	maxRetries := config.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	return &OpenRouterAdapter{
		config:     config,
		httpClient: adapter.NewHTTPClientFromConfig(config, timeout, maxRetries),
		baseURL:    baseURL,
	}, nil
}

func validateOpenRouterConfig(config *adapter.AdapterConfig) error {
	if config == nil {
		return adapter.ErrInvalidConfig
	}
	if config.APIKey == "" {
		return fmt.Errorf("%w: API key is required", adapter.ErrInvalidConfig)
	}
	return nil
}

// ChatCompletion executes chat completion request
func (a *OpenRouterAdapter) ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	url := fmt.Sprintf("%s/chat/completions", a.baseURL)
	headers := a.buildHeaders()

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, request)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if statusCode != 200 {
		return nil, a.handleError(statusCode, respBody)
	}

	var response adapter.ChatResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

// ChatCompletionStream executes streaming chat completion request
func (a *OpenRouterAdapter) ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	request.Stream = true
	url := fmt.Sprintf("%s/chat/completions", a.baseURL)
	headers := a.buildHeaders()

	resp, err := a.httpClient.DoStreamRequest(ctx, "POST", url, headers, request)
	if err != nil {
		return nil, fmt.Errorf("stream request failed: %w", err)
	}

	respChan := make(chan adapter.StreamResponse, 10)
	dataChan := make(chan string, 10)
	errChan := make(chan error, 1)

	go adapter.ParseSSE(resp.Body, dataChan, errChan)

	go func() {
		defer close(respChan)
		defer func() {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()

		for {
			select {
			case <-ctx.Done():
				respChan <- adapter.StreamResponse{Error: ctx.Err(), Done: true}
				return
			case err := <-errChan:
				if err != nil {
					respChan <- adapter.StreamResponse{Error: err, Done: true}
				}
				return
			case data, ok := <-dataChan:
				if !ok {
					respChan <- adapter.StreamResponse{Done: true}
					return
				}

				var streamResp adapter.StreamResponse
				if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
					respChan <- adapter.StreamResponse{
						Error: fmt.Errorf("failed to parse stream data: %w", err),
						Done:  true,
					}
					return
				}

				respChan <- streamResp
			}
		}
	}()

	return respChan, nil
}

// CreateResponse executes response creation request
func (a *OpenRouterAdapter) CreateResponse(ctx context.Context, request *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	url := fmt.Sprintf("%s/responses", a.baseURL)
	headers := a.buildHeaders()

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, request)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if statusCode != 200 {
		return nil, a.handleError(statusCode, respBody)
	}

	var response adapter.CreateResponseResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

// CreateResponseRaw forwards the native Responses request to OpenRouter.
func (a *OpenRouterAdapter) CreateResponseRaw(ctx context.Context, request *adapter.RawResponseRequest) (*adapter.RawResponse, error) {
	return rawOpenAIResponseRequest(ctx, a.httpClient, a.baseURL, a.buildHeaders(), request, a.handleError)
}

// CreateResponseStream forwards the native Responses stream to OpenRouter.
func (a *OpenRouterAdapter) CreateResponseStream(ctx context.Context, request *adapter.RawResponseRequest) (<-chan adapter.RawStreamEvent, error) {
	return rawOpenAIResponseStream(ctx, a.httpClient, a.baseURL, a.buildHeaders(), request)
}

func (a *OpenRouterAdapter) CreateAnthropicMessage(ctx context.Context, request *adapter.AnthropicMessageRequest) (*adapter.RawResponse, error) {
	return rawAnthropicMessageRequest(ctx, a.httpClient, a.baseURL, buildAnthropicBearerHeaders(a.config, request.Headers), request, a.handleError)
}

func (a *OpenRouterAdapter) CreateAnthropicMessageStream(ctx context.Context, request *adapter.AnthropicMessageRequest) (<-chan adapter.RawStreamEvent, error) {
	return rawAnthropicMessageStream(ctx, a.httpClient, a.baseURL, buildAnthropicBearerHeaders(a.config, request.Headers), request)
}

// CreateEmbeddings executes embeddings creation request
func (a *OpenRouterAdapter) CreateEmbeddings(ctx context.Context, request *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	// OpenRouter might support embeddings for some models
	url := fmt.Sprintf("%s/embeddings", a.baseURL)
	headers := a.buildHeaders()
	payload, err := buildOpenAICompatibleEmbeddingsPayload(request)
	if err != nil {
		return nil, err
	}

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if statusCode != 200 {
		return nil, a.handleError(statusCode, respBody)
	}

	var response adapter.EmbeddingsResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

// CreateImage executes image generation request (not supported by OpenRouter)
func (a *OpenRouterAdapter) CreateImage(ctx context.Context, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, fmt.Errorf("image generation is not supported by OpenRouter")
}

// Rerank executes rerank request (not natively supported by OpenRouter)
func (a *OpenRouterAdapter) Rerank(ctx context.Context, request *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, fmt.Errorf("Rerank is not supported by OpenRouter")
}

// ListModels gets model list
func (a *OpenRouterAdapter) ListModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
	// Fetch chat models
	chatURL := fmt.Sprintf("%s/models", a.baseURL)
	chatModels, err := a.fetchModelsFromURL(ctx, chatURL, apiKey, []string{"chat"})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch chat models: %w", err)
	}

	// Fetch embedding models
	embeddingURL := fmt.Sprintf("%s/embeddings/models", a.baseURL)
	embeddingModels, err := a.fetchModelsFromURL(ctx, embeddingURL, apiKey, []string{"embedding"})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch embedding models: %w", err)
	}

	return append(chatModels, embeddingModels...), nil
}

func (a *OpenRouterAdapter) fetchModelsFromURL(ctx context.Context, url, apiKey string, capabilities []string) ([]adapter.Model, error) {
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

	var response struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			Description   string `json:"description"`
			ContextLength int    `json:"context_length"`
			Pricing       struct {
				Prompt            string `json:"prompt"`
				Completion        string `json:"completion"`
				Image             string `json:"image"`
				ImageToken        string `json:"image_token"`
				ImageOutput       string `json:"image_output"`
				Audio             string `json:"audio"`
				InputAudioCache   string `json:"input_audio_cache"`
				WebSearch         string `json:"web_search"`
				InternalReasoning string `json:"internal_reasoning"`
				InputCacheRead    string `json:"input_cache_read"`
				InputCacheWrite   string `json:"input_cache_write"`
				Request           string `json:"request"`
			} `json:"pricing"`
			Architecture struct {
				Modality         string   `json:"modality"`
				InputModalities  []string `json:"input_modalities"`
				OutputModalities []string `json:"output_modalities"`
				Tokenizer        string   `json:"tokenizer"`
				InstructType     string   `json:"instruct_type"`
			} `json:"architecture"`
			TopProvider struct {
				ContextLength       int  `json:"context_length"`
				MaxCompletionTokens int  `json:"max_completion_tokens"`
				IsModerated         bool `json:"is_moderated"`
			} `json:"top_provider"`
			SupportedParameters []string               `json:"supported_parameters"`
			DefaultParameters   map[string]interface{} `json:"default_parameters"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	models := make([]adapter.Model, 0)
	for _, m := range response.Data {
		promptPrice, _ := decimal.NewFromString(m.Pricing.Prompt)
		completionPrice, _ := decimal.NewFromString(m.Pricing.Completion)
		imagePrice, _ := decimal.NewFromString(m.Pricing.Image)
		imageTokenPrice, _ := decimal.NewFromString(m.Pricing.ImageToken)
		imageOutputPrice, _ := decimal.NewFromString(m.Pricing.ImageOutput)
		audioPrice, _ := decimal.NewFromString(m.Pricing.Audio)
		inputAudioCachePrice, _ := decimal.NewFromString(m.Pricing.InputAudioCache)
		webSearchPrice, _ := decimal.NewFromString(m.Pricing.WebSearch)
		internalReasoningPrice, _ := decimal.NewFromString(m.Pricing.InternalReasoning)
		inputCacheReadPrice, _ := decimal.NewFromString(m.Pricing.InputCacheRead)
		inputCacheWritePrice, _ := decimal.NewFromString(m.Pricing.InputCacheWrite)
		requestPrice, _ := decimal.NewFromString(m.Pricing.Request)

		// OpenRouter price unit is per million tokens usually, but here raw values are likely per token if small?
		// Doc says "Prompt": "0.00003". If this is per token, it's huge. 0.00003 * 1M = 30. Sounds like GPT-4.
		// Existing code multiplies by 1,000,000. So assuming inputs are per token.

		multiplier := decimal.NewFromInt(1000000)
		promptPrice = promptPrice.Mul(multiplier)
		completionPrice = completionPrice.Mul(multiplier)
		// Image is usually per image, not per million images?
		// If pricing is per 1 unit.
		// "image": "0"
		// Let's keep other prices as is if they are not per-token or if we want them per-1-unit.
		// However, our DB stores "InputPrice" (decimal(10,4)).
		// It seems we want unified units.
		// Adapter returns `Pricing` struct. The service layer will decide how to store it.
		// But the `adapter.Pricing` comment says "Price per million tokens".
		// So we should normalize per-token costs to per-million.
		// For things like Image (per image), we should probably store as is.

		inputCacheReadPrice = inputCacheReadPrice.Mul(multiplier)
		inputCacheWritePrice = inputCacheWritePrice.Mul(multiplier)
		internalReasoningPrice = internalReasoningPrice.Mul(multiplier)
		// Audio is likely per second or minute.
		// Let's pass raw values for non-token based costs for now, or strictly follow "per unit".
		// The DB comments say: CostImage "Cost per image", CostAudio "Cost per minute".

		model := adapter.Model{
			ID:            m.ID,
			Name:          m.Name,
			Description:   m.Description,
			ContextLength: m.ContextLength,
			Capabilities:  capabilities,
			Pricing: &adapter.Pricing{
				Prompt:            promptPrice,
				Completion:        completionPrice,
				Image:             imagePrice,
				ImageToken:        imageTokenPrice,
				ImageOutput:       imageOutputPrice,
				Audio:             audioPrice,
				InputAudioCache:   inputAudioCachePrice,
				WebSearch:         webSearchPrice,
				InternalReasoning: internalReasoningPrice,
				InputCacheRead:    inputCacheReadPrice,
				InputCacheWrite:   inputCacheWritePrice,
				Request:           requestPrice,
			},
			Architecture: &adapter.ModelArchitecture{
				Modality:         m.Architecture.Modality,
				InputModalities:  m.Architecture.InputModalities,
				OutputModalities: m.Architecture.OutputModalities,
				Tokenizer:        m.Architecture.Tokenizer,
				InstructType:     m.Architecture.InstructType,
			},
			SupportedParameters: m.SupportedParameters,
			DefaultParameters:   m.DefaultParameters,
			IsModerated:         m.TopProvider.IsModerated,
		}

		models = append(models, model)
	}

	return models, nil
}

// GetBalance gets balance information
// OpenRouter API: https://openrouter.ai/api/v1/credits
// Response format:
//
//	{
//	  "data": {
//	    "total_credits": 100.5,
//	    "total_usage": 25.75
//	  }
//	}
func (a *OpenRouterAdapter) GetBalance(ctx context.Context, apiKey string) (*adapter.Balance, error) {
	url := fmt.Sprintf("%s/credits", a.baseURL)
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

	var creditsInfo struct {
		Data struct {
			TotalCredits float64 `json:"total_credits"` // Total credits (USD)
			TotalUsage   float64 `json:"total_usage"`   // Used amount (USD)
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &creditsInfo); err != nil {
		return nil, fmt.Errorf("failed to parse credits info: %w", err)
	}

	totalCredits := decimal.NewFromFloat(creditsInfo.Data.TotalCredits)
	totalUsage := decimal.NewFromFloat(creditsInfo.Data.TotalUsage)
	remaining := totalCredits.Sub(totalUsage)

	balance := &adapter.Balance{
		Total:     totalCredits,
		Used:      totalUsage,
		Remaining: remaining,
		Currency:  "USD",
	}

	// If total credits is 0, it may be unlimited or not set
	if totalCredits.IsZero() {
		balance.IsUnlimited = true
	}

	return balance, nil
}

// ValidateConfig validates configuration
func (a *OpenRouterAdapter) ValidateConfig(config *adapter.AdapterConfig) error {
	return validateOpenRouterConfig(config)
}

// GetProviderInfo gets provider information
func (a *OpenRouterAdapter) GetProviderInfo() *adapter.ProviderInfo {
	return &adapter.ProviderInfo{
		Name:         "openrouter",
		Type:         "openai_compatible",
		DisplayName:  "OpenRouter",
		Description:  "OpenRouter aggregated AI models",
		BaseURL:      a.baseURL,
		Capabilities: []string{"chat", "completion"},
		Version:      "v1",
	}
}

// buildHeaders builds request headers
func (a *OpenRouterAdapter) buildHeaders() map[string]string {
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", a.config.APIKey),
	}

	// OpenRouter specific headers
	if referer, ok := a.config.CustomParams["http_referer"].(string); ok {
		headers["HTTP-Referer"] = referer
	} else {
		headers["HTTP-Referer"] = "https://github.com/zgiai/zgi/api"
	}

	if title, ok := a.config.CustomParams["x_title"].(string); ok {
		headers["X-Title"] = title
	} else {
		headers["X-Title"] = "ZGI AI Platform"
	}

	for k, v := range a.config.Headers {
		headers[k] = v
	}

	return headers
}

// handleError handles error response
func (a *OpenRouterAdapter) handleError(statusCode int, body []byte) error {
	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil {
		return adapter.HandleNonJSONError(statusCode, body)
	}

	switch statusCode {
	case 401:
		return adapter.NewAdapterError(errResp.Error.Code, errResp.Error.Message, statusCode, adapter.ErrAuthFailed)
	case 429:
		return adapter.NewAdapterError(errResp.Error.Code, errResp.Error.Message, statusCode, adapter.ErrRateLimited)
	case 402:
		return adapter.NewAdapterError(errResp.Error.Code, errResp.Error.Message, statusCode, adapter.ErrInsufficientBalance)
	case 404:
		return adapter.NewAdapterError(errResp.Error.Code, errResp.Error.Message, statusCode, adapter.ErrModelNotFound)
	default:
		return adapter.NewAdapterError(errResp.Error.Code, errResp.Error.Message, statusCode, adapter.ErrUpstreamError)
	}
}
