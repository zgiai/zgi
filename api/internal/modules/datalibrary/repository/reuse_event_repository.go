package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/datalibrary/model"
	"gorm.io/gorm"
)

type ReuseEventListFilter struct {
	OrganizationID string
	AssetID        *uuid.UUID
	ConsumerType   string
	ConsumerID     string
	Limit          int
	Offset         int
}

type ReuseEventRepository interface {
	Create(ctx context.Context, item *model.ReuseEvent) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.ReuseEvent, error)
	List(ctx context.Context, filter ReuseEventListFilter) ([]*model.ReuseEvent, int64, error)
	SummaryByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) (int64, int64, int64, error)
	SumSavingsByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) (int64, int64, error)
}

type reuseEventRepository struct {
	db *gorm.DB
}

func NewReuseEventRepository(db *gorm.DB) ReuseEventRepository {
	return &reuseEventRepository{db: db}
}

func (r *reuseEventRepository) Create(ctx context.Context, item *model.ReuseEvent) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *reuseEventRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.ReuseEvent, error) {
	var item model.ReuseEvent
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *reuseEventRepository) List(ctx context.Context, filter ReuseEventListFilter) ([]*model.ReuseEvent, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.ReuseEvent{}).
		Where("organization_id = ?", filter.OrganizationID)
	if filter.AssetID != nil {
		query = query.Where("asset_id = ?", *filter.AssetID)
	}
	if filter.ConsumerType != "" {
		query = query.Where("consumer_type = ?", filter.ConsumerType)
	}
	if filter.ConsumerID != "" {
		query = query.Where("consumer_id = ?", filter.ConsumerID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var items []*model.ReuseEvent
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *reuseEventRepository) SumSavingsByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) (int64, int64, error) {
	_, savedSeconds, savedCostMicros, err := r.SummaryByAssetID(ctx, organizationID, assetID)
	return savedSeconds, savedCostMicros, err
}

func (r *reuseEventRepository) SummaryByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) (int64, int64, int64, error) {
	var result struct {
		ReuseCount      int64
		SavedSeconds    int64
		SavedCostMicros int64
	}
	err := r.db.WithContext(ctx).Model(&model.ReuseEvent{}).
		Select("COUNT(*) AS reuse_count, COALESCE(SUM(saved_seconds), 0) AS saved_seconds, COALESCE(SUM(saved_cost_micros), 0) AS saved_cost_micros").
		Where("organization_id = ? AND asset_id = ?", organizationID, assetID).
		Scan(&result).Error
	if err != nil {
		return 0, 0, 0, err
	}
	return result.ReuseCount, result.SavedSeconds, result.SavedCostMicros, nil
}
