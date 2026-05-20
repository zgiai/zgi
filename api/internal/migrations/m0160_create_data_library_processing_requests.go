package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0160ID = "20260519000160"

func M0160_create_data_library_processing_requests() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0160ID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`
				CREATE TABLE IF NOT EXISTS data_library_processing_requests (
					id UUID PRIMARY KEY,
					organization_id VARCHAR(255) NOT NULL,
					workspace_id VARCHAR(255) NULL,
					asset_id UUID NOT NULL REFERENCES data_library_document_assets(id) ON DELETE CASCADE,
					target_level VARCHAR(32) NOT NULL,
					status VARCHAR(32) NOT NULL DEFAULT 'planned',
					requested_by VARCHAR(255) NULL,
					force BOOLEAN NOT NULL DEFAULT FALSE,
					plan_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					request_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ NULL
				)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_processing_requests_org_status
				ON data_library_processing_requests (organization_id, status, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_processing_requests_asset_created
				ON data_library_processing_requests (asset_id, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_processing_requests_workspace
				ON data_library_processing_requests (workspace_id)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_processing_requests_deleted_at
				ON data_library_processing_requests (deleted_at)
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
			return tx.Exec(`DROP TABLE IF EXISTS data_library_processing_requests`).Error
		},
	}
}
