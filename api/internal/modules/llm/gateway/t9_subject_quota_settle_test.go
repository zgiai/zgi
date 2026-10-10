package gateway

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/millipoints"
)

// T9 acceptance 1 and 3 at the shared quota layer: the local subject is
// pre-deducted first, settlement refunds or charges back the difference, and the
// same attempt_id is never deducted twice.
type t9BillingFixture struct {
	service   *BillingService
	db        *gorm.DB
	orgID     uuid.UUID
	apiKeyID  string
	channelID uuid.UUID
	quota     int64
	wallet    int64
}

func newT9BillingFixture(t *testing.T) *t9BillingFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
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
		t.Fatalf("automigrate billing tables: %v", err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX uq_t9_billing_entry ON billing_attempt_entries (attempt_id, entry_type, ledger_type)").Error; err != nil {
		t.Fatalf("create billing entry index: %v", err)
	}
	if err := db.Exec("CREATE TABLE llm_routes (id text PRIMARY KEY, organization_id text NOT NULL, balance numeric NOT NULL DEFAULT 0, updated_at datetime, deleted_at datetime)").Error; err != nil {
		t.Fatalf("create llm_routes: %v", err)
	}
	if err := db.Exec("CREATE TABLE organizations (id text PRIMARY KEY, billing_display_currency text NOT NULL, usd_to_cny_rate text NOT NULL)").Error; err != nil {
		t.Fatalf("create organizations: %v", err)
	}

	orgID := uuid.New()
	if err := db.Exec("INSERT INTO organizations (id, billing_display_currency, usd_to_cny_rate) VALUES (?, ?, ?)",
		orgID.String(), "USD", "7.2").Error; err != nil {
		t.Fatalf("seed organization: %v", err)
	}

	quota := int64(100)
	apiKeyID := uuid.NewString()
	if err := db.Create(&apikeymodel.TenantAPIKey{
		ID:             apiKeyID,
		OrganizationID: orgID.String(),
		Key:            "encrypted",
		Name:           "t9-subject-quota",
		Status:         "active",
		QuotaLimit:     &quota,
		RemainQuota:    quota,
	}).Error; err != nil {
		t.Fatalf("seed api key: %v", err)
	}

	channelID := uuid.New()
	if err := db.Exec("INSERT INTO llm_routes (id, organization_id, balance, updated_at) VALUES (?, ?, ?, ?)",
		channelID.String(), orgID.String(), 200, time.Now()).Error; err != nil {
		t.Fatalf("seed route: %v", err)
	}
	if err := db.Create(&ChannelWallet{
		ChannelID:      channelID,
		OrganizationID: orgID,
		Balance:        200,
		Status:         channelWalletStatusActive,
	}).Error; err != nil {
		t.Fatalf("seed channel wallet: %v", err)
	}

	return &t9BillingFixture{
		service:   &BillingService{db: db},
		db:        db,
		orgID:     orgID,
		apiKeyID:  apiKeyID,
		channelID: channelID,
		quota:     quota,
		wallet:    200,
	}
}

func (f *t9BillingFixture) billingContext(attemptID string, estimated int64) *BillingContext {
	return &BillingContext{
		APIKeyID:          f.apiKeyID,
		OrganizationID:    f.orgID.String(),
		AttemptID:         attemptID,
		RequestID:         attemptID,
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    f.apiKeyID,
		ChannelID:         &f.channelID,
		BillingLane:       UsageBillingLanePrivate,
		UseSystemProvider: false,
		EstimatedCredits:  estimated,
		ModelID:           uuid.New(),
		ModelName:         "t9-model",
		ProviderID:        uuid.New(),
		ProviderName:      "minimax",
		PricingOperation:  PricingOperationMusic,
		PricingSource:     PricingSourceUpstreamModelPrice,
		UsageSource:       UsageSourceRequestParameters,
		PricingSnapshot:   datatypes.JSON([]byte("{\"operation\":\"music_generation\",\"meter\":\"output_track\",\"base_unit\":\"track\",\"quantity\":1}")),
		RequestCreatedAt:  time.Now().Add(-time.Second),
	}
}

func (f *t9BillingFixture) assertAPIKeyQuota(t *testing.T, wantUsed, wantRemain int64) {
	t.Helper()
	var apiKey apikeymodel.TenantAPIKey
	if err := f.db.First(&apiKey, "id = ?", f.apiKeyID).Error; err != nil {
		t.Fatalf("load api key: %v", err)
	}
	if apiKey.UsedQuota != wantUsed || apiKey.RemainQuota != wantRemain {
		t.Fatalf("api key used/remain = %d/%d, want %d/%d", apiKey.UsedQuota, apiKey.RemainQuota, wantUsed, wantRemain)
	}
}

func (f *t9BillingFixture) assertWalletBalance(t *testing.T, want int64) {
	t.Helper()
	var wallet ChannelWallet
	if err := f.db.First(&wallet, "channel_id = ?", f.channelID).Error; err != nil {
		t.Fatalf("load channel wallet: %v", err)
	}
	if int64(wallet.Balance) != want {
		t.Fatalf("channel wallet balance = %d, want %d", wallet.Balance, want)
	}
}

func (f *t9BillingFixture) countWalletTransactions(t *testing.T, attemptID, txType string) int64 {
	t.Helper()
	var count int64
	if err := f.db.Model(&ChannelWalletTransaction{}).
		Where("attempt_id = ? AND type = ?", attemptID, txType).
		Count(&count).Error; err != nil {
		t.Fatalf("count %s transactions: %v", txType, err)
	}
	return count
}

// The same attempt_id must never settle twice: the second settle is a no-op and
// the subject plus channel wallet keep the first settlement's amounts.
func TestT9APIKeySubjectSettleRefundsAndIsIdempotent(t *testing.T) {
	fixture := newT9BillingFixture(t)
	bc := fixture.billingContext("t9-key-refund-a1", 30)

	if err := fixture.service.PreDeduct(t.Context(), bc); err != nil {
		t.Fatalf("PreDeduct() error = %v", err)
	}
	fixture.assertAPIKeyQuota(t, 0, fixture.quota-30)
	fixture.assertWalletBalance(t, fixture.wallet-30)

	bc.ActualCredits = 12
	bc.Status = billingContextStatusSuccess
	if err := fixture.service.Settle(t.Context(), bc); err != nil {
		t.Fatalf("Settle() error = %v", err)
	}
	fixture.assertAPIKeyQuota(t, 12, fixture.quota-12)
	fixture.assertWalletBalance(t, fixture.wallet-12)

	// A replay of the same attempt must not deduct again even if the caller
	// reports a different actual amount.
	bc.ActualCredits = 99
	if err := fixture.service.Settle(t.Context(), bc); err != nil {
		t.Fatalf("second Settle() error = %v", err)
	}
	fixture.assertAPIKeyQuota(t, 12, fixture.quota-12)
	fixture.assertWalletBalance(t, fixture.wallet-12)

	var attempt BillingAttempt
	if err := fixture.db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatalf("load attempt: %v", err)
	}
	if attempt.Status != billingAttemptStatusSettled {
		t.Fatalf("attempt status = %q, want %q", attempt.Status, billingAttemptStatusSettled)
	}
	var subjectEntry BillingAttemptEntry
	if err := fixture.db.First(&subjectEntry, "attempt_id = ? AND entry_type = ?", bc.AttemptID, billingEntryTypeSubject).Error; err != nil {
		t.Fatalf("load subject entry: %v", err)
	}
	if int64(subjectEntry.ActualAmount) != 12 || int64(subjectEntry.RefundedAmount) != 18 {
		t.Fatalf("subject entry actual/refunded = %d/%d, want 12/18", subjectEntry.ActualAmount, subjectEntry.RefundedAmount)
	}
	if got := fixture.countWalletTransactions(t, bc.AttemptID, channelWalletTxTypePreDeduct); got != 1 {
		t.Fatalf("wallet prededuct transactions = %d, want 1", got)
	}
	if got := fixture.countWalletTransactions(t, bc.AttemptID, channelWalletTxTypeRefund); got != 1 {
		t.Fatalf("wallet refund transactions = %d, want 1", got)
	}
}

// Actual usage above the reservation charges the difference back to the subject.
func TestT9APIKeySubjectSettleChargesBackAboveEstimate(t *testing.T) {
	fixture := newT9BillingFixture(t)
	bc := fixture.billingContext("t9-key-chargeback-a1", 10)

	if err := fixture.service.PreDeduct(t.Context(), bc); err != nil {
		t.Fatalf("PreDeduct() error = %v", err)
	}
	bc.ActualCredits = 14
	bc.Status = billingContextStatusSuccess
	if err := fixture.service.Settle(t.Context(), bc); err != nil {
		t.Fatalf("Settle() error = %v", err)
	}

	fixture.assertAPIKeyQuota(t, 14, fixture.quota-14)
	fixture.assertWalletBalance(t, fixture.wallet-14)
	if got := fixture.countWalletTransactions(t, bc.AttemptID, channelWalletTxTypeSettleAdjustment); got != 1 {
		t.Fatalf("wallet settle adjustment transactions = %d, want 1", got)
	}
}

// Workspace subject quota follows the same pre-deduct then refund contract and
// is idempotent per attempt_id.
func TestT9WorkspaceSubjectSettleRefundsAndIsIdempotent(t *testing.T) {
	fixture := newT9BillingFixture(t)
	workspaceID := uuid.NewString()
	limit := millipoints.MilliPoints(100)
	if err := fixture.db.Create(&WorkspaceQuota{
		WorkspaceID:    workspaceID,
		OrganizationID: fixture.orgID,
		RemainQuota:    limit,
		UsedQuota:      0,
		QuotaLimit:     &limit,
	}).Error; err != nil {
		t.Fatalf("seed workspace quota: %v", err)
	}

	bc := fixture.billingContext("t9-workspace-refund-a1", 30)
	bc.QuotaSubjectType = quotaSubjectTypeWorkspace
	bc.QuotaSubjectID = workspaceID
	bc.WorkspaceID = workspaceID

	if err := fixture.service.PreDeduct(t.Context(), bc); err != nil {
		t.Fatalf("PreDeduct() error = %v", err)
	}
	bc.ActualCredits = 12
	bc.Status = billingContextStatusSuccess
	if err := fixture.service.Settle(t.Context(), bc); err != nil {
		t.Fatalf("Settle() error = %v", err)
	}
	// Replaying the same attempt must not touch the workspace quota again.
	if err := fixture.service.Settle(t.Context(), bc); err != nil {
		t.Fatalf("second Settle() error = %v", err)
	}

	var quota WorkspaceQuota
	if err := fixture.db.First(&quota, "workspace_id = ?", workspaceID).Error; err != nil {
		t.Fatalf("load workspace quota: %v", err)
	}
	if int64(quota.UsedQuota) != 12 || int64(quota.RemainQuota) != 88 {
		t.Fatalf("workspace quota used/remain = %d/%d, want 12/88", quota.UsedQuota, quota.RemainQuota)
	}
}
