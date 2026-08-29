package integrations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const ActionConnectionTest = "connection.test"

// AuditedConnectionTester ensures a paid provider validation never runs
// without a durable execution record. It deliberately records no query or
// credential material.
type AuditedConnectionTester struct {
	delegate ConnectionTester
	audit    ExecutionRepository
	quota    DailyQuota
	timeout  time.Duration
	outbox   ExecutionCompletionOutbox
}

func NewAuditedConnectionTester(delegate ConnectionTester, audit ExecutionRepository, timeout time.Duration) *AuditedConnectionTester {
	return &AuditedConnectionTester{delegate: delegate, audit: audit, timeout: timeout}
}

func (tester *AuditedConnectionTester) WithCompletionOutbox(outbox ExecutionCompletionOutbox) *AuditedConnectionTester {
	if tester != nil {
		tester.outbox = outbox
	}
	return tester
}

func (tester *AuditedConnectionTester) WithDailyQuota(quota DailyQuota) *AuditedConnectionTester {
	if tester != nil {
		tester.quota = quota
	}
	return tester
}

func (tester *AuditedConnectionTester) ValidateConnection(ctx context.Context, connection *ResolvedConnection) (*ConnectionProfile, error) {
	return tester.validateConnection(ctx, connection, nil)
}

func (tester *AuditedConnectionTester) ValidateConnectionAs(ctx context.Context, connection *ResolvedConnection, actorID *uuid.UUID) (*ConnectionProfile, error) {
	return tester.validateConnection(ctx, connection, actorID)
}

func (tester *AuditedConnectionTester) validateConnection(ctx context.Context, connection *ResolvedConnection, actorID *uuid.UUID) (*ConnectionProfile, error) {
	if tester == nil || tester.delegate == nil || tester.audit == nil || connection == nil {
		return nil, NewError(ErrorCodeAuditFailed, "integration connection test audit service is unavailable", nil)
	}
	organizationID, err := uuid.Parse(strings.TrimSpace(connection.OrganizationID))
	if err != nil || organizationID == uuid.Nil {
		return nil, invalidInput("organization id is required", err)
	}
	if tester.quota == nil {
		return nil, NewError(ErrorCodeQuotaExceeded, "external integration quota service is unavailable", nil)
	}
	if err := tester.quota.Acquire(ctx, organizationID.String()); err != nil {
		if errors.Is(err, ErrQuotaExceeded) {
			return nil, NewError(ErrorCodeQuotaExceeded, "organization web search daily limit has been reached", err)
		}
		return nil, NewError(ErrorCodeQuotaExceeded, "external integration quota service is unavailable", err)
	}
	now := time.Now().UTC()
	record := &ExecutionRecord{
		ID:             uuid.New(),
		OrganizationID: organizationID,
		AccountID:      cloneUUIDPointer(actorID),
		ConnectionID:   optionalUUID(connection.ID),
		IntegrationID:  strings.TrimSpace(connection.IntegrationID),
		DriverID:       strings.TrimSpace(connection.DriverID),
		ActionID:       ActionConnectionTest,
		InvokeFrom:     "management",
		Status:         "running",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := tester.audit.Create(ctx, record); err != nil {
		return nil, NewError(ErrorCodeAuditFailed, "integration connection test could not be audited", err)
	}
	operationCtx := ctx
	cancel := func() {}
	if tester.timeout > 0 {
		operationCtx, cancel = context.WithTimeout(ctx, tester.timeout)
	}
	startedAt := time.Now()
	profile, testErr := tester.delegate.ValidateConnection(operationCtx, connection)
	cancel()
	completion := ExecutionCompletion{
		Status:       "succeeded",
		DurationMS:   time.Since(startedAt).Milliseconds(),
		AttemptCount: 1,
	}
	if profile != nil {
		completion.ProviderRequestID = strings.TrimSpace(profile.ProviderRequestID)
		completion.CostUSD = profile.CostUSD
	}
	if testErr != nil {
		completion.Status = "failed"
		if ErrorCode(testErr) == ErrorCodeTimeout {
			completion.Status = "timed_out"
		}
		completion.ErrorCode = ErrorCode(testErr)
	}
	finalizeCtx, finalizeCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer finalizeCancel()
	if err := completeAuditWithRetry(finalizeCtx, tester.audit, record.ID, completion); err != nil {
		if tester.outbox == nil {
			return nil, NewError(ErrorCodeAuditFailed, "integration connection test audit could not be completed", fmt.Errorf("complete audit: %w", err))
		}
		queueCtx, queueCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		queueErr := tester.outbox.Enqueue(queueCtx, PendingExecutionCompletion{ExecutionID: record.ID, Completion: completion})
		queueCancel()
		if queueErr != nil {
			return nil, NewError(ErrorCodeAuditFailed, "integration connection test audit could not be completed", fmt.Errorf("complete audit: %w; queue completion: %v", err, queueErr))
		}
	}
	return profile, testErr
}
