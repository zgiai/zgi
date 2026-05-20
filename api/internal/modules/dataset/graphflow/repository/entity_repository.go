package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow/model"
	"gorm.io/gorm"
)

type EntityRepository struct {
	db *gorm.DB
}

func NewEntityRepository(db *gorm.DB) *EntityRepository {
	return &EntityRepository{db: db}
}

// Create inserts a new entity
func (r *EntityRepository) Create(ctx context.Context, entity *model.Entity) error {
	return r.db.WithContext(ctx).Create(entity).Error
}

// CreateBatch inserts multiple entities in a single transaction
func (r *EntityRepository) CreateBatch(ctx context.Context, entities []*model.Entity) error {
	if len(entities) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(entities, 100).Error
}

// FindByKBID retrieves all entities for a specific knowledge base
func (r *EntityRepository) FindByKBID(ctx context.Context, kbID uuid.UUID) ([]*model.Entity, error) {
	var results []*model.Entity
	err := r.db.WithContext(ctx).Where("kb_id = ? AND is_deleted = ?", kbID, false).Find(&results).Error
	return results, err
}

// FindByCanonicalName finds an entity by its canonical name within a KB
func (r *EntityRepository) FindByCanonicalName(ctx context.Context, kbID uuid.UUID, canonicalName string) (*model.Entity, error) {
	var entity model.Entity
	err := r.db.WithContext(ctx).
		Where("kb_id = ? AND canonical_name = ? AND is_deleted = ?", kbID, canonicalName, false).
		First(&entity).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &entity, err
}

// GetByID retrieves an entity by its ID
func (r *EntityRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Entity, error) {
	var entity model.Entity
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&entity).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // Return nil if not found
		}
		return nil, err
	}
	return &entity, nil
}

// FindPendingSync retrieves entities that need to be synced to Neo4j
func (r *EntityRepository) FindPendingSync(ctx context.Context, kbID uuid.UUID) ([]*model.Entity, error) {
	var results []*model.Entity
	err := r.db.WithContext(ctx).
		Where("kb_id = ? AND graph_state = ? AND is_deleted = ?", kbID, "pending", false).
		Find(&results).Error
	return results, err
}

// UpdateGraphState updates the graph sync state
func (r *EntityRepository) UpdateGraphState(ctx context.Context, entityID uuid.UUID, state, graphNodeID string) error {
	updates := map[string]interface{}{
		"graph_state":   state,
		"graph_node_id": graphNodeID,
	}
	return r.db.WithContext(ctx).Model(&model.Entity{}).Where("id = ?", entityID).Updates(updates).Error
}

// IncrementSourceCount increments the source count for an entity
func (r *EntityRepository) IncrementSourceCount(ctx context.Context, entityID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&model.Entity{}).
		Where("id = ?", entityID).
		UpdateColumn("source_count", gorm.Expr("source_count + 1")).Error
}

// FindPendingVectorSync retrieves entities that need vector embeddings
func (r *EntityRepository) FindPendingVectorSync(ctx context.Context, kbID uuid.UUID) ([]*model.Entity, error) {
	var results []*model.Entity
	err := r.db.WithContext(ctx).
		Where("kb_id = ? AND vector_state = ? AND is_deleted = ?", kbID, "pending", false).
		Find(&results).Error
	return results, err
}

// UpdateVectorState updates the vector sync state, embedding ID, and sync error log
func (r *EntityRepository) UpdateVectorState(ctx context.Context, entityID uuid.UUID, state, embeddingID, syncErrorLog string) error {
	updates := map[string]interface{}{
		"vector_state":   state,
		"embedding_id":   embeddingID,
		"sync_error_log": syncErrorLog,
	}
	return r.db.WithContext(ctx).Model(&model.Entity{}).Where("id = ?", entityID).Updates(updates).Error
}

// UpdateVectorStateBatch updates the vector sync state for multiple entities
func (r *EntityRepository) UpdateVectorStateBatch(ctx context.Context, entityIDs []uuid.UUID, state string) error {
	if len(entityIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.Entity{}).Where("id IN ?", entityIDs).Update("vector_state", state).Error
}

// UpdateVectorStateFailureBatch updates the vector sync state and error log for multiple entities
func (r *EntityRepository) UpdateVectorStateFailureBatch(ctx context.Context, entityIDs []uuid.UUID, state, syncErrorLog string) error {
	if len(entityIDs) == 0 {
		return nil
	}
	updates := map[string]interface{}{
		"vector_state":   state,
		"embedding_id":   "",
		"sync_error_log": syncErrorLog,
	}
	return r.db.WithContext(ctx).Model(&model.Entity{}).Where("id IN ?", entityIDs).Updates(updates).Error
}

// UpdateGraphStateBatch updates the graph sync state for multiple entities
func (r *EntityRepository) UpdateGraphStateBatch(ctx context.Context, entityIDs []uuid.UUID, state string) error {
	if len(entityIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.Entity{}).Where("id IN ?", entityIDs).Update("graph_state", state).Error
}

// DecrementSourceCount decrements the source count for an entity
func (r *EntityRepository) DecrementSourceCount(ctx context.Context, entityID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&model.Entity{}).
		Where("id = ?", entityID).
		UpdateColumn("source_count", gorm.Expr("source_count - 1")).Error
}

// SoftDelete performs a soft delete on an entity
func (r *EntityRepository) SoftDelete(ctx context.Context, entityID uuid.UUID) error {
	now := time.Now()
	updates := map[string]interface{}{
		"is_deleted":  true,
		"deleted_at":  &now,
		"graph_state": "pending_delete",
	}
	return r.db.WithContext(ctx).Model(&model.Entity{}).Where("id = ?", entityID).Updates(updates).Error
}

// FindPendingDelete retrieves entities that are marked for deletion
func (r *EntityRepository) FindPendingDelete(ctx context.Context, limit int) ([]*model.Entity, error) {
	var results []*model.Entity
	err := r.db.WithContext(ctx).
		Where("graph_state = ?", "pending_delete").
		Limit(limit).
		Find(&results).Error
	return results, err
}

// FindByNameOrAlias finds an entity by its name or canonical name within a KB
func (r *EntityRepository) FindByNameOrAlias(ctx context.Context, kbID uuid.UUID, name string) ([]*model.Entity, error) {
	var results []*model.Entity
	err := r.db.WithContext(ctx).
		Where("kb_id = ? AND (name = ? OR canonical_name = ?) AND is_deleted = ?", kbID, name, name, false).
		Find(&results).Error
	return results, err
}
