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

// Clearing the custom model's image price must drop the stored price. The
// dialog rebuilds the payload from the form, so a cleared field arrives as an
// explicit null with no rules[]; before this was handled the payload produced a
// valueless envelope that the strict validator rejected, leaving the stored
// price untouched.
func TestPrepareCustomModelPricingClearedImagePriceYieldsEmptyPricing(t *testing.T) {
	raw := json.RawMessage(`{"deployment_scope":"global","catalog_origin":"pre_custom","price_per_image":null}`)

	prepared, _, err := PrepareCustomModelPricing(raw, "USD", []string{"image-gen"}, CustomModelScalars{}, CustomModelScalars{})
	if err != nil {
		t.Fatalf("PrepareCustomModelPricing returned error: %v", err)
	}
	// "{}" is this package's "not configured" value: it carries no rules and no
	// price projections, so nothing can keep billing the cleared price.
	if string(prepared) != "{}" {
		t.Fatalf("prepared = %s, want {} once the last price is cleared", prepared)
	}
	var envelope structuredPricing
	if err := json.Unmarshal(prepared, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Rules) != 0 || envelope.PricePerImage != nil || envelope.VideoGeneration != nil {
		t.Fatalf("cleared pricing still carries a price: %s", prepared)
	}
}

// A cleared image price must not disturb the token prices that remain.
func TestPrepareCustomModelPricingClearKeepsTokenPrices(t *testing.T) {
	raw := json.RawMessage(`{"deployment_scope":"global","catalog_origin":"pre_custom","price_per_image":null}`)
	scalars := CustomModelScalars{InputPrice: floatPtr(1), OutputPrice: floatPtr(2)}

	prepared, _, err := PrepareCustomModelPricing(raw, "USD", []string{"text-chat", "image-gen"}, scalars, CustomModelScalars{})
	if err != nil {
		t.Fatalf("PrepareCustomModelPricing returned error: %v", err)
	}
	var envelope structuredPricing
	if err := json.Unmarshal(prepared, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.PricePerImage != nil {
		t.Fatalf("price_per_image = %v, want cleared", envelope.PricePerImage)
	}
	for _, rule := range envelope.Rules {
		if rule.Operation == "image_generation" {
			t.Fatalf("image rule present after a cleared image price: %+v", rule)
		}
	}
	if len(envelope.Rules) == 0 {
		t.Fatal("token rules must survive clearing the image price")
	}
}
