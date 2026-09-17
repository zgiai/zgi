package pricing

import (
	"encoding/json"
	"testing"
)

// The custom model dialog submits legacy projections (price_per_image /
// video_generation) even when the payload already carries rules[]; an edited
// projection must be folded into rules[] instead of being overwritten by the
// derived projection.
func TestPrepareCustomModelPricingImageProjectionOverride(t *testing.T) {
	raw := json.RawMessage(`{"version":2,"deployment_scope":"organization","rules":[
		{"id":"chat-input","operation":"chat","meter":"input_token","unit":"per_1m_tokens","price":"1","configured":false},
		{"id":"chat-output","operation":"chat","meter":"output_token","unit":"per_1m_tokens","price":"2","configured":false}
	],"price_per_image":0.1}`)
	scalars := CustomModelScalars{InputPrice: floatPtr(1), OutputPrice: floatPtr(2)}

	prepared, _, err := PrepareCustomModelPricing(raw, "USD", []string{"text-chat", "image-gen"}, scalars, CustomModelScalars{})
	if err != nil {
		t.Fatalf("PrepareCustomModelPricing returned error: %v", err)
	}
	var envelope structuredPricing
	if err := json.Unmarshal(prepared, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.PricePerImage == nil || *envelope.PricePerImage != 0.1 {
		t.Fatalf("price_per_image = %v, want 0.1", envelope.PricePerImage)
	}
	imageRules := 0
	for _, rule := range envelope.Rules {
		if rule.Operation == "image_generation" && rule.Meter == "image" {
			imageRules++
			if rule.Price != "0.1" {
				t.Fatalf("image rule price = %q, want 0.1", rule.Price)
			}
			if rule.Configured == nil || !*rule.Configured {
				t.Fatalf("image rule must be configured after an explicit edit: %+v", rule)
			}
		}
	}
	if imageRules != 1 {
		t.Fatalf("image rules = %d, want 1", imageRules)
	}
	if missing := MissingPricingForUseCases([]string{"text-chat", "image-gen"}, prepared, LegacyScalarPrices{}); len(missing) != 0 {
		t.Fatalf("missing pricing = %+v, want none", missing)
	}
}

func TestPrepareCustomModelPricingVideoProjectionOverride(t *testing.T) {
	raw := json.RawMessage(`{"version":2,"deployment_scope":"organization","rules":[
		{"id":"video-rate-0","operation":"video_generation","meter":"output_video_second","unit":"second","price":"0.01","configured":true,"conditions":{"resolution":"720p"}}
	],"video_generation":{"currency":"USD","billing_unit":"second","rates":[{"resolution":"720p","price_per_second":0.05}]}}`)

	prepared, _, err := PrepareCustomModelPricing(raw, "USD", []string{"video-gen"}, CustomModelScalars{}, CustomModelScalars{})
	if err != nil {
		t.Fatalf("PrepareCustomModelPricing returned error: %v", err)
	}
	var envelope structuredPricing
	if err := json.Unmarshal(prepared, &envelope); err != nil {
		t.Fatal(err)
	}
	var prices []string
	for _, rule := range envelope.Rules {
		if rule.Operation == "video_generation" {
			prices = append(prices, rule.Price)
		}
	}
	if len(prices) != 1 || prices[0] != "0.05" {
		t.Fatalf("video rule prices = %v, want [0.05]", prices)
	}
}
