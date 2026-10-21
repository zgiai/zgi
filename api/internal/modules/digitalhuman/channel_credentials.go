package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	providertransport "github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providertransport"
	credentialcrypto "github.com/zgiai/zgi/api/internal/infra/credentialcrypto"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// ChannelCredentialResolver owns credential storage and leasing without
// interpreting provider-specific keys.
type ChannelCredentialResolver interface {
	ResolveCredential(context.Context, *channelruntime.Installation) (*channelruntime.ResolvedChannelCredential, error)
	ResolveIdentity(context.Context, *channelruntime.Installation) (externalTenantID, externalSelfID string, err error)
	ContextReadiness(context.Context, *channelruntime.Installation) ChannelContextReadiness
}

// PersonalConnectionCredentialResolver is the only boundary allowed to open a
// Connector PersonalConnection envelope for the DigitalHuman channel runtime.
// Implementations must scope the repository read by organization and owner, so
// an installation can never lease a connection that belongs to another account.
type PersonalConnectionCredentialResolver interface {
	ResolvePersonalConnectionCredentials(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (connectorcore.PersonalConnection, map[string]string, error)
}

// WorkspaceMembershipChecker keeps an installation's credential owner from
// being used after that account leaves the target workspace.
type WorkspaceMembershipChecker interface {
	IsMemberInWorkspace(context.Context, string, string) (bool, error)
}

// channelInstallationReader is the optional persistence surface used to reload
// the installation that owns a credential lease. It stays optional so existing
// credential stores and test stubs keep working.
type channelInstallationReader interface {
	ChannelInstallationByID(context.Context, uuid.UUID) (*channelruntime.Installation, error)
}

type InstallationCredentialResolver struct {
	channelCredentials  ChannelCredentialStore
	cipher              credentialcrypto.CredentialCipher
	providers           *channelruntime.Registry
	personalConnections PersonalConnectionCredentialResolver
	workspaceMembers    WorkspaceMembershipChecker
}

func NewInstallationCredentialResolver(
	channelCredentials ChannelCredentialStore,
	cipher credentialcrypto.CredentialCipher,
	providers *channelruntime.Registry,
) *InstallationCredentialResolver {
	if channelCredentials == nil || cipher == nil || providers == nil {
		panic("channel credential resolver requires credential store, cipher, and provider registry")
	}
	return &InstallationCredentialResolver{
		channelCredentials: channelCredentials,
		cipher:             cipher,
		providers:          providers,
	}
}

// WithPersonalConnectionResolver installs the owner-scoped Connector credential
// boundary. Both dependencies are required before a Connector-backed
// installation can resolve credentials; otherwise it fails closed.
func (resolver *InstallationCredentialResolver) WithPersonalConnectionResolver(
	personalConnections PersonalConnectionCredentialResolver,
	workspaceMembers WorkspaceMembershipChecker,
) *InstallationCredentialResolver {
	if resolver != nil {
		resolver.personalConnections = personalConnections
		resolver.workspaceMembers = workspaceMembers
	}
	return resolver
}

func (resolver *InstallationCredentialResolver) ResolveCredential(
	ctx context.Context,
	installation *channelruntime.Installation,
) (*channelruntime.ResolvedChannelCredential, error) {
	if resolver == nil || installation == nil || installation.ID == uuid.Nil ||
		installation.OrganizationID == uuid.Nil || installation.ConnectionID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	if !validChannelCredentialSource(installation.CredentialSource) {
		return nil, ErrChannelNotReady
	}
	switch channelCredentialSource(installation) {
	case channelruntime.CredentialSourceEmbedded, channelruntime.CredentialSourceLegacy:
		if installation.ConnectionID != installation.ID {
			// A legacy locator without an installation-owned envelope is not a
			// Connector PersonalConnection and must never fall back to one.
			return nil, ErrChannelNotReady
		}
		return resolver.resolveEmbeddedCredential(ctx, installation)
	case channelruntime.CredentialSourcePersonalConnection:
		if installation.ConnectionID == installation.ID {
			return nil, ErrChannelNotReady
		}
		return resolver.resolvePersonalConnectionCredential(ctx, installation)
	default:
		return nil, ErrChannelNotReady
	}
}

func (resolver *InstallationCredentialResolver) resolveEmbeddedCredential(
	ctx context.Context,
	installation *channelruntime.Installation,
) (*channelruntime.ResolvedChannelCredential, error) {
	stored, err := resolver.channelCredentials.ChannelInstallationCredentialByID(
		ctx, installation.OrganizationID, installation.ID,
	)
	if err != nil {
		return nil, err
	}
	if stored.WorkspaceID != installation.WorkspaceID || !strings.EqualFold(stored.Provider, installation.Provider) {
		return nil, ErrChannelNotReady
	}
	credentials, err := resolver.cipher.DecryptCredentials(stored.EncryptedCredentials, channelCredentialAAD(stored))
	if err != nil {
		if errors.Is(err, credentialcrypto.ErrCredentialCipherUnavailable) {
			return nil, ErrCredentialStorageUnavailable
		}
		return nil, fmt.Errorf("decrypt channel installation credential: %w", err)
	}
	return &channelruntime.ResolvedChannelCredential{
		Values: credentials, Version: stored.CredentialVersion,
		Source: channelruntime.CredentialSourceEmbedded,
	}, nil
}

// resolvePersonalConnectionCredential reads a Connector PersonalConnection
// through the owner+organization scoped resolver. The installation carries no
// secret material, and the decrypted map is returned as an ephemeral lease
// owned by the recipient.
func (resolver *InstallationCredentialResolver) resolvePersonalConnectionCredential(
	ctx context.Context,
	installation *channelruntime.Installation,
) (*channelruntime.ResolvedChannelCredential, error) {
	ownerID := channelConnectionOwnerID(installation)
	if resolver.personalConnections == nil || resolver.workspaceMembers == nil || ownerID == uuid.Nil {
		return nil, ErrChannelNotReady
	}
	member, err := resolver.workspaceMembers.IsMemberInWorkspace(
		ctx, ownerID.String(), installation.WorkspaceID.String(),
	)
	if err != nil || !member {
		return nil, ErrChannelNotReady
	}
	connection, values, err := resolver.personalConnections.ResolvePersonalConnectionCredentials(
		ctx, installation.OrganizationID, ownerID, installation.ConnectionID,
	)
	if err != nil {
		return nil, ErrChannelNotReady
	}
	registered, resolveErr := resolver.providers.ResolveProvider(installation.Provider)
	if resolveErr != nil {
		destroyChannelCredentials(values)
		return nil, ErrChannelNotReady
	}
	if !validPersonalConnectionForChannel(installation, ownerID, connection, registered.Provisioner.ConnectionSpec()) {
		destroyChannelCredentials(values)
		return nil, ErrChannelNotReady
	}
	// Connector field names follow the integration catalog, not the channel
	// runtime contract; let the provider map them for the transport this
	// credential will be served on.
	if normalizer, ok := registered.Provisioner.(channelruntime.CredentialNormalizer); ok {
		transport := "webhook"
		if registered.Receiver != nil {
			transport = registered.Receiver.Mode(channelruntime.ResolvedChannelCredential{
				Source: channelruntime.CredentialSourcePersonalConnection,
			})
		}
		normalized, normalizeErr := normalizer.NormalizeCredentials(values, transport)
		destroyChannelCredentials(values)
		if normalizeErr != nil || len(normalized) == 0 {
			destroyChannelCredentials(normalized)
			return nil, ErrChannelNotReady
		}
		values = normalized
	}
	// The envelope is only needed to prove the lease is backed by a ready,
	// owner-scoped connection; it must never leave this boundary.
	connection.EncryptedCredentials = ""
	return &channelruntime.ResolvedChannelCredential{
		Values: values, Version: connection.CredentialVersion, Revision: connection.Revision,
		Source: channelruntime.CredentialSourcePersonalConnection,
	}, nil
}

func validPersonalConnectionForChannel(
	installation *channelruntime.Installation,
	ownerID uuid.UUID,
	connection connectorcore.PersonalConnection,
	spec channelruntime.ConnectionSpec,
) bool {
	if installation == nil || connection.ID == uuid.Nil || connection.OrganizationID == uuid.Nil ||
		connection.OwnerAccountID == uuid.Nil || connection.ID != installation.ConnectionID ||
		connection.OrganizationID != installation.OrganizationID || connection.OwnerAccountID != ownerID ||
		!strings.EqualFold(connection.ConnectorID.String(), installation.Provider) ||
		!strings.EqualFold(connection.ProviderID.String(), spec.IntegrationID) ||
		!connectionIdentityMatches(connection.DriverID.String(), spec.DriverID, spec.CompatibleDriverIDs) ||
		!connectionIdentityMatches(connection.AuthMethodID.String(), spec.AuthMethodID, spec.CompatibleAuthMethodIDs) ||
		connection.State != connectorcore.ReadinessReady || connection.DeletedAt != nil ||
		connection.Revision < 1 || connection.CredentialVersion < 1 || connection.EncryptedCredentials == "" {
		return false
	}
	if connectorID := strings.TrimSpace(installation.ConnectorID); connectorID != "" &&
		!strings.EqualFold(connection.ConnectorID.String(), connectorID) {
		return false
	}
	return true
}

// connectionIdentityMatches accepts the provider's canonical Connector
// identifier or one of the compatibility identifiers it declares for already
// deployed Connector catalogs.
func connectionIdentityMatches(actual, canonical string, compatible []string) bool {
	if strings.EqualFold(strings.TrimSpace(actual), strings.TrimSpace(canonical)) {
		return true
	}
	for _, candidate := range compatible {
		if strings.EqualFold(strings.TrimSpace(actual), strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}

// validChannelCredentialSource rejects an unknown source instead of guessing a
// credential boundary. An empty value is a pre-migration installation whose
// source is derived from its credential locator.
func validChannelCredentialSource(source channelruntime.CredentialSource) bool {
	switch source {
	case "", channelruntime.CredentialSourceEmbedded, channelruntime.CredentialSourcePersonalConnection, channelruntime.CredentialSourceLegacy:
		return true
	default:
		return false
	}
}

func channelCredentialSource(installation *channelruntime.Installation) channelruntime.CredentialSource {
	if installation == nil {
		return ""
	}
	if installation.CredentialSource != "" {
		return installation.CredentialSource
	}
	if installation.ConnectionID == installation.ID {
		return channelruntime.CredentialSourceEmbedded
	}
	return channelruntime.CredentialSourcePersonalConnection
}

// channelConnectionOwnerID is the account that owns the credential lease. New
// installations persist ConnectionOwnerID; legacy rows fall back to the creator,
// which is the account that selected the connection.
func channelConnectionOwnerID(installation *channelruntime.Installation) uuid.UUID {
	if installation == nil {
		return uuid.Nil
	}
	if installation.ConnectionOwnerID != uuid.Nil {
		return installation.ConnectionOwnerID
	}
	return installation.CreatedBy
}

func (resolver *InstallationCredentialResolver) ResolveIdentity(
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

func (resolver *InstallationCredentialResolver) ContextReadiness(
	_ context.Context,
	installation *channelruntime.Installation,
) ChannelContextReadiness {
	if resolver == nil || installation == nil {
		return ChannelContextReadiness{Status: ChannelContextReadinessUnavailable}
	}
	registered, err := resolver.providers.ResolveProvider(installation.Provider)
	if err != nil {
		return ChannelContextReadiness{Status: ChannelContextReadinessUnavailable}
	}
	if !registered.Adapter.Capabilities().ContextRead {
		return ChannelContextReadiness{Status: ChannelContextReadinessNotSupported}
	}
	return ChannelContextReadiness{Status: ChannelContextReadinessRuntimeVerification}
}

// Resolve leases credentials for the host-selected installation. Embedded
// installations use the installation-owned envelope; Connector-backed
// installations resolve the owner-scoped PersonalConnection through the same
// validation boundary. Provider adapters never receive a repository handle.
func (resolver *InstallationCredentialResolver) Resolve(
	ctx context.Context,
	request providertransport.CredentialRequest,
) (*providertransport.CredentialLease, error) {
	if resolver == nil || resolver.channelCredentials == nil || resolver.cipher == nil || resolver.providers == nil {
		return nil, providertransport.NewError(providertransport.ErrorCodeConnectionInvalid, "channel credential resolver is unavailable", nil)
	}
	organizationID, organizationErr := uuid.Parse(strings.TrimSpace(request.OrganizationID))
	installationID, installationErr := uuid.Parse(strings.TrimSpace(request.InstallationID))
	if organizationErr != nil || installationErr != nil || organizationID == uuid.Nil || installationID == uuid.Nil {
		return nil, providertransport.NewError(providertransport.ErrorCodeConnectionNotFound, "channel credential was not found", nil)
	}
	installation, err := resolver.channelInstallation(ctx, installationID)
	if err != nil || installation == nil || installation.ID != installationID ||
		installation.OrganizationID != organizationID ||
		installation.WorkspaceID.String() != strings.TrimSpace(request.WorkspaceID) {
		return nil, providertransport.NewError(providertransport.ErrorCodeConnectionNotFound, "channel credential was not found", nil)
	}
	registered, providerErr := resolver.providers.ResolveProvider(installation.Provider)
	if providerErr != nil || !sameConnectionSpec(request, registered.Provisioner.ConnectionSpec()) {
		return nil, providertransport.NewError(providertransport.ErrorCodeConnectionNotFound, "channel credential was not found", nil)
	}
	credential, err := resolver.ResolveCredential(ctx, installation)
	if err != nil {
		return nil, providertransport.NewError(providertransport.ErrorCodeConnectionInvalid, "channel credential is not ready", nil)
	}
	spec := registered.Provisioner.ConnectionSpec()
	lease := &providertransport.CredentialLease{
		ID:                installation.ConnectionID.String(),
		OrganizationID:    organizationID.String(),
		ProviderID:        spec.IntegrationID,
		DriverID:          spec.DriverID,
		AuthMethodID:      spec.AuthMethodID,
		AccountID:         channelConnectionOwnerID(installation).String(),
		Credentials:       credential.Values,
		Config:            map[string]any{},
		CredentialVersion: credential.Version,
		Revision:          credential.Revision,
	}
	// Ownership of the credential map moves to the lease; the intermediate
	// resolved credential must not retain a second reference.
	credential.Values = nil
	credential.Destroy()
	return lease, nil
}

func (resolver *InstallationCredentialResolver) channelInstallation(
	ctx context.Context,
	installationID uuid.UUID,
) (*channelruntime.Installation, error) {
	reader, ok := resolver.channelCredentials.(channelInstallationReader)
	if !ok {
		return nil, ErrChannelInstallationNotFound
	}
	return reader.ChannelInstallationByID(ctx, installationID)
}

func (resolver *InstallationCredentialResolver) CreateEmbeddedInstallation(
	ctx context.Context,
	installation *channelruntime.Installation,
	updates map[string]string,
) error {
	_ = ctx
	if resolver == nil || installation == nil || installation.ID == uuid.Nil || installation.ConnectionID != installation.ID {
		return ErrInvalidRequest
	}
	installation.ConnectorID = strings.ToLower(strings.TrimSpace(installation.Provider))
	installation.ConnectionOwnerID = installation.CreatedBy
	installation.CredentialSource = channelruntime.CredentialSourceEmbedded
	installation.ConnectionRevision = 0
	installation.ConnectionCredentialVersion = 1
	registered, err := resolver.providers.ResolveProvider(installation.Provider)
	if err != nil {
		return ErrChannelNotReady
	}
	credentials, err := registered.Provisioner.PrepareCredentials(nil, updates)
	if err != nil {
		return err
	}
	defer destroyChannelCredentials(credentials)
	// Persist structurally valid credentials before contacting the provider.
	// The explicit installation test resolves and stores external identity so
	// users can obtain the callback address before completing provider setup.
	installation.ExternalTenantID = ""
	installation.ExternalBotID = ""
	envelope, err := resolver.cipher.EncryptCredentials(credentials, credentialcrypto.CredentialAAD{
		OrganizationID:    installation.OrganizationID,
		ConnectionID:      installation.ID,
		IntegrationID:     strings.ToLower(installation.Provider),
		CredentialVersion: 1,
	})
	if err != nil {
		if errors.Is(err, credentialcrypto.ErrCredentialCipherUnavailable) {
			return ErrCredentialStorageUnavailable
		}
		return fmt.Errorf("encrypt channel installation credential: %w", err)
	}
	credential := &ChannelInstallationCredential{
		InstallationID: installation.ID, OrganizationID: installation.OrganizationID, WorkspaceID: installation.WorkspaceID,
		Provider: strings.ToLower(installation.Provider), EncryptedCredentials: envelope, CredentialVersion: 1,
		CreatedBy: installation.CreatedBy, UpdatedBy: installation.UpdatedBy, CreatedAt: installation.CreatedAt, UpdatedAt: installation.UpdatedAt,
	}
	return resolver.channelCredentials.CreateChannelInstallationWithCredential(ctx, installation, credential)
}

func (resolver *InstallationCredentialResolver) ReplaceEmbeddedCredential(
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
			if errors.Is(err, credentialcrypto.ErrCredentialCipherUnavailable) {
				return ErrCredentialStorageUnavailable
			}
			return fmt.Errorf("decrypt channel installation credential: %w", err)
		}
	case errors.Is(loadErr, ErrChannelCredentialNotFound):
		// A complete update can switch a Connector reference to an
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
	installation.ConnectionID = installation.ID
	installation.ConnectorID = strings.ToLower(strings.TrimSpace(installation.Provider))
	installation.ConnectionOwnerID = installation.CreatedBy
	installation.CredentialSource = channelruntime.CredentialSourceEmbedded
	installation.ConnectionRevision = 0
	installation.ConnectionCredentialVersion = version
	installation.ExternalTenantID = ""
	installation.ExternalBotID = ""
	installation.UpdatedAt = now
	envelope, err := resolver.cipher.EncryptCredentials(credentials, credentialcrypto.CredentialAAD{
		OrganizationID:    scope.OrganizationID,
		ConnectionID:      installation.ID,
		IntegrationID:     strings.ToLower(installation.Provider),
		CredentialVersion: version,
	})
	if err != nil {
		if errors.Is(err, credentialcrypto.ErrCredentialCipherUnavailable) {
			return ErrCredentialStorageUnavailable
		}
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

func channelCredentialAAD(credential *ChannelInstallationCredential) credentialcrypto.CredentialAAD {
	return credentialcrypto.CredentialAAD{
		OrganizationID:    credential.OrganizationID,
		ConnectionID:      credential.InstallationID,
		IntegrationID:     strings.ToLower(credential.Provider),
		CredentialVersion: credential.CredentialVersion,
	}
}

func sameConnectionSpec(request providertransport.CredentialRequest, spec channelruntime.ConnectionSpec) bool {
	return strings.EqualFold(strings.TrimSpace(request.ProviderID), strings.TrimSpace(spec.IntegrationID)) &&
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
