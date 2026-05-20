package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0159ID = "20260519000159"

func M0159_create_data_library_reuse_events() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0159ID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`
				CREATE TABLE IF NOT EXISTS data_library_reuse_events (
					id UUID PRIMARY KEY,
					organization_id VARCHAR(255) NOT NULL,
					workspace_id VARCHAR(255) NULL,
					asset_id UUID NOT NULL REFERENCES data_library_document_assets(id) ON DELETE CASCADE,
					version_id UUID NULL REFERENCES data_library_document_versions(id) ON DELETE SET NULL,
					artifact_type VARCHAR(64) NOT NULL,
					artifact_id UUID NULL,
					consumer_type VARCHAR(64) NOT NULL,
					consumer_id VARCHAR(255) NOT NULL,
					consumer_version VARCHAR(255) NULL,
					saved_seconds BIGINT NOT NULL DEFAULT 0,
					saved_cost_micros BIGINT NOT NULL DEFAULT 0,
					metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_by VARCHAR(255) NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ NULL
				)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_reuse_events_org_consumer
				ON data_library_reuse_events (organization_id, consumer_type, consumer_id, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_reuse_events_asset_created
				ON data_library_reuse_events (asset_id, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_reuse_events_version
				ON data_library_reuse_events (version_id)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_reuse_events_artifact
				ON data_library_reuse_events (artifact_type, artifact_id)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_reuse_events_workspace
				ON data_library_reuse_events (workspace_id)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_reuse_events_deleted_at
				ON data_library_reuse_events (deleted_at)
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
			return tx.Exec(`DROP TABLE IF EXISTS data_library_reuse_events`).Error
		},
	}
}
