package gateway

import (
	"encoding/json"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestVideoMeteredFactMapsBillingUnits(t *testing.T) {
	ten, two := 10, 2
	tests := []struct {
		name     string
		pricing  string
		req      *adapter.VideoRequest
		wantUnit string
		wantQty  int64
	}{
		{
			name:     "per second",
			pricing:  `{"deployment_scope":"global","video_generation":{"currency":"USD","billing_unit":"second","rates":[{"resolution":"720p","price_per_second":0.12}]}}`,
			req:      &adapter.VideoRequest{Resolution: "720p", Duration: &ten},
			wantUnit: "second",
			wantQty:  10,
		},
		{
			name:     "per task",
			pricing:  `{"deployment_scope":"global","video_generation":{"currency":"USD","billing_unit":"task","rates":[{"resolution":"720p","price_per_task":1.2}]}}`,
			req:      &adapter.VideoRequest{Resolution: "720p", N: &two},
			wantUnit: "task",
			wantQty:  2,
		},
		{
			name:     "million video tokens without estimator reserves one priced unit",
			pricing:  `{"deployment_scope":"global","video_generation":{"currency":"USD","billing_unit":"million_video_tokens","rates":[{"resolution":"720p","price_per_million_tokens":2}]}}`,
			req:      &adapter.VideoRequest{Resolution: "720p", Duration: &ten},
			wantUnit: "million_video_tokens",
			wantQty:  defaultVideoTokenReservation,
		},
		{
			name:     "million video tokens uses the configured estimator",
			pricing:  `{"deployment_scope":"global","video_generation":{"currency":"USD","billing_unit":"million_video_tokens","rates":[{"resolution":"720p","price_per_million_tokens":2}],"token_estimation":{"unit":"video_tokens","rules":[{"resolution":"720p","tokens_per_second":1000}]}}}`,
			req:      &adapter.VideoRequest{Resolution: "720p", Duration: &ten},
			wantUnit: "million_video_tokens",
			wantQty:  10000,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fact, ok := videoMeteredFact(json.RawMessage(tc.pricing), tc.req)
			if !ok {
				t.Fatal("expected a video metered fact")
			}
			if fact.Operation != "video_generation" || fact.Meter != "video" || fact.BaseUnit != tc.wantUnit || fact.Quantity != tc.wantQty {
				t.Fatalf("fact = %#v", fact)
			}
			if fact.Dimensions["resolution"] != "720p" {
				t.Fatalf("dimensions = %#v", fact.Dimensions)
			}
		})
	}
}

func TestVideoMeteredFactSkipsWithoutVideoPrice(t *testing.T) {
	noVideoPrice := json.RawMessage(`{"deployment_scope":"global","token_tiers":[{"min_input_tokens":0,"input_price_per_million":1,"output_price_per_million":2}]}`)
	if _, ok := videoMeteredFact(noVideoPrice, &adapter.VideoRequest{Resolution: "720p"}); ok {
		t.Fatal("a model without video pricing must not declare a fact")
	}
}
