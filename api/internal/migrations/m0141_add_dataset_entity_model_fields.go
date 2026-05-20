package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0141_add_dataset_entity_model_fields adds entity_model and entity_model_provider columns to datasets table
func M0141_add_dataset_entity_model_fields() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260410210000",
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
