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
		&workspacemodel.Organization{},
		&workspacemodel.OrganizationMember{},
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

func TestDeveloperAccessTTLRejectsDurationOverflow(t *testing.T) {
	tooLarge := maxDeveloperAccessTTLSeconds + 1
	if err := validatePolicy(PolicyInput{
		Mode: accessmodel.AccessModeSelfService, MaxKeys: 1, DefaultTTLSeconds: &tooLarge,
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("validatePolicy error = %v, want invalid oversized TTL", err)
	}
	if _, err := ttlExpiry(time.Now(), &tooLarge); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ttlExpiry error = %v, want invalid oversized TTL", err)
	}

	db := openDeveloperAccessTestDB(t)
	workspaceID, _, _, memberID := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	if _, err := service.CreateRequest(context.Background(), workspaceID, memberID, CreateRequestInput{
		Purpose: "oversized access period", RequestedTTLSeconds: &tooLarge,
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("CreateRequest error = %v, want invalid oversized TTL", err)
	}
}

func TestPutPolicyUsesFiniteCeilingsAsSecureDefaults(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, memberID := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	now := time.Date(2026, time.September, 6, 1, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	maxQuota, maxTTL := int64(40), int64(7200)

	policy, err := service.PutPolicy(context.Background(), workspaceID, ownerID, PolicyInput{
		Mode: accessmodel.AccessModeSelfService, MaxQuota: &maxQuota, MaxKeys: 2, MaxTTLSeconds: &maxTTL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if policy.DefaultQuota == nil || *policy.DefaultQuota != maxQuota || policy.DefaultTTLSeconds == nil || *policy.DefaultTTLSeconds != maxTTL {
		t.Fatalf("finite policy ceilings were not persisted as secure defaults: %#v", policy)
	}

	scope, err := service.scope(context.Background(), workspaceID, memberID)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := service.ensureGrant(context.Background(), scope, memberID)
	if err != nil {
		t.Fatal(err)
	}
	if grant.QuotaLimit == nil || *grant.QuotaLimit != maxQuota || grant.RemainQuota != maxQuota {
		t.Fatalf("self-service grant quota = %#v, want %d", grant, maxQuota)
	}
	wantExpiry := now.Add(time.Duration(maxTTL) * time.Second)
	if grant.ExpiresAt == nil || !grant.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("self-service grant expiry = %v, want %v", grant.ExpiresAt, wantExpiry)
	}
}

func TestLegacyPolicyFiniteCeilingsBoundApprovedGrant(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, organizationID, ownerID, memberID := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	now := time.Date(2026, time.September, 6, 2, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	maxQuota, maxTTL := int64(75), int64(3600)
	policy := accessmodel.Policy{
		OrganizationID: organizationID, WorkspaceID: workspaceID,
		Mode: accessmodel.AccessModeApprovalRequired, MaxQuota: &maxQuota, MaxKeys: 2,
		MaxTTLSeconds: &maxTTL, AllowedModels: []string{}, Version: 1,
	}
	if err := db.Create(&policy).Error; err != nil {
		t.Fatal(err)
	}

	request, err := service.CreateRequest(context.Background(), workspaceID, memberID, CreateRequestInput{Purpose: "bounded legacy policy"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReviewRequest(context.Background(), workspaceID, ownerID, request.ID, true, ReviewRequestInput{}); err != nil {
		t.Fatal(err)
	}
	var grant accessmodel.Grant
	if err := db.Where("workspace_id = ? AND principal_id = ?", workspaceID, memberID).First(&grant).Error; err != nil {
		t.Fatal(err)
	}
	if grant.QuotaLimit == nil || *grant.QuotaLimit != maxQuota || grant.RemainQuota != maxQuota {
		t.Fatalf("approved grant quota = %#v, want %d", grant, maxQuota)
	}
	wantExpiry := now.Add(time.Duration(maxTTL) * time.Second)
	if grant.ExpiresAt == nil || !grant.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("approved grant expiry = %v, want %v", grant.ExpiresAt, wantExpiry)
	}
}

func TestPutPolicyRejectsStaleWorkspaceScopeAfterOrganizationChange(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, _ := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	scope, err := service.scope(context.Background(), workspaceID, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	newOrganizationID := uuid.NewString()
	if err := db.Create(&workspacemodel.Organization{ID: newOrganizationID, Name: "New Organization", Status: workspacemodel.OrganizationStatusActive}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&workspacemodel.Workspace{}).Where("id = ?", workspaceID).Update("organization_id", newOrganizationID).Error; err != nil {
		t.Fatal(err)
	}
	_, err = service.putPolicy(context.Background(), scope, ownerID, PolicyInput{Mode: accessmodel.AccessModeSelfService, MaxKeys: 2})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("putPolicy error = %v, want stale organization conflict", err)
	}
	var count int64
	if err := db.Model(&accessmodel.Policy{}).Where("workspace_id = ?", workspaceID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stale policy count = %d, want 0", count)
	}
}

func TestPutPolicyRevalidatesManagementPermission(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, _ := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	scope, err := service.scope(context.Background(), workspaceID, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&workspacemodel.WorkspaceMember{}).
		Where("workspace_id = ? AND account_id = ?", workspaceID, ownerID).
		Update("role", workspacemodel.WorkspaceRoleMember).Error; err != nil {
		t.Fatal(err)
	}
	_, err = service.putPolicy(context.Background(), scope, ownerID, PolicyInput{Mode: accessmodel.AccessModeSelfService, MaxKeys: 2})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("putPolicy error = %v, want forbidden after role downgrade", err)
	}
}

func TestPutPolicyRevalidatesOrganizationAdminRoleInTransaction(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, organizationID, _, _ := seedDeveloperWorkspace(t, db)
	organizationAdminID := uuid.NewString()
	if err := db.Create(&workspacemodel.OrganizationMember{
		OrganizationID: organizationID,
		AccountID:      organizationAdminID,
		Role:           workspacemodel.OrganizationRoleAdmin,
		Status:         workspacemodel.OrganizationMemberStatusActive,
	}).Error; err != nil {
		t.Fatal(err)
	}
	var workspace workspacemodel.Workspace
	if err := db.First(&workspace, "id = ?", workspaceID).Error; err != nil {
		t.Fatal(err)
	}
	staleScope := &workspaceScope{Workspace: &workspace, CanManage: true}
	if err := db.Model(&workspacemodel.OrganizationMember{}).
		Where("organization_id = ? AND account_id = ?", organizationID, organizationAdminID).
		Update("role", workspacemodel.OrganizationRoleNormal).Error; err != nil {
		t.Fatal(err)
	}

	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	_, err := service.putPolicy(context.Background(), staleScope, organizationAdminID, PolicyInput{
		Mode: accessmodel.AccessModeSelfService, MaxKeys: 2,
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("putPolicy with stale organization admin role error = %v, want forbidden", err)
	}
	var policies int64
	if err := db.Model(&accessmodel.Policy{}).Where("workspace_id = ?", workspaceID).Count(&policies).Error; err != nil {
		t.Fatal(err)
	}
	if policies != 0 {
		t.Fatalf("stale organization admin created %d policies", policies)
	}
}

func TestCreateRequestRejectsStaleWorkspaceScopeAfterOrganizationChange(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, _, memberID := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	staleScope, err := service.scope(context.Background(), workspaceID, memberID)
	if err != nil {
		t.Fatal(err)
	}
	newOrganizationID := uuid.NewString()
	if err := db.Create(&workspacemodel.Organization{
		ID: newOrganizationID, Name: "Transferred Organization", Status: workspacemodel.OrganizationStatusActive,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&workspacemodel.Workspace{}).Where("id = ?", workspaceID).
		Update("organization_id", newOrganizationID).Error; err != nil {
		t.Fatal(err)
	}

	_, err = service.createRequest(context.Background(), staleScope, memberID, "development", CreateRequestInput{Purpose: "stale transfer"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("create request with stale workspace scope error = %v, want conflict", err)
	}
	var requests int64
	if err := db.Model(&accessmodel.AccessRequest{}).Where("workspace_id = ?", workspaceID).Count(&requests).Error; err != nil {
		t.Fatal(err)
	}
	if requests != 0 {
		t.Fatalf("stale workspace scope created %d access requests", requests)
	}
}

func TestReviewRequestRevalidatesReviewerAuthorityInTransaction(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, memberID := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	request, err := service.CreateRequest(context.Background(), workspaceID, memberID, CreateRequestInput{Purpose: "Review race"})
	if err != nil {
		t.Fatal(err)
	}
	staleScope, err := service.scope(context.Background(), workspaceID, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&workspacemodel.WorkspaceMember{}).
		Where("workspace_id = ? AND account_id = ?", workspaceID, ownerID).
		Update("role", workspacemodel.WorkspaceRoleMember).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := service.reviewRequest(context.Background(), staleScope, ownerID, request.ID, true, ReviewRequestInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("review with stale manager scope error = %v, want forbidden", err)
	}
	if err := db.First(request, "id = ?", request.ID).Error; err != nil {
		t.Fatal(err)
	}
	if request.Status != accessmodel.RequestStatusPending {
		t.Fatalf("unauthorized review changed request status to %q", request.Status)
	}
	var grants int64
	if err := db.Model(&accessmodel.Grant{}).Where("workspace_id = ? AND principal_id = ?", workspaceID, memberID).Count(&grants).Error; err != nil {
		t.Fatal(err)
	}
	if grants != 0 {
		t.Fatalf("unauthorized review created %d grants", grants)
	}
}

func TestEnsureGrantRevalidatesSelfServicePolicyInTransaction(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, memberID := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	if _, err := service.PutPolicy(context.Background(), workspaceID, ownerID, PolicyInput{
		Mode: accessmodel.AccessModeSelfService, MaxKeys: 2,
	}); err != nil {
		t.Fatal(err)
	}
	staleScope, err := service.scope(context.Background(), workspaceID, memberID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&accessmodel.Policy{}).Where("workspace_id = ?", workspaceID).
		Updates(map[string]any{"mode": accessmodel.AccessModeApprovalRequired, "version": gorm.Expr("version + 1")}).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := service.ensureGrant(context.Background(), staleScope, memberID); !errors.Is(err, ErrApprovalNeeded) {
		t.Fatalf("ensureGrant with stale self-service policy error = %v, want approval required", err)
	}
	var grants int64
	if err := db.Model(&accessmodel.Grant{}).Where("workspace_id = ? AND principal_id = ?", workspaceID, memberID).Count(&grants).Error; err != nil {
		t.Fatal(err)
	}
	if grants != 0 {
		t.Fatalf("stale self-service policy created %d grants", grants)
	}
}

func TestSetKeyStatusRevalidatesManagerAuthorityInTransaction(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, organizationID, ownerID, memberID := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	grant := accessmodel.Grant{
		OrganizationID: organizationID, WorkspaceID: workspaceID,
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: memberID,
		Source: "approved_request", Status: accessmodel.GrantStatusActive,
		MaxKeys: 2, AuthorizationVersion: 1,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	principalType := accessmodel.PrincipalTypeUser
	key := apikeymodel.TenantAPIKey{
		OrganizationID: organizationID, WorkspaceID: &workspaceID,
		PrincipalType: &principalType, PrincipalID: &memberID, AccessGrantID: &grant.ID,
		KeyHash: "member-key-manager-race", KeyPrefix: "zgi_race", KeySuffix: "race",
		SecretVersion: 2, Name: "member key", Status: "active", AuthorizationVersion: 1,
	}
	if err := db.Omit("Key").Create(&key).Error; err != nil {
		t.Fatal(err)
	}
	staleScope, err := service.scope(context.Background(), workspaceID, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&workspacemodel.WorkspaceMember{}).
		Where("workspace_id = ? AND account_id = ?", workspaceID, ownerID).
		Update("role", workspacemodel.WorkspaceRoleMember).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := service.setKeyStatus(context.Background(), staleScope, ownerID, key.ID, "inactive", "stale manager"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("status change with stale manager scope error = %v, want not found", err)
	}
	if err := db.First(&key, "id = ?", key.ID).Error; err != nil {
		t.Fatal(err)
	}
	if key.Status != "active" {
		t.Fatalf("stale manager changed member key status to %q", key.Status)
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

func TestSelfServiceRejoinedMemberRenewsRevokedGrantAsNewBudgetPeriod(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, organizationID, ownerID, memberID := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	quota, ttl := int64(900), int64(3600)
	if _, err := service.PutPolicy(context.Background(), workspaceID, ownerID, PolicyInput{
		Mode: accessmodel.AccessModeSelfService, DefaultQuota: &quota, MaxKeys: 2, DefaultTTLSeconds: &ttl,
	}); err != nil {
		t.Fatal(err)
	}
	oldQuota := int64(400)
	grant := accessmodel.Grant{
		OrganizationID: organizationID, WorkspaceID: workspaceID, PrincipalType: accessmodel.PrincipalTypeUser,
		PrincipalID: memberID, Source: "self_service", Status: accessmodel.GrantStatusRevoked,
		QuotaLimit: &oldQuota, UsedQuota: 300, RemainQuota: 100, MaxKeys: 1, AuthorizationVersion: 5,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	principalType := accessmodel.PrincipalTypeUser
	oldKey := apikeymodel.TenantAPIKey{
		OrganizationID: organizationID, WorkspaceID: &workspaceID, PrincipalType: &principalType,
		PrincipalID: &memberID, AccessGrantID: &grant.ID, CreatedByID: &memberID,
		KeyHash: "old-rejoined-key", KeyPrefix: "zgi_old", KeySuffix: "old1", SecretVersion: 2,
		Name: "before leaving", Status: "revoked", Environment: "development", AuthorizationVersion: 5,
	}
	if err := db.Omit("Key").Create(&oldKey).Error; err != nil {
		t.Fatal(err)
	}

	me, err := service.GetMe(context.Background(), workspaceID, memberID)
	if err != nil {
		t.Fatal(err)
	}
	if !me.CanCreateKey {
		t.Fatal("rejoined member should be able to start a new self-service period")
	}
	created, err := service.CreateKey(context.Background(), workspaceID, memberID, CreateKeyInput{Name: "after rejoining"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.Status != accessmodel.GrantStatusActive || grant.QuotaLimit == nil || *grant.QuotaLimit != quota ||
		grant.UsedQuota != 0 || grant.RemainQuota != quota || grant.MaxKeys != 2 || grant.AuthorizationVersion != 6 {
		t.Fatalf("unexpected renewed grant: %#v", grant)
	}
	var previous, replacement apikeymodel.TenantAPIKey
	if err := db.First(&previous, "id = ?", oldKey.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&replacement, "id = ?", created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if previous.Status != "revoked" || replacement.AuthorizationVersion != 6 {
		t.Fatalf("old/new key authorization state = %s/%d", previous.Status, replacement.AuthorizationVersion)
	}
}

func TestExhaustedGrantRejectsKeyCreationReactivationAndRotation(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, organizationID, _, memberID := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	quota := int64(100)
	grant := accessmodel.Grant{
		OrganizationID: organizationID, WorkspaceID: workspaceID, PrincipalType: accessmodel.PrincipalTypeUser,
		PrincipalID: memberID, Source: "approval", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &quota, UsedQuota: quota, RemainQuota: 0, MaxKeys: 3, AuthorizationVersion: 2,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	principalType := accessmodel.PrincipalTypeUser
	inactiveKey := apikeymodel.TenantAPIKey{
		OrganizationID: organizationID, WorkspaceID: &workspaceID, PrincipalType: &principalType,
		PrincipalID: &memberID, AccessGrantID: &grant.ID, CreatedByID: &memberID,
		KeyHash: "exhausted-grant-key", KeyPrefix: "zgi_exh", KeySuffix: "sted", SecretVersion: 2,
		Name: "exhausted", Status: "inactive", Environment: "development", AuthorizationVersion: 2,
	}
	if err := db.Omit("Key").Create(&inactiveKey).Error; err != nil {
		t.Fatal(err)
	}

	me, err := service.GetMe(context.Background(), workspaceID, memberID)
	if err != nil {
		t.Fatal(err)
	}
	if me.CanCreateKey {
		t.Fatal("exhausted finite grant must not advertise key creation")
	}
	if _, err := service.CreateKey(context.Background(), workspaceID, memberID, CreateKeyInput{Name: "unusable"}); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("CreateKey error = %v, want quota exceeded", err)
	}
	if _, err := service.SetKeyStatus(context.Background(), workspaceID, memberID, inactiveKey.ID, "active", ""); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("SetKeyStatus error = %v, want quota exceeded", err)
	}
	if _, err := service.RotateKey(context.Background(), workspaceID, memberID, inactiveKey.ID, RotateKeyInput{}); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("RotateKey error = %v, want quota exceeded", err)
	}
	var keys int64
	if err := db.Model(&apikeymodel.TenantAPIKey{}).Where("workspace_id = ? AND principal_id = ?", workspaceID, memberID).Count(&keys).Error; err != nil {
		t.Fatal(err)
	}
	if keys != 1 {
		t.Fatalf("key count = %d, want 1", keys)
	}
	if err := db.First(&inactiveKey, "id = ?", inactiveKey.ID).Error; err != nil {
		t.Fatal(err)
	}
	if inactiveKey.Status != "inactive" || inactiveKey.RevokedAt != nil {
		t.Fatalf("failed mutations changed exhausted key: %#v", inactiveKey)
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

func TestApprovalManagerRejoinedAfterRemovalRenewsRevokedGrant(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, organizationID, ownerID, _ := seedDeveloperWorkspace(t, db)
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	quota, ttl := int64(700), int64(3600)
	if _, err := service.PutPolicy(context.Background(), workspaceID, ownerID, PolicyInput{
		Mode: accessmodel.AccessModeApprovalRequired, DefaultQuota: &quota, MaxKeys: 2, DefaultTTLSeconds: &ttl,
	}); err != nil {
		t.Fatal(err)
	}
	oldQuota := int64(300)
	grant := accessmodel.Grant{
		OrganizationID: organizationID, WorkspaceID: workspaceID, PrincipalType: accessmodel.PrincipalTypeUser,
		PrincipalID: ownerID, Source: "workspace_admin", Status: accessmodel.GrantStatusRevoked,
		QuotaLimit: &oldQuota, UsedQuota: 200, RemainQuota: 100, MaxKeys: 1, AuthorizationVersion: 4,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateKey(context.Background(), workspaceID, ownerID, CreateKeyInput{Name: "manager after rejoin"}); err != nil {
		t.Fatalf("rejoined manager should renew without self-approval: %v", err)
	}
	if err := db.First(&grant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.Source != "workspace_admin" || grant.Status != accessmodel.GrantStatusActive || grant.QuotaLimit == nil ||
		*grant.QuotaLimit != quota || grant.UsedQuota != 0 || grant.RemainQuota != quota || grant.MaxKeys != 2 || grant.AuthorizationVersion != 5 {
		t.Fatalf("unexpected renewed rejoined-manager grant: %#v", grant)
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
	workspaceID, organizationID, ownerID, memberID := seedDeveloperWorkspace(t, db)
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
	previousOrganizationID := uuid.NewString()
	principalType := accessmodel.PrincipalTypeUser
	if err := db.Omit("Key").Create(&apikeymodel.TenantAPIKey{
		OrganizationID: previousOrganizationID,
		WorkspaceID:    &workspaceID,
		PrincipalType:  &principalType,
		PrincipalID:    &memberID,
		KeyHash:        uuid.NewString(),
		Name:           "retained key from previous organization",
		Status:         "revoked",
		SecretVersion:  2,
	}).Error; err != nil {
		t.Fatal(err)
	}
	page, err := service.ListKeys(context.Background(), workspaceID, ownerID, KeyQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Total != 1 || page.Items[0].PrincipalName != "Research Member" || page.Items[0].PrincipalEmail != "member@example.test" {
		t.Fatalf("principal identity missing or cross-organization key leaked for organization %s: %#v", organizationID, page)
	}
}

func TestKeyListIsPaginatedAndSeparatedByOwnership(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, organizationID, ownerID, memberID := seedDeveloperWorkspace(t, db)
	principalType := accessmodel.PrincipalTypeUser
	createdAt := time.Now().Add(-time.Hour)
	for i := 0; i < 25; i++ {
		key := apikeymodel.TenantAPIKey{
			OrganizationID: organizationID, WorkspaceID: &workspaceID,
			PrincipalType: &principalType, PrincipalID: &memberID,
			KeyHash: uuid.NewString(), Name: uuid.NewString(), Status: "revoked",
			SecretVersion: 2, CreatedAt: createdAt.Add(time.Duration(i) * time.Second),
		}
		if err := db.Omit("Key").Create(&key).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		key := apikeymodel.TenantAPIKey{
			OrganizationID: organizationID, WorkspaceID: &workspaceID,
			PrincipalType: &principalType, PrincipalID: &ownerID,
			KeyHash: uuid.NewString(), Name: uuid.NewString(), Status: "revoked", SecretVersion: 2,
		}
		if err := db.Omit("Key").Create(&key).Error; err != nil {
			t.Fatal(err)
		}
	}

	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	membersPage, err := service.ListKeys(context.Background(), workspaceID, ownerID, KeyQuery{Scope: "members", Page: 2, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if membersPage.Total != 25 || len(membersPage.Items) != 10 || membersPage.Page != 2 || membersPage.PageSize != 10 {
		t.Fatalf("unexpected member key page: %#v", membersPage)
	}
	for _, item := range membersPage.Items {
		if item.PrincipalID != memberID {
			t.Fatalf("member page leaked principal %s", item.PrincipalID)
		}
	}
	minePage, err := service.ListKeys(context.Background(), workspaceID, ownerID, KeyQuery{Scope: "mine", PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if minePage.Total != 3 || len(minePage.Items) != 3 {
		t.Fatalf("unexpected owner key page: %#v", minePage)
	}
	if _, err := service.ListKeys(context.Background(), workspaceID, memberID, KeyQuery{Scope: "members"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-manager member scope error = %v, want forbidden", err)
	}
	if _, err := service.ListKeys(context.Background(), workspaceID, ownerID, KeyQuery{Page: maxDeveloperAccessPage + 1}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized page error = %v, want invalid", err)
	}
}

func TestAccessRequestListIsPaginatedAndSeparatedByOwnership(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, organizationID, ownerID, memberID := seedDeveloperWorkspace(t, db)
	createdAt := time.Now().Add(-time.Hour)
	for i := 0; i < 25; i++ {
		status := accessmodel.RequestStatusApproved
		if i < 3 {
			status = accessmodel.RequestStatusPending
		}
		request := accessmodel.AccessRequest{
			OrganizationID: organizationID, WorkspaceID: workspaceID,
			RequesterAccountID: memberID, Purpose: uuid.NewString(), Environment: "development",
			RequestedModels: []string{}, Status: status, CreatedAt: createdAt.Add(time.Duration(i) * time.Second),
		}
		if err := db.Create(&request).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		request := accessmodel.AccessRequest{
			OrganizationID: organizationID, WorkspaceID: workspaceID,
			RequesterAccountID: ownerID, Purpose: uuid.NewString(), Environment: "development",
			RequestedModels: []string{}, Status: accessmodel.RequestStatusCancelled,
		}
		if err := db.Create(&request).Error; err != nil {
			t.Fatal(err)
		}
	}
	retained := accessmodel.AccessRequest{
		OrganizationID: uuid.NewString(), WorkspaceID: workspaceID,
		RequesterAccountID: memberID, Purpose: "retained request from previous organization", Environment: "development",
		RequestedModels: []string{}, Status: accessmodel.RequestStatusPending,
	}
	if err := db.Create(&retained).Error; err != nil {
		t.Fatal(err)
	}

	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	membersPage, err := service.ListRequests(context.Background(), workspaceID, ownerID, RequestQuery{Scope: "members", Page: 2, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if membersPage.Total != 25 || membersPage.PendingTotal != 3 || len(membersPage.Items) != 10 || membersPage.Page != 2 || membersPage.PageSize != 10 {
		t.Fatalf("unexpected member request page: %#v", membersPage)
	}
	for _, item := range membersPage.Items {
		if item.RequesterAccountID != memberID || item.OrganizationID != organizationID {
			t.Fatalf("member request page leaked item: %#v", item)
		}
	}
	minePage, err := service.ListRequests(context.Background(), workspaceID, ownerID, RequestQuery{Scope: "mine", PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if minePage.Total != 2 || len(minePage.Items) != 2 {
		t.Fatalf("unexpected owner request page: %#v", minePage)
	}
	if _, err := service.ListRequests(context.Background(), workspaceID, memberID, RequestQuery{Scope: "members"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-manager request scope error = %v, want forbidden", err)
	}
	if _, err := service.ListRequests(context.Background(), workspaceID, ownerID, RequestQuery{Page: maxDeveloperAccessPage + 1}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized request page error = %v, want invalid", err)
	}
	if _, err := service.ListRequests(context.Background(), workspaceID, ownerID, RequestQuery{Status: "unknown"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid request status error = %v, want invalid", err)
	}
}

func TestKeyMutationRejectsRetainedKeyFromPreviousOrganization(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, _, ownerID, _ := seedDeveloperWorkspace(t, db)
	previousOrganizationID := uuid.NewString()
	principalType := accessmodel.PrincipalTypeUser
	key := apikeymodel.TenantAPIKey{
		OrganizationID: previousOrganizationID,
		WorkspaceID:    &workspaceID,
		PrincipalType:  &principalType,
		PrincipalID:    &ownerID,
		KeyHash:        uuid.NewString(),
		Name:           "retained historical key",
		Status:         "revoked",
		SecretVersion:  2,
	}
	if err := db.Omit("Key").Create(&key).Error; err != nil {
		t.Fatal(err)
	}
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	if _, err := service.UpdateKey(context.Background(), workspaceID, ownerID, key.ID, UpdateKeyInput{Name: "must not change"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateKey error = %v, want not found for previous-organization key", err)
	}
	var stored apikeymodel.TenantAPIKey
	if err := db.First(&stored, "id = ?", key.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Name != key.Name {
		t.Fatalf("previous-organization key name changed to %q", stored.Name)
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
		attempt_id TEXT, entry_type TEXT, reserved_amount INTEGER, actual_amount INTEGER, refunded_amount INTEGER
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
	if err := db.Exec(`INSERT INTO billing_attempt_entries (attempt_id, entry_type, reserved_amount, actual_amount, refunded_amount)
		VALUES (?, ?, ?, ?, ?)`, "attempt-1", "subject", 20, 15, 5).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO llm_usage_bills
		(attempt_id, request_id, organization_id, workspace_id, principal_type, principal_id, auth_method,
		 api_key_id, model_name, provider_name, status, prompt_tokens, completion_tokens, total_tokens,
		 total_points, response_time_ms, request_created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"attempt-compensated", "request-compensated", organizationID, workspaceID, accessmodel.PrincipalTypeUser, memberID,
		"personal_api_key", key.ID, "music-test", "music-provider", "success", 0, 0, 0, 0, 100, time.Now().Add(time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO llm_usage_bills
		(attempt_id, request_id, organization_id, workspace_id, principal_type, principal_id, auth_method,
		 api_key_id, model_name, provider_name, status, prompt_tokens, completion_tokens, total_tokens,
		 total_points, response_time_ms, request_created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"attempt-previous-organization", "request-previous-organization", uuid.NewString(), workspaceID,
		accessmodel.PrincipalTypeUser, memberID, "personal_api_key", key.ID, "private-model", "private-provider",
		"success", 100, 50, 150, 200, 300, time.Now().Add(2*time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO billing_attempt_entries (attempt_id, entry_type, reserved_amount, actual_amount, refunded_amount)
		VALUES (?, ?, ?, ?, ?)`, "attempt-compensated", "subject", 20, 15, 20).Error; err != nil {
		t.Fatal(err)
	}
	adminPage, err := service.ListAudit(context.Background(), workspaceID, ownerID, AuditQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if adminPage.Total != 2 || len(adminPage.Items) != 2 {
		t.Fatalf("unexpected admin audit page: %#v", adminPage)
	}
	itemsByAttempt := make(map[string]AuditItem, len(adminPage.Items))
	for _, item := range adminPage.Items {
		itemsByAttempt[item.AttemptID] = item
	}
	ordinary := itemsByAttempt["attempt-1"]
	if ordinary.PrincipalEmail != "member@example.test" || ordinary.APIKeyName != "audited key" || ordinary.QuotaChargedPoints != 15 || ordinary.QuotaOveragePoints != 5 {
		t.Fatalf("unexpected ordinary audit item: %#v", ordinary)
	}
	compensated := itemsByAttempt["attempt-compensated"]
	if compensated.QuotaChargedPoints != 0 || compensated.QuotaOveragePoints != 0 || compensated.TotalPoints != 0 {
		t.Fatalf("compensated audit item still reports a charge: %#v", compensated)
	}
	memberPage, err := service.ListAudit(context.Background(), workspaceID, memberID, AuditQuery{PrincipalID: ownerID})
	if err != nil {
		t.Fatal(err)
	}
	if memberPage.Total != 2 || len(memberPage.Items) != 2 || memberPage.Items[0].PrincipalID != memberID || memberPage.Items[1].PrincipalID != memberID {
		t.Fatalf("member escaped own audit scope: %#v", memberPage)
	}
}

func TestNetSubjectChargeSubtractsOnlyCompensationRefund(t *testing.T) {
	tests := []struct {
		name                       string
		reserved, actual, refunded int64
		want                       int64
	}{
		{name: "ordinary estimate refund", reserved: 20, actual: 15, refunded: 5, want: 15},
		{name: "full delivery compensation", reserved: 20, actual: 15, refunded: 20, want: 0},
		{name: "actual exceeds estimate", reserved: 10, actual: 15, refunded: 0, want: 15},
		{name: "malformed over-refund clamps", reserved: 10, actual: 5, refunded: 99, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := netSubjectCharge(tt.reserved, tt.actual, tt.refunded); got != tt.want {
				t.Fatalf("netSubjectCharge(%d, %d, %d) = %d, want %d", tt.reserved, tt.actual, tt.refunded, got, tt.want)
			}
		})
	}
}

func seedDeveloperWorkspace(t *testing.T, db *gorm.DB) (workspaceID, organizationID, ownerID, memberID string) {
	t.Helper()
	workspaceID, organizationID, ownerID, memberID = uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	if err := db.Create(&workspacemodel.Organization{ID: organizationID, Name: "Developer Organization", Status: workspacemodel.OrganizationStatusActive}).Error; err != nil {
		t.Fatal(err)
	}
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
	page, err := service.ListKeys(context.Background(), workspaceID, ownerID, KeyQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Total != 1 || page.Items[0].KeyMasked == created.Secret {
		t.Fatalf("list leaked or lost the key: %#v", page)
	}
}

func TestZeroDefaultQuotaDoesNotAdvertiseOrCreateGrant(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, organizationID, _, memberID := seedDeveloperWorkspace(t, db)
	zero := int64(0)
	policy := accessmodel.Policy{
		OrganizationID: organizationID, WorkspaceID: workspaceID,
		Mode: accessmodel.AccessModeSelfService, DefaultQuota: &zero,
		MaxKeys: 2, AllowedModels: []string{}, Version: 1,
	}
	if err := db.Create(&policy).Error; err != nil {
		t.Fatal(err)
	}
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)

	me, err := service.GetMe(context.Background(), workspaceID, memberID)
	if err != nil {
		t.Fatal(err)
	}
	if me.CanCreateKey {
		t.Fatal("zero-quota policy must not advertise key creation")
	}
	if _, err := service.CreateKey(context.Background(), workspaceID, memberID, CreateKeyInput{Name: "zero quota"}); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("CreateKey error = %v, want quota exceeded", err)
	}
	var grantCount int64
	if err := db.Model(&accessmodel.Grant{}).Where("workspace_id = ? AND principal_id = ?", workspaceID, memberID).Count(&grantCount).Error; err != nil {
		t.Fatal(err)
	}
	if grantCount != 0 {
		t.Fatalf("zero-quota creation persisted %d grants, want 0", grantCount)
	}

	revoked := accessmodel.Grant{
		OrganizationID: organizationID, WorkspaceID: workspaceID,
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: memberID,
		Source: "self_service", Status: accessmodel.GrantStatusRevoked,
		QuotaLimit: &zero, RemainQuota: 0, MaxKeys: 2,
		AllowedModels: []string{}, AuthorizationVersion: 4,
	}
	if err := db.Create(&revoked).Error; err != nil {
		t.Fatal(err)
	}
	me, err = service.GetMe(context.Background(), workspaceID, memberID)
	if err != nil {
		t.Fatal(err)
	}
	if me.CanCreateKey {
		t.Fatal("zero-quota renewal must not advertise key creation")
	}
	if _, err := service.CreateKey(context.Background(), workspaceID, memberID, CreateKeyInput{Name: "zero quota renewal"}); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("renewal CreateKey error = %v, want quota exceeded", err)
	}
	if err := db.First(&revoked, "id = ?", revoked.ID).Error; err != nil {
		t.Fatal(err)
	}
	if revoked.Status != accessmodel.GrantStatusRevoked || revoked.AuthorizationVersion != 4 {
		t.Fatalf("failed zero-quota renewal mutated grant: %#v", revoked)
	}
}

func TestListKeysReportsAuthoritativeActivationCapability(t *testing.T) {
	db := openDeveloperAccessTestDB(t)
	workspaceID, organizationID, ownerID, memberID := seedDeveloperWorkspace(t, db)
	quota := int64(100)
	policy := accessmodel.Policy{
		OrganizationID: organizationID, WorkspaceID: workspaceID,
		Mode: accessmodel.AccessModeApprovalRequired, DefaultQuota: &quota,
		MaxKeys: 2, AllowedModels: []string{}, Version: 1,
	}
	grant := accessmodel.Grant{
		OrganizationID: organizationID, WorkspaceID: workspaceID,
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: memberID,
		Source: "approved_request", Status: accessmodel.GrantStatusActive,
		QuotaLimit: &quota, RemainQuota: quota, MaxKeys: 2,
		AllowedModels: []string{}, AuthorizationVersion: 3,
	}
	if err := db.Create(&policy).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	principalType := accessmodel.PrincipalTypeUser
	active := apikeymodel.TenantAPIKey{
		OrganizationID: organizationID, WorkspaceID: &workspaceID, PrincipalType: &principalType,
		PrincipalID: &memberID, AccessGrantID: &grant.ID, KeyHash: "activation-active",
		Name: "active", Status: "active", AuthorizationVersion: grant.AuthorizationVersion,
	}
	inactive := apikeymodel.TenantAPIKey{
		OrganizationID: organizationID, WorkspaceID: &workspaceID, PrincipalType: &principalType,
		PrincipalID: &memberID, AccessGrantID: &grant.ID, KeyHash: "activation-inactive",
		Name: "inactive", Status: "inactive", AuthorizationVersion: grant.AuthorizationVersion,
	}
	if err := db.Omit("Key").Create(&active).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Omit("Key").Create(&inactive).Error; err != nil {
		t.Fatal(err)
	}
	service := NewService(db, apikeyrepo.NewAPIKeyRepository(db), nil)
	keyView := func(accountID, keyID string) KeyView {
		page, err := service.ListKeys(context.Background(), workspaceID, accountID, KeyQuery{PageSize: 100})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			if item.ID == keyID {
				return item
			}
		}
		t.Fatalf("key %s missing from list", keyID)
		return KeyView{}
	}
	if view := keyView(ownerID, inactive.ID); !view.CanActivate || view.CanRotate {
		t.Fatalf("manager capabilities for eligible member key = %#v", view)
	}
	if view := keyView(memberID, inactive.ID); !view.CanActivate || !view.CanRotate {
		t.Fatalf("member capabilities for eligible own key = %#v", view)
	}
	if view := keyView(memberID, active.ID); !view.CanRotate {
		t.Fatalf("active key should remain rotatable at capacity: %#v", view)
	}
	if !keyView(ownerID, inactive.ID).CanActivate {
		t.Fatal("eligible inactive member key should be activatable")
	}
	if err := db.Model(&grant).Update("max_keys", 1).Error; err != nil {
		t.Fatal(err)
	}
	if view := keyView(memberID, inactive.ID); view.CanActivate || view.CanRotate {
		t.Fatal("key limit must suppress activation")
	}
	if view := keyView(memberID, active.ID); !view.CanRotate {
		t.Fatal("rotation must exclude the active key being replaced from the key limit")
	}
	if err := db.Model(&grant).Updates(map[string]any{"max_keys": 2, "remain_quota": 0}).Error; err != nil {
		t.Fatal(err)
	}
	if view := keyView(memberID, inactive.ID); view.CanActivate || view.CanRotate {
		t.Fatal("exhausted grant must suppress activation")
	}
	if err := db.Model(&grant).Updates(map[string]any{"remain_quota": quota, "authorization_version": 4}).Error; err != nil {
		t.Fatal(err)
	}
	if view := keyView(memberID, inactive.ID); view.CanActivate || view.CanRotate {
		t.Fatal("stale key authorization must suppress activation")
	}
	if err := db.Model(&grant).Update("authorization_version", 3).Error; err != nil {
		t.Fatal(err)
	}
	expiredAt := time.Now().Add(-time.Minute)
	if err := db.Model(&inactive).Update("expires_at", expiredAt).Error; err != nil {
		t.Fatal(err)
	}
	if view := keyView(memberID, inactive.ID); view.CanActivate || view.CanRotate {
		t.Fatalf("expired key exposed a lifecycle action: %#v", view)
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
	requests, err := service.ListRequests(context.Background(), workspaceID, ownerID, RequestQuery{Status: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	if len(requests.Items) != 1 || requests.Total != 1 || requests.PendingTotal != 1 || requests.Items[0].RequesterName != "Research Member" || requests.Items[0].RequesterEmail != "member@example.test" {
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
