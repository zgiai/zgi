package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"gorm.io/gorm"
)

type ExtractionArtifactListFilter struct {
	OrganizationID  string
	AssetID         uuid.UUID
	VersionID       uuid.UUID
	ParseArtifactID uuid.UUID
	DataSourceID    string
	TableID         string
	Status          string
	Limit           int
	Offset          int
}

type ExtractionArtifactRepository interface {
	Create(ctx context.Context, item *model.ExtractionArtifact) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.ExtractionArtifact, error)
	List(ctx context.Context, filter ExtractionArtifactListFilter) ([]*model.ExtractionArtifact, int64, error)
	LatestReadyByVersionID(ctx context.Context, organizationID string, versionID uuid.UUID) (*model.ExtractionArtifact, error)
}

type extractionArtifactRepository struct {
	db *gorm.DB
}

func NewExtractionArtifactRepository(db *gorm.DB) ExtractionArtifactRepository {
	return &extractionArtifactRepository{db: db}
}

func (r *extractionArtifactRepository) Create(ctx context.Context, item *model.ExtractionArtifact) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *extractionArtifactRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.ExtractionArtifact, error) {
	var item model.ExtractionArtifact
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *extractionArtifactRepository) List(ctx context.Context, filter ExtractionArtifactListFilter) ([]*model.ExtractionArtifact, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.ExtractionArtifact{}).
		Where("organization_id = ?", filter.OrganizationID)
	if filter.AssetID != uuid.Nil {
		query = query.Where("asset_id = ?", filter.AssetID)
	}
	if filter.VersionID != uuid.Nil {
		query = query.Where("version_id = ?", filter.VersionID)
	}
	if filter.ParseArtifactID != uuid.Nil {
		query = query.Where("parse_artifact_id = ?", filter.ParseArtifactID)
	}
	if filter.DataSourceID != "" {
		query = query.Where("data_source_id = ?", filter.DataSourceID)
	}
	if filter.TableID != "" {
		query = query.Where("table_id = ?", filter.TableID)
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

	var items []*model.ExtractionArtifact
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *extractionArtifactRepository) LatestReadyByVersionID(ctx context.Context, organizationID string, versionID uuid.UUID) (*model.ExtractionArtifact, error) {
	var item model.ExtractionArtifact
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND version_id = ? AND status = ?", organizationID, versionID, model.ExtractionArtifactStatusReady).
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
