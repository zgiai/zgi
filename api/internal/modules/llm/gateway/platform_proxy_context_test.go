package gateway

import (
	"context"
	"net/http"
	"testing"
)

func TestPlatformProxyRoutingHeaders(t *testing.T) {
	bc := &BillingContext{
		BillingLane:           UsageBillingLanePlatform,
		RoutingStrategy:       "speed",
		EstimatedInputTokens:  120,
		EstimatedOutputTokens: 80,
	}
	req, err := http.NewRequestWithContext(withPlatformProxyMetadata(context.Background(), bc), http.MethodPost, "http://console.internal", nil)
	if err != nil {
		t.Fatal(err)
	}
	applyPlatformProxyHeaders(req)

	if got := req.Header.Get(headerZGIRoutingStrategy); got != "speed" {
		t.Fatalf("routing strategy header = %q, want speed", got)
	}
	if got := req.Header.Get(headerZGIEstimatedInputTokens); got != "120" {
		t.Fatalf("estimated input header = %q, want 120", got)
	}
	if got := req.Header.Get(headerZGIEstimatedOutputTokens); got != "80" {
		t.Fatalf("estimated output header = %q, want 80", got)
	}
	if got := req.Header.Get(headerZGISettlementTransport); got != "" {
		t.Fatalf("non-stream settlement transport = %q, want empty", got)
	}
}

func TestPlatformProxyStreamingNegotiatesSettlementTrailers(t *testing.T) {
	bc := &BillingContext{BillingLane: UsageBillingLanePlatform, IsStreaming: true}
	req, err := http.NewRequestWithContext(withPlatformProxyMetadata(context.Background(), bc), http.MethodPost, "http://console.internal", nil)
	if err != nil {
		t.Fatal(err)
	}
	applyPlatformProxyHeaders(req)
	if got := req.Header.Get(headerZGISettlementTransport); got != "trailers" {
		t.Fatalf("settlement transport = %q, want trailers", got)
	}
}

func TestPrivateChannelDoesNotReceiveRoutingHeaders(t *testing.T) {
	bc := &BillingContext{BillingLane: UsageBillingLanePrivate, RoutingStrategy: "price"}
	req, err := http.NewRequestWithContext(withPlatformProxyMetadata(context.Background(), bc), http.MethodPost, "http://private-channel", nil)
	if err != nil {
		t.Fatal(err)
	}
	applyPlatformProxyHeaders(req)
	if got := req.Header.Get(headerZGIRoutingStrategy); got != "" {
		t.Fatalf("private channel routing strategy header = %q, want empty", got)
	}
}

func TestPlatformProxyRoutingHeadersPreserveZeroOutputEstimate(t *testing.T) {
	bc := &BillingContext{
		BillingLane:          UsageBillingLanePlatform,
		RoutingStrategy:      "price",
		EstimatedInputTokens: 120,
	}
	req, err := http.NewRequestWithContext(withPlatformProxyMetadata(context.Background(), bc), http.MethodPost, "http://console.internal", nil)
	if err != nil {
		t.Fatal(err)
	}
	applyPlatformProxyHeaders(req)

	if got := req.Header.Get(headerZGIEstimatedInputTokens); got != "120" {
		t.Fatalf("estimated input header = %q, want 120", got)
	}
	if got := req.Header.Get(headerZGIEstimatedOutputTokens); got != "0" {
		t.Fatalf("estimated output header = %q, want 0", got)
	}
}

func TestNormalizeConsoleRoutingStrategy(t *testing.T) {
	tests := map[string]string{
		"": "stability", "balanced": "stability", "unexpected": "stability",
		"speed": "speed", "PRICE": "price", " stability ": "stability",
	}
	for input, want := range tests {
		if got := normalizeConsoleRoutingStrategy(input); got != want {
			t.Fatalf("normalizeConsoleRoutingStrategy(%q) = %q, want %q", input, got, want)
		}
	}
}
