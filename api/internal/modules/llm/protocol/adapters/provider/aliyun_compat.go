package provider

import (
	"context"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters/provider/qwen"
)

// AliyunAdapter preserves the public provider type while the Qwen-specific
// implementation lives in its own package. Raw protocol helpers remain here
// because they are shared by multiple provider adapters.
type AliyunAdapter struct {
	*qwen.Adapter
	config     *adapter.AdapterConfig
	httpClient *adapter.HTTPClient
	openAI     *OpenAIAdapter
}

// NewAliyunAdapter preserves the legacy constructor for existing callers.
func NewAliyunAdapter(config *adapter.AdapterConfig) (*AliyunAdapter, error) {
	qwenAdapter, err := qwen.NewAdapter(config, qwen.Dependencies{
		NewCompatibleClient: func(base *adapter.AdapterConfig, baseURL string) (qwen.CompatibleClient, error) {
			return newOpenAIAdapterWithOverrides(base, baseURL)
		},
	})
	if err != nil {
		return nil, err
	}

	openAI, err := newOpenAIAdapterWithOverrides(config, qwenAdapter.OpenAICompatibleBaseURL())
	if err != nil {
		return nil, err
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	return &AliyunAdapter{
		Adapter:    qwenAdapter,
		config:     config,
		httpClient: adapter.NewHTTPClientFromConfig(config, timeout, 3),
		openAI:     openAI,
	}, nil
}

func (a *AliyunAdapter) CreateResponseRaw(ctx context.Context, request *adapter.RawResponseRequest) (*adapter.RawResponse, error) {
	return rawOpenAIResponseRequest(
		ctx,
		a.httpClient,
		a.OpenAICompatibleBaseURL(),
		a.openAI.buildHeaders(),
		request,
		a.HandleError,
	)
}

func (a *AliyunAdapter) CreateResponseStream(ctx context.Context, request *adapter.RawResponseRequest) (<-chan adapter.RawStreamEvent, error) {
	return rawOpenAIResponseStream(
		ctx,
		a.httpClient,
		a.OpenAICompatibleBaseURL(),
		a.openAI.buildHeaders(),
		request,
		a.HandleError,
	)
}

func (a *AliyunAdapter) CreateAnthropicMessage(ctx context.Context, request *adapter.AnthropicMessageRequest) (*adapter.RawResponse, error) {
	return rawAnthropicMessageRequest(
		ctx,
		a.httpClient,
		a.AnthropicMessagesBaseURL(),
		buildAnthropicRawHeaders(a.config, request.Headers),
		request,
		a.openAI.handleError,
	)
}

func (a *AliyunAdapter) CreateAnthropicMessageStream(ctx context.Context, request *adapter.AnthropicMessageRequest) (<-chan adapter.RawStreamEvent, error) {
	return rawAnthropicMessageStream(
		ctx,
		a.httpClient,
		a.AnthropicMessagesBaseURL(),
		buildAnthropicRawHeaders(a.config, request.Headers),
		request,
		a.HandleError,
	)
}
