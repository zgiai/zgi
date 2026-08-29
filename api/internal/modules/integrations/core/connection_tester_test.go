package integrations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type connectionTestDelegate struct {
	calls   int
	profile *ConnectionProfile
	err     error
}

type connectionTestQuota struct {
	calls          int
	organizationID string
	err            error
}

func (quota *connectionTestQuota) Acquire(_ context.Context, organizationID string) error {
	quota.calls++
	quota.organizationID = organizationID
	return quota.err
}

func (delegate *connectionTestDelegate) ValidateConnection(_ context.Context, _ *ResolvedConnection) (*ConnectionProfile, error) {
	delegate.calls++
	return delegate.profile, delegate.err
}

func TestAuditedConnectionTesterRecordsProviderMetadata(t *testing.T) {
	cost := 0.004
	delegate := &connectionTestDelegate{profile: &ConnectionProfile{
		ProviderRequestID: "exa-request-1",
		CostUSD:           &cost,
	}}
	audit := &testAudit{}
	quota := &connectionTestQuota{}
	tester := NewAuditedConnectionTester(delegate, audit, time.Second).WithDailyQuota(quota)
	connectionID := uuid.New()
	organizationID := uuid.New()
	actorID := uuid.New()

	profile, err := tester.ValidateConnectionAs(context.Background(), &ResolvedConnection{
		ID:             connectionID.String(),
		OrganizationID: organizationID.String(),
		IntegrationID:  IntegrationWebSearch,
		DriverID:       DriverExa,
	}, &actorID)
	if err != nil {
		t.Fatalf("ValidateConnection() error = %v", err)
	}
	if profile == nil || profile.ProviderRequestID != "exa-request-1" || delegate.calls != 1 {
		t.Fatalf("profile = %#v, delegate calls = %d", profile, delegate.calls)
	}
	if quota.calls != 1 || quota.organizationID != organizationID.String() {
		t.Fatalf("quota calls = %d, organization = %q", quota.calls, quota.organizationID)
	}
	if audit.created == nil || audit.created.ActionID != ActionConnectionTest || audit.created.InvokeFrom != "management" || audit.created.ConnectionID == nil || *audit.created.ConnectionID != connectionID {
		t.Fatalf("created audit = %#v", audit.created)
	}
	if audit.created.AccountID == nil || *audit.created.AccountID != actorID {
		t.Fatalf("audit actor = %#v, want %s", audit.created.AccountID, actorID)
	}
	if audit.created.InputHMAC != nil {
		t.Fatalf("connection test audit must not retain input fingerprint: %#v", audit.created.InputHMAC)
	}
	if audit.completion.Status != "succeeded" || audit.completion.ProviderRequestID != "exa-request-1" || audit.completion.CostUSD == nil || *audit.completion.CostUSD != cost {
		t.Fatalf("completion = %#v", audit.completion)
	}
}

func TestAuditedConnectionTesterFailsClosedBeforeProviderWhenAuditCreateFails(t *testing.T) {
	delegate := &connectionTestDelegate{}
	audit := &testAudit{createErr: errors.New("database unavailable")}
	tester := NewAuditedConnectionTester(delegate, audit, time.Second).WithDailyQuota(&connectionTestQuota{})

	profile, err := tester.ValidateConnection(context.Background(), &ResolvedConnection{
		OrganizationID: uuid.New().String(),
		IntegrationID:  IntegrationWebSearch,
		DriverID:       DriverExa,
	})
	if profile != nil || ErrorCode(err) != ErrorCodeAuditFailed {
		t.Fatalf("profile = %#v, error = %v, code = %q", profile, err, ErrorCode(err))
	}
	if delegate.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", delegate.calls)
	}
}

func TestAuditedConnectionTesterQueuesCompletionWithoutRepeatingProvider(t *testing.T) {
	delegate := &connectionTestDelegate{profile: &ConnectionProfile{ProviderRequestID: "exa-request-queued"}}
	audit := &testAudit{completeErr: errors.New("database unavailable")}
	outbox := &testCompletionOutbox{}
	tester := NewAuditedConnectionTester(delegate, audit, time.Second).
		WithDailyQuota(&connectionTestQuota{}).
		WithCompletionOutbox(outbox)

	profile, err := tester.ValidateConnection(context.Background(), &ResolvedConnection{
		OrganizationID: uuid.New().String(),
		IntegrationID:  IntegrationWebSearch,
		DriverID:       DriverExa,
	})
	if err != nil || profile == nil {
		t.Fatalf("profile = %#v, error = %v", profile, err)
	}
	if delegate.calls != 1 || audit.completeCalls != 3 || outbox.enqueueCalls != 1 {
		t.Fatalf("delegate=%d complete=%d enqueue=%d", delegate.calls, audit.completeCalls, outbox.enqueueCalls)
	}
	if outbox.enqueued.Completion.ProviderRequestID != "exa-request-queued" || outbox.enqueued.Completion.Status != "succeeded" {
		t.Fatalf("queued completion = %#v", outbox.enqueued)
	}
}

func TestAuditedConnectionTesterStopsBeforeAuditAndProviderWhenQuotaIsExhausted(t *testing.T) {
	delegate := &connectionTestDelegate{}
	audit := &testAudit{}
	quota := &connectionTestQuota{err: ErrQuotaExceeded}
	tester := NewAuditedConnectionTester(delegate, audit, time.Second).WithDailyQuota(quota)

	profile, err := tester.ValidateConnection(context.Background(), &ResolvedConnection{
		OrganizationID: uuid.New().String(),
		IntegrationID:  IntegrationWebSearch,
		DriverID:       DriverExa,
	})
	if profile != nil || ErrorCode(err) != ErrorCodeQuotaExceeded {
		t.Fatalf("profile = %#v, error = %v, code = %q", profile, err, ErrorCode(err))
	}
	if quota.calls != 1 || delegate.calls != 0 || audit.created != nil {
		t.Fatalf("quota=%d provider=%d audit=%#v", quota.calls, delegate.calls, audit.created)
	}
}

func TestAuditedConnectionTesterFailsClosedWhenQuotaServiceIsMissing(t *testing.T) {
	delegate := &connectionTestDelegate{}
	audit := &testAudit{}
	tester := NewAuditedConnectionTester(delegate, audit, time.Second)

	_, err := tester.ValidateConnection(context.Background(), &ResolvedConnection{
		OrganizationID: uuid.New().String(),
		IntegrationID:  IntegrationWebSearch,
		DriverID:       DriverExa,
	})
	if ErrorCode(err) != ErrorCodeQuotaExceeded || delegate.calls != 0 || audit.created != nil {
		t.Fatalf("error = %v, provider=%d audit=%#v", err, delegate.calls, audit.created)
	}
}

func TestAuditedConnectionTesterRecordsTimeoutWithQueryableStatus(t *testing.T) {
	delegate := &connectionTestDelegate{err: NewError(ErrorCodeTimeout, "provider timed out", context.DeadlineExceeded)}
	audit := &testAudit{}
	tester := NewAuditedConnectionTester(delegate, audit, time.Second).WithDailyQuota(&connectionTestQuota{})

	_, err := tester.ValidateConnection(context.Background(), &ResolvedConnection{
		OrganizationID: uuid.New().String(),
		IntegrationID:  IntegrationWebSearch,
		DriverID:       DriverExa,
	})
	if ErrorCode(err) != ErrorCodeTimeout || audit.completion.Status != "timed_out" || audit.completion.ErrorCode != ErrorCodeTimeout {
		t.Fatalf("error = %v, completion = %#v", err, audit.completion)
	}
}
