package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0162ID = "20260519000162"

func M0162_add_data_library_processing_request_execution_state() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0162ID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`
				ALTER TABLE data_library_processing_requests
				ADD COLUMN IF NOT EXISTS execution_metadata JSONB NOT NULL DEFAULT '{}'::jsonb
				`,
				`
				ALTER TABLE data_library_processing_requests
				ADD COLUMN IF NOT EXISTS executor_key VARCHAR(255) NULL
				`,
				`
				ALTER TABLE data_library_processing_requests
				ADD COLUMN IF NOT EXISTS error_code VARCHAR(128) NULL
				`,
				`
				ALTER TABLE data_library_processing_requests
				ADD COLUMN IF NOT EXISTS error_message TEXT NULL
				`,
				`
				ALTER TABLE data_library_processing_requests
				ADD COLUMN IF NOT EXISTS attempt_count INTEGER NOT NULL DEFAULT 0
				`,
				`
				ALTER TABLE data_library_processing_requests
				ADD COLUMN IF NOT EXISTS queued_at TIMESTAMPTZ NULL
				`,
				`
				ALTER TABLE data_library_processing_requests
				ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ NULL
				`,
				`
				ALTER TABLE data_library_processing_requests
				ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ NULL
				`,
				`
				ALTER TABLE data_library_processing_requests
				ADD COLUMN IF NOT EXISTS failed_at TIMESTAMPTZ NULL
				`,
				`
				ALTER TABLE data_library_processing_requests
				ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_processing_requests_executor_status
				ON data_library_processing_requests (executor_key, status, created_at DESC)
				WHERE deleted_at IS NULL
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
				`DROP INDEX IF EXISTS idx_data_library_processing_requests_executor_status`,
				`ALTER TABLE data_library_processing_requests DROP COLUMN IF EXISTS cancelled_at`,
				`ALTER TABLE data_library_processing_requests DROP COLUMN IF EXISTS failed_at`,
				`ALTER TABLE data_library_processing_requests DROP COLUMN IF EXISTS completed_at`,
				`ALTER TABLE data_library_processing_requests DROP COLUMN IF EXISTS started_at`,
				`ALTER TABLE data_library_processing_requests DROP COLUMN IF EXISTS queued_at`,
				`ALTER TABLE data_library_processing_requests DROP COLUMN IF EXISTS attempt_count`,
				`ALTER TABLE data_library_processing_requests DROP COLUMN IF EXISTS error_message`,
				`ALTER TABLE data_library_processing_requests DROP COLUMN IF EXISTS error_code`,
				`ALTER TABLE data_library_processing_requests DROP COLUMN IF EXISTS executor_key`,
				`ALTER TABLE data_library_processing_requests DROP COLUMN IF EXISTS execution_metadata`,
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
