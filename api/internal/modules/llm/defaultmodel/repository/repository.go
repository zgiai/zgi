package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/llm/defaultmodel/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type defaultModelRepository struct {
	db *gorm.DB
}

func NewDefaultModelRepository(db *gorm.DB) DefaultModelRepository {
	return &defaultModelRepository{db: db}
}

func (r *defaultModelRepository) ListByOrganization(ctx context.Context, organizationID uuid.UUID) ([]*model.DefaultModel, error) {
	var items []*model.DefaultModel
	err := r.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Order("use_case ASC").
		Find(&items).Error
	return items, err
}

func (r *defaultModelRepository) GetByOrganizationAndUseCase(ctx context.Context, organizationID uuid.UUID, useCase string) (*model.DefaultModel, error) {
	var item model.DefaultModel
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND use_case = ?", organizationID, useCase).
		First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *defaultModelRepository) Upsert(ctx context.Context, item *model.DefaultModel) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "organization_id"},
			{Name: "use_case"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"provider",
			"model",
			"params",
			"updated_by",
			"updated_at",
		}),
	}).Create(item).Error
}

func (r *defaultModelRepository) DeleteByOrganizationAndUseCase(ctx context.Context, organizationID uuid.UUID, useCase string) error {
	return r.db.WithContext(ctx).
		Where("organization_id = ? AND use_case = ?", organizationID, useCase).
		Delete(&model.DefaultModel{}).Error
}

