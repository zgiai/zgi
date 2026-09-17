package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ledongthuc/pdf"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/dto"
	datalibraryservice "github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/internal/modules/file_process/repository"
	"github.com/zgiai/zgi/api/internal/modules/file_process/service/extractor"
	llm_client "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	quota_model "github.com/zgiai/zgi/api/internal/modules/quota/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/image"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/storage"
)

// fileService file service implementation
type fileService struct {
	fileRepo          repository.FileRepository
	storage           storage.Storage
	extractor         *extractor.ExtractProcessor
	db                *gorm.DB
	quotaService      interfaces.QuotaService
	enterpriseService interfaces.OrganizationService
	extractGroup      singleflight.Group
	assetDeletion     datalibraryservice.FileAssetDeletionService
}

type UploadFileOptions struct {
	StartLegacyContentExtraction bool
}

func isAssetProcessableExtension(ext string) bool {
	normalized := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	return model.IsDocumentExtension(normalized) || model.IsImageExtension(normalized)
}

// NewFileService creates file service instance
func NewFileService(
	fileRepo repository.FileRepository,
	storage storage.Storage,
	db *gorm.DB,
	quotaService interfaces.QuotaService,
	enterpriseService interfaces.OrganizationService,
) interfaces.FileService {
	return &fileService{
		fileRepo:          fileRepo,
		storage:           storage,
		extractor:         extractor.NewExtractProcessorWithQuota(storage, quotaService, db),
		db:                db,
		quotaService:      quotaService,
		enterpriseService: enterpriseService,
	}
}

func NewFileServiceWithVision(
	fileRepo repository.FileRepository,
	storage storage.Storage,
	db *gorm.DB,
	quotaService interfaces.QuotaService,
	enterpriseService interfaces.OrganizationService,
	llmClient llm_client.LLMClient,
	defaultModelService llmdefaultservice.DefaultModelService,
) interfaces.FileService {
	return &fileService{
		fileRepo:          fileRepo,
		storage:           storage,
		extractor:         extractor.NewExtractProcessorWithQuotaAndVision(storage, quotaService, db, llmClient, defaultModelService),
		db:                db,
		quotaService:      quotaService,
		enterpriseService: enterpriseService,
	}
}

func (s *fileService) SetFileAssetDeletionService(assetDeletion datalibraryservice.FileAssetDeletionService) {
	s.assetDeletion = assetDeletion
}

// GetUploadConfig gets file upload configuration
func (s *fileService) GetUploadConfig() *interfaces.FileUploadConfigResponse {
	cfg := config.Current()

	// Use centralized configuration values
	return &interfaces.FileUploadConfigResponse{
		FileSizeLimit:           int64(cfg.Upload.FileSizeLimit),  // Already in MB from config
		BatchCountLimit:         cfg.Upload.FileBatchLimit,        // File-management concurrency; count limit elsewhere
		UploadQueueLimit:        cfg.Upload.UploadQueueLimit,      // File-management upload queue limit
		ImageFileSizeLimit:      int64(cfg.Upload.ImageSizeLimit), // Image file size limit
		VideoFileSizeLimit:      int64(cfg.Upload.VideoSizeLimit), // Video file size limit
		AudioFileSizeLimit:      int64(cfg.Upload.AudioSizeLimit), // Audio file size limit
		WorkflowFileUploadLimit: cfg.Upload.WorkflowFileLimit,     // Workflow file upload limit
	}
}

// UploadFile upload file
func (s *fileService) UploadFile(ctx context.Context, filename string, content []byte, mimeType string, userID, organizationID string, userRole model.CreatedByRole, source *interfaces.FileSource, workspaceID *string, isTemporary bool, isIcon bool) (*dto.UploadFile, error) {
	return s.UploadFileWithOptions(ctx, filename, content, mimeType, userID, organizationID, userRole, source, workspaceID, isTemporary, isIcon, UploadFileOptions{
		// Temporary chat files are parsed on demand through the routed content
		// parse service. Avoid racing that request with legacy background parsing.
		StartLegacyContentExtraction: !isTemporary,
	})
}

func (s *fileService) ReplaceFileContent(ctx context.Context, fileID string, filename string, content []byte, mimeType string, userID, organizationID string) (*dto.UploadFile, error) {
	current, err := s.fileRepo.GetByTenantAndID(ctx, organizationID, fileID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, gorm.ErrRecordNotFound
	}
	if current.IsTemporary {
		return nil, model.ErrUnsupportedFileType
	}

	extension := strings.ToLower(filepath.Ext(filename))
	if extension != "" {
		extension = extension[1:]
	}
	if len(filename) > 200 {
		nameWithoutExt := strings.TrimSuffix(filename, filepath.Ext(filename))
		if len(nameWithoutExt) > 200 {
			nameWithoutExt = nameWithoutExt[:200]
		}
		filename = nameWithoutExt + "." + extension
	}
	if !isAssetProcessableExtension(extension) {
		return nil, model.ErrUnsupportedFileType
	}

	fileSize := int64(len(content))
	if !s.IsFileSizeWithinLimit(extension, fileSize) {
		return nil, model.ErrFileTooLarge
	}

	delta := fileSize - current.Size
	var groupID *uuid.UUID
	parsedGroupID, parseErr := uuid.Parse(organizationID)
	if parseErr == nil {
		groupID = &parsedGroupID
	}
	if delta > 0 && groupID != nil && s.quotaService != nil {
		canProceed, currentUsage, limit, err := s.quotaService.CheckQuota(ctx, *groupID, quota_model.ResourceTypeStorage, delta)
		if err != nil {
			return nil, fmt.Errorf("failed to check storage quota: %w", err)
		}
		if !canProceed {
			currentGB := float64(currentUsage) / (1024 * 1024 * 1024)
			limitGB := float64(limit) / (1024 * 1024 * 1024)
			attemptGB := float64(delta) / (1024 * 1024 * 1024)
			return nil, fmt.Errorf("storage quota exceeded: current=%.2fGB, limit=%.2fGB, attempt=%.2fGB",
				currentGB, limitGB, attemptGB)
		}
	}

	storageType := config.Current().Storage.Type
	newKey := fmt.Sprintf("upload_files/%s/%s.%s", organizationID, uuid.New().String(), extension)
	hash := fmt.Sprintf("%x", sha256.Sum256(content))
	oldKey := current.Key

	if err := s.storage.Save(newKey, content); err != nil {
		return nil, fmt.Errorf("failed to save replacement file to storage: %w", err)
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{
			"storage_type": storageType,
			"key":          newKey,
			"name":         filename,
			"size":         fileSize,
			"extension":    extension,
			"mime_type":    mimeType,
			"created_by":   userID,
			"created_at":   time.Now(),
			"hash":         hash,
			"source_url":   "",
			"content_text": nil,
		}
		if err := s.fileRepo.WithTx(tx).Update(ctx, fileID, updates); err != nil {
			return fmt.Errorf("failed to update file record: %w", err)
		}
		if err := s.fileRepo.WithTx(tx).DeleteExtractionCaches(ctx, fileID); err != nil {
			return fmt.Errorf("failed to clear file extraction caches: %w", err)
		}

		if delta != 0 && groupID != nil && s.quotaService != nil {
			accountUUID, err := uuid.Parse(userID)
			if err != nil {
				return fmt.Errorf("failed to parse user ID: %w", err)
			}
			tenantUUID, err := uuid.Parse(organizationID)
			if err != nil {
				return fmt.Errorf("failed to parse tenant ID: %w", err)
			}
			operation := quota_model.OperationTypeIncrease
			if delta < 0 {
				operation = quota_model.OperationTypeDecrease
			}
			resourceName := filename
			usageRecord := &quota_model.QuotaUsageHistory{
				ID:            uuid.New().String(),
				GroupID:       *groupID,
				AccountID:     accountUUID,
				TenantID:      &tenantUUID,
				ResourceType:  quota_model.ResourceTypeStorage,
				OperationType: operation,
				Delta:         delta,
				ResourceID:    &fileID,
				ResourceName:  &resourceName,
				Metadata: &quota_model.JSONMap{
					"file_id":       fileID,
					"file_name":     filename,
					"old_file_size": current.Size,
					"new_file_size": fileSize,
					"old_key":       oldKey,
					"new_key":       newKey,
					"operation":     "replace_file_content",
				},
			}
			if err := s.quotaService.RecordUsageInTx(ctx, tx, usageRecord); err != nil {
				return fmt.Errorf("failed to record replacement storage usage: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		if deleteErr := s.storage.Delete(newKey); deleteErr != nil {
			logger.Warn("Failed to delete replacement file after DB rollback", "key", newKey, "error", deleteErr.Error())
		}
		return nil, err
	}

	if oldKey != "" && oldKey != newKey {
		if err := s.storage.Delete(oldKey); err != nil {
			logger.Warn("Failed to delete old file content after replacement", "file_id", fileID, "key", oldKey, "error", err.Error())
		}
	}

	replaced, err := s.fileRepo.GetByTenantAndID(ctx, organizationID, fileID)
	if err != nil {
		return nil, err
	}
	return s.convertToInterfaceUploadFile(replaced), nil
}

func (s *fileService) UploadFileWithOptions(ctx context.Context, filename string, content []byte, mimeType string, userID, organizationID string, userRole model.CreatedByRole, source *interfaces.FileSource, workspaceID *string, isTemporary bool, isIcon bool, options UploadFileOptions) (*dto.UploadFile, error) {
	// If isIcon is true, resize the image to max 200x200
	if isIcon {
		processedContent, err := image.ProcessIconImage(content)
		if err != nil {
			logger.Warn("Failed to process icon image: %v, using original", err)
		} else {
			content = processedContent
		}
	}

	// Get file extension
	extension := strings.ToLower(filepath.Ext(filename))
	if extension != "" {
		extension = extension[1:] // Remove dot
	}

	// Limit filename length
	if len(filename) > 200 {
		nameWithoutExt := strings.TrimSuffix(filename, filepath.Ext(filename))
		if len(nameWithoutExt) > 200 {
			nameWithoutExt = nameWithoutExt[:200]
		}
		filename = nameWithoutExt + "." + extension
	}

	// Check file type (if from datasets source)
	if source != nil && *source == interfaces.FileSourceDatasets {
		if !isAssetProcessableExtension(extension) {
			return nil, model.ErrUnsupportedFileType
		}
	}

	// Check file size
	fileSize := int64(len(content))
	if !s.IsFileSizeWithinLimit(extension, fileSize) {
		return nil, model.ErrFileTooLarge
	}

	// Determine tenant for storage and quota
	uploadOrganizationID := organizationID
	if isTemporary {
		uploadOrganizationID = config.TempFileTenantID
	}

	// Step 1: Get groupID from original tenantID for quota checking
	// In this system, tenantID IS the groupID for quota
	var groupID *uuid.UUID
	parsedGroupID, parseErr := uuid.Parse(organizationID)
	if parseErr == nil {
		groupID = &parsedGroupID
		logger.Info("File upload: using tenantID as groupID=%s", groupID.String())
	} else {
		logger.Warn("Failed to parse tenantID %s as UUID: %v", organizationID, parseErr)
	}

	// Step 2: Check storage quota if groupID exists (skip for temporary uploads)
	if !isTemporary && groupID != nil && s.quotaService != nil {
		canProceed, currentUsage, limit, err := s.quotaService.CheckQuota(ctx, *groupID, quota_model.ResourceTypeStorage, fileSize)
		if err != nil {
			return nil, fmt.Errorf("failed to check storage quota: %w", err)
		}

		// Step 3: If quota exceeded, return specific quota error
		if !canProceed {
			currentGB := float64(currentUsage) / (1024 * 1024 * 1024)
			limitGB := float64(limit) / (1024 * 1024 * 1024)
			attemptGB := float64(fileSize) / (1024 * 1024 * 1024)
			return nil, fmt.Errorf("storage quota exceeded: current=%.2fGB, limit=%.2fGB, attempt=%.2fGB",
				currentGB, limitGB, attemptGB)
		}
	}

	// NOTE: OCR quota is NOT checked during file upload.
	// OCR quota should only be checked when actually processing documents (e.g., during indexing).
	// This allows users to upload files freely and only consume OCR quota when they choose to process them.

	// Generate file UUID and storage path
	fileUUID := uuid.New().String()
	storageType := config.Current().Storage.Type
	fileKey := fmt.Sprintf("upload_files/%s/%s.%s", uploadOrganizationID, fileUUID, extension)

	// Calculate file hash
	hash := fmt.Sprintf("%x", sha256.Sum256(content))

	// Create upload file record
	localUploadFile := &model.UploadFile{
		ID:             fileUUID,
		OrganizationID: uploadOrganizationID,
		WorkspaceID:    workspaceID,
		IsTemporary:    isTemporary,
		StorageType:    storageType,
		Key:            fileKey,
		Name:           filename,
		Size:           fileSize,
		Extension:      extension,
		MimeType:       mimeType,
		CreatedByRole:  model.CreatedByRole(userRole),
		CreatedBy:      userID,
		CreatedAt:      time.Now(),
		Used:           false,
		Hash:           hash,
		SourceURL:      "",
	}

	// Step 4: Persist the object before opening the transaction. Saving to storage
	// is a network call that can take seconds, and holding a database transaction
	// (and its connection) across it serializes concurrent uploads.
	if err := s.storage.Save(fileKey, content); err != nil {
		return nil, fmt.Errorf("failed to save file to storage: %w", err)
	}

	// Step 5: Record the file and its quota usage in a transaction
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Save to database using transaction
		if err := s.fileRepo.WithTx(tx).Create(ctx, localUploadFile); err != nil {
			return fmt.Errorf("failed to save file record: %w", err)
		}

		// Step 6: Record usage history if groupID exists
		if groupID != nil && s.quotaService != nil {
			logger.Info("Recording quota usage: groupID=%s, fileSize=%d, fileUUID=%s", groupID.String(), fileSize, fileUUID)

			// Parse userID to UUID
			accountUUID, err := uuid.Parse(userID)
			if err != nil {
				logger.Info("Failed to parse user ID %s: %v", userID, err)
				return fmt.Errorf("failed to parse user ID: %w", err)
			}

			// Parse tenantID to UUID
			tenantUUID, err := uuid.Parse(organizationID)
			if err != nil {
				logger.Info("Failed to parse tenant ID %s: %v", organizationID, err)
				return fmt.Errorf("failed to parse tenant ID: %w", err)
			}

			// Create usage history record
			usageRecord := &quota_model.QuotaUsageHistory{
				ID:           uuid.New().String(),
				GroupID:      *groupID,
				AccountID:    accountUUID,
				TenantID:     &tenantUUID,
				ResourceType: quota_model.ResourceTypeStorage,
				Delta:        fileSize, // Positive delta for upload
				ResourceID:   &fileUUID,
				ResourceName: &filename,
				Metadata: &quota_model.JSONMap{
					"file_id":   fileUUID,
					"file_name": filename,
					"file_size": fileSize,
					"mime_type": mimeType,
				},
			}

			logger.Info("Created usage record: ID=%s, GroupID=%s, AccountID=%s, TenantID=%s, Delta=%d",
				usageRecord.ID, usageRecord.GroupID.String(), usageRecord.AccountID.String(),
				usageRecord.TenantID.String(), usageRecord.Delta)

			if err := s.quotaService.RecordUsageInTx(ctx, tx, usageRecord); err != nil {
				logger.Info("Failed to record storage usage: %v", err)
				return fmt.Errorf("failed to record storage usage: %w", err)
			}

			logger.Info("Successfully recorded storage quota usage")

			// NOTE: OCR quota usage is NOT recorded during file upload.
			// OCR quota is only consumed when documents are actually processed (e.g., during indexing).
		} else {
			if groupID == nil {
				logger.Warn("Skipping quota recording: groupID is nil")
			}
			if s.quotaService == nil {
				logger.Warn("Skipping quota recording: quotaService is nil")
			}
		}

		return nil
	})

	if err != nil {
		s.compensateFailedUpload(ctx, fileKey, fileUUID)
		return nil, err
	}

	if options.StartLegacyContentExtraction {
		// Start asynchronous file parsing
		go s.ParseFileContent(context.Background(), localUploadFile.ID)
	}

	return s.convertToInterfaceUploadFile(localUploadFile), nil
}

// uploadRollbackReconcileTimeout bounds the record lookup that decides whether an
// object may be removed. The request context is usually already canceled at that
// point, so the lookup runs on a detached context with its own budget.
const uploadRollbackReconcileTimeout = 3 * time.Second

// compensateFailedUpload removes the object written for an upload whose database
// transaction reported a failure.
//
// A transaction can report an error after the database actually committed, for
// example when the connection drops while the commit acknowledgement is in flight.
// Deleting unconditionally would then leave a durable record pointing at a missing
// blob, which is worse for the user than a stray object. Reconcile the record first
// and only remove the object once it is confirmed absent; when the outcome cannot be
// established, keep the object.
func (s *fileService) compensateFailedUpload(ctx context.Context, fileKey, fileID string) {
	reconcileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), uploadRollbackReconcileTimeout)
	defer cancel()

	record, err := s.fileRepo.GetByID(reconcileCtx, fileID)
	switch {
	case err == nil && record != nil:
		logger.Warn("Keeping uploaded object: its record persisted despite the transaction error",
			"key", fileKey, "file_id", fileID)
		return
	case !errors.Is(err, gorm.ErrRecordNotFound):
		// Either the lookup failed or it returned a shape we cannot read as "absent"
		// (a nil record without an error). Only a positive not-found result justifies
		// removing the object. Record the lookup error when there is one.
		fields := []interface{}{"key", fileKey, "file_id", fileID}
		if err != nil {
			fields = append(fields, "error", err.Error())
		} else {
			fields = append(fields, "record", "nil")
		}
		logger.Warn("Keeping uploaded object: could not confirm the record was rolled back", fields...)
		return
	}

	if deleteErr := s.storage.Delete(fileKey); deleteErr != nil {
		logger.Warn("Failed to delete file after DB rollback", "key", fileKey, "error", deleteErr.Error())
	}
}

// AddFileToFolder adds a file to a folder
func (s *fileService) AddFileToFolder(ctx context.Context, fileID, folderID, accountID string) error {
	// This would typically call the file folder service to add the file to a folder
	// For now, we'll return an error indicating this needs to be implemented
	// In a real implementation, this would use a service locator or dependency injection
	// to access the file folder service
	return fmt.Errorf("AddFileToFolder not implemented in fileService, use FileFolderService directly")
}

// convertToInterfaceUploadFile converts local UploadFile to interfaces.UploadFile
func (s *fileService) convertToInterfaceUploadFile(local *model.UploadFile) *dto.UploadFile {
	return &dto.UploadFile{
		ID:             local.ID,
		TenantID:       local.OrganizationID,
		OrganizationID: local.OrganizationID,
		TeamTenantID:   local.WorkspaceID,
		WorkspaceID:    local.WorkspaceID,
		StorageType:    local.StorageType,
		Key:            local.Key,
		Name:           local.Name,
		Size:           local.Size,
		Extension:      local.Extension,
		MimeType:       local.MimeType,
		CreatedByRole:  dto.CreatedByRole(local.CreatedByRole),
		CreatedBy:      local.CreatedBy,
		CreatedAt:      local.CreatedAt,
		Used:           local.Used,
		UsedBy:         local.UsedBy,
		UsedAt:         local.UsedAt,
		Hash:           local.Hash,
		SourceURL:      local.SourceURL,
		ContentText:    local.ContentText,
		IsTemporary:    local.IsTemporary,
	}
}

func (s *fileService) getFileInner(ctx context.Context, fileID string, isPreview bool, enableOCR *bool) (string, error) {
	// Get file record
	uploadFile, err := s.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		return "", model.ErrFileNotFound
	}

	// Check if file extension is supported
	if !model.IsDocumentExtension(uploadFile.Extension) {
		return "", model.ErrUnsupportedFileType
	}

	var content string
	// If parsed text content already exists, return directly
	if uploadFile.ContentText != nil && *uploadFile.ContentText != "" {
		content = *uploadFile.ContentText
	} else {
		// Use ExtractProcessor to extract file content
		var extractOutput *dto.ExtractOutput
		var err error

		// If enableOCR is specified, use LoadFromUploadFileWithSetting to control OCR
		if enableOCR != nil {
			setting := &extractor.ExtractSetting{
				DatasourceType: extractor.DatasourceTypeFile,
				UploadFile:     uploadFile,
				DocumentModel:  "text_model",
				ProcessRule: &dataset_model.DatasetProcessRule{
					Mode: "automatic",
					Rules: map[string]interface{}{
						"pre_processing_rules": []interface{}{
							map[string]interface{}{
								"id":      "image_content_recognition",
								"enabled": *enableOCR,
							},
						},
					},
				},
			}
			extractOutput, _, err = s.extractor.LoadFromUploadFileWithSetting(ctx, uploadFile, true, false, setting)
		} else {
			// Use default extraction
			extractOutput, _, err = s.extractor.LoadFromUploadFile(ctx, uploadFile, true, false)
		}

		if err != nil {
			return "", fmt.Errorf("failed to extract file content: %w", err)
		}

		content = dto.ExtractOutputText(extractOutput)
	}

	if isPreview {
		const PREVIEW_WORDS_LIMIT = 3000
		// Limit preview content length
		if utf8.RuneCountInString(content) > PREVIEW_WORDS_LIMIT {
			runes := []rune(content)
			content = string(runes[:PREVIEW_WORDS_LIMIT])
		}
	}

	return content, nil
}

// GetFilePreview get file preview content
func (s *fileService) GetFilePreview(ctx context.Context, fileID string) (string, error) {
	return s.getFileInner(ctx, fileID, true, nil)
}

// GetFilePreviewWithOCR get file preview content with OCR control
func (s *fileService) GetFilePreviewWithOCR(ctx context.Context, fileID string, enableOCR bool) (string, error) {
	return s.getFileInner(ctx, fileID, true, &enableOCR)
}

func (s *fileService) GetFile(ctx context.Context, fileID string) (string, error) {
	return s.getFileInner(ctx, fileID, false, nil)
}

// ExtractFileWithSetting explicitly extracts file content with caller-provided
// parsing settings and bypasses cached ContentText from upload-time parsing.
func (s *fileService) ExtractFileWithSetting(ctx context.Context, fileID string, setting interfaces.FileExtractionSetting) (string, error) {
	uploadFile, err := s.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		return "", model.ErrFileNotFound
	}

	if !model.IsDocumentExtension(uploadFile.Extension) {
		return "", model.ErrUnsupportedFileType
	}

	cacheKey := extractionSettingCacheKey(setting)
	if cacheKey != "" {
		cache, err := s.fileRepo.GetExtractionCache(ctx, fileID, cacheKey)
		if err == nil && strings.TrimSpace(cache.Content) != "" {
			logger.InfoContext(ctx, "file extraction cache hit", "file_id", fileID, "cache_key", cacheKey, "source", cache.Source)
			return cache.Content, nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			logger.WarnContext(ctx, "failed to read file extraction cache", "file_id", fileID, "cache_key", cacheKey, err)
		}

		groupKey := fileID + ":" + cacheKey
		value, err, shared := s.extractGroup.Do(groupKey, func() (interface{}, error) {
			cache, err := s.fileRepo.GetExtractionCache(ctx, fileID, cacheKey)
			if err == nil && strings.TrimSpace(cache.Content) != "" {
				logger.InfoContext(ctx, "file extraction cache hit after wait", "file_id", fileID, "cache_key", cacheKey, "source", cache.Source)
				return cache.Content, nil
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				logger.WarnContext(ctx, "failed to read file extraction cache", "file_id", fileID, "cache_key", cacheKey, err)
			}
			logger.InfoContext(ctx, "file extraction cache miss", "file_id", fileID, "cache_key", cacheKey, "source", setting.CacheNamespace, "strategy", setting.ExtractionStrategy)
			return s.extractAndCacheFileWithSetting(ctx, uploadFile, cacheKey, setting)
		})
		if err != nil {
			return "", err
		}
		content, _ := value.(string)
		if shared {
			logger.InfoContext(ctx, "file extraction shared in-flight result", "file_id", fileID, "cache_key", cacheKey, "source", setting.CacheNamespace)
		}
		return content, nil
	}

	return s.extractAndCacheFileWithSetting(ctx, uploadFile, "", setting)
}

func (s *fileService) extractAndCacheFileWithSetting(ctx context.Context, uploadFile *model.UploadFile, cacheKey string, setting interfaces.FileExtractionSetting) (string, error) {
	start := time.Now()
	var processRule *dataset_model.DatasetProcessRule
	if setting.EnableOCR != nil {
		processRule = &dataset_model.DatasetProcessRule{
			Mode: "automatic",
			Rules: map[string]interface{}{
				"pre_processing_rules": []interface{}{
					map[string]interface{}{
						"id":      "image_content_recognition",
						"enabled": *setting.EnableOCR,
					},
				},
			},
		}
	}

	extractSetting := &extractor.ExtractSetting{
		DatasourceType:            extractor.DatasourceTypeFile,
		UploadFile:                uploadFile,
		DocumentModel:             "text_model",
		ProcessRule:               processRule,
		ExtractionStrategy:        setting.ExtractionStrategy,
		ExtractionFallbackEnabled: setting.ExtractionFallbackEnabled,
	}

	extractOutput, text, err := s.extractor.LoadFromUploadFileWithSetting(ctx, uploadFile, true, false, extractSetting)
	if err != nil {
		return "", fmt.Errorf("failed to extract file content: %w", err)
	}
	content := text
	if text != "" {
		content = text
	} else {
		content = dto.ExtractOutputText(extractOutput)
	}
	if cacheKey != "" && strings.TrimSpace(content) != "" {
		if err := s.fileRepo.UpsertExtractionCache(ctx, &model.FileExtractionCache{
			FileID:   uploadFile.ID,
			CacheKey: cacheKey,
			Content:  content,
			Source:   setting.CacheNamespace,
		}); err != nil {
			logger.WarnContext(ctx, "failed to write file extraction cache", "file_id", uploadFile.ID, "cache_key", cacheKey, err)
		} else {
			logger.InfoContext(ctx, "file extraction cache stored", "file_id", uploadFile.ID, "cache_key", cacheKey, "source", setting.CacheNamespace, "content_len", len(content), "duration_ms", time.Since(start).Milliseconds())
		}
	}
	return content, nil
}

func extractionSettingCacheKey(setting interfaces.FileExtractionSetting) string {
	namespace := strings.TrimSpace(setting.CacheNamespace)
	if namespace == "" {
		return ""
	}
	fallback := "nil"
	if setting.ExtractionFallbackEnabled != nil {
		fallback = fmt.Sprintf("%t", *setting.ExtractionFallbackEnabled)
	}
	ocr := "nil"
	if setting.EnableOCR != nil {
		ocr = fmt.Sprintf("%t", *setting.EnableOCR)
	}
	raw := fmt.Sprintf(
		"%s|strategy=%s|fallback=%s|ocr=%s",
		namespace,
		strings.TrimSpace(setting.ExtractionStrategy),
		fallback,
		ocr,
	)
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%s:%x", namespace, sum[:16])
}

// GetSupportedFileTypes gets supported file types
func (s *fileService) GetSupportedFileTypes() []string {
	return model.GetSupportedDocumentExtensions()
}

// IsFileSizeWithinLimit check if file size is within limit
func (s *fileService) IsFileSizeWithinLimit(extension string, fileSize int64) bool {
	var fileSizeLimit int64

	cfg := config.Current()
	// Use centralized configuration values (convert MB to bytes)
	if model.IsImageExtension(extension) {
		fileSizeLimit = int64(cfg.Upload.ImageSizeLimit) * 1024 * 1024 // MB to bytes
	} else if model.IsVideoExtension(extension) {
		fileSizeLimit = int64(cfg.Upload.VideoSizeLimit) * 1024 * 1024
	} else if model.IsAudioExtension(extension) {
		fileSizeLimit = int64(cfg.Upload.AudioSizeLimit) * 1024 * 1024
	} else {
		fileSizeLimit = int64(cfg.Upload.FileSizeLimit) * 1024 * 1024
	}

	return fileSize <= fileSizeLimit
}

// ParseFileContent parse file content for indexing
func (s *fileService) ParseFileContent(ctx context.Context, uploadFileID string) {
	// Get file record
	uploadFile, err := s.fileRepo.GetByID(ctx, uploadFileID)
	if err != nil {
		return
	}

	// Only process document type files
	if !model.IsDocumentExtension(uploadFile.Extension) {
		return
	}

	// Use ExtractProcessor for all document types
	// This ensures consistent extraction across the application
	extractOutput, text, err := s.extractor.LoadFromUploadFile(ctx, uploadFile, true, false)
	if err != nil {
		// Log error but don't fail - content extraction is optional
		logger.WarnContext(ctx, "failed to extract file content", "file_id", uploadFileID, err)
		return
	}

	// If extraction succeeded and we have content, update the database
	if text != "" {
		s.fileRepo.UpdateContentText(ctx, uploadFileID, text)
	} else if combinedText := dto.ExtractOutputText(extractOutput); combinedText != "" {
		s.fileRepo.UpdateContentText(ctx, uploadFileID, combinedText)
	}
}

// GetFileByID retrieves file information by file ID
func (s *fileService) GetFileByID(ctx context.Context, fileID string) (*dto.UploadFile, error) {
	localUploadFile, err := s.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get file by ID: %w", err)
	}

	return s.convertToInterfaceUploadFile(localUploadFile), nil
}

// DownloadFile downloads a file by its ID
func (s *fileService) DownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	// Get file record from database
	uploadFile, err := s.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get file record: %w", err)
	}

	// Load file content from storage
	content, err := s.storage.Load(uploadFile.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to load file from storage: %w", err)
	}

	return content, nil
}

// GetFileURL returns a signed URL for accessing a file by its ID
func (s *fileService) GetFileURL(ctx context.Context, fileID string) (string, error) {
	_, err := s.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		return "", fmt.Errorf("failed to get file by ID: %w", err)
	}

	// Use GetSignedFileURL to generate a signed URL (same as avatar_url)
	signedURL, err := util.GetSignedFileURL(fileID)
	if err != nil {
		return "", fmt.Errorf("failed to generate signed URL: %w", err)
	}

	return signedURL, nil
}

// ListFiles paginated list of files
func (s *fileService) ListFiles(ctx context.Context, tenantID, accountID string, req *dto.FileListRequest, visibleWorkspaceIDs []string) (*dto.FileListResponse, error) {
	if len(visibleWorkspaceIDs) == 0 {
		return &dto.FileListResponse{
			Data:    []dto.UploadFile{},
			HasMore: false,
			Limit:   req.Limit,
			Total:   0,
			Page:    req.Page,
		}, nil
	}

	allowAllFolders := false

	var total int64
	var fileModels []*model.UploadFile
	fileModels, total, err := s.fileRepo.ListByTenantIDs(ctx, tenantID, accountID, allowAllFolders, visibleWorkspaceIDs, req.Page, req.Limit, req.Keyword, req.Sort, req.Extension, &req.StartTime, &req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	// Convert to interface file objects
	interfaceFiles := make([]*dto.UploadFile, len(fileModels))
	for i, file := range fileModels {
		interfaceFiles[i] = s.convertToInterfaceUploadFile(file)
	}

	// Calculate has more
	hasMore := int64(req.Page*req.Limit) < total

	// Build response data
	fileList := make([]dto.UploadFile, len(interfaceFiles))
	for i, file := range interfaceFiles {
		fileList[i] = *file
	}

	return &dto.FileListResponse{
		Data:    fileList,
		HasMore: hasMore,
		Limit:   req.Limit,
		Total:   total,
		Page:    req.Page,
	}, nil
}

// ListArchivedFiles paginated list of archived files
func (s *fileService) ListArchivedFiles(ctx context.Context, tenantID, accountID string, req *dto.FileListRequest, visibleWorkspaceIDs []string) (*dto.FileListResponse, error) {
	if len(visibleWorkspaceIDs) == 0 {
		return &dto.FileListResponse{
			Data:    []dto.UploadFile{},
			HasMore: false,
			Limit:   req.Limit,
			Total:   0,
			Page:    req.Page,
		}, nil
	}

	allowAllFolders := false

	var fileModels []*model.UploadFile
	var total int64
	fileModels, total, err := s.fileRepo.ListArchivedByTenantIDs(ctx, tenantID, accountID, allowAllFolders, visibleWorkspaceIDs, req.Page, req.Limit, req.Keyword, req.Sort, req.Extension, &req.StartTime, &req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to list archived files: %w", err)
	}

	// Convert to interface file objects
	interfaceFiles := make([]*dto.UploadFile, len(fileModels))
	for i, file := range fileModels {
		interfaceFiles[i] = s.convertToInterfaceUploadFile(file)
	}

	// Calculate has more
	hasMore := int64(req.Page*req.Limit) < total

	// Build response data
	fileList := make([]dto.UploadFile, len(interfaceFiles))
	for i, file := range interfaceFiles {
		fileList[i] = *file
	}

	return &dto.FileListResponse{
		Data:    fileList,
		HasMore: hasMore,
		Limit:   req.Limit,
		Total:   total,
		Page:    req.Page,
	}, nil
}

// DeleteFiles deletes files by their IDs after checking for related documents
func (s *fileService) DeleteFiles(ctx context.Context, fileIDs []string) error {
	for _, fileID := range fileIDs {
		// Check if file exists
		file, err := s.fileRepo.GetByID(ctx, fileID)
		if err != nil {
			return fmt.Errorf("file %s not found: %w", fileID, err)
		}

		// Check if file is used by any documents
		isUsed, err := s.fileRepo.CheckIfFileIsUsed(ctx, fileID)
		if err != nil {
			return fmt.Errorf("failed to check if file %s is used: %w", fileID, err)
		}

		if isUsed {
			return fmt.Errorf("file %s is used by documents and cannot be deleted", fileID)
		}

		// Get groupID from tenantID for quota recording
		// In this system, tenantID IS the groupID
		var groupID *uuid.UUID
		parsedGroupID, parseErr := uuid.Parse(file.OrganizationID)
		if parseErr == nil {
			groupID = &parsedGroupID
			logger.Info("File deletion: using tenantID as groupID=%s", groupID.String())
		}

		// Resolve everything that depends on the object still being present, and fail
		// before anything is destroyed. A parse failure used to abort a delete that had
		// already removed the object, and the page count was read after it was gone.
		prep, err := s.prepareDeletion(file, groupID)
		if err != nil {
			return err
		}

		// Execute deletion and record usage in a transaction. The object is deliberately
		// left in place until the transaction commits: removing it first meant a rolled
		// back delete kept the record while its content was already gone.
		err = s.db.Transaction(func(tx *gorm.DB) error {
			// Runs before every destructive step: this is the check that can still refuse
			// the delete.
			if s.assetDeletion != nil {
				if err := s.assetDeletion.DeleteBySourceFileInTx(ctx, tx, file.OrganizationID, fileID); err != nil {
					if errors.Is(err, datalibraryservice.ErrFileAssetDeletionBlocked) {
						return fmt.Errorf("file %s is used by documents and cannot be deleted: %w", fileID, err)
					}
					return fmt.Errorf("failed to delete file %s asset data: %w", fileID, err)
				}
			}

			// Delete file record from database
			if err := s.fileRepo.WithTx(tx).Delete(ctx, fileID); err != nil {
				return fmt.Errorf("failed to delete file %s record: %w", fileID, err)
			}

			// Record usage decrease if groupID exists
			if prep.quotaEnabled {
				// Create usage history record with negative delta
				usageRecord := &quota_model.QuotaUsageHistory{
					ID:           uuid.New().String(),
					GroupID:      *groupID,
					AccountID:    prep.accountUUID,
					TenantID:     &prep.tenantUUID,
					ResourceType: quota_model.ResourceTypeStorage,
					Delta:        -file.Size, // Negative delta for deletion
					ResourceID:   &fileID,
					ResourceName: &file.Name,
					Metadata: &quota_model.JSONMap{
						"file_id":   fileID,
						"file_name": file.Name,
						"file_size": file.Size,
						"action":    "deleted",
					},
				}

				if err := s.quotaService.RecordUsageInTx(ctx, tx, usageRecord); err != nil {
					return fmt.Errorf("failed to record storage usage decrease: %w", err)
				}

				// Record OCR quota decrease for PDF and image files
				if prep.countOCR {
					// Create OCR usage history record with negative delta
					ocrUsageRecord := &quota_model.QuotaUsageHistory{
						ID:           uuid.New().String(),
						GroupID:      *groupID,
						AccountID:    prep.accountUUID,
						TenantID:     &prep.tenantUUID,
						ResourceType: quota_model.ResourceTypeOCRPages,
						Delta:        -prep.ocrPages, // Negative delta for deletion
						ResourceID:   &fileID,
						ResourceName: &file.Name,
						Metadata: &quota_model.JSONMap{
							"file_id":   fileID,
							"file_name": file.Name,
							"file_size": file.Size,
							"pages":     prep.ocrPages,
							"action":    "deleted",
						},
					}

					if err := s.quotaService.RecordUsageInTx(ctx, tx, ocrUsageRecord); err != nil {
						return fmt.Errorf("failed to record OCR usage decrease: %w", err)
					}

					logger.Info("Successfully recorded OCR quota decrease: %d pages", prep.ocrPages)
				}
			}

			return nil
		})

		if err != nil {
			return err
		}

		// The record is gone, so nothing can reach the object any more. Removing it is
		// best effort: an orphan object is harmless, while failing here would report a
		// failed delete for a file that is already gone.
		if err := s.storage.Delete(file.Key); err != nil {
			logger.Warn("Failed to remove file object after its record was deleted",
				"file_id", fileID, "key", file.Key, "error", err.Error())
		}
	}

	return nil
}

// deletionPreparation holds everything a delete transaction needs that has to be resolved
// while the file object is still readable.
type deletionPreparation struct {
	quotaEnabled bool
	accountUUID  uuid.UUID
	tenantUUID   uuid.UUID
	countOCR     bool
	ocrPages     int64
}

// prepareDeletion resolves the quota identifiers and the OCR page count before the delete
// touches anything. Both used to be evaluated inside the transaction, where an
// unparseable id aborted a delete that had already removed the object, and where the page
// count was read back after the object was gone and therefore always fell back to one.
func (s *fileService) prepareDeletion(file *model.UploadFile, groupID *uuid.UUID) (deletionPreparation, error) {
	var prep deletionPreparation
	if groupID == nil || s.quotaService == nil {
		return prep, nil
	}

	accountUUID, err := uuid.Parse(file.CreatedBy)
	if err != nil {
		return prep, fmt.Errorf("failed to parse user ID: %w", err)
	}
	tenantUUID, err := uuid.Parse(file.OrganizationID)
	if err != nil {
		return prep, fmt.Errorf("failed to parse tenant ID: %w", err)
	}

	prep.quotaEnabled = true
	prep.accountUUID = accountUUID
	prep.tenantUUID = tenantUUID

	if !s.shouldCheckOCRQuota(file.Extension) {
		return prep, nil
	}

	prep.countOCR = true
	prep.ocrPages = s.expectedOCRPagesOnDelete(file)
	return prep, nil
}

// expectedOCRPagesOnDelete reports how many OCR pages to credit back for a deleted file.
// It reads the object, so it has to run before the object is removed.
func (s *fileService) expectedOCRPagesOnDelete(file *model.UploadFile) int64 {
	// Images and every other OCR format count as a single page.
	if file.Extension != "pdf" {
		return 1
	}

	content, err := s.storage.Load(file.Key)
	if err != nil {
		logger.Warn("Falling back to one OCR page: could not read the file to count its pages",
			"file_id", file.ID, "key", file.Key, "error", err.Error())
		return 1
	}

	tempDir, err := os.MkdirTemp("", "pdf_ocr_delete")
	if err != nil {
		return 1
	}
	defer os.RemoveAll(tempDir)

	tempFile := filepath.Join(tempDir, "temp.pdf")
	if err := os.WriteFile(tempFile, content, 0644); err != nil {
		return 1
	}

	pageCount, err := s.getQuickPDFPageCount(tempFile)
	if err != nil || pageCount <= 0 {
		return 1
	}
	return int64(pageCount)
}

// GetStorageUsage gets the storage usage for a tenant
func (s *fileService) GetStorageUsage(ctx context.Context, tenantID string) (int64, error) {
	return s.fileRepo.GetTotalSizeByTenantID(ctx, tenantID)
}

// UpdateContentText updates the cached content text for a file
func (s *fileService) UpdateContentText(ctx context.Context, fileID string, contentText string) error {
	return s.fileRepo.UpdateContentText(ctx, fileID, contentText)
}

// shouldCheckOCRQuota determines if OCR quota should be checked for a file extension
func (s *fileService) shouldCheckOCRQuota(extension string) bool {
	// Get ETL type from config
	cfg := config.GlobalConfig
	if cfg == nil {
		return false
	}

	etlType := cfg.ETL.Type

	// Only check OCR quota if using Reducto or Mixed mode
	if etlType != "Reducto" && etlType != "Mixed" {
		return false
	}

	// Check if the file type would use OCR
	switch extension {
	case "pdf", "png", "jpg", "jpeg", "gif", "bmp", "tiff", "tif",
		"pcx", "ppm", "apng", "psd", "cur", "dcx", "heic":
		return true
	default:
		return false
	}
}

// getQuickPDFPageCount quickly reads the page count from a PDF file without parsing content
func (s *fileService) getQuickPDFPageCount(filePath string) (int, error) {
	// Disable debug output
	pdf.DebugOn = false

	// Open PDF file
	file, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open PDF: %w", err)
	}
	defer file.Close()

	// Get file info for size
	fileInfo, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("failed to get file info: %w", err)
	}

	// Use pdf.NewReader to read the PDF
	pdfReader, err := pdf.NewReader(file, fileInfo.Size())
	if err != nil {
		return 0, fmt.Errorf("failed to create PDF reader: %w", err)
	}

	// Get the number of pages
	numPages := pdfReader.NumPage()
	return numPages, nil
}
