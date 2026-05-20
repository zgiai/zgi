package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0164ID = "20260519000164"

func M0164_create_data_library_database_asset_refs() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0164ID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`
				CREATE TABLE IF NOT EXISTS data_library_database_asset_refs (
					id UUID PRIMARY KEY,
					organization_id VARCHAR(255) NOT NULL,
					workspace_id VARCHAR(255) NULL,
					data_source_id UUID NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
					table_id UUID NULL REFERENCES data_source_tables(id) ON DELETE SET NULL,
					asset_id UUID NOT NULL REFERENCES data_library_document_assets(id) ON DELETE CASCADE,
					version_id UUID NOT NULL REFERENCES data_library_document_versions(id) ON DELETE CASCADE,
					parse_artifact_id UUID NULL REFERENCES content_parse_artifacts(id) ON DELETE SET NULL,
					extraction_artifact_id UUID NULL,
					status VARCHAR(32) NOT NULL DEFAULT 'active',
					metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_by VARCHAR(255) NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ NULL
				)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_db_asset_refs_org_source
				ON data_library_database_asset_refs (organization_id, data_source_id, status)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_db_asset_refs_table
				ON data_library_database_asset_refs (table_id)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_db_asset_refs_asset
				ON data_library_database_asset_refs (asset_id)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_db_asset_refs_version
				ON data_library_database_asset_refs (version_id)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_db_asset_refs_parse
				ON data_library_database_asset_refs (parse_artifact_id)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_db_asset_refs_extraction
				ON data_library_database_asset_refs (extraction_artifact_id)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_db_asset_refs_workspace
				ON data_library_database_asset_refs (workspace_id)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE UNIQUE INDEX IF NOT EXISTS uniq_data_library_db_asset_refs_active_source
				ON data_library_database_asset_refs (organization_id, data_source_id, asset_id, version_id)
				WHERE deleted_at IS NULL AND status = 'active' AND table_id IS NULL
				`,
				`
				CREATE UNIQUE INDEX IF NOT EXISTS uniq_data_library_db_asset_refs_active_table
				ON data_library_database_asset_refs (organization_id, data_source_id, table_id, asset_id, version_id)
				WHERE deleted_at IS NULL AND status = 'active' AND table_id IS NOT NULL
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
			return tx.Exec(`DROP TABLE IF EXISTS data_library_database_asset_refs`).Error
		},
	}
}
