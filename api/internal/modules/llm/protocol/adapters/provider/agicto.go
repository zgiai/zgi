package provider

import (
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const defaultAgictoBaseURL = "https://api.agicto.cn/v1"

// AgictoAdapter wraps OpenAI-compatible behavior and adds native Anthropic Messages.
type AgictoAdapter struct {
	*openAIAnthropicCompatAdapter
}

func NewAgictoAdapter(config *adapter.AdapterConfig) (*AgictoAdapter, error) {
	compat, err := newOpenAIAnthropicCompatAdapter(config, "agicto", defaultAgictoBaseURL)
	if err != nil {
		return nil, err
	}
	return &AgictoAdapter{openAIAnthropicCompatAdapter: compat}, nil
}
