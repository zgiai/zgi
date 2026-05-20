package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0155ID = "202605171955155"

func M0155_add_content_parse_playground_source_files() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0155ID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`ALTER TABLE content_parse_playground_runs ADD COLUMN IF NOT EXISTS source_storage_key TEXT NULL`,
				`ALTER TABLE content_parse_playground_runs ADD COLUMN IF NOT EXISTS source_storage_type VARCHAR(64) NULL`,
				`ALTER TABLE content_parse_playground_runs ADD COLUMN IF NOT EXISTS source_mime_type VARCHAR(128) NULL`,
				`ALTER TABLE content_parse_playground_runs ADD COLUMN IF NOT EXISTS source_file_ext VARCHAR(32) NULL`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_playground_runs_source_storage
				ON content_parse_playground_runs (source_content_hash, source_storage_type)
				WHERE source_storage_key IS NOT NULL
				`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			statements := []string{
				`DROP INDEX IF EXISTS idx_content_parse_playground_runs_source_storage`,
				`ALTER TABLE content_parse_playground_runs DROP COLUMN IF EXISTS source_file_ext`,
				`ALTER TABLE content_parse_playground_runs DROP COLUMN IF EXISTS source_mime_type`,
				`ALTER TABLE content_parse_playground_runs DROP COLUMN IF EXISTS source_storage_type`,
				`ALTER TABLE content_parse_playground_runs DROP COLUMN IF EXISTS source_storage_key`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
