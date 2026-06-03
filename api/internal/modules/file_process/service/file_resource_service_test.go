package service

import (
	"context"
	"testing"

	datasetrepo "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFileResourceServiceRelatedDatasetCountsUseActiveAssetRefs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	createFileResourceRelationTables(t, db)

	fileID := "file-1"
	execFileResourceSQL(t, db, `INSERT INTO datasets (id, name) VALUES (?, ?)`, "dataset-active", "Active")
	execFileResourceSQL(t, db, `INSERT INTO data_library_document_assets (id, source_file_id, deleted_at) VALUES (?, ?, NULL)`, "asset-active", fileID)
	execFileResourceSQL(t, db, `INSERT INTO data_library_document_assets (id, source_file_id, deleted_at) VALUES (?, ?, CURRENT_TIMESTAMP)`, "asset-deleted", fileID)
	execFileResourceSQL(t, db, `INSERT INTO data_library_knowledge_base_asset_refs (id, dataset_id, asset_id, dataset_document_id, deleted_at) VALUES (?, ?, ?, ?, NULL)`, "ref-active", "dataset-active", "asset-active", "doc-active")
	execFileResourceSQL(t, db, `INSERT INTO data_library_knowledge_base_asset_refs (id, dataset_id, asset_id, dataset_document_id, deleted_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`, "ref-deleted", "dataset-active", "asset-active", "doc-deleted")
	execFileResourceSQL(t, db, `INSERT INTO data_library_knowledge_base_asset_refs (id, dataset_id, asset_id, dataset_document_id, deleted_at) VALUES (?, ?, ?, ?, NULL)`, "ref-missing-dataset", "dataset-deleted", "asset-active", "doc-missing-dataset")
	execFileResourceSQL(t, db, `INSERT INTO data_library_knowledge_base_asset_refs (id, dataset_id, asset_id, dataset_document_id, deleted_at) VALUES (?, ?, ?, ?, NULL)`, "ref-deleted-asset", "dataset-active", "asset-deleted", "doc-deleted-asset")

	svc := &fileResourceService{documentRepo: datasetrepo.NewDocumentRepository(db)}

	count, err := svc.GetRelatedDatasetCount(context.Background(), fileID)
	if err != nil {
		t.Fatalf("get related dataset count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected related dataset count 1, got %d", count)
	}

	counts, err := svc.BatchGetRelatedDatasetCount(context.Background(), []string{fileID, "file-empty"})
	if err != nil {
		t.Fatalf("batch get related dataset count: %v", err)
	}
	if counts[fileID] != 1 {
		t.Fatalf("expected batch count for %s to be 1, got %d", fileID, counts[fileID])
	}
	if counts["file-empty"] != 0 {
		t.Fatalf("expected empty file count 0, got %d", counts["file-empty"])
	}

	execFileResourceSQL(t, db, `DELETE FROM datasets WHERE id = ?`, "dataset-active")
	count, err = svc.GetRelatedDatasetCount(context.Background(), fileID)
	if err != nil {
		t.Fatalf("get related dataset count after dataset delete: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected related dataset count 0 after dataset delete, got %d", count)
	}
}

func createFileResourceRelationTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	statements := []string{
		`CREATE TABLE datasets (id text primary key, name text)`,
		`CREATE TABLE data_library_document_assets (id text primary key, source_file_id text, deleted_at datetime)`,
		`CREATE TABLE data_library_knowledge_base_asset_refs (id text primary key, dataset_id text, asset_id text, dataset_document_id text, deleted_at datetime)`,
	}
	for _, statement := range statements {
		execFileResourceSQL(t, db, statement)
	}
}

func execFileResourceSQL(t *testing.T, db *gorm.DB, sql string, args ...any) {
	t.Helper()
	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatalf("exec sql failed: %v\nsql: %s", err, sql)
	}
}
