package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/llm/credential/model"
	"gorm.io/gorm"
)

type tenantCredentialRepository struct {
	db *gorm.DB
}

// NewTenantCredentialRepository creates a new tenant credential repository
func NewTenantCredentialRepository(db *gorm.DB) TenantCredentialRepository {
	return &tenantCredentialRepository{db: db}
}

func (r *tenantCredentialRepository) Create(ctx context.Context, credential *model.TenantCredential) error {
	return r.db.WithContext(ctx).Create(credential).Error
}

func (r *tenantCredentialRepository) GetByID(ctx context.Context, organizationID, id uuid.UUID) (*model.TenantCredential, error) {
	var credential model.TenantCredential
	err := r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ?", id, organizationID).
		First(&credential).Error
	if err != nil {
		return nil, err
	}
	return &credential, nil
}

func (r *tenantCredentialRepository) GetByHash(ctx context.Context, organizationID uuid.UUID, hash string) (*model.TenantCredential, error) {
	var credential model.TenantCredential
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND api_key_hash = ?", organizationID, hash).
		First(&credential).Error
	if err != nil {
		return nil, err
	}
	return &credential, nil
}

func (r *tenantCredentialRepository) GetBySignature(ctx context.Context, organizationID uuid.UUID, hash, channelProvider, apiBaseURL string) (*model.TenantCredential, error) {
	var credential model.TenantCredential
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND api_key_hash = ? AND provider = ? AND COALESCE(api_base_url, '') = ?", organizationID, hash, channelProvider, apiBaseURL).
		First(&credential).Error
	if err != nil {
		return nil, err
	}
	return &credential, nil
}

func (r *tenantCredentialRepository) List(ctx context.Context, organizationID uuid.UUID, provider string, isActive *bool, offset, limit int) ([]*model.TenantCredential, int64, error) {
	var credentials []*model.TenantCredential
	var total int64

	query := r.db.WithContext(ctx).Model(&model.TenantCredential{}).
		Where("organization_id = ?", organizationID)

	if provider != "" {
		query = query.Where("provider = ?", provider)
	}
	if isActive != nil {
		query = query.Where("is_active = ?", *isActive)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&credentials).Error; err != nil {
		return nil, 0, err
	}

	return credentials, total, nil
}

func (r *tenantCredentialRepository) Update(ctx context.Context, credential *model.TenantCredential) error {
	return r.db.WithContext(ctx).Save(credential).Error
}

func (r *tenantCredentialRepository) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ?", id, organizationID).
		Delete(&model.TenantCredential{}).Error
}

func (r *tenantCredentialRepository) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&model.TenantCredential{}).
		Where("id = ?", id).
		Update("last_used_at", gorm.Expr("NOW()")).Error
}

func (r *tenantCredentialRepository) ExistsByHash(ctx context.Context, organizationID uuid.UUID, hash string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.TenantCredential{}).
		Where("organization_id = ? AND api_key_hash = ?", organizationID, hash).
		Count(&count).Error
	return count > 0, err
}
