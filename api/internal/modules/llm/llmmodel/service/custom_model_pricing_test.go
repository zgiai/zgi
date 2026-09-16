package service

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	modelpricing "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/pricing"
)

func TestApplyCustomModelPricingPersistsRulesAndScalars(t *testing.T) {
	custom := &model.CustomModel{Currency: "USD"}
	custom.InputPrice = decimal.RequireFromString("1.25")
	custom.OutputPrice = decimal.RequireFromString("2.5")
	custom.InputPriceConfigured = true
	custom.OutputPriceConfigured = true

	if err := applyCustomModelPricing(custom, nil, []string{"text-chat"}); err != nil {
		t.Fatalf("applyCustomModelPricing returned error: %v", err)
	}
	if len(custom.Pricing) == 0 {
		t.Fatal("expected pricing JSON to be persisted")
	}
	resolved := modelpricing.ResolveConfigured([]byte(custom.Pricing))
	if !resolved.Input || !resolved.Output {
		t.Fatalf("resolved configured = %+v, want input/output true", resolved)
	}
	if !custom.InputPrice.Equal(decimal.RequireFromString("1.25")) {
		t.Fatalf("input price = %s", custom.InputPrice)
	}
}

func TestResolveConfiguredTreatsExplicitZeroAsConfigured(t *testing.T) {
	custom := &model.CustomModel{Currency: "USD"}
	custom.InputPriceConfigured = true
	custom.OutputPriceConfigured = true

	if err := applyCustomModelPricing(custom, nil, []string{"text-chat"}); err != nil {
		t.Fatalf("applyCustomModelPricing returned error: %v", err)
	}
	custom.InputPrice = decimal.Zero
	custom.OutputPrice = decimal.Zero
	if err := applyCustomModelPricing(custom, []byte(`{
		"deployment_scope": "organization",
		"rules": [{
			"id": "chat-input",
			"operation": "chat",
			"meter": "input_token",
			"unit": "per_1m_tokens",
			"price": "0",
			"configured": true
		}]
	}`), []string{"text-chat"}); err != nil {
		t.Fatalf("applyCustomModelPricing returned error: %v", err)
	}
	resolved := modelpricing.ResolveConfigured([]byte(custom.Pricing))
	if !resolved.Input {
		t.Fatalf("input configured = false, want explicit free price")
	}
}
