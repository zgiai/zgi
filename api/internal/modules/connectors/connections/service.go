package connections

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

const defaultSetupTTL = 10 * time.Minute

// DefinitionCatalog is the immutable, secret-free definition view required by
// the Key lifecycle. It intentionally exposes none of the legacy integration
// registry or setup-policy APIs.
type DefinitionCatalog interface {
	Get(connectorID string) (catalog.Definition, bool)
}

// KeySetupCommand contains request-scoped candidate values. StartKeySetup
// destroys Values on every return path, including validation and repository
// failures, so callers must pass a disposable map.
type KeySetupCommand struct {
	OrganizationID uuid.UUID
	OwnerAccountID uuid.UUID
	ConnectorID    string
	IdempotencyKey string            `json:"-"`
	Values         map[string]string `json:"-"`
	Target         *connectorcore.TargetRef
}

// KeySetupResult is transport-neutral and contains no submitted value,
// credential envelope, request digest, provider cause, or policy state.
type KeySetupResult struct {
	Attempt    connectorcore.KeySetupAttempt
	Connection *connectorcore.PersonalConnection
	Retryable  bool
}

type Service struct {
	catalog    DefinitionCatalog
	repository Repository
	verifier   CandidateVerifier
	sealer     CredentialSealer
	hasher     *CommandHasher
	now        func() time.Time
	newID      func() uuid.UUID
	setupTTL   time.Duration
}

func NewService(
	definitionCatalog DefinitionCatalog,
	repository Repository,
	verifier CandidateVerifier,
	sealer CredentialSealer,
	hasher *CommandHasher,
) (*Service, error) {
	if definitionCatalog == nil || repository == nil || verifier == nil || sealer == nil || hasher == nil {
		return nil, connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	return &Service{
		catalog: definitionCatalog, repository: repository, verifier: verifier, sealer: sealer, hasher: hasher,
		now: func() time.Time { return time.Now().UTC() }, newID: uuid.New, setupTTL: defaultSetupTTL,
	}, nil
}

// StartKeySetup performs exactly one bounded provider verification for a newly
// claimed command. A duplicate idempotent command observes the existing state
// and never verifies or seals the candidate again.
func (service *Service) StartKeySetup(ctx context.Context, command KeySetupCommand) (KeySetupResult, error) {
	defer connectorcore.DestroySecretValues(command.Values)
	if err := service.available(); err != nil {
		return KeySetupResult{}, err
	}
	definition, connectorID, err := service.validateCommand(command)
	if err != nil {
		return KeySetupResult{}, err
	}
	idempotencyHash, err := service.hasher.IdempotencyKeyHash(command.OrganizationID, command.OwnerAccountID, command.IdempotencyKey)
	if err != nil {
		return KeySetupResult{}, err
	}
	requestFingerprint, err := service.hasher.RequestFingerprint(CommandFingerprintInput{
		ConnectorID: connectorID.String(), Target: cloneTarget(command.Target), Values: command.Values,
	})
	if err != nil {
		return KeySetupResult{}, err
	}

	now := service.now().UTC()
	attempt := connectorcore.KeySetupAttempt{
		ID: service.newID(), OrganizationID: command.OrganizationID, OwnerAccountID: command.OwnerAccountID,
		ConnectorID: connectorID, Target: cloneTarget(command.Target), IdempotencyKeyHash: idempotencyHash,
		RequestFingerprint: requestFingerprint, State: connectorcore.SetupStateVerifying,
		ExpiresAt: now.Add(service.setupTTL), CreatedAt: now, UpdatedAt: now,
	}
	claimed, created, err := service.repository.ClaimSetupAttempt(ctx, attempt)
	if err != nil {
		return KeySetupResult{}, err
	}
	if !created {
		return service.resultForAttempt(ctx, claimed, true)
	}

	connectionID := service.newID()
	verification, err := service.verifier.Verify(ctx, CandidateVerificationRequest{
		OrganizationID: command.OrganizationID, OwnerAccountID: command.OwnerAccountID,
		ConnectionID: connectionID, ConnectorID: definition.ID, DriverID: definition.DriverID,
		AuthMethodID: definition.AuthMethodID, Credentials: command.Values,
	})
	if err != nil {
		return service.failClaimedSetup(ctx, claimed, err)
	}
	usable, unavailable, err := verifiedOperations(definition, verification.UsableOperationIDs)
	if err != nil {
		return service.failClaimedSetup(ctx, claimed, err)
	}

	envelope, err := service.sealer.Seal(CredentialSealRequest{
		OrganizationID: command.OrganizationID, ConnectionID: connectionID,
		ConnectorID: definition.ID, CredentialVersion: 1, Values: command.Values,
	})
	if err != nil {
		return service.failClaimedSetup(ctx, claimed, connectorcore.NewError(connectorcore.ErrorCodeInternal, true, nil))
	}

	externalAccountID, externalDisplayName, err := verifiedIdentity(verification)
	if err != nil {
		return service.failClaimedSetup(ctx, claimed, err)
	}
	connection := connectorcore.PersonalConnection{
		Constraint: connectorcore.OwnerConstraint(command.OwnerAccountID, usable),
		ID:         connectionID, OrganizationID: command.OrganizationID, OwnerAccountID: command.OwnerAccountID,
		ConnectorID: connectorID, ProviderID: connectorcore.Identifier(definition.ProviderID),
		DriverID: connectorcore.Identifier(definition.DriverID), AuthMethodID: connectorcore.Identifier(definition.AuthMethodID),
		Name:              defaultConnectionName(definition.Name, externalAccountID, externalDisplayName),
		ExternalAccountID: optionalString(externalAccountID), ExternalDisplayName: optionalString(externalDisplayName),
		EncryptedCredentials: envelope, CredentialVersion: 1, State: connectorcore.ReadinessReady,
		ProviderEvidence:   verification.Evidence.Clone(),
		UsableOperationIDs: usable, UnavailableOperations: unavailable, Revision: 1,
		LastVerifiedAt: now, LastProviderSuccessAt: timePointer(now), CreatedAt: now, UpdatedAt: now,
	}
	completed, err := service.repository.CompleteSetupReady(ctx, command.OrganizationID, command.OwnerAccountID, claimed.ID, connection)
	if err != nil {
		return service.failClaimedSetup(ctx, claimed, err)
	}
	return KeySetupResult{Attempt: completed, Connection: publicConnectionPointer(connection)}, nil
}

func (service *Service) GetKeySetup(
	ctx context.Context,
	organizationID, ownerAccountID, attemptID uuid.UUID,
) (KeySetupResult, error) {
	if err := service.available(); err != nil {
		return KeySetupResult{}, err
	}
	attempt, err := service.repository.GetSetupAttempt(ctx, organizationID, ownerAccountID, attemptID)
	if err != nil {
		return KeySetupResult{}, err
	}
	return service.resultForAttempt(ctx, attempt, false)
}

func (service *Service) CancelKeySetup(
	ctx context.Context,
	organizationID, ownerAccountID, attemptID uuid.UUID,
) (connectorcore.KeySetupAttempt, error) {
	if err := service.available(); err != nil {
		return connectorcore.KeySetupAttempt{}, err
	}
	return service.repository.CancelSetupAttempt(ctx, organizationID, ownerAccountID, attemptID)
}

func (service *Service) ListConnections(
	ctx context.Context,
	organizationID, ownerAccountID uuid.UUID,
	filter ConnectionListFilter,
) ([]connectorcore.PersonalConnection, error) {
	if err := service.available(); err != nil {
		return nil, err
	}
	items, err := service.repository.ListConnections(ctx, organizationID, ownerAccountID, filter)
	if err != nil {
		return nil, err
	}
	for index := range items {
		items[index].EncryptedCredentials = ""
	}
	return items, nil
}

func (service *Service) GetConnection(
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
	connection.EncryptedCredentials = ""
	return connection, nil
}

func (service *Service) validateCommand(command KeySetupCommand) (catalog.Definition, connectorcore.Identifier, error) {
	if command.OrganizationID == uuid.Nil || command.OwnerAccountID == uuid.Nil || len(command.Values) == 0 {
		return catalog.Definition{}, "", connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	connectorID, err := connectorcore.NormalizeIdentifier(command.ConnectorID)
	if err != nil || connectorID.String() != command.ConnectorID {
		return catalog.Definition{}, "", connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	definition, ok := service.catalog.Get(connectorID.String())
	if !ok {
		return catalog.Definition{}, "", connectorcore.NewError(connectorcore.ErrorCodeNotFound, false, nil)
	}
	if err := catalog.Validate(definition); err != nil || definition.ID != connectorID.String() || !exactCredentialSchema(definition, command.Values) {
		return catalog.Definition{}, "", connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	if command.Target != nil {
		if err := command.Target.Validate(); err != nil {
			return catalog.Definition{}, "", connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
		}
	}
	return definition, connectorID, nil
}

func (service *Service) resultForAttempt(ctx context.Context, attempt connectorcore.KeySetupAttempt, failureAsError bool) (KeySetupResult, error) {
	result := KeySetupResult{Attempt: attempt, Retryable: retryableCode(attempt.ErrorCode)}
	if attempt.State == connectorcore.SetupStateReady {
		connection, err := service.repository.GetConnection(ctx, attempt.OrganizationID, attempt.OwnerAccountID, attempt.ResultConnectionID)
		if err != nil {
			return KeySetupResult{}, err
		}
		result.Connection = publicConnectionPointer(connection)
	}
	if failureAsError && attempt.State == connectorcore.SetupStateFailed {
		return result, connectorcore.NewError(attempt.ErrorCode, result.Retryable, nil)
	}
	return result, nil
}

func (service *Service) failClaimedSetup(ctx context.Context, attempt connectorcore.KeySetupAttempt, failure error) (KeySetupResult, error) {
	code := connectorcore.ErrorCodeOf(failure)
	if !code.Valid() {
		code = connectorcore.ErrorCodeInternal
	}
	failed, persistErr := service.repository.CompleteSetupFailed(ctx, attempt.OrganizationID, attempt.OwnerAccountID, attempt.ID, code)
	if persistErr != nil {
		return KeySetupResult{}, persistErr
	}
	retryable := retryableError(failure) || retryableCode(code)
	return KeySetupResult{Attempt: failed, Retryable: retryable}, connectorcore.NewError(code, retryable, nil)
}

func (service *Service) available() error {
	if service == nil || service.catalog == nil || service.repository == nil || service.verifier == nil || service.sealer == nil || service.hasher == nil || service.now == nil || service.newID == nil || service.setupTTL <= 0 {
		return connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	return nil
}

func exactKeySchema(fields []catalog.KeyField, values map[string]string) bool {
	if len(fields) == 0 || len(values) > len(fields) {
		return false
	}
	allowed := make(map[string]catalog.KeyField, len(fields))
	for _, field := range fields {
		allowed[field.Name] = field
		value, exists := values[field.Name]
		if field.Required && (!exists || strings.TrimSpace(value) == "") {
			return false
		}
	}
	for name, value := range values {
		field, ok := allowed[name]
		if !ok || !field.Secret || strings.TrimSpace(value) == "" || len(value) > credentialValueMaxBytes {
			return false
		}
	}
	return true
}

func exactCredentialSchema(definition catalog.Definition, values map[string]string) bool {
	if definition.UsesCredentials() {
		return catalog.ValidateCredentialValues(definition, values) == nil
	}
	return exactKeySchema(definition.KeyFields, values)
}

func verifiedOperations(definition catalog.Definition, values []string) ([]connectorcore.Identifier, map[connectorcore.Identifier]string, error) {
	allowed := make(map[string]struct{}, len(definition.Operations))
	for _, operation := range definition.Operations {
		allowed[operation.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(values))
	usable := make([]connectorcore.Identifier, 0, len(values))
	for _, value := range values {
		if _, ok := allowed[value]; !ok {
			return nil, nil, connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, nil, connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
		}
		identifier, err := connectorcore.NormalizeIdentifier(value)
		if err != nil || identifier.String() != value {
			return nil, nil, connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
		}
		seen[value] = struct{}{}
		usable = append(usable, identifier)
	}
	if len(usable) == 0 {
		return nil, nil, connectorcore.NewError(connectorcore.ErrorCodeInsufficientKey, false, nil)
	}
	unavailable := make(map[connectorcore.Identifier]string, len(definition.Operations)-len(usable))
	for _, operation := range definition.Operations {
		if _, ok := seen[operation.ID]; !ok {
			unavailable[connectorcore.Identifier(operation.ID)] = "connector_capability_unavailable"
		}
	}
	return usable, unavailable, nil
}

func verifiedIdentity(result CandidateVerificationResult) (string, string, error) {
	accountID := strings.TrimSpace(result.ExternalAccountID)
	displayName := strings.TrimSpace(result.ExternalDisplayName)
	if len(accountID) > 255 || len(displayName) > 255 {
		return "", "", connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	return accountID, displayName, nil
}

func defaultConnectionName(connectorName, externalAccountID, externalDisplayName string) string {
	for _, candidate := range []string{externalDisplayName, externalAccountID, strings.TrimSpace(connectorName)} {
		if candidate != "" {
			return candidate
		}
	}
	return "Connector"
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	copyValue := value
	return &copyValue
}

func timePointer(value time.Time) *time.Time {
	copyValue := value
	return &copyValue
}

func cloneTarget(target *connectorcore.TargetRef) *connectorcore.TargetRef {
	if target == nil {
		return nil
	}
	copyValue := *target
	return &copyValue
}

func publicConnectionPointer(connection connectorcore.PersonalConnection) *connectorcore.PersonalConnection {
	connection.EncryptedCredentials = ""
	return &connection
}

func retryableCode(code connectorcore.ErrorCode) bool {
	return code == connectorcore.ErrorCodeProviderUnavailable || code == connectorcore.ErrorCodeInternal
}

func retryableError(err error) bool {
	var connectorError *connectorcore.Error
	return errors.As(err, &connectorError) && connectorError != nil && connectorError.Retryable
}
