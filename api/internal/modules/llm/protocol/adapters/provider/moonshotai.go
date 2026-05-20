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

// MoonshotAIAdapter Moonshotai adapter (OpenAI compatible)
type MoonshotAIAdapter struct {
	config     *adapter.AdapterConfig
	httpClient *adapter.HTTPClient
	baseURL    string
}

// NewMoonshotAIAdapter creates a Moonshotai adapter
func NewMoonshotAIAdapter(config *adapter.AdapterConfig) (*MoonshotAIAdapter, error) {
	if err := validateMoonshotAIConfig(config); err != nil {
		return nil, err
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.moonshot.ai/v1"
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	maxRetries := config.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	return &MoonshotAIAdapter{
		config:     config,
		httpClient: adapter.NewHTTPClient(timeout, maxRetries),
		baseURL:    baseURL,
	}, nil
}

func (a *MoonshotAIAdapter) anthropicMessagesBaseURL() string {
	baseURL := strings.TrimRight(a.baseURL, "/")
	if strings.Contains(baseURL, "/anthropic") {
		return baseURL
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return strings.TrimSuffix(baseURL, "/v1") + "/anthropic/v1"
	}
	return baseURL + "/anthropic/v1"
}

func validateMoonshotAIConfig(config *adapter.AdapterConfig) error {
	if config == nil {
		return adapter.ErrInvalidConfig
	}
	if config.APIKey == "" {
		return fmt.Errorf("%w: API key is required", adapter.ErrInvalidConfig)
	}
	return nil
}

// ChatCompletion executes chat completion request
func (a *MoonshotAIAdapter) ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error) {
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
func (a *MoonshotAIAdapter) ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
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
func (a *MoonshotAIAdapter) CreateResponse(ctx context.Context, request *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
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

func (a *MoonshotAIAdapter) CreateAnthropicMessage(ctx context.Context, request *adapter.AnthropicMessageRequest) (*adapter.RawResponse, error) {
	return rawAnthropicMessageRequest(ctx, a.httpClient, a.anthropicMessagesBaseURL(), buildAnthropicBearerHeaders(a.config, request.Headers), request, a.handleError)
}

func (a *MoonshotAIAdapter) CreateAnthropicMessageStream(ctx context.Context, request *adapter.AnthropicMessageRequest) (<-chan adapter.RawStreamEvent, error) {
	return rawAnthropicMessageStream(ctx, a.httpClient, a.anthropicMessagesBaseURL(), buildAnthropicBearerHeaders(a.config, request.Headers), request)
}

// CreateEmbeddings executes embeddings creation request
func (a *MoonshotAIAdapter) CreateEmbeddings(ctx context.Context, request *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
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

// CreateImage executes image generation request (not supported by MoonshotAI)
func (a *MoonshotAIAdapter) CreateImage(ctx context.Context, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, fmt.Errorf("image generation is not supported by MoonshotAI")
}

// Rerank executes rerank request (not natively supported by MoonshotAI)
func (a *MoonshotAIAdapter) Rerank(ctx context.Context, request *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, fmt.Errorf("Rerank is not supported by MoonshotAI")
}

// ListModels gets model list
func (a *MoonshotAIAdapter) ListModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
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
			Created:      m.Created,
			OwnedBy:      m.OwnedBy,
			Capabilities: []string{"chat"},
		}

		// Set Moonshotai model information
		a.enrichModelInfo(&model)

		models = append(models, model)
	}

	return models, nil
}

// GetBalance gets balance information
func (a *MoonshotAIAdapter) GetBalance(ctx context.Context, apiKey string) (*adapter.Balance, error) {
	url := "https://api.moonshot.ai/v1/users/me/balance"
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

	var balanceResp struct {
		Code   int    `json:"code"`
		Scode  string `json:"scode"`
		Status bool   `json:"status"`
		Data   struct {
			AvailableBalance decimal.Decimal `json:"available_balance"`
			VoucherBalance   decimal.Decimal `json:"voucher_balance"`
			CashBalance      decimal.Decimal `json:"cash_balance"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &balanceResp); err != nil {
		return nil, fmt.Errorf("failed to parse balance: %w", err)
	}

	if !balanceResp.Status || balanceResp.Code != 0 {
		return nil, fmt.Errorf("balance query failed: code=%d, scode=%s", balanceResp.Code, balanceResp.Scode)
	}

	return &adapter.Balance{
		Total:     balanceResp.Data.AvailableBalance,
		Used:      decimal.Zero, // Moonshotai API does not provide used amount
		Remaining: balanceResp.Data.AvailableBalance,
		Currency:  "USD",
	}, nil
}

// ValidateConfig validates configuration
func (a *MoonshotAIAdapter) ValidateConfig(config *adapter.AdapterConfig) error {
	return validateMoonshotAIConfig(config)
}

// GetProviderInfo gets provider information
func (a *MoonshotAIAdapter) GetProviderInfo() *adapter.ProviderInfo {
	return &adapter.ProviderInfo{
		Name:         "moonshotai",
		Type:         "openai_compatible",
		DisplayName:  "Moonshotai",
		Description:  "Moonshotai AI models",
		BaseURL:      a.baseURL,
		Capabilities: []string{"chat", "completion", "embedding"},
		Version:      "v1",
	}
}

// buildHeaders builds request headers
func (a *MoonshotAIAdapter) buildHeaders() map[string]string {
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", a.config.APIKey),
	}

	for k, v := range a.config.Headers {
		headers[k] = v
	}

	return headers
}

// handleError handles error response
func (a *MoonshotAIAdapter) handleError(statusCode int, body []byte) error {
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
func (a *MoonshotAIAdapter) enrichModelInfo(model *adapter.Model) {
	// Set default context length for Moonshotai models
	// You can customize this based on actual model specifications
	switch model.ID {
	case "moonshot-v1-8k":
		model.ContextLength = 8192
		model.Description = "Moonshotai 8K context model"
	case "moonshot-v1-32k":
		model.ContextLength = 32768
		model.Description = "Moonshotai 32K context model"
	case "moonshot-v1-128k":
		model.ContextLength = 131072
		model.Description = "Moonshotai 128K context model"
	default:
		model.ContextLength = 8192
	}
}
