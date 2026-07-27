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

func TestConnectionHealthSummarySerializesScopeArraysAsJSONB(t *testing.T) {
	updates := connectionHealthSummaryUpdates(IntegrationConnection{
		GrantedScopes:         []string{"repo:read"},
		MissingRequiredScopes: []string{"issues:read"},
	})
	for _, key := range []string{"granted_scopes", "missing_required_scopes"} {
		value, ok := updates[key].(datatypes.JSON)
		if !ok || len(value) == 0 || value[0] != '[' {
			t.Fatalf("%s update = %#v, want JSON array", key, updates[key])
		}
	}
}
