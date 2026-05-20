package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"

	quota_model "github.com/zgiai/ginext/internal/modules/quota/model"
	quota_repo "github.com/zgiai/ginext/internal/modules/quota/repository"
)

func newQuotaServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_loc=auto", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&quota_model.QuotaUsageHistory{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	return db
}

func TestCheckQuota_AlwaysAllowsAndReturnsCurrentUsage(t *testing.T) {
	db := newQuotaServiceTestDB(t)

	svc := NewQuotaService(quota_repo.NewQuotaRepository(db), db)
	groupID := uuid.New()
	usageDelta := int64(2 * 1024 * 1024)

	if err := svc.RecordUsage(context.Background(), &quota_model.QuotaUsageHistory{
		ID:           uuid.NewString(),
		GroupID:      groupID,
		AccountID:    uuid.New(),
		ResourceType: quota_model.ResourceTypeStorage,
		Delta:        usageDelta,
	}); err != nil {
		t.Fatalf("RecordUsage returned error: %v", err)
	}

	canProceed, currentUsage, limit, err := svc.CheckQuota(context.Background(), groupID, quota_model.ResourceTypeStorage, 2*1024*1024)
	if err != nil {
		t.Fatalf("CheckQuota returned error: %v", err)
	}
	if !canProceed {
		t.Fatalf("expected quota check to pass")
	}
	if currentUsage != usageDelta {
		t.Fatalf("expected current usage %d, got %d", usageDelta, currentUsage)
	}
	if limit != -1 {
		t.Fatalf("expected unlimited limit -1, got %d", limit)
	}
}

func TestGetQuotaStatus_ReturnsUnlimitedPlanNoneWithUsage(t *testing.T) {
	db := newQuotaServiceTestDB(t)

	svc := NewQuotaService(quota_repo.NewQuotaRepository(db), db)
	groupID := uuid.New()
	usageDelta := int64(8 * 1024 * 1024)

	if err := svc.RecordUsage(context.Background(), &quota_model.QuotaUsageHistory{
		ID:           uuid.NewString(),
		GroupID:      groupID,
		AccountID:    uuid.New(),
		ResourceType: quota_model.ResourceTypeStorage,
		Delta:        usageDelta,
	}); err != nil {
		t.Fatalf("RecordUsage returned error: %v", err)
	}

	status, err := svc.GetQuotaStatus(context.Background(), groupID)
	if err != nil {
		t.Fatalf("GetQuotaStatus returned error: %v", err)
	}
	if status.PlanCode != "none" {
		t.Fatalf("expected plan code none, got %s", status.PlanCode)
	}

	storageQuota, ok := status.Resources[string(quota_model.ResourceTypeStorage)]
	if !ok {
		t.Fatalf("expected storage quota to exist")
	}
	if storageQuota.Used != usageDelta {
		t.Fatalf("expected storage used %d, got %d", usageDelta, storageQuota.Used)
	}
	if storageQuota.Limit != -1 {
		t.Fatalf("expected storage limit -1, got %d", storageQuota.Limit)
	}
	if !storageQuota.Unlimited {
		t.Fatalf("expected storage quota to be unlimited")
	}
	if storageQuota.Usage != 0 {
		t.Fatalf("expected usage percent 0, got %f", storageQuota.Usage)
	}
}
