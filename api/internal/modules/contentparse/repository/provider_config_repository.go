package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/contentparse/model"
	"gorm.io/gorm"
)

type ProviderConfigRepository interface {
	Create(ctx context.Context, item *model.ProviderConfig) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.ProviderConfig, error)
	GetByScopeAndKey(ctx context.Context, scope string, workspaceID *uuid.UUID, providerKey string) (*model.ProviderConfig, error)
	ListByScope(ctx context.Context, scope string, workspaceID *uuid.UUID) ([]*model.ProviderConfig, error)
	Update(ctx context.Context, item *model.ProviderConfig) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type providerConfigRepository struct {
	db *gorm.DB
}

func NewProviderConfigRepository(db *gorm.DB) ProviderConfigRepository {
	return &providerConfigRepository{db: db}
}

func (r *providerConfigRepository) Create(ctx context.Context, item *model.ProviderConfig) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *providerConfigRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.ProviderConfig, error) {
	var item model.ProviderConfig
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *providerConfigRepository) GetByScopeAndKey(ctx context.Context, scope string, workspaceID *uuid.UUID, providerKey string) (*model.ProviderConfig, error) {
	var item model.ProviderConfig
	query := r.db.WithContext(ctx).Where("scope = ? AND provider_key = ?", scope, providerKey)
	if workspaceID == nil {
		query = query.Where("workspace_id IS NULL")
	} else {
		query = query.Where("workspace_id = ?", *workspaceID)
	}
	err := query.First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *providerConfigRepository) ListByScope(ctx context.Context, scope string, workspaceID *uuid.UUID) ([]*model.ProviderConfig, error) {
	var items []*model.ProviderConfig
	query := r.db.WithContext(ctx).Where("scope = ?", scope)
	if workspaceID == nil {
		query = query.Where("workspace_id IS NULL")
	} else {
		query = query.Where("workspace_id = ?", *workspaceID)
	}
	err := query.Order("priority ASC, created_at DESC").Find(&items).Error
	return items, err
}

func (r *providerConfigRepository) Update(ctx context.Context, item *model.ProviderConfig) error {
	return r.db.WithContext(ctx).Save(item).Error
}

func (r *providerConfigRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.ProviderConfig{}, "id = ?", id).Error
}
