package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

// DeepSeekAdapter DeepSeek adapter (OpenAI compatible)
type DeepSeekAdapter struct {
	config     *adapter.AdapterConfig
	httpClient *adapter.HTTPClient
	baseURL    string
}

// NewDeepSeekAdapter creates a DeepSeek adapter
func NewDeepSeekAdapter(config *adapter.AdapterConfig) (*DeepSeekAdapter, error) {
	if err := validateDeepSeekConfig(config); err != nil {
		return nil, err
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.deepseek.com/v1"
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	maxRetries := config.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	return &DeepSeekAdapter{
		config:     config,
		httpClient: adapter.NewHTTPClient(timeout, maxRetries),
		baseURL:    baseURL,
	}, nil
}

func validateDeepSeekConfig(config *adapter.AdapterConfig) error {
	if config == nil {
		return adapter.ErrInvalidConfig
	}
	if config.APIKey == "" {
		return fmt.Errorf("%w: API key is required", adapter.ErrInvalidConfig)
	}
	return nil
}

func (a *DeepSeekAdapter) anthropicMessagesBaseURL() string {
	baseURL := strings.TrimRight(a.baseURL, "/")
	if strings.Contains(baseURL, "/anthropic") {
		return baseURL
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return strings.TrimSuffix(baseURL, "/v1") + "/anthropic/v1"
	}
	return baseURL + "/anthropic/v1"
}

// ChatCompletion executes chat completion request
func (a *DeepSeekAdapter) ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error) {
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
func (a *DeepSeekAdapter) ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
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
			// Drain any remaining data from response body before closing
			// This prevents connection reuse issues with stale data
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()

		var lastUsage *adapter.Usage // Keep track of the last usage info

		for {
			select {
			case <-ctx.Done():
				respChan <- adapter.StreamResponse{Error: ctx.Err(), Done: true, Usage: lastUsage}
				return
			case err := <-errChan:
				if err != nil {
					respChan <- adapter.StreamResponse{Error: err, Done: true, Usage: lastUsage}
				}
				return
			case data, ok := <-dataChan:
				if !ok {
					respChan <- adapter.StreamResponse{Done: true, Usage: lastUsage}
					return
				}

				var streamResp adapter.StreamResponse
				if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
					respChan <- adapter.StreamResponse{
						Error: fmt.Errorf("failed to parse stream data: %w", err),
						Done:  true,
						Usage: lastUsage,
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
func (a *DeepSeekAdapter) CreateResponse(ctx context.Context, request *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, fmt.Errorf("%w: DeepSeek /responses API is not documented", adapter.ErrCapabilityUnsupported)
}

func (a *DeepSeekAdapter) CreateAnthropicMessage(ctx context.Context, request *adapter.AnthropicMessageRequest) (*adapter.RawResponse, error) {
	return rawAnthropicMessageRequest(ctx, a.httpClient, a.anthropicMessagesBaseURL(), buildAnthropicRawHeaders(a.config, request.Headers), request, a.handleError)
}

func (a *DeepSeekAdapter) CreateAnthropicMessageStream(ctx context.Context, request *adapter.AnthropicMessageRequest) (<-chan adapter.RawStreamEvent, error) {
	return rawAnthropicMessageStream(ctx, a.httpClient, a.anthropicMessagesBaseURL(), buildAnthropicRawHeaders(a.config, request.Headers), request)
}

// CreateEmbeddings executes embeddings creation request
func (a *DeepSeekAdapter) CreateEmbeddings(ctx context.Context, request *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, fmt.Errorf("%w: DeepSeek embeddings API is not documented", adapter.ErrCapabilityUnsupported)
}

// CreateImage executes image generation request (not supported by DeepSeek)
func (a *DeepSeekAdapter) CreateImage(ctx context.Context, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, fmt.Errorf("%w: image generation is not supported by DeepSeek", adapter.ErrCapabilityUnsupported)
}

// Rerank executes rerank request (not natively supported by DeepSeek)
func (a *DeepSeekAdapter) Rerank(ctx context.Context, request *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, fmt.Errorf("%w: rerank is not supported by DeepSeek", adapter.ErrCapabilityUnsupported)
}

// ListModels gets model list
func (a *DeepSeekAdapter) ListModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
	url := fmt.Sprintf("%s/models", a.baseURL)
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
		model := adapter.Model{
			ID:           m.ID,
			Name:         m.ID,
			Type:         "chat",
			Created:      m.Created,
			OwnedBy:      m.OwnedBy,
			Capabilities: []string{"chat", "stream"},
			Architecture: &adapter.ModelArchitecture{
				Modality:         "text",
				InputModalities:  []string{"text"},
				OutputModalities: []string{"text"},
			},
		}

		// Normalize current official DeepSeek models into local metadata.
		a.enrichModelInfo(&model)

		models = append(models, model)
	}

	return models, nil
}

// GetBalance gets balance information
func (a *DeepSeekAdapter) GetBalance(ctx context.Context, apiKey string) (*adapter.Balance, error) {
	url := fmt.Sprintf("%s/user/balance", a.baseURL)
	headers := map[string]string{
		"Accept":        "application/json",
		"Authorization": fmt.Sprintf("Bearer %s", apiKey),
	}

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "GET", url, headers, nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if statusCode != 200 {
		return nil, a.handleError(statusCode, respBody)
	}

	var balanceResp struct {
		IsAvailable  bool `json:"is_available"`
		BalanceInfos []struct {
			Currency        string `json:"currency"`
			TotalBalance    string `json:"total_balance"`
			GrantedBalance  string `json:"granted_balance"`
			ToppedUpBalance string `json:"topped_up_balance"`
		} `json:"balance_infos"`
	}

	if err := json.Unmarshal(respBody, &balanceResp); err != nil {
		return nil, fmt.Errorf("failed to parse balance: %w", err)
	}

	if len(balanceResp.BalanceInfos) == 0 {
		return &adapter.Balance{
			Total:     decimal.Zero,
			Used:      decimal.Zero,
			Remaining: decimal.Zero,
			Currency:  "CNY",
		}, nil
	}

	info := balanceResp.BalanceInfos[0]
	total, err := decimal.NewFromString(info.TotalBalance)
	if err != nil {
		return nil, fmt.Errorf("failed to parse total balance: %w", err)
	}

	return &adapter.Balance{
		Total:     total,
		Used:      decimal.Zero, // DeepSeek API does not directly provide used amount
		Remaining: total,
		Currency:  info.Currency,
	}, nil
}

// ValidateConfig validates configuration
func (a *DeepSeekAdapter) ValidateConfig(config *adapter.AdapterConfig) error {
	return validateDeepSeekConfig(config)
}

// GetProviderInfo gets provider information
func (a *DeepSeekAdapter) GetProviderInfo() *adapter.ProviderInfo {
	return &adapter.ProviderInfo{
		Name:         "deepseek",
		Type:         "openai_compatible",
		DisplayName:  "DeepSeek",
		Description:  "DeepSeek AI models",
		BaseURL:      a.baseURL,
		Capabilities: []string{"chat", "stream", "model_listing"},
		Version:      "v1",
	}
}

// buildHeaders builds request headers
func (a *DeepSeekAdapter) buildHeaders() map[string]string {
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", a.config.APIKey),
	}

	for k, v := range a.config.Headers {
		headers[k] = v
	}

	return headers
}

// handleError handles error response
func (a *DeepSeekAdapter) handleError(statusCode int, body []byte) error {
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

	switch statusCode {
	case 401:
		return adapter.NewAdapterError(errResp.Error.Code, errResp.Error.Message, statusCode, adapter.ErrAuthFailed)
	case 429:
		return adapter.NewAdapterError(errResp.Error.Code, errResp.Error.Message, statusCode, adapter.ErrRateLimited)
	case 404:
		return adapter.NewAdapterError(errResp.Error.Code, errResp.Error.Message, statusCode, adapter.ErrModelNotFound)
	default:
		return adapter.NewAdapterError(errResp.Error.Code, errResp.Error.Message, statusCode, adapter.ErrUpstreamError)
	}
}

// enrichModelInfo enriches model information
func (a *DeepSeekAdapter) enrichModelInfo(model *adapter.Model) {
	switch model.ID {
	case "deepseek-chat":
		model.ContextLength = 128000
		model.Pricing = &adapter.Pricing{
			Prompt:         decimal.NewFromFloat(2.0),
			Completion:     decimal.NewFromFloat(3.0),
			InputCacheRead: decimal.NewFromFloat(0.2),
		}
		model.Description = "DeepSeek-V3.2-Exp chat model with 128K context"
	case "deepseek-reasoner":
		model.ContextLength = 128000
		model.Pricing = &adapter.Pricing{
			Prompt:         decimal.NewFromFloat(2.0),
			Completion:     decimal.NewFromFloat(3.0),
			InputCacheRead: decimal.NewFromFloat(0.2),
		}
		model.Description = "DeepSeek-R1 reasoning model with 128K context"
	default:
		model.ContextLength = 0
	}
}
