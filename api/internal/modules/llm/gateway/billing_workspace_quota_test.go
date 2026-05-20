package gateway

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openWorkspaceQuotaTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := "file:gateway_workspace_quota_" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skip("sqlite driver requires cgo in this environment")
		}
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&ChannelWallet{}); err != nil {
		t.Fatalf("failed to migrate channel wallet: %v", err)
	}
	return db
}

func TestBillingService_PreDeductWorkspaceQuota_InsufficientRemain(t *testing.T) {
	db := openWorkspaceQuotaTestDB(t)
	if err := db.AutoMigrate(&WorkspaceQuota{}); err != nil {
		t.Fatalf("failed to migrate workspace quota: %v", err)
	}

	orgID := uuid.New()
	limit := int64(100)
	record := &WorkspaceQuota{
		WorkspaceID:    "ws-1",
		OrganizationID: orgID,
		UsedQuota:      10,
		RemainQuota:    5,
		QuotaLimit:     &limit,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("failed to seed workspace quota: %v", err)
	}

	svc := &BillingService{db: db}
	bc := &BillingContext{
		OrganizationID:   orgID.String(),
		QuotaSubjectType: quotaSubjectTypeWorkspace,
		QuotaSubjectID:   "ws-1",
		EstimatedCredits: 10,
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		return svc.preDeductWorkspaceQuota(context.Background(), tx, bc)
	})
	if err != ErrInsufficientQuota {
		t.Fatalf("preDeductWorkspaceQuota err = %v, want %v", err, ErrInsufficientQuota)
	}
}

func TestBillingService_SettleWorkspaceQuota_ActualGreaterThanEstimated_ClampsRemainToZero(t *testing.T) {
	db := openWorkspaceQuotaTestDB(t)
	if err := db.AutoMigrate(&WorkspaceQuota{}); err != nil {
		t.Fatalf("failed to migrate workspace quota: %v", err)
	}

	orgID := uuid.New()
	limit := int64(100)
	record := &WorkspaceQuota{
		WorkspaceID:    "ws-negative",
		OrganizationID: orgID,
		UsedQuota:      20,
		RemainQuota:    3, // pre-deducted state
		QuotaLimit:     &limit,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("failed to seed workspace quota: %v", err)
	}

	svc := &BillingService{db: db}
	bc := &BillingContext{
		OrganizationID:   orgID.String(),
		QuotaSubjectType: quotaSubjectTypeWorkspace,
		QuotaSubjectID:   "ws-negative",
		EstimatedCredits: 3,
		ActualCredits:    8,
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		return svc.settleWorkspaceQuota(context.Background(), tx, bc)
	}); err != nil {
		t.Fatalf("settleWorkspaceQuota returned error: %v", err)
	}

	var got WorkspaceQuota
	if err := db.Where("workspace_id = ?", "ws-negative").First(&got).Error; err != nil {
		t.Fatalf("failed to load workspace quota: %v", err)
	}

	// diff = estimated - actual = -5, remain 3 + (-5) would be negative, but must be clamped to 0.
	if got.RemainQuota != 0 {
		t.Fatalf("remain_quota = %d, want 0", got.RemainQuota)
	}
	if got.UsedQuota != 28 {
		t.Fatalf("used_quota = %d, want 28", got.UsedQuota)
	}
}

func TestBillingService_GetOrCreateWorkspaceQuotaForUpdate_OrganizationMismatch(t *testing.T) {
	db := openWorkspaceQuotaTestDB(t)
	if err := db.AutoMigrate(&WorkspaceQuota{}); err != nil {
		t.Fatalf("failed to migrate workspace quota: %v", err)
	}

	orgA := uuid.New()
	orgB := uuid.New()
	record := &WorkspaceQuota{
		WorkspaceID:    "ws-mismatch",
		OrganizationID: orgA,
		UsedQuota:      0,
		RemainQuota:    0,
		QuotaLimit:     nil,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("failed to seed workspace quota: %v", err)
	}

	svc := &BillingService{db: db}
	err := db.Transaction(func(tx *gorm.DB) error {
		_, err := svc.getOrCreateWorkspaceQuotaForUpdate(context.Background(), tx, "ws-mismatch", orgB)
		return err
	})
	if err == nil {
		t.Fatalf("expected organization mismatch error")
	}
	if !strings.Contains(err.Error(), "workspace quota organization mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBillingService_PreDeductOrganizationSubject_IsNoOp(t *testing.T) {
	db := openWorkspaceQuotaTestDB(t)
	svc := &BillingService{db: db}
	orgID := uuid.New()

	bc := &BillingContext{
		OrganizationID:   orgID.String(),
		QuotaSubjectType: quotaSubjectTypeOrganization,
		QuotaSubjectID:   orgID.String(),
		EstimatedCredits: 10,
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		return svc.preDeductSubjectQuota(context.Background(), tx, bc, nil)
	}); err != nil {
		t.Fatalf("preDeductSubjectQuota returned error: %v", err)
	}

	if db.Migrator().HasTable(&WorkspaceQuota{}) {
		var count int64
		if err := db.Model(&WorkspaceQuota{}).Count(&count).Error; err == nil && count != 0 {
			t.Fatalf("workspace quota rows = %d, want 0", count)
		}
	}
}

func TestBillingService_SettleOrganizationSubject_IsNoOp(t *testing.T) {
	db := openWorkspaceQuotaTestDB(t)
	svc := &BillingService{db: db}
	orgID := uuid.New()

	bc := &BillingContext{
		OrganizationID:   orgID.String(),
		QuotaSubjectType: quotaSubjectTypeOrganization,
		QuotaSubjectID:   orgID.String(),
		EstimatedCredits: 10,
		ActualCredits:    7,
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		return svc.settleSubjectQuota(context.Background(), tx, bc)
	}); err != nil {
		t.Fatalf("settleSubjectQuota returned error: %v", err)
	}

	if db.Migrator().HasTable(&WorkspaceQuota{}) {
		var count int64
		if err := db.Model(&WorkspaceQuota{}).Count(&count).Error; err == nil && count != 0 {
			t.Fatalf("workspace quota rows = %d, want 0", count)
		}
	}
}
