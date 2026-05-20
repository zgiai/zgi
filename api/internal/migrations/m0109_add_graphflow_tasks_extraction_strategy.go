package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0109_add_graphflow_tasks_extraction_strategy adds the missing extraction_strategy column to graphflow_tasks table
// This fixes the "column does not exist" error when creating GraphFlow tasks after document indexing
func M0109_add_graphflow_tasks_extraction_strategy() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260204013800",
		Migrate: func(tx *gorm.DB) error {
			// Check if table exists first
			var count int64
			tx.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_name = ?", "graphflow_tasks").Scan(&count)

			// Only alter table if it exists
			if count > 0 {
				return tx.Exec(`
					ALTER TABLE graphflow_tasks 
					ADD COLUMN IF NOT EXISTS extraction_strategy VARCHAR(20) DEFAULT 'llm';
				`).Error
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE graphflow_tasks 
				DROP COLUMN IF EXISTS extraction_strategy;
			`).Error
		},
	}
}
