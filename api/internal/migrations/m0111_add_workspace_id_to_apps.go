package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0111_add_workspace_id_to_apps adds workspace_id column to apps table
func M0111_add_workspace_id_to_apps() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260203000111",
		Migrate: func(tx *gorm.DB) error {
			// Add workspace_id column
			if err := tx.Exec(`
				ALTER TABLE apps 
				ADD COLUMN IF NOT EXISTS workspace_id VARCHAR(255)
			`).Error; err != nil {
				return err
			}

			// Update existing records: set workspace_id = tenant_id
			if err := tx.Exec(`
				UPDATE apps 
				SET workspace_id = tenant_id 
				WHERE workspace_id IS NULL
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE apps 
				DROP COLUMN IF EXISTS workspace_id
			`).Error
		},
	}
}
