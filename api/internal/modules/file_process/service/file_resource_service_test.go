package service

import (
	"context"
	"errors"
	"testing"

	datasetrepo "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	file_model "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	file_repo "github.com/zgiai/zgi/api/internal/modules/file_process/repository"
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

func TestFileResourceServiceRejectsDuplicateSiblingFolderNames(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	createFileResourceFolderTables(t, db)

	execFileResourceSQL(t, db, `INSERT INTO file_folders (id, organization_id, workspace_id, name, parent_id, created_by, created_at, updated_at, position, permission) VALUES (?, ?, ?, ?, NULL, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?)`, "existing-root", "org-1", "workspace-1", "Reports", "account-1", 0, "only_me")
	execFileResourceSQL(t, db, `INSERT INTO file_folders (id, organization_id, workspace_id, name, parent_id, created_by, created_at, updated_at, position, permission) VALUES (?, ?, ?, ?, NULL, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?)`, "rename-target", "org-1", "workspace-1", "Draft", "account-1", 0, "only_me")
	execFileResourceSQL(t, db, `INSERT INTO file_folders (id, organization_id, workspace_id, name, parent_id, created_by, created_at, updated_at, position, permission) VALUES (?, ?, ?, ?, NULL, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?)`, "target-parent", "org-1", "workspace-1", "Target", "account-1", 0, "only_me")
	execFileResourceSQL(t, db, `INSERT INTO file_folders (id, organization_id, workspace_id, name, parent_id, created_by, created_at, updated_at, position, permission) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?)`, "existing-child", "org-1", "workspace-1", "Reports", "target-parent", "account-1", 0, "only_me")
	execFileResourceSQL(t, db, `INSERT INTO file_folders (id, organization_id, workspace_id, name, parent_id, created_by, created_at, updated_at, position, permission) VALUES (?, ?, ?, ?, NULL, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?)`, "moving-folder", "org-1", "workspace-1", "Reports", "account-1", 0, "only_me")

	workspaceID := "workspace-1"
	svc := &fileResourceService{fileFolderRepo: file_repo.NewFileFolderRepository(db)}

	_, err = svc.CreateFolder(context.Background(), &file_model.FileFolder{
		ID:             "new-duplicate",
		OrganizationID: "org-1",
		WorkspaceID:    &workspaceID,
		Name:           "reports",
		CreatedBy:      "account-1",
		Permission:     "only_me",
	})
	if !errors.Is(err, ErrFolderNameConflict) {
		t.Fatalf("CreateFolder error = %v, want ErrFolderNameConflict", err)
	}

	_, err = svc.UpdateFolder(context.Background(), "rename-target", map[string]interface{}{"name": "Reports"})
	if !errors.Is(err, ErrFolderNameConflict) {
		t.Fatalf("UpdateFolder error = %v, want ErrFolderNameConflict", err)
	}

	err = svc.MoveFolderToFolder(context.Background(), "moving-folder", "target-parent", "account-1", "org-1")
	if !errors.Is(err, ErrFolderNameConflict) {
		t.Fatalf("MoveFolderToFolder error = %v, want ErrFolderNameConflict", err)
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

func createFileResourceFolderTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	execFileResourceSQL(
		t,
		db,
		`CREATE TABLE file_folders (
			id text primary key,
			organization_id text,
			workspace_id text,
			name text,
			description text,
			parent_id text,
			created_by text,
			created_at datetime,
			updated_by text,
			updated_at datetime,
			icon_type text,
			icon text,
			icon_background text,
			position integer,
			permission text
		)`,
	)
}

func execFileResourceSQL(t *testing.T, db *gorm.DB, sql string, args ...any) {
	t.Helper()
	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatalf("exec sql failed: %v\nsql: %s", err, sql)
	}
}
