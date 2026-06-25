package modelmeta

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type catalogApplyCacheInvalidatorFake struct {
	calls int
}

func (f *catalogApplyCacheInvalidatorFake) InvalidateModelCache(context.Context) {
	f.calls++
}

func openCatalogApplyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	statements := []string{
		`CREATE TABLE llm_providers (id text PRIMARY KEY, provider text, deleted_at datetime, updated_at datetime)`,
		`CREATE TABLE llm_models (id text PRIMARY KEY, provider text, name text, deleted_at datetime)`,
		`CREATE TABLE llm_catalog_sync_states (sync_key text PRIMARY KEY, last_applied_version integer, last_applied_at datetime, last_error text, created_at datetime, updated_at datetime)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create test table: %v", err)
		}
	}
	return db
}

func TestApplyPublishedCatalogInvalidatesModelCacheAfterSuccess(t *testing.T) {
	previous := currentModelCacheInvalidator()
	t.Cleanup(func() {
		SetModelCacheInvalidator(previous)
	})
	invalidator := &catalogApplyCacheInvalidatorFake{}
	SetModelCacheInvalidator(invalidator)

	svc := NewService(openCatalogApplyTestDB(t))
	err := svc.ApplyPublishedCatalog(context.Background(), PublishedCatalog{
		Version:     1,
		PublishedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("ApplyPublishedCatalog returned error: %v", err)
	}
	if invalidator.calls != 1 {
		t.Fatalf("InvalidateModelCache calls = %d, want 1", invalidator.calls)
	}
}
