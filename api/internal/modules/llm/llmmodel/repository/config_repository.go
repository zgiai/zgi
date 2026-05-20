package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	"gorm.io/gorm"
)

type modelConfigRepository struct {
	db *gorm.DB
}

// NewModelConfigRepository creates a new model config repository
func NewModelConfigRepository(db *gorm.DB) ModelConfigRepository {
	return &modelConfigRepository{db: db}
}

func (r *modelConfigRepository) Create(ctx context.Context, config *model.ModelConfig) error {
	return r.db.WithContext(ctx).Create(config).Error
}

func (r *modelConfigRepository) GetByID(ctx context.Context, organizationID, id uuid.UUID) (*model.ModelConfig, error) {
	var config model.ModelConfig
	err := r.db.WithContext(ctx).
		Preload("Model").
		Where("id = ? AND organization_id = ?", id, organizationID).
		First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *modelConfigRepository) GetByModelID(ctx context.Context, organizationID, modelID uuid.UUID) (*model.ModelConfig, error) {
	var config model.ModelConfig
	err := r.db.WithContext(ctx).
		Preload("Model").
		Where("organization_id = ? AND model_id = ?", organizationID, modelID).
		First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *modelConfigRepository) List(ctx context.Context, organizationID uuid.UUID, isEnabled *bool, offset, limit int) ([]*model.ModelConfig, int64, error) {
	var configs []*model.ModelConfig
	var total int64

	query := r.db.WithContext(ctx).Model(&model.ModelConfig{}).
		Where("organization_id = ?", organizationID)

	if isEnabled != nil {
		query = query.Where("is_enabled = ?", *isEnabled)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Preload("Model").Order("sort_order ASC, created_at DESC").Offset(offset).Limit(limit).Find(&configs).Error; err != nil {
		return nil, 0, err
	}

	return configs, total, nil
}

func (r *modelConfigRepository) ListAvailableConfigs(ctx context.Context, organizationID uuid.UUID) ([]*model.ModelConfig, error) {
	var configs []*model.ModelConfig
	err := r.db.WithContext(ctx).
		Model(&model.ModelConfig{}).
		Select("id, organization_id, model_id, is_enabled, custom_display_name").
		Where("organization_id = ?", organizationID).
		Order("sort_order ASC, created_at DESC").
		Find(&configs).Error
	return configs, err
}

func (r *modelConfigRepository) Update(ctx context.Context, config *model.ModelConfig) error {
	return r.db.WithContext(ctx).Save(config).Error
}

func (r *modelConfigRepository) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ?", id, organizationID).
		Delete(&model.ModelConfig{}).Error
}

func (r *modelConfigRepository) Upsert(ctx context.Context, config *model.ModelConfig) error {
	// Check if config already exists
	var existing model.ModelConfig
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND model_id = ?", config.OrganizationID, config.ModelID).
		First(&existing).Error

	if err == nil {
		// Record exists, update it
		config.ID = existing.ID
		config.CreatedAt = existing.CreatedAt
		return r.db.WithContext(ctx).Save(config).Error
	}

	// Record doesn't exist, create it
	return r.db.WithContext(ctx).Create(config).Error
}

func (r *modelConfigRepository) BatchCreate(ctx context.Context, configs []*model.ModelConfig) error {
	if len(configs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(configs, 100).Error
}

// ============================================================================
// Custom Model Repository
// ============================================================================

type customModelRepository struct {
	db *gorm.DB
}

const activeCustomProviderJoin = `
JOIN llm_custom_providers active_custom_providers
  ON active_custom_providers.id = llm_custom_models.provider_id
 AND active_custom_providers.organization_id = llm_custom_models.organization_id
 AND active_custom_providers.is_active = ?
 AND active_custom_providers.deleted_at IS NULL`

func joinActiveCustomProvider(query *gorm.DB) *gorm.DB {
	return query.Joins(activeCustomProviderJoin, true)
}

// NewCustomModelRepository creates a new custom model repository
func NewCustomModelRepository(db *gorm.DB) CustomModelRepository {
	return &customModelRepository{db: db}
}

func (r *customModelRepository) Create(ctx context.Context, m *model.CustomModel) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *customModelRepository) GetByID(ctx context.Context, organizationID, id uuid.UUID) (*model.CustomModel, error) {
	var m model.CustomModel
	err := r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ?", id, organizationID).
		First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *customModelRepository) GetByProviderAndName(ctx context.Context, organizationID, providerID uuid.UUID, name string) (*model.CustomModel, error) {
	var m model.CustomModel
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND provider_id = ? AND name = ?", organizationID, providerID, name).
		First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *customModelRepository) GetByProviderAndModel(ctx context.Context, organizationID uuid.UUID, provider string, name string) (*model.CustomModel, error) {
	var m model.CustomModel
	err := joinActiveCustomProvider(r.db.WithContext(ctx).Model(&model.CustomModel{})).
		Where("llm_custom_models.organization_id = ? AND llm_custom_models.provider = ? AND llm_custom_models.name = ?", organizationID, provider, name).
		Order("llm_custom_models.sort_order ASC, llm_custom_models.created_at DESC").
		First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *customModelRepository) ListByNames(ctx context.Context, organizationID uuid.UUID, names []string, isActive *bool) ([]*model.CustomModel, error) {
	if len(names) == 0 {
		return []*model.CustomModel{}, nil
	}

	var models []*model.CustomModel
	query := r.db.WithContext(ctx).Model(&model.CustomModel{}).
		Where("llm_custom_models.organization_id = ? AND llm_custom_models.name IN ?", organizationID, names)
	if isActive != nil {
		query = query.Where("llm_custom_models.is_active = ?", *isActive)
		if *isActive {
			query = joinActiveCustomProvider(query)
		}
	}

	if err := query.Order("llm_custom_models.sort_order ASC, llm_custom_models.name ASC, llm_custom_models.created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}

	return models, nil
}

func (r *customModelRepository) List(ctx context.Context, organizationID uuid.UUID, providerID *uuid.UUID, provider string, useCase string, isActive *bool, offset, limit int) ([]*model.CustomModel, int64, error) {
	var models []*model.CustomModel
	var total int64

	query := r.db.WithContext(ctx).Model(&model.CustomModel{}).
		Where("llm_custom_models.organization_id = ?", organizationID)

	if providerID != nil {
		query = query.Where("llm_custom_models.provider_id = ?", *providerID)
	}
	if provider != "" {
		query = query.Where("llm_custom_models.provider = ?", provider)
	}
	if useCase != "" {
		query = query.Where("? = ANY(use_cases)", useCase)
	}
	if isActive != nil {
		query = query.Where("llm_custom_models.is_active = ?", *isActive)
		if *isActive {
			query = joinActiveCustomProvider(query)
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("llm_custom_models.sort_order ASC, llm_custom_models.created_at DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, err
	}

	return models, total, nil
}

func (r *customModelRepository) Update(ctx context.Context, m *model.CustomModel) error {
	return r.db.WithContext(ctx).Save(m).Error
}

func (r *customModelRepository) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ?", id, organizationID).
		Delete(&model.CustomModel{}).Error
}

func (r *customModelRepository) ListByProvider(ctx context.Context, organizationID, providerID uuid.UUID) ([]*model.CustomModel, error) {
	var models []*model.CustomModel
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND provider_id = ? AND is_active = true", organizationID, providerID).
		Order("sort_order ASC, name ASC").
		Find(&models).Error
	return models, err
}
