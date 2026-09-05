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

func TestValidateAPIKeyRejectsStalePrincipalGrant(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&apikeymodel.TenantAPIKey{},
		&workspacemodel.Workspace{},
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
	grant := accessmodel.Grant{
		OrganizationID: organizationID, WorkspaceID: workspaceID,
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: principalID,
		Source: "approved_request", Status: accessmodel.GrantStatusActive,
		MaxKeys: 1, AllowedModels: []string{}, AuthorizationVersion: 2,
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	secret := "zgi_validation_stale_principal_secret"
	principalType := accessmodel.PrincipalTypeUser
	key := apikeymodel.TenantAPIKey{
		OrganizationID: organizationID, WorkspaceID: &workspaceID,
		PrincipalType: &principalType, PrincipalID: &principalID, AccessGrantID: &grant.ID,
		KeyHash: util.HashAPIKey(secret), Name: "stale personal key", Status: "active",
		AuthorizationVersion: 1,
	}
	if err := db.Create(&key).Error; err != nil {
		t.Fatal(err)
	}

	repo := repository.NewAPIKeyRepository(db)
	service := NewAPIKeyService(db, repo, nil, nil)
	result, err := service.ValidateAPIKey(context.Background(), secret)
	if err != nil {
		t.Fatalf("validate stale personal key: %v", err)
	}
	if result.Valid || result.Message != "API key principal access is not active" {
		t.Fatalf("stale principal key validation = %#v", result)
	}
}
