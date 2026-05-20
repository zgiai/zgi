package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/dataset/model"
	"gorm.io/gorm"
)

type ChunkRepository interface {
	Create(ctx context.Context, chunk *model.DocumentSegment) error
	CreateWithTx(ctx context.Context, tx *gorm.DB, chunk *model.DocumentSegment) error
	GetByID(ctx context.Context, id string) (*model.DocumentSegment, error)
	GetByDocumentID(ctx context.Context, documentID string) ([]*model.DocumentSegment, error)
	GetByDatasetID(ctx context.Context, datasetID string, limit int) ([]*model.DocumentSegment, error)
	Update(ctx context.Context, chunk *model.DocumentSegment) error
	Delete(ctx context.Context, id string) error
	DeleteByDocumentID(ctx context.Context, documentID string) error

	GetByStatus(ctx context.Context, documentID, status string) ([]*model.DocumentSegment, error)
	CountByDocumentID(ctx context.Context, documentID string) (int64, error)
	CountByStatus(ctx context.Context, documentID, status string) (int64, error)
	GetSegmentCounts(ctx context.Context, documentID string) (completed int, total int, err error)

	UpdateIndexingStatus(ctx context.Context, segmentID, status string, indexingTime *time.Time) error
	UpdateVectorData(ctx context.Context, segmentID, indexNodeID string, completedTime *time.Time) error
	UpdateError(ctx context.Context, segmentID, errorMsg string) error

	CreateChildChunk(ctx context.Context, childChunk *model.ChildChunk) error
	UpdateChildChunk(ctx context.Context, childChunk *model.ChildChunk) error
	GetChildChunkByID(ctx context.Context, childChunkID string) (*model.ChildChunk, error)
	GetChildChunksBySegmentID(ctx context.Context, segmentID string) ([]*model.ChildChunk, error)
	DeleteChildChunksBySegmentID(ctx context.Context, segmentID string) error
	GetMaxChildChunkPosition(ctx context.Context, datasetID, documentID, segmentID string) (int, error)

	WithTx(tx *gorm.DB) ChunkRepository
}

type chunkRepository struct {
	db *gorm.DB
}

func NewChunkRepository(db *gorm.DB) ChunkRepository {
	return &chunkRepository{db: db}
}

func (r *chunkRepository) Create(ctx context.Context, chunk *model.DocumentSegment) error {
	if chunk.ID == "" {
		chunk.ID = uuid.New().String()
	}
	chunk.CreatedAt = time.Now()
	chunk.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Create(chunk).Error
}

func (r *chunkRepository) CreateWithTx(ctx context.Context, tx *gorm.DB, chunk *model.DocumentSegment) error {
	if chunk.ID == "" {
		chunk.ID = uuid.New().String()
	}
	chunk.CreatedAt = time.Now()
	chunk.UpdatedAt = time.Now()
	return tx.WithContext(ctx).Create(chunk).Error
}

func (r *chunkRepository) GetByID(ctx context.Context, id string) (*model.DocumentSegment, error) {
	var chunk model.DocumentSegment
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&chunk).Error
	if err != nil {
		return nil, err
	}
	return &chunk, nil
}

func (r *chunkRepository) GetByDocumentID(ctx context.Context, documentID string) ([]*model.DocumentSegment, error) {
	var chunks []*model.DocumentSegment
	err := r.db.WithContext(ctx).
		Where("document_id = ?", documentID).
		Order("position ASC").
		Find(&chunks).Error
	return chunks, err
}

func (r *chunkRepository) GetByDatasetID(ctx context.Context, datasetID string, limit int) ([]*model.DocumentSegment, error) {
	var chunks []*model.DocumentSegment
	query := r.db.WithContext(ctx).Where("dataset_id = ?", datasetID)
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&chunks).Error
	return chunks, err
}

func (r *chunkRepository) Update(ctx context.Context, chunk *model.DocumentSegment) error {
	chunk.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(chunk).Error
}

func (r *chunkRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.DocumentSegment{}, "id = ?", id).Error
}

func (r *chunkRepository) DeleteByDocumentID(ctx context.Context, documentID string) error {
	return r.db.WithContext(ctx).Delete(&model.DocumentSegment{}, "document_id = ?", documentID).Error
}

func (r *chunkRepository) GetByStatus(ctx context.Context, documentID, status string) ([]*model.DocumentSegment, error) {
	var chunks []*model.DocumentSegment
	err := r.db.WithContext(ctx).
		Where("document_id = ? AND indexing_status = ?", documentID, status).
		Find(&chunks).Error
	return chunks, err
}

func (r *chunkRepository) CountByDocumentID(ctx context.Context, documentID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("document_id = ?", documentID).
		Count(&count).Error
	return count, err
}

func (r *chunkRepository) CountByStatus(ctx context.Context, documentID, status string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("document_id = ? AND indexing_status = ?", documentID, status).
		Count(&count).Error
	return count, err
}

func (r *chunkRepository) GetSegmentCounts(ctx context.Context, documentID string) (completed int, total int, err error) {
	var totalCount int64
	err = r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("document_id = ?", documentID).
		Count(&totalCount).Error
	if err != nil {
		return 0, 0, err
	}

	var completedCount int64
	err = r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("document_id = ? AND indexing_status = ?", documentID, "completed").
		Count(&completedCount).Error
	if err != nil {
		return 0, 0, err
	}

	return int(completedCount), int(totalCount), nil
}

func (r *chunkRepository) UpdateIndexingStatus(ctx context.Context, segmentID, status string, indexingTime *time.Time) error {
	updates := map[string]interface{}{
		"indexing_status": status,
		"updated_at":      time.Now(),
	}
	if indexingTime != nil {
		updates["indexing_at"] = indexingTime
	}
	return r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("id = ?", segmentID).
		Updates(updates).Error
}

func (r *chunkRepository) UpdateVectorData(ctx context.Context, segmentID, indexNodeID string, completedTime *time.Time) error {
	updates := map[string]interface{}{
		"index_node_id": indexNodeID,
		"updated_at":    time.Now(),
	}
	if completedTime != nil {
		updates["completed_at"] = completedTime
	}
	return r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("id = ?", segmentID).
		Updates(updates).Error
}

func (r *chunkRepository) UpdateError(ctx context.Context, segmentID, errorMsg string) error {
	return r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("id = ?", segmentID).
		Updates(map[string]interface{}{
			"error":      errorMsg,
			"updated_at": time.Now(),
		}).Error
}

func (r *chunkRepository) CreateChildChunk(ctx context.Context, childChunk *model.ChildChunk) error {
	if childChunk.ID == "" {
		childChunk.ID = uuid.New().String()
	}
	childChunk.CreatedAt = time.Now()
	childChunk.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Create(childChunk).Error
}

func (r *chunkRepository) UpdateChildChunk(ctx context.Context, childChunk *model.ChildChunk) error {
	childChunk.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(childChunk).Error
}

func (r *chunkRepository) GetChildChunkByID(ctx context.Context, childChunkID string) (*model.ChildChunk, error) {
	var childChunk model.ChildChunk
	err := r.db.WithContext(ctx).Where("id = ?", childChunkID).First(&childChunk).Error
	if err != nil {
		return nil, err
	}
	return &childChunk, nil
}

func (r *chunkRepository) GetChildChunksBySegmentID(ctx context.Context, segmentID string) ([]*model.ChildChunk, error) {
	var childChunks []*model.ChildChunk
	err := r.db.WithContext(ctx).
		Where("segment_id = ?", segmentID).
		Order("position ASC").
		Find(&childChunks).Error
	return childChunks, err
}

func (r *chunkRepository) DeleteChildChunksBySegmentID(ctx context.Context, segmentID string) error {
	return r.db.WithContext(ctx).Delete(&model.ChildChunk{}, "segment_id = ?", segmentID).Error
}

func (r *chunkRepository) GetMaxChildChunkPosition(ctx context.Context, datasetID, documentID, segmentID string) (int, error) {
	var maxPosition int64
	err := r.db.WithContext(ctx).Model(&model.ChildChunk{}).
		Where("dataset_id = ? AND document_id = ? AND segment_id = ?", datasetID, documentID, segmentID).
		Select("COALESCE(MAX(position), 0)").
		Scan(&maxPosition).Error
	return int(maxPosition), err
}

func (r *chunkRepository) WithTx(tx *gorm.DB) ChunkRepository {
	return NewChunkRepository(tx)
}
