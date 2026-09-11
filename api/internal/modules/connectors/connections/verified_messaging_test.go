package connections

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func messagingCheckpointFixture(t *testing.T) (*Service, *recordingCandidateVerifier, CandidateVerificationResult) {
	t.Helper()
	service, liveVerifier, _, _ := newKeyLifecycleTestService(t)
	definition, _ := catalog.MessagingDefinition("wecom")
	var err error
	service.catalog, err = catalog.New(definition)
	if err != nil {
		t.Fatal(err)
	}
	return service, liveVerifier, CandidateVerificationResult{ExternalAccountID: "wecom:bot:bot-original", UsableOperationIDs: []string{"wecom.identity.get"}, Evidence: &core.ProviderEvidence{Messaging: &core.MessagingEvidence{Version: 1, Provider: "wecom", Subject: "bot", Identity: "wecom:bot:bot-original", CredentialVerified: true, Operations: []core.OperationEvidence{{ID: "wecom.identity.get", Availability: "verified"}}}}}
}

func TestVerifiedMessagingCommitUsesFixedIDAndNeverCallsProvider(t *testing.T) {
	service, liveVerifier, verified := messagingCheckpointFixture(t)
	organizationID, ownerID, candidateID := uuid.New(), uuid.New(), uuid.New()
	command := func() KeySetupCommand {
		return KeySetupCommand{OrganizationID: organizationID, OwnerAccountID: ownerID, ConnectorID: "wecom", IdempotencyKey: "checkpoint-promote-0001", Values: map[string]string{"bot_id": "bot-original", "secret": candidateKeySentinel}}
	}
	firstCommand := command()
	first, err := service.CommitVerifiedMessaging(context.Background(), firstCommand, candidateID, verified)
	if err != nil || first.Connection == nil || first.Connection.ID != candidateID {
		t.Fatalf("commit = %#v, %v", first, err)
	}
	second, err := service.CommitVerifiedMessaging(context.Background(), command(), candidateID, verified)
	if err != nil || second.Connection == nil || second.Connection.ID != candidateID {
		t.Fatalf("replay = %#v, %v", second, err)
	}
	if liveVerifier.calls != 0 {
		t.Fatalf("commit issued %d network probes", liveVerifier.calls)
	}
	if len(firstCommand.Values) != 0 {
		t.Fatal("candidate secret retained")
	}
}

func TestVerifiedMessagingCommitRejectsMismatchedEvidenceBeforePersistence(t *testing.T) {
	for _, mutation := range []string{"identity", "provider", "unverified", "denied", "missing_operation", "wrong_scope"} {
		t.Run(mutation, func(t *testing.T) {
			service, liveVerifier, verified := messagingCheckpointFixture(t)
			switch mutation {
			case "identity":
				verified.ExternalAccountID = "wecom:bot:other-bot"
			case "provider":
				verified.Evidence.Messaging.Provider = "other"
			case "unverified":
				verified.Evidence.Messaging.CredentialVerified = false
			case "denied":
				verified.Evidence.Messaging.Operations[0].Availability = "denied"
			case "missing_operation":
				verified.Evidence.Messaging.Operations = nil
			}
			ownerID := uuid.New()
			if mutation == "wrong_scope" {
				ownerID = uuid.Nil
			}
			command := KeySetupCommand{OrganizationID: uuid.New(), OwnerAccountID: ownerID, ConnectorID: "wecom", IdempotencyKey: "checkpoint-reject-0001", Values: map[string]string{"bot_id": "bot-original", "secret": candidateKeySentinel}}
			_, err := service.CommitVerifiedMessaging(context.Background(), command, uuid.New(), verified)
			if err == nil || len(service.repository.(*memoryRepository).connections) != 0 || liveVerifier.calls != 0 || len(command.Values) != 0 {
				t.Fatalf("unsafe commit accepted: %v", err)
			}
		})
	}
}

func TestVerifiedMessagingReconnectGuardsRevisionAndOriginalSubject(t *testing.T) {
	for _, scenario := range []string{"same", "revision_changed", "credential_changed", "wrong_subject"} {
		t.Run(scenario, func(t *testing.T) {
			service, repository, _, liveVerifier, _, _ := newMaintenanceFixture(t)
			definition, _ := catalog.MessagingDefinition("wecom")
			service.catalog, _ = catalog.New(definition)
			existing := maintenanceConnection(service.now(), uuid.New(), uuid.New())
			existing.ConnectorID = "wecom"
			existing.ProviderID = "wecom"
			existing.DriverID = core.Identifier(definition.DriverID)
			existing.AuthMethodID = core.Identifier(definition.AuthMethodID)
			existing.ExternalAccountID = optionalString("wecom:bot:bot-original")
			existing.UsableOperationIDs = []core.Identifier{"wecom.identity.get"}
			existing.Constraint = core.OwnerConstraint(existing.OwnerAccountID, existing.UsableOperationIDs)
			repository.connections[existing.ID] = existing
			_, _, verified := messagingCheckpointFixture(t)
			revision, credentialVersion := existing.Revision, existing.CredentialVersion
			values := map[string]string{"bot_id": "bot-original", "secret": candidateKeySentinel}
			switch scenario {
			case "revision_changed":
				revision++
			case "credential_changed":
				credentialVersion++
			case "wrong_subject":
				values["bot_id"] = "other-bot"
				verified.ExternalAccountID = "wecom:bot:other-bot"
				verified.Evidence.Messaging.Identity = "wecom:bot:other-bot"
			}
			result, err := service.CommitVerifiedMessagingReconnect(context.Background(), ReconnectCommand{OrganizationID: existing.OrganizationID, OwnerAccountID: existing.OwnerAccountID, ConnectionID: existing.ID, IdempotencyKey: "checkpoint-reconnect-0001", Values: values}, revision, credentialVersion, verified)
			if scenario == "same" {
				if err != nil || result.Connection == nil || !result.ReplacedExisting || result.Connection.CredentialVersion != existing.CredentialVersion+1 {
					t.Fatalf("reconnect=%#v, %v", result, err)
				}
			} else if !core.IsErrorCode(err, core.ErrorCodeConflict) || repository.connections[existing.ID].CredentialVersion != existing.CredentialVersion {
				t.Fatalf("stale/mismatched checkpoint accepted: %v", err)
			}
			if liveVerifier.calls != 0 || len(values) != 0 {
				t.Fatalf("provider calls=%d secret fields=%d", liveVerifier.calls, len(values))
			}
		})
	}
}
