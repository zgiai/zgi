package pricing

import (
	"encoding/json"
	"testing"
)

func TestMergeOrganizationToolsOnlyOverride(t *testing.T) {
	base := json.RawMessage(`{"version":2,"schema_version":1,"billing_lane":"platform","pricing_version":"platform","rules":[{"id":"chat-in","operation":"chat","meter":"input_token","unit":"per_1m_tokens","price":"1","configured":true}],"tools":{"default":{"web_search":{"amount":"99","currency":"USD","unit":"call","per_quantity":1}}}}`)
	for _, tc := range []struct{ name, tools, wantSource, wantAmount string }{
		{"default", `{"default":{"web_search":{"amount":"0.001","currency":"USD","unit":"call","per_quantity":1}}}`, "default", "0.001"},
		{"channel", `{"default":{"web_search":{"amount":"0.001","currency":"USD","unit":"call","per_quantity":1}},"channels":{"channel":{"web_search":{"amount":"0.002","currency":"USD","unit":"call","per_quantity":1}}}}`, "channel", "0.002"},
		{"null", `{"default":{"web_search":{"amount":"0.001","currency":"USD","unit":"call","per_quantity":1}},"channels":{"channel":{"web_search":null}}}`, "free", ""},
		{"all free", `null`, "free", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			override := json.RawMessage(`{"schema_version":1,"billing_lane":"private","pricing_version":"tenant-v1","tools":` + tc.tools + `}`)
			merged := MergeOrganizationPricingOverride(base, override, "USD")
			resolved, err := ResolveToolPrice(merged, "private", "channel", "web_search")
			if err != nil || resolved.Source != tc.wantSource || resolved.Price.Amount != tc.wantAmount || resolved.PricingVersion != "tenant-v1" {
				t.Fatalf("%+v %v", resolved, err)
			}
			var envelope structuredPricing
			if err := json.Unmarshal(merged, &envelope); err != nil {
				t.Fatal(err)
			}
			if len(envelope.Rules) != 1 || envelope.Rules[0].Price != "1" {
				t.Fatalf("token rules changed: %s", merged)
			}
			if string(MergeOrganizationPricingOverride(merged, override, "USD")) != string(merged) {
				t.Fatal("merge is not idempotent")
			}
			// A tenant block must not pull an absent tool from platform pricing.
			code, err := ResolveToolPrice(merged, "private", "channel", "code_interpreter")
			if err != nil || code.Source != "free" {
				t.Fatalf("%+v %v", code, err)
			}
		})
	}
}
