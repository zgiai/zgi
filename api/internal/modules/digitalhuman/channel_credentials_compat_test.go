package digitalhuman

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providers/dingtalk"
	providertransport "github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providertransport"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

type unusedConnectionResolverStub struct{}

func (unusedConnectionResolverStub) Resolve(context.Context, providertransport.CredentialRequest) (*providertransport.CredentialLease, error) {
	return nil, providertransport.NewError(providertransport.ErrorCodeConnectionNotFound, "connection not found", nil)
}

// The Connector catalog stores DingTalk connections under its own driver,
// auth method and field names. The channel resolver must accept the adapter's
// declared compatible identifiers and hand the receiver runtime field names.
func TestConnectionBackedDingTalkCredentialUsesCatalogIdentifiersAndFieldNames(t *testing.T) {
	provider, err := dingtalk.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := dingtalk.NewDirectChannelAdapter(provider, unusedConnectionResolverStub{})
	if err != nil {
		t.Fatal(err)
	}
	providers, err := channelruntime.NewRegistry(channelruntime.Provider{
		Adapter: adapter, Provisioner: adapter, Receiver: dingtalk.NewStreamReceiver(adapter),
	})
	if err != nil {
		t.Fatal(err)
	}
	organizationID, workspaceID, ownerID, connectionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	installation := &channelruntime.Installation{
		ID: uuid.New(), OrganizationID: organizationID, WorkspaceID: workspaceID,
		Provider: dingtalk.IntegrationID, ConnectorID: dingtalk.IntegrationID, ConnectionID: connectionID,
		ConnectionOwnerID: ownerID, CredentialSource: channelruntime.CredentialSourcePersonalConnection,
	}
	catalogConnection := func(driverID string) connectorcore.PersonalConnection {
		return connectorcore.PersonalConnection{
			ID: connectionID, OrganizationID: organizationID, OwnerAccountID: ownerID,
			ConnectorID: dingtalk.IntegrationID, ProviderID: dingtalk.IntegrationID,
			DriverID: connectorcore.Identifier(driverID), AuthMethodID: "dingtalk.credentials",
			EncryptedCredentials: "sealed", CredentialVersion: 1, State: connectorcore.ReadinessReady, Revision: 1,
		}
	}
	resolve := func(connection connectorcore.PersonalConnection) (*channelruntime.ResolvedChannelCredential, error) {
		resolver := NewInstallationCredentialResolver(&channelCredentialStoreStub{}, newTestChannelCredentialCipher(t), providers).
			WithPersonalConnectionResolver(
				personalChannelCredentialResolverStub{connection: connection, values: map[string]string{
					"client_id": "ding-app-key", "client_secret": "ding-app-secret",
				}},
				workspaceMembershipStub{member: true},
			)
		return resolver.ResolveCredential(t.Context(), installation)
	}

	credential, err := resolve(catalogConnection("dingtalk-http-v1"))
	if err != nil {
		t.Fatalf("ResolveCredential() error = %v", err)
	}
	defer credential.Destroy()
	if credential.Values["app_key"] != "ding-app-key" || credential.Values["app_secret"] != "ding-app-secret" ||
		credential.Values["client_id"] != "" || credential.Values["client_secret"] != "" {
		t.Fatalf("resolved credential fields = %v, want runtime app_key/app_secret only", credential.Values)
	}
	if _, err := resolve(catalogConnection("unknown-driver")); err == nil {
		t.Fatal("ResolveCredential() accepted a Connector driver the adapter does not declare")
	}
}
