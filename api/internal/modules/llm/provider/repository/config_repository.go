package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
	"gorm.io/gorm"
)

type providerConfigRepository struct {
	db *gorm.DB
}

// NewProviderConfigRepository creates a new provider config repository
func NewProviderConfigRepository(db *gorm.DB) ProviderConfigRepository {
	return &providerConfigRepository{db: db}
}

func (r *providerConfigRepository) Create(ctx context.Context, config *model.ProviderConfig) error {
	return r.db.WithContext(ctx).Create(config).Error
}

func (r *providerConfigRepository) GetByID(ctx context.Context, organizationID, id uuid.UUID) (*model.ProviderConfig, error) {
	var config model.ProviderConfig
	err := r.db.WithContext(ctx).
		Preload("Provider").
		Where("id = ? AND organization_id = ?", id, organizationID).
		First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *providerConfigRepository) GetByProviderID(ctx context.Context, organizationID, providerID uuid.UUID) (*model.ProviderConfig, error) {
	var config model.ProviderConfig
	err := r.db.WithContext(ctx).
		Preload("Provider").
		Where("organization_id = ? AND provider_id = ?", organizationID, providerID).
		First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *providerConfigRepository) List(ctx context.Context, organizationID uuid.UUID, isEnabled *bool, offset, limit int) ([]*model.ProviderConfig, int64, error) {
	var configs []*model.ProviderConfig
	var total int64

	query := r.db.WithContext(ctx).Model(&model.ProviderConfig{}).
		Where("organization_id = ?", organizationID)

	if isEnabled != nil {
		query = query.Where("is_enabled = ?", *isEnabled)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Temporarily disable Preload to test if it's causing the organization_id error
	if err := query.Order("sort_order ASC, created_at DESC").Offset(offset).Limit(limit).Find(&configs).Error; err != nil {
		return nil, 0, err
	}

	return configs, total, nil
}

func (r *providerConfigRepository) Update(ctx context.Context, config *model.ProviderConfig) error {
	return r.db.WithContext(ctx).Save(config).Error
}

func (r *providerConfigRepository) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ?", id, organizationID).
		Delete(&model.ProviderConfig{}).Error
}

func (r *providerConfigRepository) Upsert(ctx context.Context, config *model.ProviderConfig) error {
	var existing model.ProviderConfig
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND provider_id = ?", config.OrganizationID, config.ProviderID).
		First(&existing).Error

	if err == nil {
		// Record exists, update it
		existing.IsEnabled = config.IsEnabled
		if config.CustomDisplayName != "" {
			existing.CustomDisplayName = config.CustomDisplayName
		}
		if config.CustomAPIBaseURL != "" {
			existing.CustomAPIBaseURL = config.CustomAPIBaseURL
		}
		if config.CustomLogoURL != "" {
			existing.CustomLogoURL = config.CustomLogoURL
		}
		return r.db.WithContext(ctx).Save(&existing).Error
	}

	// Record doesn't exist, create it
	// Use Select to force include is_enabled field even when it's false (zero value)
	// Without this, GORM will skip false values and use database default (true)
	return r.db.WithContext(ctx).Select("*").Create(config).Error
}

// ============================================================================
// Custom Provider Repository
// ============================================================================

type customProviderRepository struct {
	db *gorm.DB
}

// NewCustomProviderRepository creates a new custom provider repository
func NewCustomProviderRepository(db *gorm.DB) CustomProviderRepository {
	return &customProviderRepository{db: db}
}

func (r *customProviderRepository) Create(ctx context.Context, provider *model.CustomProvider) error {
	return r.db.WithContext(ctx).Create(provider).Error
}

func (r *customProviderRepository) GetByID(ctx context.Context, organizationID, id uuid.UUID) (*model.CustomProvider, error) {
	var provider model.CustomProvider
	err := r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ?", id, organizationID).
		First(&provider).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

func (r *customProviderRepository) GetByProvider(ctx context.Context, organizationID uuid.UUID, provider string) (*model.CustomProvider, error) {
	var result model.CustomProvider
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND provider = ?", organizationID, provider).
		First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *customProviderRepository) List(ctx context.Context, organizationID uuid.UUID, isActive *bool, offset, limit int) ([]*model.CustomProvider, int64, error) {
	var providers []*model.CustomProvider
	var total int64

	query := r.db.WithContext(ctx).Model(&model.CustomProvider{}).
		Where("organization_id = ?", organizationID)

	if isActive != nil {
		query = query.Where("is_active = ?", *isActive)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("sort_order ASC, created_at DESC").Offset(offset).Limit(limit).Find(&providers).Error; err != nil {
		return nil, 0, err
	}

	return providers, total, nil
}

func (r *customProviderRepository) Update(ctx context.Context, provider *model.CustomProvider) error {
	return r.db.WithContext(ctx).Save(provider).Error
}

func (r *customProviderRepository) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ?", id, organizationID).
		Delete(&model.CustomProvider{}).Error
}

func (r *customProviderRepository) ExistsByProvider(ctx context.Context, organizationID uuid.UUID, provider string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.CustomProvider{}).
		Where("organization_id = ? AND provider = ?", organizationID, provider).
		Count(&count).Error
	return count > 0, err
}
