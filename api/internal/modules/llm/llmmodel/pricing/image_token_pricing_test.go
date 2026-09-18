package pricing

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResolveImageTokenPricingResolvesThreeTiers(t *testing.T) {
	raw := json.RawMessage(`{"deployment_scope":"global","image_token_pricing":{"currency":"USD","text_input_price_per_million":5,"image_input_price_per_million":8,"output_price_per_million":30}}`)
	price, present, err := ResolveImageTokenPricing(raw)
	if err != nil || !present {
		t.Fatalf("ResolveImageTokenPricing() present=%v err=%v", present, err)
	}
	if price.TextInputPricePerMillion != 5 || price.ImageInputPricePerMillion != 8 || price.OutputPricePerMillion != 30 {
		t.Fatalf("price = %+v, want 5/8/30", price)
	}
}

func TestPrepareStructuredPricingForWriteRoundTripsImageTokenPricing(t *testing.T) {
	in := json.RawMessage(`{"catalog_origin":"test","deployment_scope":"global","price_per_image":0.1,"image_token_pricing":{"currency":"USD","text_input_price_per_million":5,"image_input_price_per_million":8,"output_price_per_million":30}}`)
	out := PrepareStructuredPricingForWrite(in, "USD", LegacyScalarPrices{}, PricingWriteOptions{UserAuthored: true})

	price, present, err := ResolveImageTokenPricing(out)
	if err != nil || !present {
		t.Fatalf("round-trip present=%v err=%v pricing=%s", present, err, string(out))
	}
	if price.TextInputPricePerMillion != 5 || price.ImageInputPricePerMillion != 8 || price.OutputPricePerMillion != 30 {
		t.Fatalf("round-trip price = %+v, want 5/8/30", price)
	}
	if !strings.Contains(string(out), `"input_text_token"`) || !strings.Contains(string(out), `"input_image_token"`) {
		t.Fatalf("pricing rules missing image token meters: %s", string(out))
	}
	if !pricingHasAnyPrice(out) {
		t.Fatal("pricingHasAnyPrice() = false, want true for image token pricing")
	}
}

func TestValidateStructuredPricingRejectsNegativeImageTokenPrice(t *testing.T) {
	raw := json.RawMessage(`{"deployment_scope":"global","image_token_pricing":{"text_input_price_per_million":-1}}`)
	present, err := ValidateStructuredPricing(raw)
	if !present || err == nil {
		t.Fatalf("ValidateStructuredPricing() present=%v err=%v, want present with error", present, err)
	}
}
