package integrations

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type capturingConnectionHealthRepository struct {
	observation ConnectionHealthObservation
}

func (repository *capturingConnectionHealthRepository) Record(_ context.Context, observation ConnectionHealthObservation) (ConnectionHealthEvent, error) {
	repository.observation = observation
	return ConnectionHealthEvent{ID: uuid.New()}, nil
}

func (*capturingConnectionHealthRepository) List(context.Context, uuid.UUID, uuid.UUID, int, int) ([]ConnectionHealthEvent, int64, error) {
	return nil, 0, nil
}

func TestConnectionHealthFailureThresholdIsPolicyNotHardcoded(t *testing.T) {
	connection := &IntegrationConnection{
		HealthStatus: ConnectionHealthUnknown, AuthStatus: ConnectionAuthValid,
		ScopeStatus: ConnectionScopeVerified, HealthRevision: 1,
	}
	now := time.Now().UTC()
	observation := ConnectionHealthObservation{
		Classification:   ConnectionHealthClassificationTransient,
		FailureThreshold: 2, ObservedAt: now,
	}
	applyConnectionHealthObservation(connection, observation, &ConnectionHealthEvent{})
	if connection.HealthStatus == ConnectionHealthDegraded || connection.ConsecutiveFailures != 1 {
		t.Fatalf("first transient failure = %#v", connection)
	}
	observation.ObservedAt = now.Add(time.Second)
	applyConnectionHealthObservation(connection, observation, &ConnectionHealthEvent{})
	if connection.HealthStatus != ConnectionHealthDegraded || connection.ConsecutiveFailures != 2 {
		t.Fatalf("threshold failure = %#v", connection)
	}
}

func TestPassiveRuntimeSuccessDoesNotClearReconnectRequired(t *testing.T) {
	now := time.Now().UTC()
	connection := &IntegrationConnection{
		HealthStatus:        ConnectionHealthUnhealthy,
		AuthStatus:          ConnectionAuthReconnectRequired,
		ScopeStatus:         ConnectionScopeVerified,
		AttentionCode:       stringPointer(ConnectionAttentionReconnectRequired),
		ConsecutiveFailures: 2,
		HealthRevision:      4,
	}
	applyConnectionHealthObservation(connection, ConnectionHealthObservation{
		Source:         ConnectionHealthSourceRuntime,
		Classification: ConnectionHealthClassificationSuccess,
		ObservedAt:     now,
	}, &ConnectionHealthEvent{})
	if connection.AuthStatus != ConnectionAuthReconnectRequired || connection.HealthStatus != ConnectionHealthUnhealthy {
		t.Fatalf("passive success recovered explicit auth failure: %#v", connection)
	}
	if connection.AttentionCode == nil || *connection.AttentionCode != ConnectionAttentionReconnectRequired {
		t.Fatalf("passive success cleared reconnect attention: %#v", connection.AttentionCode)
	}
	if connection.LastRuntimeSuccessAt == nil || !connection.LastRuntimeSuccessAt.Equal(now) {
		t.Fatalf("passive success timestamp = %#v, want %s", connection.LastRuntimeSuccessAt, now)
	}
}

func TestActiveProbeSuccessCanRecoverReconnectRequired(t *testing.T) {
	connection := &IntegrationConnection{
		HealthStatus:   ConnectionHealthUnhealthy,
		AuthStatus:     ConnectionAuthReconnectRequired,
		ScopeStatus:    ConnectionScopeVerified,
		AttentionCode:  stringPointer(ConnectionAttentionReconnectRequired),
		HealthRevision: 4,
	}
	applyConnectionHealthObservation(connection, ConnectionHealthObservation{
		Source:         ConnectionHealthSourceManual,
		CheckKind:      ConnectionHealthCheckFull,
		Classification: ConnectionHealthClassificationSuccess,
		ObservedAt:     time.Now().UTC(),
	}, &ConnectionHealthEvent{})
	if connection.AuthStatus != ConnectionAuthValid || connection.HealthStatus != ConnectionHealthHealthy || connection.AttentionCode != nil {
		t.Fatalf("active success did not recover connection: %#v", connection)
	}
}

func TestPassiveResourceAccessDeniedDoesNotDegradeWholeConnection(t *testing.T) {
	now := time.Now().UTC()
	connection := &IntegrationConnection{
		HealthStatus:   ConnectionHealthHealthy,
		AuthStatus:     ConnectionAuthValid,
		ScopeStatus:    ConnectionScopeVerified,
		HealthRevision: 4,
	}
	applyConnectionHealthObservation(connection, ConnectionHealthObservation{
		Source:         ConnectionHealthSourceRuntime,
		CheckKind:      ConnectionHealthCheckPassive,
		Classification: ConnectionHealthClassificationAccessDenied,
		ObservedAt:     now,
	}, &ConnectionHealthEvent{})
	if connection.HealthStatus != ConnectionHealthHealthy ||
		connection.AuthStatus != ConnectionAuthValid ||
		connection.ScopeStatus != ConnectionScopeVerified ||
		connection.AttentionCode != nil {
		t.Fatalf("ambiguous resource access failure contaminated connection health: %#v", connection)
	}
	if connection.LastRuntimeFailureAt == nil || !connection.LastRuntimeFailureAt.Equal(now) {
		t.Fatalf("runtime failure timestamp = %#v, want %s", connection.LastRuntimeFailureAt, now)
	}
	if connection.HealthRevision != 5 {
		t.Fatalf("health revision = %d, want event revision 5", connection.HealthRevision)
	}
}

func TestPassiveAccessDeniedWithExplicitMissingScopesStillDegradesConnection(t *testing.T) {
	connection := &IntegrationConnection{
		HealthStatus: ConnectionHealthHealthy, AuthStatus: ConnectionAuthValid,
		ScopeStatus: ConnectionScopeVerified,
	}
	applyConnectionHealthObservation(connection, ConnectionHealthObservation{
		Source:         ConnectionHealthSourceRuntime,
		CheckKind:      ConnectionHealthCheckPassive,
		Classification: ConnectionHealthClassificationAccessDenied,
		MissingScopes:  []string{"messages:write"},
		ObservedAt:     time.Now().UTC(),
	}, &ConnectionHealthEvent{})
	if connection.HealthStatus != ConnectionHealthDegraded ||
		connection.ScopeStatus != ConnectionScopeDrifted ||
		connection.AttentionCode == nil ||
		*connection.AttentionCode != ConnectionAttentionScopeUpdateRequired {
		t.Fatalf("explicit missing scopes did not degrade connection: %#v", connection)
	}
}

func TestConnectorRuntimeEvidenceIsRecordedPerAction(t *testing.T) {
	connection := &IntegrationConnection{
		HealthStatus: ConnectionHealthHealthy,
		AuthStatus:   ConnectionAuthValid,
		ScopeStatus:  ConnectionScopeUnknown,
	}
	now := time.Now().UTC()
	applyConnectionHealthObservation(connection, ConnectionHealthObservation{
		Source:         ConnectionHealthSourceRuntime,
		CheckKind:      ConnectionHealthCheckPassive,
		Classification: ConnectionHealthClassificationScopeDrift,
		ActionID:       "dingtalk.contact.search",
		ScopeEvidence:  AuthScopeEvidenceConnectorDeclared,
		ObservedAt:     now,
	}, &ConnectionHealthEvent{})
	if len(connection.DeniedActionIDs) != 1 || connection.DeniedActionIDs[0] != "dingtalk.contact.search" ||
		len(connection.VerifiedActionIDs) != 0 || connection.HealthStatus != ConnectionHealthDegraded {
		t.Fatalf("denied action evidence = %#v", connection)
	}

	applyConnectionHealthObservation(connection, ConnectionHealthObservation{
		Source:         ConnectionHealthSourceRuntime,
		CheckKind:      ConnectionHealthCheckPassive,
		Classification: ConnectionHealthClassificationSuccess,
		ActionID:       "dingtalk.department.list",
		ScopeEvidence:  AuthScopeEvidenceConnectorDeclared,
		ObservedAt:     now.Add(time.Second),
	}, &ConnectionHealthEvent{})
	if len(connection.VerifiedActionIDs) != 1 || connection.VerifiedActionIDs[0] != "dingtalk.department.list" ||
		len(connection.DeniedActionIDs) != 1 || connection.HealthStatus != ConnectionHealthDegraded {
		t.Fatalf("unrelated success cleared exact denial: %#v", connection)
	}

	applyConnectionHealthObservation(connection, ConnectionHealthObservation{
		Source:         ConnectionHealthSourceRuntime,
		CheckKind:      ConnectionHealthCheckPassive,
		Classification: ConnectionHealthClassificationSuccess,
		ActionID:       "dingtalk.contact.search",
		ScopeEvidence:  AuthScopeEvidenceConnectorDeclared,
		ObservedAt:     now.Add(2 * time.Second),
	}, &ConnectionHealthEvent{})
	if len(connection.DeniedActionIDs) != 0 || len(connection.VerifiedActionIDs) != 2 ||
		connection.HealthStatus != ConnectionHealthHealthy || connection.AttentionCode != nil {
		t.Fatalf("successful exact action did not recover evidence: %#v", connection)
	}
}

func TestLateRuntimeSuccessIsStaleAfterNewerAuthFailure(t *testing.T) {
	newer := time.Now().UTC()
	older := newer.Add(-time.Minute)
	connection := IntegrationConnection{
		CredentialVersion:    3,
		HealthRevision:       8,
		LastHealthCheckedAt:  &newer,
		LastRuntimeFailureAt: &newer,
	}
	if !connectionHealthObservationIsStale(connection, ConnectionHealthObservation{
		CredentialVersion: 3,
		Source:            ConnectionHealthSourceRuntime,
		Classification:    ConnectionHealthClassificationSuccess,
		ObservedAt:        older,
	}) {
		t.Fatal("late runtime success must be stale after a newer auth failure")
	}
	if connectionHealthObservationIsStale(connection, ConnectionHealthObservation{
		CredentialVersion: 3,
		Source:            ConnectionHealthSourceRuntime,
		Classification:    ConnectionHealthClassificationSuccess,
		ObservedAt:        newer.Add(time.Second),
	}) {
		t.Fatal("new runtime observation was incorrectly marked stale")
	}
}

func TestConnectionHealthServiceInjectsConfiguredFailureThreshold(t *testing.T) {
	repository := &capturingConnectionHealthRepository{}
	service := NewConnectionHealthService(repository).WithFailureThreshold(7)
	_, err := service.RecordConnectionHealthObservation(context.Background(), ConnectionHealthObservation{
		OrganizationID: uuid.New(), ConnectionID: uuid.New(), CredentialVersion: 1,
		Classification: ConnectionHealthClassificationTransient,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.observation.FailureThreshold != 7 {
		t.Fatalf("failure threshold = %d", repository.observation.FailureThreshold)
	}
}

func TestConnectionHealthServicePersistsSafeRuntimeProviderDiagnostics(t *testing.T) {
	repository := &capturingConnectionHealthRepository{}
	service := NewConnectionHealthService(repository)
	status := 403
	retryAfter := time.Date(2026, 7, 28, 14, 0, 0, 0, time.UTC)
	err := service.PublishConnectionHealthSignal(context.Background(), ConnectionHealthSignal{
		OrganizationID:     uuid.New(),
		ConnectionID:       uuid.New(),
		IntegrationID:      "feishu",
		DriverID:           "feishu",
		ActionID:           "feishu.contact.search",
		ScopeEvidence:      AuthScopeEvidenceConnectorDeclared,
		CredentialVersion:  1,
		ExecutionID:        uuid.New(),
		ProviderRequestID:  "feishu-log-123",
		ProviderErrorCode:  "99991672",
		ProviderHTTPStatus: &status,
		RetryAfterAt:       &retryAfter,
		ErrorCode:          ErrorCodeAccessDenied,
		ObservedAt:         time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := repository.observation
	if got.ProviderErrorCode != "99991672" ||
		got.ActionID != "feishu.contact.search" ||
		got.ScopeEvidence != AuthScopeEvidenceConnectorDeclared ||
		got.ProviderRequestID != "feishu-log-123" ||
		got.ProviderHTTPStatus == nil ||
		*got.ProviderHTTPStatus != status ||
		got.RetryAfterAt == nil ||
		!got.RetryAfterAt.Equal(retryAfter) {
		t.Fatalf("runtime observation = %#v", got)
	}
}

func TestConnectionTestRecordsProviderRejectionAsActionableConfigurationFailure(t *testing.T) {
	repository := &capturingConnectionHealthRepository{}
	service := NewConnectionHealthService(repository)
	connection := &IntegrationConnection{
		ID: uuid.New(), OrganizationID: uuid.New(), IntegrationID: "wecom", DriverID: "wecom-open-api",
		CredentialVersion: 1, HealthRevision: 2,
	}
	testErr := NewProviderError(
		ErrorCodeProviderRejected,
		"provider rejected validation",
		nil,
		ProviderDiagnostics{ErrorCode: "60020", RequestID: "req-1", HTTPStatus: 200},
	)
	recordConnectionTestHealth(
		context.Background(),
		service,
		connection,
		nil,
		testErr,
		ConnectionHealthSourceManual,
		nil,
		time.Now().Add(-time.Millisecond),
	)
	got := repository.observation
	if got.Classification != ConnectionHealthClassificationAccessDenied || got.ReasonCode != ErrorCodeProviderRejected || got.ProviderErrorCode != "60020" || got.ProviderRequestID != "req-1" {
		t.Fatalf("connection-test observation = %#v", got)
	}
	if got.CheckKind != ConnectionHealthCheckFull {
		t.Fatalf("check kind = %q", got.CheckKind)
	}
}

func TestConnectionHealthEventNormalizesUnsafeProviderDiagnostics(t *testing.T) {
	observation := normalizeConnectionHealthObservation(ConnectionHealthObservation{
		ProviderErrorCode:  `{"message":"do not persist"}`,
		ProviderRequestID:  "request id with spaces",
		ProviderHTTPStatus: intPointerForHealthTest(42),
	})
	if observation.ProviderErrorCode != "" || observation.ProviderRequestID != "" || observation.ProviderHTTPStatus != nil {
		t.Fatalf("unsafe observation diagnostics retained: %#v", observation)
	}

	safe := normalizeConnectionHealthObservation(ConnectionHealthObservation{
		ProviderErrorCode:  "99991672",
		ProviderRequestID:  "feishu-log-123",
		ProviderHTTPStatus: intPointerForHealthTest(403),
	})
	event := connectionHealthEventFromObservation(IntegrationConnection{}, safe)
	if event.ProviderErrorCode == nil ||
		*event.ProviderErrorCode != "99991672" ||
		event.ProviderRequestID == nil ||
		*event.ProviderRequestID != "feishu-log-123" ||
		event.ProviderHTTPStatus == nil ||
		*event.ProviderHTTPStatus != 403 {
		t.Fatalf("health event = %#v", event)
	}
}

func TestClassifyRuntimeConnectionHealthSignalCoversAuthorizationLifecycle(t *testing.T) {
	tests := []struct {
		errorCode      string
		classification ConnectionHealthClassification
	}{
		{ErrorCodeReconnectRequired, ConnectionHealthClassificationAuthInvalid},
		{ErrorCodeConnectionInvalid, ConnectionHealthClassificationAuthInvalid},
		{ErrorCodeConnectionExpired, ConnectionHealthClassificationOAuthExpired},
		{ErrorCodeInsufficientScope, ConnectionHealthClassificationScopeDrift},
		{ErrorCodeActionAuthMethod, ConnectionHealthClassificationAccessDenied},
		{ErrorCodeProviderRejected, ConnectionHealthClassificationIgnored},
	}
	for _, test := range tests {
		t.Run(test.errorCode, func(t *testing.T) {
			classification, reason := classifyRuntimeConnectionHealthSignal(test.errorCode)
			if classification != test.classification || reason != test.errorCode {
				t.Fatalf("classification = %q, reason = %q", classification, reason)
			}
		})
	}
}

func TestConnectionHealthSummarySerializesScopeArraysAsJSONB(t *testing.T) {
	updates := connectionHealthSummaryUpdates(IntegrationConnection{
		GrantedScopes:         []string{"repo:read"},
		MissingRequiredScopes: []string{"issues:read"},
		VerifiedActionIDs:     []string{"repository.list"},
		DeniedActionIDs:       []string{"repository.delete"},
	})
	for _, key := range []string{"granted_scopes", "missing_required_scopes", "verified_action_ids", "denied_action_ids"} {
		value, ok := updates[key].(datatypes.JSON)
		if !ok || len(value) == 0 || value[0] != '[' {
			t.Fatalf("%s update = %#v, want JSON array", key, updates[key])
		}
	}
}

func intPointerForHealthTest(value int) *int { return &value }
