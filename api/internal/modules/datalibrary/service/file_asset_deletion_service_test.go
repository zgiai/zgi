package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	contentparsemodel "github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFileAssetDeletionServiceBlocksReferencedAsset(t *testing.T) {
	db := newFileAssetDeletionTestDB(t)
	assetID := uuid.New()
	datasetID := uuid.New()
	execFileAssetDeletionSQL(t, db, "INSERT INTO datasets (id) VALUES (?)", datasetID.String())
	execFileAssetDeletionSQL(t, db, "INSERT INTO data_library_document_assets (id, organization_id, title, source_file_id, generation_no, updated_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)", assetID.String(), "org-1", "doc.md", "file-1", 1)
	execFileAssetDeletionSQL(t, db, "INSERT INTO data_library_knowledge_base_asset_refs (id, organization_id, dataset_id, asset_id, version_id, status) VALUES (?, ?, ?, ?, ?, ?)", uuid.New().String(), "org-1", datasetID.String(), assetID.String(), uuid.New().String(), model.KnowledgeBaseAssetRefStatusActive)

	vectorIndex := &fileAssetDeletionVectorIndex{}
	svc := NewFileAssetDeletionService(db, vectorIndex)
	err := svc.DeleteBySourceFile(context.Background(), "org-1", "file-1")
	if !errors.Is(err, ErrFileAssetDeletionBlocked) {
		t.Fatalf("err=%v want ErrFileAssetDeletionBlocked", err)
	}
	if vectorIndex.deleted {
		t.Fatalf("vector index should not be deleted when refs exist")
	}
}

func TestFileAssetDeletionServiceIgnoresRefsForDeletedDatasets(t *testing.T) {
	db := newFileAssetDeletionTestDB(t)
	assetID := uuid.New()
	execFileAssetDeletionSQL(t, db, "INSERT INTO data_library_document_assets (id, organization_id, title, source_file_id, generation_no, updated_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)", assetID.String(), "org-1", "doc.md", "file-1", 1)
	execFileAssetDeletionSQL(t, db, "INSERT INTO data_library_knowledge_base_asset_refs (id, organization_id, dataset_id, asset_id, version_id, status) VALUES (?, ?, ?, ?, ?, ?)", uuid.New().String(), "org-1", uuid.New().String(), assetID.String(), uuid.New().String(), model.KnowledgeBaseAssetRefStatusActive)

	vectorIndex := &fileAssetDeletionVectorIndex{}
	svc := NewFileAssetDeletionService(db, vectorIndex)
	if err := svc.DeleteBySourceFile(context.Background(), "org-1", "file-1"); err != nil {
		t.Fatalf("DeleteBySourceFile: %v", err)
	}
	if !vectorIndex.deleted {
		t.Fatalf("vector index should be deleted when only deleted dataset refs exist")
	}
}

func TestFileAssetDeletionServiceIgnoresRemovedRefs(t *testing.T) {
	db := newFileAssetDeletionTestDB(t)
	assetID := uuid.New()
	execFileAssetDeletionSQL(t, db, "INSERT INTO data_library_document_assets (id, organization_id, title, source_file_id, generation_no, updated_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)", assetID.String(), "org-1", "doc.md", "file-1", 1)
	execFileAssetDeletionSQL(t, db, "INSERT INTO data_library_knowledge_base_asset_refs (id, organization_id, dataset_id, asset_id, version_id, status, deleted_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)", uuid.New().String(), "org-1", uuid.New().String(), assetID.String(), uuid.New().String(), model.KnowledgeBaseAssetRefStatusActive)

	vectorIndex := &fileAssetDeletionVectorIndex{}
	svc := NewFileAssetDeletionService(db, vectorIndex)
	if err := svc.DeleteBySourceFile(context.Background(), "org-1", "file-1"); err != nil {
		t.Fatalf("DeleteBySourceFile: %v", err)
	}
	if !vectorIndex.deleted {
		t.Fatalf("vector index should be deleted when only removed refs exist")
	}
}

func TestFileAssetDeletionServiceDeletesAssetRowsAndVectorIndex(t *testing.T) {
	db := newFileAssetDeletionTestDB(t)
	assetID := uuid.New()
	runID := uuid.New()
	chunkID := uuid.New()
	parseArtifactID := uuid.New()
	chunkArtifactSetID := uuid.New()
	versionID := uuid.New()

	execFileAssetDeletionSQL(t, db, "INSERT INTO content_parse_artifacts (id, source_content_hash, profile, canonical_ir_version, provider_signature) VALUES (?, ?, ?, ?, ?)", parseArtifactID.String(), "hash", "default", "v1", "provider")
	execFileAssetDeletionSQL(t, db, "INSERT INTO content_parse_chunk_artifact_sets (id, parse_artifact_id, source_content_hash, use_case, planner_name, chunker_version, signature, content_hash) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", chunkArtifactSetID.String(), parseArtifactID.String(), "hash", "file", "planner", "v1", "sig", "chunk-hash")
	execFileAssetDeletionSQL(t, db, "INSERT INTO content_parse_runs (id, artifact_id, source_type, intent, profile, status, quality_level) VALUES (?, ?, ?, ?, ?, ?, ?)", runID.String(), parseArtifactID.String(), "upload_file", "parse", "default", "succeeded", "standard")
	execFileAssetDeletionSQL(t, db, "INSERT INTO content_parse_chunking_runs (id, parse_run_id, chunk_artifact_set_id, use_case, planner_name) VALUES (?, ?, ?, ?, ?)", uuid.New().String(), runID.String(), chunkArtifactSetID.String(), "file", "planner")
	execFileAssetDeletionSQL(t, db, "INSERT INTO data_library_document_assets (id, organization_id, title, source_file_id, parse_artifact_id, chunk_artifact_set_id, generation_no, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)", assetID.String(), "org-1", "doc.md", "file-1", parseArtifactID.String(), chunkArtifactSetID.String(), 1)
	execFileAssetDeletionSQL(t, db, "INSERT INTO data_library_document_versions (id, asset_id, version_no, source_file_id, parse_artifact_id, chunk_artifact_set_id) VALUES (?, ?, ?, ?, ?, ?)", versionID.String(), assetID.String(), 1, "file-1", parseArtifactID.String(), chunkArtifactSetID.String())
	execFileAssetDeletionSQL(t, db, "INSERT INTO data_library_document_chunks (id, organization_id, asset_id, processing_run_id, generation_no, chunk_artifact_set_id, chunk_type, content, content_hash, enabled, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", chunkID.String(), "org-1", assetID.String(), runID.String(), 1, chunkArtifactSetID.String(), model.DocumentChunkTypeChild, "hello", "chunk-hash", true, model.DocumentChunkStatusReady)
	execFileAssetDeletionSQL(t, db, "INSERT INTO data_library_document_chunk_embeddings (id, organization_id, asset_id, chunk_id, processing_run_id, generation_no, embedding_provider, embedding_model, embedding_dimension, embedding_vector, content_hash, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", uuid.New().String(), "org-1", assetID.String(), chunkID.String(), runID.String(), 1, "qwen", "text-embedding-v4", 3, "{1,2,3}", "chunk-hash", model.DocumentChunkEmbeddingStatusReady)
	execFileAssetDeletionSQL(t, db, "INSERT INTO data_library_processing_requests (id, organization_id, asset_id, target_level, status) VALUES (?, ?, ?, ?, ?)", uuid.New().String(), "org-1", assetID.String(), model.DocumentProcessingLevelFull, model.ProcessingRequestStatusRunning)

	vectorIndex := &fileAssetDeletionVectorIndex{}
	svc := NewFileAssetDeletionService(db, vectorIndex)
	if err := svc.DeleteBySourceFile(context.Background(), "org-1", "file-1"); err != nil {
		t.Fatalf("DeleteBySourceFile: %v", err)
	}
	if !vectorIndex.deleted || vectorIndex.assetID != assetID {
		t.Fatalf("vector index deleted=%v asset=%s", vectorIndex.deleted, vectorIndex.assetID)
	}

	assertTableCount(t, db, &model.DocumentAsset{}, 0)
	assertTableCount(t, db, &model.DocumentVersion{}, 0)
	assertTableCount(t, db, &model.DocumentChunk{}, 0)
	assertTableCount(t, db, &model.DocumentChunkEmbedding{}, 0)
	assertTableCount(t, db, &model.ProcessingRequest{}, 0)
	assertTableCount(t, db, &contentparsemodel.ChunkingRun{}, 0)
	assertTableCount(t, db, &contentparsemodel.ParseRun{}, 0)
	assertTableCount(t, db, &contentparsemodel.ChunkArtifactSet{}, 0)
	assertTableCount(t, db, &contentparsemodel.Artifact{}, 0)
}

func newFileAssetDeletionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	statements := []string{
		`CREATE TABLE data_library_document_assets (id text primary key, organization_id text, title text, source_file_id text, current_version_id text, active_processing_request_id text, parse_artifact_id text, chunk_artifact_set_id text, generation_no integer, updated_at datetime, deleted_at datetime)`,
		`CREATE TABLE datasets (id text primary key)`,
		`CREATE TABLE data_library_document_versions (id text primary key, asset_id text, version_no integer, source_file_id text, parse_artifact_id text, chunk_artifact_set_id text, deleted_at datetime)`,
		`CREATE TABLE data_library_document_chunks (id text primary key, organization_id text, asset_id text, processing_run_id text, generation_no integer, chunk_artifact_set_id text, chunk_type text, content text, content_hash text, enabled boolean, status text, deleted_at datetime)`,
		`CREATE TABLE data_library_document_chunk_embeddings (id text primary key, organization_id text, asset_id text, chunk_id text, processing_run_id text, generation_no integer, embedding_provider text, embedding_model text, embedding_dimension integer, embedding_vector text, content_hash text, status text, deleted_at datetime)`,
		`CREATE TABLE data_library_parse_confirmation_items (id text primary key, asset_id text, deleted_at datetime)`,
		`CREATE TABLE data_library_processing_requests (id text primary key, organization_id text, asset_id text, target_level text, status text, cancelled_at datetime, updated_at datetime, deleted_at datetime)`,
		`CREATE TABLE data_library_knowledge_base_asset_refs (id text primary key, organization_id text, dataset_id text, asset_id text, version_id text, chunk_artifact_set_id text, status text, deleted_at datetime)`,
		`CREATE TABLE data_library_database_asset_refs (id text primary key, organization_id text, data_source_id text, asset_id text, version_id text, parse_artifact_id text, extraction_artifact_id text, status text, deleted_at datetime)`,
		`CREATE TABLE data_library_reuse_events (id text primary key, asset_id text, deleted_at datetime)`,
		`CREATE TABLE data_library_vector_artifacts (id text primary key, asset_id text, version_id text, chunk_artifact_set_id text, deleted_at datetime)`,
		`CREATE TABLE data_library_extraction_artifacts (id text primary key, asset_id text, version_id text, parse_artifact_id text, deleted_at datetime)`,
		`CREATE TABLE content_parse_artifacts (id text primary key, source_content_hash text, profile text, canonical_ir_version text, provider_signature text, deleted_at datetime)`,
		`CREATE TABLE content_parse_chunk_artifact_sets (id text primary key, parse_artifact_id text, source_content_hash text, use_case text, planner_name text, chunker_version text, signature text, content_hash text, deleted_at datetime)`,
		`CREATE TABLE content_parse_runs (id text primary key, artifact_id text, source_type text, intent text, profile text, status text, quality_level text)`,
		`CREATE TABLE content_parse_chunking_runs (id text primary key, parse_run_id text, chunk_artifact_set_id text, use_case text, planner_name text)`,
	}
	for _, statement := range statements {
		execFileAssetDeletionSQL(t, db, statement)
	}
	return db
}

func execFileAssetDeletionSQL(t *testing.T, db *gorm.DB, statement string, args ...any) {
	t.Helper()
	if err := db.Exec(statement, args...).Error; err != nil {
		t.Fatalf("exec %q: %v", statement, err)
	}
}

func assertTableCount(t *testing.T, db *gorm.DB, modelValue any, want int64) {
	t.Helper()
	var got int64
	if err := db.Unscoped().Model(modelValue).Count(&got).Error; err != nil {
		t.Fatalf("count %T: %v", modelValue, err)
	}
	if got != want {
		t.Fatalf("count %T=%d want %d", modelValue, got, want)
	}
}

type fileAssetDeletionVectorIndex struct {
	deleted bool
	assetID uuid.UUID
}

func (v *fileAssetDeletionVectorIndex) EnsureAssetIndexed(ctx context.Context, asset *model.DocumentAsset) error {
	return nil
}

func (v *fileAssetDeletionVectorIndex) RebuildAssetIndex(ctx context.Context, asset *model.DocumentAsset) (int, error) {
	return 0, nil
}

func (v *fileAssetDeletionVectorIndex) IndexChunkEmbeddings(ctx context.Context, asset *model.DocumentAsset, chunks []*model.DocumentChunk, embeddings []*model.DocumentChunkEmbedding, resetAsset bool) error {
	return nil
}

func (v *fileAssetDeletionVectorIndex) DeleteAssetIndex(ctx context.Context, asset *model.DocumentAsset) error {
	v.deleted = true
	if asset != nil {
		v.assetID = asset.ID
	}
	return nil
}

func (v *fileAssetDeletionVectorIndex) DeleteChunkVector(ctx context.Context, asset *model.DocumentAsset, chunkID uuid.UUID) error {
	return nil
}

func (v *fileAssetDeletionVectorIndex) DeleteChildVectorsByParent(ctx context.Context, asset *model.DocumentAsset, parentChunkID uuid.UUID) error {
	return nil
}

func (v *fileAssetDeletionVectorIndex) Search(ctx context.Context, asset *model.DocumentAsset, queryVector []float64, limit int) ([]map[string]interface{}, error) {
	return nil, nil
}
