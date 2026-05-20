package gateway

import (
	"strings"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

const cursorModelPrefix = "cursor-"

func normalizeRequestedModelName(modelName string) string {
	normalized := strings.TrimSpace(modelName)
	if strings.HasPrefix(normalized, cursorModelPrefix) {
		normalized = strings.TrimSpace(strings.TrimPrefix(normalized, cursorModelPrefix))
	}
	return normalized
}

func cloneChatRequestWithNormalizedModel(req *adapter.ChatRequest) *adapter.ChatRequest {
	if req == nil {
		return nil
	}

	cloned := *req
	cloned.Model = normalizeRequestedModelName(req.Model)
	return &cloned
}
