package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	openaiparam "github.com/openai/openai-go/v3/packages/param"
	openairesponses "github.com/openai/openai-go/v3/responses"
	"github.com/shopspring/decimal"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// OpenAIAdapter OpenAI adapter
type OpenAIAdapter struct {
	config     *adapter.AdapterConfig
	httpClient *adapter.HTTPClient
	baseURL    string
	exactURL   bool
}

// NewOpenAIAdapter creates an OpenAI adapter
func NewOpenAIAdapter(config *adapter.AdapterConfig) (*OpenAIAdapter, error) {
	if err := validateOpenAIConfig(config); err != nil {
		return nil, err
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL, exactURL, err := normalizeOpenAIBaseURL(config.ProviderName, baseURL)
	if err != nil {
		return nil, err
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	maxRetries := config.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	return &OpenAIAdapter{
		config:     config,
		httpClient: adapter.NewHTTPClientFromConfig(config, timeout, maxRetries),
		baseURL:    baseURL,
		exactURL:   exactURL,
	}, nil
}

func validateOpenAIConfig(config *adapter.AdapterConfig) error {
	if config == nil {
		return adapter.ErrInvalidConfig
	}
	// API key is not required when AuthHook is set (e.g., HMAC-signed internal APIs)
	if config.APIKey == "" && config.AuthHook == nil {
		return fmt.Errorf("%w: API key is required (or set AuthHook for custom auth)", adapter.ErrInvalidConfig)
	}
	return nil
}

func newOpenAIAdapterWithOverrides(base *adapter.AdapterConfig, baseURL string) (*OpenAIAdapter, error) {
	if base == nil {
		return nil, adapter.ErrInvalidConfig
	}
	cfg := *base
	if strings.TrimSpace(baseURL) != "" {
		cfg.BaseURL = strings.TrimSpace(baseURL)
	}
	return NewOpenAIAdapter(&cfg)
}

func normalizeOpenAIBaseURL(providerName, raw string) (string, bool, error) {
	baseURL := strings.TrimSpace(raw)
	if baseURL == "" {
		return "", false, nil
	}
	if !strings.HasSuffix(baseURL, "#") {
		return baseURL, false, nil
	}

	baseURL = strings.TrimSpace(strings.TrimSuffix(baseURL, "#"))
	if baseURL == "" {
		return "", false, fmt.Errorf("%w: base_url before # is required", adapter.ErrInvalidConfig)
	}

	return baseURL, providerName == "openai-compatible", nil
}

func (a *OpenAIAdapter) requestURL(path string) string {
	if a.exactURL {
		return a.baseURL
	}
	return fmt.Sprintf("%s%s", a.baseURL, path)
}

func (a *OpenAIAdapter) unsupportedExactMetadata(capability string) error {
	return fmt.Errorf(
		"%w: %s is unavailable when openai-compatible base_url ends with #",
		adapter.ErrCapabilityUnsupported,
		capability,
	)
}

// ChatCompletion executes chat completion request
func (a *OpenAIAdapter) ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	url := a.requestURL("/chat/completions")
	headers := a.buildHeaders()

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, a.buildChatPayload(request))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if statusCode != 200 {
		return nil, a.handleError(statusCode, respBody)
	}

	// Some proxies (like agicto) return 200 OK but with an error body
	// We check for "error" field even on 200 OK to be robust
	if strings.Contains(string(respBody), "\"error\":") {
		var errorCheck struct {
			Error interface{} `json:"error"`
		}
		if err := json.Unmarshal(respBody, &errorCheck); err == nil && errorCheck.Error != nil {
			return nil, a.handleError(statusCode, respBody)
		}
	}

	var response adapter.ChatResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

// ChatCompletionStream executes streaming chat completion request
func (a *OpenAIAdapter) ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	request.Stream = true
	url := a.requestURL("/chat/completions")
	headers := a.buildHeaders()

	resp, err := a.httpClient.DoStreamRequest(ctx, "POST", url, headers, a.buildChatPayload(request))
	if err != nil {
		var statusErr *adapter.HTTPStatusError
		if errors.As(err, &statusErr) {
			return nil, a.handleError(statusErr.StatusCode, statusErr.Body)
		}
		return nil, fmt.Errorf("stream request failed: %w", err)
	}

	respChan := make(chan adapter.StreamResponse, 10)
	dataChan := make(chan string, 10)
	errChan := make(chan error, 1)

	// Start SSE parsing
	go adapter.ParseSSE(resp.Body, dataChan, errChan)

	// Process streaming data
	go func() {
		defer close(respChan)
		defer func() {
			// Drain any remaining data from response body before closing
			// This prevents connection reuse issues with stale data
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()

		var lastUsage *adapter.Usage // Keep track of the last usage info

		for {
			select {
			case <-ctx.Done():
				respChan <- adapter.StreamResponse{
					Error: ctx.Err(),
					Done:  true,
					Usage: lastUsage, // Include last known usage
				}
				return
			case err := <-errChan:
				if err != nil {
					respChan <- adapter.StreamResponse{
						Error: err,
						Done:  true,
						Usage: lastUsage, // Include last known usage
					}
				}
				return
			case data, ok := <-dataChan:
				if !ok {
					// Stream ended - send final message with usage
					respChan <- adapter.StreamResponse{
						Done:  true,
						Usage: lastUsage, // Include last known usage
					}
					return
				}

				var streamResp adapter.StreamResponse
				if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
					respChan <- adapter.StreamResponse{
						Error: fmt.Errorf("failed to parse stream data: %w", err),
						Done:  true,
						Usage: lastUsage, // Include last known usage
					}
					return
				}

				// Track usage info from any chunk
				if streamResp.Usage != nil {
					lastUsage = streamResp.Usage
				}

				respChan <- streamResp
			}
		}
	}()

	return respChan, nil
}

// CreateResponse executes response creation request
func (a *OpenAIAdapter) CreateResponse(ctx context.Context, request *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	url := a.requestURL("/responses")
	headers := a.buildHeaders()

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, a.buildCreateResponsePayload(request))
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

// CreateResponseRaw executes the native OpenAI Responses API without reshaping the JSON body.
func (a *OpenAIAdapter) CreateResponseRaw(ctx context.Context, request *adapter.RawResponseRequest) (*adapter.RawResponse, error) {
	if a.exactURL {
		return nil, a.unsupportedExactMetadata("responses")
	}
	body, err := rawRequestBody(request.Body)
	if err != nil {
		return nil, err
	}

	client := openai.NewClient(a.openAIClientOptions()...)
	var params openairesponses.ResponseNewParams
	openaiparam.SetJSON(body, &params)

	var raw json.RawMessage
	if _, err := client.Responses.New(ctx, params, openaioption.WithResponseBodyInto(&raw)); err != nil {
		return nil, fmt.Errorf("responses request failed: %w", err)
	}

	return &adapter.RawResponse{
		Body:  raw,
		Usage: openAIUsageFromRaw(raw),
	}, nil
}

// CreateResponseStream executes the native OpenAI Responses streaming API.
func (a *OpenAIAdapter) CreateResponseStream(ctx context.Context, request *adapter.RawResponseRequest) (<-chan adapter.RawStreamEvent, error) {
	if a.exactURL {
		return nil, a.unsupportedExactMetadata("responses")
	}
	body, err := rawRequestBody(request.Body)
	if err != nil {
		return nil, err
	}

	client := openai.NewClient(a.openAIClientOptions()...)
	var params openairesponses.ResponseNewParams
	openaiparam.SetJSON(body, &params)

	stream := client.Responses.NewStreaming(ctx, params)
	out := make(chan adapter.RawStreamEvent, 10)

	go func() {
		defer close(out)
		defer stream.Close()

		var lastUsage *adapter.Usage
		for stream.Next() {
			event := stream.Current()
			raw := json.RawMessage(event.RawJSON())
			eventType := strings.TrimSpace(event.Type)
			if eventType == "" {
				eventType = rawEventType(raw)
			}
			if usage := openAIUsageFromRaw(raw); usage != nil {
				lastUsage = usage
			}
			out <- adapter.RawStreamEvent{
				Event: eventType,
				Data:  raw,
				Usage: lastUsage,
			}
		}
		if err := stream.Err(); err != nil {
			out <- adapter.RawStreamEvent{Error: err, Done: true, Usage: lastUsage}
			return
		}
		out <- adapter.RawStreamEvent{Done: true, Usage: lastUsage}
	}()

	return out, nil
}

// CreateEmbeddings executes embeddings creation request
func (a *OpenAIAdapter) CreateEmbeddings(ctx context.Context, request *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	url := a.requestURL("/embeddings")
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

func buildOpenAICompatibleEmbeddingsPayload(request *adapter.EmbeddingsRequest) (map[string]interface{}, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: request is required", adapter.ErrInvalidRequest)
	}
	if request.Model == "" {
		return nil, fmt.Errorf("%w: model is required", adapter.ErrInvalidRequest)
	}
	if request.Input == nil {
		return nil, fmt.Errorf("%w: input is required", adapter.ErrInvalidRequest)
	}

	payload := map[string]interface{}{
		"model": request.Model,
		"input": request.Input,
	}
	if request.EncodingFormat != "" {
		payload["encoding_format"] = request.EncodingFormat
	}
	if request.Dimensions > 0 {
		payload["dimensions"] = request.Dimensions
	}
	if request.User != "" {
		payload["user"] = request.User
	}
	return payload, nil
}

// CreateImage executes image generation request
func (a *OpenAIAdapter) CreateImage(ctx context.Context, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	url := a.requestURL("/images/generations")
	headers := a.buildHeaders()

	// Build payload
	payload := map[string]interface{}{
		"model":  request.Model,
		"prompt": request.Prompt,
	}
	if request.N != nil {
		payload["n"] = *request.N
	}
	if request.Size != "" {
		payload["size"] = request.Size
	}
	if request.Quality != "" {
		payload["quality"] = request.Quality
	}
	if request.Style != "" {
		payload["style"] = request.Style
	}
	if request.ResponseFormat != "" {
		payload["response_format"] = request.ResponseFormat
	}
	if request.User != "" {
		payload["user"] = request.User
	}
	for k, v := range request.AdditionalParameters {
		payload[k] = v
	}

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if statusCode != 200 {
		return nil, a.handleError(statusCode, respBody)
	}

	var response adapter.ImageResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

// Rerank executes rerank request (not natively supported by OpenAI)
func (a *OpenAIAdapter) Rerank(ctx context.Context, request *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	url := a.requestURL("/v1/rerank")
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

// ListModels gets model list
func (a *OpenAIAdapter) ListModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
	if a.exactURL {
		return nil, a.unsupportedExactMetadata("model listing")
	}

	url := a.requestURL("/models")
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", apiKey),
	}

	if a.config.Organization != "" {
		headers["OpenAI-Organization"] = a.config.Organization
	}

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "GET", url, headers, nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if statusCode != 200 {
		if shouldTreatOpenAIListModelsAsCapabilityUnsupported(statusCode, respBody) {
			return nil, fmt.Errorf("%w: upstream /models endpoint is unavailable", adapter.ErrCapabilityUnsupported)
		}
		return nil, a.handleError(statusCode, respBody)
	}

	var response struct {
		Data []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	models := make([]adapter.Model, 0)
	for _, m := range response.Data {
		// Only return chat models
		// if !strings.Contains(m.ID, "gpt") && !strings.Contains(m.ID, "chat") {
		// 	continue
		// }

		model := adapter.Model{
			ID:           m.ID,
			Name:         m.ID,
			Created:      m.Created,
			OwnedBy:      m.OwnedBy,
			Capabilities: []string{"chat"},
		}

		// Set context length and pricing (based on known models)
		a.enrichModelInfo(&model)

		models = append(models, model)
	}

	return models, nil
}

func shouldTreatOpenAIListModelsAsCapabilityUnsupported(statusCode int, body []byte) bool {
	switch statusCode {
	case 404, 405, 501:
		return true
	}

	msg := strings.ToLower(string(body))
	return strings.Contains(msg, "not implemented") ||
		strings.Contains(msg, "unsupported") ||
		strings.Contains(msg, "method not allowed")
}

// GetBalance gets balance information
func (a *OpenAIAdapter) GetBalance(ctx context.Context, apiKey string) (*adapter.Balance, error) {
	return nil, fmt.Errorf("%w: OpenAI inference keys do not expose account balance", adapter.ErrCapabilityUnsupported)
}

type openAISubscription struct {
	HardLimitUSD float64 `json:"hard_limit_usd"`
	AccessUntil  int64   `json:"access_until"`
}

type openAIUsage struct {
	TotalUsage float64 `json:"total_usage"` // in cents
}

func (a *OpenAIAdapter) getSubscription(ctx context.Context, apiKey string) (*openAISubscription, error) {
	url := a.requestURL("/dashboard/billing/subscription")
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

	var subscription openAISubscription
	if err := json.Unmarshal(respBody, &subscription); err != nil {
		return nil, fmt.Errorf("failed to parse subscription: %w", err)
	}

	return &subscription, nil
}

func (a *OpenAIAdapter) getUsage(ctx context.Context, apiKey string) (*openAIUsage, error) {
	// Get current month usage
	now := time.Now()
	startDate := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	endDate := now.Format("2006-01-02")

	url := fmt.Sprintf("%s?start_date=%s&end_date=%s", a.requestURL("/dashboard/billing/usage"), startDate, endDate)
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

	var usage openAIUsage
	if err := json.Unmarshal(respBody, &usage); err != nil {
		return nil, fmt.Errorf("failed to parse usage: %w", err)
	}

	return &usage, nil
}

// ValidateConfig validates configuration
func (a *OpenAIAdapter) ValidateConfig(config *adapter.AdapterConfig) error {
	return validateOpenAIConfig(config)
}

// GetProviderInfo gets provider information
func (a *OpenAIAdapter) GetProviderInfo() *adapter.ProviderInfo {
	return &adapter.ProviderInfo{
		Name:         "openai",
		Type:         "openai",
		DisplayName:  "OpenAI",
		Description:  "OpenAI GPT models",
		BaseURL:      a.baseURL,
		Capabilities: []string{"chat", "completion", "embedding", "image"},
		Version:      "v1",
	}
}

// buildHeaders builds request headers
func (a *OpenAIAdapter) buildHeaders() map[string]string {
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", a.config.APIKey),
	}

	if a.config.Organization != "" {
		headers["OpenAI-Organization"] = a.config.Organization
	}

	// Add custom headers
	for k, v := range a.config.Headers {
		headers[k] = v
	}

	return headers
}

func (a *OpenAIAdapter) openAIClientOptions() []openaioption.RequestOption {
	baseURL := strings.TrimRight(a.baseURL, "/") + "/"
	opts := []openaioption.RequestOption{
		openaioption.WithAPIKey(a.config.APIKey),
		openaioption.WithBaseURL(baseURL),
		openaioption.WithMaxRetries(a.config.MaxRetries),
		openaioption.WithRequestTimeout(a.config.Timeout),
	}
	if client := a.httpClient.StandardClient(); client != nil {
		opts = append(opts, openaioption.WithHTTPClient(client))
	}
	if a.config.Organization != "" {
		opts = append(opts, openaioption.WithOrganization(a.config.Organization))
	}
	for k, v := range a.config.Headers {
		opts = append(opts, openaioption.WithHeader(k, v))
	}
	return opts
}

// handleError handles error response
func (a *OpenAIAdapter) handleError(statusCode int, body []byte) error {
	return handleOpenAICompatibleError(statusCode, body)
}

func handleOpenAICompatibleError(statusCode int, body []byte) error {
	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil {
		return adapter.HandleNonJSONError(statusCode, body)
	}
	platformErrorCode := errResp.Error.Code
	if platformErrorCode == "" && errResp.Error.Type == adapter.ErrorCodePlatformChannelUnavailable {
		platformErrorCode = errResp.Error.Type
	}
	if platformErrorCode == adapter.ErrorCodePlatformChannelUnavailable {
		return adapter.NewAdapterError(platformErrorCode, errResp.Error.Message, statusCode, adapter.ErrPlatformChannelUnavailable)
	}

	switch statusCode {
	case 401:
		return adapter.NewAdapterError(errResp.Error.Code, errResp.Error.Message, statusCode, adapter.ErrAuthFailed)
	case 429:
		return adapter.NewAdapterError(errResp.Error.Code, errResp.Error.Message, statusCode, adapter.ErrRateLimited)
	case 404:
		return adapter.NewAdapterError(errResp.Error.Code, errResp.Error.Message, statusCode, adapter.ErrModelNotFound)
	case 400:
		if errResp.Error.Code == "content_policy_violation" {
			return adapter.NewAdapterError(errResp.Error.Code, errResp.Error.Message, statusCode, adapter.ErrContentPolicyViolation)
		}
		if errResp.Error.Code == "billing_hard_limit_reached" {
			return adapter.NewAdapterError(errResp.Error.Code, errResp.Error.Message, statusCode, adapter.ErrInsufficientBalance)
		}
		return adapter.NewAdapterError(errResp.Error.Code, errResp.Error.Message, statusCode, adapter.ErrInvalidRequest)
	default:
		return adapter.NewAdapterError(errResp.Error.Code, errResp.Error.Message, statusCode, adapter.ErrUpstreamError)
	}
}

// enrichModelInfo enriches model information (context length, pricing, etc.)
func (a *OpenAIAdapter) enrichModelInfo(model *adapter.Model) {
	switch {
	case strings.Contains(model.ID, "gpt-4-turbo"):
		model.ContextLength = 128000
		model.Pricing = &adapter.Pricing{
			Prompt:     decimal.NewFromFloat(10.0),
			Completion: decimal.NewFromFloat(30.0),
		}
	case strings.Contains(model.ID, "gpt-4-32k"):
		model.ContextLength = 32768
		model.Pricing = &adapter.Pricing{
			Prompt:     decimal.NewFromFloat(60.0),
			Completion: decimal.NewFromFloat(120.0),
		}
	case strings.Contains(model.ID, "gpt-4"):
		model.ContextLength = 8192
		model.Pricing = &adapter.Pricing{
			Prompt:     decimal.NewFromFloat(30.0),
			Completion: decimal.NewFromFloat(60.0),
		}
	case strings.Contains(model.ID, "gpt-3.5-turbo-16k"):
		model.ContextLength = 16384
		model.Pricing = &adapter.Pricing{
			Prompt:     decimal.NewFromFloat(3.0),
			Completion: decimal.NewFromFloat(4.0),
		}
	case strings.Contains(model.ID, "gpt-3.5-turbo"):
		model.ContextLength = 4096
		model.Pricing = &adapter.Pricing{
			Prompt:     decimal.NewFromFloat(1.5),
			Completion: decimal.NewFromFloat(2.0),
		}
	default:
		model.ContextLength = 4096
	}
}

// buildChatPayload builds the final JSON payload for OpenAI API
func (a *OpenAIAdapter) buildChatPayload(request *adapter.ChatRequest) map[string]interface{} {
	return buildOpenAICompatibleChatPayload(request)
}

func buildOpenAICompatibleChatPayload(request *adapter.ChatRequest) map[string]interface{} {
	payload := make(map[string]interface{})
	payload["model"] = request.Model
	payload["messages"] = request.Messages

	isO1 := strings.Contains(request.Model, "o1")

	if request.Temperature != nil && !isO1 {
		payload["temperature"] = *request.Temperature
	}
	if request.TopP != nil && !isO1 {
		payload["top_p"] = *request.TopP
	}
	if request.MaxTokens != nil {
		payload["max_tokens"] = *request.MaxTokens
	}
	if request.Stream {
		payload["stream"] = true
	}
	if request.StreamOptions != nil {
		payload["stream_options"] = request.StreamOptions
	}
	if len(request.Stop) > 0 {
		payload["stop"] = request.Stop
	}
	if request.PresencePenalty != nil {
		payload["presence_penalty"] = *request.PresencePenalty
	}
	if request.FrequencyPenalty != nil {
		payload["frequency_penalty"] = *request.FrequencyPenalty
	}
	if request.User != "" {
		payload["user"] = request.User
	}
	if len(request.Tools) > 0 {
		payload["tools"] = request.Tools
		if request.ToolChoice != nil {
			payload["tool_choice"] = request.ToolChoice
		}
	}
	if request.ResponseFormat != nil {
		payload["response_format"] = request.ResponseFormat
	}
	if request.Seed != nil {
		payload["seed"] = *request.Seed
	}
	if request.N != nil {
		payload["n"] = *request.N
	}
	if len(request.LogitBias) > 0 {
		payload["logit_bias"] = request.LogitBias
	}

	// Process AdditionalParameters (Dynamic Mapping)
	for k, v := range request.AdditionalParameters {
		switch k {
		case "reasoning_effort":
			// Map to OpenAI o1 structure: "reasoning": {"effort": "..."}
			payload["reasoning"] = map[string]interface{}{"effort": v}
			// o1 doesn't support temperature/top_p, but we usually let the user omit them
			// If they are provided, we could strip them here if needed.
		default:
			// Passthrough for any other parameters
			payload[k] = v
		}
	}

	return payload
}

// buildCreateResponsePayload builds payload for /v1/responses
func (a *OpenAIAdapter) buildCreateResponsePayload(request *adapter.CreateResponseRequest) map[string]interface{} {
	payload := make(map[string]interface{})
	payload["model"] = request.Model
	if request.Input != nil {
		payload["input"] = request.Input
	}
	if len(request.Messages) > 0 {
		payload["messages"] = request.Messages
	}
	if len(request.Tools) > 0 {
		payload["tools"] = request.Tools
		if request.ToolChoice != nil {
			payload["tool_choice"] = request.ToolChoice
		}
	}
	if request.ResponseFormat != nil {
		payload["response_format"] = request.ResponseFormat
	}
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		payload["top_p"] = *request.TopP
	}
	if request.MaxTokens != nil {
		payload["max_tokens"] = *request.MaxTokens
	}
	if request.MaxOutputTokens != nil {
		payload["max_output_tokens"] = *request.MaxOutputTokens
	}
	if request.Stream {
		payload["stream"] = true
	}
	if len(request.Metadata) > 0 {
		payload["metadata"] = request.Metadata
	}
	if request.Instructions != "" {
		payload["instructions"] = request.Instructions
	}
	if len(request.Modalities) > 0 {
		payload["modalities"] = request.Modalities
	}

	// Process AdditionalParameters
	for k, v := range request.AdditionalParameters {
		switch k {
		case "reasoning_effort":
			payload["reasoning"] = map[string]interface{}{"effort": v}
		default:
			payload[k] = v
		}
	}

	return payload
}
