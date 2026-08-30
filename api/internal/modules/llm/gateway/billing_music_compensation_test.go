package gateway

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBillingServiceCompensatePrivateMusicDeliveryRefundsExactlyOnce(t *testing.T) {
	service, db, billing, organizationID, apiKeyID, channelID, requestID := newMusicCompensationFixture(t)

	if err := service.CompensatePrivateMusicDelivery(t.Context(), organizationID, requestID); err != nil {
		t.Fatalf("CompensatePrivateMusicDelivery() error = %v", err)
	}
	if err := service.CompensatePrivateMusicDelivery(t.Context(), organizationID, requestID); err != nil {
		t.Fatalf("second CompensatePrivateMusicDelivery() error = %v", err)
	}
	if err := service.Settle(t.Context(), billing); err != nil {
		t.Fatalf("Settle() after compensation error = %v", err)
	}

	var apiKey apikeymodel.TenantAPIKey
	if err := db.Unscoped().First(&apiKey, "id = ?", apiKeyID).Error; err != nil {
		t.Fatal(err)
	}
	if apiKey.UsedQuota != 0 || apiKey.RemainQuota != 1000 {
		t.Fatalf("api key quota used/remain = %d/%d, want 0/1000", apiKey.UsedQuota, apiKey.RemainQuota)
	}
	var wallet ChannelWallet
	if err := db.First(&wallet, "channel_id = ?", channelID).Error; err != nil {
		t.Fatal(err)
	}
	if wallet.Balance != 100 || wallet.Status != channelWalletStatusActive {
		t.Fatalf("wallet balance/status = %d/%s, want 100/ACTIVE", wallet.Balance, wallet.Status)
	}
	var refundCount int64
	if err := db.Model(&ChannelWalletTransaction{}).
		Where("attempt_id = ? AND type = ?", requestID, channelWalletTxTypeRefund).
		Count(&refundCount).Error; err != nil {
		t.Fatal(err)
	}
	if refundCount != 1 {
		t.Fatalf("refund transaction count = %d, want 1", refundCount)
	}
	var attempt BillingAttempt
	if err := db.First(&attempt, "attempt_id = ?", requestID).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.Status != billingAttemptStatusCompensated {
		t.Fatalf("attempt status = %q, want %q", attempt.Status, billingAttemptStatusCompensated)
	}
	var entries []BillingAttemptEntry
	if err := db.Find(&entries, "attempt_id = ?", requestID).Error; err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}
	for _, entry := range entries {
		if entry.Status != billingEntryStatusRefunded || entry.ActualAmount != 17 || entry.RefundedAmount != 17 {
			t.Fatalf("entry = %#v", entry)
		}
	}
	var bill UsageBill
	if err := db.First(&bill, "attempt_id = ?", requestID).Error; err != nil {
		t.Fatal(err)
	}
	if bill.Status != usageBillStatusFailed || bill.PrivatePoints != 0 || bill.TotalPoints != 0 {
		t.Fatalf("usage bill status/private/total = %s/%d/%d", bill.Status, bill.PrivatePoints, bill.TotalPoints)
	}
}

func TestBillingServiceCompensatePrivateMusicDeliveryRejectsNonMusicAttempt(t *testing.T) {
	service, db, _, organizationID, _, _, requestID := newMusicCompensationFixture(t)
	if err := db.Model(&UsageBill{}).Where("attempt_id = ?", requestID).Update("pricing_snapshot", datatypes.JSON(`{"operation":"speech_generation","meter":"input_text","base_unit":"billed_character"}`)).Error; err != nil {
		t.Fatal(err)
	}

	err := service.CompensatePrivateMusicDelivery(t.Context(), organizationID, requestID)
	if !errors.Is(err, adapter.ErrMusicCompensationNotCharged) {
		t.Fatalf("CompensatePrivateMusicDelivery() error = %v, want ErrMusicCompensationNotCharged", err)
	}
	var wallet ChannelWallet
	if err := db.First(&wallet).Error; err != nil {
		t.Fatal(err)
	}
	if wallet.Balance != 83 {
		t.Fatalf("wallet balance = %d, want unchanged 83", wallet.Balance)
	}
}

func newMusicCompensationFixture(t *testing.T) (*BillingService, *gorm.DB, *BillingContext, uuid.UUID, string, uuid.UUID, string) {
	t.Helper()
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&apikeymodel.TenantAPIKey{},
		&BillingAttempt{},
		&BillingAttemptEntry{},
		&ChannelWallet{},
		&ChannelWalletTransaction{},
		&UsageBill{},
		&WorkspaceQuota{},
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX uq_music_compensation_entry ON billing_attempt_entries (attempt_id, entry_type, ledger_type)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE llm_routes (id text PRIMARY KEY, organization_id text NOT NULL, balance numeric NOT NULL DEFAULT 0, updated_at datetime, deleted_at datetime)`).Error; err != nil {
		t.Fatal(err)
	}

	organizationID := uuid.New()
	apiKeyID := uuid.NewString()
	channelID := uuid.New()
	requestID := uuid.NewString()
	quotaLimit := int64(1000)
	if err := db.Create(&apikeymodel.TenantAPIKey{
		ID:             apiKeyID,
		OrganizationID: organizationID.String(),
		Key:            "encrypted",
		Name:           "music-test",
		Status:         "active",
		QuotaLimit:     &quotaLimit,
		RemainQuota:    quotaLimit,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO llm_routes (id, organization_id, balance, updated_at) VALUES (?, ?, ?, ?)`, channelID.String(), organizationID.String(), 100, time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&ChannelWallet{ChannelID: channelID, OrganizationID: organizationID, Balance: 100, Status: channelWalletStatusActive}).Error; err != nil {
		t.Fatal(err)
	}

	service := &BillingService{db: db}
	billing := &BillingContext{
		APIKeyID:          apiKeyID,
		OrganizationID:    organizationID.String(),
		AttemptID:         requestID,
		RequestID:         requestID,
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    apiKeyID,
		ModelID:           uuid.New(),
		ModelName:         "music-3.0",
		ProviderID:        uuid.New(),
		ProviderName:      "minimax",
		ChannelID:         &channelID,
		BillingLane:       UsageBillingLanePrivate,
		UseSystemProvider: false,
		EstimatedCredits:  17,
		ActualCredits:     17,
		PricingOperation:  PricingOperationMusic,
		PricingSource:     PricingSourceUpstreamModelPrice,
		UsageSource:       UsageSourceRequestParameters,
		PricingSnapshot:   datatypes.JSON(`{"operation":"music_generation","meter":"output_track","base_unit":"track","quantity":1}`),
		Status:            billingContextStatusSuccess,
		RequestCreatedAt:  time.Now().Add(-time.Second),
	}
	if err := service.PreDeduct(t.Context(), billing); err != nil {
		t.Fatalf("PreDeduct() error = %v", err)
	}
	if err := service.Settle(t.Context(), billing); err != nil {
		t.Fatalf("Settle() error = %v", err)
	}
	return service, db, billing, organizationID, apiKeyID, channelID, requestID
}
