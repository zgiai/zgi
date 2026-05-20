package migrationsv2

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0007_add_dataset_entity_fields() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2AddDatasetEntityFieldsID,
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE datasets
				ADD COLUMN IF NOT EXISTS entity_model VARCHAR(255) DEFAULT 'gpt-4o',
				ADD COLUMN IF NOT EXISTS entity_model_provider VARCHAR(255) DEFAULT 'openai';
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE datasets
				DROP COLUMN IF EXISTS entity_model,
				DROP COLUMN IF EXISTS entity_model_provider;
			`).Error
		},
	}
}
