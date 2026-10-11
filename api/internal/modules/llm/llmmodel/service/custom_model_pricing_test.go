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

	if err := applyCustomModelPricing(custom, nil, []string{"text-chat"}, customModelScalarsFromModel(custom)); err != nil {
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

	if err := applyCustomModelPricing(custom, nil, []string{"text-chat"}, customModelScalarsFromModel(custom)); err != nil {
		t.Fatalf("applyCustomModelPricing returned error: %v", err)
	}
	custom.InputPrice = decimal.Zero
	custom.OutputPrice = decimal.Zero
	freeOverride := modelpricing.CustomModelScalars{InputPrice: float64Ptr(0), OutputPrice: float64Ptr(0)}
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
	}`), []string{"text-chat"}, freeOverride); err != nil {
		t.Fatalf("applyCustomModelPricing returned error: %v", err)
	}
	resolved := modelpricing.ResolveConfigured([]byte(custom.Pricing))
	if !resolved.Input {
		t.Fatalf("input configured = false, want explicit free price")
	}
}

// A scalar token price edit must win over the rules already stored on the
// model; otherwise editing input/output price in the dialog silently keeps the
// previous billed price.
func TestApplyCustomModelPricingScalarOverrideWinsOverStoredRules(t *testing.T) {
	custom := &model.CustomModel{Currency: "USD"}
	custom.Pricing = []byte(`{"version":2,"deployment_scope":"organization","rules":[
		{"id":"chat-input","operation":"chat","meter":"input_token","unit":"per_1m_tokens","price":"1","configured":true},
		{"id":"chat-output","operation":"chat","meter":"output_token","unit":"per_1m_tokens","price":"2","configured":true}
	]}`)
	custom.InputPrice = decimal.RequireFromString("9")
	custom.OutputPrice = decimal.RequireFromString("9")
	custom.InputPriceConfigured = true
	custom.OutputPriceConfigured = true

	overrides := modelpricing.CustomModelScalars{InputPrice: float64Ptr(9), OutputPrice: float64Ptr(9)}
	if err := applyCustomModelPricing(custom, custom.Pricing, []string{"text-chat"}, overrides); err != nil {
		t.Fatalf("applyCustomModelPricing returned error: %v", err)
	}
	if !custom.InputPrice.Equal(decimal.RequireFromString("9")) || !custom.OutputPrice.Equal(decimal.RequireFromString("9")) {
		t.Fatalf("derived scalars = %s/%s, want 9/9", custom.InputPrice, custom.OutputPrice)
	}
	if resolved := modelpricing.ResolveConfigured([]byte(custom.Pricing)); !resolved.Input || !resolved.Output {
		t.Fatalf("resolved configured = %+v, want input/output true", resolved)
	}
}

func float64Ptr(value float64) *float64 { return &value }

// Clearing every price on a custom model must persist "not configured" so the
// previous image price cannot keep billing.
func TestApplyCustomModelPricingClearsStoredPricingWhenEveryPriceIsCleared(t *testing.T) {
	custom := &model.CustomModel{Currency: "USD"}
	custom.Pricing = []byte(`{"version":2,"deployment_scope":"organization","catalog_origin":"pre_custom","rules":[{"id":"image-default","operation":"image_generation","meter":"image","unit":"per_image","price":"0.3","configured":true}],"price_per_image":0.3}`)

	// What the dialog now submits when the tenant empties the image price.
	raw := []byte(`{"deployment_scope":"global","catalog_origin":"pre_custom","price_per_image":null}`)
	if err := applyCustomModelPricing(custom, raw, []string{"image-gen"}, modelpricing.CustomModelScalars{}); err != nil {
		t.Fatalf("applyCustomModelPricing returned error: %v", err)
	}
	if string(custom.Pricing) != "{}" {
		t.Fatalf("pricing = %s, want {} (not configured)", custom.Pricing)
	}
	if resolved := modelpricing.ResolveConfigured([]byte(custom.Pricing)); resolved.Input || resolved.Output {
		t.Fatalf("resolved configured = %+v, want everything unconfigured", resolved)
	}
}
