package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"gorm.io/gorm"
)

type KnowledgeBaseAssetRefListFilter struct {
	OrganizationID string
	DatasetID      string
	AssetID        uuid.UUID
	VersionID      uuid.UUID
	Status         string
	Limit          int
	Offset         int
}

type KnowledgeBaseAssetRefRepository interface {
	Create(ctx context.Context, item *model.KnowledgeBaseAssetRef) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.KnowledgeBaseAssetRef, error)
	List(ctx context.Context, filter KnowledgeBaseAssetRefListFilter) ([]*model.KnowledgeBaseAssetRef, int64, error)
	FindActive(ctx context.Context, organizationID string, datasetID string, assetID uuid.UUID, versionID uuid.UUID) (*model.KnowledgeBaseAssetRef, error)
	CountActiveByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) (int64, error)
	UpdateStatus(ctx context.Context, organizationID string, id uuid.UUID, status string) (*model.KnowledgeBaseAssetRef, error)
}

type knowledgeBaseAssetRefRepository struct {
	db *gorm.DB
}

func NewKnowledgeBaseAssetRefRepository(db *gorm.DB) KnowledgeBaseAssetRefRepository {
	return &knowledgeBaseAssetRefRepository{db: db}
}

func (r *knowledgeBaseAssetRefRepository) Create(ctx context.Context, item *model.KnowledgeBaseAssetRef) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *knowledgeBaseAssetRefRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.KnowledgeBaseAssetRef, error) {
	var item model.KnowledgeBaseAssetRef
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *knowledgeBaseAssetRefRepository) List(ctx context.Context, filter KnowledgeBaseAssetRefListFilter) ([]*model.KnowledgeBaseAssetRef, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.KnowledgeBaseAssetRef{}).
		Where("organization_id = ?", filter.OrganizationID)
	if filter.DatasetID != "" {
		query = query.Where("dataset_id = ?", filter.DatasetID)
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

	var items []*model.KnowledgeBaseAssetRef
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *knowledgeBaseAssetRefRepository) FindActive(ctx context.Context, organizationID string, datasetID string, assetID uuid.UUID, versionID uuid.UUID) (*model.KnowledgeBaseAssetRef, error) {
	var item model.KnowledgeBaseAssetRef
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND dataset_id = ? AND asset_id = ? AND version_id = ? AND status = ?",
			organizationID, datasetID, assetID, versionID, model.KnowledgeBaseAssetRefStatusActive).
		First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *knowledgeBaseAssetRefRepository) CountActiveByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.KnowledgeBaseAssetRef{}).
		Where("organization_id = ? AND asset_id = ? AND status = ?", organizationID, assetID, model.KnowledgeBaseAssetRefStatusActive).
		Count(&count).Error
	return count, err
}

func (r *knowledgeBaseAssetRefRepository) UpdateStatus(ctx context.Context, organizationID string, id uuid.UUID, status string) (*model.KnowledgeBaseAssetRef, error) {
	result := r.db.WithContext(ctx).Model(&model.KnowledgeBaseAssetRef{}).
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
