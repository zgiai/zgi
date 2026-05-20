package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openGatewayTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := "file:gateway_billing_reconcile_" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skip("sqlite driver requires cgo in this environment")
		}
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	return db
}

func TestRemoteBilling_ScheduleReconcileFailure_WithRemainingRetries(t *testing.T) {
	db := openGatewayTestDB(t)
	if err := db.AutoMigrate(&BillingAttempt{}); err != nil {
		t.Fatalf("failed to migrate billing_attempts: %v", err)
	}

	now := time.Now()
	attempt := &BillingAttempt{
		AttemptID:         "attempt-1",
		RequestID:         "req-1",
		OrganizationID:    uuid.New(),
		Lane:              billingAttemptLaneRemote,
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    "key-1",
		Status:            billingAttemptStatusSettlePending,
		ReconcileAttempts: 1,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := db.Create(attempt).Error; err != nil {
		t.Fatalf("failed to seed attempt: %v", err)
	}

	rb := &RemoteBilling{
		localService: &BillingService{db: db},
	}
	if err := rb.scheduleReconcileFailure(context.Background(), attempt.AttemptID, errors.New("grpc timeout")); err != nil {
		t.Fatalf("scheduleReconcileFailure returned error: %v", err)
	}

	var got BillingAttempt
	if err := db.Where("attempt_id = ?", attempt.AttemptID).First(&got).Error; err != nil {
		t.Fatalf("failed to load attempt: %v", err)
	}

	if got.Status != billingAttemptStatusPartial {
		t.Fatalf("status = %s, want %s", got.Status, billingAttemptStatusPartial)
	}
	if got.NextReconcileAt == nil {
		t.Fatalf("next_reconcile_at should be set when retries remain")
	}
	if got.ErrorCode == nil || *got.ErrorCode != "ATTEMPT_RECONCILE_FAILED" {
		t.Fatalf("error_code = %v, want ATTEMPT_RECONCILE_FAILED", got.ErrorCode)
	}
}

func TestRemoteBilling_ScheduleReconcileFailure_ToDeadLetterAtMaxRetries(t *testing.T) {
	db := openGatewayTestDB(t)
	if err := db.AutoMigrate(&BillingAttempt{}); err != nil {
		t.Fatalf("failed to migrate billing_attempts: %v", err)
	}

	now := time.Now()
	attempt := &BillingAttempt{
		AttemptID:         "attempt-max",
		RequestID:         "req-max",
		OrganizationID:    uuid.New(),
		Lane:              billingAttemptLaneRemote,
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    "key-1",
		Status:            billingAttemptStatusSettlePending,
		ReconcileAttempts: defaultReconcileMaxRetries,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := db.Create(attempt).Error; err != nil {
		t.Fatalf("failed to seed attempt: %v", err)
	}

	rb := &RemoteBilling{
		localService: &BillingService{db: db},
	}
	if err := rb.scheduleReconcileFailure(context.Background(), attempt.AttemptID, errors.New("grpc timeout")); err != nil {
		t.Fatalf("scheduleReconcileFailure returned error: %v", err)
	}

	var got BillingAttempt
	if err := db.Where("attempt_id = ?", attempt.AttemptID).First(&got).Error; err != nil {
		t.Fatalf("failed to load attempt: %v", err)
	}

	if got.Status != billingAttemptStatusDeadLetter {
		t.Fatalf("status = %s, want %s", got.Status, billingAttemptStatusDeadLetter)
	}
	if got.NextReconcileAt != nil {
		t.Fatalf("next_reconcile_at should be nil for dead-letter attempt")
	}
}

func TestRemoteBilling_RecoverStaleSettlePendingAttempts(t *testing.T) {
	db := openGatewayTestDB(t)
	if err := db.AutoMigrate(&BillingAttempt{}); err != nil {
		t.Fatalf("failed to migrate billing_attempts: %v", err)
	}

	old := time.Now().Add(-(defaultSettlePendingTimeout + time.Minute))
	recent := time.Now()

	stale := &BillingAttempt{
		AttemptID:        "attempt-stale",
		RequestID:        "req-stale",
		OrganizationID:   uuid.New(),
		Lane:             billingAttemptLaneRemote,
		QuotaSubjectType: quotaSubjectTypeAPIKey,
		QuotaSubjectID:   "key-1",
		Status:           billingAttemptStatusSettlePending,
		CreatedAt:        old,
		UpdatedAt:        old,
	}
	fresh := &BillingAttempt{
		AttemptID:        "attempt-fresh",
		RequestID:        "req-fresh",
		OrganizationID:   uuid.New(),
		Lane:             billingAttemptLaneRemote,
		QuotaSubjectType: quotaSubjectTypeAPIKey,
		QuotaSubjectID:   "key-2",
		Status:           billingAttemptStatusSettlePending,
		CreatedAt:        recent,
		UpdatedAt:        recent,
	}
	if err := db.Create(stale).Error; err != nil {
		t.Fatalf("failed to seed stale attempt: %v", err)
	}
	if err := db.Create(fresh).Error; err != nil {
		t.Fatalf("failed to seed fresh attempt: %v", err)
	}

	rb := &RemoteBilling{
		localService: &BillingService{db: db},
	}
	if err := rb.recoverStaleSettlePendingAttempts(context.Background()); err != nil {
		t.Fatalf("recoverStaleSettlePendingAttempts returned error: %v", err)
	}

	var gotStale BillingAttempt
	if err := db.Where("attempt_id = ?", stale.AttemptID).First(&gotStale).Error; err != nil {
		t.Fatalf("failed to load stale attempt: %v", err)
	}
	if gotStale.Status != billingAttemptStatusPartial {
		t.Fatalf("stale status = %s, want %s", gotStale.Status, billingAttemptStatusPartial)
	}
	if gotStale.NextReconcileAt == nil {
		t.Fatalf("stale attempt should have next_reconcile_at")
	}

	var gotFresh BillingAttempt
	if err := db.Where("attempt_id = ?", fresh.AttemptID).First(&gotFresh).Error; err != nil {
		t.Fatalf("failed to load fresh attempt: %v", err)
	}
	if gotFresh.Status != billingAttemptStatusSettlePending {
		t.Fatalf("fresh status = %s, want %s", gotFresh.Status, billingAttemptStatusSettlePending)
	}
}

func TestRemoteBilling_ReconcileMissingDeductionID_RequeuesInsteadOfImmediateDeadLetter(t *testing.T) {
	db := openGatewayTestDB(t)
	if err := db.AutoMigrate(&BillingAttempt{}, &BillingAttemptEntry{}); err != nil {
		t.Fatalf("failed to migrate billing tables: %v", err)
	}

	now := time.Now()
	attempt := &BillingAttempt{
		AttemptID:         "attempt-missing-deduction",
		RequestID:         "req-missing-deduction",
		OrganizationID:    uuid.New(),
		Lane:              billingAttemptLaneRemote,
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    "key-1",
		Status:            billingAttemptStatusPartial,
		ReconcileAttempts: 0,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := db.Create(attempt).Error; err != nil {
		t.Fatalf("failed to seed attempt: %v", err)
	}

	fundEntry := &BillingAttemptEntry{
		ID:             uuid.New(),
		AttemptID:      attempt.AttemptID,
		EntryType:      billingEntryTypeFund,
		LedgerType:     billingLedgerTypeOrgFunds,
		LedgerRefID:    attempt.OrganizationID.String(),
		ReservedAmount: 12,
		Status:         billingEntryStatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(fundEntry).Error; err != nil {
		t.Fatalf("failed to seed fund entry: %v", err)
	}

	rb := &RemoteBilling{
		localService: &BillingService{db: db},
	}
	if err := rb.reconcilePartialSettledAttempts(context.Background()); err != nil {
		t.Fatalf("reconcilePartialSettledAttempts returned error: %v", err)
	}

	var got BillingAttempt
	if err := db.Where("attempt_id = ?", attempt.AttemptID).First(&got).Error; err != nil {
		t.Fatalf("failed to load attempt: %v", err)
	}

	if got.Status != billingAttemptStatusPartial {
		t.Fatalf("status = %s, want %s", got.Status, billingAttemptStatusPartial)
	}
	if got.ErrorCode == nil || *got.ErrorCode != "RECONCILE_MISSING_DEDUCTION_ID" {
		t.Fatalf("error_code = %v, want RECONCILE_MISSING_DEDUCTION_ID", got.ErrorCode)
	}
	if got.NextReconcileAt == nil {
		t.Fatalf("next_reconcile_at should be set for retry")
	}
	if got.ReconcileAttempts != 1 {
		t.Fatalf("reconcile_attempts = %d, want 1", got.ReconcileAttempts)
	}
}
