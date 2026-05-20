package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
	"gorm.io/gorm"
)

type providerRepository struct {
	db *gorm.DB
}

// NewProviderRepository creates a new global provider repository
func NewProviderRepository(db *gorm.DB) ProviderRepository {
	return &providerRepository{db: db}
}

func (r *providerRepository) Create(ctx context.Context, provider *model.LLMProvider) error {
	return r.db.WithContext(ctx).Create(provider).Error
}

func (r *providerRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.LLMProvider, error) {
	var provider model.LLMProvider
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&provider).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

func (r *providerRepository) GetByName(ctx context.Context, name string) (*model.LLMProvider, error) {
	var provider model.LLMProvider
	err := r.db.WithContext(ctx).Where("provider = ?", name).First(&provider).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

func (r *providerRepository) List(ctx context.Context, isActive *bool, offset, limit int) ([]*model.LLMProvider, int64, error) {
	var providers []*model.LLMProvider
	var total int64

	query := r.db.WithContext(ctx).Model(&model.LLMProvider{})

	if isActive != nil {
		query = query.Where("is_active = ?", *isActive)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("sort_order ASC, provider ASC").Offset(offset).Limit(limit).Find(&providers).Error; err != nil {
		return nil, 0, err
	}

	return providers, total, nil
}

func (r *providerRepository) Update(ctx context.Context, provider *model.LLMProvider) error {
	return r.db.WithContext(ctx).Save(provider).Error
}

func (r *providerRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.LLMProvider{}, "id = ?", id).Error
}

func (r *providerRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.LLMProvider{}).Where("provider = ?", name).Count(&count).Error
	return count > 0, err
}
