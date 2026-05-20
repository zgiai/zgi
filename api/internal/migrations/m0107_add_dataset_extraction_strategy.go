package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0107_add_dataset_extraction_strategy adds extraction_strategy column to datasets table
func M0107_add_dataset_extraction_strategy() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260202235500",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE datasets 
				ADD COLUMN IF NOT EXISTS extraction_strategy VARCHAR(20) DEFAULT 'openie';
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE datasets 
				DROP COLUMN IF EXISTS extraction_strategy;
			`).Error
		},
	}
}
