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

func TestDocumentAssetRepositoryCreateAndRead(t *testing.T) {
	db := openDocumentAssetRepoTestDB(t)
	repo := NewDocumentAssetRepository(db)
	ctx := context.Background()
	workspaceID := "workspace-1"

	asset := &model.DocumentAsset{
		OrganizationID:  "org-1",
		WorkspaceID:     &workspaceID,
		Title:           "Handbook.pdf",
		SourceFileID:    "file-1",
		ContentHash:     "hash-1",
		Status:          model.DocumentAssetStatusReady,
		ProcessingLevel: model.DocumentProcessingLevelSplit,
		CreatedBy:       "user-1",
	}
	if err := repo.CreateAsset(ctx, asset); err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}
	if asset.ID == uuid.Nil {
		t.Fatal("expected asset id")
	}

	got, err := repo.GetAssetByID(ctx, asset.ID)
	if err != nil {
		t.Fatalf("GetAssetByID: %v", err)
	}
	if got == nil || got.Title != "Handbook.pdf" || got.Status != model.DocumentAssetStatusReady {
		t.Fatalf("asset=%+v", got)
	}

	byFile, err := repo.FindAssetBySourceFileID(ctx, "org-1", "file-1")
	if err != nil {
		t.Fatalf("FindAssetBySourceFileID: %v", err)
	}
	if byFile == nil || byFile.ID != asset.ID {
		t.Fatalf("byFile=%+v", byFile)
	}
}

func TestDocumentAssetRepositoryCreateAssetWithVersion(t *testing.T) {
	db := openDocumentAssetRepoTestDB(t)
	repo := NewDocumentAssetRepository(db)
	ctx := context.Background()
	assetID := uuid.New()
	versionID := uuid.New()

	asset := &model.DocumentAsset{
		ID:               assetID,
		OrganizationID:   "org-1",
		Title:            "Handbook.pdf",
		SourceFileID:     "file-1",
		CurrentVersionID: &versionID,
	}
	version := &model.DocumentVersion{
		ID:           versionID,
		AssetID:      assetID,
		VersionNo:    1,
		SourceFileID: "file-1",
		FileName:     "Handbook.pdf",
	}
	if err := repo.CreateAssetWithVersion(ctx, asset, version); err != nil {
		t.Fatalf("CreateAssetWithVersion: %v", err)
	}

	gotAsset, err := repo.GetAssetByID(ctx, assetID)
	if err != nil {
		t.Fatalf("GetAssetByID: %v", err)
	}
	if gotAsset == nil || gotAsset.CurrentVersionID == nil || *gotAsset.CurrentVersionID != versionID {
		t.Fatalf("asset=%+v", gotAsset)
	}
	gotVersion, err := repo.GetVersionByID(ctx, versionID)
	if err != nil {
		t.Fatalf("GetVersionByID: %v", err)
	}
	if gotVersion == nil || gotVersion.AssetID != assetID {
		t.Fatalf("version=%+v", gotVersion)
	}
}

func TestDocumentAssetRepositoryListAssets(t *testing.T) {
	db := openDocumentAssetRepoTestDB(t)
	repo := NewDocumentAssetRepository(db)
	ctx := context.Background()
	workspaceA := "workspace-a"
	workspaceB := "workspace-b"

	seedAssets := []*model.DocumentAsset{
		{OrganizationID: "org-1", WorkspaceID: &workspaceA, Title: "A", SourceFileID: "file-a", Status: model.DocumentAssetStatusReady, UpdatedAt: time.Now().Add(2 * time.Minute)},
		{OrganizationID: "org-1", WorkspaceID: &workspaceB, Title: "B", SourceFileID: "file-b", Status: model.DocumentAssetStatusArchived, UpdatedAt: time.Now().Add(time.Minute)},
		{OrganizationID: "org-2", WorkspaceID: &workspaceA, Title: "C", SourceFileID: "file-c", Status: model.DocumentAssetStatusReady, UpdatedAt: time.Now()},
	}
	for _, item := range seedAssets {
		if err := repo.CreateAsset(ctx, item); err != nil {
			t.Fatalf("CreateAsset seed: %v", err)
		}
	}

	items, total, err := repo.ListAssets(ctx, DocumentAssetListFilter{
		OrganizationID: "org-1",
		WorkspaceID:    &workspaceA,
		Status:         model.DocumentAssetStatusReady,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].Title != "A" {
		t.Fatalf("items=%v total=%d", items, total)
	}
}

func TestDocumentAssetRepositoryFindAssetsBySourceFileIDs(t *testing.T) {
	db := openDocumentAssetRepoTestDB(t)
	repo := NewDocumentAssetRepository(db)
	ctx := context.Background()
	newer := time.Now().Add(time.Minute)

	seedAssets := []*model.DocumentAsset{
		{OrganizationID: "org-1", Title: "A older", SourceFileID: "file-a", UpdatedAt: time.Now()},
		{OrganizationID: "org-1", Title: "A newer", SourceFileID: "file-a", UpdatedAt: newer},
		{OrganizationID: "org-1", Title: "B", SourceFileID: "file-b", UpdatedAt: time.Now()},
		{OrganizationID: "org-2", Title: "C", SourceFileID: "file-c", UpdatedAt: time.Now()},
	}
	for _, item := range seedAssets {
		if err := repo.CreateAsset(ctx, item); err != nil {
			t.Fatalf("CreateAsset seed: %v", err)
		}
	}

	items, err := repo.FindAssetsBySourceFileIDs(ctx, "org-1", []string{"file-a", "file-b", "file-c"})
	if err != nil {
		t.Fatalf("FindAssetsBySourceFileIDs: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items=%+v", items)
	}
	if items["file-a"] == nil || items["file-a"].Title != "A newer" {
		t.Fatalf("file-a=%+v", items["file-a"])
	}
	if items["file-b"] == nil || items["file-b"].Title != "B" {
		t.Fatalf("file-b=%+v", items["file-b"])
	}
	if items["file-c"] != nil {
		t.Fatalf("file-c should be scoped out: %+v", items["file-c"])
	}
}

func TestDocumentAssetRepositoryVersions(t *testing.T) {
	db := openDocumentAssetRepoTestDB(t)
	repo := NewDocumentAssetRepository(db)
	ctx := context.Background()

	asset := &model.DocumentAsset{OrganizationID: "org-1", Title: "Asset", SourceFileID: "file-1"}
	if err := repo.CreateAsset(ctx, asset); err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}

	v1 := &model.DocumentVersion{AssetID: asset.ID, VersionNo: 1, SourceFileID: "file-1", FileName: "a-v1.pdf"}
	v2 := &model.DocumentVersion{AssetID: asset.ID, VersionNo: 2, SourceFileID: "file-2", FileName: "a-v2.pdf"}
	if err := repo.CreateVersion(ctx, v1); err != nil {
		t.Fatalf("CreateVersion v1: %v", err)
	}
	if err := repo.CreateVersion(ctx, v2); err != nil {
		t.Fatalf("CreateVersion v2: %v", err)
	}

	got, err := repo.GetVersionByID(ctx, v1.ID)
	if err != nil {
		t.Fatalf("GetVersionByID: %v", err)
	}
	if got == nil || got.FileName != "a-v1.pdf" {
		t.Fatalf("version=%+v", got)
	}

	versions, err := repo.ListVersionsByAssetID(ctx, asset.ID)
	if err != nil {
		t.Fatalf("ListVersionsByAssetID: %v", err)
	}
	if len(versions) != 2 || versions[0].VersionNo != 2 || versions[1].VersionNo != 1 {
		t.Fatalf("versions=%+v", versions)
	}
}

func openDocumentAssetRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	schema := []string{
		`CREATE TABLE data_library_document_assets (
			id text PRIMARY KEY,
			organization_id text NOT NULL,
			workspace_id text,
			title text NOT NULL,
			source_file_id text NOT NULL,
			current_version_id text,
			content_hash text,
			status text NOT NULL DEFAULT 'archived',
			processing_level text NOT NULL DEFAULT 'archive',
			quality_score real,
			metadata_json text NOT NULL DEFAULT '{}',
			permission_policy text NOT NULL DEFAULT '{}',
			created_by text,
			created_at datetime NOT NULL,
			updated_at datetime NOT NULL,
			deleted_at datetime
		)`,
		`CREATE INDEX idx_data_library_assets_org_workspace_status
			ON data_library_document_assets (organization_id, workspace_id, status)`,
		`CREATE INDEX idx_data_library_assets_source_file
			ON data_library_document_assets (source_file_id)`,
		`CREATE TABLE data_library_document_versions (
			id text PRIMARY KEY,
			asset_id text NOT NULL,
			version_no integer NOT NULL,
			source_file_id text NOT NULL,
			content_hash text,
			file_name text,
			file_size integer,
			mime_type text,
			parse_artifact_id text,
			chunk_artifact_set_id text,
			status text NOT NULL DEFAULT 'archived',
			quality_score real,
			uploaded_by text,
			metadata_json text NOT NULL DEFAULT '{}',
			created_at datetime NOT NULL,
			deleted_at datetime
		)`,
		`CREATE INDEX idx_data_library_versions_asset_created
			ON data_library_document_versions (asset_id, created_at)`,
		`CREATE INDEX idx_data_library_versions_source_file
			ON data_library_document_versions (source_file_id)`,
	}
	for _, stmt := range schema {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create test schema: %v", err)
		}
	}
	return db
}
