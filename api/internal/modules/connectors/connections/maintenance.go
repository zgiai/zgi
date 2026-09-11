package connections

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

type MaintenanceRepository interface {
	GetConnection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (connectorcore.PersonalConnection, error)
	UpdateConnection(context.Context, uuid.UUID, uuid.UUID, connectorcore.PersonalConnection, int) (connectorcore.PersonalConnection, error)
	ClaimSetupAttempt(context.Context, connectorcore.KeySetupAttempt) (connectorcore.KeySetupAttempt, bool, error)
	GetSetupAttempt(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (connectorcore.KeySetupAttempt, error)
	CompleteReconnectReady(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, ReconnectCompletion) (connectorcore.KeySetupAttempt, error)
	CompleteSetupFailed(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, connectorcore.ErrorCode) (connectorcore.KeySetupAttempt, error)
	ForceRemoveConnection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int, []BindingRevisionSnapshot) error
}

type MaintenanceBindingReader interface {
	ListBindingsByConnection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, connectorbindings.ListOptions) ([]connectorcore.TargetBinding, error)
}

// MaintenanceCredentialResolver decrypts one owner-scoped Connection into a
// disposable request map. Implementations must apply the same AAD tuple used at
// creation and reconnect.
type MaintenanceCredentialResolver interface {
	ResolveConnectionCredentials(context.Context, connectorcore.PersonalConnection) (map[string]string, error)
}

type ReconnectCommand struct {
	OrganizationID          uuid.UUID
	OwnerAccountID          uuid.UUID
	ConnectionID            uuid.UUID
	IdempotencyKey          string            `json:"-"`
	Values                  map[string]string `json:"-"`
	MailUsage               string
	ConfirmUnstableIdentity bool
}

type ReconnectResult struct {
	KeySetupResult
	ReplacedExisting bool
}

type ConnectionRemovalImpact struct {
	ConnectionID     uuid.UUID
	CanRemove        bool
	ImpactToken      string
	ExpiresAt        time.Time
	AffectedBindings []connectorcore.TargetBinding
}

type RemoveConnectionCommand struct {
	OrganizationID uuid.UUID
	OwnerAccountID uuid.UUID
	ConnectionID   uuid.UUID
	Force          bool
	ImpactToken    string
}

type MaintenanceService struct {
	catalog      DefinitionCatalog
	repository   MaintenanceRepository
	bindings     MaintenanceBindingReader
	verifier     CandidateVerifier
	sealer       CredentialSealer
	credentials  MaintenanceCredentialResolver
	hasher       *CommandHasher
	impactTokens *ImpactTokenCodec
	now          func() time.Time
	newID        func() uuid.UUID
	setupTTL     time.Duration
}

func NewMaintenanceService(
	definitionCatalog DefinitionCatalog,
	repository MaintenanceRepository,
	bindings MaintenanceBindingReader,
	verifier CandidateVerifier,
	sealer CredentialSealer,
	credentials MaintenanceCredentialResolver,
	hasher *CommandHasher,
	impactTokens *ImpactTokenCodec,
) (*MaintenanceService, error) {
	if definitionCatalog == nil || repository == nil || bindings == nil || verifier == nil || sealer == nil ||
		credentials == nil || hasher == nil || impactTokens == nil {
		return nil, connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	return &MaintenanceService{
		catalog: definitionCatalog, repository: repository, bindings: bindings,
		verifier: verifier, sealer: sealer, credentials: credentials,
		hasher: hasher, impactTokens: impactTokens,
		now: func() time.Time { return time.Now().UTC() }, newID: uuid.New, setupTTL: defaultSetupTTL,
	}, nil
}

func (service *MaintenanceService) Rename(
	ctx context.Context,
	organizationID, ownerAccountID, connectionID uuid.UUID,
	expectedRevision int,
	name string,
) (connectorcore.PersonalConnection, error) {
	if err := service.available(); err != nil {
		return connectorcore.PersonalConnection{}, err
	}
	name = strings.TrimSpace(name)
	if organizationID == uuid.Nil || ownerAccountID == uuid.Nil || connectionID == uuid.Nil ||
		expectedRevision < 1 || name == "" || utf8.RuneCountInString(name) > 128 {
		return connectorcore.PersonalConnection{}, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	connection, err := service.repository.GetConnection(ctx, organizationID, ownerAccountID, connectionID)
	if err != nil {
		return connectorcore.PersonalConnection{}, err
	}
	if connection.Revision != expectedRevision {
		return connectorcore.PersonalConnection{}, connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	connection.Name = name
	lifecycle, err := service.lifecycleRepository()
	if err != nil {
		return connectorcore.PersonalConnection{}, err
	}
	updated, err := lifecycle.UpdateConnectionWithLifecycle(
		ctx, organizationID, ownerAccountID, connection, expectedRevision,
		newConnectionLifecycleMutation(
			MutationConnectionRenamed,
			"connection-rename:"+connection.ID.String()+":"+strconv.Itoa(expectedRevision),
			service.now().UTC(),
			ConnectionChange{Type: MutationConnectionRenamed, Name: name},
		),
	)
	if err != nil {
		return connectorcore.PersonalConnection{}, err
	}
	updated.EncryptedCredentials = ""
	return updated, nil
}

// Reconnect verifies a request-scoped candidate before sealing or mutating the
// current Connection. Same stable identity swaps the credential and increments
// its version; a different identity creates a new unbound Connection. When a
// provider yields no stable identity, activation requires an explicit owner
// confirmation and a failed attempt leaves the prior Key active.
func (service *MaintenanceService) Reconnect(ctx context.Context, command ReconnectCommand) (ReconnectResult, error) {
	defer connectorcore.DestroySecretValues(command.Values)
	if err := service.available(); err != nil {
		return ReconnectResult{}, err
	}
	if command.OrganizationID == uuid.Nil || command.OwnerAccountID == uuid.Nil || command.ConnectionID == uuid.Nil || len(command.Values) == 0 {
		return ReconnectResult{}, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	existing, err := service.repository.GetConnection(ctx, command.OrganizationID, command.OwnerAccountID, command.ConnectionID)
	if err != nil {
		return ReconnectResult{}, err
	}
	definition, found := service.catalog.Get(existing.ConnectorID.String())
	if found && definition.IsMailCredentials() {
		if catalog.NormalizeMailValuesFor(definition.ID, command.Values) != nil || existing.ExternalAccountID == nil || *existing.ExternalAccountID != command.Values["email"] || existing.AuthMethodID != connectorcore.Identifier(definition.AuthMethodID) {
			return ReconnectResult{}, connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
		}
		if command.MailUsage == "" {
			command.MailUsage = mailUsage(existing)
		}
		if len(catalog.MailOperationsFor(definition.ID, command.MailUsage)) == 0 {
			return ReconnectResult{}, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
		}
	} else if command.MailUsage != "" {
		return ReconnectResult{}, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	if !found || !exactCredentialSchema(definition, command.Values) {
		return ReconnectResult{}, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	if definition.IsMessagingCredentials() && (existing.ExternalAccountID == nil || *existing.ExternalAccountID != catalog.CredentialIdentity(definition, command.Values)) {
		return ReconnectResult{}, connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	idempotencyHash, err := service.hasher.IdempotencyKeyHash(command.OrganizationID, command.OwnerAccountID, command.IdempotencyKey)
	if err != nil {
		return ReconnectResult{}, err
	}
	fingerprint, err := service.hasher.RequestFingerprint(CommandFingerprintInput{
		ConnectorID: existing.ConnectorID.String(), ReconnectConnectionID: existing.ID, Values: command.Values,
		MailUsage: command.MailUsage, DriverID: definition.DriverID, AuthMethodID: definition.AuthMethodID,
	})
	if err != nil {
		return ReconnectResult{}, err
	}
	now := service.now().UTC()
	attempt := connectorcore.KeySetupAttempt{
		ID: service.newID(), OrganizationID: command.OrganizationID, OwnerAccountID: command.OwnerAccountID,
		ConnectorID: existing.ConnectorID, ConnectionID: existing.ID,
		IdempotencyKeyHash: idempotencyHash, RequestFingerprint: fingerprint,
		State: connectorcore.SetupStateVerifying, ExpiresAt: now.Add(service.setupTTL), CreatedAt: now, UpdatedAt: now,
	}
	claimed, created, err := service.repository.ClaimSetupAttempt(ctx, attempt)
	if err != nil {
		return ReconnectResult{}, err
	}
	if !created {
		return service.reconnectResultForAttempt(ctx, existing, claimed, true)
	}

	verification, err := service.verifier.Verify(ctx, CandidateVerificationRequest{
		OrganizationID: command.OrganizationID, OwnerAccountID: command.OwnerAccountID,
		ConnectionID: existing.ID, ConnectorID: definition.ID, DriverID: definition.DriverID,
		AuthMethodID: definition.AuthMethodID, Credentials: command.Values,
		MailUsage: command.MailUsage, OperationIDs: catalog.MailOperationsFor(definition.ID, command.MailUsage),
	})
	if err != nil {
		result, failure := service.failReconnect(ctx, claimed, err)
		result.MailVerification = verification.MailVerification.SafeCopy()
		return result, failure
	}
	if err := validateMailVerification(definition, command.MailUsage, command.Values["email"], verification); err != nil {
		return service.failReconnect(ctx, claimed, err)
	}
	usable, unavailable, err := verifiedOperations(definition, verification.UsableOperationIDs)
	if err != nil {
		return service.failReconnect(ctx, claimed, err)
	}
	externalAccountID, externalDisplayName, err := verifiedIdentity(verification)
	if err != nil {
		return service.failReconnect(ctx, claimed, err)
	}
	existingIdentity := optionalValue(existing.ExternalAccountID)
	if (definition.IsMessagingCredentials() || definition.IsMailCredentials()) && (externalAccountID == "" || externalAccountID != existingIdentity) {
		return service.failReconnect(ctx, claimed, connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil))
	}
	stableIdentity := existingIdentity != "" && externalAccountID != ""
	replaceExisting := stableIdentity && existingIdentity == externalAccountID
	if stableIdentity && !replaceExisting {
		// A different verified account is a new unbound Personal Connection.
	} else if !stableIdentity {
		if !command.ConfirmUnstableIdentity {
			return service.failReconnect(ctx, claimed, connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil))
		}
		replaceExisting = true
	}

	connectionID := service.newID()
	credentialVersion := 1
	name := defaultConnectionName(definition.Name, externalAccountID, externalDisplayName)
	createdAt := now
	revision := 1
	if replaceExisting {
		connectionID = existing.ID
		credentialVersion = existing.CredentialVersion + 1
		name = existing.Name
		createdAt = existing.CreatedAt
		revision = existing.Revision
	}
	envelope, err := service.sealer.Seal(CredentialSealRequest{
		OrganizationID: command.OrganizationID, ConnectionID: connectionID,
		ConnectorID: definition.ID, CredentialVersion: credentialVersion, Values: command.Values,
	})
	if err != nil {
		return service.failReconnect(ctx, claimed, connectorcore.NewError(connectorcore.ErrorCodeInternal, true, nil))
	}
	constraint := connectorcore.OwnerConstraint(command.OwnerAccountID, usable)
	if replaceExisting {
		constraint = existing.Constraint
		if definition.IsMailCredentials() && constraint != nil {
			copy := *constraint
			copy.AllowedOperationIDs = append([]connectorcore.Identifier{}, usable...)
			constraint = &copy
		}
	}
	resultConnection := connectorcore.PersonalConnection{
		Constraint: constraint,
		ID:         connectionID, OrganizationID: command.OrganizationID, OwnerAccountID: command.OwnerAccountID,
		ConnectorID: existing.ConnectorID, ProviderID: existing.ProviderID, DriverID: existing.DriverID, AuthMethodID: existing.AuthMethodID,
		Name: name, ExternalAccountID: optionalString(externalAccountID), ExternalDisplayName: optionalString(externalDisplayName),
		EncryptedCredentials: envelope, CredentialVersion: credentialVersion, State: connectorcore.ReadinessReady,
		ProviderEvidence:   verification.Evidence.Clone(),
		UsableOperationIDs: usable, UnavailableOperations: unavailable, Revision: revision,
		LastVerifiedAt: now, LastProviderSuccessAt: timePointer(now), CreatedAt: createdAt, UpdatedAt: now,
	}
	changes := []ConnectionChange(nil)
	if replaceExisting {
		changes = append(changes,
			ConnectionChange{
				Type: MutationCredentialRotated, CredentialVersion: resultConnection.CredentialVersion,
			},
			ConnectionChange{Type: MutationReadinessChanged, Readiness: resultConnection.State},
		)
	}
	lifecycle, err := service.lifecycleRepository()
	if err != nil {
		return ReconnectResult{}, err
	}
	completed, err := lifecycle.CompleteReconnectReadyWithLifecycle(
		ctx, command.OrganizationID, command.OwnerAccountID, claimed.ID,
		ReconnectCompletion{
			ExistingConnectionID: existing.ID, ExpectedRevision: existing.Revision,
			ReplaceExisting: replaceExisting, Result: resultConnection,
		},
		newConnectionLifecycleMutation(
			DecisionConnectionReconnected, "connection-reconnect:"+claimed.ID.String(), now, changes...,
		),
	)
	if err != nil {
		return ReconnectResult{}, err
	}
	if replaceExisting {
		resultConnection.Revision = existing.Revision + 1
	}
	resultConnection.EncryptedCredentials = ""
	return ReconnectResult{
		KeySetupResult:   KeySetupResult{Attempt: completed, Connection: &resultConnection},
		ReplacedExisting: replaceExisting,
	}, nil
}

func (service *MaintenanceService) Diagnose(
	ctx context.Context,
	organizationID, ownerAccountID, connectionID uuid.UUID,
) (connectorcore.PersonalConnection, error) {
	if err := service.available(); err != nil {
		return connectorcore.PersonalConnection{}, err
	}
	connection, err := service.repository.GetConnection(ctx, organizationID, ownerAccountID, connectionID)
	if err != nil {
		return connectorcore.PersonalConnection{}, err
	}
	definition, found := service.catalog.Get(connection.ConnectorID.String())
	if !found {
		return service.persistDiagnostic(ctx, connection, connectorcore.ErrorCodeConnectorUnavailable)
	}
	credentials, err := service.credentials.ResolveConnectionCredentials(ctx, connection)
	if err != nil {
		return service.persistDiagnostic(ctx, connection, connectorcore.ErrorCodeInternal)
	}
	defer connectorcore.DestroySecretValues(credentials)
	verification, verifyErr := service.verifier.Verify(ctx, CandidateVerificationRequest{
		OrganizationID: organizationID, OwnerAccountID: ownerAccountID, ConnectionID: connection.ID,
		ConnectorID: definition.ID, DriverID: definition.DriverID, AuthMethodID: definition.AuthMethodID,
		Credentials: credentials,
		MailUsage:   mailUsage(connection), OperationIDs: mailOperationIDs(connection),
	})
	if verifyErr != nil {
		return service.persistDiagnostic(ctx, connection, connectorcore.ErrorCodeOf(verifyErr))
	}
	if err := validateMailVerification(definition, mailUsage(connection), optionalValue(connection.ExternalAccountID), verification); err != nil {
		return service.persistDiagnostic(ctx, connection, connectorcore.ErrorCodeOf(err))
	}
	usable, unavailable, verifyErr := verifiedOperations(definition, verification.UsableOperationIDs)
	if verifyErr != nil {
		return service.persistDiagnostic(ctx, connection, connectorcore.ErrorCodeOf(verifyErr))
	}
	externalAccountID, externalDisplayName, verifyErr := verifiedIdentity(verification)
	if verifyErr != nil {
		return service.persistDiagnostic(ctx, connection, connectorcore.ErrorCodeOf(verifyErr))
	}
	now := service.now().UTC()
	connection.State = connectorcore.ReadinessReady
	// Diagnostics refresh evidence, but cannot accept newly shipped WeCom
	// resource operations. Only explicit reconnect admits that larger set.
	if isMessagingConnection(connection) && connection.ConnectorID == "wecom" {
		accepted := make(map[connectorcore.Identifier]bool)
		for _, id := range connection.UsableOperationIDs {
			accepted[id] = true
		}
		for id := range connection.UnavailableOperations {
			accepted[id] = true
		}
		filtered := make([]connectorcore.Identifier, 0, len(usable))
		for _, id := range usable {
			if !catalog.IsWeComResourceRead(id.String()) || accepted[id] {
				filtered = append(filtered, id)
			}
		}
		for id := range unavailable {
			if catalog.IsWeComResourceRead(id.String()) && !accepted[id] {
				delete(unavailable, id)
			}
		}
		usable = filtered
	}
	connection.UsableOperationIDs = usable
	connection.UnavailableOperations = unavailable
	connection.DiagnosticCode = nil
	connection.ProviderEvidence = verification.Evidence.Clone()
	connection.ExternalAccountID = optionalString(externalAccountID)
	connection.ExternalDisplayName = optionalString(externalDisplayName)
	connection.LastVerifiedAt = now
	connection.LastProviderSuccessAt = timePointer(now)
	connection.LastProviderFailureAt = nil
	lifecycle, err := service.lifecycleRepository()
	if err != nil {
		return connectorcore.PersonalConnection{}, err
	}
	updated, err := lifecycle.UpdateConnectionWithLifecycle(
		ctx, organizationID, ownerAccountID, connection, connection.Revision,
		newConnectionLifecycleMutation(
			DecisionConnectionDiagnosed,
			"connection-diagnose:"+connection.ID.String()+":"+strconv.Itoa(connection.Revision),
			now,
			ConnectionChange{Type: MutationReadinessChanged, Readiness: connection.State},
		),
	)
	if err != nil {
		return connectorcore.PersonalConnection{}, err
	}
	updated.EncryptedCredentials = ""
	return updated, nil
}

func (service *MaintenanceService) GetRemovalImpact(
	ctx context.Context,
	organizationID, ownerAccountID, connectionID uuid.UUID,
) (ConnectionRemovalImpact, error) {
	if err := service.available(); err != nil {
		return ConnectionRemovalImpact{}, err
	}
	connection, err := service.repository.GetConnection(ctx, organizationID, ownerAccountID, connectionID)
	if err != nil {
		return ConnectionRemovalImpact{}, err
	}
	bindings, err := service.bindings.ListBindingsByConnection(
		ctx, organizationID, ownerAccountID, connectionID,
		connectorbindings.ListOptions{},
	)
	if err != nil {
		return ConnectionRemovalImpact{}, err
	}
	snapshot := activeBindingSnapshot(bindings)
	token, expiresAt, err := service.impactTokens.Issue(
		organizationID, ownerAccountID, connectionID, connection.Revision, snapshot, service.now().UTC(),
	)
	if err != nil {
		return ConnectionRemovalImpact{}, err
	}
	return ConnectionRemovalImpact{
		ConnectionID: connectionID, CanRemove: len(snapshot) == 0,
		ImpactToken: token, ExpiresAt: expiresAt, AffectedBindings: bindings,
	}, nil
}

func (service *MaintenanceService) Remove(ctx context.Context, command RemoveConnectionCommand) error {
	if err := service.available(); err != nil {
		return err
	}
	if command.OrganizationID == uuid.Nil || command.OwnerAccountID == uuid.Nil || command.ConnectionID == uuid.Nil {
		return connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	connection, err := service.repository.GetConnection(ctx, command.OrganizationID, command.OwnerAccountID, command.ConnectionID)
	if err != nil {
		return err
	}
	bindings, err := service.bindings.ListBindingsByConnection(
		ctx, command.OrganizationID, command.OwnerAccountID, command.ConnectionID,
		connectorbindings.ListOptions{},
	)
	if err != nil {
		return err
	}
	snapshot := activeBindingSnapshot(bindings)
	if len(snapshot) > 0 && !command.Force {
		return connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	if command.Force {
		if err := service.impactTokens.Verify(
			command.ImpactToken, command.OrganizationID, command.OwnerAccountID, command.ConnectionID,
			connection.Revision, snapshot, service.now().UTC(),
		); err != nil {
			return err
		}
	} else {
		snapshot = nil
	}
	lifecycle, err := service.lifecycleRepository()
	if err != nil {
		return err
	}
	return lifecycle.ForceRemoveConnectionWithLifecycle(
		ctx, command.OrganizationID, command.OwnerAccountID, command.ConnectionID, connection.Revision, snapshot,
		newConnectionLifecycleMutation(
			MutationConnectionRemoved,
			"connection-remove:"+connection.ID.String()+":"+strconv.Itoa(connection.Revision),
			service.now().UTC(),
			ConnectionChange{Type: MutationConnectionRemoved},
		),
	)
}

func (service *MaintenanceService) reconnectResultForAttempt(
	ctx context.Context,
	existing connectorcore.PersonalConnection,
	attempt connectorcore.KeySetupAttempt,
	failureAsError bool,
) (ReconnectResult, error) {
	result := ReconnectResult{KeySetupResult: KeySetupResult{Attempt: attempt, Retryable: retryableCode(attempt.ErrorCode)}}
	if attempt.State == connectorcore.SetupStateReady {
		connection, err := service.repository.GetConnection(ctx, attempt.OrganizationID, attempt.OwnerAccountID, attempt.ResultConnectionID)
		if err != nil {
			return ReconnectResult{}, err
		}
		connection.EncryptedCredentials = ""
		result.Connection = &connection
		result.ReplacedExisting = attempt.ResultConnectionID == existing.ID
	}
	if failureAsError && attempt.State == connectorcore.SetupStateFailed {
		return result, connectorcore.NewError(attempt.ErrorCode, result.Retryable, nil)
	}
	return result, nil
}

func (service *MaintenanceService) failReconnect(
	ctx context.Context,
	attempt connectorcore.KeySetupAttempt,
	failure error,
) (ReconnectResult, error) {
	code := connectorcore.ErrorCodeOf(failure)
	if !code.Valid() {
		code = connectorcore.ErrorCodeInternal
	}
	failed, err := service.repository.CompleteSetupFailed(ctx, attempt.OrganizationID, attempt.OwnerAccountID, attempt.ID, code)
	if err != nil {
		return ReconnectResult{}, err
	}
	retryable := retryableError(failure) || retryableCode(code)
	return ReconnectResult{KeySetupResult: KeySetupResult{Attempt: failed, Retryable: retryable}}, connectorcore.NewError(code, retryable, nil)
}

func (service *MaintenanceService) persistDiagnostic(
	ctx context.Context,
	connection connectorcore.PersonalConnection,
	code connectorcore.ErrorCode,
) (connectorcore.PersonalConnection, error) {
	if !code.Valid() {
		code = connectorcore.ErrorCodeInternal
	}
	now := service.now().UTC()
	if code == connectorcore.ErrorCodeInvalidKey || code == connectorcore.ErrorCodeInsufficientKey || code == connectorcore.ErrorCodeInvalidAppCredentials || code == connectorcore.ErrorCodeMailAuthenticationRejected || code == connectorcore.ErrorCodeMailAppPasswordRequired {
		connection.State = connectorcore.ReadinessReconnectRequired
	} else {
		connection.State = connectorcore.ReadinessUnavailable
	}
	diagnostic := string(code)
	connection.DiagnosticCode = &diagnostic
	connection.LastProviderFailureAt = timePointer(now)
	lifecycle, err := service.lifecycleRepository()
	if err != nil {
		return connectorcore.PersonalConnection{}, err
	}
	updated, err := lifecycle.UpdateConnectionWithLifecycle(
		ctx, connection.OrganizationID, connection.OwnerAccountID, connection, connection.Revision,
		newConnectionLifecycleMutation(
			DecisionConnectionDiagnosed,
			"connection-diagnose:"+connection.ID.String()+":"+strconv.Itoa(connection.Revision),
			now,
			ConnectionChange{Type: MutationReadinessChanged, Readiness: connection.State},
		),
	)
	if err != nil {
		return connectorcore.PersonalConnection{}, err
	}
	updated.EncryptedCredentials = ""
	return updated, nil
}

func (service *MaintenanceService) available() error {
	if service == nil || service.catalog == nil || service.repository == nil || service.bindings == nil ||
		service.verifier == nil || service.sealer == nil || service.credentials == nil || service.hasher == nil ||
		service.impactTokens == nil || service.now == nil || service.newID == nil || service.setupTTL <= 0 {
		return connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	return nil
}

func (service *MaintenanceService) lifecycleRepository() (MaintenanceLifecycleRepository, error) {
	repository, ok := service.repository.(MaintenanceLifecycleRepository)
	if !ok {
		return nil, connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	return repository, nil
}

func activeBindingSnapshot(bindings []connectorcore.TargetBinding) []BindingRevisionSnapshot {
	result := make([]BindingRevisionSnapshot, 0, len(bindings))
	for _, binding := range bindings {
		if binding.Status == connectorcore.BindingStatusActive {
			result = append(result, BindingRevisionSnapshot{ID: binding.ID, Revision: binding.Revision})
		}
	}
	sortBindingSnapshots(result)
	return result
}

func optionalValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
