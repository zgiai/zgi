package pricing

import (
	"encoding/json"
	"testing"
)

func TestResolveConfiguredFromExplicitRule(t *testing.T) {
	raw := json.RawMessage(`{
		"rules": [{
			"operation": "chat",
			"meter": "input_token",
			"unit": "per_1m_tokens",
			"price": "0",
			"configured": true
		}]
	}`)
	resolved := ResolveConfigured(raw)
	if !resolved.Input {
		t.Fatalf("input configured = false, want true for explicit free rule")
	}
}

func TestResolveConfiguredFromTokenTier(t *testing.T) {
	raw := json.RawMessage(`{
		"token_tiers": [{
			"min_input_tokens": 0,
			"input_price_per_million": 1.5,
			"output_price_per_million": 2.5
		}]
	}`)
	resolved := ResolveConfigured(raw)
	if !resolved.Input || !resolved.Output {
		t.Fatalf("resolved = %+v, want input and output configured", resolved)
	}
}

func TestResolveConfiguredMissingPricing(t *testing.T) {
	resolved := ResolveConfigured(json.RawMessage(`{}`))
	if resolved.Input || resolved.Output {
		t.Fatalf("resolved = %+v, want all false", resolved)
	}
}

func TestPrepareCustomModelPricingFromScalars(t *testing.T) {
	input := 1.25
	output := 2.5
	prepared, derived, err := PrepareCustomModelPricing(nil, "USD", []string{"text-chat"}, CustomModelScalars{
		InputPrice:  &input,
		OutputPrice: &output,
	}, CustomModelScalars{
		InputPrice:  &input,
		OutputPrice: &output,
	})
	if err != nil {
		t.Fatalf("PrepareCustomModelPricing returned error: %v", err)
	}
	if len(prepared) == 0 {
		t.Fatal("expected pricing JSON")
	}
	if derived.InputPrice == nil || *derived.InputPrice != input {
		t.Fatalf("derived input = %v, want %v", derived.InputPrice, input)
	}
	if derived.OutputPrice == nil || *derived.OutputPrice != output {
		t.Fatalf("derived output = %v, want %v", derived.OutputPrice, output)
	}
	resolved := ResolveConfigured(prepared)
	if !resolved.Input || !resolved.Output {
		t.Fatalf("resolved = %+v, want configured input/output", resolved)
	}
}

func TestApplyOrganizationOverrides(t *testing.T) {
	resolved := ApplyOrganizationOverrides(ResolvedConfigured{}, OrganizationOverrides{Output: true})
	if resolved.Output != true || resolved.Input != false {
		t.Fatalf("resolved = %+v", resolved)
	}
}
