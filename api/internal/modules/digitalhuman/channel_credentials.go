package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

// ChannelCredentialResolver owns credential storage and leasing without
// interpreting provider-specific keys.
type ChannelCredentialResolver interface {
	ResolveCredential(context.Context, *channelruntime.Installation) (*channelruntime.ResolvedChannelCredential, error)
	ResolveIdentity(context.Context, *channelruntime.Installation) (externalTenantID, externalSelfID string, err error)
	ContextReadiness(context.Context, *channelruntime.Installation) ChannelContextReadiness
}

type IntegrationChannelCredentialResolver struct {
	channelCredentials ChannelCredentialStore
	resolver           integrations.ConnectionResolver
	cipher             integrations.CredentialCipher
	providers          *channelruntime.Registry
}

func NewIntegrationChannelCredentialResolver(
	channelCredentials ChannelCredentialStore,
	resolver integrations.ConnectionResolver,
	cipher integrations.CredentialCipher,
	providers *channelruntime.Registry,
) *IntegrationChannelCredentialResolver {
	if channelCredentials == nil || resolver == nil || cipher == nil || providers == nil {
		panic("channel credential resolver requires credential store, connection resolver, cipher, and provider registry")
	}
	return &IntegrationChannelCredentialResolver{
		channelCredentials: channelCredentials,
		resolver:           resolver,
		cipher:             cipher,
		providers:          providers,
	}
}

func (resolver *IntegrationChannelCredentialResolver) ResolveCredential(
	ctx context.Context,
	installation *channelruntime.Installation,
) (*channelruntime.ResolvedChannelCredential, error) {
	if resolver == nil || installation == nil || installation.ID == uuid.Nil ||
		installation.OrganizationID == uuid.Nil || installation.ConnectionID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	if installation.ConnectionID == installation.ID {
		stored, err := resolver.channelCredentials.ChannelInstallationCredentialByID(
			ctx, installation.OrganizationID, installation.ID,
		)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(stored.Provider, installation.Provider) {
			return nil, ErrChannelNotReady
		}
		credentials, err := resolver.cipher.DecryptCredentials(stored.EncryptedCredentials, channelCredentialAAD(stored))
		if err != nil {
			return nil, fmt.Errorf("decrypt channel installation credential: %w", err)
		}
		return &channelruntime.ResolvedChannelCredential{Values: credentials, Version: stored.CredentialVersion}, nil
	}

	registered, err := resolver.providers.ResolveProvider(installation.Provider)
	if err != nil {
		return nil, ErrChannelNotReady
	}
	spec := registered.Provisioner.ConnectionSpec()
	connection, err := resolver.resolver.Resolve(ctx, integrations.ConnectionResolveRequest{
		OrganizationID:            installation.OrganizationID.String(),
		IntegrationID:             spec.IntegrationID,
		DriverID:                  spec.DriverID,
		ConnectionID:              installation.ConnectionID.String(),
		DisallowAccountCredential: true,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve channel connection credentials: %w", err)
	}
	defer connection.Destroy()
	return &channelruntime.ResolvedChannelCredential{
		Values:  cloneChannelCredentials(connection.Credentials),
		Version: connection.CredentialVersion,
	}, nil
}

func (resolver *IntegrationChannelCredentialResolver) ResolveIdentity(
	ctx context.Context,
	installation *channelruntime.Installation,
) (string, string, error) {
	registered, err := resolver.providers.ResolveProvider(installation.Provider)
	if err != nil {
		return "", "", ErrChannelNotReady
	}
	credential, err := resolver.ResolveCredential(ctx, installation)
	if err != nil {
		return "", "", err
	}
	defer credential.Destroy()
	identity, err := registered.Provisioner.ResolveIdentity(ctx, *credential)
	if err != nil {
		return "", "", fmt.Errorf("resolve channel provider identity: %w", err)
	}
	identity.ExternalTenantID = strings.TrimSpace(identity.ExternalTenantID)
	identity.ExternalSelfID = strings.TrimSpace(identity.ExternalSelfID)
	if identity.ExternalTenantID == "" || identity.ExternalSelfID == "" {
		return "", "", ErrChannelNotReady
	}
	return identity.ExternalTenantID, identity.ExternalSelfID, nil
}

func (resolver *IntegrationChannelCredentialResolver) ContextReadiness(
	_ context.Context,
	installation *channelruntime.Installation,
) ChannelContextReadiness {
	if resolver == nil || installation == nil {
		return ChannelContextReadiness{Status: ChannelContextReadinessUnavailable}
	}
	registered, err := resolver.providers.ResolveProvider(installation.Provider)
	if err != nil || !registered.Adapter.Capabilities().ContextRead {
		return ChannelContextReadiness{Status: ChannelContextReadinessUnavailable}
	}
	return ChannelContextReadiness{Status: ChannelContextReadinessRuntimeVerification}
}

// Resolve exposes installation-owned credentials to provider adapters that
// reuse the integrations transport client. Connection Center remains the
// optional fallback for a referenced credential.
func (resolver *IntegrationChannelCredentialResolver) Resolve(
	ctx context.Context,
	request integrations.ConnectionResolveRequest,
) (*integrations.ResolvedConnection, error) {
	if resolver == nil || resolver.channelCredentials == nil || resolver.cipher == nil || resolver.providers == nil {
		return nil, integrations.NewError(integrations.ErrorCodeConnectionInvalid, "channel credential resolver is unavailable", nil)
	}
	organizationID, organizationErr := uuid.Parse(strings.TrimSpace(request.OrganizationID))
	installationID, installationErr := uuid.Parse(strings.TrimSpace(request.ConnectionID))
	if organizationErr == nil && installationErr == nil && organizationID != uuid.Nil && installationID != uuid.Nil {
		stored, err := resolver.channelCredentials.ChannelInstallationCredentialByID(ctx, organizationID, installationID)
		switch {
		case err == nil:
			registered, providerErr := resolver.providers.ResolveProvider(stored.Provider)
			if providerErr != nil {
				return nil, integrations.NewError(integrations.ErrorCodeConnectionNotFound, "channel provider was not found", providerErr)
			}
			spec := registered.Provisioner.ConnectionSpec()
			if !sameConnectionSpec(request, spec) {
				return nil, integrations.NewError(integrations.ErrorCodeConnectionNotFound, "channel credential was not found", nil)
			}
			credentials, decryptErr := resolver.cipher.DecryptCredentials(stored.EncryptedCredentials, channelCredentialAAD(stored))
			if decryptErr != nil {
				return nil, integrations.NewError(integrations.ErrorCodeConnectionInvalid, "channel credential could not be decrypted", decryptErr)
			}
			return &integrations.ResolvedConnection{
				ID:                installationID.String(),
				OrganizationID:    organizationID.String(),
				IntegrationID:     spec.IntegrationID,
				DriverID:          spec.DriverID,
				CredentialSource:  integrations.ConnectionCredentialSourceOrganization,
				AuthType:          integrations.ConnectionAuthTypeCustomCredential,
				AuthMethodID:      spec.AuthMethodID,
				Credentials:       credentials,
				Config:            map[string]any{},
				CredentialVersion: stored.CredentialVersion,
				Revision:          stored.CredentialVersion,
			}, nil
		case !errors.Is(err, ErrChannelCredentialNotFound):
			return nil, integrations.NewError(integrations.ErrorCodeConnectionInvalid, "channel credential could not be loaded", err)
		}
	}
	return resolver.resolver.Resolve(ctx, request)
}

func (resolver *IntegrationChannelCredentialResolver) CreateEmbeddedInstallation(
	ctx context.Context,
	installation *channelruntime.Installation,
	updates map[string]string,
) error {
	if resolver == nil || installation == nil || installation.ID == uuid.Nil || installation.ConnectionID != installation.ID {
		return ErrInvalidRequest
	}
	registered, err := resolver.providers.ResolveProvider(installation.Provider)
	if err != nil {
		return ErrChannelNotReady
	}
	credentials, err := registered.Provisioner.PrepareCredentials(nil, updates)
	if err != nil {
		return err
	}
	defer destroyChannelCredentials(credentials)
	identity, err := registered.Provisioner.ResolveIdentity(ctx, channelruntime.ResolvedChannelCredential{Values: credentials, Version: 1})
	if err != nil || strings.TrimSpace(identity.ExternalTenantID) == "" || strings.TrimSpace(identity.ExternalSelfID) == "" {
		return ErrChannelNotReady
	}
	installation.ExternalTenantID = strings.TrimSpace(identity.ExternalTenantID)
	installation.ExternalBotID = strings.TrimSpace(identity.ExternalSelfID)
	envelope, err := resolver.cipher.EncryptCredentials(credentials, integrations.CredentialAAD{
		OrganizationID:    installation.OrganizationID,
		ConnectionID:      installation.ID,
		IntegrationID:     strings.ToLower(installation.Provider),
		CredentialVersion: 1,
	})
	if err != nil {
		return fmt.Errorf("encrypt channel installation credential: %w", err)
	}
	credential := &ChannelInstallationCredential{
		InstallationID: installation.ID, OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID,
		Provider: strings.ToLower(installation.Provider), EncryptedCredentials: envelope, CredentialVersion: 1,
		CreatedBy: installation.CreatedBy, UpdatedBy: installation.UpdatedBy, CreatedAt: installation.CreatedAt, UpdatedAt: installation.UpdatedAt,
	}
	return resolver.channelCredentials.CreateChannelInstallationWithCredential(ctx, installation, credential)
}

func (resolver *IntegrationChannelCredentialResolver) ReplaceEmbeddedCredential(
	ctx context.Context,
	scope Scope,
	installation *channelruntime.Installation,
	updates map[string]string,
	expectedRevision int64,
	now time.Time,
) error {
	if resolver == nil || installation == nil || installation.ID == uuid.Nil || expectedRevision < 1 {
		return ErrInvalidRequest
	}
	registered, err := resolver.providers.ResolveProvider(installation.Provider)
	if err != nil {
		return ErrChannelNotReady
	}
	version := 1
	var current map[string]string
	stored, loadErr := resolver.channelCredentials.ChannelInstallationCredentialByID(ctx, scope.OrganizationID, installation.ID)
	switch {
	case loadErr == nil:
		version = stored.CredentialVersion + 1
		current, err = resolver.cipher.DecryptCredentials(stored.EncryptedCredentials, channelCredentialAAD(stored))
		if err != nil {
			return fmt.Errorf("decrypt channel installation credential: %w", err)
		}
	case errors.Is(loadErr, ErrChannelCredentialNotFound):
		// A complete update can switch a Connection Center reference to an
		// installation-owned credential.
	default:
		return loadErr
	}
	defer destroyChannelCredentials(current)
	credentials, err := registered.Provisioner.PrepareCredentials(current, updates)
	if err != nil {
		return err
	}
	defer destroyChannelCredentials(credentials)
	identity, err := registered.Provisioner.ResolveIdentity(ctx, channelruntime.ResolvedChannelCredential{Values: credentials, Version: version})
	if err != nil || strings.TrimSpace(identity.ExternalTenantID) == "" || strings.TrimSpace(identity.ExternalSelfID) == "" {
		return ErrChannelNotReady
	}
	installation.ConnectionID = installation.ID
	installation.ExternalTenantID = strings.TrimSpace(identity.ExternalTenantID)
	installation.ExternalBotID = strings.TrimSpace(identity.ExternalSelfID)
	installation.UpdatedAt = now
	envelope, err := resolver.cipher.EncryptCredentials(credentials, integrations.CredentialAAD{
		OrganizationID:    scope.OrganizationID,
		ConnectionID:      installation.ID,
		IntegrationID:     strings.ToLower(installation.Provider),
		CredentialVersion: version,
	})
	if err != nil {
		return fmt.Errorf("encrypt channel installation credential: %w", err)
	}
	createdAt, createdBy := now, scope.ActorID
	if stored != nil {
		createdAt, createdBy = stored.CreatedAt, stored.CreatedBy
	}
	credential := &ChannelInstallationCredential{
		InstallationID: installation.ID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		Provider: strings.ToLower(installation.Provider), EncryptedCredentials: envelope, CredentialVersion: version,
		CreatedBy: createdBy, UpdatedBy: scope.ActorID, CreatedAt: createdAt, UpdatedAt: now,
	}
	return resolver.channelCredentials.ReplaceChannelInstallationCredential(ctx, scope, installation, credential, expectedRevision)
}

func channelCredentialAAD(credential *ChannelInstallationCredential) integrations.CredentialAAD {
	return integrations.CredentialAAD{
		OrganizationID:    credential.OrganizationID,
		ConnectionID:      credential.InstallationID,
		IntegrationID:     strings.ToLower(credential.Provider),
		CredentialVersion: credential.CredentialVersion,
	}
}

func sameConnectionSpec(request integrations.ConnectionResolveRequest, spec channelruntime.ConnectionSpec) bool {
	return strings.EqualFold(strings.TrimSpace(request.IntegrationID), strings.TrimSpace(spec.IntegrationID)) &&
		strings.EqualFold(strings.TrimSpace(request.DriverID), strings.TrimSpace(spec.DriverID))
}

func cloneChannelCredentials(values map[string]string) map[string]string {
	copyValues := make(map[string]string, len(values))
	for key, value := range values {
		copyValues[key] = value
	}
	return copyValues
}

func destroyChannelCredentials(values map[string]string) {
	for key := range values {
		values[key] = ""
		delete(values, key)
	}
}
