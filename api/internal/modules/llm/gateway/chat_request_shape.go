package gateway

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/llm/channelprovider"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	// Values below this are treated as explicit truncation tests (LLM-058).
	explicitTruncationTokenCap = 32
	// Reasoning models count hidden thinking against max_tokens. A short
	// client cap such as 120/240 leaves no room for a visible answer or JSON.
	reasoningUpstreamCompletionFloor = 1024
	jsonObjectCompletionFloor        = 256

	// The Anthropic Messages API requires max_tokens, while OpenAI-compatible
	// upstreams use the model's own output limit when it is omitted. A request
	// without max_tokens therefore gets the model's catalog limit, bounded so
	// latency stays reasonable (zgi-pre#2914). The values follow Claude Code:
	// 32,000 by default, and at most 16,000 without streaming, where the
	// Anthropic SDKs expect long requests to stream to avoid HTTP timeouts.
	anthropicDefaultMaxTokens      = 32000
	anthropicNonStreamingMaxTokens = 16000
	anthropicUnknownModelMaxTokens = 8192
	anthropicMessagesAdapterKey    = "claude"
	zgiCloudAdapterKey             = "zgi-cloud"
)

func shapeChatRequestForUpstream(req *adapter.ChatRequest, selection *ProviderSelection, streaming bool) *adapter.ChatRequest {
	if req == nil {
		return nil
	}
	cloned := *req
	applyChatCompletionBudgetFloor(&cloned, selection)
	applyAnthropicDefaultMaxTokens(&cloned, selection, streaming)
	cloned.ToolChoice = adapter.NormalizeChatToolChoice(cloned.ToolChoice)
	return &cloned
}

func applyChatCompletionBudgetFloor(req *adapter.ChatRequest, selection *ProviderSelection) {
	if req == nil || req.MaxTokens == nil {
		return
	}
	requested := *req.MaxTokens
	if requested <= 0 || requested < explicitTruncationTokenCap {
		return
	}
	floor := requested
	if selection != nil && selection.Model.SupportsReasoning {
		floor = max(floor, reasoningUpstreamCompletionFloor)
	}
	if wantsJSONObject(req.ResponseFormat) {
		floor = max(floor, jsonObjectCompletionFloor)
	}
	if selection != nil && selection.Model.MaxOutputTokens > 0 && floor > selection.Model.MaxOutputTokens {
		floor = selection.Model.MaxOutputTokens
	}
	if floor == requested {
		return
	}
	req.MaxTokens = &floor
}

func wantsJSONObject(format *adapter.ResponseFormat) bool {
	if format == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(format.Type)) {
	case "json_object", "json_schema":
		return true
	default:
		return false
	}
}

// applyAnthropicDefaultMaxTokens fills max_tokens for a Claude request that
// sets none. It changes only the upstream request: the credit reservation is
// computed from the caller's request, where a missing max_tokens already
// reserves min(MaxOutputTokens, chatCompletionReservationCap).
func applyAnthropicDefaultMaxTokens(req *adapter.ChatRequest, selection *ProviderSelection, streaming bool) {
	if req == nil || selection == nil || (req.MaxTokens != nil && *req.MaxTokens > 0) {
		return
	}
	if !requiresAnthropicMaxTokens(selection) {
		return
	}
	limit := anthropicUnknownModelMaxTokens
	if selection.Model.MaxOutputTokens > 0 {
		limit = min(selection.Model.MaxOutputTokens, anthropicDefaultMaxTokens)
	}
	if !streaming {
		limit = min(limit, anthropicNonStreamingMaxTokens)
	}
	req.MaxTokens = &limit
}

// requiresAnthropicMaxTokens reports a request that reaches the Anthropic
// Messages API: a native Anthropic channel, or a Claude model on the platform
// channel, whose console route may be an Anthropic-protocol channel.
func requiresAnthropicMaxTokens(selection *ProviderSelection) bool {
	switch upstreamAdapterKey(selection) {
	case anthropicMessagesAdapterKey:
		return true
	case zgiCloudAdapterKey:
		return isClaudeModel(selection)
	default:
		return false
	}
}

// upstreamAdapterKey mirrors the adapter choice in createAdapterConfig.
func upstreamAdapterKey(selection *ProviderSelection) string {
	if override := strings.TrimSpace(selection.AdapterProviderOverride); override != "" {
		return strings.ToLower(override)
	}
	if spec, err := channelprovider.Resolve(selection.ChannelProvider); err == nil {
		return strings.ToLower(spec.AdapterKey)
	}
	return strings.ToLower(strings.TrimSpace(selection.ChannelProvider))
}

func isClaudeModel(selection *ProviderSelection) bool {
	for _, vendor := range []string{selection.ModelVendor, selection.Model.Provider} {
		switch strings.ToLower(strings.TrimSpace(vendor)) {
		case "anthropic", "claude":
			return true
		}
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(selection.Model.Model)), "claude")
}
