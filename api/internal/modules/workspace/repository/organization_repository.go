package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/workspace/model"
	"gorm.io/gorm"
)

type OrganizationRepository interface {
	Create(ctx context.Context, organization *model.Organization) error
	CreateWithTx(ctx context.Context, tx *gorm.DB, organization *model.Organization) error
	GetByID(ctx context.Context, id string) (*model.Organization, error)
	Update(ctx context.Context, organization *model.Organization) error
	Delete(ctx context.Context, id string) error
	ExistsByName(ctx context.Context, name string) (bool, error)

	CreateAccountJoin(ctx context.Context, join *model.OrganizationMember) error
	CreateAccountJoinWithTx(ctx context.Context, tx *gorm.DB, join *model.OrganizationMember) error
	GetAccountJoin(ctx context.Context, organizationID, accountID string) (*model.OrganizationMember, error)
	UpdateAccountJoin(ctx context.Context, join *model.OrganizationMember) error
	DeleteAccountJoin(ctx context.Context, organizationID, accountID string) error
	GetAccountsByOrganizationID(ctx context.Context, organizationID string) ([]*model.OrganizationMember, error)
	ExistsMemberByName(ctx context.Context, organizationID string, name string, excludeAccountID string) (bool, error)

	// Workspace management (migrated to workspaces.organization_id)
	AddWorkspaceToOrganization(ctx context.Context, organizationID, workspaceID string) error
	RemoveWorkspaceFromOrganization(ctx context.Context, organizationID, workspaceID string) error
	GetWorkspacesByOrganizationID(ctx context.Context, organizationID string) ([]*model.Workspace, error)
	GetOrganizationIDByWorkspaceID(ctx context.Context, workspaceID string) (string, error)

	ListAllWithPagination(ctx context.Context, page, limit int, status string) ([]*model.Organization, int64, error)
	ListUserOrganizationsForAccount(ctx context.Context, page, limit int, status, accountID string) ([]*model.Organization, int64, error)

	GetInviteLinkByDepartment(ctx context.Context, organizationID, departmentID string) (*model.OrganizationInviteLink, error)
	GetInviteLinkByOrganization(ctx context.Context, organizationID string) (*model.OrganizationInviteLink, error)
	GetInviteLinkByToken(ctx context.Context, token string) (*model.OrganizationInviteLink, error)
	CreateInviteLink(ctx context.Context, link *model.OrganizationInviteLink) error
	UpdateInviteLink(ctx context.Context, link *model.OrganizationInviteLink) error

	CreateJoinRequest(ctx context.Context, req *model.OrganizationJoinRequest) error
	UpdateJoinRequest(ctx context.Context, req *model.OrganizationJoinRequest) error
	GetJoinRequestByID(ctx context.Context, id string) (*model.OrganizationJoinRequest, error)
	ListJoinRequestsByDepartment(ctx context.Context, organizationID, departmentID string, status *model.OrganizationJoinRequestStatus) ([]*model.OrganizationJoinRequest, error)
	ListJoinRequestsByOrganization(ctx context.Context, organizationID string, departmentID *string, status *model.OrganizationJoinRequestStatus, page, limit int) ([]*model.OrganizationJoinRequest, int64, error)
	GetPendingJoinRequest(ctx context.Context, organizationID, accountID string) (*model.OrganizationJoinRequest, error)

	GetFirstOwnedOrganization(ctx context.Context, accountID string) (*model.Organization, error)
	GetFirstJoinedOrganization(ctx context.Context, accountID string) (*model.Organization, error)

	GetDB() *gorm.DB
	WithTx(tx *gorm.DB) OrganizationRepository
}

type organizationRepository struct {
	db *gorm.DB
}

func NewOrganizationRepository(db *gorm.DB) OrganizationRepository {
	return &organizationRepository{db: db}
}

func (r *organizationRepository) Create(ctx context.Context, organization *model.Organization) error {
	if organization.ID == "" {
		organization.ID = uuid.New().String()
	}
	organization.CreatedAt = time.Now()
	organization.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Create(organization).Error
}

func (r *organizationRepository) CreateWithTx(ctx context.Context, tx *gorm.DB, organization *model.Organization) error {
	if organization.ID == "" {
		organization.ID = uuid.New().String()
	}
	organization.CreatedAt = time.Now()
	organization.UpdatedAt = time.Now()
	return tx.WithContext(ctx).Create(organization).Error
}

func (r *organizationRepository) GetByID(ctx context.Context, id string) (*model.Organization, error) {
	var organization model.Organization
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&organization).Error
	if err != nil {
		return nil, err
	}
	return &organization, nil
}

func (r *organizationRepository) Update(ctx context.Context, organization *model.Organization) error {
	organization.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(organization).Error
}

func (r *organizationRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.Organization{}, "id = ?", id).Error
}

func (r *organizationRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Organization{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

func (r *organizationRepository) CreateAccountJoin(ctx context.Context, join *model.OrganizationMember) error {
	join.CreatedAt = time.Now()
	join.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Create(join).Error
}

func (r *organizationRepository) CreateAccountJoinWithTx(ctx context.Context, tx *gorm.DB, join *model.OrganizationMember) error {
	join.CreatedAt = time.Now()
	join.UpdatedAt = time.Now()
	return tx.WithContext(ctx).Create(join).Error
}

func (r *organizationRepository) GetAccountJoin(ctx context.Context, organizationID, accountID string) (*model.OrganizationMember, error) {
	var join model.OrganizationMember
	err := r.db.WithContext(ctx).Where("organization_id = ? AND account_id = ?", organizationID, accountID).First(&join).Error
	if err != nil {
		return nil, err
	}
	return &join, nil
}

func (r *organizationRepository) UpdateAccountJoin(ctx context.Context, join *model.OrganizationMember) error {
	join.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(join).Error
}

func (r *organizationRepository) DeleteAccountJoin(ctx context.Context, organizationID, accountID string) error {
	return r.db.WithContext(ctx).Delete(&model.OrganizationMember{}, "organization_id = ? AND account_id = ?", organizationID, accountID).Error
}

func (r *organizationRepository) GetAccountsByOrganizationID(ctx context.Context, organizationID string) ([]*model.OrganizationMember, error) {
	var joins []*model.OrganizationMember
	err := r.db.WithContext(ctx).Where("organization_id = ?", organizationID).Find(&joins).Error
	return joins, err
}

func (r *organizationRepository) ExistsMemberByName(ctx context.Context, organizationID string, name string, excludeAccountID string) (bool, error) {
	var count int64
	query := r.db.WithContext(ctx).Model(&model.OrganizationMember{}).
		Where("organization_id = ? AND name = ?", organizationID, name)

	if excludeAccountID != "" {
		query = query.Where("account_id != ?", excludeAccountID)
	}

	err := query.Count(&count).Error
	return count > 0, err
}

func (r *organizationRepository) AddWorkspaceToOrganization(ctx context.Context, organizationID, workspaceID string) error {
	return r.db.WithContext(ctx).Model(&model.Workspace{}).
		Where("id = ?", workspaceID).
		Update("organization_id", organizationID).Error
}

func (r *organizationRepository) RemoveWorkspaceFromOrganization(ctx context.Context, organizationID, workspaceID string) error {
	return r.db.WithContext(ctx).Model(&model.Workspace{}).
		Where("id = ? AND organization_id = ?", workspaceID, organizationID).
		Updates(map[string]interface{}{
			"organization_id": nil,
		}).Error
}

func (r *organizationRepository) GetWorkspacesByOrganizationID(ctx context.Context, organizationID string) ([]*model.Workspace, error) {
	var workspaces []*model.Workspace
	err := r.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Find(&workspaces).Error
	if err != nil {
		return nil, err
	}
	return workspaces, nil
}

func (r *organizationRepository) GetOrganizationIDByWorkspaceID(ctx context.Context, workspaceID string) (string, error) {
	var workspace model.Workspace
	err := r.db.WithContext(ctx).
		Select("organization_id").
		Where("id = ?", workspaceID).
		First(&workspace).Error
	if err != nil {
		return "", err
	}
	if workspace.OrganizationID == nil {
		return "", nil
	}
	return *workspace.OrganizationID, nil
}

func (r *organizationRepository) ListAllWithPagination(ctx context.Context, page, limit int, status string) ([]*model.Organization, int64, error) {
	var organizations []*model.Organization
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Organization{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	err = query.Offset(offset).Limit(limit).Find(&organizations).Error
	return organizations, total, err
}

func (r *organizationRepository) ListUserOrganizationsForAccount(ctx context.Context, page, limit int, status, accountID string) ([]*model.Organization, int64, error) {
	db := r.db

	// Build the query
	query := db.WithContext(ctx).Model(&model.Organization{})

	// Direct condition - user is directly in the organization
	directCondition := `
		EXISTS (
			SELECT 1 FROM members 
			WHERE members.organization_id = organizations.id 
			AND members.account_id = ?
		)`

	// Indirect condition - user is in a workspace that belongs to the organization
	indirectCondition := `
		EXISTS (
			SELECT 1 FROM workspaces 
			JOIN workspace_members ON workspace_members.workspace_id = workspaces.id
			WHERE workspaces.organization_id = organizations.id 
			AND workspace_members.account_id = ?
		)`

	// Combine both conditions with OR
	query = query.Where(directCondition+" OR "+indirectCondition, accountID, accountID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	// Get total count
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * limit
	var organizations []*model.Organization
	err = query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&organizations).Error
	if err != nil {
		return nil, 0, err
	}

	return organizations, total, nil
}

func (r *organizationRepository) GetDB() *gorm.DB {
	return r.db
}

func (r *organizationRepository) WithTx(tx *gorm.DB) OrganizationRepository {
	return NewOrganizationRepository(tx)
}

func (r *organizationRepository) GetInviteLinkByDepartment(ctx context.Context, organizationID, departmentID string) (*model.OrganizationInviteLink, error) {
	var link model.OrganizationInviteLink
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND department_id = ?", organizationID, departmentID).
		Order("created_at DESC").
		First(&link).Error
	if err != nil {
		return nil, err
	}
	return &link, nil
}

func (r *organizationRepository) GetInviteLinkByOrganization(ctx context.Context, organizationID string) (*model.OrganizationInviteLink, error) {
	var link model.OrganizationInviteLink
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND department_id IS NULL", organizationID).
		Order("created_at DESC").
		First(&link).Error
	if err != nil {
		return nil, err
	}
	return &link, nil
}

func (r *organizationRepository) GetInviteLinkByToken(ctx context.Context, token string) (*model.OrganizationInviteLink, error) {
	var link model.OrganizationInviteLink
	err := r.db.WithContext(ctx).
		Where("token = ?", token).
		First(&link).Error
	if err != nil {
		return nil, err
	}
	return &link, nil
}

func (r *organizationRepository) CreateInviteLink(ctx context.Context, link *model.OrganizationInviteLink) error {
	now := time.Now()
	if link.CreatedAt.IsZero() {
		link.CreatedAt = now
	}
	link.UpdatedAt = now
	if link.ID == "" {
		link.ID = uuid.New().String()
	}
	return r.db.WithContext(ctx).Create(link).Error
}

func (r *organizationRepository) UpdateInviteLink(ctx context.Context, link *model.OrganizationInviteLink) error {
	link.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(link).Error
}

func (r *organizationRepository) CreateJoinRequest(ctx context.Context, req *model.OrganizationJoinRequest) error {
	if req.ID == "" {
		req.ID = uuid.New().String()
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now()
	}
	return r.db.WithContext(ctx).Create(req).Error
}

func (r *organizationRepository) UpdateJoinRequest(ctx context.Context, req *model.OrganizationJoinRequest) error {
	return r.db.WithContext(ctx).Save(req).Error
}

func (r *organizationRepository) GetJoinRequestByID(ctx context.Context, id string) (*model.OrganizationJoinRequest, error) {
	var req model.OrganizationJoinRequest
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&req).Error
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *organizationRepository) ListJoinRequestsByDepartment(ctx context.Context, organizationID, departmentID string, status *model.OrganizationJoinRequestStatus) ([]*model.OrganizationJoinRequest, error) {
	var requests []*model.OrganizationJoinRequest
	query := r.db.WithContext(ctx).
		Where("group_id = ? AND department_id = ?", organizationID, departmentID)
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	err := query.Order("created_at DESC").Find(&requests).Error
	if err != nil {
		return nil, err
	}
	return requests, nil
}

func (r *organizationRepository) ListJoinRequestsByOrganization(ctx context.Context, organizationID string, departmentID *string, status *model.OrganizationJoinRequestStatus, page, limit int) ([]*model.OrganizationJoinRequest, int64, error) {
	var requests []*model.OrganizationJoinRequest
	query := r.db.WithContext(ctx).
		Model(&model.OrganizationJoinRequest{}).
		Where("group_id = ?", organizationID)
	if departmentID != nil && *departmentID != "" {
		query = query.Where("department_id = ?", *departmentID)
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&requests).Error; err != nil {
		return nil, 0, err
	}

	return requests, total, nil
}

func (r *organizationRepository) GetPendingJoinRequest(ctx context.Context, organizationID, accountID string) (*model.OrganizationJoinRequest, error) {
	var req model.OrganizationJoinRequest
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND account_id = ? AND status = ?", organizationID, accountID, model.OrganizationJoinRequestStatusPending).
		First(&req).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &req, nil
}

func (r *organizationRepository) GetFirstOwnedOrganization(ctx context.Context, accountID string) (*model.Organization, error) {
	var organization model.Organization
	// Join with members table
	err := r.db.WithContext(ctx).
		Model(&model.Organization{}).
		Joins("JOIN members ON members.organization_id = organizations.id").
		Where("members.account_id = ? AND members.role = ?", accountID, "owner").
		Order("organizations.created_at ASC").
		First(&organization).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &organization, nil
}

func (r *organizationRepository) GetFirstJoinedOrganization(ctx context.Context, accountID string) (*model.Organization, error) {
	var organization model.Organization
	err := r.db.WithContext(ctx).
		Model(&model.Organization{}).
		Joins("JOIN members ON members.organization_id = organizations.id").
		Where("members.account_id = ?", accountID).
		Order("organizations.created_at ASC").
		First(&organization).Error

	if err != nil {
		return nil, err
	}
	return &organization, nil
}
