package migrations

import (
	"encoding/json"
	"testing"
)

func float64Ptr(value float64) *float64 { return &value }

func TestBuildPriceConfiguredBackfillPricingProjectsConfiguredScalars(t *testing.T) {
	row := priceConfiguredBackfillRow{
		ID:              "model-1",
		InputPrice:      float64Ptr(1.5),
		OutputPrice:     float64Ptr(2.5),
		InputConfigured: true, OutputConfigured: true,
	}
	next, ok, err := buildPriceConfiguredBackfillPricing(row, "pre_custom")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected backfill for configured scalars")
	}
	var envelope struct {
		Version       int    `json:"version"`
		CatalogOrigin string `json:"catalog_origin"`
		Rules         []struct {
			Meter      string `json:"meter"`
			Price      string `json:"price"`
			Configured *bool  `json:"configured"`
		} `json:"rules"`
		TokenTiers []struct {
			InputPricePerMillion  *float64 `json:"input_price_per_million"`
			OutputPricePerMillion *float64 `json:"output_price_per_million"`
		} `json:"token_tiers"`
	}
	if err := json.Unmarshal(next, &envelope); err != nil {
		t.Fatalf("unmarshal pricing: %v (%s)", err, next)
	}
	if envelope.CatalogOrigin != "pre_custom" {
		t.Fatalf("catalog_origin = %q, want pre_custom", envelope.CatalogOrigin)
	}
	if len(envelope.TokenTiers) != 1 || envelope.TokenTiers[0].InputPricePerMillion == nil || *envelope.TokenTiers[0].InputPricePerMillion != 1.5 {
		t.Fatalf("token tiers = %+v, want input 1.5", envelope.TokenTiers)
	}
	configured := map[string]string{}
	for _, rule := range envelope.Rules {
		if rule.Configured == nil || !*rule.Configured {
			continue
		}
		configured[rule.Meter] = rule.Price
	}
	if configured["input_token"] != "1.5" || configured["output_token"] != "2.5" {
		t.Fatalf("configured rules = %+v, want input/output 1.5/2.5", configured)
	}
}

func TestBuildPriceConfiguredBackfillPricingKeepsExplicitZeroAsFree(t *testing.T) {
	zero := 0.0
	row := priceConfiguredBackfillRow{
		ID:              "model-free",
		InputPrice:      &zero,
		OutputPrice:     &zero,
		InputConfigured: true, OutputConfigured: true,
	}
	next, ok, err := buildPriceConfiguredBackfillPricing(row, "")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("explicit zero price must be projected as a free rule")
	}
	var envelope struct {
		Rules []struct {
			Meter      string `json:"meter"`
			Price      string `json:"price"`
			Configured *bool  `json:"configured"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(next, &envelope); err != nil {
		t.Fatal(err)
	}
	for _, rule := range envelope.Rules {
		if rule.Meter == "input_token" {
			if rule.Price != "0" || rule.Configured == nil || !*rule.Configured {
				t.Fatalf("input rule = %+v, want configured free (price 0)", rule)
			}
			return
		}
	}
	t.Fatalf("no input_token rule in %s", next)
}

func TestBuildPriceConfiguredBackfillPricingSkipsRowsWithoutConfiguredMeters(t *testing.T) {
	row := priceConfiguredBackfillRow{ID: "model-2"}
	if _, ok, err := buildPriceConfiguredBackfillPricing(row, ""); err != nil || ok {
		t.Fatalf("ok=%v err=%v, want skip", ok, err)
	}
}

func TestBuildPriceConfiguredBackfillPricingIsIdempotent(t *testing.T) {
	row := priceConfiguredBackfillRow{
		ID:              "model-3",
		InputPrice:      float64Ptr(1),
		OutputPrice:     float64Ptr(2),
		InputConfigured: true, OutputConfigured: true,
		Pricing: []byte(`{"version":2,"rules":[
			{"id":"chat-input","operation":"chat","meter":"input_token","unit":"per_1m_tokens","price":"1","configured":true},
			{"id":"chat-output","operation":"chat","meter":"output_token","unit":"per_1m_tokens","price":"2","configured":true}
		]}`),
	}
	next, ok, err := buildPriceConfiguredBackfillPricing(row, "")
	if err != nil {
		t.Fatal(err)
	}
	if ok || next != nil {
		t.Fatalf("second run must be a no-op, got ok=%v pricing=%s", ok, next)
	}
}
