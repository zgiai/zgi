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

type DocumentChunkEmbeddingListFilter struct {
	OrganizationID    string
	AssetID           uuid.UUID
	ChunkID           uuid.UUID
	GenerationNo      *int64
	EmbeddingProvider string
	EmbeddingModel    string
	Status            string
	Limit             int
	Offset            int
}

type DocumentChunkEmbeddingModelTarget struct {
	EmbeddingProvider string
	EmbeddingModel    string
}

type DocumentChunkEmbeddingRepository interface {
	Create(ctx context.Context, item *model.DocumentChunkEmbedding) error
	Upsert(ctx context.Context, item *model.DocumentChunkEmbedding) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.DocumentChunkEmbedding, error)
	FindByChunkModel(ctx context.Context, chunkID uuid.UUID, provider string, embeddingModel string) (*model.DocumentChunkEmbedding, error)
	List(ctx context.Context, filter DocumentChunkEmbeddingListFilter) ([]*model.DocumentChunkEmbedding, int64, error)
	ListModelTargetsByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) ([]DocumentChunkEmbeddingModelTarget, error)
	ListModelTargetsByChunkIDs(ctx context.Context, organizationID string, chunkIDs []uuid.UUID) ([]DocumentChunkEmbeddingModelTarget, error)
	CountReadyByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) (int64, error)
	CountReadyByAssetGenerationModel(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64, provider string, embeddingModel string) (int64, error)
	DeleteByChunkID(ctx context.Context, organizationID string, chunkID uuid.UUID) error
	DeleteByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) error
	DeleteByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) error
}

type documentChunkEmbeddingRepository struct {
	db *gorm.DB
}

func NewDocumentChunkEmbeddingRepository(db *gorm.DB) DocumentChunkEmbeddingRepository {
	return &documentChunkEmbeddingRepository{db: db}
}

func (r *documentChunkEmbeddingRepository) Create(ctx context.Context, item *model.DocumentChunkEmbedding) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *documentChunkEmbeddingRepository) Upsert(ctx context.Context, item *model.DocumentChunkEmbedding) error {
	if item == nil {
		return nil
	}
	now := time.Now()
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = now
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.DocumentChunkEmbedding
		err := tx.Where("chunk_id = ?", item.ChunkID).
			Where("embedding_provider = ?", item.EmbeddingProvider).
			Where("embedding_model = ?", item.EmbeddingModel).
			Where("status <> ?", model.DocumentChunkEmbeddingStatusDeleted).
			Where("deleted_at IS NULL").
			First(&existing).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return tx.Create(item).Error
			}
			return err
		}
		metadataJSON, err := json.Marshal(item.MetadataJSON)
		if err != nil {
			return err
		}
		item.ID = existing.ID
		updates := map[string]any{
			"organization_id":     item.OrganizationID,
			"workspace_id":        item.WorkspaceID,
			"asset_id":            item.AssetID,
			"processing_run_id":   item.ProcessingRunID,
			"generation_no":       item.GenerationNo,
			"embedding_dimension": item.EmbeddingDimension,
			"embedding_vector":    item.EmbeddingVector,
			"content_hash":        item.ContentHash,
			"status":              item.Status,
			"metadata_json":       datatypes.JSON(metadataJSON),
			"updated_at":          item.UpdatedAt,
			"deleted_at":          item.DeletedAt,
		}
		return tx.Model(&model.DocumentChunkEmbedding{}).
			Where("id = ?", existing.ID).
			Updates(updates).Error
	})
}

func (r *documentChunkEmbeddingRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.DocumentChunkEmbedding, error) {
	var item model.DocumentChunkEmbedding
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *documentChunkEmbeddingRepository) FindByChunkModel(ctx context.Context, chunkID uuid.UUID, provider string, embeddingModel string) (*model.DocumentChunkEmbedding, error) {
	var item model.DocumentChunkEmbedding
	err := r.db.WithContext(ctx).
		Where("chunk_id = ?", chunkID).
		Where("embedding_provider = ?", provider).
		Where("embedding_model = ?", embeddingModel).
		Where("deleted_at IS NULL").
		First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *documentChunkEmbeddingRepository) List(ctx context.Context, filter DocumentChunkEmbeddingListFilter) ([]*model.DocumentChunkEmbedding, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.DocumentChunkEmbedding{}).
		Where("organization_id = ?", filter.OrganizationID)
	if filter.AssetID != uuid.Nil {
		query = query.Where("asset_id = ?", filter.AssetID)
	}
	if filter.ChunkID != uuid.Nil {
		query = query.Where("chunk_id = ?", filter.ChunkID)
	}
	if filter.GenerationNo != nil {
		query = query.Where("generation_no = ?", *filter.GenerationNo)
	}
	if filter.EmbeddingProvider != "" {
		query = query.Where("embedding_provider = ?", filter.EmbeddingProvider)
	}
	if filter.EmbeddingModel != "" {
		query = query.Where("embedding_model = ?", filter.EmbeddingModel)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
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

	var items []*model.DocumentChunkEmbedding
	if err := query.Order("created_at ASC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *documentChunkEmbeddingRepository) ListModelTargetsByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) ([]DocumentChunkEmbeddingModelTarget, error) {
	if organizationID == "" || assetID == uuid.Nil {
		return []DocumentChunkEmbeddingModelTarget{}, nil
	}
	var items []DocumentChunkEmbeddingModelTarget
	err := r.db.WithContext(ctx).Model(&model.DocumentChunkEmbedding{}).
		Select("embedding_provider, embedding_model").
		Where("organization_id = ?", organizationID).
		Where("asset_id = ?", assetID).
		Where("embedding_model <> ''").
		Where("deleted_at IS NULL").
		Group("embedding_provider, embedding_model").
		Order("MIN(created_at) ASC").
		Find(&items).Error
	return items, err
}

func (r *documentChunkEmbeddingRepository) ListModelTargetsByChunkIDs(ctx context.Context, organizationID string, chunkIDs []uuid.UUID) ([]DocumentChunkEmbeddingModelTarget, error) {
	if organizationID == "" || len(chunkIDs) == 0 {
		return []DocumentChunkEmbeddingModelTarget{}, nil
	}
	var items []DocumentChunkEmbeddingModelTarget
	err := r.db.WithContext(ctx).Model(&model.DocumentChunkEmbedding{}).
		Select("embedding_provider, embedding_model").
		Where("organization_id = ?", organizationID).
		Where("chunk_id IN ?", chunkIDs).
		Where("embedding_model <> ''").
		Where("deleted_at IS NULL").
		Group("embedding_provider, embedding_model").
		Order("MIN(created_at) ASC").
		Find(&items).Error
	return items, err
}

func (r *documentChunkEmbeddingRepository) CountReadyByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) (int64, error) {
	if organizationID == "" || assetID == uuid.Nil {
		return 0, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.DocumentChunkEmbedding{}).
		Where("organization_id = ?", organizationID).
		Where("asset_id = ?", assetID).
		Where("generation_no = ?", generationNo).
		Where("status = ?", model.DocumentChunkEmbeddingStatusReady).
		Where("deleted_at IS NULL").
		Count(&count).Error
	return count, err
}

func (r *documentChunkEmbeddingRepository) CountReadyByAssetGenerationModel(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64, provider string, embeddingModel string) (int64, error) {
	if organizationID == "" || assetID == uuid.Nil || generationNo <= 0 || embeddingModel == "" {
		return 0, nil
	}
	query := r.db.WithContext(ctx).Model(&model.DocumentChunkEmbedding{}).
		Where("organization_id = ?", organizationID).
		Where("asset_id = ?", assetID).
		Where("generation_no = ?", generationNo).
		Where("embedding_model = ?", embeddingModel).
		Where("status = ?", model.DocumentChunkEmbeddingStatusReady).
		Where("deleted_at IS NULL")
	if provider != "" {
		query = query.Where("embedding_provider = ?", provider)
	}
	var count int64
	err := query.Count(&count).Error
	return count, err
}

func (r *documentChunkEmbeddingRepository) DeleteByChunkID(ctx context.Context, organizationID string, chunkID uuid.UUID) error {
	if organizationID == "" || chunkID == uuid.Nil {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Where("chunk_id = ?", chunkID).
		Delete(&model.DocumentChunkEmbedding{}).Error
}

func (r *documentChunkEmbeddingRepository) DeleteByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) error {
	if organizationID == "" || assetID == uuid.Nil {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Where("asset_id = ?", assetID).
		Delete(&model.DocumentChunkEmbedding{}).Error
}

func (r *documentChunkEmbeddingRepository) DeleteByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) error {
	if organizationID == "" || assetID == uuid.Nil || generationNo <= 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Where("asset_id = ?", assetID).
		Where("generation_no = ?", generationNo).
		Delete(&model.DocumentChunkEmbedding{}).Error
}
