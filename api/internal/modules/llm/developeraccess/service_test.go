package developeraccess

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	accessmodel "github.com/zgiai/zgi/api/internal/modules/llm/developeraccess/model"
	accountmodel "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openDeveloperAccessTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&workspacemodel.Workspace{},
		&workspacemodel.WorkspaceMember{},
		&accessmodel.Policy{},
		&accessmodel.AccessRequest{},
		&accessmodel.Grant{},
		&apikeymodel.TenantAPIKey{},
		&accountmodel.Account{},
	); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func seedDeveloperWorkspace(t *testing.T, db *gorm.DB) (workspaceID, organizationID, ownerID, memberID string) {
	t.Helper()
	workspaceID, organizationID, ownerID, memberID = uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	workspace := workspacemodel.Workspace{ID: workspaceID, Name: "Developer Workspace", Plan: "basic", Status: workspacemodel.WorkspaceStatusNormal, OrganizationID: &organizationID}
	if err := db.Create(&workspace).Error; err != nil {
		t.Fatal(err)
	}
	for _, account := range []accountmodel.Account{
		{ID: ownerID, Name: "Workspace Owner", Email: "owner@example.test", Status: accountmodel.AccountStatusActive},
		{ID: memberID, Name: "Research Member", Email: "member@example.test", Status: accountmodel.AccountStatusActive},
	} {
		if err := db.Create(&account).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, member := range []workspacemodel.WorkspaceMember{
		{ID: uuid.NewString(), WorkspaceID: workspaceID, AccountID: ownerID, Role: workspacemodel.WorkspaceRoleOwner, Permissions: []string{}},
		{ID: uuid.NewString(), WorkspaceID: workspaceID, AccountID: memberID, Role: workspacemodel.WorkspaceRoleMember, Permissions: []string{}},
	} {
		if err := db.Create(&member).Error; err != nil {
			t.Fatal(err)
		}
	}
	return
}

func TestOwnerCanCreateKeyUnderDefaultMemberApprovalPolicy(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, memberID := seedDeveloperWorkspace(t, db)
	repo := apikeyrepo.NewAPIKeyRepository(db)
	service := NewService(db, repo, nil)

	me, err := service.GetMe(context.Background(), workspaceID, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if me.Mode != accessmodel.AccessModeApprovalRequired || !me.CanCreateKey {
		t.Fatalf("unexpected access summary: %#v", me)
	}
	memberView, err := service.GetMe(context.Background(), workspaceID, memberID)
	if err != nil {
		t.Fatal(err)
	}
	if memberView.Mode != me.Mode || memberView.CanCreateKey {
		t.Fatalf("workspace policy changed by viewer role: owner=%#v member=%#v", me, memberView)
	}

	created, err := service.CreateKey(context.Background(), workspaceID, ownerID, CreateKeyInput{Name: "local experiment", Environment: "development"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Secret == "" || created.KeyMasked == created.Secret {
		t.Fatalf("secret response is invalid: %#v", created)
	}

	var stored apikeymodel.TenantAPIKey
	if err := db.First(&stored, "id = ?", created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Key != "" {
		t.Fatal("new personal API keys must not persist encrypted plaintext")
	}
	if stored.KeyHash == "" || stored.KeyPrefix == "" || stored.KeySuffix == "" {
		t.Fatalf("hash-only metadata missing: %#v", stored)
	}
	listed, err := service.ListKeys(context.Background(), workspaceID, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].KeyMasked == created.Secret {
		t.Fatalf("list leaked or lost the key: %#v", listed)
	}
}

func TestMemberApprovalCreatesGrantBeforePersonalKey(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, memberID := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)

	if _, err := service.CreateKey(context.Background(), workspaceID, memberID, CreateKeyInput{Name: "blocked"}); !errors.Is(err, ErrApprovalNeeded) {
		t.Fatalf("CreateKey error = %v, want approval required", err)
	}
	quota := int64(1200)
	request, err := service.CreateRequest(context.Background(), workspaceID, memberID, CreateRequestInput{Purpose: "Evaluate retrieval quality", Environment: "development", RequestedQuota: &quota})
	if err != nil {
		t.Fatal(err)
	}
	requests, err := service.ListRequests(context.Background(), workspaceID, ownerID, "pending")
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || requests[0].RequesterName != "Research Member" || requests[0].RequesterEmail != "member@example.test" {
		t.Fatalf("requester identity was not hydrated: %#v", requests)
	}
	if _, err := service.ReviewRequest(context.Background(), workspaceID, ownerID, request.ID, true, ReviewRequestInput{}); err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateKey(context.Background(), workspaceID, memberID, CreateKeyInput{Name: "approved experiment"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Secret == "" {
		t.Fatal("approved member did not receive one-time secret")
	}

	var grant accessmodel.Grant
	if err := db.Where("workspace_id = ? AND principal_id = ?", workspaceID, memberID).First(&grant).Error; err != nil {
		t.Fatal(err)
	}
	if grant.QuotaLimit == nil || *grant.QuotaLimit != quota || grant.RemainQuota != quota {
		t.Fatalf("unexpected approved grant: %#v", grant)
	}
	var stored apikeymodel.TenantAPIKey
	if err := db.First(&stored, "id = ?", created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.AccessGrantID == nil || *stored.AccessGrantID != grant.ID {
		t.Fatalf("key is not bound to shared grant: %#v", stored)
	}
}

func TestPrincipalValidationFailsAfterMemberLeavesWorkspace(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, _ := seedDeveloperWorkspace(t, db)
	repo := apikeyrepo.NewAPIKeyRepository(db)
	service := NewService(db, repo, nil)
	created, err := service.CreateKey(context.Background(), workspaceID, ownerID, CreateKeyInput{Name: "temporary"})
	if err != nil {
		t.Fatal(err)
	}
	var key apikeymodel.TenantAPIKey
	if err := db.First(&key, "id = ?", created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.(interface {
		ValidatePrincipalAccess(context.Context, *apikeymodel.TenantAPIKey) error
	}).ValidatePrincipalAccess(context.Background(), &key); err != nil {
		t.Fatalf("initial validation: %v", err)
	}
	if err := db.Where("workspace_id = ? AND account_id = ?", workspaceID, ownerID).Delete(&workspacemodel.WorkspaceMember{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.(interface {
		ValidatePrincipalAccess(context.Context, *apikeymodel.TenantAPIKey) error
	}).ValidatePrincipalAccess(context.Background(), &key); err == nil {
		t.Fatal("expected validation failure after membership removal")
	}
}

func TestPrincipalValidationFailsWhenWorkspaceDeveloperAccessIsDisabled(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, _ := seedDeveloperWorkspace(t, db)
	repo := apikeyrepo.NewAPIKeyRepository(db)
	service := NewService(db, repo, nil)
	created, err := service.CreateKey(context.Background(), workspaceID, ownerID, CreateKeyInput{Name: "workspace kill switch"})
	if err != nil {
		t.Fatal(err)
	}
	var key apikeymodel.TenantAPIKey
	if err := db.First(&key, "id = ?", created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.PutPolicy(context.Background(), workspaceID, ownerID, PolicyInput{
		Mode: accessmodel.AccessModeDisabled, MaxKeys: 3,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.(interface {
		ValidatePrincipalAccess(context.Context, *apikeymodel.TenantAPIKey) error
	}).ValidatePrincipalAccess(context.Background(), &key); err == nil {
		t.Fatal("expected workspace developer access kill switch to reject the key")
	}
}
