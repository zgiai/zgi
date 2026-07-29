package excelimport

import (
	"context"
	"time"

	"github.com/google/uuid"
	model "github.com/zgiai/zgi/api/internal/modules/datasource/model/excelimport"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ColumnAliasRepository struct {
	db *gorm.DB
}

func NewColumnAliasRepository(db *gorm.DB) *ColumnAliasRepository {
	return &ColumnAliasRepository{db: db}
}

func (r *ColumnAliasRepository) ListByTableID(ctx context.Context, organizationID, dataSourceID, tableID string) ([]model.ColumnAlias, error) {
	var aliases []model.ColumnAlias
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND data_source_id = ? AND table_id = ?", organizationID, dataSourceID, tableID).
		Find(&aliases).Error
	return aliases, err
}

func (r *ColumnAliasRepository) Confirm(ctx context.Context, alias model.ColumnAlias) error {
	now := time.Now()
	if alias.ID == "" {
		alias.ID = uuid.NewString()
	}
	alias.ConfirmedCount = 1
	alias.CreatedAt = now
	alias.UpdatedAt = now
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "table_id"}, {Name: "target_column_id"}, {Name: "normalized_header"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"source_header":      alias.SourceHeader,
			"target_column_name": alias.TargetColumnName,
			"confirmed_count":    gorm.Expr("data_source_import_column_aliases.confirmed_count + 1"),
			"updated_by":         alias.UpdatedBy,
			"updated_at":         now,
		}),
	}).Create(&alias).Error
}
