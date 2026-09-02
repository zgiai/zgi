package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPredeductFailureClosesAttemptEntries(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&BillingAttempt{}, &BillingAttemptEntry{}); err != nil {
		t.Fatal(err)
	}
	attemptID := uuid.NewString()
	now := time.Now()
	attempt := BillingAttempt{
		AttemptID:        attemptID,
		RequestID:        uuid.NewString(),
		OrganizationID:   uuid.New(),
		Lane:             billingAttemptLaneLocal,
		InvocationSource: InvocationSourceAPI,
		QuotaSubjectType: quotaSubjectTypeAccessGrant,
		QuotaSubjectID:   uuid.NewString(),
		AuthMethod:       "personal_api_key",
		Status:           billingAttemptStatusInit,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := db.Create(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	for _, entryType := range []string{billingEntryTypeSubject, billingEntryTypeFund} {
		entry := BillingAttemptEntry{
			AttemptID:      attemptID,
			EntryType:      entryType,
			LedgerType:     entryType + "_ledger",
			LedgerRefID:    uuid.NewString(),
			ReservedAmount: 30,
			Status:         billingEntryStatusPending,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := db.Create(&entry).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := &BillingService{db: db}
	code, message, invocation := "PREDEDUCT_FAILED", "insufficient quota", "error"
	bc := &BillingContext{AttemptID: attemptID}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return service.updateAttemptStatus(context.Background(), tx, bc, billingAttemptStatusPredeductFailed, &invocation, &code, &message)
	}); err != nil {
		t.Fatal(err)
	}
	if !billingAttemptStatusIsFinalized(billingAttemptStatusPredeductFailed) {
		t.Fatal("PREDEDUCT_FAILED must be final")
	}
	var entries []BillingAttemptEntry
	if err := db.Where("attempt_id = ?", attemptID).Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	for _, entry := range entries {
		if entry.Status != billingEntryStatusFailed || entry.ErrorCode == nil || *entry.ErrorCode != code {
			t.Fatalf("entry not closed: %#v", entry)
		}
	}
}

func TestSettlementRecordsCappedSubjectChargeAndActualFundCost(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&BillingAttemptEntry{}); err != nil {
		t.Fatal(err)
	}
	attemptID := uuid.NewString()
	channelID := uuid.NewString()
	now := time.Now()
	entries := []BillingAttemptEntry{
		{AttemptID: attemptID, EntryType: billingEntryTypeSubject, LedgerType: billingLedgerTypeGrantQuota, LedgerRefID: uuid.NewString(), ReservedAmount: 30, Status: billingEntryStatusPending, CreatedAt: now, UpdatedAt: now},
		{AttemptID: attemptID, EntryType: billingEntryTypeFund, LedgerType: billingLedgerTypeChannelWallet, LedgerRefID: channelID, ReservedAmount: 30, Status: billingEntryStatusPending, CreatedAt: now, UpdatedAt: now},
	}
	if err := db.Create(&entries).Error; err != nil {
		t.Fatal(err)
	}
	charged := int64(40)
	bc := &BillingContext{AttemptID: attemptID, EstimatedCredits: 30, ActualCredits: 50, QuotaChargedCredits: &charged, Status: "success"}
	service := &BillingService{db: db}
	if err := service.updateAttemptEntriesAfterSettle(context.Background(), db, bc, billingLedgerTypeChannelWallet, channelID); err != nil {
		t.Fatal(err)
	}
	var subject, fund BillingAttemptEntry
	if err := db.Where("attempt_id = ? AND entry_type = ?", attemptID, billingEntryTypeSubject).First(&subject).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("attempt_id = ? AND entry_type = ?", attemptID, billingEntryTypeFund).First(&fund).Error; err != nil {
		t.Fatal(err)
	}
	if subject.ActualAmount != 40 || subject.RefundedAmount != 0 || fund.ActualAmount != 50 || fund.RefundedAmount != 0 {
		t.Fatalf("subject/fund settlement = %#v / %#v", subject, fund)
	}
}
