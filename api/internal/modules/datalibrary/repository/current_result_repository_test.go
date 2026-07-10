package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCurrentResultRepositories(t *testing.T) {
	db := openCurrentResultRepositoryTestDB(t)
	ctx := context.Background()
	organizationID := "org-1"
	assetID := uuid.New()
	runID := uuid.New()
	generationNo := int64(3)

	if err := db.Create(&model.DocumentAsset{
		ID:              assetID,
		OrganizationID:  organizationID,
		Title:           "guide.pdf",
		SourceFileID:    "file-1",
		ProcessingRunID: &runID,
		GenerationNo:    generationNo,
		MetadataJSON: map[string]any{
			"source":               "file_upload",
			"final_parse_provider": "old",
		},
	}).Error; err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	assetRepo := NewDocumentAssetRepository(db)
	ready := model.DocumentAssetProductStatusReady
	progress := 100
	gotAsset, err := assetRepo.UpdateCurrentResult(ctx, assetID, DocumentAssetCurrentResultPatch{
		OrganizationID:         organizationID,
		ProductStatus:          &ready,
		ProcessingProgress:     &progress,
		MetadataJSON:           map[string]any{"final_parse_provider": "reducto"},
		RequireProcessingRunID: &runID,
		RequireGenerationNo:    &generationNo,
	})
	if err != nil {
		t.Fatalf("UpdateCurrentResult: %v", err)
	}
	if gotAsset == nil || gotAsset.ProductStatus != ready || gotAsset.ProcessingProgress != progress {
		t.Fatalf("asset patch not applied: %+v", gotAsset)
	}
	if gotAsset.MetadataJSON["source"] != "file_upload" || gotAsset.MetadataJSON["final_parse_provider"] != "reducto" {
		t.Fatalf("metadata should be merged, got %+v", gotAsset.MetadataJSON)
	}

	confirmationRepo := NewParseConfirmationItemRepository(db)
	item := &model.ParseConfirmationItem{
		OrganizationID:  organizationID,
		AssetID:         assetID,
		ProcessingRunID: runID,
		GenerationNo:    generationNo,
		ItemType:        model.ParseConfirmationItemTypeLowConfidenceText,
		OriginalContent: "teh",
	}
	if err := confirmationRepo.Create(ctx, item); err != nil {
		t.Fatalf("create confirmation: %v", err)
	}
	pending, err := confirmationRepo.CountPendingByRun(ctx, organizationID, assetID, runID, generationNo)
	if err != nil || pending != 1 {
		t.Fatalf("pending count=%d err=%v", pending, err)
	}
	finalContent := "the"
	resolved, err := confirmationRepo.Resolve(ctx, item.ID, ParseConfirmationItemResolvePatch{
		OrganizationID: organizationID,
		Status:         model.ParseConfirmationItemStatusEdited,
		FinalContent:   &finalContent,
		AllowedFrom:    []string{model.ParseConfirmationItemStatusPending},
	})
	if err != nil {
		t.Fatalf("resolve confirmation: %v", err)
	}
	if resolved == nil || resolved.Status != model.ParseConfirmationItemStatusEdited || resolved.FinalContent == nil || *resolved.FinalContent != finalContent {
		t.Fatalf("confirmation not resolved: %+v", resolved)
	}

	chunkRepo := NewDocumentChunkRepository(db)
	chunk := &model.DocumentChunk{
		OrganizationID:  organizationID,
		AssetID:         assetID,
		ProcessingRunID: runID,
		GenerationNo:    generationNo,
		Position:        1,
		ChunkType:       model.DocumentChunkTypeAuto,
		Content:         "hello",
		ContentHash:     "hash-1",
	}
	if err := chunkRepo.Create(ctx, chunk); err != nil {
		t.Fatalf("create chunk: %v", err)
	}
	chunks, total, err := chunkRepo.List(ctx, DocumentChunkListFilter{
		OrganizationID: organizationID,
		AssetID:        assetID,
		GenerationNo:   &generationNo,
		Status:         model.DocumentChunkStatusReady,
	})
	if err != nil || total != 1 || len(chunks) != 1 || chunks[0].ID != chunk.ID {
		t.Fatalf("chunk list total=%d len=%d err=%v", total, len(chunks), err)
	}
	updatedContent := "hello world"
	updatedHash := "hash-2"
	updatedChunk, err := chunkRepo.Update(ctx, chunk.ID, DocumentChunkPatch{
		OrganizationID: organizationID,
		Content:        &updatedContent,
		ContentHash:    &updatedHash,
	})
	if err != nil {
		t.Fatalf("update chunk: %v", err)
	}
	if updatedChunk.Content != updatedContent || updatedChunk.ContentHash != updatedHash {
		t.Fatalf("chunk not updated: %+v", updatedChunk)
	}

	embeddingRepo := NewDocumentChunkEmbeddingRepository(db)
	embedding := &model.DocumentChunkEmbedding{
		OrganizationID:     organizationID,
		AssetID:            assetID,
		ChunkID:            chunk.ID,
		ProcessingRunID:    runID,
		GenerationNo:       generationNo,
		EmbeddingProvider:  "openai",
		EmbeddingModel:     "text-embedding-3-small",
		EmbeddingDimension: 3,
		EmbeddingVector:    model.Float32Array{0.1, 0.2, 0.3},
		ContentHash:        updatedHash,
	}
	if err := embeddingRepo.Upsert(ctx, embedding); err != nil {
		t.Fatalf("upsert embedding: %v", err)
	}
	embedding.EmbeddingVector = model.Float32Array{0.4, 0.5, 0.6}
	embedding.ContentHash = "hash-3"
	if err := embeddingRepo.Upsert(ctx, embedding); err != nil {
		t.Fatalf("upsert embedding second time: %v", err)
	}
	gotEmbedding, err := embeddingRepo.FindByChunkModel(ctx, chunk.ID, "openai", "text-embedding-3-small")
	if err != nil {
		t.Fatalf("find embedding: %v", err)
	}
	if gotEmbedding == nil || gotEmbedding.ContentHash != "hash-3" || len(gotEmbedding.EmbeddingVector) != 3 || gotEmbedding.EmbeddingVector[0] != 0.4 {
		t.Fatalf("embedding not upserted: %+v", gotEmbedding)
	}
}

func openCurrentResultRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`
		CREATE TABLE data_library_document_assets (
			id text primary key,
			organization_id text not null,
			workspace_id text,
			title text not null,
			source_file_id text not null,
			current_version_id text,
			content_hash text,
			status text not null default 'archived',
			processing_level text not null default 'archive',
			product_status text not null default 'stored_only',
			processing_stage text,
			processing_progress integer not null default 0,
			active_processing_request_id text,
			processing_run_id text,
			generation_no integer not null default 0,
			parse_artifact_id text,
			chunk_artifact_set_id text,
			chunk_count integer not null default 0,
			embedding_provider text,
			embedding_model text,
			embedding_dimension integer,
			vector_status text not null default 'none',
			last_error_code text,
			last_error_message text,
			quality_score real,
			metadata_json text not null default '{}',
			permission_policy text not null default '{}',
			created_by text,
			created_at datetime,
			updated_at datetime,
			deleted_at datetime
		)
		`,
		`
		CREATE TABLE data_library_parse_confirmation_items (
			id text primary key,
			organization_id text not null,
			workspace_id text,
			asset_id text not null,
			processing_run_id text not null,
			generation_no integer not null,
			item_type text not null,
			status text not null default 'pending',
			source_locator_json text not null default '{}',
			original_content text not null,
			suggested_content text,
			final_content text,
			confidence real,
			review_reason text,
			created_by text,
			updated_by text,
			resolved_at datetime,
			created_at datetime,
			updated_at datetime,
			deleted_at datetime
		)
		`,
		`
		CREATE TABLE data_library_document_chunks (
			id text primary key,
			organization_id text not null,
			workspace_id text,
			asset_id text not null,
			processing_run_id text not null,
			generation_no integer not null,
			chunk_artifact_set_id text,
			parent_chunk_id text,
			position integer not null default 0,
			chunk_type text not null,
			content text not null,
			content_hash text not null,
			source_locator_json text not null default '{}',
			enabled boolean not null default true,
			status text not null default 'ready',
			metadata_json text not null default '{}',
			created_by text,
			updated_by text,
			created_at datetime,
			updated_at datetime,
			deleted_at datetime
		)
		`,
		`
		CREATE TABLE data_library_document_chunk_embeddings (
			id text primary key,
			organization_id text not null,
			workspace_id text,
			asset_id text not null,
			chunk_id text not null,
			processing_run_id text not null,
			generation_no integer not null,
			embedding_provider text not null,
			embedding_model text not null,
			embedding_dimension integer not null default 0,
			embedding_vector text not null default '{}',
			content_hash text not null,
			status text not null default 'ready',
			metadata_json text not null default '{}',
			created_at datetime,
			updated_at datetime,
			deleted_at datetime
		)
		`,
		`
		CREATE UNIQUE INDEX uq_data_library_chunk_embeddings_active_model
		ON data_library_document_chunk_embeddings (chunk_id, embedding_provider, embedding_model)
		WHERE deleted_at IS NULL AND status <> 'deleted'
		`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create test table: %v", err)
		}
	}
	return db
}
