package repository

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"gorm.io/gorm"
)

// EntityMentionRepository handles CRUD for kb_entity_mentions
type EntityMentionRepository struct {
	db *gorm.DB
}

// NewEntityMentionRepository creates a new repository instance
func NewEntityMentionRepository(db *gorm.DB) *EntityMentionRepository {
	return &EntityMentionRepository{db: db}
}

// CreateBatch inserts multiple entity mentions in a single transaction
func (r *EntityMentionRepository) CreateBatch(ctx context.Context, mentions []*model.EntityMention) error {
	if len(mentions) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(mentions, 100).Error
}

// FindPendingByKBID retrieves all pending mentions for a specific knowledge base with a limit
func (r *EntityMentionRepository) FindPendingByKBID(ctx context.Context, kbID uuid.UUID, limit int) ([]*model.EntityMention, error) {
	var results []*model.EntityMention
	query := r.db.WithContext(ctx).
		Where("kb_id = ? AND status = ?", kbID, "pending")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&results).Error
	return results, err
}

// FindBySegmentID retrieves all mentions for a specific segment
func (r *EntityMentionRepository) FindBySegmentID(ctx context.Context, segmentID uuid.UUID) ([]*model.EntityMention, error) {
	var results []*model.EntityMention
	err := r.db.WithContext(ctx).Where("segment_id = ?", segmentID).Find(&results).Error
	return results, err
}

// UpdateStatus updates the status and optionally links to an entity
func (r *EntityMentionRepository) UpdateStatus(ctx context.Context, mentionID uuid.UUID, status string, entityID *uuid.UUID) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if entityID != nil {
		updates["entity_id"] = entityID
	}
	return r.db.WithContext(ctx).Model(&model.EntityMention{}).Where("id = ?", mentionID).Updates(updates).Error
}

// UpdateBatchStatus updates status for multiple mentions
func (r *EntityMentionRepository) UpdateBatchStatus(ctx context.Context, mentionIDs []uuid.UUID, status string, entityID *uuid.UUID) error {
	if len(mentionIDs) == 0 {
		return nil
	}
	updates := map[string]interface{}{
		"status": status,
	}
	if entityID != nil {
		updates["entity_id"] = entityID
	}
	return r.db.WithContext(ctx).Model(&model.EntityMention{}).Where("id IN ?", mentionIDs).Updates(updates).Error
}

// DeleteByDocumentSegments deletes all entity mentions for segments belonging to a document
func (r *EntityMentionRepository) DeleteByDocumentSegments(ctx context.Context, documentID uuid.UUID) error {
	// Delete entity mentions where segment_id belongs to the given document
	return r.db.WithContext(ctx).
		Where("segment_id IN (SELECT id FROM document_segments WHERE document_id = ?)", documentID).
		Delete(&model.EntityMention{}).Error
}

// DeleteByKBID deletes all entity mentions for a knowledge base
func (r *EntityMentionRepository) DeleteByKBID(ctx context.Context, kbID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("kb_id = ?", kbID).Delete(&model.EntityMention{}).Error
}

// FindAlignedSync retrieves mentions that are aligned but not yet synced (status = 'aligned')
func (r *EntityMentionRepository) FindAlignedSync(ctx context.Context, kbID uuid.UUID, limit int) ([]*model.EntityMention, error) {
	var results []*model.EntityMention
	query := r.db.WithContext(ctx).
		Where("kb_id = ? AND status = ?", kbID, "aligned")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&results).Error
	return results, err
}

// FindSegmentsByEntityNames finds segment IDs that contain mentions of the given entity names
// Used for graph-enhanced retrieval to find chunks related to extracted entities
func (r *EntityMentionRepository) FindSegmentsByEntityNames(ctx context.Context, kbID uuid.UUID, entityNames []string) ([]uuid.UUID, error) {
	if len(entityNames) == 0 {
		return nil, nil
	}

	// Convert entity names to lowercase for case-insensitive matching
	lowerNames := make([]string, len(entityNames))
	for i, name := range entityNames {
		lowerNames[i] = strings.ToLower(name)
	}

	var segmentIDs []uuid.UUID
	err := r.db.WithContext(ctx).
		Model(&model.EntityMention{}).
		Select("DISTINCT segment_id").
		Where("kb_id = ? AND LOWER(raw_name) IN ? AND status IN ?", kbID, lowerNames, []string{"aligned", "synced"}).
		Pluck("segment_id", &segmentIDs).Error
	return segmentIDs, err
}

// SoftDeleteByDocumentSegments performs a soft delete on entity mentions for segments of a document
func (r *EntityMentionRepository) SoftDeleteByDocumentSegments(ctx context.Context, documentID uuid.UUID) error {
	now := time.Now()
	updates := map[string]interface{}{
		"is_deleted": true,
		"deleted_at": &now,
	}
	// Update entity mentions where segment_id belongs to the given document
	return r.db.WithContext(ctx).Model(&model.EntityMention{}).
		Where("segment_id IN (SELECT id FROM document_segments WHERE document_id = ?)", documentID).
		Updates(updates).Error
}

// FindByDocumentSegments retrieves all entity mentions for segments belonging to a document
func (r *EntityMentionRepository) FindByDocumentSegments(ctx context.Context, documentID uuid.UUID) ([]*model.EntityMention, error) {
	var results []*model.EntityMention
	err := r.db.WithContext(ctx).
		Where("segment_id IN (SELECT id FROM document_segments WHERE document_id = ?)", documentID).
		Find(&results).Error
	return results, err
}

// FindMentionsByEntityNames retrieves full entity mention records matching specific names in a KB
func (r *EntityMentionRepository) FindMentionsByEntityNames(ctx context.Context, kbID uuid.UUID, entityNames []string) ([]*model.EntityMention, error) {
	if len(entityNames) == 0 {
		return nil, nil
	}

	lowerNames := make([]string, len(entityNames))
	for i, name := range entityNames {
		lowerNames[i] = strings.ToLower(name)
	}

	var results []*model.EntityMention
	err := r.db.WithContext(ctx).
		Where("kb_id = ? AND LOWER(raw_name) IN ?", kbID, lowerNames).
		Find(&results).Error
	return results, err
}

// FindMentionsByEntityIDs retrieves full entity mention records matching specific canonical entity IDs in a KB
func (r *EntityMentionRepository) FindMentionsByEntityIDs(ctx context.Context, kbID uuid.UUID, entityIDs []uuid.UUID) ([]*model.EntityMention, error) {
	if len(entityIDs) == 0 {
		return nil, nil
	}

	var results []*model.EntityMention
	err := r.db.WithContext(ctx).
		Where("kb_id = ? AND entity_id IN ?", kbID, entityIDs).
		Find(&results).Error
	return results, err
}

// GetByKBID retrieves all entity mentions for a knowledge base
func (r *EntityMentionRepository) GetByKBID(ctx context.Context, kbID string) ([]*model.EntityMention, error) {
	var results []*model.EntityMention
	err := r.db.WithContext(ctx).
		Where("kb_id = ?", kbID).
		Find(&results).Error
	return results, err
}
