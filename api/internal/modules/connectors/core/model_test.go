package core

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestConnectorIdentifierNormalizationAndValidation(t *testing.T) {
	id, err := NormalizeIdentifier("  GitHub.Search-V1  ")
	if err != nil {
		t.Fatalf("NormalizeIdentifier() error = %v", err)
	}
	if got, want := id.String(), "github.search-v1"; got != want {
		t.Fatalf("normalized identifier = %q, want %q", got, want)
	}

	maxLength := "a" + strings.Repeat("b", IdentifierMaxLength-1)
	if _, err := NormalizeIdentifier(maxLength); err != nil {
		t.Fatalf("maximum-length identifier rejected: %v", err)
	}
	for _, invalid := range []string{"", " ", "-github", "github/search", maxLength + "x"} {
		if _, err := NormalizeIdentifier(invalid); err == nil {
			t.Fatalf("NormalizeIdentifier(%q) unexpectedly succeeded", invalid)
		}
	}
}

func TestConnectorEnumInvariants(t *testing.T) {
	valid := []interface{ Valid() bool }{
		ClassificationConnector,
		ClassificationDataSource,
		DefinitionStatusActive,
		DefinitionStatusDisabled,
		DefinitionStatusUnavailable,
		ReadinessReady,
		ReadinessReconnectRequired,
		ReadinessUnavailable,
		SetupStateVerifying,
		SetupStateReady,
		SetupStateFailed,
		SetupStateCancelled,
		TargetTypeChat,
		TargetTypeAgent,
		TargetTypeWorkflow,
		BindingStatusActive,
		BindingStatusDisabled,
		BindingStatusNeedsReplacement,
		DecisionStatusSubmitted,
		DecisionStatusDenied,
		DecisionStatusBlocked,
		DecisionStatusCancelled,
		DecisionStatusFailed,
		DecisionStatusSuccessful,
		DecisionStatusUnknown,
		OutcomeStatusSuccessful,
		OutcomeStatusFailed,
		OutcomeStatusUnknown,
	}
	for i, value := range valid {
		if !value.Valid() {
			t.Fatalf("valid enum at index %d was rejected", i)
		}
	}

	invalid := []interface{ Valid() bool }{
		Classification("shared"),
		DefinitionStatus("pending"),
		Readiness("active"),
		SetupState("pending"),
		TargetType("organization"),
		BindingStatus("deleted"),
		DecisionStatus("approved"),
		OutcomeStatus("cancelled"),
	}
	for i, value := range invalid {
		if value.Valid() {
			t.Fatalf("invalid enum at index %d was accepted", i)
		}
	}
}

func TestTargetRefAndBindingInvariants(t *testing.T) {
	for _, target := range []TargetRef{
		{Type: TargetTypeChat, ID: "conversation-1"},
		{Type: TargetTypeAgent, ID: "agent-draft-1"},
		{Type: TargetTypeWorkflow, ID: "workflow-1", BindingInstanceID: "node-7"},
	} {
		if err := target.Validate(); err != nil {
			t.Fatalf("TargetRef.Validate(%+v) error = %v", target, err)
		}
	}

	for _, target := range []TargetRef{
		{Type: TargetTypeWorkflow, ID: "workflow-1"},
		{Type: TargetTypeChat, ID: "conversation-1", BindingInstanceID: "node-7"},
		{Type: TargetType("shared"), ID: "target-1"},
		{Type: TargetTypeAgent},
	} {
		if err := target.Validate(); err == nil {
			t.Fatalf("TargetRef.Validate(%+v) unexpectedly succeeded", target)
		}
	}

	owner := uuid.New()
	binding := TargetBinding{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		OwnerAccountID: owner,
		ConnectionID:   uuid.New(),
		ConnectorID:    Identifier("github"),
		Target:         TargetRef{Type: TargetTypeAgent, ID: "agent-draft-1"},
		Status:         BindingStatusActive,

		Revision: 1,
	}
	if err := binding.Validate(); err != nil {
		t.Fatalf("TargetBinding.Validate() error = %v", err)
	}
	binding.Revision = 0
	if err := binding.Validate(); err == nil {
		t.Fatal("TargetBinding.Validate() accepted a non-positive revision")
	}
}

func TestPersonalConnectionRequiresOwnerEnvelopeAndPositiveVersions(t *testing.T) {
	credentialSentinel := "connector-key-sentinel-model-6f779c"
	connection := PersonalConnection{
		ID:                    uuid.New(),
		OrganizationID:        uuid.New(),
		OwnerAccountID:        uuid.New(),
		ConnectorID:           Identifier("github"),
		ProviderID:            Identifier("github"),
		DriverID:              Identifier("github-pat"),
		AuthMethodID:          Identifier("pat-personal"),
		Name:                  "GitHub",
		EncryptedCredentials:  credentialSentinel,
		CredentialVersion:     1,
		State:                 ReadinessReady,
		UsableOperationIDs:    []Identifier{Identifier("github.search_repositories")},
		UnavailableOperations: map[Identifier]string{},
		Revision:              1,
		LastVerifiedAt:        time.Now().UTC(),
	}
	if err := connection.Validate(); err != nil {
		t.Fatalf("PersonalConnection.Validate() error = %v", err)
	}

	encoded, err := json.Marshal(connection)
	if err != nil {
		t.Fatalf("json.Marshal(connection) error = %v", err)
	}
	if strings.Contains(string(encoded), credentialSentinel) || strings.Contains(string(encoded), "encrypted_credentials") {
		t.Fatalf("serialized connection exposed credential envelope: %s", encoded)
	}

	connection.CredentialVersion = 0
	if err := connection.Validate(); err == nil {
		t.Fatal("PersonalConnection.Validate() accepted a non-positive credential version")
	}
	connection.CredentialVersion = 1
	connection.OwnerAccountID = uuid.Nil
	if err := connection.Validate(); err == nil {
		t.Fatal("PersonalConnection.Validate() accepted a missing owner")
	}
}

func TestKeySetupAttemptStateInvariants(t *testing.T) {
	base := KeySetupAttempt{
		ID:                 uuid.New(),
		OrganizationID:     uuid.New(),
		OwnerAccountID:     uuid.New(),
		ConnectorID:        Identifier("github"),
		IdempotencyKeyHash: strings.Repeat("a", 64),
		RequestFingerprint: strings.Repeat("b", 64),
		State:              SetupStateVerifying,
		ExpiresAt:          time.Now().UTC().Add(time.Minute),
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("verifying setup attempt error = %v", err)
	}
	workflow := base
	workflow.Target = &TargetRef{Type: TargetTypeWorkflow, ID: "workflow-1", BindingInstanceID: "node-4"}
	if err := workflow.Validate(); err != nil {
		t.Fatalf("workflow setup target error = %v", err)
	}
	missingFingerprint := base
	missingFingerprint.RequestFingerprint = ""
	if err := missingFingerprint.Validate(); err == nil {
		t.Fatal("setup attempt accepted without a request fingerprint")
	}

	ready := base
	ready.State = SetupStateReady
	ready.ResultConnectionID = uuid.New()
	if err := ready.Validate(); err != nil {
		t.Fatalf("ready setup attempt error = %v", err)
	}
	ready.ResultConnectionID = uuid.Nil
	if err := ready.Validate(); err == nil {
		t.Fatal("ready setup attempt accepted without a result Connection")
	}

	failed := base
	failed.State = SetupStateFailed
	failed.ErrorCode = "connector_key_invalid"
	if err := failed.Validate(); err != nil {
		t.Fatalf("failed setup attempt error = %v", err)
	}
	failed.ErrorCode = ""
	if err := failed.Validate(); err == nil {
		t.Fatal("failed setup attempt accepted without a stable error code")
	}
}

func TestSubmittedDecisionAndOutcomeRequireCompleteOwnerIdentity(t *testing.T) {
	owner := uuid.New()
	decision := LifecycleDecision{
		ID:                 uuid.New(),
		OrganizationID:     uuid.New(),
		OwnerAccountID:     owner,
		ActingPrincipalID:  owner,
		ConnectorID:        Identifier("github"),
		ConnectionID:       uuid.New(),
		Target:             &TargetRef{Type: TargetTypeChat, ID: "conversation-1"},
		OperationID:        Identifier("github.create_issue"),
		LogicalOperationID: "logical-operation-1",
		Decision:           "dispatch",
		Status:             DecisionStatusSubmitted,
		CorrelationID:      "correlation-1",
		Details:            map[string]any{},
	}
	if err := decision.Validate(); err != nil {
		t.Fatalf("LifecycleDecision.Validate() error = %v", err)
	}
	decision.ActingPrincipalID = uuid.New()
	if err := decision.Validate(); err == nil {
		t.Fatal("LifecycleDecision.Validate() accepted a non-owner Acting Principal")
	}
	decision.ActingPrincipalID = owner

	outcome := OperationOutcome{
		ID:                 uuid.New(),
		DecisionID:         decision.ID,
		OrganizationID:     decision.OrganizationID,
		OwnerAccountID:     owner,
		ActingPrincipalID:  owner,
		ConnectorID:        decision.ConnectorID,
		ConnectionID:       decision.ConnectionID,
		Target:             *decision.Target,
		OperationID:        decision.OperationID,
		LogicalOperationID: decision.LogicalOperationID,
		Status:             OutcomeStatusUnknown,
	}
	if err := outcome.Validate(); err != nil {
		t.Fatalf("OperationOutcome.Validate() error = %v", err)
	}
	outcome.Status = OutcomeStatus("cancelled")
	if err := outcome.Validate(); err == nil {
		t.Fatal("OperationOutcome.Validate() accepted a non-terminal outcome")
	}
}
