package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestKnowledgeBaseAssetRefRepositoryActiveByAssetRequiresExistingDataset(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	createKnowledgeBaseAssetRefRelationTables(t, db)

	assetID := uuid.New()
	activeRefID := uuid.New()
	disabledRefID := uuid.New()
	orphanRefID := uuid.New()
	execKnowledgeBaseAssetRefSQL(t, db, `INSERT INTO datasets (id) VALUES (?)`, "dataset-active")
	execKnowledgeBaseAssetRefSQL(t, db, `INSERT INTO data_library_knowledge_base_asset_refs (id, organization_id, dataset_id, asset_id, status, sync_status, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL)`, activeRefID.String(), "org-1", "dataset-active", assetID.String(), model.KnowledgeBaseAssetRefStatusActive, model.KnowledgeBaseAssetRefSyncStatusSynced)
	execKnowledgeBaseAssetRefSQL(t, db, `INSERT INTO data_library_knowledge_base_asset_refs (id, organization_id, dataset_id, asset_id, status, sync_status, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL)`, disabledRefID.String(), "org-1", "dataset-active", assetID.String(), model.KnowledgeBaseAssetRefStatusDisabled, model.KnowledgeBaseAssetRefSyncStatusSynced)
	execKnowledgeBaseAssetRefSQL(t, db, `INSERT INTO data_library_knowledge_base_asset_refs (id, organization_id, dataset_id, asset_id, status, sync_status, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL)`, orphanRefID.String(), "org-1", "dataset-deleted", assetID.String(), model.KnowledgeBaseAssetRefStatusActive, model.KnowledgeBaseAssetRefSyncStatusSynced)

	repo := NewKnowledgeBaseAssetRefRepository(db)
	count, err := repo.CountActiveByAssetID(context.Background(), "org-1", assetID)
	if err != nil {
		t.Fatalf("count active by asset: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}

	refs, err := repo.ListActiveByAsset(context.Background(), "org-1", assetID)
	if err != nil {
		t.Fatalf("list active by asset: %v", err)
	}
	if len(refs) != 1 || refs[0].ID != activeRefID {
		t.Fatalf("expected only active dataset ref %s, got %+v", activeRefID, refs)
	}
}

func TestKnowledgeBaseAssetRefRepositoryFindActiveByAssetIgnoresDisabledRef(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	createKnowledgeBaseAssetRefRelationTables(t, db)

	assetID := uuid.New()
	disabledRefID := uuid.New()
	execKnowledgeBaseAssetRefSQL(t, db, `INSERT INTO datasets (id) VALUES (?)`, "dataset-active")
	execKnowledgeBaseAssetRefSQL(t, db, `INSERT INTO data_library_knowledge_base_asset_refs (id, organization_id, dataset_id, asset_id, status, sync_status, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL)`, disabledRefID.String(), "org-1", "dataset-active", assetID.String(), model.KnowledgeBaseAssetRefStatusDisabled, model.KnowledgeBaseAssetRefSyncStatusSynced)

	repo := NewKnowledgeBaseAssetRefRepository(db)
	ref, err := repo.FindActiveByAsset(context.Background(), "org-1", "dataset-active", assetID)
	if err != nil {
		t.Fatalf("find active by asset: %v", err)
	}
	if ref != nil {
		t.Fatalf("expected disabled ref to be ignored, got %+v", ref)
	}
}

func createKnowledgeBaseAssetRefRelationTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	statements := []string{
		`CREATE TABLE datasets (id text primary key)`,
		`CREATE TABLE data_library_knowledge_base_asset_refs (
			id text primary key,
			organization_id text,
			workspace_id text,
			dataset_id text,
			asset_id text,
			version_id text,
			dataset_document_id text,
			chunk_artifact_set_id text,
			vector_artifact_id text,
			status text,
			sync_status text,
			synced_generation_no integer,
			sync_run_id text,
			last_synced_at datetime,
			sync_error_code text,
			sync_error_message text,
			metadata_json text,
			created_by text,
			created_at datetime,
			updated_at datetime,
			deleted_at datetime
		)`,
	}
	for _, statement := range statements {
		execKnowledgeBaseAssetRefSQL(t, db, statement)
	}
}

func execKnowledgeBaseAssetRefSQL(t *testing.T, db *gorm.DB, sql string, args ...any) {
	t.Helper()
	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatalf("exec sql failed: %v\nsql: %s", err, sql)
	}
}
