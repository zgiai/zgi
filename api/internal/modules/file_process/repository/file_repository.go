package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	file_model "github.com/zgiai/zgi/api/internal/modules/file_process/model"
)

type FileRepository interface {
	Create(ctx context.Context, uploadFile *file_model.UploadFile) error
	GetByID(ctx context.Context, id string) (*file_model.UploadFile, error)
	GetByTenantAndID(ctx context.Context, tenantID, id string) (*file_model.UploadFile, error)
	ListByTenantAndIDs(ctx context.Context, tenantID string, ids []string) (map[string]*file_model.UploadFile, error)
	UpdateContentText(ctx context.Context, id string, contentText string) error
	GetExtractionCache(ctx context.Context, fileID, cacheKey string) (*file_model.FileExtractionCache, error)
	UpsertExtractionCache(ctx context.Context, cache *file_model.FileExtractionCache) error
	Update(ctx context.Context, id string, updates map[string]interface{}) error
	MarkAsUsed(ctx context.Context, id, usedBy string) error
	ListByTenantID(ctx context.Context, tenantID, accountID string, allowAllFolders bool, workspaceID string, page, pageSize int, keyword, sort, extension string, startTime, endTime *time.Time) ([]*file_model.UploadFile, int64, error)
	ListArchivedByTenantID(ctx context.Context, tenantID, accountID string, allowAllFolders bool, workspaceID string, page, pageSize int, keyword, sort, extension string, startTime, endTime *time.Time) ([]*file_model.UploadFile, int64, error)
	ListByTenantIDs(ctx context.Context, tenantID, accountID string, allowAllFolders bool, workspaceIDs []string, page, pageSize int, keyword, sort, extension string, startTime, endTime *time.Time) ([]*file_model.UploadFile, int64, error)
	ListArchivedByTenantIDs(ctx context.Context, tenantID, accountID string, allowAllFolders bool, workspaceIDs []string, page, pageSize int, keyword, sort, extension string, startTime, endTime *time.Time) ([]*file_model.UploadFile, int64, error)
	// GetTotalSizeByTenantID gets the total size of all files for a tenant
	GetTotalSizeByTenantID(ctx context.Context, tenantID string) (int64, error)
	// Delete deletes a file by ID
	Delete(ctx context.Context, id string) error
	// CheckIfFileIsUsed checks if a file is used by any documents
	CheckIfFileIsUsed(ctx context.Context, id string) (bool, error)
	// WithTx returns a new repository instance with the given transaction
	WithTx(tx *gorm.DB) FileRepository
}

type fileRepository struct {
	db *gorm.DB
}

func NewFileRepository(db *gorm.DB) FileRepository {
	return &fileRepository{
		db: db,
	}
}

func (r *fileRepository) WithTx(tx *gorm.DB) FileRepository {
	return &fileRepository{
		db: tx,
	}
}

func (r *fileRepository) Create(ctx context.Context, uploadFile *file_model.UploadFile) error {
	return r.db.WithContext(ctx).Create(uploadFile).Error
}

func (r *fileRepository) GetByID(ctx context.Context, id string) (*file_model.UploadFile, error) {
	var uploadFile file_model.UploadFile
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&uploadFile).Error
	if err != nil {
		return nil, err
	}
	return &uploadFile, nil
}

func (r *fileRepository) GetByTenantAndID(ctx context.Context, tenantID, id string) (*file_model.UploadFile, error) {
	var uploadFile file_model.UploadFile
	err := r.db.WithContext(ctx).Where("organization_id = ? AND id = ?", tenantID, id).First(&uploadFile).Error
	if err != nil {
		return nil, err
	}
	return &uploadFile, nil
}

func (r *fileRepository) ListByTenantAndIDs(ctx context.Context, tenantID string, ids []string) (map[string]*file_model.UploadFile, error) {
	result := make(map[string]*file_model.UploadFile, len(ids))
	if tenantID == "" || len(ids) == 0 {
		return result, nil
	}

	var files []*file_model.UploadFile
	if err := r.db.WithContext(ctx).
		Where("organization_id = ? AND id IN ?", tenantID, ids).
		Find(&files).Error; err != nil {
		return nil, err
	}
	for _, file := range files {
		result[file.ID] = file
	}
	return result, nil
}

func (r *fileRepository) UpdateContentText(ctx context.Context, id string, contentText string) error {
	return r.db.WithContext(ctx).Model(&file_model.UploadFile{}).
		Where("id = ?", id).
		Update("content_text", contentText).Error
}

func (r *fileRepository) GetExtractionCache(ctx context.Context, fileID, cacheKey string) (*file_model.FileExtractionCache, error) {
	var cache file_model.FileExtractionCache
	err := r.db.WithContext(ctx).
		Where("file_id = ? AND cache_key = ?", fileID, cacheKey).
		First(&cache).Error
	if err != nil {
		return nil, err
	}
	return &cache, nil
}

func (r *fileRepository) UpsertExtractionCache(ctx context.Context, cache *file_model.FileExtractionCache) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "file_id"},
			{Name: "cache_key"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"content", "source", "updated_at"}),
	}).Create(cache).Error
}

func (r *fileRepository) Update(ctx context.Context, id string, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&file_model.UploadFile{}).
		Where("id = ?", id).
		Updates(updates).Error
}

func (r *fileRepository) MarkAsUsed(ctx context.Context, id, usedBy string) error {
	return r.db.WithContext(ctx).Model(&file_model.UploadFile{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"used":    true,
			"used_by": usedBy,
			"used_at": gorm.Expr("NOW()"),
		}).Error
}

func applyVisibleFileAccessFilter(query *gorm.DB, workspaceIDs []string, accountID string, allowAllFolders bool) *gorm.DB {
	if allowAllFolders || len(workspaceIDs) == 0 {
		return query
	}

	return query.Where(
		`NOT EXISTS (
			SELECT 1
			FROM file_folder_joins ffj
			WHERE ffj.file_id = upload_files.id
		) OR EXISTS (
			SELECT 1
			FROM file_folder_joins ffj
			JOIN file_folders ff ON ff.id = ffj.folder_id
			WHERE ffj.file_id = upload_files.id
			  AND (
			    ff.permission = ?
			    OR (ff.permission = ? AND ff.created_by = ?)
			    OR (
			      ff.permission = ?
			      AND EXISTS (
			        SELECT 1
			        FROM file_folder_permissions ffp
			        WHERE ffp.folder_id = ff.id AND ffp.workspace_id IN ?
			      )
			    )
			  )
		)`,
		string(file_model.FileFolderPermissionAllTeam),
		string(file_model.FileFolderPermissionOnlyMe),
		accountID,
		string(file_model.FileFolderPermissionPartialTeam),
		workspaceIDs,
	)
}

func (r *fileRepository) ListByTenantID(ctx context.Context, tenantID, accountID string, allowAllFolders bool, workspaceID string, page, pageSize int, keyword, sort, extension string, startTime, endTime *time.Time) ([]*file_model.UploadFile, int64, error) {
	if workspaceID != "" {
		return r.ListByTenantIDs(ctx, tenantID, accountID, allowAllFolders, []string{workspaceID}, page, pageSize, keyword, sort, extension, startTime, endTime)
	}
	return r.ListByTenantIDs(ctx, tenantID, accountID, allowAllFolders, nil, page, pageSize, keyword, sort, extension, startTime, endTime)
}

func (r *fileRepository) ListByTenantIDs(ctx context.Context, tenantID, accountID string, allowAllFolders bool, workspaceIDs []string, page, pageSize int, keyword, sort, extension string, startTime, endTime *time.Time) ([]*file_model.UploadFile, int64, error) {
	var files []*file_model.UploadFile
	var total int64

	// Calculate offset
	offset := (page - 1) * pageSize

	// Build query
	query := r.db.WithContext(ctx).Model(&file_model.UploadFile{}).Where("organization_id = ?", tenantID)

	if len(workspaceIDs) > 0 {
		query = query.Where("workspace_id IN ?", workspaceIDs)
	}
	query = applyVisibleFileAccessFilter(query, workspaceIDs, accountID, allowAllFolders)

	// Add keyword filter if provided
	if keyword != "" {
		likeKeyword := "%" + keyword + "%"
		query = query.Where("name LIKE ?", likeKeyword)
	}

	// Add extension filter if provided
	if extension != "" {
		query = query.Where("extension = ?", extension)
	}

	// Add time range filter if provided
	if startTime != nil && !startTime.IsZero() {
		query = query.Where("created_at >= ?", startTime)
	}
	if endTime != nil && !endTime.IsZero() {
		query = query.Where("created_at <= ?", endTime)
	}

	// Query total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Determine sort order
	order := "created_at DESC"
	if sort != "" {
		switch sort {
		case "created_at_asc":
			order = "created_at ASC"
		case "created_at_desc":
			order = "created_at DESC"
		case "name_asc":
			order = "name ASC"
		case "name_desc":
			order = "name DESC"
		case "size_asc":
			order = "size ASC"
		case "size_desc":
			order = "size DESC"
		default:
			order = "created_at DESC"
		}
	}

	// Query current page data, excluding content_text field to reduce network transfer
	if err := query.Select([]string{
		"id", "organization_id", "storage_type", "key", "name", "size",
		"extension", "mime_type", "created_by_role", "created_by",
		"created_at", "used", "used_by", "used_at", "hash", "source_url",
	}).
		Offset(offset).
		Limit(pageSize).
		Order(order).
		Find(&files).Error; err != nil {
		return nil, 0, err
	}

	return files, total, nil
}

func (r *fileRepository) ListArchivedByTenantID(ctx context.Context, organizationID, accountID string, allowAllFolders bool, workspaceID string, page, pageSize int, keyword, sort, extension string, startTime, endTime *time.Time) ([]*file_model.UploadFile, int64, error) {
	if workspaceID != "" {
		return r.ListArchivedByTenantIDs(ctx, organizationID, accountID, allowAllFolders, []string{workspaceID}, page, pageSize, keyword, sort, extension, startTime, endTime)
	}
	return r.ListArchivedByTenantIDs(ctx, organizationID, accountID, allowAllFolders, nil, page, pageSize, keyword, sort, extension, startTime, endTime)
}

func (r *fileRepository) ListArchivedByTenantIDs(ctx context.Context, organizationID, accountID string, allowAllFolders bool, workspaceIDs []string, page, pageSize int, keyword, sort, extension string, startTime, endTime *time.Time) ([]*file_model.UploadFile, int64, error) {
	var files []*file_model.UploadFile
	var total int64

	// Calculate offset
	offset := (page - 1) * pageSize

	// Build query for archived files
	query := r.db.WithContext(ctx).Model(&file_model.UploadFile{}).Where("organization_id = ? AND is_archived = true", organizationID)

	if len(workspaceIDs) > 0 {
		query = query.Where("workspace_id IN ?", workspaceIDs)
	}
	query = applyVisibleFileAccessFilter(query, workspaceIDs, accountID, allowAllFolders)

	// Add keyword filter if provided
	if keyword != "" {
		likeKeyword := "%" + keyword + "%"
		query = query.Where("name LIKE ?", likeKeyword)
	}

	// Add extension filter if provided
	if extension != "" {
		query = query.Where("extension = ?", extension)
	}

	// Add time range filter if provided
	if startTime != nil && !startTime.IsZero() {
		query = query.Where("created_at >= ?", startTime)
	}
	if endTime != nil && !endTime.IsZero() {
		query = query.Where("created_at <= ?", endTime)
	}

	// Query total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Determine sort order
	order := "created_at DESC"
	if sort != "" {
		switch sort {
		case "created_at_asc":
			order = "created_at ASC"
		case "created_at_desc":
			order = "created_at DESC"
		case "name_asc":
			order = "name ASC"
		case "name_desc":
			order = "name DESC"
		case "size_asc":
			order = "size ASC"
		case "size_desc":
			order = "size DESC"
		default:
			order = "created_at DESC"
		}
	}

	// Query current page data, excluding content_text field to reduce network transfer
	if err := query.Select([]string{
		"id", "organization_id", "storage_type", "key", "name", "size",
		"extension", "mime_type", "created_by_role", "created_by",
		"created_at", "used", "used_by", "used_at", "hash", "source_url",
		"is_archived", "archived_at", "archived_by",
	}).
		Offset(offset).
		Limit(pageSize).
		Order(order).
		Find(&files).Error; err != nil {
		return nil, 0, err
	}

	return files, total, nil
}

// GetTotalSizeByTenantID gets the total size of all files for a tenant
func (r *fileRepository) GetTotalSizeByTenantID(ctx context.Context, organizationID string) (int64, error) {
	var totalSize int64
	err := r.db.WithContext(ctx).
		Model(&file_model.UploadFile{}).
		Where("organization_id = ?", organizationID).
		Select("COALESCE(SUM(size), 0)").
		Scan(&totalSize).Error
	return totalSize, err
}

// Delete deletes a file by ID
func (r *fileRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// First delete any folder associations
		if err := tx.Where("file_id = ?", id).Delete(&file_model.FileFolderJoins{}).Error; err != nil {
			return err
		}

		// Delete any favorite associations
		if err := tx.Where("file_id = ?", id).Delete(&file_model.FileFavorite{}).Error; err != nil {
			return err
		}

		// Then delete the file itself
		if err := tx.Where("id = ?", id).Delete(&file_model.UploadFile{}).Error; err != nil {
			return err
		}

		return nil
	})
}

// CheckIfFileIsUsed checks if a file is referenced by active knowledge-base asset refs.
func (r *fileRepository) CheckIfFileIsUsed(ctx context.Context, id string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("data_library_document_assets AS assets").
		Joins("JOIN data_library_knowledge_base_asset_refs AS refs ON refs.asset_id = assets.id AND refs.deleted_at IS NULL").
		Joins("JOIN datasets ON datasets.id = refs.dataset_id").
		Where("assets.source_file_id = ? AND assets.deleted_at IS NULL", id).
		Distinct("refs.dataset_id").
		Count(&count).Error

	if err != nil {
		return false, err
	}

	return count > 0, nil
}
