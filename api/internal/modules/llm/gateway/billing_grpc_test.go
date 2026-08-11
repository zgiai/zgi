package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type failingSettleQuotaClient struct{}

func (failingSettleQuotaClient) PreDeductQuota(context.Context, *PreDeductQuotaRequest) (*PreDeductQuotaResponse, error) {
	return nil, errors.New("unexpected pre-deduct")
}

func (failingSettleQuotaClient) SettleQuota(context.Context, *SettleQuotaRequest) (*SettleQuotaResponse, error) {
	return nil, errors.New("quota service unavailable")
}

func (failingSettleQuotaClient) CheckCreditBalance(context.Context, string, int64) (bool, int64, error) {
	return false, 0, errors.New("unexpected balance check")
}

func (failingSettleQuotaClient) Close() error { return nil }

func openRemoteBillingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&BillingAttempt{}, &BillingAttemptEntry{}, &UsageBill{}); err != nil {
		t.Fatalf("automigrate billing tables: %v", err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX uq_billing_attempt_entry ON billing_attempt_entries (attempt_id, entry_type, ledger_type)`).Error; err != nil {
		t.Fatalf("create billing attempt entry unique index: %v", err)
	}
	return db
}

func TestRemoteBillingMarkAttemptSettleFailedWritesPartialUsageBill(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	remote := &RemoteBilling{localService: &BillingService{db: db}}
	bc := testUsageBillContext(time.Now().Add(-time.Second), time.Now())
	bc.BillingLane = UsageBillingLanePlatform
	bc.UseSystemProvider = true
	bc.ActualCredits = 9

	err := remote.markAttemptSettleFailed(context.Background(), bc, "SETTLE_FAILED", "grpc down")
	if err != nil {
		t.Fatalf("markAttemptSettleFailed returned error: %v", err)
	}

	var bill UsageBill
	if err := db.Where("attempt_id = ?", bc.AttemptID).First(&bill).Error; err != nil {
		t.Fatalf("load usage bill: %v", err)
	}
	if bill.Status != usageBillStatusPartial {
		t.Fatalf("usage bill status = %q, want %q", bill.Status, usageBillStatusPartial)
	}
	if bill.OfficialPoints != 9 || bill.TotalPoints != 9 {
		t.Fatalf("usage bill points = official %d total %d, want 9/9", bill.OfficialPoints, bill.TotalPoints)
	}
	if bill.ErrorCode == nil || *bill.ErrorCode != "SETTLE_FAILED" {
		t.Fatalf("usage bill error code = %v, want SETTLE_FAILED", bill.ErrorCode)
	}
}

func TestBillingAttemptRecoveryPreservesInvocationSource(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	service := &BillingService{db: db}
	bc := testUsageBillContext(time.Now().Add(-time.Second), time.Now())
	bc.InvocationSource = InvocationSourceAPI
	bc.EstimatedCredits = 11

	if err := db.Transaction(func(tx *gorm.DB) error {
		return service.upsertAttemptInit(context.Background(), tx, bc)
	}); err != nil {
		t.Fatalf("upsert billing attempt: %v", err)
	}

	var attempt BillingAttempt
	if err := db.Where("attempt_id = ?", bc.AttemptID).First(&attempt).Error; err != nil {
		t.Fatalf("load billing attempt: %v", err)
	}
	if attempt.InvocationSource != InvocationSourceAPI {
		t.Fatalf("persisted invocation source = %q, want %q", attempt.InvocationSource, InvocationSourceAPI)
	}

	recovered, err := service.buildLocalRecoveryBillingContext(context.Background(), bc.AttemptID)
	if err != nil {
		t.Fatalf("build recovery billing context: %v", err)
	}
	if recovered.InvocationSource != InvocationSourceAPI {
		t.Fatalf("recovered invocation source = %q, want %q", recovered.InvocationSource, InvocationSourceAPI)
	}
}

func TestRemoteBillingRecoveryPreservesInvocationSourceInPartialUsageBill(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	organizationID := uuid.New()
	attemptID := uuid.NewString()
	deductionID := uuid.NewString()
	invocationResult := "success"
	now := time.Now().UTC()
	attempt := BillingAttempt{
		AttemptID:        attemptID,
		RequestID:        uuid.NewString(),
		OrganizationID:   organizationID,
		Lane:             billingAttemptLaneRemote,
		InvocationSource: InvocationSourceAPI,
		QuotaSubjectType: quotaSubjectTypeOrganization,
		QuotaSubjectID:   organizationID.String(),
		Status:           billingAttemptStatusSettlePending,
		InvocationResult: &invocationResult,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	entries := []BillingAttemptEntry{
		{
			AttemptID: attemptID, EntryType: billingEntryTypeSubject,
			LedgerType: quotaSubjectTypeOrganization + "_quota", LedgerRefID: organizationID.String(),
			ReservedAmount: 7, ActualAmount: 7, Status: billingEntryStatusPending,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			AttemptID: attemptID, EntryType: billingEntryTypeFund,
			LedgerType: billingLedgerTypeOrgFunds, LedgerRefID: organizationID.String(),
			ReservedAmount: 7, ActualAmount: 7, Status: billingEntryStatusPending,
			IdempotencyKey: &deductionID, CreatedAt: now, UpdatedAt: now,
		},
	}
	if err := db.Create(&attempt).Error; err != nil {
		t.Fatalf("create billing attempt: %v", err)
	}
	if err := db.Create(&entries).Error; err != nil {
		t.Fatalf("create billing attempt entries: %v", err)
	}

	remote := &RemoteBilling{
		localService: &BillingService{db: db},
		grpcClient:   failingSettleQuotaClient{},
	}
	if err := remote.reconcileAttempt(context.Background(), attemptID); err == nil {
		t.Fatal("expected failed quota settlement")
	}

	var bill UsageBill
	if err := db.Where("attempt_id = ?", attemptID).First(&bill).Error; err != nil {
		t.Fatalf("load partial usage bill: %v", err)
	}
	if bill.InvocationSource != InvocationSourceAPI {
		t.Fatalf("partial usage bill invocation source = %q, want %q", bill.InvocationSource, InvocationSourceAPI)
	}
}
