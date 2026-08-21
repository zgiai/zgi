package channelruntime

import (
	"context"
	"testing"
)

type testAdapter struct{ provider string }

func (adapter testAdapter) Provider() string { return adapter.provider }
func (testAdapter) Verify(context.Context, WebhookRequest, SecurityMaterial) (*VerifiedWebhook, error) {
	return nil, nil
}
func (testAdapter) Normalize(context.Context, *VerifiedWebhook) (*InboundMessage, error) {
	return nil, nil
}
func (testAdapter) Deliver(context.Context, DeliveryRequest) (*DeliveryReceipt, error) {
	return nil, nil
}
func (testAdapter) Edit(context.Context, DeliveryRequest, string) (*DeliveryReceipt, error) {
	return nil, nil
}
func (testAdapter) Retract(context.Context, DeliveryRequest, string) error { return nil }

func TestRegistryNormalizesProviderAndRejectsDuplicates(t *testing.T) {
	registry, err := NewRegistry(testAdapter{provider: " FeiShu "})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if _, err := registry.Resolve("feishu"); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if err := registry.Register(testAdapter{provider: "FEISHU"}); err == nil {
		t.Fatal("Register() duplicate error = nil")
	}
}
