package integrations

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

type DefaultConnectionHealthService struct {
	repository       ConnectionHealthEventRepository
	failureThreshold int
}

func NewConnectionHealthService(repository ConnectionHealthEventRepository) *DefaultConnectionHealthService {
	return &DefaultConnectionHealthService{repository: repository, failureThreshold: 3}
}

func (service *DefaultConnectionHealthService) WithFailureThreshold(threshold int) *DefaultConnectionHealthService {
	if service != nil && threshold > 0 {
		service.failureThreshold = threshold
	}
	return service
}

func (service *DefaultConnectionHealthService) RecordConnectionHealthObservation(ctx context.Context, observation ConnectionHealthObservation) (ConnectionHealthEvent, error) {
	if service == nil || service.repository == nil {
		return ConnectionHealthEvent{}, NewError(ErrorCodeAuditFailed, "connection health history is unavailable", nil)
	}
	if observation.Classification == "" {
		observation.Classification = ConnectionHealthClassificationIgnored
	}
	if observation.FailureThreshold < 1 {
		observation.FailureThreshold = service.failureThreshold
	}
	return service.repository.Record(ctx, observation)
}

func (service *DefaultConnectionHealthService) PublishConnectionHealthSignal(ctx context.Context, signal ConnectionHealthSignal) error {
	classification, reason := classifyRuntimeConnectionHealthSignal(signal.ErrorCode)
	if classification == ConnectionHealthClassificationIgnored {
		return nil
	}
	_, err := service.RecordConnectionHealthObservation(ctx, ConnectionHealthObservation{
		OrganizationID:     signal.OrganizationID,
		ConnectionID:       signal.ConnectionID,
		IntegrationID:      signal.IntegrationID,
		DriverID:           signal.DriverID,
		ActionID:           signal.ActionID,
		Source:             ConnectionHealthSourceRuntime,
		CheckKind:          ConnectionHealthCheckPassive,
		Classification:     classification,
		ReasonCode:         reason,
		CredentialVersion:  signal.CredentialVersion,
		ExecutionID:        optionalHealthUUID(signal.ExecutionID),
		ProviderRequestID:  signal.ProviderRequestID,
		ProviderErrorCode:  signal.ProviderErrorCode,
		ProviderHTTPStatus: providerHTTPStatusPointer(providerHTTPStatusValue(signal.ProviderHTTPStatus)),
		RetryAfterAt:       cloneTimePointer(signal.RetryAfterAt),
		LatencyMS:          signal.DurationMS,
		ObservedAt:         signal.ObservedAt,
		ScopeEvidence:      signal.ScopeEvidence,
	})
	return err
}

func classifyRuntimeConnectionHealthSignal(errorCode string) (ConnectionHealthClassification, string) {
	errorCode = strings.TrimSpace(errorCode)
	switch errorCode {
	case "":
		return ConnectionHealthClassificationSuccess, "runtime_success"
	case ErrorCodeAuthInvalid:
		return ConnectionHealthClassificationAuthInvalid, ErrorCodeAuthInvalid
	case ErrorCodeReconnectRequired, ErrorCodeConnectionInvalid:
		return ConnectionHealthClassificationAuthInvalid, errorCode
	case ErrorCodeConnectionExpired:
		return ConnectionHealthClassificationOAuthExpired, ErrorCodeConnectionExpired
	case ErrorCodeInsufficientScope:
		return ConnectionHealthClassificationScopeDrift, ErrorCodeInsufficientScope
	case ErrorCodeAccessDenied, ErrorCodeActionAuthMethod:
		// A generic 403 is ambiguous: it can be an action scope or resource ACL
		// failure. Never invalidate credentials without provider-owned evidence.
		return ConnectionHealthClassificationAccessDenied, errorCode
	case ErrorCodeBudgetExceeded:
		return ConnectionHealthClassificationBudgetExhausted, ErrorCodeBudgetExceeded
	case ErrorCodeRateLimited:
		return ConnectionHealthClassificationRateLimited, ErrorCodeRateLimited
	case ErrorCodeTimeout, ErrorCodeUpstream:
		return ConnectionHealthClassificationTransient, errorCode
	case ErrorCodeResponseInvalid:
		return ConnectionHealthClassificationProviderIncident, errorCode
	default:
		// Input, policy, safety, platform quota and audit errors say nothing
		// about provider credentials and must not contaminate health.
		return ConnectionHealthClassificationIgnored, errorCode
	}
}

func optionalHealthUUID(value uuid.UUID) *uuid.UUID {
	if value == uuid.Nil {
		return nil
	}
	return &value
}

// recordConnectionTestHealth is shared by manual tests and scheduled probes.
func recordConnectionTestHealth(ctx context.Context, recorder ConnectionHealthObservationRecorder, connection *IntegrationConnection, profile *ConnectionProfile, testErr error, source ConnectionHealthSource, actorID *uuid.UUID, startedAt time.Time) {
	if recorder == nil || connection == nil || connection.ID == uuid.Nil || connection.OrganizationID == uuid.Nil {
		return
	}
	classification, reason := classifyRuntimeConnectionHealthSignal(ErrorCode(testErr))
	if testErr == nil {
		classification, reason = ConnectionHealthClassificationSuccess, "connection_test_succeeded"
	} else if ErrorCode(testErr) == ErrorCodeProviderRejected {
		// A provider business-rule rejection during a normal Action says
		// nothing about connection health. During the dedicated full
		// connection probe, however, it means the saved app configuration
		// itself could not be verified and must be surfaced to the admin.
		classification, reason = ConnectionHealthClassificationAccessDenied, ErrorCodeProviderRejected
	}
	observation := ConnectionHealthObservation{
		OrganizationID:         connection.OrganizationID,
		ConnectionID:           connection.ID,
		IntegrationID:          connection.IntegrationID,
		DriverID:               connection.DriverID,
		Source:                 source,
		CheckKind:              ConnectionHealthCheckFull,
		Classification:         classification,
		ReasonCode:             reason,
		CredentialVersion:      connection.CredentialVersion,
		ExpectedHealthRevision: connection.HealthRevision,
		ActorID:                cloneUUIDPointer(actorID),
		ObservedAt:             time.Now().UTC(),
		LatencyMS:              time.Since(startedAt).Milliseconds(),
		SummaryAlreadyApplied:  true,
	}
	diagnostics := ProviderDiagnosticsFromError(testErr)
	observation.ProviderErrorCode = diagnostics.ErrorCode
	observation.ProviderHTTPStatus = providerHTTPStatusPointer(diagnostics.HTTPStatus)
	observation.RetryAfterAt = cloneTimePointer(diagnostics.RetryAfterAt)
	observation.ProviderRequestID = diagnostics.RequestID
	if profile != nil {
		if requestID := normalizeProviderDiagnostics(ProviderDiagnostics{RequestID: profile.ProviderRequestID}).RequestID; requestID != "" {
			observation.ProviderRequestID = requestID
		}
		observation.GrantedScopes = append([]string(nil), profile.GrantedScopes...)
		observation.ScopeEvidence = profile.ScopeEvidence
		observation.ScopeSnapshotObserved = profile.GrantedScopes != nil &&
			normalizedAuthScopeEvidence(profile.ScopeEvidence) != AuthScopeEvidenceConnectorDeclared
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_, _ = recorder.RecordConnectionHealthObservation(recordCtx, observation)
}
