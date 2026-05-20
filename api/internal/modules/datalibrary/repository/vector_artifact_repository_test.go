package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/datalibrary/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestVectorArtifactRepositoryCreateListAndLatest(t *testing.T) {
	db := openVectorArtifactRepoTestDB(t)
	repo := NewVectorArtifactRepository(db)
	ctx := context.Background()
	assetID := uuid.New()
	versionID := uuid.New()
	chunkSetID := uuid.New()
	olderID := uuid.New()
	newerID := uuid.New()

	older := &model.VectorArtifact{
		ID:                 olderID,
		OrganizationID:     "org-1",
		AssetID:            assetID,
		VersionID:          versionID,
		ChunkArtifactSetID: chunkSetID,
		EmbeddingProvider:  "openai",
		EmbeddingModel:     "text-embedding-3-large",
		EmbeddingDimension: 3072,
		VectorCollection:   "data_library_vectors",
		VectorNamespace:    "workspace-1",
		VectorCount:        10,
		Status:             model.VectorArtifactStatusReady,
		CreatedAt:          time.Now().Add(-time.Minute),
	}
	newer := &model.VectorArtifact{
		ID:                 newerID,
		OrganizationID:     "org-1",
		AssetID:            assetID,
		VersionID:          versionID,
		ChunkArtifactSetID: chunkSetID,
		EmbeddingProvider:  "openai",
		EmbeddingModel:     "text-embedding-3-large",
		EmbeddingDimension: 3072,
		VectorCollection:   "data_library_vectors",
		VectorNamespace:    "workspace-1",
		VectorCount:        12,
		Status:             model.VectorArtifactStatusReady,
		CreatedAt:          time.Now(),
	}
	for _, item := range []*model.VectorArtifact{older, newer} {
		if err := repo.Create(ctx, item); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	items, total, err := repo.List(ctx, VectorArtifactListFilter{
		OrganizationID: "org-1",
		AssetID:        assetID,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 || len(items) != 2 || items[0].ID != newerID {
		t.Fatalf("items=%+v total=%d", items, total)
	}

	latest, err := repo.LatestReadyByVersionID(ctx, "org-1", versionID)
	if err != nil {
		t.Fatalf("LatestReadyByVersionID: %v", err)
	}
	if latest == nil || latest.ID != newerID || latest.VectorCount != 12 {
		t.Fatalf("latest=%+v", latest)
	}
}

func openVectorArtifactRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	schema := []string{
		`CREATE TABLE data_library_vector_artifacts (
			id text PRIMARY KEY,
			organization_id text NOT NULL,
			workspace_id text,
			asset_id text NOT NULL,
			version_id text NOT NULL,
			chunk_artifact_set_id text NOT NULL,
			embedding_provider text NOT NULL,
			embedding_model text NOT NULL,
			embedding_dimension integer NOT NULL DEFAULT 0,
			vector_collection text NOT NULL,
			vector_namespace text,
			vector_count integer NOT NULL DEFAULT 0,
			status text NOT NULL DEFAULT 'pending',
			content_hash text,
			metadata_json text NOT NULL DEFAULT '{}',
			created_by text,
			created_at datetime NOT NULL,
			updated_at datetime NOT NULL,
			deleted_at datetime
		)`,
		`CREATE INDEX idx_data_library_vector_artifacts_asset_created
			ON data_library_vector_artifacts (asset_id, created_at)`,
	}
	for _, stmt := range schema {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create test schema: %v", err)
		}
	}
	return db
}
