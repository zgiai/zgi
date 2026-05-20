package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow/model"
	"gorm.io/gorm"
)

// TripleMentionRepository handles CRUD for kb_triple_mentions
type TripleMentionRepository struct {
	db *gorm.DB
}

// NewTripleMentionRepository creates a new repository instance
func NewTripleMentionRepository(db *gorm.DB) *TripleMentionRepository {
	return &TripleMentionRepository{db: db}
}

// CreateBatch inserts multiple triple mentions in a single transaction
func (r *TripleMentionRepository) CreateBatch(ctx context.Context, triples []*model.TripleMention) error {
	if len(triples) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(triples, 100).Error
}

// FindPendingByKBID retrieves all pending triple mentions for a specific knowledge base with a limit
func (r *TripleMentionRepository) FindPendingByKBID(ctx context.Context, kbID uuid.UUID, limit int) ([]*model.TripleMention, error) {
	var results []*model.TripleMention
	query := r.db.WithContext(ctx).
		Where("kb_id = ? AND status = ?", kbID, "pending")
	
	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&results).Error
	return results, err
}

// FindBySegmentID retrieves all triple mentions for a specific segment
func (r *TripleMentionRepository) FindBySegmentID(ctx context.Context, segmentID uuid.UUID) ([]*model.TripleMention, error) {
	var results []*model.TripleMention
	err := r.db.WithContext(ctx).Where("segment_id = ?", segmentID).Find(&results).Error
	return results, err
}

// UpdateStatus updates the status and entity links
func (r *TripleMentionRepository) UpdateStatus(ctx context.Context, tripleID uuid.UUID, status string, headEntityID, tailEntityID *uuid.UUID) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if headEntityID != nil {
		updates["head_entity_id"] = headEntityID
	}
	if tailEntityID != nil {
		updates["tail_entity_id"] = tailEntityID
	}
	return r.db.WithContext(ctx).Model(&model.TripleMention{}).Where("id = ?", tripleID).Updates(updates).Error
}

// DeleteByDocumentSegments deletes all triple mentions for segments belonging to a document
func (r *TripleMentionRepository) DeleteByDocumentSegments(ctx context.Context, documentID uuid.UUID) error {
	// Delete triple mentions where segment_id belongs to the given document
	return r.db.WithContext(ctx).
		Where("segment_id IN (SELECT id FROM document_segments WHERE document_id = ?)", documentID).
		Delete(&model.TripleMention{}).Error
}

// SoftDeleteByDocumentSegments performs a soft delete on triple mentions for segments of a document
func (r *TripleMentionRepository) SoftDeleteByDocumentSegments(ctx context.Context, documentID uuid.UUID) error {
	now := time.Now()
	updates := map[string]interface{}{
		"is_deleted": true,
		"deleted_at": &now,
	}
	// Update triple mentions where segment_id belongs to the given document
	return r.db.WithContext(ctx).Model(&model.TripleMention{}).
		Where("segment_id IN (SELECT id FROM document_segments WHERE document_id = ?)", documentID).
		Updates(updates).Error
}

// DeleteByKBID deletes all triple mentions for a knowledge base
func (r *TripleMentionRepository) DeleteByKBID(ctx context.Context, kbID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("kb_id = ?", kbID).Delete(&model.TripleMention{}).Error
}
