package gateway

import (
	"context"
	"net/http"
	"testing"
)

func TestPlatformProxyRoutingHeaders(t *testing.T) {
	bc := &BillingContext{
		BillingLane:           UsageBillingLanePlatform,
		RoutingStrategy:       "price",
		EstimatedInputTokens:  120,
		EstimatedOutputTokens: 80,
	}
	req, err := http.NewRequestWithContext(withPlatformProxyMetadata(context.Background(), bc), http.MethodPost, "http://console.internal", nil)
	if err != nil {
		t.Fatal(err)
	}
	applyPlatformProxyHeaders(req)

	if got := req.Header.Get(headerZGIRoutingStrategy); got != "price" {
		t.Fatalf("routing strategy header = %q, want price", got)
	}
	if got := req.Header.Get(headerZGIEstimatedInputTokens); got != "120" {
		t.Fatalf("estimated input header = %q, want 120", got)
	}
	if got := req.Header.Get(headerZGIEstimatedOutputTokens); got != "80" {
		t.Fatalf("estimated output header = %q, want 80", got)
	}
}

func TestPrivateChannelDoesNotReceiveRoutingHeaders(t *testing.T) {
	bc := &BillingContext{BillingLane: UsageBillingLanePrivate, RoutingStrategy: "speed"}
	req, err := http.NewRequestWithContext(withPlatformProxyMetadata(context.Background(), bc), http.MethodPost, "http://private-channel", nil)
	if err != nil {
		t.Fatal(err)
	}
	applyPlatformProxyHeaders(req)
	if got := req.Header.Get(headerZGIRoutingStrategy); got != "" {
		t.Fatalf("private channel routing strategy header = %q, want empty", got)
	}
}
