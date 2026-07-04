package gateway

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

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
