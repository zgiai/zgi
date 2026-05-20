package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/llm/gateway"
	"github.com/zgiai/ginext/internal/modules/llm/workspacequota/dto"
	workspacemodel "github.com/zgiai/ginext/internal/modules/workspace/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openWorkspaceQuotaServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := "file:workspace_quota_service_" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skip("sqlite driver requires cgo in this environment")
		}
		t.Fatalf("open sqlite db: %v", err)
	}

	if err := db.AutoMigrate(&workspacemodel.Workspace{}, &gateway.WorkspaceQuota{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func seedWorkspace(t *testing.T, db *gorm.DB, workspaceID, organizationID string) {
	t.Helper()
	ws := &workspacemodel.Workspace{
		ID:             workspaceID,
		Name:           "workspace-test",
		Plan:           "basic",
		OrganizationID: &organizationID,
	}
	if err := db.Create(ws).Error; err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
}

func TestWorkspaceQuotaService_GetWorkspaceQuota_DefaultWhenUnconfigured(t *testing.T) {
	db := openWorkspaceQuotaServiceTestDB(t)
	orgID := uuid.NewString()
	workspaceID := "ws-default"
	seedWorkspace(t, db, workspaceID, orgID)

	svc := NewWorkspaceQuotaService(db)
	got, err := svc.GetWorkspaceQuota(context.Background(), orgID, workspaceID)
	if err != nil {
		t.Fatalf("GetWorkspaceQuota returned error: %v", err)
	}
	if got.Configured {
		t.Fatalf("configured = %v, want false", got.Configured)
	}
	if got.QuotaLimit != nil {
		t.Fatalf("quota_limit = %v, want nil", *got.QuotaLimit)
	}
	if got.RemainQuota != 0 {
		t.Fatalf("remain_quota = %d, want 0", got.RemainQuota)
	}
}

func TestWorkspaceQuotaService_UpdateWorkspaceQuota_CustomSuccess(t *testing.T) {
	db := openWorkspaceQuotaServiceTestDB(t)
	orgID := uuid.NewString()
	workspaceID := "ws-custom"
	seedWorkspace(t, db, workspaceID, orgID)

	svc := NewWorkspaceQuotaService(db)
	limit := int64(100)
	remain := int64(80)
	got, err := svc.UpdateWorkspaceQuota(context.Background(), orgID, workspaceID, &dto.UpdateWorkspaceQuotaRequest{
		QuotaType:   dto.QuotaTypeCustom,
		QuotaAmount: &limit,
		RemainQuota: &remain,
	})
	if err != nil {
		t.Fatalf("UpdateWorkspaceQuota returned error: %v", err)
	}
	if !got.Configured {
		t.Fatalf("configured = %v, want true", got.Configured)
	}
	if got.QuotaLimit == nil || *got.QuotaLimit != 100 {
		t.Fatalf("quota_limit = %v, want 100", got.QuotaLimit)
	}
	if got.RemainQuota != 80 {
		t.Fatalf("remain_quota = %d, want 80", got.RemainQuota)
	}
}

func TestWorkspaceQuotaService_UpdateWorkspaceQuota_OrgMismatch(t *testing.T) {
	db := openWorkspaceQuotaServiceTestDB(t)
	orgA := uuid.NewString()
	orgB := uuid.NewString()
	workspaceID := "ws-mismatch"
	seedWorkspace(t, db, workspaceID, orgA)

	svc := NewWorkspaceQuotaService(db)
	limit := int64(100)
	_, err := svc.UpdateWorkspaceQuota(context.Background(), orgB, workspaceID, &dto.UpdateWorkspaceQuotaRequest{
		QuotaType:   dto.QuotaTypeCustom,
		QuotaAmount: &limit,
	})
	if !errors.Is(err, ErrWorkspaceOrgMismatch) {
		t.Fatalf("error = %v, want %v", err, ErrWorkspaceOrgMismatch)
	}
}

func TestWorkspaceQuotaService_UpdateWorkspaceQuota_RemainExceedsLimit(t *testing.T) {
	db := openWorkspaceQuotaServiceTestDB(t)
	orgID := uuid.NewString()
	workspaceID := "ws-exceed"
	seedWorkspace(t, db, workspaceID, orgID)

	svc := NewWorkspaceQuotaService(db)
	limit := int64(100)
	remain := int64(101)
	_, err := svc.UpdateWorkspaceQuota(context.Background(), orgID, workspaceID, &dto.UpdateWorkspaceQuotaRequest{
		QuotaType:   dto.QuotaTypeCustom,
		QuotaAmount: &limit,
		RemainQuota: &remain,
	})
	if !errors.Is(err, ErrRemainExceedsLimit) {
		t.Fatalf("error = %v, want %v", err, ErrRemainExceedsLimit)
	}
}
