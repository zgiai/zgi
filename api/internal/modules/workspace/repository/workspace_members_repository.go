package repository

import (
	"context"
	"errors"

	auth_model "github.com/zgiai/ginext/internal/modules/user/auth/model"
	"github.com/zgiai/ginext/internal/modules/workspace/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WorkspaceMemberRepository interface {
	Create(ctx context.Context, join *model.WorkspaceMember) error
	GetByID(ctx context.Context, id string) (*model.WorkspaceMember, error)
	GetByWorkspaceAndMember(ctx context.Context, workspaceID, memberID string) (*model.WorkspaceMember, error)
	Update(ctx context.Context, join *model.WorkspaceMember) error
	Delete(ctx context.Context, id string) error

	UpdateRole(ctx context.Context, workspaceID, memberID string, role model.WorkspaceMemberRole) error
	GetMemberRole(ctx context.Context, workspaceID, memberID string) (model.WorkspaceMemberRole, error)

	SetCurrentWorkspace(ctx context.Context, memberID, workspaceID string) error
	GetCurrentOrganization(ctx context.Context, memberID string) (*model.OrganizationMember, error)
	GetCurrentWorkspace(ctx context.Context, memberID string) (*model.WorkspaceMember, error)
	ClearCurrentWorkspace(ctx context.Context, memberID string) error

	GetJoinsByWorkspaceID(ctx context.Context, workspaceID string) ([]*model.WorkspaceMember, error)
	GetJoinsByMemberID(ctx context.Context, memberID string) ([]*model.WorkspaceMember, error)
	GetOwnerByWorkspaceID(ctx context.Context, workspaceID string) (*model.WorkspaceMember, error)
	GetAdminsByWorkspaceID(ctx context.Context, workspaceID string) ([]*model.WorkspaceMember, error)

	IsMemberInWorkspace(ctx context.Context, memberID, workspaceID string) (bool, error)
	IsMemberOwner(ctx context.Context, memberID, workspaceID string) (bool, error)
	IsMemberAdmin(ctx context.Context, memberID, workspaceID string) (bool, error)
	WithTx(tx *gorm.DB) WorkspaceMemberRepository
}

type workspaceMemberRepository struct {
	db *gorm.DB
}

func NewWorkspaceMemberRepository(db *gorm.DB) WorkspaceMemberRepository {
	return &workspaceMemberRepository{db: db}
}

func (r *workspaceMemberRepository) Create(ctx context.Context, join *model.WorkspaceMember) error {
	if join.ID == "" {
		join.ID = uuid.New().String()
	}
	return r.db.WithContext(ctx).Create(join).Error
}

func (r *workspaceMemberRepository) GetByID(ctx context.Context, id string) (*model.WorkspaceMember, error) {
	var join model.WorkspaceMember
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&join).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &join, nil
}

func (r *workspaceMemberRepository) GetByWorkspaceAndMember(ctx context.Context, workspaceID, memberID string) (*model.WorkspaceMember, error) {
	var join model.WorkspaceMember
	err := r.db.WithContext(ctx).Where("workspace_id = ? AND account_id = ?", workspaceID, memberID).First(&join).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &join, nil
}

func (r *workspaceMemberRepository) Update(ctx context.Context, join *model.WorkspaceMember) error {
	return r.db.WithContext(ctx).Save(join).Error
}

func (r *workspaceMemberRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.WorkspaceMember{}).Error
}

func (r *workspaceMemberRepository) UpdateRole(ctx context.Context, workspaceID, memberID string, role model.WorkspaceMemberRole) error {
	return r.db.WithContext(ctx).Model(&model.WorkspaceMember{}).
		Where("workspace_id = ? AND account_id = ?", workspaceID, memberID).
		Update("role", role).Error
}

func (r *workspaceMemberRepository) GetMemberRole(ctx context.Context, workspaceID, memberID string) (model.WorkspaceMemberRole, error) {
	var join model.WorkspaceMember
	err := r.db.WithContext(ctx).Select("role").Where("workspace_id = ? AND account_id = ?", workspaceID, memberID).First(&join).Error
	if err != nil {
		return "", err
	}
	return join.Role, nil
}

func (r *workspaceMemberRepository) SetCurrentWorkspace(ctx context.Context, memberID, workspaceID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Update AccountContext
		if err := tx.Model(&auth_model.AccountContext{}).Where("account_id = ?", memberID).Update("current_workspace_id", workspaceID).Error; err != nil {
			return err
		}
		// Legacy update for compatibility
		if err := tx.Model(&model.WorkspaceMember{}).Where("account_id = ?", memberID).Update("current", false).Error; err != nil {
			return err
		}
		return tx.Model(&model.WorkspaceMember{}).Where("account_id = ? AND workspace_id = ?", memberID, workspaceID).Update("current", true).Error
	})
}

func (r *workspaceMemberRepository) GetCurrentOrganization(ctx context.Context, memberID string) (*model.OrganizationMember, error) {
	var accountContext auth_model.AccountContext
	// 1. Get CurrentOrganizationID from AccountContext
	if err := r.db.WithContext(ctx).Where("account_id = ?", memberID).First(&accountContext).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if accountContext.CurrentOrganizationID == nil {
		return nil, nil
	}

	// 2. Query OrganizationMember
	var join model.OrganizationMember
	err := r.db.WithContext(ctx).Where("organization_id = ? AND account_id = ?", *accountContext.CurrentOrganizationID, memberID).First(&join).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &join, nil
}

func (r *workspaceMemberRepository) GetCurrentWorkspace(ctx context.Context, memberID string) (*model.WorkspaceMember, error) {
	var accountContext auth_model.AccountContext
	// Get CurrentWorkspaceID from AccountContext
	if err := r.db.WithContext(ctx).Where("account_id = ?", memberID).First(&accountContext).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if accountContext.CurrentWorkspaceID == nil {
		return nil, nil
	}

	var join model.WorkspaceMember
	err := r.db.WithContext(ctx).Where("workspace_id = ? AND account_id = ?", *accountContext.CurrentWorkspaceID, memberID).First(&join).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &join, nil
}

func (r *workspaceMemberRepository) ClearCurrentWorkspace(ctx context.Context, memberID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Model(&auth_model.AccountContext{}).Where("account_id = ?", memberID).Update("current_workspace_id", nil).Error
	})
}

func (r *workspaceMemberRepository) GetJoinsByWorkspaceID(ctx context.Context, workspaceID string) ([]*model.WorkspaceMember, error) {
	var joins []*model.WorkspaceMember
	err := r.db.WithContext(ctx).Where("workspace_id = ?", workspaceID).Find(&joins).Error
	return joins, err
}

func (r *workspaceMemberRepository) GetJoinsByMemberID(ctx context.Context, memberID string) ([]*model.WorkspaceMember, error) {
	var joins []*model.WorkspaceMember
	err := r.db.WithContext(ctx).Where("account_id = ?", memberID).Find(&joins).Error
	return joins, err
}

func (r *workspaceMemberRepository) GetOwnerByWorkspaceID(ctx context.Context, workspaceID string) (*model.WorkspaceMember, error) {
	var join model.WorkspaceMember
	err := r.db.WithContext(ctx).Where("workspace_id = ? AND role = ?", workspaceID, model.WorkspaceRoleOwner).First(&join).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &join, nil
}

func (r *workspaceMemberRepository) GetAdminsByWorkspaceID(ctx context.Context, workspaceID string) ([]*model.WorkspaceMember, error) {
	var joins []*model.WorkspaceMember
	err := r.db.WithContext(ctx).Where("workspace_id = ? AND role IN ?", workspaceID, []model.WorkspaceMemberRole{model.WorkspaceRoleOwner, model.WorkspaceRoleAdmin}).Find(&joins).Error
	return joins, err
}

func (r *workspaceMemberRepository) IsMemberInWorkspace(ctx context.Context, memberID, workspaceID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.WorkspaceMember{}).Where("account_id = ? AND workspace_id = ?", memberID, workspaceID).Count(&count).Error
	return count > 0, err
}

func (r *workspaceMemberRepository) IsMemberOwner(ctx context.Context, memberID, workspaceID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.WorkspaceMember{}).Where("account_id = ? AND workspace_id = ? AND role = ?", memberID, workspaceID, model.WorkspaceRoleOwner).Count(&count).Error
	return count > 0, err
}

func (r *workspaceMemberRepository) IsMemberAdmin(ctx context.Context, memberID, workspaceID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.WorkspaceMember{}).Where("account_id = ? AND workspace_id = ? AND role IN ?", memberID, workspaceID, []model.WorkspaceMemberRole{model.WorkspaceRoleOwner, model.WorkspaceRoleAdmin}).Count(&count).Error
	return count > 0, err
}

func (r *workspaceMemberRepository) WithTx(tx *gorm.DB) WorkspaceMemberRepository {
	return NewWorkspaceMemberRepository(tx)
}
