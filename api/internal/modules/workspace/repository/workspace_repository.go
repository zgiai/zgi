package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
)

// WorkspaceRepository handles workspace-related database operations
type WorkspaceRepository interface {
	GetWorkspaceByID(ctx context.Context, workspaceID string) (*model.Workspace, error)
	GetWorkspaceMember(ctx context.Context, workspaceID, accountID string) (*model.WorkspaceMember, error)
	UpdateWorkspaceCustomConfig(ctx context.Context, workspaceID string, customConfig string) error
	CreateWorkspace(ctx context.Context, name string) (*model.Workspace, error)
	CreateWorkspaceMember(ctx context.Context, workspaceID, accountID, role string, current bool) error
	UpdateWorkspaceName(ctx context.Context, workspaceID, name string) error
	UpdateWorkspaceStatus(ctx context.Context, workspaceID string, status model.WorkspaceStatus) error
	CheckWorkspaceNameExists(ctx context.Context, organizationID, name, excludeWorkspaceID string) (bool, error)
	GetWorkspaceOrganizationID(ctx context.Context, workspaceID string) (string, error)
	GetWorkspaceStatistics(ctx context.Context, workspaceID string) (int, int, int, int, error)
	Create(ctx context.Context, workspace *model.Workspace) error
	CreateWithTx(ctx context.Context, tx *gorm.DB, workspace *model.Workspace) error
	GetByID(ctx context.Context, id string) (*model.Workspace, error)
	GetByName(ctx context.Context, name string) (*model.Workspace, error)
	Update(ctx context.Context, workspace *model.Workspace) error
	Delete(ctx context.Context, id string) error
	GetByStatus(ctx context.Context, status string) ([]*model.Workspace, error)
	GetByIDs(ctx context.Context, ids []string) ([]*model.Workspace, error)
	ExistsByName(ctx context.Context, name string) (bool, error)
	List(ctx context.Context, offset, limit int) ([]*model.Workspace, error)
	Count(ctx context.Context) (int64, error)
	GetAccountWorkspaceJoins(ctx context.Context, accountID string) ([]*model.WorkspaceMember, error)
	GetWorkspaceAccountJoins(ctx context.Context, workspaceID string) ([]*model.WorkspaceMember, error)
	CreateWorkspaceAccountJoin(ctx context.Context, join *model.WorkspaceMember) error
	DeleteWorkspaceAccountJoin(ctx context.Context, workspaceID, accountID string) error
	GetWorkspaceIDsByOrganizationID(ctx context.Context, organizationID string) ([]string, error)
	GetDB() *gorm.DB
	WithTx(tx *gorm.DB) WorkspaceRepository
}

type workspaceRepository struct {
	db *gorm.DB
}

func NewWorkspaceRepository(db *gorm.DB) WorkspaceRepository {
	return &workspaceRepository{
		db: db,
	}
}

// GetWorkspaceByID gets a workspace by ID
func (r *workspaceRepository) GetWorkspaceByID(ctx context.Context, workspaceID string) (*model.Workspace, error) {
	var workspace model.Workspace
	err := r.db.WithContext(ctx).Where("id = ?", workspaceID).First(&workspace).Error
	if err != nil {
		return nil, err
	}
	return &workspace, nil
}

// GetWorkspaceMember gets the workspace member relationship
func (r *workspaceRepository) GetWorkspaceMember(ctx context.Context, workspaceID, accountID string) (*model.WorkspaceMember, error) {
	var join model.WorkspaceMember
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND account_id = ?", workspaceID, accountID).
		First(&join).Error
	if err != nil {
		return nil, err
	}
	return &join, nil
}

// UpdateWorkspaceCustomConfig updates workspace custom configuration
func (r *workspaceRepository) UpdateWorkspaceCustomConfig(ctx context.Context, workspaceID string, customConfig string) error {
	return r.db.WithContext(ctx).
		Model(&model.Workspace{}).
		Where("id = ?", workspaceID).
		Update("custom_config", customConfig).Error
}

// CreateWorkspace creates a new workspace
func (r *workspaceRepository) CreateWorkspace(ctx context.Context, name string) (*model.Workspace, error) {
	workspace := &model.Workspace{
		Name:      name,
		Plan:      "basic",
		Status:    model.WorkspaceStatusNormal,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := r.db.WithContext(ctx).Create(workspace).Error
	if err != nil {
		return nil, err
	}
	return workspace, nil
}

// CreateWorkspaceMember creates a workspace member relationship
func (r *workspaceRepository) CreateWorkspaceMember(ctx context.Context, workspaceID, accountID, role string, current bool) error {
	join := &model.WorkspaceMember{
		WorkspaceID: workspaceID,
		AccountID:   accountID,
		Role:        model.WorkspaceMemberRole(role),
		Current:     current,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	model.ApplyWorkspaceMemberDefaults(join)

	return r.db.WithContext(ctx).Create(join).Error
}

// UpdateWorkspaceName updates a workspace name
func (r *workspaceRepository) UpdateWorkspaceName(ctx context.Context, workspaceID, name string) error {
	return r.db.WithContext(ctx).
		Model(&model.Workspace{}).
		Where("id = ?", workspaceID).
		Update("name", name).Error
}

// UpdateWorkspaceStatus updates a workspace status
func (r *workspaceRepository) UpdateWorkspaceStatus(ctx context.Context, workspaceID string, status model.WorkspaceStatus) error {
	return r.db.WithContext(ctx).
		Model(&model.Workspace{}).
		Where("id = ?", workspaceID).
		Update("status", status).Error
}

// CheckWorkspaceNameExists checks if a workspace name already exists in the same organization.
func (r *workspaceRepository) CheckWorkspaceNameExists(ctx context.Context, organizationID, name, excludeWorkspaceID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("workspaces").
		Where("organization_id = ? AND name = ? AND id != ?", organizationID, name, excludeWorkspaceID).
		Count(&count).Error
	return count > 0, err
}

// GetWorkspaceOrganizationID gets the organization ID for a workspace.
func (r *workspaceRepository) GetWorkspaceOrganizationID(ctx context.Context, workspaceID string) (string, error) {
	var organizationID string
	err := r.db.WithContext(ctx).
		Table("workspaces").
		Select("organization_id").
		Where("id = ?", workspaceID).
		Scan(&organizationID).Error
	return organizationID, err
}

// GetWorkspaceStatistics gets workspace statistics including member counts and entity counts
func (r *workspaceRepository) GetWorkspaceStatistics(ctx context.Context, workspaceID string) (int, int, int, int, error) {
	// Get role counts
	var roleCounts []struct {
		Role  string `json:"role"`
		Count int    `json:"count"`
	}

	err := r.db.WithContext(ctx).
		Table("workspace_members").
		Select("role, COUNT(role) as count").
		Where("workspace_id = ?", workspaceID).
		Group("role").
		Scan(&roleCounts).Error
	if err != nil {
		return 0, 0, 0, 0, err
	}

	adminsCount := 0
	membersCount := 0
	for _, roleCount := range roleCounts {
		if roleCount.Role == "owner" || roleCount.Role == "admin" {
			adminsCount += roleCount.Count
		} else {
			membersCount += roleCount.Count
		}
	}

	// Get datasets count
	var datasetsCount int64
	err = r.db.WithContext(ctx).
		Model(&dataset_model.Dataset{}).
		Where("workspace_id = ?", workspaceID).
		Count(&datasetsCount).Error
	if err != nil {
		return 0, 0, 0, 0, err
	}

	// Get tenant-owned agents count.
	var agentsCount int64
	err = r.db.WithContext(ctx).
		Table("agents").
		Where("tenant_id = ? AND deleted_at IS NULL AND is_universal = ?", workspaceID, false).
		Count(&agentsCount).Error
	if err != nil {
		return 0, 0, 0, 0, err
	}

	return adminsCount, membersCount, int(datasetsCount), int(agentsCount), nil
}

func (r *workspaceRepository) Create(ctx context.Context, workspace *model.Workspace) error {
	if workspace.ID == "" {
		workspace.ID = uuid.New().String()
	}
	workspace.CreatedAt = time.Now()
	workspace.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Create(workspace).Error
}

func (r *workspaceRepository) CreateWithTx(ctx context.Context, tx *gorm.DB, workspace *model.Workspace) error {
	if workspace.ID == "" {
		workspace.ID = uuid.New().String()
	}
	workspace.CreatedAt = time.Now()
	workspace.UpdatedAt = time.Now()
	return tx.WithContext(ctx).Create(workspace).Error
}

func (r *workspaceRepository) GetByID(ctx context.Context, id string) (*model.Workspace, error) {
	var workspace model.Workspace
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&workspace).Error
	if err != nil {
		return nil, err
	}
	return &workspace, nil
}

func (r *workspaceRepository) GetByName(ctx context.Context, name string) (*model.Workspace, error) {
	var workspace model.Workspace
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&workspace).Error
	if err != nil {
		return nil, err
	}
	return &workspace, nil
}

func (r *workspaceRepository) Update(ctx context.Context, workspace *model.Workspace) error {
	workspace.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(workspace).Error
}

func (r *workspaceRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.Workspace{}, "id = ?", id).Error
}

func (r *workspaceRepository) GetByStatus(ctx context.Context, status string) ([]*model.Workspace, error) {
	var workspaces []*model.Workspace
	err := r.db.WithContext(ctx).Where("status = ?", status).Find(&workspaces).Error
	return workspaces, err
}

func (r *workspaceRepository) GetByIDs(ctx context.Context, ids []string) ([]*model.Workspace, error) {
	var workspaces []*model.Workspace
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&workspaces).Error
	return workspaces, err
}

func (r *workspaceRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Workspace{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

func (r *workspaceRepository) List(ctx context.Context, offset, limit int) ([]*model.Workspace, error) {
	var workspaces []*model.Workspace
	err := r.db.WithContext(ctx).Offset(offset).Limit(limit).Find(&workspaces).Error
	return workspaces, err
}

func (r *workspaceRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Workspace{}).Count(&count).Error
	return count, err
}

func (r *workspaceRepository) GetAccountWorkspaceJoins(ctx context.Context, accountID string) ([]*model.WorkspaceMember, error) {
	var joins []*model.WorkspaceMember
	err := r.db.WithContext(ctx).Where("account_id = ?", accountID).Find(&joins).Error
	return joins, err
}

func (r *workspaceRepository) GetWorkspaceAccountJoins(ctx context.Context, workspaceID string) ([]*model.WorkspaceMember, error) {
	var joins []*model.WorkspaceMember
	err := r.db.WithContext(ctx).Where("workspace_id = ?", workspaceID).Find(&joins).Error
	return joins, err
}

func (r *workspaceRepository) CreateWorkspaceAccountJoin(ctx context.Context, join *model.WorkspaceMember) error {
	join.CreatedAt = time.Now()
	return r.db.WithContext(ctx).Create(join).Error
}

func (r *workspaceRepository) DeleteWorkspaceAccountJoin(ctx context.Context, workspaceID, accountID string) error {
	return r.db.WithContext(ctx).
		Delete(&model.WorkspaceMember{}, "workspace_id = ? AND account_id = ?", workspaceID, accountID).Error
}

func (r *workspaceRepository) GetWorkspaceIDsByOrganizationID(ctx context.Context, organizationID string) ([]string, error) {
	var workspaceIDs []string

	err := r.db.WithContext(ctx).
		Table("workspaces").
		Select("id").
		Where("organization_id = ?", organizationID).
		Pluck("id", &workspaceIDs).Error

	if err != nil {
		return nil, err
	}

	return workspaceIDs, nil
}

func (r *workspaceRepository) WithTx(tx *gorm.DB) WorkspaceRepository {
	return NewWorkspaceRepository(tx)
}

func (r *workspaceRepository) GetDB() *gorm.DB {
	return r.db
}
