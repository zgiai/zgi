package pricing

import (
	"encoding/json"
	"testing"
)

func TestMissingPricingForUseCasesRealtimeAudioModes(t *testing.T) {
	// Text mode: no realtime_audio rule, so chat input/output tokens are required.
	textMode := MissingPricingForUseCases([]string{"realtime-audio"}, json.RawMessage(`{"rules":[]}`), LegacyScalarPrices{})
	if len(textMode) != 2 || textMode[0].Operation != "chat" || textMode[0].Meter != "input_token" {
		t.Fatalf("text mode missing = %#v, want chat input/output", textMode)
	}

	// Audio mode: a priced realtime_audio input rule switches the requirement.
	audioMode := MissingPricingForUseCases([]string{"realtime-audio"}, json.RawMessage(`{
		"rules":[{"operation":"realtime_audio","meter":"input_token","unit":"per_1m_tokens","price":"32","configured":true}]
	}`), LegacyScalarPrices{})
	if len(audioMode) != 1 || audioMode[0].Operation != "realtime_audio" || audioMode[0].Meter != "output_token" {
		t.Fatalf("audio mode missing = %#v, want realtime_audio/output_token", audioMode)
	}

	// Chat scalar columns must not stand in for audio tokens.
	input := 1.0
	output := 2.0
	scalarOnly := MissingPricingForUseCases([]string{"realtime-audio"}, json.RawMessage(`{
		"rules":[{"operation":"realtime_audio","meter":"input_token","unit":"per_1m_tokens","price":"32","configured":true}]
	}`), LegacyScalarPrices{InputPrice: &input, OutputPrice: &output})
	if len(scalarOnly) != 1 || scalarOnly[0].Meter != "output_token" {
		t.Fatalf("scalar-only missing = %#v, want realtime_audio/output_token", scalarOnly)
	}

	complete := MissingPricingForUseCases([]string{"realtime-audio"}, json.RawMessage(`{
		"rules":[
			{"operation":"realtime_audio","meter":"input_token","unit":"per_1m_tokens","price":"32","configured":true},
			{"operation":"realtime_audio","meter":"output_token","unit":"per_1m_tokens","price":"64","configured":true}
		]
	}`), LegacyScalarPrices{})
	if len(complete) != 0 {
		t.Fatalf("complete missing = %#v, want none", complete)
	}
}

func TestMissingPricingForUseCasesVideoRequiresPricedRate(t *testing.T) {
	empty := MissingPricingForUseCases([]string{"video-gen"}, json.RawMessage(`{
		"video_generation":{"currency":"USD","billing_unit":"second","rates":[{"resolution":"720p"}]}
	}`), LegacyScalarPrices{})
	if len(empty) != 1 || empty[0].Meter != "video" {
		t.Fatalf("empty video missing = %#v, want video requirement", empty)
	}

	priced := MissingPricingForUseCases([]string{"video-gen"}, json.RawMessage(`{
		"video_generation":{"currency":"USD","billing_unit":"second","rates":[{"resolution":"720p","price_per_second":0.8}]}
	}`), LegacyScalarPrices{})
	if len(priced) != 0 {
		t.Fatalf("priced video missing = %#v, want none", priced)
	}
}

func TestValidateStructuredPricingAcceptsRealtimeAudioRules(t *testing.T) {
	present, err := ValidateStructuredPricing(json.RawMessage(`{
		"deployment_scope":"global",
		"rules":[
			{"operation":"realtime_audio","meter":"input_token","unit":"per_1m_tokens","price":"32","configured":true},
			{"operation":"realtime_audio","meter":"output_token","unit":"per_1m_tokens","price":"64","configured":true}
		]
	}`))
	if err != nil {
		t.Fatalf("ValidateStructuredPricing error = %v", err)
	}
	if !present {
		t.Fatal("ValidateStructuredPricing = not present, want present")
	}
}
