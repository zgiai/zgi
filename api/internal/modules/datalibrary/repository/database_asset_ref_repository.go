package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/datalibrary/model"
	"gorm.io/gorm"
)

type DatabaseAssetRefListFilter struct {
	OrganizationID string
	DataSourceID   string
	TableID        string
	AssetID        uuid.UUID
	VersionID      uuid.UUID
	Status         string
	Limit          int
	Offset         int
}

type DatabaseAssetRefRepository interface {
	Create(ctx context.Context, item *model.DatabaseAssetRef) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.DatabaseAssetRef, error)
	List(ctx context.Context, filter DatabaseAssetRefListFilter) ([]*model.DatabaseAssetRef, int64, error)
	FindActive(ctx context.Context, organizationID string, dataSourceID string, tableID *string, assetID uuid.UUID, versionID uuid.UUID) (*model.DatabaseAssetRef, error)
	CountActiveByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) (int64, error)
	UpdateStatus(ctx context.Context, organizationID string, id uuid.UUID, status string) (*model.DatabaseAssetRef, error)
}

type databaseAssetRefRepository struct {
	db *gorm.DB
}

func NewDatabaseAssetRefRepository(db *gorm.DB) DatabaseAssetRefRepository {
	return &databaseAssetRefRepository{db: db}
}

func (r *databaseAssetRefRepository) Create(ctx context.Context, item *model.DatabaseAssetRef) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *databaseAssetRefRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.DatabaseAssetRef, error) {
	var item model.DatabaseAssetRef
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *databaseAssetRefRepository) List(ctx context.Context, filter DatabaseAssetRefListFilter) ([]*model.DatabaseAssetRef, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.DatabaseAssetRef{}).
		Where("organization_id = ?", filter.OrganizationID)
	if filter.DataSourceID != "" {
		query = query.Where("data_source_id = ?", filter.DataSourceID)
	}
	if filter.TableID != "" {
		query = query.Where("table_id = ?", filter.TableID)
	}
	if filter.AssetID != uuid.Nil {
		query = query.Where("asset_id = ?", filter.AssetID)
	}
	if filter.VersionID != uuid.Nil {
		query = query.Where("version_id = ?", filter.VersionID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
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

	var items []*model.DatabaseAssetRef
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *databaseAssetRefRepository) FindActive(ctx context.Context, organizationID string, dataSourceID string, tableID *string, assetID uuid.UUID, versionID uuid.UUID) (*model.DatabaseAssetRef, error) {
	query := r.db.WithContext(ctx).
		Where("organization_id = ? AND data_source_id = ? AND asset_id = ? AND version_id = ? AND status = ?",
			organizationID, dataSourceID, assetID, versionID, model.DatabaseAssetRefStatusActive)
	if tableID == nil || *tableID == "" {
		query = query.Where("table_id IS NULL")
	} else {
		query = query.Where("table_id = ?", *tableID)
	}

	var item model.DatabaseAssetRef
	err := query.First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *databaseAssetRefRepository) CountActiveByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.DatabaseAssetRef{}).
		Where("organization_id = ? AND asset_id = ? AND status = ?", organizationID, assetID, model.DatabaseAssetRefStatusActive).
		Count(&count).Error
	return count, err
}

func (r *databaseAssetRefRepository) UpdateStatus(ctx context.Context, organizationID string, id uuid.UUID, status string) (*model.DatabaseAssetRef, error) {
	result := r.db.WithContext(ctx).Model(&model.DatabaseAssetRef{}).
		Where("organization_id = ? AND id = ?", organizationID, id).
		Update("status", status)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return r.GetByID(ctx, id)
}
