package provider

import (
	"context"
	"fmt"
	"path"
	"strings"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

type openAIAnthropicCompatAdapter struct {
	openAI       *OpenAIAdapter
	providerName string
}

func newOpenAIAnthropicCompatAdapter(config *adapter.AdapterConfig, providerName string, defaultBaseURL string) (*openAIAnthropicCompatAdapter, error) {
	if err := validateOpenAIConfig(config); err != nil {
		return nil, err
	}

	cfg := *config
	cfg.ProviderName = providerName
	cfg.BaseURL = ensureVersionedV1BaseURL(config.BaseURL, defaultBaseURL)

	openAIAdapter, err := NewOpenAIAdapter(&cfg)
	if err != nil {
		return nil, err
	}

	return &openAIAnthropicCompatAdapter{
		openAI:       openAIAdapter,
		providerName: providerName,
	}, nil
}

func ensureVersionedV1BaseURL(raw string, defaultBaseURL string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(raw), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(defaultBaseURL), "/")
	}
	if hasVersionedBasePath(baseURL) {
		return baseURL
	}
	return baseURL + "/v1"
}

func hasVersionedBasePath(baseURL string) bool {
	lastSegment := path.Base(strings.TrimSpace(baseURL))
	if len(lastSegment) < 2 || lastSegment[0] != 'v' {
		return false
	}
	return lastSegment[1] >= '0' && lastSegment[1] <= '9'
}

func (a *openAIAnthropicCompatAdapter) ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return a.openAI.ChatCompletion(ctx, request)
}

func (a *openAIAnthropicCompatAdapter) ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return a.openAI.ChatCompletionStream(ctx, request)
}

func (a *openAIAnthropicCompatAdapter) CreateResponse(ctx context.Context, request *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return a.openAI.CreateResponse(ctx, request)
}

func (a *openAIAnthropicCompatAdapter) CreateResponseRaw(ctx context.Context, request *adapter.RawResponseRequest) (*adapter.RawResponse, error) {
	return a.openAI.CreateResponseRaw(ctx, request)
}

func (a *openAIAnthropicCompatAdapter) CreateResponseStream(ctx context.Context, request *adapter.RawResponseRequest) (<-chan adapter.RawStreamEvent, error) {
	return a.openAI.CreateResponseStream(ctx, request)
}

func (a *openAIAnthropicCompatAdapter) CreateAnthropicMessage(ctx context.Context, request *adapter.AnthropicMessageRequest) (*adapter.RawResponse, error) {
	return rawAnthropicMessageRequest(
		ctx,
		a.openAI.httpClient,
		a.openAI.baseURL,
		buildAnthropicRawHeaders(a.openAI.config, request.Headers),
		request,
		a.openAI.handleError,
	)
}

func (a *openAIAnthropicCompatAdapter) CreateAnthropicMessageStream(ctx context.Context, request *adapter.AnthropicMessageRequest) (<-chan adapter.RawStreamEvent, error) {
	return rawAnthropicMessageStream(
		ctx,
		a.openAI.httpClient,
		a.openAI.baseURL,
		buildAnthropicRawHeaders(a.openAI.config, request.Headers),
		request,
	)
}

func (a *openAIAnthropicCompatAdapter) CreateEmbeddings(ctx context.Context, request *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return a.openAI.CreateEmbeddings(ctx, request)
}

func (a *openAIAnthropicCompatAdapter) CreateImage(ctx context.Context, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return a.openAI.CreateImage(ctx, request)
}

func (a *openAIAnthropicCompatAdapter) Rerank(ctx context.Context, request *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return a.openAI.Rerank(ctx, request)
}

func (a *openAIAnthropicCompatAdapter) ListModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
	return a.openAI.ListModels(ctx, apiKey)
}

func (a *openAIAnthropicCompatAdapter) GetBalance(ctx context.Context, apiKey string) (*adapter.Balance, error) {
	return a.openAI.GetBalance(ctx, apiKey)
}

func (a *openAIAnthropicCompatAdapter) ValidateConfig(config *adapter.AdapterConfig) error {
	return validateOpenAIConfig(config)
}

func (a *openAIAnthropicCompatAdapter) GetProviderInfo() *adapter.ProviderInfo {
	info := a.openAI.GetProviderInfo()
	if info == nil {
		return nil
	}
	copied := *info
	copied.Name = a.providerName
	copied.Type = a.providerName
	copied.DisplayName = strings.ToUpper(a.providerName)
	return &copied
}

func (a *openAIAnthropicCompatAdapter) unsupportedResponsesError() error {
	return fmt.Errorf("%w: %s /responses API is not documented", adapter.ErrCapabilityUnsupported, strings.ToUpper(a.providerName))
}
