package digitalhuman

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

type channelCredentialStoreStub struct {
	credential   *ChannelInstallationCredential
	installation *channelruntime.Installation
}

func (store *channelCredentialStoreStub) CreateChannelInstallationWithCredential(_ context.Context, installation *channelruntime.Installation, credential *ChannelInstallationCredential) error {
	store.installation, store.credential = installation, credential
	return nil
}

func (store *channelCredentialStoreStub) ChannelInstallationCredentialByID(_ context.Context, organizationID, installationID uuid.UUID) (*ChannelInstallationCredential, error) {
	if store.credential == nil || store.credential.OrganizationID != organizationID || store.credential.InstallationID != installationID {
		return nil, ErrChannelCredentialNotFound
	}
	return store.credential, nil
}

func (*channelCredentialStoreStub) ReplaceChannelInstallationCredential(context.Context, Scope, *channelruntime.Installation, *ChannelInstallationCredential, int64) error {
	return errors.New("not implemented")
}

type connectionResolverStub struct{ called bool }

func (resolver *connectionResolverStub) Resolve(context.Context, integrations.ConnectionResolveRequest) (*integrations.ResolvedConnection, error) {
	resolver.called = true
	return nil, integrations.NewError(integrations.ErrorCodeConnectionNotFound, "not found", nil)
}

type alternateChannelProviderStub struct{}

func (alternateChannelProviderStub) Provider() string { return "acme-chat" }
func (alternateChannelProviderStub) Capabilities() channelruntime.ProviderCapabilities {
	return channelruntime.ProviderCapabilities{ContextRead: true}
}
func (alternateChannelProviderStub) Verify(context.Context, channelruntime.WebhookRequest, channelruntime.ResolvedChannelCredential) (*channelruntime.VerifiedWebhook, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}
func (alternateChannelProviderStub) Normalize(context.Context, *channelruntime.VerifiedWebhook, channelruntime.NormalizationContext) (*channelruntime.InboundMessage, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}
func (alternateChannelProviderStub) Acknowledgement() channelruntime.WebhookResponse {
	return channelruntime.WebhookResponse{StatusCode: 200}
}
func (alternateChannelProviderStub) Deliver(context.Context, channelruntime.DeliveryRequest) (*channelruntime.DeliveryReceipt, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}
func (alternateChannelProviderStub) Edit(context.Context, channelruntime.DeliveryRequest, string) (*channelruntime.DeliveryReceipt, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}
func (alternateChannelProviderStub) Retract(context.Context, channelruntime.DeliveryRequest, string) error {
	return channelruntime.ErrUnsupportedEvent
}
func (alternateChannelProviderStub) PrepareCredentials(current, updates map[string]string) (map[string]string, error) {
	prepared := cloneChannelCredentials(current)
	for key, value := range updates {
		if key != "client_token" && key != "callback_secret" {
			return nil, ErrInvalidRequest
		}
		if value != "" {
			prepared[key] = value
		}
	}
	if prepared["client_token"] == "" || prepared["callback_secret"] == "" {
		return nil, ErrInvalidRequest
	}
	return prepared, nil
}
func (alternateChannelProviderStub) ResolveIdentity(_ context.Context, credential channelruntime.ResolvedChannelCredential) (channelruntime.InstallationIdentity, error) {
	if credential.Values["client_token"] == "" {
		return channelruntime.InstallationIdentity{}, ErrChannelNotReady
	}
	return channelruntime.InstallationIdentity{ExternalTenantID: "acme-workspace", ExternalSelfID: "acme-assistant"}, nil
}
func (alternateChannelProviderStub) ConnectionSpec() channelruntime.ConnectionSpec {
	return channelruntime.ConnectionSpec{IntegrationID: "acme-chat", DriverID: "acme-chat-api", AuthMethodID: "acme-app"}
}

type alternateChannelProviderWithoutContextStub struct {
	alternateChannelProviderStub
}

func (alternateChannelProviderWithoutContextStub) Provider() string { return "acme-basic-chat" }

func (alternateChannelProviderWithoutContextStub) Capabilities() channelruntime.ProviderCapabilities {
	return channelruntime.ProviderCapabilities{}
}

func TestChannelContextReadinessDistinguishesUnsupportedAndUnavailable(t *testing.T) {
	supported := alternateChannelProviderStub{}
	unsupported := alternateChannelProviderWithoutContextStub{}
	providers, err := channelruntime.NewRegistry(
		channelruntime.Provider{Adapter: supported, Provisioner: supported},
		channelruntime.Provider{Adapter: unsupported, Provisioner: unsupported},
	)
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := integrations.NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewIntegrationChannelCredentialResolver(
		&channelCredentialStoreStub{},
		&connectionResolverStub{},
		cipher,
		providers,
	)

	tests := []struct {
		name     string
		provider string
		want     ChannelContextReadinessStatus
	}{
		{name: "supported reader", provider: "acme-chat", want: ChannelContextReadinessRuntimeVerification},
		{name: "unsupported reader", provider: unsupported.Provider(), want: ChannelContextReadinessNotSupported},
		{name: "unknown provider", provider: "missing-provider", want: ChannelContextReadinessUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			readiness := resolver.ContextReadiness(t.Context(), &channelruntime.Installation{Provider: test.provider})
			if readiness.Status != test.want {
				t.Fatalf("ContextReadiness() status = %q, want %q", readiness.Status, test.want)
			}
		})
	}
}

func TestEmbeddedChannelCredentialUsesRegisteredProviderWithoutCoreBranch(t *testing.T) {
	cipher, err := integrations.NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("create credential cipher: %v", err)
	}
	provider := alternateChannelProviderStub{}
	providers, err := channelruntime.NewRegistry(channelruntime.Provider{Adapter: provider, Provisioner: provider})
	if err != nil {
		t.Fatal(err)
	}
	store := &channelCredentialStoreStub{}
	fallback := &connectionResolverStub{}
	resolver := NewIntegrationChannelCredentialResolver(store, fallback, cipher, providers)
	now := time.Now().UTC()
	installation := &channelruntime.Installation{
		ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(), Provider: "acme-chat", Name: "Acme",
		CreatedBy: uuid.New(), UpdatedBy: uuid.New(), CreatedAt: now, UpdatedAt: now,
	}
	installation.ConnectionID = installation.ID
	input := map[string]string{"client_token": "token", "callback_secret": "callback"}
	if err := resolver.CreateEmbeddedInstallation(t.Context(), installation, input); err != nil {
		t.Fatalf("create embedded installation: %v", err)
	}
	if store.credential == nil || store.installation != installation {
		t.Fatal("installation and encrypted credential were not persisted together")
	}
	if store.credential.EncryptedCredentials == "" || store.credential.EncryptedCredentials == "token" {
		t.Fatal("credential was not stored as an encrypted envelope")
	}
	resolved, err := resolver.Resolve(t.Context(), integrations.ConnectionResolveRequest{
		OrganizationID: installation.OrganizationID.String(), IntegrationID: "acme-chat",
		DriverID: "acme-chat-api", ConnectionID: installation.ID.String(), DisallowAccountCredential: true,
	})
	if err != nil {
		t.Fatalf("resolve embedded credential: %v", err)
	}
	defer resolved.Destroy()
	if fallback.called {
		t.Fatal("embedded credential unexpectedly used Connection Center fallback")
	}
	if resolved.Credentials["client_token"] != "token" || resolved.Credentials["callback_secret"] != "callback" {
		t.Fatalf("resolved credentials = %#v", resolved.Credentials)
	}
	if installation.ExternalTenantID != "" || installation.ExternalBotID != "" {
		t.Fatalf("identity was resolved before explicit test = %q/%q", installation.ExternalTenantID, installation.ExternalBotID)
	}
	externalTenantID, externalBotID, err := resolver.ResolveIdentity(t.Context(), installation)
	if err != nil || externalTenantID != "acme-workspace" || externalBotID != "acme-assistant" {
		t.Fatalf("resolved identity = %q/%q, %v", externalTenantID, externalBotID, err)
	}
}

func TestEmbeddedChannelCredentialRejectsProviderUnknownField(t *testing.T) {
	provider := alternateChannelProviderStub{}
	if _, err := provider.PrepareCredentials(nil, map[string]string{"verification_token": "wrong-provider-field"}); err == nil {
		t.Fatal("PrepareCredentials() unknown field error = nil")
	}
}
