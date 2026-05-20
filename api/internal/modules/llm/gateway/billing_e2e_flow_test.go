package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/ginext/internal/modules/llm/apikey/model"
	"gorm.io/gorm"
)

func openGatewayE2ETestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := "file:gateway_e2e_" + uuid.NewString() + "?mode=memory&cache=shared&_loc=auto"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	return db
}

func migrateGatewayE2ETables(t *testing.T, db *gorm.DB) {
	t.Helper()

	if err := db.AutoMigrate(
		&BillingAttempt{},
		&BillingAttemptEntry{},
		&UsageBill{},
		&ChannelWallet{},
		&ChannelWalletTransaction{},
		&WorkspaceQuota{},
		&apikeymodel.TenantAPIKey{},
	); err != nil {
		t.Fatalf("failed to migrate e2e tables: %v", err)
	}

	// syncRouteBalanceSnapshot updates llm_routes during private lane billing.
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS llm_routes (
			id TEXT PRIMARY KEY,
			balance DECIMAL(15,4) NOT NULL DEFAULT 0,
			updated_at DATETIME,
			deleted_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("failed to create llm_routes table: %v", err)
	}

	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS uq_billing_attempt_entry
		ON billing_attempt_entries(attempt_id, entry_type, ledger_type)
	`).Error; err != nil {
		t.Fatalf("failed to create billing_attempt_entries unique index: %v", err)
	}
}

func seedAPIKey(t *testing.T, db *gorm.DB, keyID string, organizationID string) {
	t.Helper()

	apiKey := &apikeymodel.TenantAPIKey{
		ID:             keyID,
		OrganizationID: organizationID,
		Key:            "encrypted",
		KeyHash:        keyID + "-hash",
		Name:           "test-key",
		Status:         "active",
		UsedQuota:      0,
		RemainQuota:    0,
		QuotaLimit:     nil, // unlimited
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := db.Create(apiKey).Error; err != nil {
		t.Fatalf("failed to seed api key: %v", err)
	}
}

func seedPrivateWallet(t *testing.T, db *gorm.DB, channelID uuid.UUID, organizationID uuid.UUID, balance int64, status string) {
	t.Helper()

	if err := db.Exec(`INSERT INTO llm_routes(id, balance, updated_at) VALUES (?, ?, ?)`, channelID.String(), balance, time.Now()).Error; err != nil {
		t.Fatalf("failed to seed llm_route: %v", err)
	}

	wallet := &ChannelWallet{
		ChannelID:      channelID,
		OrganizationID: organizationID,
		Balance:        balance,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := db.Create(wallet).Error; err != nil {
		t.Fatalf("failed to seed channel wallet: %v", err)
	}
}

func TestBillingService_PrivateWalletDebt_BlocksNextPreDeduct(t *testing.T) {
	db := openGatewayE2ETestDB(t)
	migrateGatewayE2ETables(t, db)

	orgID := uuid.New()
	channelID := uuid.New()
	keyID := "key-debt-1"
	seedAPIKey(t, db, keyID, orgID.String())
	seedPrivateWallet(t, db, channelID, orgID, 5, channelWalletStatusActive)

	svc := &BillingService{db: db}
	bc := &BillingContext{
		APIKeyID:          keyID,
		OrganizationID:    orgID.String(),
		RequestID:         "req-debt-1",
		AttemptID:         "req-debt-1-a1",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    keyID,
		ChannelID:         &channelID,
		EstimatedCredits:  5,
		ActualCredits:     10,
		UseSystemProvider: false,
		Status:            "success",
	}

	if err := svc.PreDeduct(context.Background(), bc); err != nil {
		t.Fatalf("first PreDeduct returned error: %v", err)
	}
	if err := svc.Settle(context.Background(), bc); err != nil {
		t.Fatalf("first Settle returned error: %v", err)
	}

	var wallet ChannelWallet
	if err := db.Where("channel_id = ?", channelID).First(&wallet).Error; err != nil {
		t.Fatalf("failed to load wallet: %v", err)
	}
	if wallet.Status != channelWalletStatusDebt {
		t.Fatalf("wallet status = %s, want %s", wallet.Status, channelWalletStatusDebt)
	}
	if wallet.Balance >= 0 {
		t.Fatalf("wallet balance = %d, want negative", wallet.Balance)
	}

	bc2 := &BillingContext{
		APIKeyID:          keyID,
		OrganizationID:    orgID.String(),
		RequestID:         "req-debt-2",
		AttemptID:         "req-debt-2-a1",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    keyID,
		ChannelID:         &channelID,
		EstimatedCredits:  1,
		UseSystemProvider: false,
		Status:            "success",
	}
	err := svc.PreDeduct(context.Background(), bc2)
	if err == nil {
		t.Fatalf("expected second PreDeduct to fail for debt wallet")
	}
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("second PreDeduct err = %v, want %v", err, ErrInsufficientBalance)
	}
}

func TestBillingService_WorkspaceQuota_EndToEndRemainClampedToZeroBlocksNextPreDeduct(t *testing.T) {
	db := openGatewayE2ETestDB(t)
	migrateGatewayE2ETables(t, db)

	orgID := uuid.New()
	channelID := uuid.New()
	keyID := "key-ws-1"
	workspaceID := "ws-e2e-1"
	limit := int64(100)

	seedAPIKey(t, db, keyID, orgID.String())
	seedPrivateWallet(t, db, channelID, orgID, 100, channelWalletStatusActive)

	quota := &WorkspaceQuota{
		WorkspaceID:    workspaceID,
		OrganizationID: orgID,
		UsedQuota:      0,
		RemainQuota:    5,
		QuotaLimit:     &limit,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := db.Create(quota).Error; err != nil {
		t.Fatalf("failed to seed workspace quota: %v", err)
	}

	svc := &BillingService{db: db}
	bc := &BillingContext{
		APIKeyID:          keyID,
		OrganizationID:    orgID.String(),
		RequestID:         "req-ws-1",
		AttemptID:         "req-ws-1-a1",
		QuotaSubjectType:  quotaSubjectTypeWorkspace,
		QuotaSubjectID:    workspaceID,
		ChannelID:         &channelID,
		EstimatedCredits:  5,
		ActualCredits:     8, // actual > estimated
		UseSystemProvider: false,
		Status:            "success",
	}

	if err := svc.PreDeduct(context.Background(), bc); err != nil {
		t.Fatalf("workspace PreDeduct returned error: %v", err)
	}
	if err := svc.Settle(context.Background(), bc); err != nil {
		t.Fatalf("workspace Settle returned error: %v", err)
	}

	var got WorkspaceQuota
	if err := db.Where("workspace_id = ?", workspaceID).First(&got).Error; err != nil {
		t.Fatalf("failed to load workspace quota: %v", err)
	}
	if got.RemainQuota != 0 {
		t.Fatalf("remain_quota = %d, want 0", got.RemainQuota)
	}
	if got.UsedQuota != 8 {
		t.Fatalf("used_quota = %d, want 8", got.UsedQuota)
	}

	bc2 := &BillingContext{
		APIKeyID:          keyID,
		OrganizationID:    orgID.String(),
		RequestID:         "req-ws-2",
		AttemptID:         "req-ws-2-a1",
		QuotaSubjectType:  quotaSubjectTypeWorkspace,
		QuotaSubjectID:    workspaceID,
		ChannelID:         &channelID,
		EstimatedCredits:  1,
		UseSystemProvider: false,
		Status:            "success",
	}
	err := svc.PreDeduct(context.Background(), bc2)
	if err == nil {
		t.Fatalf("expected second workspace PreDeduct to fail")
	}
	if !errors.Is(err, ErrInsufficientQuota) {
		t.Fatalf("second workspace PreDeduct err = %v, want %v", err, ErrInsufficientQuota)
	}
}

func TestBillingService_PrivateWalletOrganizationMismatch_FailsPreDeduct(t *testing.T) {
	db := openGatewayE2ETestDB(t)
	migrateGatewayE2ETables(t, db)

	walletOrgID := uuid.New()
	requestOrgID := uuid.New()
	channelID := uuid.New()
	keyID := "key-wallet-mismatch-1"

	seedAPIKey(t, db, keyID, requestOrgID.String())
	seedPrivateWallet(t, db, channelID, walletOrgID, 100, channelWalletStatusActive)

	svc := &BillingService{db: db}
	bc := &BillingContext{
		APIKeyID:          keyID,
		OrganizationID:    requestOrgID.String(),
		RequestID:         "req-wallet-mismatch",
		AttemptID:         "req-wallet-mismatch-a1",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    keyID,
		ChannelID:         &channelID,
		EstimatedCredits:  10,
		UseSystemProvider: false,
		Status:            "success",
	}

	err := svc.PreDeduct(context.Background(), bc)
	if err == nil {
		t.Fatalf("expected PreDeduct to fail on wallet organization mismatch")
	}
	if !strings.Contains(err.Error(), "channel wallet organization mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBillingService_PrivateWalletPreDeduct_RequiresRouteSnapshotUpdate(t *testing.T) {
	db := openGatewayE2ETestDB(t)
	migrateGatewayE2ETables(t, db)

	orgID := uuid.New()
	channelID := uuid.New()
	keyID := "key-route-snapshot-1"

	seedAPIKey(t, db, keyID, orgID.String())
	// Intentionally create wallet without llm_routes row to force snapshot update RowsAffected=0.
	wallet := &ChannelWallet{
		ChannelID:      channelID,
		OrganizationID: orgID,
		Balance:        100,
		Status:         channelWalletStatusActive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := db.Create(wallet).Error; err != nil {
		t.Fatalf("failed to seed channel wallet: %v", err)
	}

	svc := &BillingService{db: db}
	bc := &BillingContext{
		APIKeyID:          keyID,
		OrganizationID:    orgID.String(),
		RequestID:         "req-route-snapshot",
		AttemptID:         "req-route-snapshot-a1",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    keyID,
		ChannelID:         &channelID,
		EstimatedCredits:  10,
		UseSystemProvider: false,
		Status:            "success",
	}

	err := svc.PreDeduct(context.Background(), bc)
	if err == nil {
		t.Fatalf("expected PreDeduct to fail when route snapshot row is missing")
	}
	if !strings.Contains(err.Error(), "sync route balance snapshot") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBillingService_PrivateSettle_Idempotent_NoDoubleDeduct(t *testing.T) {
	db := openGatewayE2ETestDB(t)
	migrateGatewayE2ETables(t, db)

	orgID := uuid.New()
	channelID := uuid.New()
	keyID := "key-idempotent-1"

	seedAPIKey(t, db, keyID, orgID.String())
	seedPrivateWallet(t, db, channelID, orgID, 100, channelWalletStatusActive)

	svc := &BillingService{db: db}
	bc := &BillingContext{
		APIKeyID:          keyID,
		OrganizationID:    orgID.String(),
		RequestID:         "req-idempotent",
		AttemptID:         "req-idempotent-a1",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    keyID,
		ChannelID:         &channelID,
		EstimatedCredits:  10,
		ActualCredits:     6,
		UseSystemProvider: false,
		Status:            "success",
	}

	if err := svc.PreDeduct(context.Background(), bc); err != nil {
		t.Fatalf("PreDeduct returned error: %v", err)
	}
	if err := svc.Settle(context.Background(), bc); err != nil {
		t.Fatalf("first Settle returned error: %v", err)
	}
	if err := svc.Settle(context.Background(), bc); err != nil {
		t.Fatalf("second Settle returned error: %v", err)
	}

	var wallet ChannelWallet
	if err := db.Where("channel_id = ?", channelID).First(&wallet).Error; err != nil {
		t.Fatalf("failed to load wallet: %v", err)
	}
	// 100 - 10 + (10 - 6) = 94; idempotent settle should keep it at 94.
	if wallet.Balance != 94 {
		t.Fatalf("wallet balance = %d, want 94", wallet.Balance)
	}

	var apiKey apikeymodel.TenantAPIKey
	if err := db.Where("id = ?", keyID).First(&apiKey).Error; err != nil {
		t.Fatalf("failed to load api key: %v", err)
	}
	if apiKey.UsedQuota != 6 {
		t.Fatalf("used_quota = %d, want 6", apiKey.UsedQuota)
	}

	var entries []BillingAttemptEntry
	if err := db.Where("attempt_id = ?", bc.AttemptID).Find(&entries).Error; err != nil {
		t.Fatalf("failed to load attempt entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries count = %d, want 2", len(entries))
	}
	for _, entry := range entries {
		if entry.Status != billingEntryStatusSettled {
			t.Fatalf("entry %s/%s status = %s, want %s", entry.EntryType, entry.LedgerType, entry.Status, billingEntryStatusSettled)
		}
	}
}

func TestBillingService_RecoverStaleLocalPredeductAttempts_AutoRollback(t *testing.T) {
	db := openGatewayE2ETestDB(t)
	migrateGatewayE2ETables(t, db)

	orgID := uuid.New()
	channelID := uuid.New()
	keyID := "key-local-recover-1"
	limit := int64(100)
	apiKey := &apikeymodel.TenantAPIKey{
		ID:             keyID,
		OrganizationID: orgID.String(),
		Key:            "encrypted",
		KeyHash:        keyID + "-hash",
		Name:           "recover-key",
		Status:         "active",
		UsedQuota:      0,
		RemainQuota:    100,
		QuotaLimit:     &limit,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := db.Create(apiKey).Error; err != nil {
		t.Fatalf("failed to seed api key: %v", err)
	}
	seedPrivateWallet(t, db, channelID, orgID, 100, channelWalletStatusActive)

	svc := &BillingService{db: db}
	bc := &BillingContext{
		APIKeyID:          keyID,
		OrganizationID:    orgID.String(),
		RequestID:         "req-local-recover",
		AttemptID:         "req-local-recover-a1",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    keyID,
		ChannelID:         &channelID,
		EstimatedCredits:  10,
		UseSystemProvider: false,
		Status:            "success",
	}
	if err := svc.PreDeduct(context.Background(), bc); err != nil {
		t.Fatalf("PreDeduct returned error: %v", err)
	}
	// Simulate crash-gap: pre-deduct succeeded but settle never arrived.
	staleTime := time.Now().Add(-(defaultLocalPredeductTimeout + time.Minute))
	if err := db.Model(&BillingAttempt{}).
		Where("attempt_id = ?", bc.AttemptID).
		Update("updated_at", staleTime).Error; err != nil {
		t.Fatalf("failed to backdate attempt: %v", err)
	}

	if err := svc.recoverStaleLocalPredeductAttempts(context.Background()); err != nil {
		t.Fatalf("recoverStaleLocalPredeductAttempts returned error: %v", err)
	}

	var attempt BillingAttempt
	if err := db.Where("attempt_id = ?", bc.AttemptID).First(&attempt).Error; err != nil {
		t.Fatalf("failed to load attempt: %v", err)
	}
	if attempt.Status != billingAttemptStatusRolledBack {
		t.Fatalf("attempt status = %s, want %s", attempt.Status, billingAttemptStatusRolledBack)
	}

	var wallet ChannelWallet
	if err := db.Where("channel_id = ?", channelID).First(&wallet).Error; err != nil {
		t.Fatalf("failed to load wallet: %v", err)
	}
	if wallet.Balance != 100 {
		t.Fatalf("wallet balance = %d, want 100 after recovery rollback", wallet.Balance)
	}

	var gotKey apikeymodel.TenantAPIKey
	if err := db.Where("id = ?", keyID).First(&gotKey).Error; err != nil {
		t.Fatalf("failed to load api key: %v", err)
	}
	if gotKey.RemainQuota != 100 {
		t.Fatalf("remain_quota = %d, want 100 after recovery rollback", gotKey.RemainQuota)
	}
	if gotKey.UsedQuota != 0 {
		t.Fatalf("used_quota = %d, want 0 after recovery rollback", gotKey.UsedQuota)
	}
}

func TestBillingService_RecoverStaleLocalPredeductAttempts_DoesNotTouchFreshAttempt(t *testing.T) {
	db := openGatewayE2ETestDB(t)
	migrateGatewayE2ETables(t, db)

	orgID := uuid.New()
	channelID := uuid.New()
	keyID := "key-local-fresh-1"
	limit := int64(100)
	apiKey := &apikeymodel.TenantAPIKey{
		ID:             keyID,
		OrganizationID: orgID.String(),
		Key:            "encrypted",
		KeyHash:        keyID + "-hash",
		Name:           "fresh-key",
		Status:         "active",
		UsedQuota:      0,
		RemainQuota:    100,
		QuotaLimit:     &limit,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := db.Create(apiKey).Error; err != nil {
		t.Fatalf("failed to seed api key: %v", err)
	}
	seedPrivateWallet(t, db, channelID, orgID, 100, channelWalletStatusActive)

	svc := &BillingService{db: db}
	bc := &BillingContext{
		APIKeyID:          keyID,
		OrganizationID:    orgID.String(),
		RequestID:         "req-local-fresh",
		AttemptID:         "req-local-fresh-a1",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    keyID,
		ChannelID:         &channelID,
		EstimatedCredits:  10,
		UseSystemProvider: false,
		Status:            "success",
	}
	if err := svc.PreDeduct(context.Background(), bc); err != nil {
		t.Fatalf("PreDeduct returned error: %v", err)
	}

	if err := svc.recoverStaleLocalPredeductAttempts(context.Background()); err != nil {
		t.Fatalf("recoverStaleLocalPredeductAttempts returned error: %v", err)
	}

	var attempt BillingAttempt
	if err := db.Where("attempt_id = ?", bc.AttemptID).First(&attempt).Error; err != nil {
		t.Fatalf("failed to load attempt: %v", err)
	}
	if attempt.Status != billingAttemptStatusPre {
		t.Fatalf("fresh attempt status = %s, want %s", attempt.Status, billingAttemptStatusPre)
	}
}

func TestBillingService_RecoverStaleLocalPredeductAttempts_ChannelMismatch_SchedulesRetry(t *testing.T) {
	db := openGatewayE2ETestDB(t)
	migrateGatewayE2ETables(t, db)

	orgID := uuid.New()
	channelID := uuid.New()
	keyID := "key-local-mismatch-1"
	limit := int64(100)
	apiKey := &apikeymodel.TenantAPIKey{
		ID:             keyID,
		OrganizationID: orgID.String(),
		Key:            "encrypted",
		KeyHash:        keyID + "-hash",
		Name:           "mismatch-key",
		Status:         "active",
		UsedQuota:      0,
		RemainQuota:    100,
		QuotaLimit:     &limit,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := db.Create(apiKey).Error; err != nil {
		t.Fatalf("failed to seed api key: %v", err)
	}
	seedPrivateWallet(t, db, channelID, orgID, 100, channelWalletStatusActive)

	svc := &BillingService{db: db}
	bc := &BillingContext{
		APIKeyID:          keyID,
		OrganizationID:    orgID.String(),
		RequestID:         "req-local-mismatch",
		AttemptID:         "req-local-mismatch-a1",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    keyID,
		ChannelID:         &channelID,
		EstimatedCredits:  10,
		UseSystemProvider: false,
		Status:            "success",
	}
	if err := svc.PreDeduct(context.Background(), bc); err != nil {
		t.Fatalf("PreDeduct returned error: %v", err)
	}

	badRouteID := uuid.New()
	staleTime := time.Now().Add(-(defaultLocalPredeductTimeout + time.Minute))
	if err := db.Model(&BillingAttempt{}).
		Where("attempt_id = ?", bc.AttemptID).
		Updates(map[string]any{
			"route_id":      badRouteID,
			"updated_at":    staleTime,
			"status":        billingAttemptStatusPre,
			"error_code":    nil,
			"error_message": nil,
		}).Error; err != nil {
		t.Fatalf("failed to tamper attempt route_id: %v", err)
	}

	if err := svc.recoverStaleLocalPredeductAttempts(context.Background()); err != nil {
		t.Fatalf("recoverStaleLocalPredeductAttempts returned error: %v", err)
	}

	var attempt BillingAttempt
	if err := db.Where("attempt_id = ?", bc.AttemptID).First(&attempt).Error; err != nil {
		t.Fatalf("failed to load attempt: %v", err)
	}
	if attempt.Status != billingAttemptStatusPartial {
		t.Fatalf("status = %s, want %s", attempt.Status, billingAttemptStatusPartial)
	}
	if attempt.NextReconcileAt == nil {
		t.Fatalf("next_reconcile_at should be set for local retry")
	}
	if attempt.ErrorCode == nil || *attempt.ErrorCode != "LOCAL_PREDEDUCT_RECOVERY_FAILED" {
		t.Fatalf("error_code = %v, want LOCAL_PREDEDUCT_RECOVERY_FAILED", attempt.ErrorCode)
	}
}

func TestBillingService_RecoverStaleLocalPredeductAttempts_RecoversFromPartialRetry(t *testing.T) {
	db := openGatewayE2ETestDB(t)
	migrateGatewayE2ETables(t, db)

	orgID := uuid.New()
	channelID := uuid.New()
	keyID := "key-local-partial-retry-1"
	limit := int64(100)
	apiKey := &apikeymodel.TenantAPIKey{
		ID:             keyID,
		OrganizationID: orgID.String(),
		Key:            "encrypted",
		KeyHash:        keyID + "-hash",
		Name:           "partial-retry-key",
		Status:         "active",
		UsedQuota:      0,
		RemainQuota:    100,
		QuotaLimit:     &limit,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := db.Create(apiKey).Error; err != nil {
		t.Fatalf("failed to seed api key: %v", err)
	}
	seedPrivateWallet(t, db, channelID, orgID, 100, channelWalletStatusActive)

	svc := &BillingService{db: db}
	bc := &BillingContext{
		APIKeyID:          keyID,
		OrganizationID:    orgID.String(),
		RequestID:         "req-local-partial-retry",
		AttemptID:         "req-local-partial-retry-a1",
		QuotaSubjectType:  quotaSubjectTypeAPIKey,
		QuotaSubjectID:    keyID,
		ChannelID:         &channelID,
		EstimatedCredits:  10,
		UseSystemProvider: false,
		Status:            "success",
	}
	if err := svc.PreDeduct(context.Background(), bc); err != nil {
		t.Fatalf("PreDeduct returned error: %v", err)
	}

	if err := db.Model(&BillingAttempt{}).
		Where("attempt_id = ?", bc.AttemptID).
		Updates(map[string]any{
			"status":             billingAttemptStatusPartial,
			"reconcile_attempts": 1,
			"next_reconcile_at":  time.Now().Add(-time.Second),
		}).Error; err != nil {
		t.Fatalf("failed to force partial retry state: %v", err)
	}

	if err := svc.recoverStaleLocalPredeductAttempts(context.Background()); err != nil {
		t.Fatalf("recoverStaleLocalPredeductAttempts returned error: %v", err)
	}

	var attempt BillingAttempt
	if err := db.Where("attempt_id = ?", bc.AttemptID).First(&attempt).Error; err != nil {
		t.Fatalf("failed to load attempt: %v", err)
	}
	if attempt.Status != billingAttemptStatusRolledBack {
		t.Fatalf("status = %s, want %s", attempt.Status, billingAttemptStatusRolledBack)
	}
}
