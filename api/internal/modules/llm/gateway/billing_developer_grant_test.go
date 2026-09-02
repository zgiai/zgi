package gateway

import (
	"context"
	"testing"

	"github.com/google/uuid"
	accessmodel "github.com/zgiai/zgi/api/internal/modules/llm/developeraccess/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDeveloperGrantQuotaIsSharedBillingSubject(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&accessmodel.Grant{}); err != nil {
		t.Fatal(err)
	}
	organizationID, workspaceID, principalID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	quota := int64(100)
	grant := accessmodel.Grant{
		OrganizationID: organizationID, WorkspaceID: workspaceID,
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: principalID,
		Source: "approved_request", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &quota, RemainQuota: quota, MaxKeys: 3, AllowedModels: []string{}, AuthorizationVersion: 1,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	service := &BillingService{db: db}
	ctx := context.Background()
	billing := &BillingContext{
		OrganizationID: organizationID, AccessGrantID: grant.ID,
		QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: grant.ID,
		EstimatedCredits: 30, ActualCredits: 20,
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return service.preDeductSubjectQuota(ctx, tx, billing, nil) }); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.RemainQuota != 70 {
		t.Fatalf("remain after pre-deduct = %d, want 70", grant.RemainQuota)
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return service.settleSubjectQuota(ctx, tx, billing) }); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.RemainQuota != 80 || grant.UsedQuota != 20 {
		t.Fatalf("settled grant = %#v", grant)
	}
}
