package provider

import (
	"context"
	"fmt"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// Rerank reports that Ollama rerank is unsupported because Ollama has no standard rerank API.
func (a *OllamaAdapter) Rerank(context.Context, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, fmt.Errorf("%w: ollama rerank is not supported because ollama does not provide a standard rerank API", adapter.ErrCapabilityUnsupported)
}
