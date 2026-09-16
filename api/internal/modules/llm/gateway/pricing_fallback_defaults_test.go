package gateway

import "testing"

func TestDefaultPricingFallbackRulesWithoutConfiguredPrices(t *testing.T) {
	t.Setenv(PricingFallbackDefaultPricesEnv, "")
	rules := DefaultPricingFallbackRules()
	if len(rules) != 10 {
		t.Fatalf("rules = %d, want 10 skeletons", len(rules))
	}
	for _, rule := range rules {
		if rule.ID == "" || rule.Operation == "" || rule.Meter == "" {
			t.Fatalf("skeleton %+v is missing identity", rule)
		}
		if rule.Enabled == nil || !*rule.Enabled {
			t.Fatalf("skeleton %s must stay enabled", rule.ID)
		}
		if rule.PriceUSDPer1MTokens != "" || rule.Credits != 0 {
			t.Fatalf("skeleton %s must not carry a hardcoded price: %+v", rule.ID, rule)
		}
	}
}

func TestDefaultPricingFallbackRulesApplyConfiguredPrices(t *testing.T) {
	t.Setenv(PricingFallbackDefaultPricesEnv, `{
		"token.chat.input.default": "1.5",
		"image.openai.default": 250,
		"image.generic.default": "200",
		"unknown.rule": 42
	}`)
	rules := DefaultPricingFallbackRules()
	byID := make(map[string]PricingFallbackRule, len(rules))
	for _, rule := range rules {
		byID[rule.ID] = rule
	}
	if byID["token.chat.input.default"].PriceUSDPer1MTokens != "1.5" {
		t.Fatalf("chat input price = %q, want 1.5", byID["token.chat.input.default"].PriceUSDPer1MTokens)
	}
	if byID["image.openai.default"].Credits != 250 {
		t.Fatalf("openai image credits = %d, want 250", byID["image.openai.default"].Credits)
	}
	if byID["image.generic.default"].Credits != 200 {
		t.Fatalf("generic image credits = %d, want 200 from string form", byID["image.generic.default"].Credits)
	}
	if byID["token.chat.output.default"].PriceUSDPer1MTokens != "" {
		t.Fatalf("unconfigured rule must stay priceless: %+v", byID["token.chat.output.default"])
	}
}

func TestDefaultPricingFallbackRulesTolerateInvalidConfig(t *testing.T) {
	t.Setenv(PricingFallbackDefaultPricesEnv, "{not-json")
	rules := DefaultPricingFallbackRules()
	if len(rules) != 10 {
		t.Fatalf("rules = %d, want skeletons even with invalid config", len(rules))
	}
	for _, rule := range rules {
		if rule.PriceUSDPer1MTokens != "" || rule.Credits != 0 {
			t.Fatalf("invalid config must not invent prices: %+v", rule)
		}
	}
}
