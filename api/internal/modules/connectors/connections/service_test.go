package connections

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

const candidateKeySentinel = "candidate-key-TEST-SENTINEL"

func TestServiceRejectsNonExactKeySchemaAndDestroysCandidateValues(t *testing.T) {
	service, verifier, sealer, _ := newKeyLifecycleTestService(t)
	for name, values := range map[string]map[string]string{
		"missing required": {},
		"blank required":   {"token": "   "},
		"unknown field":    {"token": candidateKeySentinel, "scope": "repo"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := service.StartKeySetup(context.Background(), KeySetupCommand{
				OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectorID: "github",
				IdempotencyKey: uuid.NewString(), Values: values,
			})
			if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeInvalidInput) {
				t.Fatalf("StartKeySetup() error = %v, want invalid input", err)
			}
			if len(values) != 0 {
				t.Fatalf("candidate values retained after rejection: %#v", values)
			}
		})
	}
	if verifier.calls != 0 || sealer.calls != 0 {
		t.Fatalf("invalid schema reached verifier/sealer: %d/%d", verifier.calls, sealer.calls)
	}
}

func TestServiceOneSubmissionCreatesReadyPersonalConnectionWithoutRequiredName(t *testing.T) {
	service, verifier, sealer, repository := newKeyLifecycleTestService(t)
	organizationID, ownerID := uuid.New(), uuid.New()
	values := map[string]string{"token": candidateKeySentinel}

	result, err := service.StartKeySetup(context.Background(), KeySetupCommand{
		OrganizationID: organizationID, OwnerAccountID: ownerID, ConnectorID: "github",
		IdempotencyKey: "key-lifecycle-ready-0001", Values: values,
	})
	if err != nil {
		t.Fatalf("StartKeySetup() error = %v", err)
	}
	if result.Attempt.State != connectorcore.SetupStateReady || result.Connection == nil {
		t.Fatalf("StartKeySetup() = %#v", result)
	}
	if result.Connection.Name != "octocat" || result.Connection.State != connectorcore.ReadinessReady {
		t.Fatalf("ready connection = %#v", result.Connection)
	}
	if result.Connection.EncryptedCredentials != "" {
		t.Fatal("service result exposed encrypted credentials")
	}
	if len(result.Connection.UsableOperationIDs) != 1 || result.Connection.UsableOperationIDs[0] != "github.list_repositories" {
		t.Fatalf("usable operations = %#v", result.Connection.UsableOperationIDs)
	}
	if result.Connection.UnavailableOperations["github.create_issue"] != "connector_capability_unavailable" {
		t.Fatalf("unavailable operations = %#v", result.Connection.UnavailableOperations)
	}
	if verifier.calls != 1 || sealer.calls != 1 {
		t.Fatalf("provider/sealer calls = %d/%d, want 1/1", verifier.calls, sealer.calls)
	}
	if verifier.credentials["token"] != candidateKeySentinel || sealer.values["token"] != candidateKeySentinel {
		t.Fatal("candidate did not reach the bounded verifier and sealer")
	}
	if len(values) != 0 {
		t.Fatalf("candidate values retained after success: %#v", values)
	}

	stored, err := repository.GetConnection(context.Background(), organizationID, ownerID, result.Connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.EncryptedCredentials != "sealed-envelope-without-plaintext" {
		t.Fatalf("stored envelope = %q", stored.EncryptedCredentials)
	}
	assertLifecycleJSONHasNoCandidate(t, result)
}

func TestServiceIdempotentReplayReturnsSameConnectionWithoutSecondProviderCall(t *testing.T) {
	service, verifier, sealer, _ := newKeyLifecycleTestService(t)
	organizationID, ownerID := uuid.New(), uuid.New()
	command := func(value string) KeySetupCommand {
		return KeySetupCommand{
			OrganizationID: organizationID, OwnerAccountID: ownerID, ConnectorID: "github",
			IdempotencyKey: "key-lifecycle-idempotent-0001", Values: map[string]string{"token": value},
		}
	}
	first, err := service.StartKeySetup(context.Background(), command(candidateKeySentinel))
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.StartKeySetup(context.Background(), command(candidateKeySentinel))
	if err != nil {
		t.Fatal(err)
	}
	if first.Connection == nil || second.Connection == nil || first.Connection.ID != second.Connection.ID || first.Attempt.ID != second.Attempt.ID {
		t.Fatalf("idempotent results differ: %#v / %#v", first, second)
	}
	if verifier.calls != 1 || sealer.calls != 1 {
		t.Fatalf("idempotent replay calls = %d/%d, want 1/1", verifier.calls, sealer.calls)
	}

	changed := command("different-candidate-key")
	_, err = service.StartKeySetup(context.Background(), changed)
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConflict) {
		t.Fatalf("changed frozen request error = %v, want conflict", err)
	}
	if len(changed.Values) != 0 || verifier.calls != 1 || sealer.calls != 1 {
		t.Fatalf("conflict retained or reprocessed candidate: values=%#v calls=%d/%d", changed.Values, verifier.calls, sealer.calls)
	}
}

func TestServiceDistinctSetupAttemptsKeepMultipleConnectionsForSameExternalAccount(t *testing.T) {
	service, verifier, sealer, repository := newKeyLifecycleTestService(t)
	organizationID, ownerID := uuid.New(), uuid.New()
	create := func(idempotencyKey, candidate string) KeySetupResult {
		t.Helper()
		result, err := service.StartKeySetup(context.Background(), KeySetupCommand{
			OrganizationID: organizationID, OwnerAccountID: ownerID, ConnectorID: "github",
			IdempotencyKey: idempotencyKey, Values: map[string]string{"token": candidate},
		})
		if err != nil || result.Connection == nil {
			t.Fatalf("StartKeySetup() = %#v, %v", result, err)
		}
		return result
	}
	first := create("multiple-account-key-first-0001", candidateKeySentinel)
	second := create("multiple-account-key-second-0001", "second-test-candidate-key")
	if first.Connection.ID == second.Connection.ID || first.Attempt.ID == second.Attempt.ID ||
		first.Connection.ExternalAccountID == nil || second.Connection.ExternalAccountID == nil ||
		*first.Connection.ExternalAccountID != *second.Connection.ExternalAccountID ||
		first.Connection.ProviderID != second.Connection.ProviderID || first.Connection.OwnerAccountID != second.Connection.OwnerAccountID {
		t.Fatalf("same-account setup was deduplicated: %#v / %#v", first, second)
	}
	replay := create("multiple-account-key-second-0001", "second-test-candidate-key")
	if replay.Connection.ID != second.Connection.ID || replay.Attempt.ID != second.Attempt.ID || verifier.calls != 2 || sealer.calls != 2 {
		t.Fatalf("same-attempt replay changed multiplicity: %#v; verifier/sealer=%d/%d", replay, verifier.calls, sealer.calls)
	}
	items, err := service.ListConnections(context.Background(), organizationID, ownerID, ConnectionListFilter{ConnectorID: "github"})
	if err != nil || len(items) != 2 || len(repository.connections) != 2 {
		t.Fatalf("same-account connection list = %#v, %v", items, err)
	}
	for _, item := range items {
		if item.EncryptedCredentials != "" || item.CredentialVersion != 1 {
			t.Fatalf("public connection projection = %#v", item)
		}
	}
}

func TestServiceFailedVerificationCreatesNoConnectionAndRetainsNoCandidate(t *testing.T) {
	service, verifier, _, repository := newKeyLifecycleTestService(t)
	verifier.err = connectorcore.NewError(connectorcore.ErrorCodeInvalidKey, false, nil)
	organizationID, ownerID := uuid.New(), uuid.New()
	values := map[string]string{"token": candidateKeySentinel}

	result, err := service.StartKeySetup(context.Background(), KeySetupCommand{
		OrganizationID: organizationID, OwnerAccountID: ownerID, ConnectorID: "github",
		IdempotencyKey: "key-lifecycle-invalid-0001", Values: values,
	})
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeInvalidKey) {
		t.Fatalf("StartKeySetup() error = %v, want invalid key", err)
	}
	if strings.Contains(err.Error(), candidateKeySentinel) {
		t.Fatal("failure exposed submitted Key")
	}
	if result.Attempt.State != connectorcore.SetupStateFailed || result.Attempt.ErrorCode != connectorcore.ErrorCodeInvalidKey || result.Connection != nil || result.Retryable {
		t.Fatalf("failed result = %#v", result)
	}
	items, listErr := repository.ListConnections(context.Background(), organizationID, ownerID, ConnectionListFilter{})
	if listErr != nil || len(items) != 0 {
		t.Fatalf("connections after failed setup = %#v, %v", items, listErr)
	}
	if len(values) != 0 {
		t.Fatalf("candidate retained after failure: %#v", values)
	}
	assertLifecycleJSONHasNoCandidate(t, result)
}

func TestServiceSeparatesProviderUnavailableFromInvalidKey(t *testing.T) {
	service, verifier, _, _ := newKeyLifecycleTestService(t)
	verifier.err = connectorcore.NewError(connectorcore.ErrorCodeProviderUnavailable, true, nil)
	result, err := service.StartKeySetup(context.Background(), KeySetupCommand{
		OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectorID: "github",
		IdempotencyKey: "key-lifecycle-upstream-0001", Values: map[string]string{"token": candidateKeySentinel},
	})
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeProviderUnavailable) || !result.Retryable {
		t.Fatalf("provider-unavailable result/error = %#v, %v", result, err)
	}
	if result.Attempt.State != connectorcore.SetupStateFailed || result.Attempt.ErrorCode != connectorcore.ErrorCodeProviderUnavailable {
		t.Fatalf("provider-unavailable attempt = %#v", result.Attempt)
	}
}

func TestServiceRequiresAtLeastOneIndependentlyVerifiedCapability(t *testing.T) {
	service, verifier, sealer, repository := newKeyLifecycleTestService(t)
	verifier.result.UsableOperationIDs = nil
	organizationID, ownerID := uuid.New(), uuid.New()
	result, err := service.StartKeySetup(context.Background(), KeySetupCommand{
		OrganizationID: organizationID, OwnerAccountID: ownerID, ConnectorID: "github",
		IdempotencyKey: "key-lifecycle-capability-0001", Values: map[string]string{"token": candidateKeySentinel},
	})
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeInsufficientKey) {
		t.Fatalf("zero-capability error = %v", err)
	}
	if result.Attempt.State != connectorcore.SetupStateFailed || sealer.calls != 0 {
		t.Fatalf("zero-capability result/sealer = %#v / %d", result, sealer.calls)
	}
	items, listErr := repository.ListConnections(context.Background(), organizationID, ownerID, ConnectionListFilter{})
	if listErr != nil || len(items) != 0 {
		t.Fatalf("connections after insufficient key = %#v, %v", items, listErr)
	}
}

func TestServiceGetAndCancelSetupAreOwnerScoped(t *testing.T) {
	service, verifier, _, _ := newKeyLifecycleTestService(t)
	verifier.err = connectorcore.NewError(connectorcore.ErrorCodeInvalidKey, false, nil)
	organizationID, ownerID := uuid.New(), uuid.New()
	failed, _ := service.StartKeySetup(context.Background(), KeySetupCommand{
		OrganizationID: organizationID, OwnerAccountID: ownerID, ConnectorID: "github",
		IdempotencyKey: "key-lifecycle-owner-scope-1", Values: map[string]string{"token": candidateKeySentinel},
	})
	got, err := service.GetKeySetup(context.Background(), organizationID, ownerID, failed.Attempt.ID)
	if err != nil || got.Attempt.State != connectorcore.SetupStateFailed {
		t.Fatalf("GetKeySetup() = %#v, %v", got, err)
	}
	if _, err := service.GetKeySetup(context.Background(), organizationID, uuid.New(), failed.Attempt.ID); !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeNotFound) {
		t.Fatalf("cross-owner GetKeySetup() error = %v", err)
	}
}

func newKeyLifecycleTestService(t *testing.T) (*Service, *recordingCandidateVerifier, *recordingCredentialSealer, *memoryRepository) {
	t.Helper()
	repository := newMemoryRepository()
	definition := catalog.Definition{
		ID: "github", PluginID: "zgi.github", PluginVersion: "1.0.0",
		SourceDialect: catalog.SourceDialectAgentPluginsV1, SourceSchemaVersion: catalog.SourceSchemaVersion1,
		ProviderID: "github", DriverID: "github-rest", AuthMethodID: "personal_access_token",
		Name: "GitHub", Description: "GitHub repositories and issues", Classification: catalog.ClassificationConnector,
		CapabilityTypes: []catalog.CapabilityType{catalog.CapabilityTypeAction, catalog.CapabilityTypeData},
		SetupMethod:     catalog.SetupMethodKey, Ownership: catalog.OwnershipPersonal, Surfaces: catalog.FirstReleaseSurfaces(),
		KeyFields: []catalog.KeyField{{Name: "token", Label: "Personal access token", Required: true, Secret: true}},
		Operations: []catalog.Operation{
			{ID: "github.list_repositories", ToolName: "github.list_repositories", Name: "List repositories"},
			{ID: "github.create_issue", ToolName: "github.create_issue", Name: "Create issue"},
		},
	}
	definitionCatalog, err := catalog.New(definition)
	if err != nil {
		t.Fatal(err)
	}
	verifier := &recordingCandidateVerifier{result: CandidateVerificationResult{
		ExternalAccountID: "583231", ExternalDisplayName: "octocat",
		UsableOperationIDs: []string{"github.list_repositories"}, ProviderRequestID: "provider-request-1",
	}}
	sealer := &recordingCredentialSealer{envelope: "sealed-envelope-without-plaintext"}
	hasher, err := NewCommandHasher(strings.Repeat("m", 32))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(definitionCatalog, repository, verifier, sealer, hasher)
	if err != nil {
		t.Fatal(err)
	}
	fixedNow := time.Date(2026, time.September, 1, 9, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedNow }
	return service, verifier, sealer, repository
}

type recordingCandidateVerifier struct {
	calls       int
	credentials map[string]string
	result      CandidateVerificationResult
	err         error
}

func (verifier *recordingCandidateVerifier) Verify(_ context.Context, request CandidateVerificationRequest) (CandidateVerificationResult, error) {
	verifier.calls++
	verifier.credentials = cloneCredentialMap(request.Credentials)
	return verifier.result, verifier.err
}

type recordingCredentialSealer struct {
	calls    int
	values   map[string]string
	envelope string
	err      error
}

func (sealer *recordingCredentialSealer) Seal(request CredentialSealRequest) (string, error) {
	sealer.calls++
	sealer.values = cloneCredentialMap(request.Values)
	return sealer.envelope, sealer.err
}

func cloneCredentialMap(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func assertLifecycleJSONHasNoCandidate(t *testing.T, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{candidateKeySentinel, "sealed-envelope-without-plaintext", "IdempotencyKeyHash", "RequestFingerprint"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("lifecycle JSON exposed %q: %s", forbidden, text)
		}
	}
}

type memoryRepository struct {
	connections map[uuid.UUID]connectorcore.PersonalConnection
	attempts    map[uuid.UUID]connectorcore.KeySetupAttempt
	claims      map[string]uuid.UUID
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		connections: map[uuid.UUID]connectorcore.PersonalConnection{},
		attempts:    map[uuid.UUID]connectorcore.KeySetupAttempt{},
		claims:      map[string]uuid.UUID{},
	}
}

func (repository *memoryRepository) CreateConnection(_ context.Context, connection connectorcore.PersonalConnection) error {
	if _, exists := repository.connections[connection.ID]; exists {
		return connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	repository.connections[connection.ID] = connection
	return nil
}

func (repository *memoryRepository) GetConnection(_ context.Context, organizationID, ownerID, connectionID uuid.UUID) (connectorcore.PersonalConnection, error) {
	connection, ok := repository.connections[connectionID]
	if !ok || connection.OrganizationID != organizationID || connection.OwnerAccountID != ownerID {
		return connectorcore.PersonalConnection{}, connectorcore.NewError(connectorcore.ErrorCodeNotFound, false, nil)
	}
	return connection, nil
}

func (repository *memoryRepository) ListConnections(_ context.Context, organizationID, ownerID uuid.UUID, filter ConnectionListFilter) ([]connectorcore.PersonalConnection, error) {
	result := make([]connectorcore.PersonalConnection, 0)
	for _, connection := range repository.connections {
		if connection.OrganizationID != organizationID || connection.OwnerAccountID != ownerID {
			continue
		}
		if filter.ConnectorID != "" && connection.ConnectorID != filter.ConnectorID {
			continue
		}
		if len(filter.States) > 0 && !containsReadiness(filter.States, connection.State) {
			continue
		}
		result = append(result, connection)
	}
	return result, nil
}

func (repository *memoryRepository) UpdateConnection(_ context.Context, organizationID, ownerID uuid.UUID, connection connectorcore.PersonalConnection, expectedRevision int) (connectorcore.PersonalConnection, error) {
	stored, err := repository.GetConnection(context.Background(), organizationID, ownerID, connection.ID)
	if err != nil {
		return connectorcore.PersonalConnection{}, err
	}
	if stored.Revision != expectedRevision {
		return connectorcore.PersonalConnection{}, connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	connection.Revision = expectedRevision + 1
	repository.connections[connection.ID] = connection
	return connection, nil
}

func (repository *memoryRepository) ClaimSetupAttempt(_ context.Context, attempt connectorcore.KeySetupAttempt) (connectorcore.KeySetupAttempt, bool, error) {
	claimKey := attempt.OrganizationID.String() + "/" + attempt.OwnerAccountID.String() + "/" + attempt.IdempotencyKeyHash
	if attemptID, exists := repository.claims[claimKey]; exists {
		claimed := repository.attempts[attemptID]
		if claimed.RequestFingerprint != attempt.RequestFingerprint {
			return connectorcore.KeySetupAttempt{}, false, connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
		}
		return claimed, false, nil
	}
	repository.claims[claimKey] = attempt.ID
	repository.attempts[attempt.ID] = attempt
	return attempt, true, nil
}

func (repository *memoryRepository) GetSetupAttempt(_ context.Context, organizationID, ownerID, attemptID uuid.UUID) (connectorcore.KeySetupAttempt, error) {
	attempt, ok := repository.attempts[attemptID]
	if !ok || attempt.OrganizationID != organizationID || attempt.OwnerAccountID != ownerID {
		return connectorcore.KeySetupAttempt{}, connectorcore.NewError(connectorcore.ErrorCodeNotFound, false, nil)
	}
	return attempt, nil
}

func (repository *memoryRepository) ListSetupAttempts(_ context.Context, organizationID, ownerID uuid.UUID, filter SetupAttemptListFilter) ([]connectorcore.KeySetupAttempt, error) {
	result := make([]connectorcore.KeySetupAttempt, 0)
	for _, attempt := range repository.attempts {
		if attempt.OrganizationID != organizationID || attempt.OwnerAccountID != ownerID {
			continue
		}
		if filter.ConnectorID != "" && attempt.ConnectorID != filter.ConnectorID {
			continue
		}
		if len(filter.States) > 0 && !containsSetupState(filter.States, attempt.State) {
			continue
		}
		result = append(result, attempt)
	}
	return result, nil
}

func (repository *memoryRepository) CompleteSetupReady(_ context.Context, organizationID, ownerID, attemptID uuid.UUID, connection connectorcore.PersonalConnection) (connectorcore.KeySetupAttempt, error) {
	attempt, err := repository.GetSetupAttempt(context.Background(), organizationID, ownerID, attemptID)
	if err != nil {
		return connectorcore.KeySetupAttempt{}, err
	}
	if attempt.State != connectorcore.SetupStateVerifying {
		return connectorcore.KeySetupAttempt{}, connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	if _, exists := repository.connections[connection.ID]; exists {
		return connectorcore.KeySetupAttempt{}, connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	repository.connections[connection.ID] = connection
	attempt.State = connectorcore.SetupStateReady
	attempt.ResultConnectionID = connection.ID
	attempt.UpdatedAt = connection.UpdatedAt
	repository.attempts[attempt.ID] = attempt
	return attempt, nil
}

func (repository *memoryRepository) CompleteSetupFailed(_ context.Context, organizationID, ownerID, attemptID uuid.UUID, code connectorcore.ErrorCode) (connectorcore.KeySetupAttempt, error) {
	attempt, err := repository.GetSetupAttempt(context.Background(), organizationID, ownerID, attemptID)
	if err != nil {
		return connectorcore.KeySetupAttempt{}, err
	}
	if attempt.State != connectorcore.SetupStateVerifying {
		return connectorcore.KeySetupAttempt{}, connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	attempt.State = connectorcore.SetupStateFailed
	attempt.ErrorCode = code
	repository.attempts[attempt.ID] = attempt
	return attempt, nil
}

func (repository *memoryRepository) CancelSetupAttempt(_ context.Context, organizationID, ownerID, attemptID uuid.UUID) (connectorcore.KeySetupAttempt, error) {
	attempt, err := repository.GetSetupAttempt(context.Background(), organizationID, ownerID, attemptID)
	if err != nil {
		return connectorcore.KeySetupAttempt{}, err
	}
	if attempt.State != connectorcore.SetupStateVerifying {
		return connectorcore.KeySetupAttempt{}, connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	attempt.State = connectorcore.SetupStateCancelled
	repository.attempts[attempt.ID] = attempt
	return attempt, nil
}

func containsReadiness(values []connectorcore.Readiness, candidate connectorcore.Readiness) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func containsSetupState(values []connectorcore.SetupState, candidate connectorcore.SetupState) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
