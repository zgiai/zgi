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

func TestReuseEventRepositoryCreateReadAndList(t *testing.T) {
	db := openReuseEventRepoTestDB(t)
	repo := NewReuseEventRepository(db)
	ctx := context.Background()
	assetID := uuid.New()
	versionID := uuid.New()
	artifactID := uuid.New()

	event := &model.ReuseEvent{
		OrganizationID:  "org-1",
		AssetID:         assetID,
		VersionID:       &versionID,
		ArtifactType:    model.ReuseArtifactChunkArtifact,
		ArtifactID:      &artifactID,
		ConsumerType:    model.ReuseConsumerKnowledgeBase,
		ConsumerID:      "kb-1",
		SavedSeconds:    42,
		SavedCostMicros: 1200,
	}
	if err := repo.Create(ctx, event); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if event.ID == uuid.Nil {
		t.Fatal("expected id")
	}

	got, err := repo.GetByID(ctx, event.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.ConsumerID != "kb-1" || got.ArtifactType != model.ReuseArtifactChunkArtifact {
		t.Fatalf("event=%+v", got)
	}

	items, total, err := repo.List(ctx, ReuseEventListFilter{
		OrganizationID: "org-1",
		AssetID:        &assetID,
		ConsumerType:   model.ReuseConsumerKnowledgeBase,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != event.ID {
		t.Fatalf("items=%+v total=%d", items, total)
	}
}

func TestReuseEventRepositorySummarizesSavingsByAssetID(t *testing.T) {
	db := openReuseEventRepoTestDB(t)
	repo := NewReuseEventRepository(db)
	ctx := context.Background()
	assetID := uuid.New()

	events := []*model.ReuseEvent{
		{
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ArtifactType:    model.ReuseArtifactParseArtifact,
			ConsumerType:    model.ReuseConsumerDatabase,
			ConsumerID:      "db-1",
			SavedSeconds:    10,
			SavedCostMicros: 100,
			CreatedAt:       time.Now().Add(time.Minute),
		},
		{
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ArtifactType:    model.ReuseArtifactChunkArtifact,
			ConsumerType:    model.ReuseConsumerKnowledgeBase,
			ConsumerID:      "kb-1",
			SavedSeconds:    20,
			SavedCostMicros: 200,
			CreatedAt:       time.Now(),
		},
		{
			OrganizationID:  "org-2",
			AssetID:         assetID,
			ArtifactType:    model.ReuseArtifactChunkArtifact,
			ConsumerType:    model.ReuseConsumerKnowledgeBase,
			ConsumerID:      "kb-2",
			SavedSeconds:    999,
			SavedCostMicros: 999,
		},
	}
	for _, event := range events {
		if err := repo.Create(ctx, event); err != nil {
			t.Fatalf("Create seed: %v", err)
		}
	}

	seconds, costMicros, err := repo.SumSavingsByAssetID(ctx, "org-1", assetID)
	if err != nil {
		t.Fatalf("SumSavingsByAssetID: %v", err)
	}
	if seconds != 30 || costMicros != 300 {
		t.Fatalf("seconds=%d costMicros=%d", seconds, costMicros)
	}

	reuseCount, seconds, costMicros, err := repo.SummaryByAssetID(ctx, "org-1", assetID)
	if err != nil {
		t.Fatalf("SummaryByAssetID: %v", err)
	}
	if reuseCount != 2 || seconds != 30 || costMicros != 300 {
		t.Fatalf("reuseCount=%d seconds=%d costMicros=%d", reuseCount, seconds, costMicros)
	}
}

func openReuseEventRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	schema := []string{
		`CREATE TABLE data_library_reuse_events (
			id text PRIMARY KEY,
			organization_id text NOT NULL,
			workspace_id text,
			asset_id text NOT NULL,
			version_id text,
			artifact_type text NOT NULL,
			artifact_id text,
			consumer_type text NOT NULL,
			consumer_id text NOT NULL,
			consumer_version text,
			saved_seconds integer NOT NULL DEFAULT 0,
			saved_cost_micros integer NOT NULL DEFAULT 0,
			metadata_json text NOT NULL DEFAULT '{}',
			created_by text,
			created_at datetime NOT NULL,
			deleted_at datetime
		)`,
		`CREATE INDEX idx_data_library_reuse_events_org_consumer
			ON data_library_reuse_events (organization_id, consumer_type, consumer_id, created_at)`,
		`CREATE INDEX idx_data_library_reuse_events_asset_created
			ON data_library_reuse_events (asset_id, created_at)`,
	}
	for _, stmt := range schema {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create test schema: %v", err)
		}
	}
	return db
}
