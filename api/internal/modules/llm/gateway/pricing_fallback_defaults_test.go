package gateway

import (
	"context"
	"testing"

	"github.com/google/uuid"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// builtInTokenFallbackPrices are the code defaults the panel shows out of the box.
var builtInTokenFallbackPrices = map[string]string{
	"token.chat.input.default":      "1",
	"token.chat.output.default":     "2",
	"token.embedding.input.default": "1",
	"token.rerank.input.default":    "1",
}

// builtInImageFallbackCredits mirrors the screenshot the product owner uses as
// the reference fallback table.
var builtInImageFallbackCredits = map[string]int64{
	"image.qwen.default":       160,
	"image.doubao.default":     100,
	"image.openai.default":     200,
	"image.gcp.default":        180,
	"image.midjourney.default": 300,
	"image.generic.default":    200,
}

func fallbackRulesByID(t *testing.T, rules []PricingFallbackRule) map[string]PricingFallbackRule {
	t.Helper()
	if len(rules) != 10 {
		t.Fatalf("rules = %d, want 10 fallback rules", len(rules))
	}
	byID := make(map[string]PricingFallbackRule, len(rules))
	for _, rule := range rules {
		if rule.ID == "" || rule.Operation == "" || rule.Meter == "" {
			t.Fatalf("rule %+v is missing identity", rule)
		}
		if rule.Enabled == nil || !*rule.Enabled {
			t.Fatalf("rule %s must stay enabled", rule.ID)
		}
		byID[rule.ID] = rule
	}
	return byID
}

// The built-in amounts must be present without configuring anything, so the
// fallback panel and the code-default quote path work on a fresh deployment.
func TestDefaultPricingFallbackRulesUseBuiltInDefaultsWithoutConfig(t *testing.T) {
	t.Setenv(PricingFallbackDefaultPricesEnv, "")
	byID := fallbackRulesByID(t, DefaultPricingFallbackRules())

	for id, want := range builtInTokenFallbackPrices {
		if got := byID[id].PriceUSDPer1MTokens; got != want {
			t.Fatalf("%s price = %q, want built-in %q", id, got, want)
		}
	}
	for id, want := range builtInImageFallbackCredits {
		if got := byID[id].Credits; got != want {
			t.Fatalf("%s credits = %d, want built-in %d", id, got, want)
		}
	}
}

// PRICING_FALLBACK_DEFAULT_PRICES stays an optional override: listed entries win,
// unlisted ones keep their built-in amount.
func TestDefaultPricingFallbackRulesApplyConfiguredPrices(t *testing.T) {
	t.Setenv(PricingFallbackDefaultPricesEnv, `{
		"token.chat.input.default": "1.5",
		"image.openai.default": 250,
		"image.generic.default": "200",
		"unknown.rule": 42
	}`)
	byID := fallbackRulesByID(t, DefaultPricingFallbackRules())

	if got := byID["token.chat.input.default"].PriceUSDPer1MTokens; got != "1.5" {
		t.Fatalf("chat input price = %q, want the configured override 1.5", got)
	}
	if got := byID["image.openai.default"].Credits; got != 250 {
		t.Fatalf("openai image credits = %d, want the configured override 250", got)
	}
	if got := byID["image.generic.default"].Credits; got != 200 {
		t.Fatalf("generic image credits = %d, want 200 from string form", got)
	}
	if got := byID["token.chat.output.default"].PriceUSDPer1MTokens; got != "2" {
		t.Fatalf("chat output price = %q, want the built-in 2 kept", got)
	}
	if got := byID["image.qwen.default"].Credits; got != 160 {
		t.Fatalf("qwen image credits = %d, want the built-in 160 kept", got)
	}
}

// A malformed override must not break startup: the built-in amounts still apply.
func TestDefaultPricingFallbackRulesTolerateInvalidConfig(t *testing.T) {
	t.Setenv(PricingFallbackDefaultPricesEnv, "{not-json")
	byID := fallbackRulesByID(t, DefaultPricingFallbackRules())

	if got := byID["token.chat.input.default"].PriceUSDPer1MTokens; got != "1" {
		t.Fatalf("chat input price = %q, want the built-in 1", got)
	}
	if got := byID["image.qwen.default"].Credits; got != 160 {
		t.Fatalf("qwen image credits = %d, want the built-in 160", got)
	}
}

// End-to-end guarantee for the requirement: an organization that enables the
// fallback switch gets a code-default quote even though no
// PRICING_FALLBACK_DEFAULT_PRICES is configured.
func TestPricingEngineCodeFallbackWorksWithoutConfiguredPrices(t *testing.T) {
	t.Setenv(PricingFallbackDefaultPricesEnv, "")
	db := openPricingEngineTestDB(t)
	organizationID := uuid.New()
	savePricingFallbackForOrg(t, db, organizationID, UpdatePricingFallbackRequest{Enabled: true})
	modelID := uuid.New()
	insertPricingModel(t, db, modelID, "openai", "0", "0", false, false, "[]")
	n := 1

	quote, err := NewPricingEngine(db).QuoteImage(context.Background(), PricingModelRef{
		OrganizationID: organizationID,
		ModelID:        modelID,
		Source:         PricingModelSourceGlobal,
	}, &adapter.ImageRequest{Model: "gpt-image-1", N: &n})
	if err != nil {
		t.Fatalf("QuoteImage returned error: %v", err)
	}
	if quote.TotalCredits != 200 {
		t.Fatalf("total credits = %d, want the built-in openai default 200", quote.TotalCredits)
	}
	if quote.PricingSource != PricingSourceCodeDefaultFallback {
		t.Fatalf("pricing source = %q, want code default fallback", quote.PricingSource)
	}
}
