package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	datalibModel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnableDocumentsSkipsUnsyncedDatasetFileRefs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	createDocumentRepositoryTestTables(t, db)
	repo := NewDocumentRepository(db)
	ctx := context.Background()

	syncedDocumentID := uuid.New()
	failedDocumentID := uuid.New()
	syncedSegmentID := uuid.New()
	failedSegmentID := uuid.New()
	assetID := uuid.New()
	insertDocumentRepositoryTestDocument(t, db, syncedDocumentID.String())
	insertDocumentRepositoryTestDocument(t, db, failedDocumentID.String())
	insertDocumentRepositoryTestSegment(t, db, syncedSegmentID.String(), syncedDocumentID.String())
	insertDocumentRepositoryTestSegment(t, db, failedSegmentID.String(), failedDocumentID.String())
	insertDocumentRepositoryTestRef(t, db, uuid.New(), assetID, syncedDocumentID, datalibModel.KnowledgeBaseAssetRefSyncStatusSynced)
	insertDocumentRepositoryTestRef(t, db, uuid.New(), uuid.New(), failedDocumentID, datalibModel.KnowledgeBaseAssetRefSyncStatusFailed)

	if err := repo.EnableDocuments(ctx, "dataset-1", []string{syncedDocumentID.String(), failedDocumentID.String()}); err != nil {
		t.Fatalf("EnableDocuments: %v", err)
	}

	assertDocumentEnabled(t, db, syncedDocumentID.String(), true)
	assertDocumentEnabled(t, db, failedDocumentID.String(), false)
	assertSegmentEnabled(t, db, syncedSegmentID.String(), true)
	assertSegmentEnabled(t, db, failedSegmentID.String(), false)
}

func assertDocumentEnabled(t *testing.T, db *gorm.DB, id string, want bool) {
	t.Helper()
	var document model.Document
	if err := db.Where("id = ?", id).First(&document).Error; err != nil {
		t.Fatalf("load document %s: %v", id, err)
	}
	if document.Enabled != want {
		t.Fatalf("document %s enabled=%v want %v", id, document.Enabled, want)
	}
}

func insertDocumentRepositoryTestDocument(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO documents (id, organization_id, dataset_id, position, data_source_type, batch, name, created_from, created_by, indexing_status, enabled, doc_form)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "org-1", "dataset-1", 1, "file_asset", "batch-1", "doc.md", "data_library", "user-1", model.DocumentStatusCompleted, false, "text_model",
	).Error
	if err != nil {
		t.Fatalf("insert document: %v", err)
	}
}

func insertDocumentRepositoryTestSegment(t *testing.T, db *gorm.DB, id string, documentID string) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO document_segments (id, organization_id, dataset_id, document_id, position, content, word_count, tokens, enabled, status, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "org-1", "dataset-1", documentID, 1, "hello", 5, 5, false, model.SegmentStatusCompleted, "user-1",
	).Error
	if err != nil {
		t.Fatalf("insert segment: %v", err)
	}
}

func insertDocumentRepositoryTestRef(t *testing.T, db *gorm.DB, id uuid.UUID, assetID uuid.UUID, documentID uuid.UUID, syncStatus string) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO data_library_knowledge_base_asset_refs (id, organization_id, dataset_id, asset_id, dataset_document_id, sync_status)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id.String(), "org-1", "dataset-1", assetID.String(), documentID.String(), syncStatus,
	).Error
	if err != nil {
		t.Fatalf("insert ref: %v", err)
	}
}

func assertSegmentEnabled(t *testing.T, db *gorm.DB, id string, want bool) {
	t.Helper()
	var segment model.DocumentSegment
	if err := db.Where("id = ?", id).First(&segment).Error; err != nil {
		t.Fatalf("load segment %s: %v", id, err)
	}
	if segment.Enabled != want {
		t.Fatalf("segment %s enabled=%v want %v", id, segment.Enabled, want)
	}
}

func createDocumentRepositoryTestTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	statements := []string{
		`CREATE TABLE documents (
			id text PRIMARY KEY,
			organization_id text,
			dataset_id text,
			position integer,
			data_source_type text,
			batch text,
			name text,
			created_from text,
			created_by text,
			indexing_status text,
			enabled boolean,
			disabled_at datetime,
			disabled_by text,
			updated_at datetime,
			doc_form text
		)`,
		`CREATE TABLE document_segments (
			id text PRIMARY KEY,
			organization_id text,
			dataset_id text,
			document_id text,
			position integer,
			content text,
			word_count integer,
			tokens integer,
			enabled boolean,
			disabled_at datetime,
			disabled_by text,
			status text,
			created_by text,
			updated_at datetime,
			deleted_at datetime
		)`,
		`CREATE TABLE data_library_knowledge_base_asset_refs (
			id text PRIMARY KEY,
			organization_id text,
			dataset_id text,
			asset_id text,
			dataset_document_id text,
			sync_status text,
			metadata_json text,
			created_at datetime,
			updated_at datetime,
			deleted_at datetime
		)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
}
