package digitalhuman

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	providertransport "github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providertransport"
	credentialcrypto "github.com/zgiai/zgi/api/internal/infra/credentialcrypto"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
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

func (store *channelCredentialStoreStub) ChannelInstallationByID(_ context.Context, installationID uuid.UUID) (*channelruntime.Installation, error) {
	if store.installation == nil || store.installation.ID != installationID {
		return nil, ErrChannelInstallationNotFound
	}
	return store.installation, nil
}

func (*channelCredentialStoreStub) ReplaceChannelInstallationCredential(context.Context, Scope, *channelruntime.Installation, *ChannelInstallationCredential, int64) error {
	return errors.New("not implemented")
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

func newTestChannelCredentialCipher(t *testing.T) credentialcrypto.CredentialCipher {
	t.Helper()
	cipher, err := credentialcrypto.NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("create credential cipher: %v", err)
	}
	return cipher
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
	resolver := NewInstallationCredentialResolver(
		&channelCredentialStoreStub{},
		newTestChannelCredentialCipher(t),
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
	cipher := newTestChannelCredentialCipher(t)
	provider := alternateChannelProviderStub{}
	providers, err := channelruntime.NewRegistry(channelruntime.Provider{Adapter: provider, Provisioner: provider})
	if err != nil {
		t.Fatal(err)
	}
	store := &channelCredentialStoreStub{}
	resolver := NewInstallationCredentialResolver(store, cipher, providers)
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
	if installation.CredentialSource != channelruntime.CredentialSourceEmbedded ||
		installation.ConnectionOwnerID != installation.CreatedBy ||
		installation.ConnectorID != "acme-chat" {
		t.Fatalf("embedded installation credential binding = %#v", installation)
	}
	lease, err := resolver.Resolve(t.Context(), providertransport.CredentialRequest{
		OrganizationID: installation.OrganizationID.String(), WorkspaceID: installation.WorkspaceID.String(),
		ProviderID: "acme-chat", DriverID: "acme-chat-api", InstallationID: installation.ID.String(),
	})
	if err != nil {
		t.Fatalf("resolve embedded credential: %v", err)
	}
	defer lease.Destroy()
	if lease.Credentials["client_token"] != "token" || lease.Credentials["callback_secret"] != "callback" {
		t.Fatalf("resolved credentials = %#v", lease.Credentials)
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

type personalChannelCredentialResolverStub struct {
	connection connectorcore.PersonalConnection
	values     map[string]string
	err        error
}

func (stub personalChannelCredentialResolverStub) ResolvePersonalConnectionCredentials(
	context.Context, uuid.UUID, uuid.UUID, uuid.UUID,
) (connectorcore.PersonalConnection, map[string]string, error) {
	return stub.connection, cloneChannelCredentials(stub.values), stub.err
}

type workspaceMembershipStub struct {
	member bool
	err    error
}

func (stub workspaceMembershipStub) IsMemberInWorkspace(context.Context, string, string) (bool, error) {
	return stub.member, stub.err
}

func TestConnectionBackedChannelCredentialUsesOwnerAndConnectionSpec(t *testing.T) {
	provider := alternateChannelProviderStub{}
	providers, err := channelruntime.NewRegistry(channelruntime.Provider{Adapter: provider, Provisioner: provider})
	if err != nil {
		t.Fatal(err)
	}
	cipher := newTestChannelCredentialCipher(t)
	organizationID, workspaceID, ownerID, installationID, connectionID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	installation := &channelruntime.Installation{
		ID: installationID, OrganizationID: organizationID, WorkspaceID: workspaceID,
		Provider: provider.Provider(), ConnectorID: provider.Provider(), ConnectionID: connectionID,
		ConnectionOwnerID: ownerID, CredentialSource: channelruntime.CredentialSourcePersonalConnection,
	}
	connection := connectorcore.PersonalConnection{
		ID: connectionID, OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectorID: connectorcore.Identifier(provider.Provider()), ProviderID: connectorcore.Identifier(provider.ConnectionSpec().IntegrationID),
		DriverID: connectorcore.Identifier(provider.ConnectionSpec().DriverID), AuthMethodID: connectorcore.Identifier(provider.ConnectionSpec().AuthMethodID),
		EncryptedCredentials: "sealed", CredentialVersion: 4, State: connectorcore.ReadinessReady, Revision: 9,
	}
	store := &channelCredentialStoreStub{installation: installation}
	resolver := NewInstallationCredentialResolver(store, cipher, providers).
		WithPersonalConnectionResolver(
			personalChannelCredentialResolverStub{connection: connection, values: map[string]string{"client_token": "token"}},
			workspaceMembershipStub{member: true},
		)
	credential, err := resolver.ResolveCredential(t.Context(), installation)
	if err != nil {
		t.Fatalf("ResolveCredential() error = %v", err)
	}
	if credential.Version != 4 || credential.Revision != 9 ||
		credential.Source != channelruntime.CredentialSourcePersonalConnection ||
		credential.Values["client_token"] != "token" {
		t.Fatalf("resolved credential = %#v", credential)
	}
	credential.Destroy()
	lease, err := resolver.Resolve(t.Context(), providertransport.CredentialRequest{
		OrganizationID: organizationID.String(), WorkspaceID: workspaceID.String(), InstallationID: installationID.String(),
		ProviderID: provider.ConnectionSpec().IntegrationID, DriverID: provider.ConnectionSpec().DriverID,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if lease.ID != connectionID.String() || lease.AccountID != ownerID.String() || lease.Revision != 9 || lease.CredentialVersion != 4 {
		t.Fatalf("lease identity/version = %#v", lease)
	}
	lease.Destroy()
}

// TestConnectionBackedChannelCredentialFallsBackToCreator covers installations
// persisted before ConnectionOwnerID existed: the creator is the owner that
// selected the connection, and the source is derived from the locator.
func TestConnectionBackedChannelCredentialFallsBackToCreator(t *testing.T) {
	provider := alternateChannelProviderStub{}
	providers, err := channelruntime.NewRegistry(channelruntime.Provider{Adapter: provider, Provisioner: provider})
	if err != nil {
		t.Fatal(err)
	}
	organizationID, workspaceID, ownerID, installationID, connectionID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	installation := &channelruntime.Installation{
		ID: installationID, OrganizationID: organizationID, WorkspaceID: workspaceID,
		Provider: provider.Provider(), ConnectionID: connectionID, CreatedBy: ownerID,
	}
	connection := connectorcore.PersonalConnection{
		ID: connectionID, OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectorID: connectorcore.Identifier(provider.Provider()), ProviderID: connectorcore.Identifier(provider.ConnectionSpec().IntegrationID),
		DriverID: connectorcore.Identifier(provider.ConnectionSpec().DriverID), AuthMethodID: connectorcore.Identifier(provider.ConnectionSpec().AuthMethodID),
		EncryptedCredentials: "sealed", CredentialVersion: 2, State: connectorcore.ReadinessReady, Revision: 3,
	}
	resolver := NewInstallationCredentialResolver(&channelCredentialStoreStub{}, newTestChannelCredentialCipher(t), providers).
		WithPersonalConnectionResolver(
			personalChannelCredentialResolverStub{connection: connection, values: map[string]string{"client_token": "token"}},
			workspaceMembershipStub{member: true},
		)
	credential, err := resolver.ResolveCredential(t.Context(), installation)
	if err != nil {
		t.Fatalf("ResolveCredential() error = %v", err)
	}
	if credential.Revision != 3 || credential.Values["client_token"] != "token" {
		t.Fatalf("resolved credential = %#v", credential)
	}
	credential.Destroy()
}

func TestConnectionBackedChannelCredentialFailsClosedForScopeAndIdentity(t *testing.T) {
	provider := alternateChannelProviderStub{}
	providers, err := channelruntime.NewRegistry(channelruntime.Provider{Adapter: provider, Provisioner: provider})
	if err != nil {
		t.Fatal(err)
	}
	cipher := newTestChannelCredentialCipher(t)
	organizationID, workspaceID, ownerID, installationID, connectionID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	installation := &channelruntime.Installation{
		ID: installationID, OrganizationID: organizationID, WorkspaceID: workspaceID,
		Provider: provider.Provider(), ConnectorID: provider.Provider(), ConnectionID: connectionID,
		ConnectionOwnerID: ownerID, CredentialSource: channelruntime.CredentialSourcePersonalConnection,
	}
	base := connectorcore.PersonalConnection{
		ID: connectionID, OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectorID: connectorcore.Identifier(provider.Provider()), ProviderID: connectorcore.Identifier(provider.ConnectionSpec().IntegrationID),
		DriverID: connectorcore.Identifier(provider.ConnectionSpec().DriverID), AuthMethodID: connectorcore.Identifier(provider.ConnectionSpec().AuthMethodID),
		EncryptedCredentials: "sealed", CredentialVersion: 1, State: connectorcore.ReadinessReady, Revision: 1,
	}
	values := map[string]string{"client_token": "token"}
	for _, test := range []struct {
		name      string
		mutate    func(*connectorcore.PersonalConnection)
		workspace bool
	}{
		{name: "organization mismatch", mutate: func(connection *connectorcore.PersonalConnection) {
			connection.OrganizationID = uuid.New()
		}, workspace: true},
		{name: "owner mismatch", mutate: func(connection *connectorcore.PersonalConnection) {
			connection.OwnerAccountID = uuid.New()
		}, workspace: true},
		{name: "connector mismatch", mutate: func(connection *connectorcore.PersonalConnection) {
			connection.ConnectorID = "other"
		}, workspace: true},
		{name: "provider mismatch", mutate: func(connection *connectorcore.PersonalConnection) {
			connection.ProviderID = "other"
		}, workspace: true},
		{name: "driver mismatch", mutate: func(connection *connectorcore.PersonalConnection) {
			connection.DriverID = "other"
		}, workspace: true},
		{name: "auth method mismatch", mutate: func(connection *connectorcore.PersonalConnection) {
			connection.AuthMethodID = "other"
		}, workspace: true},
		{name: "not ready", mutate: func(connection *connectorcore.PersonalConnection) {
			connection.State = connectorcore.ReadinessUnavailable
		}, workspace: true},
		{name: "deleted", mutate: func(connection *connectorcore.PersonalConnection) {
			deletedAt := time.Now().UTC()
			connection.DeletedAt = &deletedAt
		}, workspace: true},
		{name: "revision missing", mutate: func(connection *connectorcore.PersonalConnection) {
			connection.Revision = 0
		}, workspace: true},
		{name: "credential version missing", mutate: func(connection *connectorcore.PersonalConnection) {
			connection.CredentialVersion = 0
		}, workspace: true},
		{name: "envelope missing", mutate: func(connection *connectorcore.PersonalConnection) {
			connection.EncryptedCredentials = ""
		}, workspace: true},
		{name: "workspace membership revoked", mutate: func(*connectorcore.PersonalConnection) {}, workspace: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			candidate.DeletedAt = nil
			test.mutate(&candidate)
			resolver := NewInstallationCredentialResolver(&channelCredentialStoreStub{}, cipher, providers).
				WithPersonalConnectionResolver(
					personalChannelCredentialResolverStub{connection: candidate, values: values},
					workspaceMembershipStub{member: test.workspace},
				)
			if _, err := resolver.ResolveCredential(t.Context(), installation); !errors.Is(err, ErrChannelNotReady) {
				t.Fatalf("ResolveCredential() error = %v, want %v", err, ErrChannelNotReady)
			}
		})
	}
}

func TestConnectionBackedChannelCredentialRequiresPersonalResolver(t *testing.T) {
	provider := alternateChannelProviderStub{}
	providers, err := channelruntime.NewRegistry(channelruntime.Provider{Adapter: provider, Provisioner: provider})
	if err != nil {
		t.Fatal(err)
	}
	installation := &channelruntime.Installation{
		ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(), Provider: provider.Provider(),
		ConnectionID: uuid.New(), ConnectionOwnerID: uuid.New(),
		CredentialSource: channelruntime.CredentialSourcePersonalConnection,
	}
	resolver := NewInstallationCredentialResolver(&channelCredentialStoreStub{}, newTestChannelCredentialCipher(t), providers)
	if _, err := resolver.ResolveCredential(t.Context(), installation); !errors.Is(err, ErrChannelNotReady) {
		t.Fatalf("ResolveCredential() error = %v, want %v", err, ErrChannelNotReady)
	}
}

func TestUnknownChannelCredentialSourceFailsClosed(t *testing.T) {
	provider := alternateChannelProviderStub{}
	providers, err := channelruntime.NewRegistry(channelruntime.Provider{Adapter: provider, Provisioner: provider})
	if err != nil {
		t.Fatal(err)
	}
	installation := &channelruntime.Installation{
		ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(), Provider: provider.Provider(),
		ConnectionID: uuid.New(), CredentialSource: channelruntime.CredentialSource("future_source"),
	}
	resolver := NewInstallationCredentialResolver(&channelCredentialStoreStub{}, newTestChannelCredentialCipher(t), providers)
	if _, err := resolver.ResolveCredential(t.Context(), installation); !errors.Is(err, ErrChannelNotReady) {
		t.Fatalf("ResolveCredential() error = %v, want %v", err, ErrChannelNotReady)
	}
}

type unavailableCipherForTest struct{}

func (unavailableCipherForTest) EncryptCredentials(map[string]string, credentialcrypto.CredentialAAD) (string, error) {
	return "", credentialcrypto.ErrCredentialCipherUnavailable
}

func (unavailableCipherForTest) DecryptCredentials(string, credentialcrypto.CredentialAAD) (map[string]string, error) {
	return nil, credentialcrypto.ErrCredentialCipherUnavailable
}

func TestEmbeddedChannelCredentialFailsClosedWhenCipherUnavailable(t *testing.T) {
	provider := alternateChannelProviderStub{}
	providers, err := channelruntime.NewRegistry(channelruntime.Provider{Adapter: provider, Provisioner: provider})
	if err != nil {
		t.Fatal(err)
	}
	installation := &channelruntime.Installation{Provider: provider.Provider(), ConnectionID: uuid.New()}
	installation.ID = installation.ConnectionID
	resolver := NewInstallationCredentialResolver(&channelCredentialStoreStub{}, unavailableCipherForTest{}, providers)
	if err := resolver.CreateEmbeddedInstallation(t.Context(), installation, map[string]string{
		"client_token": "token", "callback_secret": "callback",
	}); !errors.Is(err, ErrCredentialStorageUnavailable) {
		t.Fatalf("CreateEmbeddedInstallation() error = %v, want %v", err, ErrCredentialStorageUnavailable)
	}
}
