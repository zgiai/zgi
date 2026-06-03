package repository

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFileRepositoryCheckIfFileIsUsedUsesActiveAssetRefs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	createFileRepositoryUsageTables(t, db)

	repo := NewFileRepository(db)
	execFileRepositorySQL(t, db, `INSERT INTO data_library_document_assets (id, source_file_id, deleted_at) VALUES (?, ?, NULL)`, "asset-1", "file-1")
	execFileRepositorySQL(t, db, `INSERT INTO data_library_knowledge_base_asset_refs (id, dataset_id, asset_id, deleted_at) VALUES (?, ?, ?, NULL)`, "orphan-ref", "dataset-deleted", "asset-1")

	used, err := repo.CheckIfFileIsUsed(context.Background(), "file-1")
	if err != nil {
		t.Fatalf("check if file is used: %v", err)
	}
	if used {
		t.Fatalf("expected orphan ref without dataset to be ignored")
	}

	execFileRepositorySQL(t, db, `INSERT INTO datasets (id) VALUES (?)`, "dataset-active")
	execFileRepositorySQL(t, db, `INSERT INTO data_library_knowledge_base_asset_refs (id, dataset_id, asset_id, deleted_at) VALUES (?, ?, ?, NULL)`, "active-ref", "dataset-active", "asset-1")
	used, err = repo.CheckIfFileIsUsed(context.Background(), "file-1")
	if err != nil {
		t.Fatalf("check if file is used after active ref: %v", err)
	}
	if !used {
		t.Fatalf("expected active ref with existing dataset to block deletion")
	}
}

func createFileRepositoryUsageTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	statements := []string{
		`CREATE TABLE datasets (id text primary key)`,
		`CREATE TABLE data_library_document_assets (id text primary key, source_file_id text, deleted_at datetime)`,
		`CREATE TABLE data_library_knowledge_base_asset_refs (id text primary key, dataset_id text, asset_id text, deleted_at datetime)`,
	}
	for _, statement := range statements {
		execFileRepositorySQL(t, db, statement)
	}
}

func execFileRepositorySQL(t *testing.T, db *gorm.DB, sql string, args ...any) {
	t.Helper()
	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatalf("exec sql failed: %v\nsql: %s", err, sql)
	}
}
