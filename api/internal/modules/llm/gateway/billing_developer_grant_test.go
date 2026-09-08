package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	accessmodel "github.com/zgiai/zgi/api/internal/modules/llm/developeraccess/model"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
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

func TestDeveloperGrantUnknownPlatformCostSerializesBoundedRequests(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&accessmodel.Grant{}, &BillingAttempt{}, &BillingAttemptEntry{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX uq_billing_attempt_entry ON billing_attempt_entries (attempt_id, entry_type, ledger_type)`).Error; err != nil {
		t.Fatal(err)
	}
	quota := int64(100)
	grant := accessmodel.Grant{
		OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(),
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: uuid.NewString(),
		Source: "approved_request", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &quota, UsedQuota: 90, RemainQuota: 10,
		MaxKeys: 1, AllowedModels: []string{}, AuthorizationVersion: 4,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	service := &BillingService{db: db}
	first := &BillingContext{
		OrganizationID: grant.OrganizationID, AccessGrantID: grant.ID,
		QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: grant.ID,
		EstimatedCredits: 0, BillingLane: UsageBillingLanePlatform, UseSystemProvider: true,
		AttemptID: uuid.NewString(), RequestID: uuid.NewString(),
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := service.preDeductSubjectQuota(context.Background(), tx, first, nil); err != nil {
			return err
		}
		return service.upsertAttemptInit(context.Background(), tx, first)
	}); err != nil {
		t.Fatal(err)
	}
	if first.SubjectReservedCredits != 10 || first.GrantAuthorizationVersion == nil || *first.GrantAuthorizationVersion != 4 {
		t.Fatalf("reservation/version = %d/%v, want 10/4", first.SubjectReservedCredits, first.GrantAuthorizationVersion)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.RemainQuota != 0 {
		t.Fatalf("remain after unknown-cost reservation = %d, want 0", grant.RemainQuota)
	}
	var attempt BillingAttempt
	if err := db.First(&attempt, "attempt_id = ?", first.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.GrantAuthorizationVersion == nil || *attempt.GrantAuthorizationVersion != 4 {
		t.Fatalf("persisted grant version = %v, want 4", attempt.GrantAuthorizationVersion)
	}
	var entries []BillingAttemptEntry
	if err := db.Where("attempt_id = ?", first.AttemptID).Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("attempt entries = %d, want 2", len(entries))
	}
	for _, entry := range entries {
		if entry.EntryType == billingEntryTypeSubject && entry.ReservedAmount != 10 {
			t.Fatalf("subject reservation = %d, want 10", entry.ReservedAmount)
		}
		if entry.EntryType == billingEntryTypeFund && entry.ReservedAmount != 0 {
			t.Fatalf("organization fund reservation = %d, want 0", entry.ReservedAmount)
		}
	}

	second := &BillingContext{
		OrganizationID: grant.OrganizationID, AccessGrantID: grant.ID,
		QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: grant.ID,
		EstimatedCredits: 0, BillingLane: UsageBillingLanePlatform, UseSystemProvider: true,
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		return service.preDeductSubjectQuota(context.Background(), tx, second, nil)
	})
	if err != ErrInsufficientQuota {
		t.Fatalf("second unknown-cost pre-deduct error = %v, want %v", err, ErrInsufficientQuota)
	}

	first.ActualCredits = 3
	if err := db.Transaction(func(tx *gorm.DB) error {
		return service.settleSubjectQuota(context.Background(), tx, first)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.UsedQuota != 93 || grant.RemainQuota != 7 {
		t.Fatalf("settled unknown-cost grant used/remain = %d/%d, want 93/7", grant.UsedQuota, grant.RemainQuota)
	}
}

func TestDeveloperGrantSettlementDoesNotMutateNewAuthorizationPeriod(t *testing.T) {
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
		QuotaLimit: &quota, RemainQuota: quota, MaxKeys: 1,
		AllowedModels: []string{}, AuthorizationVersion: 1,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	billing := &BillingContext{
		OrganizationID: grant.OrganizationID, AccessGrantID: grant.ID,
		QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: grant.ID,
		EstimatedCredits: 20, ActualCredits: 10,
	}
	service := &BillingService{db: db}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return service.preDeductSubjectQuota(context.Background(), tx, billing, nil)
	}); err != nil {
		t.Fatal(err)
	}
	if billing.GrantAuthorizationVersion == nil || *billing.GrantAuthorizationVersion != 1 {
		t.Fatalf("captured grant version = %v, want 1", billing.GrantAuthorizationVersion)
	}
	if err := db.Model(&accessmodel.Grant{}).Where("id = ?", grant.ID).Updates(map[string]any{
		"authorization_version": 2,
		"used_quota":            0,
		"remain_quota":          quota,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return service.settleSubjectQuota(context.Background(), tx, billing)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.AuthorizationVersion != 2 || grant.UsedQuota != 0 || grant.RemainQuota != quota {
		t.Fatalf("new authorization period was mutated: %#v", grant)
	}
	if billing.QuotaChargedCredits == nil || *billing.QuotaChargedCredits != 0 {
		t.Fatalf("stale-period quota charge = %v, want 0", billing.QuotaChargedCredits)
	}
}

func TestDeveloperGrantPreDeductRejectsStaleKeyAuthorizationPeriod(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&accessmodel.Grant{}, &accessmodel.Policy{}, &workspacemodel.Organization{}, &workspacemodel.Workspace{}); err != nil {
		t.Fatal(err)
	}
	quota := int64(100)
	grant := accessmodel.Grant{
		OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(),
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: uuid.NewString(),
		Source: "approved_request", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &quota, RemainQuota: quota, MaxKeys: 1,
		AllowedModels: []string{}, AuthorizationVersion: 2,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	organizationID := grant.OrganizationID
	if err := db.Create(&workspacemodel.Organization{
		ID: organizationID, Name: "Developer billing organization", Status: workspacemodel.OrganizationStatusActive,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&workspacemodel.Workspace{
		ID: grant.WorkspaceID, Name: "Developer billing workspace", Plan: "basic",
		Status: workspacemodel.WorkspaceStatusNormal, OrganizationID: &organizationID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	staleVersion := int64(1)
	billing := &BillingContext{
		OrganizationID: grant.OrganizationID, WorkspaceID: grant.WorkspaceID, AccessGrantID: grant.ID,
		QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: grant.ID,
		GrantAuthorizationVersion: &staleVersion, AuthMethod: "personal_api_key",
		EstimatedCredits: 10, BillingLane: UsageBillingLanePlatform, UseSystemProvider: true,
	}
	service := &BillingService{db: db}
	err = db.Transaction(func(tx *gorm.DB) error {
		return service.preDeductSubjectQuota(context.Background(), tx, billing, nil)
	})
	if err != ErrAPIKeyInactive {
		t.Fatalf("stale key pre-deduct error = %v, want %v", err, ErrAPIKeyInactive)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.AuthorizationVersion != 2 || grant.UsedQuota != 0 || grant.RemainQuota != quota {
		t.Fatalf("stale key mutated current authorization period: %#v", grant)
	}
}

func TestDeveloperGrantReservationRollsBackWhenKeyIsInactiveOrExpired(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	for _, testCase := range []struct {
		name               string
		status             string
		expiresAt          *time.Time
		policyMode         string
		workspaceStatus    workspacemodel.WorkspaceStatus
		organizationStatus workspacemodel.OrganizationStatus
	}{
		{name: "inactive", status: "inactive", workspaceStatus: workspacemodel.WorkspaceStatusNormal, organizationStatus: workspacemodel.OrganizationStatusActive},
		{name: "expired", status: "active", expiresAt: &past, workspaceStatus: workspacemodel.WorkspaceStatusNormal, organizationStatus: workspacemodel.OrganizationStatusActive},
		{name: "policy disabled", status: "active", policyMode: accessmodel.AccessModeDisabled, workspaceStatus: workspacemodel.WorkspaceStatusNormal, organizationStatus: workspacemodel.OrganizationStatusActive},
		{name: "workspace archived", status: "active", workspaceStatus: workspacemodel.WorkspaceStatusArchived, organizationStatus: workspacemodel.OrganizationStatusActive},
		{name: "organization archived", status: "active", workspaceStatus: workspacemodel.WorkspaceStatusNormal, organizationStatus: workspacemodel.OrganizationStatusArchived},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(
				&accessmodel.Grant{},
				&accessmodel.Policy{},
				&apikeymodel.TenantAPIKey{},
				&workspacemodel.Organization{},
				&workspacemodel.Workspace{},
				&BillingAttempt{},
				&BillingAttemptEntry{},
			); err != nil {
				t.Fatal(err)
			}
			if err := db.Exec(`CREATE UNIQUE INDEX uq_inactive_key_billing_entry ON billing_attempt_entries (attempt_id, entry_type, ledger_type)`).Error; err != nil {
				t.Fatal(err)
			}
			organizationID, workspaceID, principalID := uuid.NewString(), uuid.NewString(), uuid.NewString()
			if err := db.Create(&workspacemodel.Organization{
				ID: organizationID, Name: "Developer billing organization", Status: testCase.organizationStatus,
			}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&workspacemodel.Workspace{
				ID: workspaceID, Name: "Developer billing workspace", Plan: "basic",
				Status: testCase.workspaceStatus, OrganizationID: &organizationID,
			}).Error; err != nil {
				t.Fatal(err)
			}
			if testCase.policyMode != "" {
				if err := db.Create(&accessmodel.Policy{
					OrganizationID: organizationID, WorkspaceID: workspaceID, Mode: testCase.policyMode,
					MaxKeys: 1, AllowedModels: []string{}, Version: 1,
				}).Error; err != nil {
					t.Fatal(err)
				}
			}
			quota := int64(100)
			grant := accessmodel.Grant{
				OrganizationID: organizationID, WorkspaceID: workspaceID,
				PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: principalID,
				Source: "approved_request", Status: accessmodel.GrantStatusActive,
				QuotaLimit: &quota, RemainQuota: quota, MaxKeys: 1, AuthorizationVersion: 3,
			}
			if err := db.Create(&grant).Error; err != nil {
				t.Fatal(err)
			}
			principalType := accessmodel.PrincipalTypeUser
			key := apikeymodel.TenantAPIKey{
				OrganizationID: organizationID, WorkspaceID: &workspaceID,
				PrincipalType: &principalType, PrincipalID: &principalID, AccessGrantID: &grant.ID,
				KeyHash: uuid.NewString(), Name: testCase.name, Status: testCase.status, ExpiresAt: testCase.expiresAt,
				SecretVersion: 2, AuthorizationVersion: grant.AuthorizationVersion,
			}
			if err := db.Omit("Key").Create(&key).Error; err != nil {
				t.Fatal(err)
			}
			version := grant.AuthorizationVersion
			billing := &BillingContext{
				AttemptID: uuid.NewString() + "-a1", RequestID: uuid.NewString(),
				OrganizationID: organizationID, WorkspaceID: workspaceID, APIKeyID: key.ID,
				PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: principalID,
				AccessGrantID: grant.ID, GrantAuthorizationVersion: &version, AuthMethod: "personal_api_key",
				QuotaSubjectType: quotaSubjectTypeAccessGrant, QuotaSubjectID: grant.ID,
				EstimatedCredits: 10, BillingLane: UsageBillingLanePlatform, UseSystemProvider: true,
			}
			service := &BillingService{db: db}
			if err := service.PreDeduct(context.Background(), billing); !errors.Is(err, ErrAPIKeyInactive) {
				t.Fatalf("pre-deduct %s key error = %v, want %v", testCase.name, err, ErrAPIKeyInactive)
			}
			if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
				t.Fatal(err)
			}
			if grant.UsedQuota != 0 || grant.RemainQuota != quota {
				t.Fatalf("%s key left a grant reservation: used/remain = %d/%d", testCase.name, grant.UsedQuota, grant.RemainQuota)
			}
		})
	}
}

func int64Ptr(value int64) *int64 { return &value }
