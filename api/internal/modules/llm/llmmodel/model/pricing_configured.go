package model

import (
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/pricing"
)

// PopulateConfiguredFromPricing derives API/billing configured flags from pricing JSON.
func (m *LLMModel) PopulateConfiguredFromPricing() {
	if m == nil {
		return
	}
	applyResolvedConfigured(&m.InputPriceConfigured, &m.OutputPriceConfigured, &m.CacheReadPriceConfigured, &m.CacheWritePriceConfigured, &m.CacheWrite5mPriceConfigured, &m.CacheWrite1hPriceConfigured, m.Pricing)
}

// PopulateConfiguredFromPricing derives API/billing configured flags from pricing JSON.
func (m *CustomModel) PopulateConfiguredFromPricing() {
	if m == nil {
		return
	}
	applyResolvedConfigured(&m.InputPriceConfigured, &m.OutputPriceConfigured, &m.CacheReadPriceConfigured, &m.CacheWritePriceConfigured, &m.CacheWrite5mPriceConfigured, &m.CacheWrite1hPriceConfigured, m.Pricing)
}

func applyResolvedConfigured(
	input, output, cacheRead, cacheWrite, cacheWrite5m, cacheWrite1h *bool,
	raw []byte,
) {
	resolved := pricing.ResolveConfigured(raw)
	*input = resolved.Input
	*output = resolved.Output
	*cacheRead = resolved.CacheRead
	*cacheWrite = resolved.CacheWrite
	*cacheWrite5m = resolved.CacheWrite5m
	*cacheWrite1h = resolved.CacheWrite1h
}
