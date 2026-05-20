package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0161ID = "20260519000161"

func M0161_create_data_library_vector_artifacts() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0161ID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`
				CREATE TABLE IF NOT EXISTS data_library_vector_artifacts (
					id UUID PRIMARY KEY,
					organization_id VARCHAR(255) NOT NULL,
					workspace_id VARCHAR(255) NULL,
					asset_id UUID NOT NULL REFERENCES data_library_document_assets(id) ON DELETE CASCADE,
					version_id UUID NOT NULL REFERENCES data_library_document_versions(id) ON DELETE CASCADE,
					chunk_artifact_set_id UUID NOT NULL REFERENCES content_parse_chunk_artifact_sets(id) ON DELETE RESTRICT,
					embedding_provider VARCHAR(128) NOT NULL,
					embedding_model VARCHAR(255) NOT NULL,
					embedding_dimension INTEGER NOT NULL DEFAULT 0,
					vector_collection VARCHAR(255) NOT NULL,
					vector_namespace VARCHAR(255) NULL,
					vector_count BIGINT NOT NULL DEFAULT 0,
					status VARCHAR(32) NOT NULL DEFAULT 'pending',
					content_hash VARCHAR(255) NULL,
					metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_by VARCHAR(255) NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ NULL
				)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_vector_artifacts_org_status
				ON data_library_vector_artifacts (organization_id, status, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_vector_artifacts_asset_created
				ON data_library_vector_artifacts (asset_id, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_vector_artifacts_version
				ON data_library_vector_artifacts (version_id, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_vector_artifacts_chunk_set
				ON data_library_vector_artifacts (chunk_artifact_set_id, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_vector_artifacts_workspace
				ON data_library_vector_artifacts (workspace_id)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_vector_artifacts_content_hash
				ON data_library_vector_artifacts (content_hash)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_vector_artifacts_deleted_at
				ON data_library_vector_artifacts (deleted_at)
				`,
				`
				CREATE UNIQUE INDEX IF NOT EXISTS uq_data_library_vector_artifacts_active_model
				ON data_library_vector_artifacts (
					organization_id,
					chunk_artifact_set_id,
					embedding_provider,
					embedding_model,
					vector_collection,
					COALESCE(vector_namespace, '')
				)
				WHERE deleted_at IS NULL AND status IN ('pending', 'ready')
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
			return tx.Exec(`DROP TABLE IF EXISTS data_library_vector_artifacts`).Error
		},
	}
}
