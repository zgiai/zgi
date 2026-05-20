package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"gorm.io/gorm"
)

type VectorArtifactListFilter struct {
	OrganizationID     string
	AssetID            uuid.UUID
	VersionID          uuid.UUID
	ChunkArtifactSetID uuid.UUID
	Status             string
	Limit              int
	Offset             int
}

type VectorArtifactRepository interface {
	Create(ctx context.Context, item *model.VectorArtifact) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.VectorArtifact, error)
	List(ctx context.Context, filter VectorArtifactListFilter) ([]*model.VectorArtifact, int64, error)
	LatestReadyByVersionID(ctx context.Context, organizationID string, versionID uuid.UUID) (*model.VectorArtifact, error)
}

type vectorArtifactRepository struct {
	db *gorm.DB
}

func NewVectorArtifactRepository(db *gorm.DB) VectorArtifactRepository {
	return &vectorArtifactRepository{db: db}
}

func (r *vectorArtifactRepository) Create(ctx context.Context, item *model.VectorArtifact) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *vectorArtifactRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.VectorArtifact, error) {
	var item model.VectorArtifact
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *vectorArtifactRepository) List(ctx context.Context, filter VectorArtifactListFilter) ([]*model.VectorArtifact, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.VectorArtifact{}).
		Where("organization_id = ?", filter.OrganizationID)
	if filter.AssetID != uuid.Nil {
		query = query.Where("asset_id = ?", filter.AssetID)
	}
	if filter.VersionID != uuid.Nil {
		query = query.Where("version_id = ?", filter.VersionID)
	}
	if filter.ChunkArtifactSetID != uuid.Nil {
		query = query.Where("chunk_artifact_set_id = ?", filter.ChunkArtifactSetID)
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

	var items []*model.VectorArtifact
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *vectorArtifactRepository) LatestReadyByVersionID(ctx context.Context, organizationID string, versionID uuid.UUID) (*model.VectorArtifact, error) {
	var item model.VectorArtifact
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND version_id = ? AND status = ?", organizationID, versionID, model.VectorArtifactStatusReady).
		Order("created_at DESC").
		First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}
