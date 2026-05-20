package apikey_test

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	apikeymodel "github.com/zgiai/ginext/internal/modules/llm/apikey/model"
	apikeyrepo "github.com/zgiai/ginext/internal/modules/llm/apikey/repository"
	"gorm.io/gorm"
)

func TestAPIKeyRepositoryGetByIDInOrganizationsScopesByOrganization(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:apikey_repo_scope?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&apikeymodel.TenantAPIKey{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	keys := []*apikeymodel.TenantAPIKey{
		{ID: "key-allowed", OrganizationID: "org-allowed", Key: "encrypted-1", KeyHash: "hash-1", Name: "allowed", Status: "active"},
		{ID: "key-foreign", OrganizationID: "org-foreign", Key: "encrypted-2", KeyHash: "hash-2", Name: "foreign", Status: "active"},
		{ID: "key-internal", OrganizationID: "org-allowed", Key: "encrypted-3", KeyHash: "hash-3", Name: "internal", Status: "active", IsInternal: true},
	}
	if err := db.Create(&keys).Error; err != nil {
		t.Fatalf("seed api keys: %v", err)
	}

	repo := apikeyrepo.NewAPIKeyRepository(db)
	ctx := context.Background()

	got, err := repo.GetByIDInOrganizations(ctx, "key-allowed", []string{"org-allowed"})
	if err != nil {
		t.Fatalf("expected allowed key, got error: %v", err)
	}
	if got.ID != "key-allowed" || got.OrganizationID != "org-allowed" {
		t.Fatalf("got key %s org %s", got.ID, got.OrganizationID)
	}

	if _, err := repo.GetByIDInOrganizations(ctx, "key-foreign", []string{"org-allowed"}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected foreign key to be hidden, got %v", err)
	}

	if _, err := repo.GetByIDInOrganizations(ctx, "key-internal", []string{"org-allowed"}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected internal key to be hidden, got %v", err)
	}
}
