package repository

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDocumentAssetRepositoryListAssetsFiltersActiveSourceFiles(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	execDocumentAssetRepoSQL(t, db, `
		CREATE TABLE data_library_document_assets (
			id text primary key,
			organization_id text not null,
			workspace_id text,
			title text not null,
			source_file_id text not null,
			current_version_id text,
			content_hash text,
			status text,
			processing_level text,
			product_status text,
			processing_stage text,
			processing_progress integer,
			active_processing_request_id text,
			processing_run_id text,
			generation_no integer,
			parse_artifact_id text,
			chunk_artifact_set_id text,
			chunk_count integer,
			embedding_provider text,
			embedding_model text,
			embedding_dimension integer,
			vector_status text,
			last_error_code text,
			last_error_message text,
			quality_score real,
			metadata_json text,
			permission_policy text,
			created_by text,
			created_at datetime default CURRENT_TIMESTAMP,
			updated_at datetime default CURRENT_TIMESTAMP,
			deleted_at datetime
		)
	`)
	execDocumentAssetRepoSQL(t, db, `
		CREATE TABLE upload_files (
			id text primary key,
			organization_id text not null,
			workspace_id text,
			name text,
			is_archived bool default false
		)
	`)
	execDocumentAssetRepoSQL(t, db, `INSERT INTO upload_files (id, organization_id, name, is_archived) VALUES (?, ?, ?, ?)`, "file-active", "org-1", "active.pdf", false)
	execDocumentAssetRepoSQL(t, db, `INSERT INTO upload_files (id, organization_id, name, is_archived) VALUES (?, ?, ?, ?)`, "file-archived", "org-1", "archived.pdf", true)
	execDocumentAssetRepoSQL(t, db, `INSERT INTO data_library_document_assets (id, organization_id, source_file_id, title, product_status) VALUES (?, ?, ?, ?, ?)`, "11111111-1111-1111-1111-111111111111", "org-1", "file-active", "active", "ready")
	execDocumentAssetRepoSQL(t, db, `INSERT INTO data_library_document_assets (id, organization_id, source_file_id, title, product_status) VALUES (?, ?, ?, ?, ?)`, "22222222-2222-2222-2222-222222222222", "org-1", "file-archived", "archived", "ready")
	execDocumentAssetRepoSQL(t, db, `INSERT INTO data_library_document_assets (id, organization_id, source_file_id, title, product_status) VALUES (?, ?, ?, ?, ?)`, "33333333-3333-3333-3333-333333333333", "org-1", "file-missing", "missing", "ready")

	repo := NewDocumentAssetRepository(db)
	ctx := context.Background()

	items, total, err := repo.ListAssets(ctx, DocumentAssetListFilter{
		OrganizationID:       "org-1",
		ProductStatus:        "ready",
		ActiveSourceFileOnly: true,
		Limit:                20,
	})
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].SourceFileID != "file-active" {
		t.Fatalf("total=%d items=%+v", total, items)
	}
}

func execDocumentAssetRepoSQL(t *testing.T, db *gorm.DB, sql string, args ...any) {
	t.Helper()
	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatalf("exec sql: %v\n%s", err, sql)
	}
}
