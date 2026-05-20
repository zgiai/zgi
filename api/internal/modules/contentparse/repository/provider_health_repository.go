package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/contentparse/model"
	"gorm.io/gorm"
)

type ProviderHealthRepository interface {
	Create(ctx context.Context, item *model.ProviderHealthCheck) error
	ListByProviderConfigID(ctx context.Context, providerConfigID uuid.UUID, limit int) ([]*model.ProviderHealthCheck, error)
	GetLatestByProviderConfigID(ctx context.Context, providerConfigID uuid.UUID) (*model.ProviderHealthCheck, error)
}

type providerHealthRepository struct {
	db *gorm.DB
}

func NewProviderHealthRepository(db *gorm.DB) ProviderHealthRepository {
	return &providerHealthRepository{db: db}
}

func (r *providerHealthRepository) Create(ctx context.Context, item *model.ProviderHealthCheck) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *providerHealthRepository) ListByProviderConfigID(ctx context.Context, providerConfigID uuid.UUID, limit int) ([]*model.ProviderHealthCheck, error) {
	var items []*model.ProviderHealthCheck
	query := r.db.WithContext(ctx).
		Where("provider_config_id = ?", providerConfigID).
		Order("checked_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&items).Error
	return items, err
}

func (r *providerHealthRepository) GetLatestByProviderConfigID(ctx context.Context, providerConfigID uuid.UUID) (*model.ProviderHealthCheck, error) {
	var item model.ProviderHealthCheck
	err := r.db.WithContext(ctx).
		Where("provider_config_id = ?", providerConfigID).
		Order("checked_at DESC").
		First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}
