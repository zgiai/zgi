package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type DatasetRepository interface {
	Create(ctx context.Context, dataset *model.Dataset) error
	CreateWithTx(ctx context.Context, tx *gorm.DB, dataset *model.Dataset) error
	GetByID(ctx context.Context, id string) (*model.Dataset, error)
	GetByName(ctx context.Context, name string) (*model.Dataset, error)
	Update(ctx context.Context, dataset *model.Dataset) error
	Delete(ctx context.Context, id string) error

	GetByTenantID(ctx context.Context, tenantID string) ([]*model.Dataset, error)
	GetByTenantIDs1(ctx context.Context, tenantIDs []string, offset, limit int) ([]*model.Dataset, error)
	GetPaginatedByTenantIDs(ctx context.Context, tenantIDs []string, page, limit int, search string, sort string) ([]*model.Dataset, int64, error)
	GetPaginatedByTenantIDsWithPermissions(ctx context.Context, tenantIDs []string, accountID string, isGroupAdmin bool, allGroupTenantIDs []string, page, limit int, search string, sort string) ([]*model.Dataset, int64, error)
	GetByIDs(ctx context.Context, ids []string) ([]*model.Dataset, error)
	List(ctx context.Context, offset, limit int) ([]*model.Dataset, error)
	Count(ctx context.Context) (int64, error)
	CountByTenantID(ctx context.Context, tenantID string) (int64, error)

	CheckOwnership(ctx context.Context, datasetID, tenantID string) (bool, error)

	GetExternalKnowledgeBindingByDatasetID(ctx context.Context, datasetID string) (*model.ExternalKnowledgeBinding, error)
	GetExternalKnowledgeApiByID(ctx context.Context, apiID string) (*model.ExternalKnowledgeApi, error)

	WithTx(tx *gorm.DB) DatasetRepository
	GetByTenantIDs(
		ctx context.Context,
		tenantIDs []string,
		page, limit int,
		search string,
		datasetAdmin bool,
		accountID string,
	) ([]*model.Dataset, int64, error)

	CheckDatasetPermission(ctx context.Context, datasetID, accountID, tenantID string) (bool, error)

	GetDatasetWithPermissionCheck(ctx context.Context, datasetID, accountID, tenantID string) (*model.Dataset, error)
}

type datasetRepository struct {
	db *gorm.DB
}

func NewDatasetRepository(db *gorm.DB) DatasetRepository {
	return &datasetRepository{db: db}
}

func (r *datasetRepository) Create(ctx context.Context, dataset *model.Dataset) error {
	if dataset.ID == "" {
		dataset.ID = uuid.New().String()
	}
	dataset.CreatedAt = time.Now()
	dataset.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Create(dataset).Error
}

func (r *datasetRepository) CreateWithTx(ctx context.Context, tx *gorm.DB, dataset *model.Dataset) error {
	if dataset.ID == "" {
		dataset.ID = uuid.New().String()
	}
	dataset.CreatedAt = time.Now()
	dataset.UpdatedAt = time.Now()
	return tx.WithContext(ctx).Create(dataset).Error
}

func (r *datasetRepository) GetByID(ctx context.Context, id string) (*model.Dataset, error) {
	var dataset model.Dataset
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&dataset).Error
	if err != nil {
		return nil, err
	}
	logger.DebugContext(ctx, "dataset loaded by id",
		zap.String("dataset_id", dataset.ID),
		zap.String("tenant_id", dataset.WorkspaceID),
		zap.Bool("enable_graph_flow", dataset.EnableGraphFlow),
	)
	return &dataset, nil
}

func (r *datasetRepository) GetByName(ctx context.Context, name string) (*model.Dataset, error) {
	var dataset model.Dataset
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&dataset).Error
	if err != nil {
		return nil, err
	}
	return &dataset, nil
}

func (r *datasetRepository) Update(ctx context.Context, dataset *model.Dataset) error {
	dataset.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(dataset).Error
}

func (r *datasetRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.Dataset{}, "id = ?", id).Error
}

func (r *datasetRepository) GetByTenantID(ctx context.Context, workspaceID string) ([]*model.Dataset, error) {
	var datasets []*model.Dataset
	err := r.db.WithContext(ctx).Where("workspace_id = ?", workspaceID).Find(&datasets).Error
	return datasets, err
}

func (r *datasetRepository) GetByTenantIDs1(ctx context.Context, workspaceIDs []string, offset, limit int) ([]*model.Dataset, error) {
	var datasets []*model.Dataset
	err := r.db.WithContext(ctx).
		Where("workspace_id IN ?", workspaceIDs).
		Offset(offset).Limit(limit).
		Find(&datasets).Error
	return datasets, err
}

func (r *datasetRepository) GetPaginatedByTenantIDs(ctx context.Context, workspaceIDs []string, page, limit int, search string, sort string) ([]*model.Dataset, int64, error) {
	var datasets []*model.Dataset
	var total int64

	// Build base query
	query := r.db.WithContext(ctx).Model(&model.Dataset{}).Where("workspace_id IN ?", workspaceIDs)

	// Add search filter if provided
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("name ILIKE ? OR description ILIKE ?", searchPattern, searchPattern)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results with ordering by created_at DESC (newest first)
	offset := (page - 1) * limit
	err := query.Order("created_at " + sort + ", id DESC").Offset(offset).Limit(limit).Find(&datasets).Error

	return datasets, total, err
}

// GetPaginatedByTenantIDsWithPermissions gets paginated datasets with permission filtering
// isGroupAdmin should be checked by the caller (handler layer) using accountService.CheckGroupAdminByWorkspace
// allGroupTenantIDs contains all tenant IDs in the group, used for all_group permission filtering
func (r *datasetRepository) GetPaginatedByTenantIDsWithPermissions(ctx context.Context, tenantIDs []string, accountID string, isGroupAdmin bool, allGroupTenantIDs []string, page, limit int, search string, sort string) ([]*model.Dataset, int64, error) {
	var datasets []*model.Dataset
	var total int64

	// Build base query with workspace_id filter
	// For all_group permission, we need to include all tenants in the group
	// So we use a union of user's accessible tenants and all group tenants for all_group datasets
	var queryWorkspaceIDs []string
	if len(allGroupTenantIDs) > 0 {
		// Combine user's accessible tenants with all group tenants (for all_group permission)
		tenantIDMap := make(map[string]bool)
		for _, id := range tenantIDs {
			tenantIDMap[id] = true
		}
		for _, id := range allGroupTenantIDs {
			tenantIDMap[id] = true
		}
		queryWorkspaceIDs = make([]string, 0, len(tenantIDMap))
		for id := range tenantIDMap {
			queryWorkspaceIDs = append(queryWorkspaceIDs, id)
		}
	} else {
		queryWorkspaceIDs = tenantIDs
	}

	query := r.db.WithContext(ctx).Model(&model.Dataset{}).Where("workspace_id IN ?", queryWorkspaceIDs)

	// Add search filter if provided
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("name ILIKE ? OR description ILIKE ?", searchPattern, searchPattern)
	}

	// NOTE: Temporary simplification - dataset visibility is unified at group level.
	// Original per-dataset permission filters (only_me, all_team, all_group)
	// are disabled for now to make datasets visible based on tenantIDs/allGroupTenantIDs.
	// To avoid exposing datasets from tenants where the user is not a member,
	// we still enforce a basic membership check for non-group-admin users.
	if !isGroupAdmin {
		membershipSubquery := r.db.Table("workspace_members").
			Select("1").
			Where("workspace_members.workspace_id = datasets.workspace_id").
			Where("workspace_members.account_id = ?", accountID)

		query = query.Where("EXISTS (?)", membershipSubquery)
	}

	// If we need fine-grained permissions again, restore the block below.

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results with ordering by created_at DESC (newest first)
	offset := (page - 1) * limit
	err := query.Order("created_at " + sort + ", id DESC").Offset(offset).Limit(limit).Find(&datasets).Error

	return datasets, total, err
}

func (r *datasetRepository) GetByIDs(ctx context.Context, ids []string) ([]*model.Dataset, error) {
	var datasets []*model.Dataset
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&datasets).Error
	return datasets, err
}

func (r *datasetRepository) List(ctx context.Context, offset, limit int) ([]*model.Dataset, error) {
	var datasets []*model.Dataset
	err := r.db.WithContext(ctx).Offset(offset).Limit(limit).Find(&datasets).Error
	return datasets, err
}

func (r *datasetRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Dataset{}).Count(&count).Error
	return count, err
}

func (r *datasetRepository) CountByTenantID(ctx context.Context, workspaceID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Dataset{}).Where("workspace_id = ?", workspaceID).Count(&count).Error
	return count, err
}

func (r *datasetRepository) CheckOwnership(ctx context.Context, datasetID, workspaceID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Dataset{}).
		Where("id = ? AND workspace_id = ?", datasetID, workspaceID).
		Count(&count).Error
	return count > 0, err
}

func (r *datasetRepository) WithTx(tx *gorm.DB) DatasetRepository {
	return NewDatasetRepository(tx)
}

func (r *datasetRepository) GetExternalKnowledgeBindingByDatasetID(ctx context.Context, datasetID string) (*model.ExternalKnowledgeBinding, error) {
	var binding model.ExternalKnowledgeBinding
	err := r.db.WithContext(ctx).Where("dataset_id = ?", datasetID).First(&binding).Error
	if err != nil {
		return nil, err
	}
	return &binding, nil
}

func (r *datasetRepository) GetExternalKnowledgeApiByID(ctx context.Context, apiID string) (*model.ExternalKnowledgeApi, error) {
	var api model.ExternalKnowledgeApi
	err := r.db.WithContext(ctx).Where("id = ?", apiID).First(&api).Error
	if err != nil {
		return nil, err
	}
	return &api, nil
}

func (r *datasetRepository) GetByTenantIDs(
	ctx context.Context,
	workspaceIDs []string,
	page, limit int,
	search string,
	datasetAdmin bool,
	accountID string,
) ([]*model.Dataset, int64, error) {
	var datasets []*model.Dataset
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Dataset{}).Where("workspace_id IN ?", workspaceIDs)

	if search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	if !datasetAdmin {

		permissionConditions := []string{
			`EXISTS (
				SELECT 1 FROM workspace_members
				WHERE workspace_members.workspace_id = datasets.workspace_id
				AND workspace_members.account_id = ?
				AND workspace_members.role IN ('owner', 'admin')
				LIMIT 1
			)`,
			"permission = 'all_team'",
			"(permission = 'only_me' AND created_by = ?)",
		}

		whereClause := "(" + strings.Join(permissionConditions, " OR ") + ")"
		query = query.Where(whereClause, accountID, accountID)
	}

	countQuery := query.Session(&gorm.Session{})
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count datasets: %w", err)
	}

	offset := (page - 1) * limit
	if err := query.
		Order("created_at DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Find(&datasets).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get datasets: %w", err)
	}

	return datasets, total, nil
}

func (r *datasetRepository) GetDatasetWithPermissionCheck(ctx context.Context, datasetID, accountID, workspaceID string) (*model.Dataset, error) {
	var dataset model.Dataset

	if err := r.db.WithContext(ctx).Where("id = ? AND workspace_id = ?", datasetID, workspaceID).First(&dataset).Error; err != nil {
		return nil, err
	}
	logger.DebugContext(ctx, "dataset loaded with permission check",
		zap.String("dataset_id", dataset.ID),
		zap.String("tenant_id", workspaceID),
		zap.String("account_id", accountID),
		zap.Bool("enable_graph_flow", dataset.EnableGraphFlow),
	)

	hasPermission, err := r.CheckDatasetPermission(ctx, datasetID, accountID, workspaceID)
	if err != nil {
		return nil, err
	}

	if !hasPermission {
		return nil, errors.New("no permission to access this dataset")
	}

	return &dataset, nil
}

func (r *datasetRepository) CheckDatasetPermission(ctx context.Context, datasetID, accountID, workspaceID string) (bool, error) {
	var dataset model.Dataset
	if err := r.db.WithContext(ctx).Where("id = ?", datasetID).First(&dataset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	var ownerAdminCount int64
	if err := r.db.WithContext(ctx).Table("workspace_members").
		Where("workspace_id = ? AND account_id = ? AND role IN ?", dataset.WorkspaceID, accountID, []string{"owner", "admin"}).
		Count(&ownerAdminCount).Error; err != nil {
		return false, err
	}

	if ownerAdminCount > 0 {
		return true, nil
	}

	// Default permission: only creator has access
	return dataset.CreatedBy == accountID, nil
}
