package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0076_upload_files_is_temporary adds is_temporary flag to upload_files
func M0076_upload_files_is_temporary() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601100076",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE upload_files
				ADD COLUMN IF NOT EXISTS is_temporary BOOLEAN NOT NULL DEFAULT false
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE upload_files
				DROP COLUMN IF EXISTS is_temporary
			`).Error
		},
	}
}
