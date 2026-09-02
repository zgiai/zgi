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

func TestDeveloperGrantSettlementCapsChargeAtHardLimit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&accessmodel.Grant{}); err != nil {
		t.Fatal(err)
	}
	organizationID := uuid.NewString()
	quota := int64(40)
	grant := accessmodel.Grant{
		OrganizationID:       organizationID,
		WorkspaceID:          uuid.NewString(),
		PrincipalType:        accessmodel.PrincipalTypeUser,
		PrincipalID:          uuid.NewString(),
		Source:               "approved_request",
		Status:               accessmodel.GrantStatusActive,
		QuotaLimit:           &quota,
		RemainQuota:          quota,
		MaxKeys:              1,
		AllowedModels:        []string{},
		AuthorizationVersion: 1,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	service := &BillingService{db: db}
	billing := &BillingContext{
		OrganizationID:   organizationID,
		AccessGrantID:    grant.ID,
		QuotaSubjectType: quotaSubjectTypeAccessGrant,
		QuotaSubjectID:   grant.ID,
		EstimatedCredits: 30,
		ActualCredits:    50,
	}
	ctx := context.Background()
	if err := db.Transaction(func(tx *gorm.DB) error { return service.preDeductSubjectQuota(ctx, tx, billing, nil) }); err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return service.settleSubjectQuota(ctx, tx, billing) }); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.UsedQuota != quota || grant.RemainQuota != 0 {
		t.Fatalf("settled grant used/remain = %d/%d, want %d/0", grant.UsedQuota, grant.RemainQuota, quota)
	}
	if billing.QuotaChargedCredits == nil || *billing.QuotaChargedCredits != quota {
		t.Fatalf("quota charged = %v, want %d", billing.QuotaChargedCredits, quota)
	}
}

func TestDeveloperGrantSettlementUsesLimitWhenStoredRemainIsInconsistent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&accessmodel.Grant{}); err != nil {
		t.Fatal(err)
	}
	quota := int64(100)
	grant := accessmodel.Grant{
		OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(),
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: uuid.NewString(),
		Source: "approved_request", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &quota, UsedQuota: 90, RemainQuota: 40, MaxKeys: 1,
		AllowedModels: []string{}, AuthorizationVersion: 1,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	charged := &BillingContext{
		OrganizationID: grant.OrganizationID, AccessGrantID: grant.ID,
		QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: grant.ID,
		EstimatedCredits: 5, ActualCredits: 20,
	}
	service := &BillingService{db: db}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return service.preDeductSubjectQuota(context.Background(), tx, charged, nil)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return service.settleSubjectQuota(context.Background(), tx, charged)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.UsedQuota != quota || grant.RemainQuota != 0 || charged.QuotaChargedCredits == nil || *charged.QuotaChargedCredits != 10 {
		t.Fatalf("inconsistent grant settlement = %#v, charged=%v", grant, charged.QuotaChargedCredits)
	}
}

func TestDeveloperGrantZeroCreditReservationHonorsBoundedExhaustion(t *testing.T) {
	tests := []struct {
		name        string
		quotaLimit  *int64
		usedQuota   int64
		remainQuota int64
		wantErr     bool
	}{
		{name: "exhausted bounded grant", quotaLimit: int64Ptr(100), usedQuota: 100, remainQuota: 0, wantErr: true},
		{name: "bounded grant with balance", quotaLimit: int64Ptr(100), usedQuota: 90, remainQuota: 10},
		{name: "unlimited grant", quotaLimit: nil, usedQuota: 0, remainQuota: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&accessmodel.Grant{}); err != nil {
				t.Fatal(err)
			}
			grant := accessmodel.Grant{
				OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(),
				PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: uuid.NewString(),
				Source: "approved_request", Status: accessmodel.GrantStatusActive,
				QuotaLimit: test.quotaLimit, UsedQuota: test.usedQuota, RemainQuota: test.remainQuota,
				MaxKeys: 1, AllowedModels: []string{}, AuthorizationVersion: 1,
			}
			if err := db.Create(&grant).Error; err != nil {
				t.Fatal(err)
			}
			billing := &BillingContext{
				OrganizationID: grant.OrganizationID, AccessGrantID: grant.ID,
				QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: grant.ID,
				EstimatedCredits: 0,
			}
			service := &BillingService{db: db}
			err = db.Transaction(func(tx *gorm.DB) error {
				return service.preDeductSubjectQuota(context.Background(), tx, billing, nil)
			})
			if test.wantErr && err != ErrInsufficientQuota {
				t.Fatalf("pre-deduct error = %v, want %v", err, ErrInsufficientQuota)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("pre-deduct zero estimate: %v", err)
			}
		})
	}
}

func int64Ptr(value int64) *int64 { return &value }
