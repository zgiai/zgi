package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	accessmodel "github.com/zgiai/zgi/api/internal/modules/llm/developeraccess/model"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
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
	}{
		{name: "stale authorization period", grantVersion: 2, keyVersion: 1},
		{name: "exhausted bounded grant", grantVersion: 1, keyVersion: 1, grantQuota: &quotaLimit, remaining: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(
				&apikeymodel.TenantAPIKey{},
				&workspacemodel.Workspace{},
				&workspacemodel.WorkspaceMember{},
				&accessmodel.Grant{},
				&accessmodel.Policy{},
			); err != nil {
				t.Fatal(err)
			}
			organizationID, workspaceID, principalID := uuid.NewString(), uuid.NewString(), uuid.NewString()
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
