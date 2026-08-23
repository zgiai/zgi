package channelruntime

import (
	"context"
	"testing"
)

type testAdapter struct{ provider string }

func (adapter testAdapter) Provider() string           { return adapter.provider }
func (testAdapter) Capabilities() ProviderCapabilities { return ProviderCapabilities{} }
func (testAdapter) Verify(context.Context, WebhookRequest, ResolvedChannelCredential) (*VerifiedWebhook, error) {
	return nil, nil
}
func (testAdapter) Normalize(context.Context, *VerifiedWebhook, NormalizationContext) (*InboundMessage, error) {
	return nil, nil
}
func (testAdapter) Acknowledgement() WebhookResponse { return WebhookResponse{} }
func (testAdapter) Deliver(context.Context, DeliveryRequest) (*DeliveryReceipt, error) {
	return nil, nil
}
func (testAdapter) Edit(context.Context, DeliveryRequest, string) (*DeliveryReceipt, error) {
	return nil, nil
}
func (testAdapter) Retract(context.Context, DeliveryRequest, string) error { return nil }

type testProvisioner struct{}

func (testProvisioner) PrepareCredentials(_, updates map[string]string) (map[string]string, error) {
	return updates, nil
}
func (testProvisioner) ResolveIdentity(context.Context, ResolvedChannelCredential) (InstallationIdentity, error) {
	return InstallationIdentity{}, nil
}
func (testProvisioner) ConnectionSpec() ConnectionSpec { return ConnectionSpec{} }

func TestRegistryNormalizesProviderAndRejectsDuplicates(t *testing.T) {
	registry, err := NewRegistry(Provider{Adapter: testAdapter{provider: " FeiShu "}, Provisioner: testProvisioner{}})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if _, err := registry.Resolve("feishu"); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if err := registry.Register(Provider{Adapter: testAdapter{provider: "FEISHU"}, Provisioner: testProvisioner{}}); err == nil {
		t.Fatal("Register() duplicate error = nil")
	}
}
