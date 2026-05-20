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

func TestExtractionArtifactRepositoryCreateListAndLatest(t *testing.T) {
	db := openExtractionArtifactRepoTestDB(t)
	repo := NewExtractionArtifactRepository(db)
	ctx := context.Background()
	assetID := uuid.New()
	versionID := uuid.New()
	parseArtifactID := uuid.New()
	olderID := uuid.New()
	newerID := uuid.New()
	score := 0.91

	older := &model.ExtractionArtifact{
		ID:                olderID,
		OrganizationID:    "org-1",
		AssetID:           assetID,
		VersionID:         versionID,
		ParseArtifactID:   &parseArtifactID,
		SchemaName:        "invoice",
		SchemaHash:        "schema-v1",
		ExtractorProvider: "openai",
		ExtractorModel:    "gpt-4.1-mini",
		RecordCount:       8,
		FieldCount:        6,
		EvidenceCount:     12,
		Status:            model.ExtractionArtifactStatusReady,
		QualityScore:      &score,
		CreatedAt:         time.Now().Add(-time.Minute),
	}
	newer := &model.ExtractionArtifact{
		ID:                newerID,
		OrganizationID:    "org-1",
		AssetID:           assetID,
		VersionID:         versionID,
		ParseArtifactID:   &parseArtifactID,
		SchemaName:        "invoice",
		SchemaHash:        "schema-v2",
		ExtractorProvider: "openai",
		ExtractorModel:    "gpt-4.1-mini",
		RecordCount:       10,
		FieldCount:        7,
		EvidenceCount:     15,
		Status:            model.ExtractionArtifactStatusReady,
		QualityScore:      &score,
		CreatedAt:         time.Now(),
	}
	for _, item := range []*model.ExtractionArtifact{older, newer} {
		if err := repo.Create(ctx, item); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	items, total, err := repo.List(ctx, ExtractionArtifactListFilter{
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
	if latest == nil || latest.ID != newerID || latest.RecordCount != 10 {
		t.Fatalf("latest=%+v", latest)
	}
}

func openExtractionArtifactRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	schema := []string{
		`CREATE TABLE data_library_extraction_artifacts (
			id text PRIMARY KEY,
			organization_id text NOT NULL,
			workspace_id text,
			asset_id text NOT NULL,
			version_id text NOT NULL,
			parse_artifact_id text,
			data_source_id text,
			table_id text,
			schema_name text,
			schema_hash text,
			extractor_provider text,
			extractor_model text,
			record_count integer NOT NULL DEFAULT 0,
			field_count integer NOT NULL DEFAULT 0,
			evidence_count integer NOT NULL DEFAULT 0,
			status text NOT NULL DEFAULT 'pending',
			quality_score real,
			content_hash text,
			output_uri text,
			metadata_json text NOT NULL DEFAULT '{}',
			created_by text,
			created_at datetime NOT NULL,
			updated_at datetime NOT NULL,
			deleted_at datetime
		)`,
		`CREATE INDEX idx_data_library_extraction_artifacts_asset_created
			ON data_library_extraction_artifacts (asset_id, created_at)`,
	}
	for _, stmt := range schema {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create test schema: %v", err)
		}
	}
	return db
}
