package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	filemodel "github.com/zgiai/zgi/api/internal/modules/file_process/model"
)

var (
	ErrFileIDRequired    = errors.New("file_id is required")
	ErrFileIDsRequired   = errors.New("file_ids is required")
	ErrSourceFileMissing = errors.New("source file is missing")
)

const (
	fileSyncMetadataSource        = "file_sync"
	fileSyncMetadataProcessing    = "archive"
	fileSyncMetadataPipelineState = "not_started"
)

type FileAssetLookup interface {
	GetByTenantAndID(ctx context.Context, organizationID, id string) (*filemodel.UploadFile, error)
}

type BatchFileAssetLookup interface {
	ListByTenantAndIDs(ctx context.Context, organizationID string, ids []string) (map[string]*filemodel.UploadFile, error)
}

type FileAssetSyncService interface {
	SyncFileAsArchivedAsset(ctx context.Context, organizationID string, fileID string, createdBy string) (*DocumentAssetView, bool, error)
	SyncFilesAsArchivedAssets(ctx context.Context, organizationID string, fileIDs []string, createdBy string) (*BatchFileAssetSyncResult, error)
}

type BatchFileAssetSyncResult struct {
	Items        []FileAssetSyncResult `json:"items"`
	Total        int                   `json:"total"`
	CreatedCount int                   `json:"created_count"`
	ReusedCount  int                   `json:"reused_count"`
	FailedCount  int                   `json:"failed_count"`
}

type FileAssetSyncResult struct {
	FileID  string             `json:"file_id"`
	Asset   *DocumentAssetView `json:"asset,omitempty"`
	Created bool               `json:"created"`
	Error   string             `json:"error,omitempty"`
}

type fileAssetSyncService struct {
	files        FileAssetLookup
	assets       repository.DocumentAssetRepository
	assetService DocumentAssetService
}

func NewFileAssetSyncService(files FileAssetLookup, assets repository.DocumentAssetRepository, assetService DocumentAssetService) FileAssetSyncService {
	return &fileAssetSyncService{
		files:        files,
		assets:       assets,
		assetService: assetService,
	}
}

func (s *fileAssetSyncService) SyncFilesAsArchivedAssets(ctx context.Context, organizationID string, fileIDs []string, createdBy string) (*BatchFileAssetSyncResult, error) {
	if organizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if len(fileIDs) == 0 {
		return nil, ErrFileIDsRequired
	}
	uniqueFileIDs := uniqueNonEmptyFileIDs(fileIDs)
	if len(uniqueFileIDs) == 0 {
		return nil, ErrFileIDsRequired
	}

	result := &BatchFileAssetSyncResult{
		Items: make([]FileAssetSyncResult, 0, len(fileIDs)),
		Total: len(fileIDs),
	}
	existingAssets, err := s.assets.FindAssetsBySourceFileIDs(ctx, organizationID, uniqueFileIDs)
	if err != nil {
		return nil, err
	}
	sourceFiles, err := s.lookupFiles(ctx, organizationID, uniqueFileIDs)
	if err != nil {
		return nil, err
	}
	syncedViews := make(map[string]*DocumentAssetView, len(uniqueFileIDs))

	for _, fileID := range fileIDs {
		item := FileAssetSyncResult{FileID: fileID}
		if fileID == "" {
			item.Error = ErrFileIDRequired.Error()
			result.FailedCount++
			result.Items = append(result.Items, item)
			continue
		}

		if cached := syncedViews[fileID]; cached != nil {
			item.Asset = cached
			item.Created = false
			result.ReusedCount++
			result.Items = append(result.Items, item)
			continue
		}

		if existing := existingAssets[fileID]; existing != nil {
			view, err := s.assetService.GetAssetViewByID(ctx, existing.ID)
			if err != nil {
				item.Error = err.Error()
				result.FailedCount++
				result.Items = append(result.Items, item)
				continue
			}
			syncedViews[fileID] = view
			item.Asset = view
			item.Created = false
			result.ReusedCount++
			result.Items = append(result.Items, item)
			continue
		}

		file := sourceFiles[fileID]
		if file == nil {
			item.Error = ErrSourceFileMissing.Error()
			result.FailedCount++
			result.Items = append(result.Items, item)
			continue
		}

		view, created, err := s.createArchivedAssetFromFile(ctx, file, createdBy)
		if err != nil {
			item.Error = err.Error()
			result.FailedCount++
			result.Items = append(result.Items, item)
			continue
		}
		syncedViews[fileID] = view
		item.Asset = view
		item.Created = created
		if created {
			result.CreatedCount++
		} else {
			result.ReusedCount++
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func (s *fileAssetSyncService) SyncFileAsArchivedAsset(ctx context.Context, organizationID string, fileID string, createdBy string) (*DocumentAssetView, bool, error) {
	if organizationID == "" {
		return nil, false, ErrOrganizationIDRequired
	}
	if fileID == "" {
		return nil, false, ErrFileIDRequired
	}

	existing, err := s.assets.FindAssetBySourceFileID(ctx, organizationID, fileID)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		view, err := s.assetService.GetAssetViewByID(ctx, existing.ID)
		return view, false, err
	}

	file, err := s.files.GetByTenantAndID(ctx, organizationID, fileID)
	if err != nil {
		return nil, false, err
	}
	if file == nil {
		return nil, false, ErrSourceFileMissing
	}

	return s.createArchivedAssetFromFile(ctx, file, createdBy)
}

func (s *fileAssetSyncService) lookupFiles(ctx context.Context, organizationID string, fileIDs []string) (map[string]*filemodel.UploadFile, error) {
	if batchLookup, ok := s.files.(BatchFileAssetLookup); ok {
		return batchLookup.ListByTenantAndIDs(ctx, organizationID, fileIDs)
	}

	files := make(map[string]*filemodel.UploadFile, len(fileIDs))
	for _, fileID := range fileIDs {
		file, err := s.files.GetByTenantAndID(ctx, organizationID, fileID)
		if err != nil {
			return nil, err
		}
		if file != nil {
			files[fileID] = file
		}
	}
	return files, nil
}

func (s *fileAssetSyncService) createArchivedAssetFromFile(ctx context.Context, file *filemodel.UploadFile, createdBy string) (*DocumentAssetView, bool, error) {
	assetID := uuid.New()
	versionID := uuid.New()
	asset := &model.DocumentAsset{
		ID:               assetID,
		OrganizationID:   file.OrganizationID,
		WorkspaceID:      file.WorkspaceID,
		Title:            file.Name,
		SourceFileID:     file.ID,
		CurrentVersionID: &versionID,
		ContentHash:      file.Hash,
		Status:           model.DocumentAssetStatusArchived,
		ProcessingLevel:  model.DocumentProcessingLevelArchive,
		MetadataJSON:     fileAssetSyncMetadata(file),
		CreatedBy:        createdBy,
		CreatedAt:        file.CreatedAt,
		UpdatedAt:        file.CreatedAt,
	}
	version := &model.DocumentVersion{
		ID:           versionID,
		AssetID:      assetID,
		VersionNo:    1,
		SourceFileID: file.ID,
		ContentHash:  file.Hash,
		FileName:     file.Name,
		FileSize:     file.Size,
		MimeType:     file.MimeType,
		Status:       model.DocumentVersionStatusArchived,
		MetadataJSON: fileAssetSyncMetadata(file),
		UploadedBy:   file.CreatedBy,
		CreatedAt:    file.CreatedAt,
	}

	if err := s.assets.CreateAssetWithVersion(ctx, asset, version); err != nil {
		if existing, lookupErr := s.assets.FindAssetBySourceFileID(ctx, file.OrganizationID, file.ID); lookupErr == nil && existing != nil {
			view, viewErr := s.assetService.GetAssetViewByID(ctx, existing.ID)
			return view, false, viewErr
		}
		return nil, false, err
	}

	view, err := s.assetService.GetAssetViewByID(ctx, assetID)
	return view, true, err
}

func uniqueNonEmptyFileIDs(fileIDs []string) []string {
	seen := make(map[string]struct{}, len(fileIDs))
	unique := make([]string, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		if fileID == "" {
			continue
		}
		if _, ok := seen[fileID]; ok {
			continue
		}
		seen[fileID] = struct{}{}
		unique = append(unique, fileID)
	}
	return unique
}

func fileAssetSyncMetadata(file *filemodel.UploadFile) map[string]any {
	metadata := map[string]any{
		"source":           fileSyncMetadataSource,
		"processing_level": fileSyncMetadataProcessing,
		"pipeline_state":   fileSyncMetadataPipelineState,
		"parse_triggered":  false,
		"split_triggered":  false,
		"source_file_id":   file.ID,
		"source_file_key":  file.Key,
		"source_file_hash": file.Hash,
	}
	if file.Extension != "" {
		metadata["source_file_extension"] = file.Extension
	}
	if file.StorageType != "" {
		metadata["source_file_storage_type"] = file.StorageType
	}
	return metadata
}
