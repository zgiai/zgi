package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/datalibrary/model"
	dlrepo "github.com/zgiai/ginext/internal/modules/datalibrary/repository"
	filemodel "github.com/zgiai/ginext/internal/modules/file_process/model"
	filerepo "github.com/zgiai/ginext/internal/modules/file_process/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFileAssetSyncServiceCreatesArchivedAssetFromFile(t *testing.T) {
	workspaceID := "workspace-1"
	createdAt := time.Now().Add(-time.Hour)
	file := &filemodel.UploadFile{
		ID:             "file-1",
		OrganizationID: "org-1",
		WorkspaceID:    &workspaceID,
		Name:           "Handbook.pdf",
		Size:           1234,
		MimeType:       "application/pdf",
		Hash:           "hash-1",
		CreatedBy:      "user-1",
		CreatedAt:      createdAt,
	}
	assetRepo := &fakeDocumentAssetRepository{}
	assetSvc := NewDocumentAssetService(assetRepo)
	syncSvc := NewFileAssetSyncService(&fakeFileAssetLookup{file: file}, assetRepo, assetSvc)

	view, created, err := syncSvc.SyncFileAsArchivedAsset(context.Background(), "org-1", "file-1", "sync-user")
	if err != nil {
		t.Fatalf("SyncFileAsArchivedAsset: %v", err)
	}
	if !created {
		t.Fatal("expected created asset")
	}
	if view == nil || view.Title != "Handbook.pdf" || view.Status != model.DocumentAssetStatusArchived {
		t.Fatalf("view=%+v", view)
	}
	if view.CurrentVersion == nil || view.CurrentVersion.FileName != "Handbook.pdf" || view.CurrentVersion.FileSize != 1234 {
		t.Fatalf("version=%+v", view.CurrentVersion)
	}
	if view.CurrentVersionID == nil || *view.CurrentVersionID != view.CurrentVersion.ID {
		t.Fatalf("current_version_id=%+v version=%+v", view.CurrentVersionID, view.CurrentVersion)
	}
	if assetRepo.asset.CreatedBy != "sync-user" || assetRepo.asset.ProcessingLevel != model.DocumentProcessingLevelArchive {
		t.Fatalf("asset=%+v", assetRepo.asset)
	}
	if assetRepo.asset.MetadataJSON["source"] != fileSyncMetadataSource ||
		assetRepo.asset.MetadataJSON["processing_level"] != fileSyncMetadataProcessing ||
		assetRepo.asset.MetadataJSON["parse_triggered"] != false ||
		assetRepo.asset.MetadataJSON["split_triggered"] != false {
		t.Fatalf("asset metadata=%+v", assetRepo.asset.MetadataJSON)
	}
	if assetRepo.version.MetadataJSON["source"] != fileSyncMetadataSource {
		t.Fatalf("version metadata=%+v", assetRepo.version.MetadataJSON)
	}
}

func TestFileAssetSyncServicePersistsUploadFileAsArchivedAsset(t *testing.T) {
	db := openFileAssetSyncTestDB(t)
	ctx := context.Background()
	workspaceID := "workspace-1"
	createdAt := time.Now().Add(-time.Hour).UTC()
	uploadFile := &filemodel.UploadFile{
		ID:             "file-1",
		OrganizationID: "org-1",
		WorkspaceID:    &workspaceID,
		StorageType:    "local",
		Key:            "uploads/file-1.pdf",
		Name:           "Handbook.pdf",
		Size:           2048,
		Extension:      ".pdf",
		MimeType:       "application/pdf",
		CreatedByRole:  filemodel.CreatedByRoleAccount,
		CreatedBy:      "user-1",
		CreatedAt:      createdAt,
		Hash:           "hash-1",
	}
	if err := db.Create(uploadFile).Error; err != nil {
		t.Fatalf("create upload file: %v", err)
	}

	assetRepo := dlrepo.NewDocumentAssetRepository(db)
	assetSvc := NewDocumentAssetService(assetRepo)
	syncSvc := NewFileAssetSyncService(filerepo.NewFileRepository(db), assetRepo, assetSvc)

	view, created, err := syncSvc.SyncFileAsArchivedAsset(ctx, "org-1", "file-1", "sync-user")
	if err != nil {
		t.Fatalf("SyncFileAsArchivedAsset: %v", err)
	}
	if !created {
		t.Fatal("expected created asset")
	}
	if view == nil || view.Title != "Handbook.pdf" || view.Status != model.DocumentAssetStatusArchived {
		t.Fatalf("view=%+v", view)
	}

	asset, err := assetRepo.FindAssetBySourceFileID(ctx, "org-1", "file-1")
	if err != nil {
		t.Fatalf("FindAssetBySourceFileID: %v", err)
	}
	if asset == nil || asset.CurrentVersionID == nil || asset.CreatedBy != "sync-user" {
		t.Fatalf("asset=%+v", asset)
	}
	if asset.MetadataJSON["source"] != fileSyncMetadataSource || asset.MetadataJSON["pipeline_state"] != fileSyncMetadataPipelineState {
		t.Fatalf("asset metadata=%+v", asset.MetadataJSON)
	}
	version, err := assetRepo.GetVersionByID(ctx, *asset.CurrentVersionID)
	if err != nil {
		t.Fatalf("GetVersionByID: %v", err)
	}
	if version == nil || version.FileName != "Handbook.pdf" || version.FileSize != 2048 || version.SourceFileID != "file-1" {
		t.Fatalf("version=%+v", version)
	}
	if version.MetadataJSON["source"] != fileSyncMetadataSource || version.MetadataJSON["parse_triggered"] != false {
		t.Fatalf("version metadata=%+v", version.MetadataJSON)
	}
}

func TestFileAssetSyncServiceReturnsExistingAsset(t *testing.T) {
	assetID := uuid.New()
	versionID := uuid.New()
	assetRepo := &fakeDocumentAssetRepository{
		asset: &model.DocumentAsset{
			ID:               assetID,
			OrganizationID:   "org-1",
			Title:            "Existing.pdf",
			SourceFileID:     "file-1",
			CurrentVersionID: &versionID,
		},
		version: &model.DocumentVersion{
			ID:           versionID,
			AssetID:      assetID,
			VersionNo:    1,
			SourceFileID: "file-1",
			FileName:     "Existing.pdf",
		},
	}
	files := &fakeFileAssetLookup{err: errors.New("file lookup should not run")}
	syncSvc := NewFileAssetSyncService(files, assetRepo, NewDocumentAssetService(assetRepo))

	view, created, err := syncSvc.SyncFileAsArchivedAsset(context.Background(), "org-1", "file-1", "sync-user")
	if err != nil {
		t.Fatalf("SyncFileAsArchivedAsset: %v", err)
	}
	if created {
		t.Fatal("expected existing asset")
	}
	if view == nil || view.ID != assetID {
		t.Fatalf("view=%+v", view)
	}
	if files.calls != 0 {
		t.Fatalf("file lookup calls=%d", files.calls)
	}
}

func TestFileAssetSyncServiceReturnsExistingAssetAfterConcurrentCreate(t *testing.T) {
	createErr := errors.New("duplicate active asset")
	assetID := uuid.New()
	versionID := uuid.New()
	assetRepo := &fakeDocumentAssetRepository{
		createErr:     createErr,
		skipFirstFind: true,
		assetsBySourceFileID: map[string]*model.DocumentAsset{
			"file-1": {
				ID:               assetID,
				OrganizationID:   "org-1",
				Title:            "Existing.pdf",
				SourceFileID:     "file-1",
				CurrentVersionID: &versionID,
				Status:           model.DocumentAssetStatusArchived,
				ProcessingLevel:  model.DocumentProcessingLevelArchive,
			},
		},
		versionsByID: map[uuid.UUID]*model.DocumentVersion{
			versionID: {
				ID:           versionID,
				AssetID:      assetID,
				VersionNo:    1,
				SourceFileID: "file-1",
				FileName:     "Existing.pdf",
			},
		},
	}
	files := &fakeFileAssetLookup{file: &filemodel.UploadFile{
		ID:             "file-1",
		OrganizationID: "org-1",
		Name:           "Handbook.pdf",
		CreatedAt:      time.Now(),
	}}
	syncSvc := NewFileAssetSyncService(files, assetRepo, NewDocumentAssetService(assetRepo))

	view, created, err := syncSvc.SyncFileAsArchivedAsset(context.Background(), "org-1", "file-1", "sync-user")
	if err != nil {
		t.Fatalf("SyncFileAsArchivedAsset: %v", err)
	}
	if created {
		t.Fatal("expected existing asset after concurrent create")
	}
	if view == nil || view.ID != assetID {
		t.Fatalf("view=%+v", view)
	}
}

func TestFileAssetSyncServiceSyncsFilesAsArchivedAssetsWithBatchLookups(t *testing.T) {
	workspaceID := "workspace-1"
	createdAt := time.Now().Add(-time.Hour)
	assetRepo := &fakeDocumentAssetRepository{
		assetsBySourceFileID: map[string]*model.DocumentAsset{},
		versionsByID:         map[uuid.UUID]*model.DocumentVersion{},
	}
	files := &fakeFileAssetLookup{
		filesByID: map[string]*filemodel.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: "org-1",
				WorkspaceID:    &workspaceID,
				Name:           "A.pdf",
				Size:           100,
				MimeType:       "application/pdf",
				Hash:           "hash-a",
				CreatedBy:      "user-1",
				CreatedAt:      createdAt,
			},
			"file-2": {
				ID:             "file-2",
				OrganizationID: "org-1",
				WorkspaceID:    &workspaceID,
				Name:           "B.pdf",
				Size:           200,
				MimeType:       "application/pdf",
				Hash:           "hash-b",
				CreatedBy:      "user-1",
				CreatedAt:      createdAt,
			},
		},
	}
	syncSvc := NewFileAssetSyncService(files, assetRepo, NewDocumentAssetService(assetRepo))

	result, err := syncSvc.SyncFilesAsArchivedAssets(context.Background(), "org-1", []string{"file-1", "file-1", "missing", "file-2", ""}, "sync-user")
	if err != nil {
		t.Fatalf("SyncFilesAsArchivedAssets: %v", err)
	}
	if result.Total != 5 || result.CreatedCount != 2 || result.ReusedCount != 1 || result.FailedCount != 2 {
		t.Fatalf("result=%+v", result)
	}
	if len(result.Items) != 5 {
		t.Fatalf("items=%+v", result.Items)
	}
	if result.Items[0].Asset == nil || result.Items[0].Asset.SourceFileID != "file-1" || !result.Items[0].Created {
		t.Fatalf("first item=%+v", result.Items[0])
	}
	if result.Items[1].Asset == nil || result.Items[1].Asset.SourceFileID != "file-1" || result.Items[1].Created {
		t.Fatalf("duplicate item=%+v", result.Items[1])
	}
	if result.Items[2].FileID != "missing" || result.Items[2].Error == "" {
		t.Fatalf("missing item=%+v", result.Items[2])
	}
	if result.Items[3].Asset == nil || result.Items[3].Asset.SourceFileID != "file-2" || !result.Items[3].Created {
		t.Fatalf("fourth item=%+v", result.Items[3])
	}
	if result.Items[4].Error == "" {
		t.Fatalf("empty file item=%+v", result.Items[4])
	}
	if files.batchCalls != 1 || files.calls != 0 {
		t.Fatalf("file lookup calls=%d batchCalls=%d", files.calls, files.batchCalls)
	}
}

func TestFileAssetSyncServiceValidatesRequiredFields(t *testing.T) {
	syncSvc := NewFileAssetSyncService(&fakeFileAssetLookup{}, &fakeDocumentAssetRepository{}, NewDocumentAssetService(&fakeDocumentAssetRepository{}))
	if _, _, err := syncSvc.SyncFileAsArchivedAsset(context.Background(), "", "file-1", "user-1"); !errors.Is(err, ErrOrganizationIDRequired) {
		t.Fatalf("organization error=%v", err)
	}
	if _, _, err := syncSvc.SyncFileAsArchivedAsset(context.Background(), "org-1", "", "user-1"); !errors.Is(err, ErrFileIDRequired) {
		t.Fatalf("file error=%v", err)
	}
	if _, err := syncSvc.SyncFilesAsArchivedAssets(context.Background(), "", []string{"file-1"}, "user-1"); !errors.Is(err, ErrOrganizationIDRequired) {
		t.Fatalf("batch organization error=%v", err)
	}
	if _, err := syncSvc.SyncFilesAsArchivedAssets(context.Background(), "org-1", nil, "user-1"); !errors.Is(err, ErrFileIDsRequired) {
		t.Fatalf("batch file ids error=%v", err)
	}
}

type fakeFileAssetLookup struct {
	file       *filemodel.UploadFile
	filesByID  map[string]*filemodel.UploadFile
	err        error
	calls      int
	batchCalls int
}

func (f *fakeFileAssetLookup) GetByTenantAndID(ctx context.Context, organizationID, id string) (*filemodel.UploadFile, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if f.filesByID != nil {
		return f.filesByID[id], nil
	}
	return f.file, nil
}

func (f *fakeFileAssetLookup) ListByTenantAndIDs(ctx context.Context, organizationID string, ids []string) (map[string]*filemodel.UploadFile, error) {
	f.batchCalls++
	if f.err != nil {
		return nil, f.err
	}
	files := make(map[string]*filemodel.UploadFile, len(ids))
	for _, id := range ids {
		if f.filesByID != nil {
			if file := f.filesByID[id]; file != nil {
				files[id] = file
			}
			continue
		}
		if f.file != nil && f.file.ID == id {
			files[id] = f.file
		}
	}
	return files, nil
}

var _ FileAssetLookup = (*fakeFileAssetLookup)(nil)
var _ BatchFileAssetLookup = (*fakeFileAssetLookup)(nil)
var _ dlrepo.DocumentAssetRepository = (*fakeDocumentAssetRepository)(nil)

func openFileAssetSyncTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&filemodel.UploadFile{}); err != nil {
		t.Fatalf("migrate upload files: %v", err)
	}
	for _, stmt := range dataLibraryAssetSyncSchema() {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create data library schema: %v", err)
		}
	}
	return db
}

func dataLibraryAssetSyncSchema() []string {
	return []string{
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
	}
}
