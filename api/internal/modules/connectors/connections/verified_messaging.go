package connections

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// CommitVerifiedMessaging is the transaction-only half of messaging setup.
// The caller must authenticate and expire the persisted verification checkpoint
// and hold its flow generation lock. This method never contacts a provider.
func (service *Service) CommitVerifiedMessaging(ctx context.Context, command KeySetupCommand, candidateConnectionID uuid.UUID, verified CandidateVerificationResult) (KeySetupResult, error) {
	if err := service.available(); err != nil {
		core.DestroySecretValues(command.Values)
		return KeySetupResult{}, err
	}
	definition, _, err := service.validateCommand(command)
	if err != nil {
		core.DestroySecretValues(command.Values)
		return KeySetupResult{}, err
	}
	verifier, err := newMessagingCheckpointVerifier(definition, command.OrganizationID, command.OwnerAccountID, candidateConnectionID, command.Values, verified)
	if err != nil {
		core.DestroySecretValues(command.Values)
		return KeySetupResult{}, err
	}
	copy := *service
	copy.verifier = verifier
	allocatedAttempt := false
	copy.newID = func() uuid.UUID {
		if !allocatedAttempt {
			allocatedAttempt = true
			return service.newID()
		}
		return candidateConnectionID
	}
	result, err := copy.StartKeySetup(ctx, command)
	if err == nil && (result.Connection == nil || result.Connection.ID != candidateConnectionID) {
		return KeySetupResult{}, core.NewError(core.ErrorCodeConflict, false, nil)
	}
	return result, err
}

// CommitVerifiedMessagingReconnect consumes a checkpoint for the original
// subject. Revision checks prevent promotion after a concurrent rotation.
// The transaction repository must be scoped to the caller's flow transaction.
func (service *MaintenanceService) CommitVerifiedMessagingReconnect(ctx context.Context, command ReconnectCommand, expectedRevision, expectedCredentialVersion int, verified CandidateVerificationResult) (ReconnectResult, error) {
	if err := service.available(); err != nil {
		core.DestroySecretValues(command.Values)
		return ReconnectResult{}, err
	}
	existing, err := service.repository.GetConnection(ctx, command.OrganizationID, command.OwnerAccountID, command.ConnectionID)
	if err != nil {
		core.DestroySecretValues(command.Values)
		return ReconnectResult{}, err
	}
	if expectedRevision < 1 || expectedCredentialVersion < 1 || existing.Revision != expectedRevision || existing.CredentialVersion != expectedCredentialVersion || existing.DeletedAt != nil {
		core.DestroySecretValues(command.Values)
		return ReconnectResult{}, core.NewError(core.ErrorCodeConflict, false, nil)
	}
	definition, found := service.catalog.Get(existing.ConnectorID.String())
	if !found {
		core.DestroySecretValues(command.Values)
		return ReconnectResult{}, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
	}
	verifier, err := newMessagingCheckpointVerifier(definition, command.OrganizationID, command.OwnerAccountID, command.ConnectionID, command.Values, verified)
	if err != nil {
		core.DestroySecretValues(command.Values)
		return ReconnectResult{}, err
	}
	copy := *service
	copy.verifier = verifier
	return copy.Reconnect(ctx, command)
}

type messagingCheckpointVerifier struct {
	organizationID, ownerID, connectionID uuid.UUID
	definition                            catalog.Definition
	identity                              string
	result                                CandidateVerificationResult
}

func newMessagingCheckpointVerifier(definition catalog.Definition, organizationID, ownerID, connectionID uuid.UUID, values map[string]string, result CandidateVerificationResult) (*messagingCheckpointVerifier, error) {
	invalid := func() (*messagingCheckpointVerifier, error) {
		return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	if organizationID == uuid.Nil || ownerID == uuid.Nil || connectionID == uuid.Nil || !definition.IsMessagingCredentials() || catalog.Validate(definition) != nil || !exactCredentialSchema(definition, values) {
		return invalid()
	}
	identity := catalog.CredentialIdentity(definition, values)
	if identity == "" || result.ExternalAccountID != identity || result.Evidence == nil || result.Evidence.Messaging == nil {
		return invalid()
	}
	evidence := result.Evidence.Messaging
	if evidence.Version != 1 || evidence.Provider != definition.ProviderID || evidence.Identity != identity || !evidence.CredentialVerified {
		return invalid()
	}
	if _, _, err := verifiedOperations(definition, result.UsableOperationIDs); err != nil {
		return invalid()
	}
	observed := make(map[string]bool, len(evidence.Operations))
	for _, operation := range evidence.Operations {
		if operation.Availability != "verified" && operation.Availability != "conditional" && operation.Availability != "context_required" && operation.Availability != "denied" {
			return invalid()
		}
		if _, duplicate := observed[operation.ID]; duplicate {
			return invalid()
		}
		observed[operation.ID] = operation.Availability != "denied"
	}
	for _, operation := range result.UsableOperationIDs {
		if !observed[operation] {
			return invalid()
		}
	}
	result.Evidence = result.Evidence.Clone()
	result.UsableOperationIDs = append([]string(nil), result.UsableOperationIDs...)
	return &messagingCheckpointVerifier{organizationID: organizationID, ownerID: ownerID, connectionID: connectionID, definition: definition, identity: identity, result: result}, nil
}

func (verifier *messagingCheckpointVerifier) Verify(_ context.Context, request CandidateVerificationRequest) (CandidateVerificationResult, error) {
	if request.OrganizationID != verifier.organizationID || request.OwnerAccountID != verifier.ownerID || request.ConnectionID != verifier.connectionID || request.ConnectorID != verifier.definition.ID || request.DriverID != verifier.definition.DriverID || request.AuthMethodID != verifier.definition.AuthMethodID || catalog.CredentialIdentity(verifier.definition, request.Credentials) != verifier.identity {
		return CandidateVerificationResult{}, core.NewError(core.ErrorCodeConflict, false, nil)
	}
	return verifier.result, nil
}
