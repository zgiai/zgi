package gateway

import (
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	// Values below this are treated as explicit truncation tests (LLM-058).
	explicitTruncationTokenCap = 32
	// Reasoning models count hidden thinking against max_tokens. A short
	// client cap such as 120/240 leaves no room for a visible answer or JSON.
	reasoningUpstreamCompletionFloor = 1024
	jsonObjectCompletionFloor        = 256
)

func shapeChatRequestForUpstream(req *adapter.ChatRequest, selection *ProviderSelection) *adapter.ChatRequest {
	if req == nil {
		return nil
	}
	cloned := *req
	applyChatCompletionBudgetFloor(&cloned, selection)
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
