package pricing

import (
	"encoding/json"
	"testing"
)

func TestMergeOrganizationPricingOverridePartialOperation(t *testing.T) {
	base := json.RawMessage(`{
		"version":2,
		"rules":[
			{"id":"chat-input","operation":"chat","meter":"input_token","unit":"per_1m_tokens","price":"1","configured":true},
			{"id":"image-default","operation":"image_generation","meter":"image","unit":"per_image","price":"0.04","configured":true}
		]
	}`)
	override := json.RawMessage(`{
		"rules":[
			{"id":"image-default","operation":"image_generation","meter":"image","unit":"per_image","price":"0.08","configured":true}
		]
	}`)

	merged := MergeOrganizationPricingOverride(base, override, "USD")
	var envelope structuredPricing
	if err := json.Unmarshal(merged, &envelope); err != nil {
		t.Fatalf("unmarshal merged pricing: %v", err)
	}
	if len(envelope.Rules) != 2 {
		t.Fatalf("rules len = %d, want 2", len(envelope.Rules))
	}
	prices := map[string]string{}
	for _, rule := range envelope.Rules {
		prices[rule.ID] = rule.Price
	}
	if prices["chat-input"] != "1" {
		t.Fatalf("chat price = %q, want 1", prices["chat-input"])
	}
	if prices["image-default"] != "0.08" {
		t.Fatalf("image price = %q, want 0.08", prices["image-default"])
	}
	if envelope.PricePerImage == nil || *envelope.PricePerImage != 0.08 {
		t.Fatalf("derived price_per_image = %v, want 0.08", envelope.PricePerImage)
	}
}

func TestMergeOrganizationPricingOverrideIsIdempotent(t *testing.T) {
	base := json.RawMessage(`{
		"version":2,
		"rules":[
			{"id":"chat-input","operation":"chat","meter":"input_token","unit":"per_1m_tokens","price":"1","configured":true},
			{"id":"video-720p","operation":"video_generation","meter":"output_video_second","unit":"second","price":"0.02","configured":true,"conditions":{"resolution":"720p"}}
		]
	}`)
	override := json.RawMessage(`{
		"rules":[
			{"id":"video-720p","price":"0.05","configured":true}
		]
	}`)

	first := MergeOrganizationPricingOverride(base, override, "USD")
	second := MergeOrganizationPricingOverride(first, override, "USD")
	if string(first) != string(second) {
		t.Fatalf("second merge changed pricing:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestMergeOrganizationPricingOverrideMatchesStableIdentity(t *testing.T) {
	base := json.RawMessage(`{
		"version":2,
		"rules":[
			{"operation":"video_generation","meter":"output_video_second","unit":"second","price":"0.01","configured":true,"conditions":{"resolution":"480p","audio":"false"}}
		]
	}`)
	override := json.RawMessage(`{
		"rules":[
			{"operation":"video_generation","meter":"output_video_second","unit":"second","price":"0.03","configured":true,"conditions":{"resolution":"480p","audio":"false"}}
		]
	}`)

	merged := MergeOrganizationPricingOverride(base, override, "USD")
	var envelope structuredPricing
	if err := json.Unmarshal(merged, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Rules) != 1 {
		t.Fatalf("rules len = %d, want 1", len(envelope.Rules))
	}
	if envelope.Rules[0].Price != "0.03" {
		t.Fatalf("merged price = %q, want 0.03", envelope.Rules[0].Price)
	}
}
