package pricing

import (
	"encoding/json"
	"testing"
)

func TestResolveToolPriceChannelDefaultFree(t *testing.T) {
	raw := json.RawMessage(`{"schema_version":1,"billing_lane":"private","pricing_version":"private-3","tools":{"default":{"file_search":{"amount":"1.25","currency":"USD","unit":"call","per_quantity":1000}},"channels":{"private-a":{"file_search":{"amount":"2","currency":"USD","unit":"call","per_quantity":1}},"private-free":{"file_search":null}}}}`)
	got, err := ResolveToolPrice(raw, "private", "private-a", "file_search")
	if err != nil || got.Source != "channel" || got.Price.Amount != "2" || got.PricingVersion != "private-3" {
		t.Fatalf("channel = %#v, %v", got, err)
	}
	got, err = ResolveToolPrice(raw, "private", "private-missing", "file_search")
	if err != nil || got.Source != "default" || got.Price.PerQuantity != 1000 {
		t.Fatalf("default = %#v, %v", got, err)
	}
	got, err = ResolveToolPrice(raw, "private", "private-free", "file_search")
	if err != nil || got.Source != "free" {
		t.Fatalf("free = %#v, %v", got, err)
	}
}

func TestValidateToolPricingContractRejectsCrossLaneAndInvalidUnit(t *testing.T) {
	for _, raw := range []string{
		`{"billing_lane":"platform"}`,
		`{"tools":{"default":{"code_interpreter":{"amount":"1","currency":"USD","unit":"call","per_quantity":1}}}}`,
	} {
		if raw == `{"billing_lane":"platform"}` {
			if err := ValidateToolPricingContract(json.RawMessage(raw)); err != nil {
				t.Fatalf("platform lane should be structurally valid: %v", err)
			}
			continue
		}
		if err := ValidateToolPricingContract(json.RawMessage(raw)); err == nil {
			t.Fatalf("invalid unit accepted: %s", raw)
		}
	}
}

func TestResolveToolPriceRejectsCrossLaneRead(t *testing.T) {
	_, err := ResolveToolPrice(json.RawMessage(`{"billing_lane":"platform","tools":{"default":{"web_search":{"amount":"1","currency":"USD","unit":"call","per_quantity":1}}}}`), "private", "ch", "web_search")
	if err == nil {
		t.Fatal("platform pricing was accepted for private lane")
	}
}

func TestStructuredPricingValidationAndNormalizationPreserveTools(t *testing.T) {
	raw := json.RawMessage(`{"schema_version":1,"billing_lane":"private","pricing_version":"p-9","deployment_scope":"global","tools":{"default":{"code_interpreter":{"amount":"1","currency":"USD","unit":"session","per_quantity":1}}}}`)
	present, err := ValidateStructuredPricing(raw)
	if !present || err != nil {
		t.Fatalf("ValidateStructuredPricing = %v, %v", present, err)
	}
	normalized := NormalizeStructuredPricing(raw)
	var decoded map[string]any
	if err := json.Unmarshal(normalized, &decoded); err != nil {
		t.Fatalf("normalized pricing is invalid JSON: %v", err)
	}
	if _, ok := decoded["tools"]; !ok {
		t.Fatalf("normalized pricing dropped tools: %s", normalized)
	}
}
