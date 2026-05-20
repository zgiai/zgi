package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0133_tool_files_lifecycle() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603150133",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE tool_files
				ADD COLUMN IF NOT EXISTS lifecycle VARCHAR(32) NOT NULL DEFAULT 'persistent'
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				ALTER TABLE tool_files
				ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NULL
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_tool_files_lifecycle_expires_at
				ON tool_files (lifecycle, expires_at)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				DROP INDEX IF EXISTS idx_tool_files_lifecycle_expires_at
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				ALTER TABLE tool_files
				DROP COLUMN IF EXISTS expires_at
			`).Error; err != nil {
				return err
			}

			return tx.Exec(`
				ALTER TABLE tool_files
				DROP COLUMN IF EXISTS lifecycle
			`).Error
		},
	}
}
