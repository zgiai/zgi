package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"gorm.io/gorm"
)

type RelationshipRepository struct {
	db *gorm.DB
}

func NewRelationshipRepository(db *gorm.DB) *RelationshipRepository {
	return &RelationshipRepository{db: db}
}

// Create inserts a new relationship
func (r *RelationshipRepository) Create(ctx context.Context, rel *model.Relationship) error {
	return r.db.WithContext(ctx).Create(rel).Error
}

// CreateBatch inserts multiple relationships in a single transaction
func (r *RelationshipRepository) CreateBatch(ctx context.Context, relations []*model.Relationship) error {
	if len(relations) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(relations, 100).Error
}

// FindByKBID retrieves all relationships for a specific knowledge base
func (r *RelationshipRepository) FindByKBID(ctx context.Context, kbID uuid.UUID) ([]*model.Relationship, error) {
	var results []*model.Relationship
	err := r.db.WithContext(ctx).Where("kb_id = ?", kbID).Find(&results).Error
	return results, err
}

// FindPendingSync retrieves relationships that need to be synced to Neo4j
func (r *RelationshipRepository) FindPendingSync(ctx context.Context, kbID uuid.UUID) ([]*model.Relationship, error) {
	var results []*model.Relationship
	err := r.db.WithContext(ctx).
		Where("kb_id = ? AND graph_state = ?", kbID, "pending").
		Find(&results).Error
	return results, err
}

// FindExisting checks if a relationship already exists
func (r *RelationshipRepository) FindExisting(ctx context.Context, kbID, headID, tailID uuid.UUID, relationType string) (*model.Relationship, error) {
	var rel model.Relationship
	err := r.db.WithContext(ctx).
		Where("kb_id = ? AND head_entity_id = ? AND tail_entity_id = ? AND relation_type = ?", kbID, headID, tailID, relationType).
		First(&rel).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &rel, err
}

// UpdateGraphState updates the graph sync state
func (r *RelationshipRepository) UpdateGraphState(ctx context.Context, relID uuid.UUID, state string) error {
	return r.db.WithContext(ctx).Model(&model.Relationship{}).Where("id = ?", relID).Update("graph_state", state).Error
}

// UpdateGraphStateBatch updates the graph sync state for multiple relationships
func (r *RelationshipRepository) UpdateGraphStateBatch(ctx context.Context, relIDs []uuid.UUID, state string) error {
	if len(relIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.Relationship{}).Where("id IN ?", relIDs).Update("graph_state", state).Error
}

// IncrementWeight increments the weight for a relationship
func (r *RelationshipRepository) IncrementWeight(ctx context.Context, relID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&model.Relationship{}).
		Where("id = ?", relID).
		UpdateColumn("weight", gorm.Expr("weight + 1")).Error
}

// SoftDeleteByEntityID performs a soft delete on relationships where the entity is head or tail
func (r *RelationshipRepository) SoftDeleteByEntityID(ctx context.Context, entityID uuid.UUID) error {
	now := time.Now()
	updates := map[string]interface{}{
		"is_deleted":  true,
		"deleted_at":  &now,
		"graph_state": "pending_delete",
	}
	return r.db.WithContext(ctx).Model(&model.Relationship{}).
		Where("head_entity_id = ? OR tail_entity_id = ?", entityID, entityID).
		Updates(updates).Error
}

// FindPendingDelete retrieves relationships that are marked for deletion
func (r *RelationshipRepository) FindPendingDelete(ctx context.Context, limit int) ([]*model.Relationship, error) {
	var results []*model.Relationship
	err := r.db.WithContext(ctx).
		Where("graph_state = ?", "pending_delete").
		Limit(limit).
		Find(&results).Error
	return results, err
}

// FindByEntityIDs finds relationships where both entities are in the provided list
func (r *RelationshipRepository) FindByEntityIDs(ctx context.Context, kbID uuid.UUID, entityIDs []uuid.UUID) ([]*model.Relationship, error) {
	if len(entityIDs) < 2 {
		return nil, nil
	}
	var results []*model.Relationship
	err := r.db.WithContext(ctx).
		Where("kb_id = ? AND head_entity_id IN ? AND tail_entity_id IN ? AND is_deleted = ?", kbID, entityIDs, entityIDs, false).
		Find(&results).Error
	return results, err
}
