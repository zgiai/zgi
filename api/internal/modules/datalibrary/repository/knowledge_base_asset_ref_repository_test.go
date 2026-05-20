package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/datalibrary/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestKnowledgeBaseAssetRefRepositoryCreateReadListAndFindActive(t *testing.T) {
	db := openKnowledgeBaseAssetRefRepoTestDB(t)
	repo := NewKnowledgeBaseAssetRefRepository(db)
	ctx := context.Background()
	assetID := uuid.New()
	versionID := uuid.New()
	chunkSetID := uuid.New()
	vectorID := uuid.New()

	item := &model.KnowledgeBaseAssetRef{
		OrganizationID:     "org-1",
		DatasetID:          uuid.NewString(),
		AssetID:            assetID,
		VersionID:          versionID,
		ChunkArtifactSetID: &chunkSetID,
		VectorArtifactID:   &vectorID,
		MetadataJSON: map[string]any{
			"retrieval_mode": "shared_vector",
		},
		CreatedBy: "account-1",
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if item.ID == uuid.Nil || item.Status != model.KnowledgeBaseAssetRefStatusActive {
		t.Fatalf("item=%+v", item)
	}

	got, err := repo.GetByID(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.DatasetID != item.DatasetID || got.CreatedBy != "account-1" {
		t.Fatalf("got=%+v", got)
	}

	items, total, err := repo.List(ctx, KnowledgeBaseAssetRefListFilter{
		OrganizationID: "org-1",
		DatasetID:      item.DatasetID,
		Status:         model.KnowledgeBaseAssetRefStatusActive,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("items=%+v total=%d", items, total)
	}

	active, err := repo.FindActive(ctx, "org-1", item.DatasetID, assetID, versionID)
	if err != nil {
		t.Fatalf("FindActive: %v", err)
	}
	if active == nil || active.ID != item.ID {
		t.Fatalf("active=%+v", active)
	}

	count, err := repo.CountActiveByAssetID(ctx, "org-1", assetID)
	if err != nil {
		t.Fatalf("CountActiveByAssetID: %v", err)
	}
	if count != 1 {
		t.Fatalf("count=%d", count)
	}

	disabled, err := repo.UpdateStatus(ctx, "org-1", item.ID, model.KnowledgeBaseAssetRefStatusDisabled)
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if disabled == nil || disabled.ID != item.ID || disabled.Status != model.KnowledgeBaseAssetRefStatusDisabled {
		t.Fatalf("disabled=%+v", disabled)
	}

	active, err = repo.FindActive(ctx, "org-1", item.DatasetID, assetID, versionID)
	if err != nil {
		t.Fatalf("FindActive after disable: %v", err)
	}
	if active != nil {
		t.Fatalf("expected no active ref after disable, got %+v", active)
	}

	count, err = repo.CountActiveByAssetID(ctx, "org-1", assetID)
	if err != nil {
		t.Fatalf("CountActiveByAssetID after disable: %v", err)
	}
	if count != 0 {
		t.Fatalf("count after disable=%d", count)
	}

	missing, err := repo.UpdateStatus(ctx, "org-2", item.ID, model.KnowledgeBaseAssetRefStatusActive)
	if err != nil {
		t.Fatalf("UpdateStatus wrong organization: %v", err)
	}
	if missing != nil {
		t.Fatalf("missing=%+v", missing)
	}
}

func openKnowledgeBaseAssetRefRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	schema := []string{
		`CREATE TABLE data_library_knowledge_base_asset_refs (
			id text PRIMARY KEY,
			organization_id text NOT NULL,
			workspace_id text,
			dataset_id text NOT NULL,
			asset_id text NOT NULL,
			version_id text NOT NULL,
			chunk_artifact_set_id text,
			vector_artifact_id text,
			status text NOT NULL DEFAULT 'active',
			metadata_json text NOT NULL DEFAULT '{}',
			created_by text,
			created_at datetime NOT NULL,
			updated_at datetime NOT NULL,
			deleted_at datetime
		)`,
		`CREATE INDEX idx_data_library_kb_asset_refs_org_dataset
			ON data_library_knowledge_base_asset_refs (organization_id, dataset_id, status)`,
	}
	for _, stmt := range schema {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create test schema: %v", err)
		}
	}
	return db
}
