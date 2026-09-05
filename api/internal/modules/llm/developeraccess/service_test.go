package developeraccess

import (
	"context"
	"errors"
	"testing"
	"time"

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
	// Match the production partial unique index on the legacy plaintext
	// column. Hash-only personal keys must write NULL, not an empty string.
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_tenant_api_keys_key
		ON llm_organization_api_keys (key) WHERE deleted_at IS NULL`).Error; err != nil {
		t.Fatalf("create legacy key index: %v", err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_developer_access_grants_principal
		ON llm_developer_access_grants (workspace_id, principal_type, principal_id) WHERE deleted_at IS NULL`).Error; err != nil {
		t.Fatalf("create developer grant principal index: %v", err)
	}
	return db
}

func TestReviewerCannotApproveOwnRequest(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, _ := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)

	request, err := service.CreateRequest(context.Background(), workspaceID, ownerID, CreateRequestInput{Purpose: "Owner experiment"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReviewRequest(context.Background(), workspaceID, ownerID, request.ID, true, ReviewRequestInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ReviewRequest error = %v, want forbidden self-review", err)
	}
}

func TestSelfServiceRenewsExpiredGrantAsNewBudgetPeriod(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, organizationID, ownerID, memberID := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	quota, ttl := int64(900), int64(3600)
	if _, err := service.PutPolicy(context.Background(), workspaceID, ownerID, PolicyInput{
		Mode: accessmodel.AccessModeSelfService, DefaultQuota: &quota, MaxKeys: 2, DefaultTTLSeconds: &ttl,
	}); err != nil {
		t.Fatal(err)
	}
	expiredAt := time.Now().Add(-time.Minute)
	oldQuota := int64(400)
	grant := accessmodel.Grant{
		OrganizationID: organizationID, WorkspaceID: workspaceID, PrincipalType: accessmodel.PrincipalTypeUser,
		PrincipalID: memberID, Source: "self_service", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &oldQuota, UsedQuota: oldQuota, MaxKeys: 1, ExpiresAt: &expiredAt, AuthorizationVersion: 4,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateKey(context.Background(), workspaceID, memberID, CreateKeyInput{Name: "renewed"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Secret == "" {
		t.Fatal("renewed grant did not create a key")
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.QuotaLimit == nil || *grant.QuotaLimit != quota || grant.UsedQuota != 0 || grant.RemainQuota != quota || grant.MaxKeys != 2 || grant.AuthorizationVersion != 5 {
		t.Fatalf("unexpected renewed grant: %#v", grant)
	}
	if grant.ExpiresAt == nil || !grant.ExpiresAt.After(time.Now()) {
		t.Fatalf("renewed grant expiry = %v", grant.ExpiresAt)
	}
}

func TestApprovalManagerRenewsExpiredGrantAsNewBudgetPeriod(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, organizationID, ownerID, _ := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	quota, ttl := int64(700), int64(3600)
	if _, err := service.PutPolicy(context.Background(), workspaceID, ownerID, PolicyInput{
		Mode: accessmodel.AccessModeApprovalRequired, DefaultQuota: &quota, MaxKeys: 2, DefaultTTLSeconds: &ttl,
	}); err != nil {
		t.Fatal(err)
	}
	expiredAt := time.Now().Add(-time.Minute)
	oldQuota := int64(300)
	grant := accessmodel.Grant{
		OrganizationID: organizationID, WorkspaceID: workspaceID, PrincipalType: accessmodel.PrincipalTypeUser,
		PrincipalID: ownerID, Source: "workspace_admin", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &oldQuota, UsedQuota: oldQuota, MaxKeys: 1, ExpiresAt: &expiredAt, AuthorizationVersion: 2,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateKey(context.Background(), workspaceID, ownerID, CreateKeyInput{Name: "manager renewal"}); err != nil {
		t.Fatalf("manager should renew without self-approval: %v", err)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.Source != "workspace_admin" || grant.QuotaLimit == nil || *grant.QuotaLimit != quota || grant.UsedQuota != 0 || grant.RemainQuota != quota || grant.MaxKeys != 2 || grant.AuthorizationVersion != 3 {
		t.Fatalf("unexpected renewed manager grant: %#v", grant)
	}
}

func TestApprovalOfExpiredGrantStartsNewBudgetPeriod(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, memberID := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	oldQuota := int64(300)
	first, err := service.CreateRequest(context.Background(), workspaceID, memberID, CreateRequestInput{Purpose: "Initial access", RequestedQuota: &oldQuota})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReviewRequest(context.Background(), workspaceID, ownerID, first.ID, true, ReviewRequestInput{}); err != nil {
		t.Fatal(err)
	}
	var grant accessmodel.Grant
	if err := db.Where("workspace_id = ? AND principal_id = ?", workspaceID, memberID).First(&grant).Error; err != nil {
		t.Fatal(err)
	}
	expiredAt := time.Now().Add(-time.Minute)
	if err := db.Model(&grant).Updates(map[string]interface{}{"used_quota": oldQuota, "remain_quota": 0, "expires_at": expiredAt}).Error; err != nil {
		t.Fatal(err)
	}
	newQuota := int64(800)
	second, err := service.CreateRequest(context.Background(), workspaceID, memberID, CreateRequestInput{Purpose: "Renew access", RequestedQuota: &newQuota})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReviewRequest(context.Background(), workspaceID, ownerID, second.ID, true, ReviewRequestInput{}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.QuotaLimit == nil || *grant.QuotaLimit != newQuota || grant.UsedQuota != 0 || grant.RemainQuota != newQuota {
		t.Fatalf("renewed approval inherited the previous budget: %#v", grant)
	}
}

func TestApprovalUpdateOfActiveGrantPreservesUsage(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, memberID := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	oldQuota := int64(300)
	first, err := service.CreateRequest(context.Background(), workspaceID, memberID, CreateRequestInput{Purpose: "Initial access", RequestedQuota: &oldQuota})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReviewRequest(context.Background(), workspaceID, ownerID, first.ID, true, ReviewRequestInput{}); err != nil {
		t.Fatal(err)
	}
	var grant accessmodel.Grant
	if err := db.Where("workspace_id = ? AND principal_id = ?", workspaceID, memberID).First(&grant).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&grant).Updates(map[string]interface{}{"used_quota": int64(100), "remain_quota": int64(200)}).Error; err != nil {
		t.Fatal(err)
	}
	newQuota := int64(500)
	second, err := service.CreateRequest(context.Background(), workspaceID, memberID, CreateRequestInput{Purpose: "Expand active access", RequestedQuota: &newQuota})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReviewRequest(context.Background(), workspaceID, ownerID, second.ID, true, ReviewRequestInput{}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.UsedQuota != 100 || grant.RemainQuota != 400 {
		t.Fatalf("active approval update reset usage unexpectedly: %#v", grant)
	}
}

func TestDeveloperAccessRequestCapabilityIsExplicit(t *testing.T) {
	policy := &accessmodel.Policy{Mode: accessmodel.AccessModeApprovalRequired}
	member := &workspacemodel.WorkspaceMember{}
	if canRequestDeveloperAccess(&workspaceScope{Member: nil, CanManage: true}, policy, false, false) {
		t.Fatal("management-only principal must not be offered an impossible access request")
	}
	if !canRequestDeveloperAccess(&workspaceScope{Member: member}, policy, false, false) {
		t.Fatal("workspace member without a grant should be able to request access")
	}
	if canRequestDeveloperAccess(&workspaceScope{Member: member}, policy, false, true) {
		t.Fatal("member with a pending request must not be offered another request")
	}
}

func TestManagerKeyListIncludesPrincipalIdentity(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, memberID := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	request, err := service.CreateRequest(context.Background(), workspaceID, memberID, CreateRequestInput{Purpose: "Member experiment"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReviewRequest(context.Background(), workspaceID, ownerID, request.ID, true, ReviewRequestInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateKey(context.Background(), workspaceID, memberID, CreateKeyInput{Name: "member key"}); err != nil {
		t.Fatal(err)
	}
	keys, err := service.ListKeys(context.Background(), workspaceID, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].PrincipalName != "Research Member" || keys[0].PrincipalEmail != "member@example.test" {
		t.Fatalf("principal identity missing from manager list: %#v", keys)
	}
}

func TestDeveloperAuditIsScopedAndHydrated(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, organizationID, ownerID, memberID := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	request, err := service.CreateRequest(context.Background(), workspaceID, memberID, CreateRequestInput{Purpose: "Audit experiment"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReviewRequest(context.Background(), workspaceID, ownerID, request.ID, true, ReviewRequestInput{}); err != nil {
		t.Fatal(err)
	}
	key, err := service.CreateKey(context.Background(), workspaceID, memberID, CreateKeyInput{Name: "audited key"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE llm_usage_bills (
		attempt_id TEXT, request_id TEXT, organization_id TEXT, workspace_id TEXT, principal_type TEXT,
		principal_id TEXT, auth_method TEXT, api_key_id TEXT, model_name TEXT, provider_name TEXT,
		status TEXT, prompt_tokens INTEGER, completion_tokens INTEGER, total_tokens INTEGER,
		total_points INTEGER, response_time_ms INTEGER, error_code TEXT, request_created_at DATETIME
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE billing_attempt_entries (
		attempt_id TEXT, entry_type TEXT, actual_amount INTEGER
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO llm_usage_bills
		(attempt_id, request_id, organization_id, workspace_id, principal_type, principal_id, auth_method,
		 api_key_id, model_name, provider_name, status, prompt_tokens, completion_tokens, total_tokens,
		 total_points, response_time_ms, request_created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"attempt-1", "request-1", organizationID, workspaceID, accessmodel.PrincipalTypeUser, memberID,
		"personal_api_key", key.ID, "qwen-test", "qwen", "success", 10, 5, 15, 20, 100, time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO billing_attempt_entries (attempt_id, entry_type, actual_amount)
		VALUES (?, ?, ?)`, "attempt-1", "subject", 15).Error; err != nil {
		t.Fatal(err)
	}
	adminPage, err := service.ListAudit(context.Background(), workspaceID, ownerID, AuditQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if adminPage.Total != 1 || len(adminPage.Items) != 1 || adminPage.Items[0].PrincipalEmail != "member@example.test" || adminPage.Items[0].APIKeyName != "audited key" || adminPage.Items[0].QuotaChargedPoints != 15 || adminPage.Items[0].QuotaOveragePoints != 5 {
		t.Fatalf("unexpected admin audit page: %#v", adminPage)
	}
	memberPage, err := service.ListAudit(context.Background(), workspaceID, memberID, AuditQuery{PrincipalID: ownerID})
	if err != nil {
		t.Fatal(err)
	}
	if memberPage.Total != 1 || memberPage.Items[0].PrincipalID != memberID {
		t.Fatalf("member escaped own audit scope: %#v", memberPage)
	}
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

func TestCreateGrantReloadsConcurrentWinner(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, organizationID, ownerID, _ := seedDeveloperWorkspace(t, db)
	quota := int64(1000)
	winner := accessmodel.Grant{
		OrganizationID: organizationID, WorkspaceID: workspaceID,
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: ownerID,
		Source: "self_service", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &quota, RemainQuota: quota, MaxKeys: 3,
		AllowedModels: []string{}, AuthorizationVersion: 1,
	}
	if err := db.Create(&winner).Error; err != nil {
		t.Fatal(err)
	}

	candidate := accessmodel.Grant{
		OrganizationID: organizationID, WorkspaceID: workspaceID,
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: ownerID,
		Source: "self_service", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &quota, RemainQuota: quota, MaxKeys: 3,
		AllowedModels: []string{}, AuthorizationVersion: 1,
	}
	var created bool
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		created, err = createGrantOrReloadForUpdate(tx, &candidate)
		return err
	}); err != nil {
		t.Fatalf("reuse concurrent grant winner: %v", err)
	}
	if created {
		t.Fatal("conflicting candidate reported a new grant")
	}
	if candidate.ID != winner.ID || candidate.AuthorizationVersion != winner.AuthorizationVersion {
		t.Fatalf("candidate did not reload winner: got %#v want id=%s version=%d", candidate, winner.ID, winner.AuthorizationVersion)
	}
	var count int64
	if err := db.Model(&accessmodel.Grant{}).
		Where("workspace_id = ? AND principal_type = ? AND principal_id = ?", workspaceID, accessmodel.PrincipalTypeUser, ownerID).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("grant count = %d, want 1", count)
	}
}

func TestApprovalAppliesLimitsToExistingSelfServiceGrant(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, organizationID, ownerID, memberID := seedDeveloperWorkspace(t, db)
	oldQuota := int64(5000)
	winner := accessmodel.Grant{
		OrganizationID: organizationID, WorkspaceID: workspaceID,
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: memberID,
		Source: "self_service", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &oldQuota, UsedQuota: 400, RemainQuota: 4600, MaxKeys: 5,
		AllowedModels: []string{"broad-model"}, AuthorizationVersion: 1,
	}
	if err := db.Create(&winner).Error; err != nil {
		t.Fatal(err)
	}

	approvedQuota := int64(1000)
	expiresAt := time.Now().Add(time.Hour)
	scope := &workspaceScope{Workspace: &workspacemodel.Workspace{ID: workspaceID, OrganizationID: &organizationID}}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return upsertGrant(tx, scope, memberID, ownerID, "approved_request", &approvedQuota, 2, []string{"approved-model"}, &expiresAt, time.Now())
	}); err != nil {
		t.Fatalf("apply approved grant settings: %v", err)
	}
	if err := db.First(&winner, "id = ?", winner.ID).Error; err != nil {
		t.Fatal(err)
	}
	if winner.Source != "approved_request" || winner.QuotaLimit == nil || *winner.QuotaLimit != approvedQuota ||
		winner.UsedQuota != 400 || winner.RemainQuota != 600 || winner.MaxKeys != 2 ||
		len(winner.AllowedModels) != 1 || winner.AllowedModels[0] != "approved-model" || winner.AuthorizationVersion != 2 {
		t.Fatalf("approval settings were not applied to existing grant: %#v", winner)
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

func TestPrincipalValidationRechecksPersistedKeyStatus(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, _ := seedDeveloperWorkspace(t, db)
	repo := apikeyrepo.NewAPIKeyRepository(db)
	service := NewService(db, repo, nil)
	created, err := service.CreateKey(context.Background(), workspaceID, ownerID, CreateKeyInput{Name: "cached key"})
	if err != nil {
		t.Fatal(err)
	}
	var stale apikeymodel.TenantAPIKey
	if err := db.First(&stale, "id = ?", created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&apikeymodel.TenantAPIKey{}).Where("id = ?", created.ID).Update("status", "inactive").Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.(interface {
		ValidatePrincipalAccess(context.Context, *apikeymodel.TenantAPIKey) error
	}).ValidatePrincipalAccess(context.Background(), &stale); err == nil {
		t.Fatal("expected persisted inactive status to override stale cached key")
	}
	past := time.Now().Add(-time.Minute)
	if err := db.Model(&apikeymodel.TenantAPIKey{}).Where("id = ?", created.ID).
		Updates(map[string]interface{}{"status": "active", "expires_at": past}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.(interface {
		ValidatePrincipalAccess(context.Context, *apikeymodel.TenantAPIKey) error
	}).ValidatePrincipalAccess(context.Background(), &stale); err == nil {
		t.Fatal("expected persisted expiration to override stale cached key")
	}
}

func TestArchivedWorkspaceRejectsDeveloperAccessAndPersonalKeys(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, _ := seedDeveloperWorkspace(t, db)
	repo := apikeyrepo.NewAPIKeyRepository(db)
	service := NewService(db, repo, nil)
	created, err := service.CreateKey(context.Background(), workspaceID, ownerID, CreateKeyInput{Name: "archived workspace key"})
	if err != nil {
		t.Fatal(err)
	}
	var key apikeymodel.TenantAPIKey
	if err := db.First(&key, "id = ?", created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&workspacemodel.Workspace{}).Where("id = ?", workspaceID).Update("status", workspacemodel.WorkspaceStatusArchived).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetMe(context.Background(), workspaceID, ownerID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("GetMe error = %v, want forbidden for archived workspace", err)
	}
	if err := repo.(interface {
		ValidatePrincipalAccess(context.Context, *apikeymodel.TenantAPIKey) error
	}).ValidatePrincipalAccess(context.Background(), &key); err == nil {
		t.Fatal("expected archived workspace to reject personal key")
	}
}

func TestReenableCannotBypassGrantMaximumActiveKeys(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, _ := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	created := make([]*CreatedKey, 0, 3)
	for _, name := range []string{"first", "second", "third"} {
		key, err := service.CreateKey(context.Background(), workspaceID, ownerID, CreateKeyInput{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, key)
	}
	if _, err := service.SetKeyStatus(context.Background(), workspaceID, ownerID, created[0].ID, "inactive", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateKey(context.Background(), workspaceID, ownerID, CreateKeyInput{Name: "replacement"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetKeyStatus(context.Background(), workspaceID, ownerID, created[0].ID, "active", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("SetKeyStatus error = %v, want active-key limit conflict", err)
	}
}

func TestPersonalKeyMutationsPreserveNullLegacyKey(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, _ := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)

	first, err := service.CreateKey(context.Background(), workspaceID, ownerID, CreateKeyInput{Name: "first"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateKey(context.Background(), workspaceID, ownerID, CreateKeyInput{Name: "second"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateKey(context.Background(), workspaceID, ownerID, first.ID, UpdateKeyInput{Name: "first renamed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateKey(context.Background(), workspaceID, ownerID, second.ID, UpdateKeyInput{Name: "second renamed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetKeyStatus(context.Background(), workspaceID, ownerID, first.ID, "inactive", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetKeyStatus(context.Background(), workspaceID, ownerID, second.ID, "revoked", "regression complete"); err != nil {
		t.Fatal(err)
	}

	var nonNullLegacyKeys int64
	if err := db.Model(&apikeymodel.TenantAPIKey{}).
		Where("id IN ? AND key IS NOT NULL", []string{first.ID, second.ID}).
		Count(&nonNullLegacyKeys).Error; err != nil {
		t.Fatal(err)
	}
	if nonNullLegacyKeys != 0 {
		t.Fatalf("personal key mutation wrote %d legacy plaintext values", nonNullLegacyKeys)
	}
}

func TestMemberCanRotateOnlyOwnKey(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, memberID := seedDeveloperWorkspace(t, db)
	repo := apikeyrepo.NewAPIKeyRepository(db)
	service := NewService(db, repo, nil)
	request, err := service.CreateRequest(context.Background(), workspaceID, memberID, CreateRequestInput{Purpose: "Rotate safely"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReviewRequest(context.Background(), workspaceID, ownerID, request.ID, true, ReviewRequestInput{}); err != nil {
		t.Fatal(err)
	}
	current, err := service.CreateKey(context.Background(), workspaceID, memberID, CreateKeyInput{Name: "member key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RotateKey(context.Background(), workspaceID, ownerID, current.ID, RotateKeyInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("admin rotated another member's secret: %v", err)
	}
	replacement, err := service.RotateKey(context.Background(), workspaceID, memberID, current.ID, RotateKeyInput{Name: "member key v2"})
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Secret == "" || replacement.ID == current.ID || replacement.Name != "member key v2" {
		t.Fatalf("unexpected replacement: %#v", replacement)
	}
	var previous, stored apikeymodel.TenantAPIKey
	if err := db.First(&previous, "id = ?", current.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&stored, "id = ?", replacement.ID).Error; err != nil {
		t.Fatal(err)
	}
	if previous.Status != "revoked" || previous.RevokedReason == nil || *previous.RevokedReason != "rotated" || stored.RotatedFromID == nil || *stored.RotatedFromID != previous.ID {
		t.Fatalf("rotation linkage invalid: previous=%#v replacement=%#v", previous, stored)
	}
}

func TestDisabledPolicyRejectsKeyReactivationAndRotation(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, memberID := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	request, err := service.CreateRequest(context.Background(), workspaceID, memberID, CreateRequestInput{Purpose: "Test disabled lifecycle"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReviewRequest(context.Background(), workspaceID, ownerID, request.ID, true, ReviewRequestInput{}); err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateKey(context.Background(), workspaceID, memberID, CreateKeyInput{Name: "disabled lifecycle"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetKeyStatus(context.Background(), workspaceID, memberID, created.ID, "inactive", ""); err != nil {
		t.Fatal(err)
	}
	policy, err := service.GetPolicy(context.Background(), workspaceID, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PutPolicy(context.Background(), workspaceID, ownerID, PolicyInput{
		Mode: accessmodel.AccessModeDisabled, DefaultQuota: policy.DefaultQuota, MaxQuota: policy.MaxQuota,
		MaxKeys: policy.MaxKeys, DefaultTTLSeconds: policy.DefaultTTLSeconds, MaxTTLSeconds: policy.MaxTTLSeconds,
		AllowedModels: policy.AllowedModels,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetKeyStatus(context.Background(), workspaceID, memberID, created.ID, "active", ""); !errors.Is(err, ErrAccessDisabled) {
		t.Fatalf("reactivate error = %v, want disabled policy", err)
	}
	if _, err := service.RotateKey(context.Background(), workspaceID, memberID, created.ID, RotateKeyInput{}); !errors.Is(err, ErrAccessDisabled) {
		t.Fatalf("rotate error = %v, want disabled policy", err)
	}
	var stored apikeymodel.TenantAPIKey
	if err := db.First(&stored, "id = ?", created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != "inactive" || stored.RevokedAt != nil {
		t.Fatalf("disabled lifecycle mutation changed key: %#v", stored)
	}
}

func TestExpiredActiveKeyDoesNotConsumeGrantSlot(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, memberID := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	request, err := service.CreateRequest(context.Background(), workspaceID, memberID, CreateRequestInput{Purpose: "Replace expired key"})
	if err != nil {
		t.Fatal(err)
	}
	maxKeys := 1
	if _, err := service.ReviewRequest(context.Background(), workspaceID, ownerID, request.ID, true, ReviewRequestInput{MaxKeys: &maxKeys}); err != nil {
		t.Fatal(err)
	}
	key, err := service.CreateKey(context.Background(), workspaceID, memberID, CreateKeyInput{Name: "short lived"})
	if err != nil {
		t.Fatal(err)
	}
	expiredAt := time.Now().Add(-time.Minute)
	if err := db.Model(&apikeymodel.TenantAPIKey{}).Where("id = ?", key.ID).Update("expires_at", expiredAt).Error; err != nil {
		t.Fatal(err)
	}
	me, err := service.GetMe(context.Background(), workspaceID, memberID)
	if err != nil {
		t.Fatal(err)
	}
	if me.ActiveKeyCount != 0 {
		t.Fatalf("active key count = %d, want 0 for expired key", me.ActiveKeyCount)
	}
	if _, err := service.CreateKey(context.Background(), workspaceID, memberID, CreateKeyInput{Name: "replacement"}); err != nil {
		t.Fatalf("create replacement after expiry: %v", err)
	}
}
