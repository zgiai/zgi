package provider

import (
	"context"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const defaultSiliconFlowBaseURL = "https://api.siliconflow.com/v1"

// SiliconFlowAdapter wraps OpenAI-compatible behavior and adds native Anthropic Messages.
type SiliconFlowAdapter struct {
	*openAIAnthropicCompatAdapter
}

func NewSiliconFlowAdapter(config *adapter.AdapterConfig) (*SiliconFlowAdapter, error) {
	compat, err := newOpenAIAnthropicCompatAdapter(config, "siliconflow", defaultSiliconFlowBaseURL)
	if err != nil {
		return nil, err
	}
	return &SiliconFlowAdapter{openAIAnthropicCompatAdapter: compat}, nil
}

func (a *SiliconFlowAdapter) CreateResponse(ctx context.Context, request *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, a.unsupportedResponsesError()
}

func (a *SiliconFlowAdapter) CreateResponseRaw(ctx context.Context, request *adapter.RawResponseRequest) (*adapter.RawResponse, error) {
	return nil, a.unsupportedResponsesError()
}

func (a *SiliconFlowAdapter) CreateResponseStream(ctx context.Context, request *adapter.RawResponseRequest) (<-chan adapter.RawStreamEvent, error) {
	return nil, a.unsupportedResponsesError()
}
