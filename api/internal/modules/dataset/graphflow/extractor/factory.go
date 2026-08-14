package extractor

import (
	"github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
)

// StrategyLLM is the only supported GraphFlow extraction strategy.
const StrategyLLM = "llm"

// NewExtractorByStrategy always returns the LLM extractor. The strategy
// parameter remains in the signature for compatibility with persisted tasks
// created before GraphFlow standardized on LLM extraction.
func NewExtractorByStrategy(_ string, llmClient client.LLMClient, defaultModelSvc llmdefaultservice.DefaultModelService, model *string, provider *string) Extractor {
	return NewLLMExtractor(llmClient, defaultModelSvc, model, provider)
}
