package extractor

import "testing"

func TestNewExtractorByStrategyAlwaysUsesLLM(t *testing.T) {
	for _, strategy := range []string{"", StrategyLLM, "openie", "legacy-value"} {
		extractor := NewExtractorByStrategy(strategy, nil, nil, nil, nil)
		if _, ok := extractor.(*LLMExtractor); !ok {
			t.Fatalf("strategy %q returned %T, want *LLMExtractor", strategy, extractor)
		}
	}
}
