package repository

import (
	"context"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"

	file_model "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type FileFolderRepository interface {
	// Folder operations
	CreateFolder(ctx context.Context, folder *file_model.FileFolder) error
	GetFolderByID(ctx context.Context, id string) (*file_model.FileFolder, error)
	GetFolderByIDAndTenant(ctx context.Context, id, tenantID string) (*file_model.FileFolder, error)
	UpdateFolder(ctx context.Context, id string, updates map[string]interface{}) (*file_model.FileFolder, error)
	DeleteFolder(ctx context.Context, id string) error
	ListFolders(ctx context.Context, tenantID string, parentID *string, page, limit int, keyword, sort string, workspaceIDs []string) ([]*file_model.FileFolder, int64, error)
	ListFoldersWithPermissionFilter(ctx context.Context, tenantID, accountID string, parentID *string, page, limit int, keyword, sort, workspaceID string, workspaceIDs []string) ([]*file_model.FileFolder, int64, error)
	GetFolderFileCount(ctx context.Context, folderID string) (int64, error)

	// File-folder association operations
	AddFileToFolder(ctx context.Context, fileID, folderID, createdBy string) error
	RemoveFileFromFolder(ctx context.Context, fileID, folderID string) error
	ListFilesInFolder(ctx context.Context, folderID string, page, limit int) ([]*file_model.UploadFile, int64, error)
	ListFilesInFolderWithFilters(ctx context.Context, folderID string, page, limit int, keyword, sort, extension string, startTime, endTime *time.Time) ([]*file_model.UploadFile, int64, error)
	ListFavoriteFileIDs(ctx context.Context, accountID string, page, limit int) ([]string, int64, error)
	ListFilesByIDs(ctx context.Context, fileIDs []string, keyword, sort, extension string, startTime, endTime *time.Time, tenantID string) ([]*file_model.UploadFile, error)
	ListFilesInFolderWithFiltersAndTenant(ctx context.Context, folderID string, page, limit int, keyword, sort, extension string, startTime, endTime *time.Time, tenantID string, workspaceIDs []string) ([]*file_model.UploadFile, int64, error)
	ListAllFilesWithFiltersAndTenant(ctx context.Context, page, limit int, keyword, sort, extension, processingStatus string, startTime, endTime *time.Time, tenantID, accountID string, allowAllFolders bool, workspaceIDs []string) ([]*file_model.UploadFile, int64, error)
	ListFavoriteFilesWithFilters(ctx context.Context, accountID string, page, limit int, keyword, sort, extension string, startTime, endTime *time.Time, tenantID string, allowAllFolders bool, workspaceIDs []string) ([]*file_model.UploadFile, int64, error)
	MoveFileToFolder(ctx context.Context, fileID, fromFolderID, toFolderID, updatedBy string) error
	GetFileFolderID(ctx context.Context, fileID string) (string, error)

	// File statistics operations
	GetTotalFileCount(ctx context.Context, tenantID string) (int64, error)
	// GetRecentFileCount gets the count of recent files (within last 3 months) for a tenant
	// Note: This method counts all recent files without applying any limit.
	// For actual file listing with limit, use ListAllFilesWithFiltersAndTenant method with appropriate parameters.
	GetRecentFileCount(ctx context.Context, tenantID string) (int64, error)
	GetFavoriteFileCount(ctx context.Context, accountID, tenantID string) (int64, error)
	GetRootFolderFileCount(ctx context.Context, tenantID string) (int64, error)
	GetArchivedFileCount(ctx context.Context, tenantID string) (int64, error)

	// Permission operations
	GetFolderPermissionTenants(ctx context.Context, folderID string) ([]string, error)
	GetFolderPermissionTenantsWithDetails(ctx context.Context, folderID string) ([]*workspace_model.Workspace, error)
	AddFolderPermission(ctx context.Context, permission *file_model.FileFolderPermission) error
	DeleteFolderPermissionsByFolderID(ctx context.Context, folderID string) error
}

type fileFolderRepository struct {
	db *gorm.DB
}

func applyWorkspaceIDsFilter(query *gorm.DB, workspaceIDs []string, column string) *gorm.DB {
	if len(workspaceIDs) == 0 {
		return query
	}

	filtered := slices.Compact(workspaceIDs)
	if len(filtered) == 0 {
		return query
	}

	if query != nil && query.Dialector != nil && query.Dialector.Name() == "postgres" {
		return query.Where(column+"::text = ANY(string_to_array(?, ','))", strings.Join(filtered, ","))
	}

	return query.Where(column+" IN ?", filtered)
}

func parseProcessingStatusFilter(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	statuses := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		status := strings.TrimSpace(part)
		if status == "" {
			continue
		}
		if _, exists := seen[status]; exists {
			continue
		}
		seen[status] = struct{}{}
		statuses = append(statuses, status)
	}
	return statuses
}

func applyCurrentAssetProductStatusFilter(query *gorm.DB, statuses []string) *gorm.DB {
	if len(statuses) == 0 {
		return query
	}

	if query != nil && query.Dialector != nil && query.Dialector.Name() == "postgres" {
		return query.Where(`
			EXISTS (
				SELECT 1
				FROM data_library_document_assets AS dla
				WHERE dla.organization_id = upload_files.organization_id::text
					AND dla.source_file_id = upload_files.id::text
					AND dla.deleted_at IS NULL
					AND dla.product_status IN ?
					AND dla.updated_at = (
						SELECT MAX(latest_dla.updated_at)
						FROM data_library_document_assets AS latest_dla
						WHERE latest_dla.organization_id = upload_files.organization_id::text
							AND latest_dla.source_file_id = upload_files.id::text
							AND latest_dla.deleted_at IS NULL
					)
			)
		`, statuses)
	}

	return query.Where(`
		EXISTS (
			SELECT 1
			FROM data_library_document_assets AS dla
			WHERE dla.organization_id = upload_files.organization_id
				AND dla.source_file_id = upload_files.id
				AND dla.deleted_at IS NULL
				AND dla.product_status IN ?
				AND dla.updated_at = (
					SELECT MAX(latest_dla.updated_at)
					FROM data_library_document_assets AS latest_dla
					WHERE latest_dla.organization_id = upload_files.organization_id
						AND latest_dla.source_file_id = upload_files.id
						AND latest_dla.deleted_at IS NULL
				)
		)
	`, statuses)
}

func NewFileFolderRepository(db *gorm.DB) FileFolderRepository {
	return &fileFolderRepository{
		db: db,
	}
}

// CreateFolder creates a new folder
func (r *fileFolderRepository) CreateFolder(ctx context.Context, folder *file_model.FileFolder) error {
	return r.db.WithContext(ctx).Create(folder).Error
}

// GetFolderByID gets a folder by its ID
func (r *fileFolderRepository) GetFolderByID(ctx context.Context, id string) (*file_model.FileFolder, error) {
	var folder file_model.FileFolder
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&folder).Error
	if err != nil {
		return nil, err
	}
	return &folder, nil
}

// GetFolderByIDAndTenant gets a folder by its ID and tenant ID
func (r *fileFolderRepository) GetFolderByIDAndTenant(ctx context.Context, id, organizationID string) (*file_model.FileFolder, error) {
	var folder file_model.FileFolder
	err := r.db.WithContext(ctx).Where("id = ? AND organization_id = ?", id, organizationID).First(&folder).Error
	if err != nil {
		return nil, err
	}
	return &folder, nil
}

// UpdateFolder updates a folder with the given updates
func (r *fileFolderRepository) UpdateFolder(ctx context.Context, id string, updates map[string]interface{}) (*file_model.FileFolder, error) {
	var folder file_model.FileFolder
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&folder).Error
	if err != nil {
		return nil, err
	}

	// Update the folder
	err = r.db.WithContext(ctx).Model(&folder).Updates(updates).Error
	if err != nil {
		return nil, err
	}

	// Refresh the folder data
	err = r.db.WithContext(ctx).Where("id = ?", id).First(&folder).Error
	if err != nil {
		return nil, err
	}

	return &folder, nil
}

// DeleteFolder deletes a folder
func (r *fileFolderRepository) DeleteFolder(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&file_model.FileFolder{}).Error
}

// ListFolders lists folders with optional parent filter
func (r *fileFolderRepository) ListFolders(ctx context.Context, organizationID string, parentID *string, page, limit int, keyword, sort string, workspaceIDs []string) ([]*file_model.FileFolder, int64, error) {
	var folders []*file_model.FileFolder
	var total int64

	offset := (page - 1) * limit
	query := r.db.WithContext(ctx).Model(&file_model.FileFolder{}).Where("organization_id = ?", organizationID)

	// Apply parent filter
	if parentID == nil {
		// Get top-level folders (no parent)
		query = query.Where("parent_id IS NULL")
	} else {
		// Get folders under a specific parent
		query = query.Where("parent_id = ?", *parentID)
	}

	// Add keyword filter
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}

	// Get total count
	query = applyWorkspaceIDsFilter(query, workspaceIDs, "workspace_id")

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
		default:
			order = "created_at DESC"
		}
	}

	// Get paginated results
	if err := query.Offset(offset).Limit(limit).Order(order).Find(&folders).Error; err != nil {
		return nil, 0, err
	}

	return folders, total, nil
}

// ListFoldersWithPermissionFilter lists folders with permission filtering at database level
func (r *fileFolderRepository) ListFoldersWithPermissionFilter(ctx context.Context, organizationID, accountID string, parentID *string, page, limit int, keyword, sort, workspaceID string, workspaceIDs []string) ([]*file_model.FileFolder, int64, error) {
	var folders []*file_model.FileFolder
	var total int64

	offset := (page - 1) * limit

	// Build the main query
	query := r.db.WithContext(ctx).Model(&file_model.FileFolder{}).Where("organization_id = ?", organizationID)

	// Apply parent filter
	if parentID == nil {
		// Get top-level folders (no parent)
		query = query.Where("parent_id IS NULL")
	} else {
		// Get folders under a specific parent
		query = query.Where("parent_id = ?", *parentID)
	}

	// Add keyword filter
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}

	query = applyWorkspaceIDsFilter(query, workspaceIDs, "workspace_id")

	// First, get all tenant IDs associated with the group
	tenantIDsSubQuery := r.db.WithContext(ctx).Table("workspaces").
		Where("organization_id = ?", organizationID).
		Select("id")

	// Check if there are any tenants associated with the group
	var tenantCount int64
	r.db.WithContext(ctx).Table("workspaces").
		Where("organization_id = ?", organizationID).
		Count(&tenantCount)

	// Build the full query with permission checks
	if tenantCount > 0 {
		// If there are associated tenants, use them in the partial_team check
		// For partial_team: folder must be in file_folder_permissions AND user must be member of tenant in workspaces (via organization_id)
		query = query.Where(
			"(permission = ? OR "+
				"(permission = ? AND created_by = ?) OR "+
				"(permission = ? AND EXISTS(SELECT 1 FROM file_folder_permissions ffp WHERE ffp.folder_id = file_folders.id AND ffp.workspace_id IN (SELECT taj.workspace_id FROM workspace_members taj WHERE taj.account_id = ? AND taj.workspace_id IN (?)))))",
			"all_team",
			"only_me", accountID,
			"partial_team", accountID, tenantIDsSubQuery)
	} else {
		// If there are no associated tenants, treat the tenantID as a regular tenant
		// For partial_team: folder must be in file_folder_permissions AND user must be member of tenant
		query = query.Where(
			"(permission = ? OR "+
				"(permission = ? AND created_by = ?) OR "+
				"(permission = ? AND EXISTS(SELECT 1 FROM file_folder_permissions ffp WHERE ffp.folder_id = file_folders.id AND ffp.workspace_id = ? AND EXISTS(SELECT 1 FROM workspace_members taj WHERE taj.account_id = ? AND taj.workspace_id = ?))))",
			"all_team",
			"only_me", accountID,
			"partial_team", organizationID, accountID, organizationID)
	}

	// Get total count
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
		default:
			order = "created_at DESC"
		}
	}

	// Get paginated results
	if err := query.Offset(offset).Limit(limit).Order(order).Find(&folders).Error; err != nil {
		return nil, 0, err
	}

	return folders, total, nil
}

// GetFolderFileCount gets the count of files in a folder
func (r *fileFolderRepository) GetFolderFileCount(ctx context.Context, folderID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&file_model.FileFolderJoins{}).
		Where("folder_id = ?", folderID).
		Count(&count).Error
	return count, err
}

// AddFileToFolder adds a file to a folder
func (r *fileFolderRepository) AddFileToFolder(ctx context.Context, fileID, folderID, createdBy string) error {
	// Check if the association already exists
	var count int64
	err := r.db.WithContext(ctx).Model(&file_model.FileFolderJoins{}).
		Where("file_id = ? AND folder_id = ?", fileID, folderID).
		Count(&count).Error
	if err != nil {
		return err
	}

	// If the association already exists, return
	if count > 0 {
		return nil
	}

	// Create the association
	join := &file_model.FileFolderJoins{
		FileID:    fileID,
		FolderID:  folderID,
		CreatedBy: createdBy,
	}

	return r.db.WithContext(ctx).Create(join).Error
}

// RemoveFileFromFolder removes a file from a folder
func (r *fileFolderRepository) RemoveFileFromFolder(ctx context.Context, fileID, folderID string) error {
	return r.db.WithContext(ctx).
		Where("file_id = ? AND folder_id = ?", fileID, folderID).
		Delete(&file_model.FileFolderJoins{}).Error
}

// ListFilesInFolder lists files in a folder with pagination
func (r *fileFolderRepository) ListFilesInFolder(ctx context.Context, folderID string, page, limit int) ([]*file_model.UploadFile, int64, error) {
	var files []*file_model.UploadFile
	var total int64

	offset := (page - 1) * limit

	// First get the total count
	countQuery := r.db.WithContext(ctx).
		Model(&file_model.UploadFile{}).
		Joins("JOIN file_folder_joins ON upload_files.id = file_folder_joins.file_id").
		Where("is_archived = false AND file_folder_joins.folder_id = ?", folderID)

	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Then get the paginated results
	query := r.db.WithContext(ctx).
		Select("upload_files.*").
		Joins("JOIN file_folder_joins ON upload_files.id = file_folder_joins.file_id").
		Where("is_archived = false AND file_folder_joins.folder_id = ?", folderID).
		Offset(offset).
		Limit(limit).
		Order("upload_files.created_at DESC")

	if err := query.Find(&files).Error; err != nil {
		return nil, 0, err
	}

	return files, total, nil
}

// ListFilesInFolderWithFilters lists files in a folder with additional filters
func (r *fileFolderRepository) ListFilesInFolderWithFilters(ctx context.Context, folderID string, page, limit int, keyword, sort, extension string, startTime, endTime *time.Time) ([]*file_model.UploadFile, int64, error) {
	var files []*file_model.UploadFile
	var total int64

	offset := (page - 1) * limit

	// Build query
	var query *gorm.DB
	var countQuery *gorm.DB

	// Handle root folder (empty folderID) vs specific folder
	if folderID == "" {
		// Root folder - files not in any folder
		query = r.db.WithContext(ctx).Model(&file_model.UploadFile{}).
			Where("is_archived = false AND id NOT IN (SELECT DISTINCT file_id FROM file_folder_joins WHERE file_id IS NOT NULL)")
		countQuery = r.db.WithContext(ctx).Model(&file_model.UploadFile{}).
			Where("is_archived = false AND id NOT IN (SELECT DISTINCT file_id FROM file_folder_joins WHERE file_id IS NOT NULL)")
	} else {
		// Specific folder
		query = r.db.WithContext(ctx).Model(&file_model.UploadFile{}).
			Joins("JOIN file_folder_joins ON upload_files.id = file_folder_joins.file_id").
			Where("is_archived = false AND file_folder_joins.folder_id = ?", folderID)
		countQuery = r.db.WithContext(ctx).Model(&file_model.UploadFile{}).
			Joins("JOIN file_folder_joins ON upload_files.id = file_folder_joins.file_id").
			Where("is_archived = false AND file_folder_joins.folder_id = ?", folderID)
	}

	// Add keyword filter if provided
	if keyword != "" {
		likeKeyword := "%" + keyword + "%"
		query = query.Where("name LIKE ?", likeKeyword)
		countQuery = countQuery.Where("name LIKE ?", likeKeyword)
	}

	// Add extension filter if provided
	if extension != "" {
		// Handle comma-separated extensions
		if strings.Contains(extension, ",") {
			extensions := strings.Split(extension, ",")
			var cleanedExtensions []string
			for _, ext := range extensions {
				ext = strings.TrimSpace(ext)
				ext = strings.TrimPrefix(ext, ".")
				if ext != "" {
					cleanedExtensions = append(cleanedExtensions, ext)
				}
			}
			if len(cleanedExtensions) > 0 {
				query = query.Where("extension IN ?", cleanedExtensions)
				countQuery = countQuery.Where("extension IN ?", cleanedExtensions)
			}
		} else {
			// Single extension
			extension = strings.TrimPrefix(strings.TrimSpace(extension), ".")
			if extension != "" {
				query = query.Where("extension = ?", extension)
				countQuery = countQuery.Where("extension = ?", extension)
			}
		}
	}

	// Add time range filter if provided
	if startTime != nil && !startTime.IsZero() {
		query = query.Where("created_at >= ?", startTime)
		countQuery = countQuery.Where("created_at >= ?", startTime)
	}
	if endTime != nil && !endTime.IsZero() {
		query = query.Where("created_at <= ?", endTime)
		countQuery = countQuery.Where("created_at <= ?", endTime)
	}

	// Get total count
	if err := countQuery.Count(&total).Error; err != nil {
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

	// Get paginated results
	if err := query.Offset(offset).Limit(limit).Order(order).Find(&files).Error; err != nil {
		return nil, 0, err
	}

	return files, total, nil
}

// ListFilesInFolderWithFiltersAndTenant lists files in a folder with additional filters and tenant check
func (r *fileFolderRepository) ListFilesInFolderWithFiltersAndTenant(ctx context.Context, folderID string, page, limit int, keyword, sort, extension string, startTime, endTime *time.Time, tenantID string, workspaceIDs []string) ([]*file_model.UploadFile, int64, error) {
	var files []*file_model.UploadFile
	var total int64

	offset := (page - 1) * limit

	// Build query
	var query *gorm.DB
	var countQuery *gorm.DB

	// Handle root folder (empty folderID) vs specific folder
	if folderID == "" {
		// Root folder - files not in any folder
		query = r.db.WithContext(ctx).Model(&file_model.UploadFile{}).
			Where("organization_id = ? AND is_archived = false AND id NOT IN (SELECT DISTINCT file_id FROM file_folder_joins WHERE file_id IS NOT NULL)", tenantID)
		countQuery = r.db.WithContext(ctx).Model(&file_model.UploadFile{}).
			Where("organization_id = ? AND is_archived = false AND id NOT IN (SELECT DISTINCT file_id FROM file_folder_joins WHERE file_id IS NOT NULL)", tenantID)
	} else {
		// Specific folder
		query = r.db.WithContext(ctx).Model(&file_model.UploadFile{}).
			Joins("JOIN file_folder_joins ON upload_files.id = file_folder_joins.file_id").
			Where("organization_id = ? AND is_archived = false AND file_folder_joins.folder_id = ?", tenantID, folderID)
		countQuery = r.db.WithContext(ctx).Model(&file_model.UploadFile{}).
			Joins("JOIN file_folder_joins ON upload_files.id = file_folder_joins.file_id").
			Where("organization_id = ? AND is_archived = false AND file_folder_joins.folder_id = ?", tenantID, folderID)
	}

	query = applyWorkspaceIDsFilter(query, workspaceIDs, "workspace_id")
	countQuery = applyWorkspaceIDsFilter(countQuery, workspaceIDs, "workspace_id")

	// Add keyword filter if provided
	if keyword != "" {
		likeKeyword := "%" + keyword + "%"
		query = query.Where("name LIKE ?", likeKeyword)
		countQuery = countQuery.Where("name LIKE ?", likeKeyword)
	}

	// Add extension filter if provided
	if extension != "" {
		// Handle comma-separated extensions
		if strings.Contains(extension, ",") {
			extensions := strings.Split(extension, ",")
			var cleanedExtensions []string
			for _, ext := range extensions {
				ext = strings.TrimSpace(ext)
				ext = strings.TrimPrefix(ext, ".")
				if ext != "" {
					cleanedExtensions = append(cleanedExtensions, ext)
				}
			}
			if len(cleanedExtensions) > 0 {
				query = query.Where("extension IN ?", cleanedExtensions)
				countQuery = countQuery.Where("extension IN ?", cleanedExtensions)
			}
		} else {
			// Single extension
			extension = strings.TrimPrefix(strings.TrimSpace(extension), ".")
			if extension != "" {
				query = query.Where("extension = ?", extension)
				countQuery = countQuery.Where("extension = ?", extension)
			}
		}
	}

	// Add time range filter if provided
	if startTime != nil && !startTime.IsZero() {
		query = query.Where("created_at >= ?", startTime)
		countQuery = countQuery.Where("created_at >= ?", startTime)
	}
	if endTime != nil && !endTime.IsZero() {
		query = query.Where("created_at <= ?", endTime)
		countQuery = countQuery.Where("created_at <= ?", endTime)
	}

	// Get total count
	if err := countQuery.Count(&total).Error; err != nil {
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

	// Get paginated results
	if err := query.Offset(offset).Limit(limit).Order(order).Find(&files).Error; err != nil {
		return nil, 0, err
	}

	return files, total, nil
}

// ListAllFilesWithFiltersAndTenant lists all files with additional filters and tenant check
func (r *fileFolderRepository) ListAllFilesWithFiltersAndTenant(ctx context.Context, page, limit int, keyword, sort, extension, processingStatus string, startTime, endTime *time.Time, tenantID, accountID string, allowAllFolders bool, workspaceIDs []string) ([]*file_model.UploadFile, int64, error) {
	var files []*file_model.UploadFile
	var total int64

	offset := (page - 1) * limit

	// Build query for all files (regardless of folder association)
	query := r.db.WithContext(ctx).Model(&file_model.UploadFile{}).
		Where("organization_id = ? AND is_archived = false", tenantID)
	countQuery := r.db.WithContext(ctx).Model(&file_model.UploadFile{}).
		Where("organization_id = ? AND is_archived = false", tenantID)

	query = applyWorkspaceIDsFilter(query, workspaceIDs, "workspace_id")
	countQuery = applyWorkspaceIDsFilter(countQuery, workspaceIDs, "workspace_id")
	query = applyVisibleFileAccessFilter(query, workspaceIDs, accountID, allowAllFolders)
	countQuery = applyVisibleFileAccessFilter(countQuery, workspaceIDs, accountID, allowAllFolders)

	// Add keyword filter if provided
	if keyword != "" {
		likeKeyword := "%" + keyword + "%"
		query = query.Where("name LIKE ?", likeKeyword)
		countQuery = countQuery.Where("name LIKE ?", likeKeyword)
	}

	// Add extension filter if provided
	if extension != "" {
		// Handle comma-separated extensions
		if strings.Contains(extension, ",") {
			extensions := strings.Split(extension, ",")
			var cleanedExtensions []string
			for _, ext := range extensions {
				ext = strings.TrimSpace(ext)
				ext = strings.TrimPrefix(ext, ".")
				if ext != "" {
					cleanedExtensions = append(cleanedExtensions, ext)
				}
			}
			if len(cleanedExtensions) > 0 {
				query = query.Where("extension IN ?", cleanedExtensions)
				countQuery = countQuery.Where("extension IN ?", cleanedExtensions)
			}
		} else {
			// Single extension
			extension = strings.TrimPrefix(strings.TrimSpace(extension), ".")
			if extension != "" {
				query = query.Where("extension = ?", extension)
				countQuery = countQuery.Where("extension = ?", extension)
			}
		}
	}

	// Add time range filter if provided
	if startTime != nil && !startTime.IsZero() {
		query = query.Where("created_at >= ?", startTime)
		countQuery = countQuery.Where("created_at >= ?", startTime)
	}
	if endTime != nil && !endTime.IsZero() {
		query = query.Where("created_at <= ?", endTime)
		countQuery = countQuery.Where("created_at <= ?", endTime)
	}

	if statuses := parseProcessingStatusFilter(processingStatus); len(statuses) > 0 {
		query = applyCurrentAssetProductStatusFilter(query, statuses)
		countQuery = applyCurrentAssetProductStatusFilter(countQuery, statuses)
	}

	// Get total count
	if err := countQuery.Count(&total).Error; err != nil {
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

	// Get paginated results
	if err := query.Offset(offset).Limit(limit).Order(order).Find(&files).Error; err != nil {
		return nil, 0, err
	}

	return files, total, nil
}

// ListFavoriteFileIDs lists favorite file IDs for an account
func (r *fileFolderRepository) ListFavoriteFileIDs(ctx context.Context, accountID string, page, limit int) ([]string, int64, error) {
	var fileIDs []string
	var total int64

	offset := (page - 1) * limit

	// Get total count
	err := r.db.WithContext(ctx).
		Model(&file_model.FileFavorite{}).
		Where("account_id = ?", accountID).
		Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// Get paginated results
	err = r.db.WithContext(ctx).
		Model(&file_model.FileFavorite{}).
		Where("account_id = ?", accountID).
		Offset(offset).
		Limit(limit).
		Order("created_at DESC").
		Pluck("file_id", &fileIDs).Error
	if err != nil {
		return nil, 0, err
	}

	return fileIDs, total, nil
}

// ListFilesByIDs lists files by their IDs with additional filters
func (r *fileFolderRepository) ListFilesByIDs(ctx context.Context, fileIDs []string, keyword, sort, extension string, startTime, endTime *time.Time, tenantID string) ([]*file_model.UploadFile, error) {
	var files []*file_model.UploadFile

	// Build query
	query := r.db.WithContext(ctx).Model(&file_model.UploadFile{}).
		Where("id IN ? AND organization_id = ? AND is_archived = false", fileIDs, tenantID)

	// Add keyword filter if provided
	if keyword != "" {
		likeKeyword := "%" + keyword + "%"
		query = query.Where("name LIKE ?", likeKeyword)
	}

	// Add extension filter if provided
	if extension != "" {
		// Handle comma-separated extensions
		if strings.Contains(extension, ",") {
			extensions := strings.Split(extension, ",")
			var cleanedExtensions []string
			for _, ext := range extensions {
				ext = strings.TrimSpace(ext)
				ext = strings.TrimPrefix(ext, ".")
				if ext != "" {
					cleanedExtensions = append(cleanedExtensions, ext)
				}
			}
			if len(cleanedExtensions) > 0 {
				query = query.Where("extension IN ?", cleanedExtensions)
			}
		} else {
			// Single extension
			extension = strings.TrimPrefix(strings.TrimSpace(extension), ".")
			if extension != "" {
				query = query.Where("extension = ?", extension)
			}
		}
	}

	// Add time range filter if provided
	if startTime != nil && !startTime.IsZero() {
		query = query.Where("created_at >= ?", startTime)
	}
	if endTime != nil && !endTime.IsZero() {
		query = query.Where("created_at <= ?", endTime)
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

	// Get results
	if err := query.Order(order).Find(&files).Error; err != nil {
		return nil, err
	}

	return files, nil
}

// ListFavoriteFilesWithFilters lists favorite files with filters using a single JOIN query (optimized)
func (r *fileFolderRepository) ListFavoriteFilesWithFilters(ctx context.Context, accountID string, page, limit int, keyword, sort, extension string, startTime, endTime *time.Time, tenantID string, allowAllFolders bool, workspaceIDs []string) ([]*file_model.UploadFile, int64, error) {
	var files []*file_model.UploadFile
	var total int64

	offset := (page - 1) * limit

	// Build base query with JOIN
	baseQuery := r.db.WithContext(ctx).
		Model(&file_model.UploadFile{}).
		Joins("INNER JOIN file_favorites ON file_favorites.file_id = upload_files.id").
		Where("file_favorites.account_id = ? AND upload_files.organization_id = ? AND upload_files.is_archived = false", accountID, tenantID)

	baseQuery = applyWorkspaceIDsFilter(baseQuery, workspaceIDs, "upload_files.workspace_id")
	baseQuery = applyVisibleFileAccessFilter(baseQuery, workspaceIDs, accountID, allowAllFolders)

	// Add keyword filter if provided
	if keyword != "" {
		likeKeyword := "%" + keyword + "%"
		baseQuery = baseQuery.Where("upload_files.name LIKE ?", likeKeyword)
	}

	// Add extension filter if provided
	if extension != "" {
		// Handle comma-separated extensions
		if strings.Contains(extension, ",") {
			extensions := strings.Split(extension, ",")
			var cleanedExtensions []string
			for _, ext := range extensions {
				ext = strings.TrimSpace(ext)
				ext = strings.TrimPrefix(ext, ".")
				if ext != "" {
					cleanedExtensions = append(cleanedExtensions, ext)
				}
			}
			if len(cleanedExtensions) > 0 {
				baseQuery = baseQuery.Where("upload_files.extension IN ?", cleanedExtensions)
			}
		} else {
			// Single extension
			extension = strings.TrimPrefix(strings.TrimSpace(extension), ".")
			if extension != "" {
				baseQuery = baseQuery.Where("upload_files.extension = ?", extension)
			}
		}
	}

	// Add time range filter if provided
	if startTime != nil && !startTime.IsZero() {
		baseQuery = baseQuery.Where("upload_files.created_at >= ?", startTime)
	}
	if endTime != nil && !endTime.IsZero() {
		baseQuery = baseQuery.Where("upload_files.created_at <= ?", endTime)
	}

	// Get total count
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Determine sort order
	order := "file_favorites.created_at DESC"
	if sort != "" {
		switch sort {
		case "created_at_asc":
			order = "upload_files.created_at ASC"
		case "created_at_desc":
			order = "upload_files.created_at DESC"
		case "name_asc":
			order = "upload_files.name ASC"
		case "name_desc":
			order = "upload_files.name DESC"
		case "size_asc":
			order = "upload_files.size ASC"
		case "size_desc":
			order = "upload_files.size DESC"
		default:
			order = "file_favorites.created_at DESC"
		}
	}

	// Get paginated results
	if err := baseQuery.
		Order(order).
		Offset(offset).
		Limit(limit).
		Find(&files).Error; err != nil {
		return nil, 0, err
	}

	return files, total, nil
}

// MoveFileToFolder moves a file from one folder to another
func (r *fileFolderRepository) MoveFileToFolder(ctx context.Context, fileID, fromFolderID, toFolderID, updatedBy string) error {
	// Check if trying to move file to the same folder
	if fromFolderID == toFolderID {
		return nil // Nothing to do
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Remove from the old folder
		if fromFolderID != "" {
			result := tx.WithContext(ctx).
				Where("file_id = ? AND folder_id = ?", fileID, fromFolderID).
				Delete(&file_model.FileFolderJoins{})

			if result.Error != nil {
				return result.Error
			}

			// Check if any row was affected
			if result.RowsAffected == 0 {
				// File was not in the fromFolder, but we still continue to add it to the toFolder
			}
		}

		// Add to the new folder
		if toFolderID != "" {
			// Check if file is already in the target folder
			var count int64
			if err := tx.WithContext(ctx).
				Model(&file_model.FileFolderJoins{}).
				Where("file_id = ? AND folder_id = ?", fileID, toFolderID).
				Count(&count).Error; err != nil {
				return err
			}

			// If already in target folder, nothing to do
			if count > 0 {
				return nil
			}

			join := &file_model.FileFolderJoins{
				FileID:    fileID,
				FolderID:  toFolderID,
				CreatedBy: updatedBy,
			}

			if err := tx.WithContext(ctx).Create(join).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// GetFileFolderID gets the folder ID that a file currently belongs to
func (r *fileFolderRepository) GetFileFolderID(ctx context.Context, fileID string) (string, error) {
	// Query the file_folder_joins table to find which folder a file belongs to
	var join file_model.FileFolderJoins
	err := r.db.WithContext(ctx).
		Where("file_id = ?", fileID).
		First(&join).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// File is not in any folder
			return "", nil
		}
		return "", err
	}

	return join.FolderID, nil
}

// GetTotalFileCount gets the total count of files for a tenant
func (r *fileFolderRepository) GetTotalFileCount(ctx context.Context, tenantID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&file_model.UploadFile{}).
		Where("organization_id = ? AND is_archived = false", tenantID).
		Count(&count).Error
	return count, err
}

// GetRecentFileCount gets the count of recent files (within last 3 months) for a tenant
// Note: This method counts all recent files without applying any limit.
// For actual file listing with limit, use ListAllFilesWithFiltersAndTenant method with appropriate parameters.
func (r *fileFolderRepository) GetRecentFileCount(ctx context.Context, tenantID string) (int64, error) {
	var count int64
	threeMonthsAgo := time.Now().AddDate(0, -3, 0)
	err := r.db.WithContext(ctx).
		Model(&file_model.UploadFile{}).
		Where("organization_id = ? AND is_archived = false AND created_at >= ?", tenantID, threeMonthsAgo).
		Count(&count).Error
	return count, err
}

// GetFavoriteFileCount gets the count of favorite files for an account
func (r *fileFolderRepository) GetFavoriteFileCount(ctx context.Context, accountID, tenantID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&file_model.FileFavorite{}).
		Joins("JOIN upload_files ON upload_files.id = file_favorites.file_id").
		Where("file_favorites.account_id = ? AND upload_files.organization_id = ? AND upload_files.is_archived = false", accountID, tenantID).
		Count(&count).Error
	return count, err
}

// GetRootFolderFileCount gets the count of files in the root folder (not in any folder) for a tenant
func (r *fileFolderRepository) GetRootFolderFileCount(ctx context.Context, tenantID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&file_model.UploadFile{}).
		Where("organization_id = ? AND is_archived = false AND id NOT IN (SELECT DISTINCT file_id FROM file_folder_joins WHERE file_id IS NOT NULL)", tenantID).
		Count(&count).Error
	return count, err
}

// GetArchivedFileCount gets the count of archived files for a tenant
func (r *fileFolderRepository) GetArchivedFileCount(ctx context.Context, tenantID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&file_model.UploadFile{}).
		Where("organization_id = ? AND is_archived = true", tenantID).
		Count(&count).Error
	return count, err
}

// GetFolderPermissionTenants gets the list of tenant IDs that have permission to access a folder
func (r *fileFolderRepository) GetFolderPermissionTenants(ctx context.Context, folderID string) ([]string, error) {
	var workspaceIDs []string
	err := r.db.WithContext(ctx).
		Model(&file_model.FileFolderPermission{}).
		Where("folder_id = ?", folderID).
		Pluck("workspace_id", &workspaceIDs).Error
	return workspaceIDs, err
}

// GetFolderPermissionTenantsWithDetails gets the list of tenants with details that have permission to access a folder
func (r *fileFolderRepository) GetFolderPermissionTenantsWithDetails(ctx context.Context, folderID string) ([]*workspace_model.Workspace, error) {
	// Get tenant IDs that have permission to access the folder
	tenantIDs, err := r.GetFolderPermissionTenants(ctx, folderID)
	if err != nil {
		return nil, err
	}

	// Prepare the tenants list
	var tenants []*workspace_model.Workspace

	// Get tenant details for each tenant ID
	for _, tenantID := range tenantIDs {
		var tenant workspace_model.Workspace

		// Direct database query to get tenant details, avoiding cross-repo dependencies
		err := r.db.WithContext(ctx).Table("workspaces").Where("id = ?", tenantID).First(&tenant).Error
		if err != nil {
			// If we can't get the tenant, create a minimal tenant object with ID as name
			tenants = append(tenants, &workspace_model.Workspace{
				ID:   tenantID,
				Name: tenantID, // Use ID as name to maintain consistency
			})
			continue
		}

		// Add complete tenant object
		tenants = append(tenants, &tenant)
	}

	return tenants, nil
}

// AddFolderPermission adds a new folder permission
func (r *fileFolderRepository) AddFolderPermission(ctx context.Context, permission *file_model.FileFolderPermission) error {
	return r.db.WithContext(ctx).Create(permission).Error
}

// DeleteFolderPermissionsByFolderID deletes all folder permissions by folder ID
func (r *fileFolderRepository) DeleteFolderPermissionsByFolderID(ctx context.Context, folderID string) error {
	return r.db.WithContext(ctx).
		Where("folder_id = ?", folderID).
		Delete(&file_model.FileFolderPermission{}).Error
}
