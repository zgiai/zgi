package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0165ID = "20260519000165"

func M0165_create_data_library_extraction_artifacts() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0165ID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`
				CREATE TABLE IF NOT EXISTS data_library_extraction_artifacts (
					id UUID PRIMARY KEY,
					organization_id VARCHAR(255) NOT NULL,
					workspace_id VARCHAR(255) NULL,
					asset_id UUID NOT NULL REFERENCES data_library_document_assets(id) ON DELETE CASCADE,
					version_id UUID NOT NULL REFERENCES data_library_document_versions(id) ON DELETE CASCADE,
					parse_artifact_id UUID NULL REFERENCES content_parse_artifacts(id) ON DELETE SET NULL,
					data_source_id UUID NULL REFERENCES data_sources(id) ON DELETE SET NULL,
					table_id UUID NULL REFERENCES data_source_tables(id) ON DELETE SET NULL,
					schema_name VARCHAR(255) NULL,
					schema_hash VARCHAR(255) NULL,
					extractor_provider VARCHAR(128) NULL,
					extractor_model VARCHAR(255) NULL,
					record_count BIGINT NOT NULL DEFAULT 0,
					field_count BIGINT NOT NULL DEFAULT 0,
					evidence_count BIGINT NOT NULL DEFAULT 0,
					status VARCHAR(32) NOT NULL DEFAULT 'pending',
					quality_score DOUBLE PRECISION NULL,
					content_hash VARCHAR(255) NULL,
					output_uri TEXT NULL,
					metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_by VARCHAR(255) NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ NULL
				)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_org_status
				ON data_library_extraction_artifacts (organization_id, status, created_at DESC)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_asset_created
				ON data_library_extraction_artifacts (asset_id, created_at DESC)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_version
				ON data_library_extraction_artifacts (version_id, created_at DESC)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_parse
				ON data_library_extraction_artifacts (parse_artifact_id)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_source
				ON data_library_extraction_artifacts (data_source_id)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_table
				ON data_library_extraction_artifacts (table_id)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_workspace
				ON data_library_extraction_artifacts (workspace_id)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_schema_hash
				ON data_library_extraction_artifacts (schema_hash)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_content_hash
				ON data_library_extraction_artifacts (content_hash)
				WHERE deleted_at IS NULL
				`,
				`
				ALTER TABLE data_library_database_asset_refs
				ADD CONSTRAINT fk_data_library_db_asset_refs_extraction_artifact
				FOREIGN KEY (extraction_artifact_id)
				REFERENCES data_library_extraction_artifacts(id)
				ON DELETE SET NULL
				`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					if isDuplicateConstraintError(err) {
						continue
					}
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`ALTER TABLE data_library_database_asset_refs DROP CONSTRAINT IF EXISTS fk_data_library_db_asset_refs_extraction_artifact`).Error; err != nil {
				return err
			}
			return tx.Exec(`DROP TABLE IF EXISTS data_library_extraction_artifacts`).Error
		},
	}
}
