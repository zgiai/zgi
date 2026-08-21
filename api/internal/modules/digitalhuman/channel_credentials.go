package digitalhuman

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/feishu"
)

type ChannelCredentialResolver interface {
	ResolveIdentity(context.Context, *channelruntime.Installation) (externalTenantID, externalBotID string, err error)
	ResolveSecurity(context.Context, *channelruntime.Installation) (channelruntime.SecurityMaterial, func(), error)
}

// ChannelIdentityResolver discovers provider-owned tenant and bot identifiers
// from one request-scoped integration connection. Implementations must not
// retain the connection or any credential material after returning.
type ChannelIdentityResolver interface {
	ResolveChannelIdentity(context.Context, *integrations.ResolvedConnection) (externalTenantID, externalBotID string, err error)
}

type IntegrationChannelCredentialResolver struct {
	connections integrations.ConnectionRepository
	resolver    integrations.ConnectionResolver
	identities  ChannelIdentityResolver
}

func NewIntegrationChannelCredentialResolver(connections integrations.ConnectionRepository, resolver integrations.ConnectionResolver, identities ChannelIdentityResolver) *IntegrationChannelCredentialResolver {
	if connections == nil || resolver == nil || identities == nil {
		panic("channel credential resolver requires integration connection stores and identity resolver")
	}
	return &IntegrationChannelCredentialResolver{connections: connections, resolver: resolver, identities: identities}
}

func (resolver *IntegrationChannelCredentialResolver) ResolveIdentity(ctx context.Context, installation *channelruntime.Installation) (string, string, error) {
	resolved, cleanup, err := resolver.resolveConnection(ctx, installation)
	if err != nil {
		return "", "", err
	}
	defer cleanup()
	if strings.TrimSpace(resolved.Credentials["verification_token"]) == "" {
		return "", "", ErrChannelNotReady
	}
	externalTenantID, externalBotID, err := resolver.identities.ResolveChannelIdentity(ctx, resolved)
	if err != nil {
		return "", "", fmt.Errorf("resolve channel provider identity: %w", err)
	}
	externalTenantID = strings.TrimSpace(externalTenantID)
	externalBotID = strings.TrimSpace(externalBotID)
	if externalTenantID == "" || externalBotID == "" {
		return "", "", ErrChannelNotReady
	}
	return externalTenantID, externalBotID, nil
}

func (resolver *IntegrationChannelCredentialResolver) ResolveSecurity(ctx context.Context, installation *channelruntime.Installation) (channelruntime.SecurityMaterial, func(), error) {
	resolved, cleanup, err := resolver.resolveConnection(ctx, installation)
	if err != nil {
		return channelruntime.SecurityMaterial{}, func() {}, err
	}
	material := channelruntime.SecurityMaterial{
		VerificationToken: strings.TrimSpace(resolved.Credentials["verification_token"]),
		EncryptKey:        strings.TrimSpace(resolved.Credentials["encrypt_key"]),
	}
	if material.VerificationToken == "" {
		cleanup()
		return channelruntime.SecurityMaterial{}, func() {}, ErrChannelNotReady
	}
	return material, cleanup, nil
}

func (resolver *IntegrationChannelCredentialResolver) resolveConnection(ctx context.Context, installation *channelruntime.Installation) (*integrations.ResolvedConnection, func(), error) {
	connection, err := resolver.connection(ctx, installation)
	if err != nil {
		return nil, func() {}, err
	}
	resolved, err := resolver.resolver.Resolve(ctx, integrations.ConnectionResolveRequest{
		OrganizationID: installation.OrganizationID.String(), IntegrationID: connection.IntegrationID,
		DriverID: connection.DriverID, ConnectionID: connection.ID.String(), DisallowAccountCredential: true,
	})
	if err != nil {
		return nil, func() {}, fmt.Errorf("resolve channel connection credentials: %w", err)
	}
	return resolved, func() { resolved.Destroy() }, nil
}

func (resolver *IntegrationChannelCredentialResolver) connection(ctx context.Context, installation *channelruntime.Installation) (*integrations.IntegrationConnection, error) {
	if resolver == nil || installation == nil || installation.ID == uuid.Nil || installation.OrganizationID == uuid.Nil || installation.ConnectionID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	connection, err := resolver.connections.GetByID(ctx, installation.OrganizationID, installation.ConnectionID)
	if err != nil {
		return nil, fmt.Errorf("load channel integration connection: %w", err)
	}
	if connection.Status != integrations.ConnectionStatusActive || connection.CredentialSource != integrations.ConnectionCredentialSourceOrganization ||
		!strings.EqualFold(connection.IntegrationID, installation.Provider) || !strings.EqualFold(connection.IntegrationID, feishu.IntegrationID) ||
		connection.AuthMethodID != feishu.TenantAppAuthMethodID {
		return nil, ErrChannelNotReady
	}
	return connection, nil
}
