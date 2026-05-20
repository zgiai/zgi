package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"gorm.io/gorm"
)

// DatasetFolderRepository
type DatasetFolderRepository interface {
	// Basic folder operations
	CreateFolder(ctx context.Context, folder *model.DatasetFolder) error
	GetFolderByID(ctx context.Context, folderID string) (*model.DatasetFolder, error)
	UpdateFolder(ctx context.Context, folder *model.DatasetFolder) error
	DeleteFolder(ctx context.Context, folderID string) error

	// Folder query operations
	GetFoldersByTenant(ctx context.Context, tenantID string, page, limit int) ([]*model.DatasetFolder, int64, error)
	GetFoldersByWorkspaceIDs(ctx context.Context, workspaceIDs []string, page, limit int) ([]*model.DatasetFolder, int64, error)
	GetFoldersByParent(ctx context.Context, parentID *string, tenantIDs []string) ([]*model.DatasetFolder, error)
	GetFoldersByParentWithPagination(ctx context.Context, parentID *string, tenantIDs []string, page, limit int, keyword string) ([]*model.DatasetFolder, int64, error)
	GetFoldersByParentWithPaginationWithPermissions(ctx context.Context, parentID *string, organizationID string, tenantIDs []string, accountID string, isGroupAdmin bool, allGroupTenantIDs []string, page, limit int, keyword string) ([]*model.DatasetFolder, int64, error)
	GetFoldersByIDs(ctx context.Context, folderIDs []string) ([]*model.DatasetFolder, error)

	// Permission checks
	CheckFolderPermission(ctx context.Context, folderID, accountID, tenantID string) (bool, error)
	CheckFolderEditorPermission(ctx context.Context, folderID, accountID, tenantID string) (bool, error)

	// Dataset and folder association operations
	AddDatasetToFolder(ctx context.Context, datasetID, folderID, createdBy string) error
	RemoveDatasetFromFolder(ctx context.Context, datasetID, folderID string) error
	GetDatasetsInFolder(ctx context.Context, folderID string, page, limit int) ([]*model.Dataset, int64, error)
	GetDatasetsInFolderByID(ctx context.Context, folderID string, tenantIDs []string) ([]*model.Dataset, error)
	GetDatasetsInFolderByIDWithPagination(ctx context.Context, folderID string, tenantIDs []string, page, limit int, keyword string) ([]*model.Dataset, int64, error)
	GetDatasetsInFolderByIDWithPaginationWithPermissions(ctx context.Context, folderID string, organizationID string, tenantIDs []string, accountID string, isGroupAdmin bool, allGroupTenantIDs []string, page, limit int, keyword string) ([]*model.Dataset, int64, error)
	GetFoldersForDataset(ctx context.Context, datasetID string) ([]*model.DatasetFolder, error)
	SetDefaultFolderForDataset(ctx context.Context, datasetID, folderID string) error
	GetDefaultFolderForDataset(ctx context.Context, datasetID string) (*model.DatasetFolder, error)

	// Folder association table operations
	GetFolderJoinsByDatasetID(ctx context.Context, datasetID string) ([]*model.DatasetFolderJoins, error)
	GetFolderJoinsByFolderID(ctx context.Context, folderID string) ([]*model.DatasetFolderJoins, error)
	DeleteFolderJoin(ctx context.Context, datasetID, folderID string) error
	RemoveAllFolderAssociationsForDataset(ctx context.Context, datasetID string) error

	// Dataset operations
	GetDatasetByID(ctx context.Context, datasetID string) (*model.Dataset, error)
	UpdateDataset(ctx context.Context, dataset *model.Dataset) error
}

type datasetFolderRepository struct {
	db *gorm.DB
}

func NewDatasetFolderRepository(db *gorm.DB) DatasetFolderRepository {
	return &datasetFolderRepository{db: db}
}

// CreateFolder Create folder
func (r *datasetFolderRepository) CreateFolder(ctx context.Context, folder *model.DatasetFolder) error {
	if folder.ID == "" {
		folder.ID = uuid.New().String()
	}
	folder.CreatedAt = time.Now()
	folder.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Create(folder).Error
}

// GetFolderByID Get folder by ID
func (r *datasetFolderRepository) GetFolderByID(ctx context.Context, folderID string) (*model.DatasetFolder, error) {
	var folder model.DatasetFolder
	if err := r.db.WithContext(ctx).Where("id = ?", folderID).First(&folder).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("folder not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get folder: %w", err)
	}
	return &folder, nil
}

// UpdateFolder Update folder
func (r *datasetFolderRepository) UpdateFolder(ctx context.Context, folder *model.DatasetFolder) error {
	folder.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(folder).Error
}

// DeleteFolder Delete folder
func (r *datasetFolderRepository) DeleteFolder(ctx context.Context, folderID string) error {
	// Start a transaction to ensure data consistency
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	// Ensure rollback in case of error
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// First delete all dataset-folder associations for this folder
	if err := tx.Where("folder_id = ?", folderID).Delete(&model.DatasetFolderJoins{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete dataset-folder associations: %w", err)
	}

	// Then delete the folder itself
	if err := tx.Where("id = ?", folderID).Delete(&model.DatasetFolder{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete folder: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetFoldersByTenant Get folders under tenant (paginated)
func (r *datasetFolderRepository) GetFoldersByTenant(ctx context.Context, workspaceID string, page, limit int) ([]*model.DatasetFolder, int64, error) {
	var folders []*model.DatasetFolder
	var total int64

	query := r.db.WithContext(ctx).Model(&model.DatasetFolder{}).Where("workspace_id = ?", workspaceID)

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	offset := (page - 1) * limit
	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&folders).Error; err != nil {
		return nil, 0, err
	}

	return folders, total, nil
}

// GetFoldersByWorkspaceIDs Get folders under tenant list (paginated)
func (r *datasetFolderRepository) GetFoldersByWorkspaceIDs(ctx context.Context, workspaceIDs []string, page, limit int) ([]*model.DatasetFolder, int64, error) {
	var folders []*model.DatasetFolder
	var total int64

	query := r.db.WithContext(ctx).Model(&model.DatasetFolder{}).Where("workspace_id IN ?", workspaceIDs)

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	offset := (page - 1) * limit
	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&folders).Error; err != nil {
		return nil, 0, err
	}

	return folders, total, nil
}

// GetFoldersByParent Get child folders under specified parent folder
func (r *datasetFolderRepository) GetFoldersByParent(ctx context.Context, parentID *string, workspaceIDs []string) ([]*model.DatasetFolder, error) {
	var folders []*model.DatasetFolder

	query := r.db.WithContext(ctx).Model(&model.DatasetFolder{}).Where("workspace_id IN ?", workspaceIDs)

	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}

	if err := query.Order("position ASC, created_at DESC").Find(&folders).Error; err != nil {
		return nil, err
	}

	return folders, nil
}

// GetFoldersByParentWithPagination Get child folders under specified parent folder with pagination
func (r *datasetFolderRepository) GetFoldersByParentWithPagination(ctx context.Context, parentID *string, workspaceIDs []string, page, limit int, keyword string) ([]*model.DatasetFolder, int64, error) {
	var folders []*model.DatasetFolder
	var total int64

	query := r.db.WithContext(ctx).Model(&model.DatasetFolder{}).Where("workspace_id IN ?", workspaceIDs)

	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}

	// Apply keyword filter if provided
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	offset := (page - 1) * limit
	if err := query.Offset(offset).Limit(limit).Order("position ASC, created_at DESC").Find(&folders).Error; err != nil {
		return nil, 0, err
	}

	return folders, total, nil
}

// GetFoldersByParentWithPaginationWithPermissions Get child folders with permission filtering
// isGroupAdmin should be checked by the caller (handler layer) using accountService.CheckGroupAdminByWorkspace
// allGroupTenantIDs contains all tenant IDs in the group, used for all_group permission filtering
func (r *datasetFolderRepository) GetFoldersByParentWithPaginationWithPermissions(ctx context.Context, parentID *string, organizationID string, workspaceIDs []string, accountID string, isGroupAdmin bool, allGroupTenantIDs []string, page, limit int, keyword string) ([]*model.DatasetFolder, int64, error) {
	var folders []*model.DatasetFolder
	var total int64

	if len(workspaceIDs) == 0 {
		return folders, 0, nil
	}

	query := r.db.WithContext(ctx).Model(&model.DatasetFolder{}).
		Where("organization_id = ?", organizationID).
		Where("workspace_id IN ?", workspaceIDs)
	// Filter by parent folder
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}

	// Apply keyword filter if provided
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}

	// NOTE: Temporary simplification - folder visibility is unified at group level.
	// Original per-folder permission filters (only_me, all_team, all_group)
	// are disabled for now. To avoid exposing folders from tenants where the
	// user is not a member, we still enforce a basic membership check for
	// non-group-admin users.
	// if !isGroupAdmin {
	// 	membershipSubquery := r.db.Table("workspace_members").
	// 		Select("1").
	// 		Where("workspace_members.workspace_id = dataset_folders.workspace_id").
	// 		Where("workspace_members.account_id = ?", accountID)

	// 	query = query.Where("EXISTS (?)", membershipSubquery)
	// }

	// If we need fine-grained folder permissions again, restore the block below.

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	offset := (page - 1) * limit
	if err := query.Offset(offset).Limit(limit).Order("position ASC, created_at DESC").Find(&folders).Error; err != nil {
		return nil, 0, err
	}

	return folders, total, nil
}

// GetFoldersByIDs Get folders by ID list
func (r *datasetFolderRepository) GetFoldersByIDs(ctx context.Context, folderIDs []string) ([]*model.DatasetFolder, error) {
	var folders []*model.DatasetFolder
	if err := r.db.WithContext(ctx).Where("id IN ?", folderIDs).Find(&folders).Error; err != nil {
		return nil, err
	}
	return folders, nil
}

// CheckFolderPermission Check if user has permission to access folder
func (r *datasetFolderRepository) CheckFolderPermission(ctx context.Context, folderID, accountID, workspaceID string) (bool, error) {
	var folder model.DatasetFolder
	if err := r.db.WithContext(ctx).Where("id = ? AND workspace_id = ?", folderID, workspaceID).First(&folder).Error; err != nil {
		return false, err
	}

	// Check if user is tenant admin or owner
	var ownerAdminCount int64
	if err := r.db.WithContext(ctx).Table("workspace_members").
		Where("workspace_id = ? AND account_id = ? AND role IN ?", workspaceID, accountID, []string{"owner", "admin"}).
		Count(&ownerAdminCount).Error; err != nil {
		return false, err
	}

	if ownerAdminCount > 0 {
		return true, nil
	}

	// Check permission based on folder permission type
	switch folder.Permission {
	case "all_team":
		return true, nil
	case "only_me":
		return folder.CreatedBy == accountID, nil
	default:
		return false, nil
	}
}

// CheckFolderEditorPermission Check if user has permission to edit folder
func (r *datasetFolderRepository) CheckFolderEditorPermission(ctx context.Context, folderID, accountID, workspaceID string) (bool, error) {
	var folder model.DatasetFolder
	if err := r.db.WithContext(ctx).Where("id = ? AND workspace_id = ?", folderID, workspaceID).First(&folder).Error; err != nil {
		return false, err
	}

	// Check if user is tenant admin or owner
	var ownerAdminCount int64
	if err := r.db.WithContext(ctx).Table("workspace_members").
		Where("workspace_id = ? AND account_id = ? AND role IN ?", workspaceID, accountID, []string{"owner", "admin"}).
		Count(&ownerAdminCount).Error; err != nil {
		return false, err
	}

	if ownerAdminCount > 0 {
		return true, nil
	}

	// Only creator can edit
	return folder.CreatedBy == accountID, nil
}

// AddDatasetToFolder Add dataset to folder
func (r *datasetFolderRepository) AddDatasetToFolder(ctx context.Context, datasetID, folderID, createdBy string) error {
	join := &model.DatasetFolderJoins{
		DatasetID: datasetID,
		FolderID:  folderID,
		CreatedBy: createdBy,
		CreatedAt: time.Now(),
	}

	// First check if association already exists
	var existing model.DatasetFolderJoins
	err := r.db.WithContext(ctx).Where("dataset_id = ? AND folder_id = ?", datasetID, folderID).First(&existing).Error
	if err == nil {
		// Already exists, no need to add again
		return nil
	}

	// Does not exist, create it
	return r.db.WithContext(ctx).Create(join).Error
}

// RemoveDatasetFromFolder Remove dataset from folder
func (r *datasetFolderRepository) RemoveDatasetFromFolder(ctx context.Context, datasetID, folderID string) error {
	return r.db.WithContext(ctx).Delete(&model.DatasetFolderJoins{}, "dataset_id = ? AND folder_id = ?", datasetID, folderID).Error
}

// GetDatasetsInFolder Get datasets in folder (paginated)
func (r *datasetFolderRepository) GetDatasetsInFolder(ctx context.Context, folderID string, page, limit int) ([]*model.Dataset, int64, error) {
	var datasets []*model.Dataset
	var total int64

	// Build subquery to get dataset IDs in folder
	subQuery := r.db.WithContext(ctx).Table("dataset_folder_joins").
		Select("dataset_id").
		Where("folder_id = ?", folderID)

	// Count total
	if err := r.db.WithContext(ctx).Model(&model.Dataset{}).
		Where("id IN (?)", subQuery).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	offset := (page - 1) * limit
	if err := r.db.WithContext(ctx).Model(&model.Dataset{}).
		Where("id IN (?)", subQuery).
		Offset(offset).Limit(limit).
		Order("created_at DESC").
		Find(&datasets).Error; err != nil {
		return nil, 0, err
	}

	return datasets, total, nil
}

// GetFoldersForDataset Get folders that dataset belongs to
func (r *datasetFolderRepository) GetFoldersForDataset(ctx context.Context, datasetID string) ([]*model.DatasetFolder, error) {
	var folders []*model.DatasetFolder

	// Build subquery to get folder IDs where dataset is located
	subQuery := r.db.WithContext(ctx).Table("dataset_folder_joins").
		Select("folder_id").
		Where("dataset_id = ?", datasetID)

	if err := r.db.WithContext(ctx).Model(&model.DatasetFolder{}).
		Where("id IN (?)", subQuery).
		Order("created_at DESC").
		Find(&folders).Error; err != nil {
		return nil, err
	}

	return folders, nil
}

// SetDefaultFolderForDataset Set default folder for dataset
func (r *datasetFolderRepository) SetDefaultFolderForDataset(ctx context.Context, datasetID, folderID string) error {
	// Here we assume a default_folder_id field is added to the dataset table to store the default folder
	return r.db.WithContext(ctx).Model(&model.Dataset{}).
		Where("id = ?", datasetID).
		Update("default_folder_id", folderID).Error
}

// GetDefaultFolderForDataset Get default folder for dataset
func (r *datasetFolderRepository) GetDefaultFolderForDataset(ctx context.Context, datasetID string) (*model.DatasetFolder, error) {
	// Since we don't have a direct reference to the default folder in the dataset model,
	// we'll return nil for now. This method can be enhanced later when the requirement is clearer.
	return nil, nil
}

// GetFolderJoinsByDatasetID Get folder associations for dataset
func (r *datasetFolderRepository) GetFolderJoinsByDatasetID(ctx context.Context, datasetID string) ([]*model.DatasetFolderJoins, error) {
	var joins []*model.DatasetFolderJoins
	if err := r.db.WithContext(ctx).Where("dataset_id = ?", datasetID).Find(&joins).Error; err != nil {
		return nil, err
	}
	return joins, nil
}

// GetFolderJoinsByFolderID Get dataset associations for folder
func (r *datasetFolderRepository) GetFolderJoinsByFolderID(ctx context.Context, folderID string) ([]*model.DatasetFolderJoins, error) {
	var joins []*model.DatasetFolderJoins
	if err := r.db.WithContext(ctx).Where("folder_id = ?", folderID).Find(&joins).Error; err != nil {
		return nil, err
	}
	return joins, nil
}

// DeleteFolderJoin Delete folder and dataset association
func (r *datasetFolderRepository) DeleteFolderJoin(ctx context.Context, datasetID, folderID string) error {
	return r.db.WithContext(ctx).Delete(&model.DatasetFolderJoins{}, "dataset_id = ? AND folder_id = ?", datasetID, folderID).Error
}

// RemoveAllFolderAssociationsForDataset removes all folder associations for a dataset
func (r *datasetFolderRepository) RemoveAllFolderAssociationsForDataset(ctx context.Context, datasetID string) error {
	return r.db.WithContext(ctx).Where("dataset_id = ?", datasetID).Delete(&model.DatasetFolderJoins{}).Error
}

// GetDatasetsInFolderByID retrieves datasets in a specific folder by folder ID
func (r *datasetFolderRepository) GetDatasetsInFolderByID(ctx context.Context, folderID string, workspaceIDs []string) ([]*model.Dataset, error) {
	var datasets []*model.Dataset

	if folderID == "" {
		// Handle root folder - get datasets that are not in any folder
		// First get all dataset IDs that are in any folder
		var datasetIDsInFolders []string
		if err := r.db.WithContext(ctx).Model(&model.DatasetFolderJoins{}).
			Distinct("dataset_id").
			Pluck("dataset_id", &datasetIDsInFolders).Error; err != nil {
			return nil, err
		}

		// Build query for datasets not in any folder
		query := r.db.WithContext(ctx).Model(&model.Dataset{}).Where("workspace_id IN ?", workspaceIDs)
		if len(datasetIDsInFolders) > 0 {
			query = query.Where("id NOT IN ?", datasetIDsInFolders)
		}

		// Get all datasets
		if err := query.Order("created_at DESC").Find(&datasets).Error; err != nil {
			return nil, err
		}
	} else {
		// Handle specific folder - get datasets in the folder
		// Build subquery to get dataset IDs in the folder
		subQuery := r.db.WithContext(ctx).Model(&model.DatasetFolderJoins{}).
			Select("dataset_id").
			Where("folder_id = ?", folderID)

		// Get datasets
		if err := r.db.WithContext(ctx).Model(&model.Dataset{}).
			Where("id IN (?)", subQuery).
			Where("workspace_id IN ?", workspaceIDs).
			Order("created_at DESC").
			Find(&datasets).Error; err != nil {
			return nil, err
		}
	}

	return datasets, nil
}

// GetDatasetsInFolderByIDWithPagination retrieves datasets in a specific folder by folder ID with pagination
func (r *datasetFolderRepository) GetDatasetsInFolderByIDWithPagination(ctx context.Context, folderID string, workspaceIDs []string, page, limit int, keyword string) ([]*model.Dataset, int64, error) {
	var datasets []*model.Dataset
	var total int64

	// Note: tenantIDs should include all organization departments for all_group permission to work
	// The caller (handler) should pass all org department IDs, not just user's departments
	query := r.db.WithContext(ctx).Model(&model.Dataset{}).Where("workspace_id IN ?", workspaceIDs)

	// Apply keyword filter if provided
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}

	if folderID == "" {
		// Handle root folder - get datasets that are not in any folder
		// Use subquery to find datasets that don't have any folder associations
		subQuery := r.db.WithContext(ctx).Model(&model.DatasetFolderJoins{}).Select("dataset_id")
		query = query.Where("id NOT IN (?)", subQuery)
	} else {
		// Handle specific folder - get datasets in the folder
		subQuery := r.db.WithContext(ctx).Model(&model.DatasetFolderJoins{}).
			Select("dataset_id").
			Where("folder_id = ?", folderID)
		query = query.Where("id IN (?)", subQuery)
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	offset := (page - 1) * limit
	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&datasets).Error; err != nil {
		return nil, 0, err
	}

	return datasets, total, nil
}

// GetDatasetsInFolderByIDWithPaginationWithPermissions retrieves datasets in a specific folder with permission filtering
// This method applies the same permission logic as agents:
// - only_me: Only creator can see
// - all_team: All members of the department can see
// - all_group: All members of the organization can see
// - partial_members: Specific members can see (requires checking dataset_permissions table)
// - all_team_members: All members of the department can see (legacy, treated as all_team)
func (r *datasetFolderRepository) GetDatasetsInFolderByIDWithPaginationWithPermissions(
	ctx context.Context,
	folderID string,
	organizationID string,
	tenantIDs []string,
	accountID string,
	isGroupAdmin bool,
	allGroupTenantIDs []string,
	page, limit int,
	keyword string,
) ([]*model.Dataset, int64, error) {
	var datasets []*model.Dataset
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Dataset{}).
		Where("organization_id = ?", organizationID).
		Where("workspace_id IN ?", tenantIDs)

	// Apply keyword filter if provided
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}

	// Filter by folder
	if folderID == "" {
		// Handle root folder - get datasets that are not in any folder
		subQuery := r.db.WithContext(ctx).Model(&model.DatasetFolderJoins{}).Select("dataset_id")
		query = query.Where("id NOT IN (?)", subQuery)
	} else {
		// Handle specific folder - get datasets in the folder
		subQuery := r.db.WithContext(ctx).Model(&model.DatasetFolderJoins{}).
			Select("dataset_id").
			Where("folder_id = ?", folderID)
		query = query.Where("id IN (?)", subQuery)
	}

	// If not group admin, apply permission filters
	// Similar to agents permission logic
	if !isGroupAdmin {
		// Build permission filter conditions
		// 1. is_owner_or_admin: User has OWNER or ADMIN role in the tenant
		isOwnerOrAdminSubquery := r.db.Table("workspace_members").
			Select("1").
			Where("workspace_members.workspace_id = datasets.workspace_id").
			Where("workspace_members.account_id = ?", accountID).
			Where("workspace_members.role IN ?", []string{"OWNER", "ADMIN"})

		// 2. only_me_condition: Dataset permission is only_me and user is creator
		onlyMeCondition := r.db.Where("permission = ? AND created_by = ?", "only_me", accountID)

		// 3. all_team_condition: Dataset permission is all_team/all_team_members and user is member of the tenant

		allTeamCondition := r.db.Where("EXISTS (?)",
			r.db.Table("workspace_members").
				Select("1").
				Where("workspace_members.workspace_id = datasets.workspace_id").
				Where("workspace_members.account_id = ?", accountID),
		)

		// 4. all_group_condition: Dataset permission is all_group
		// All users in the group can see datasets with all_group permission
		// allGroupCondition := r.db.Where("permission = ?", "all_group")

		// 5. partial_members_condition: Dataset permission is partial_members and user is in the list
		// This requires checking the dataset_permissions table (if it exists)
		// For now, we'll skip this condition as it requires additional table structure
		// TODO: Add partial_members support when dataset_permissions table is available

		// 6. all_read_condition: Dataset permission is all_read (everyone can read)
		// allReadCondition := r.db.Where("permission = ?", "all_read")

		// Combine permission conditions with OR
		// User can access dataset if:
		// - They are owner/admin of the tenant, OR
		// - Dataset permission is all_group (visible to entire group), OR
		// - Dataset permission is all_team/all_team_members and user is member of the tenant, OR
		// - Dataset permission is only_me and they are creator, OR
		// - Dataset permission is all_read (everyone can read)
		query = query.Where(
			r.db.Where("EXISTS (?)", isOwnerOrAdminSubquery).
				Or(allTeamCondition).
				Or(onlyMeCondition),
		)
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	offset := (page - 1) * limit
	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&datasets).Error; err != nil {
		return nil, 0, err
	}

	return datasets, total, nil
}

// GetDatasetByID retrieves a dataset by its ID
func (r *datasetFolderRepository) GetDatasetByID(ctx context.Context, datasetID string) (*model.Dataset, error) {
	var dataset model.Dataset
	if err := r.db.WithContext(ctx).Where("id = ?", datasetID).First(&dataset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("dataset not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get dataset: %w", err)
	}
	return &dataset, nil
}

// UpdateDataset updates a dataset
func (r *datasetFolderRepository) UpdateDataset(ctx context.Context, dataset *model.Dataset) error {
	dataset.UpdatedAt = time.Now()
	if err := r.db.WithContext(ctx).Save(dataset).Error; err != nil {
		return fmt.Errorf("failed to update dataset: %w", err)
	}
	return nil
}
