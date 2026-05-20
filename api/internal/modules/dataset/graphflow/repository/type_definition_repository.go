package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TypeDefinitionRepository handles database operations for type definitions
type TypeDefinitionRepository struct {
	db *gorm.DB
}

// NewTypeDefinitionRepository creates a new TypeDefinitionRepository
func NewTypeDefinitionRepository(db *gorm.DB) *TypeDefinitionRepository {
	return &TypeDefinitionRepository{db: db}
}

// GetByDatasetID retrieves all type definitions for a dataset
func (r *TypeDefinitionRepository) GetByDatasetID(ctx context.Context, datasetID uuid.UUID) ([]model.TypeDefinition, error) {
	var definitions []model.TypeDefinition
	err := r.db.WithContext(ctx).
		Where("dataset_id = ?", datasetID).
		Order("type_key ASC").
		Find(&definitions).Error
	return definitions, err
}

// GetByTypeKey retrieves a single type definition by dataset ID and type key
func (r *TypeDefinitionRepository) GetByTypeKey(ctx context.Context, datasetID uuid.UUID, typeKey string) (*model.TypeDefinition, error) {
	var definition model.TypeDefinition
	err := r.db.WithContext(ctx).
		Where("dataset_id = ? AND type_key = ?", datasetID, typeKey).
		First(&definition).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &definition, err
}

// Upsert inserts or updates a type definition
func (r *TypeDefinitionRepository) Upsert(ctx context.Context, definition *model.TypeDefinition) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "dataset_id"}, {Name: "type_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"label_zh", "label_en", "style_config", "updated_at"}),
		}).
		Create(definition).Error
}

// UpsertBatch inserts or updates multiple type definitions in a single transaction
func (r *TypeDefinitionRepository) UpsertBatch(ctx context.Context, definitions []model.TypeDefinition) error {
	if len(definitions) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "dataset_id"}, {Name: "type_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"label_zh", "label_en", "style_config", "updated_at"}),
		}).
		CreateInBatches(definitions, 100).Error
}

// Delete removes a type definition
func (r *TypeDefinitionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&model.TypeDefinition{}).Error
}

// GetTypeKeyMap returns a map of type_key -> TypeDefinition for quick lookups
func (r *TypeDefinitionRepository) GetTypeKeyMap(ctx context.Context, datasetID uuid.UUID) (map[string]*model.TypeDefinition, error) {
	definitions, err := r.GetByDatasetID(ctx, datasetID)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*model.TypeDefinition, len(definitions))
	for i := range definitions {
		result[definitions[i].TypeKey] = &definitions[i]
	}
	return result, nil
}
