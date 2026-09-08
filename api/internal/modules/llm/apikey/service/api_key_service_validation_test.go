package service

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	accessmodel "github.com/zgiai/zgi/api/internal/modules/llm/developeraccess/model"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	sharedredis "github.com/zgiai/zgi/api/pkg/redis"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestValidateAPIKeyRejectsInactivePrincipalGrant(t *testing.T) {
	quotaLimit := int64(100)
	tests := []struct {
		name         string
		grantVersion int64
		keyVersion   int64
		grantQuota   *int64
		remaining    int64
		orgStatus    workspacemodel.OrganizationStatus
	}{
		{name: "stale authorization period", grantVersion: 2, keyVersion: 1, orgStatus: workspacemodel.OrganizationStatusActive},
		{name: "exhausted bounded grant", grantVersion: 1, keyVersion: 1, grantQuota: &quotaLimit, remaining: 0, orgStatus: workspacemodel.OrganizationStatusActive},
		{name: "archived organization", grantVersion: 1, keyVersion: 1, remaining: 100, orgStatus: workspacemodel.OrganizationStatusArchived},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(
				&apikeymodel.TenantAPIKey{},
				&workspacemodel.Organization{},
				&workspacemodel.Workspace{},
				&workspacemodel.WorkspaceMember{},
				&accessmodel.Grant{},
				&accessmodel.Policy{},
			); err != nil {
				t.Fatal(err)
			}
			organizationID, workspaceID, principalID := uuid.NewString(), uuid.NewString(), uuid.NewString()
			if err := db.Create(&workspacemodel.Organization{ID: organizationID, Name: "Validation organization", Status: tt.orgStatus}).Error; err != nil {
				t.Fatal(err)
			}
			workspace := workspacemodel.Workspace{ID: workspaceID, OrganizationID: &organizationID, Name: "Validation workspace", Status: "normal"}
			if err := db.Create(&workspace).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&workspacemodel.WorkspaceMember{
				ID: uuid.NewString(), WorkspaceID: workspaceID, AccountID: principalID, Role: workspacemodel.WorkspaceRoleMember,
			}).Error; err != nil {
				t.Fatal(err)
			}
			grant := accessmodel.Grant{
				OrganizationID: organizationID, WorkspaceID: workspaceID,
				PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: principalID,
				Source: "approved_request", Status: accessmodel.GrantStatusActive,
				QuotaLimit: tt.grantQuota, RemainQuota: tt.remaining,
				MaxKeys: 1, AllowedModels: []string{}, AuthorizationVersion: tt.grantVersion,
			}
			if err := db.Create(&grant).Error; err != nil {
				t.Fatal(err)
			}
			secret := "zgi_validation_" + uuid.NewString()
			principalType := accessmodel.PrincipalTypeUser
			key := apikeymodel.TenantAPIKey{
				OrganizationID: organizationID, WorkspaceID: &workspaceID,
				PrincipalType: &principalType, PrincipalID: &principalID, AccessGrantID: &grant.ID,
				KeyHash: util.HashAPIKey(secret), Name: "inactive principal key", Status: "active",
				AuthorizationVersion: tt.keyVersion,
			}
			if err := db.Create(&key).Error; err != nil {
				t.Fatal(err)
			}

			repo := repository.NewAPIKeyRepository(db)
			service := NewAPIKeyService(db, repo, nil, nil)
			result, err := service.ValidateAPIKey(context.Background(), secret)
			if err != nil {
				t.Fatalf("validate inactive personal key: %v", err)
			}
			if result.Valid || result.Message != "API key principal access is not active" {
				t.Fatalf("inactive principal key validation = %#v", result)
			}
		})
	}
}

func TestValidateAPIKeyReloadsPersonalKeyBeforeCachedStatusCheck(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&apikeymodel.TenantAPIKey{},
		&workspacemodel.Organization{},
		&workspacemodel.Workspace{},
		&workspacemodel.WorkspaceMember{},
		&accessmodel.Grant{},
		&accessmodel.Policy{},
	); err != nil {
		t.Fatal(err)
	}
	miniRedis, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	client := redisclient.NewClient(&redisclient.Options{Addr: miniRedis.Addr()})
	previousClient := sharedredis.GetClient()
	sharedredis.SetClient(client)
	t.Cleanup(func() {
		sharedredis.SetClient(previousClient)
		_ = client.Close()
		miniRedis.Close()
	})
	organizationID, workspaceID, principalID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if err := db.Create(&workspacemodel.Organization{ID: organizationID, Name: "Cache organization", Status: workspacemodel.OrganizationStatusActive}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&workspacemodel.Workspace{ID: workspaceID, OrganizationID: &organizationID, Name: "Cache workspace", Status: "normal"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&workspacemodel.WorkspaceMember{ID: uuid.NewString(), WorkspaceID: workspaceID, AccountID: principalID, Role: workspacemodel.WorkspaceRoleMember}).Error; err != nil {
		t.Fatal(err)
	}
	grant := accessmodel.Grant{
		OrganizationID: organizationID, WorkspaceID: workspaceID,
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: principalID,
		Source: "approved_request", Status: accessmodel.GrantStatusActive,
		MaxKeys: 1, AllowedModels: []string{}, AuthorizationVersion: 1,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	secret := "zgi_reenabled_" + uuid.NewString()
	principalType := accessmodel.PrincipalTypeUser
	key := apikeymodel.TenantAPIKey{
		OrganizationID: organizationID, WorkspaceID: &workspaceID,
		PrincipalType: &principalType, PrincipalID: &principalID, AccessGrantID: &grant.ID,
		KeyHash: util.HashAPIKey(secret), Name: "re-enabled personal key", Status: "inactive",
		AuthorizationVersion: grant.AuthorizationVersion,
	}
	if err := db.Create(&key).Error; err != nil {
		t.Fatal(err)
	}

	repo := repository.NewAPIKeyRepository(db)
	// Populate the repository cache with the inactive row, then simulate a
	// successful re-enable whose best-effort cache deletion was lost.
	if _, err := repo.GetByKey(context.Background(), secret); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&apikeymodel.TenantAPIKey{}).Where("id = ?", key.ID).Update("status", "active").Error; err != nil {
		t.Fatal(err)
	}

	service := NewAPIKeyService(db, repo, nil, nil)
	result, err := service.ValidateAPIKey(context.Background(), secret)
	if err != nil {
		t.Fatalf("validate re-enabled personal key: %v", err)
	}
	if !result.Valid {
		t.Fatalf("re-enabled personal key stayed invalid from cached status: %#v", result)
	}
}
