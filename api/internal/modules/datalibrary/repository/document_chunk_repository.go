package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type DocumentChunkListFilter struct {
	OrganizationID string
	AssetID        uuid.UUID
	GenerationNo   *int64
	ParentChunkID  *uuid.UUID
	ChunkTypes     []string
	Enabled        *bool
	Status         string
	Search         string
	Limit          int
	Offset         int
}

type DocumentChunkPatch struct {
	OrganizationID string
	Content        *string
	ContentHash    *string
	Enabled        *bool
	Status         *string
	UpdatedBy      string
	MetadataJSON   map[string]any
}

type DocumentChunkRepository interface {
	Create(ctx context.Context, item *model.DocumentChunk) error
	CreateBatch(ctx context.Context, items []*model.DocumentChunk) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.DocumentChunk, error)
	ListByIDs(ctx context.Context, organizationID string, ids []uuid.UUID) ([]*model.DocumentChunk, error)
	List(ctx context.Context, filter DocumentChunkListFilter) ([]*model.DocumentChunk, int64, error)
	CountByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) (int64, error)
	CountByAssetGenerationAndTypes(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64, chunkTypes []string) (int64, error)
	DeleteByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) error
	DeleteByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) error
	DeleteChildrenByParent(ctx context.Context, organizationID string, parentChunkID uuid.UUID) error
	Update(ctx context.Context, id uuid.UUID, patch DocumentChunkPatch) (*model.DocumentChunk, error)
}

type documentChunkRepository struct {
	db *gorm.DB
}

const documentChunkCreateBatchSize = 500

func NewDocumentChunkRepository(db *gorm.DB) DocumentChunkRepository {
	return &documentChunkRepository{db: db}
}

func (r *documentChunkRepository) Create(ctx context.Context, item *model.DocumentChunk) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *documentChunkRepository) CreateBatch(ctx context.Context, items []*model.DocumentChunk) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(items, documentChunkCreateBatchSize).Error
}

func (r *documentChunkRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.DocumentChunk, error) {
	var item model.DocumentChunk
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *documentChunkRepository) ListByIDs(ctx context.Context, organizationID string, ids []uuid.UUID) ([]*model.DocumentChunk, error) {
	if organizationID == "" || len(ids) == 0 {
		return []*model.DocumentChunk{}, nil
	}
	var items []*model.DocumentChunk
	if err := r.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Where("id IN ?", ids).
		Where("deleted_at IS NULL").
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *documentChunkRepository) List(ctx context.Context, filter DocumentChunkListFilter) ([]*model.DocumentChunk, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.DocumentChunk{}).
		Where("organization_id = ?", filter.OrganizationID)
	if filter.AssetID != uuid.Nil {
		query = query.Where("asset_id = ?", filter.AssetID)
	}
	if filter.GenerationNo != nil {
		query = query.Where("generation_no = ?", *filter.GenerationNo)
	}
	if filter.ParentChunkID != nil {
		query = query.Where("parent_chunk_id = ?", *filter.ParentChunkID)
	}
	if len(filter.ChunkTypes) > 0 {
		query = query.Where("chunk_type IN ?", filter.ChunkTypes)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Search != "" {
		query = query.Where("content ILIKE ?", "%"+filter.Search+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var items []*model.DocumentChunk
	if err := query.Order("position ASC, created_at ASC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *documentChunkRepository) CountByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) (int64, error) {
	if organizationID == "" || assetID == uuid.Nil {
		return 0, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.DocumentChunk{}).
		Where("organization_id = ?", organizationID).
		Where("asset_id = ?", assetID).
		Where("generation_no = ?", generationNo).
		Where("deleted_at IS NULL").
		Count(&count).Error
	return count, err
}

func (r *documentChunkRepository) CountByAssetGenerationAndTypes(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64, chunkTypes []string) (int64, error) {
	if organizationID == "" || assetID == uuid.Nil {
		return 0, nil
	}
	query := r.db.WithContext(ctx).Model(&model.DocumentChunk{}).
		Where("organization_id = ?", organizationID).
		Where("asset_id = ?", assetID).
		Where("generation_no = ?", generationNo).
		Where("deleted_at IS NULL")
	if len(chunkTypes) > 0 {
		query = query.Where("chunk_type IN ?", chunkTypes)
	}
	var count int64
	err := query.Count(&count).Error
	return count, err
}

func (r *documentChunkRepository) DeleteByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) error {
	if organizationID == "" || assetID == uuid.Nil {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Where("asset_id = ?", assetID).
		Delete(&model.DocumentChunk{}).Error
}

func (r *documentChunkRepository) DeleteByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) error {
	if organizationID == "" || assetID == uuid.Nil || generationNo <= 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Where("asset_id = ?", assetID).
		Where("generation_no = ?", generationNo).
		Delete(&model.DocumentChunk{}).Error
}

func (r *documentChunkRepository) DeleteChildrenByParent(ctx context.Context, organizationID string, parentChunkID uuid.UUID) error {
	if organizationID == "" || parentChunkID == uuid.Nil {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Where("parent_chunk_id = ?", parentChunkID).
		Delete(&model.DocumentChunk{}).Error
}

func (r *documentChunkRepository) Update(ctx context.Context, id uuid.UUID, patch DocumentChunkPatch) (*model.DocumentChunk, error) {
	if id == uuid.Nil {
		return nil, nil
	}
	updates := map[string]any{
		"updated_at": time.Now(),
	}
	if patch.Content != nil {
		updates["content"] = *patch.Content
	}
	if patch.ContentHash != nil {
		updates["content_hash"] = *patch.ContentHash
	}
	if patch.Enabled != nil {
		updates["enabled"] = *patch.Enabled
	}
	if patch.Status != nil {
		updates["status"] = *patch.Status
	}
	if patch.UpdatedBy != "" {
		updates["updated_by"] = patch.UpdatedBy
	}
	if patch.MetadataJSON != nil {
		metadataJSON, err := json.Marshal(patch.MetadataJSON)
		if err != nil {
			return nil, err
		}
		updates["metadata_json"] = datatypes.JSON(metadataJSON)
	}

	query := r.db.WithContext(ctx).Model(&model.DocumentChunk{}).
		Where("id = ?", id).
		Where("deleted_at IS NULL")
	if patch.OrganizationID != "" {
		query = query.Where("organization_id = ?", patch.OrganizationID)
	}
	result := query.Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	return r.GetByID(ctx, id)
}
