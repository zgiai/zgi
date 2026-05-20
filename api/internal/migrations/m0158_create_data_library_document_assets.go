package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0158ID = "20260519000158"

func M0158_create_data_library_document_assets() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0158ID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`
				CREATE TABLE IF NOT EXISTS data_library_document_assets (
					id UUID PRIMARY KEY,
					organization_id VARCHAR(255) NOT NULL,
					workspace_id VARCHAR(255) NULL,
					title TEXT NOT NULL,
					source_file_id VARCHAR(255) NOT NULL,
					current_version_id UUID NULL,
					content_hash VARCHAR(255) NULL,
					status VARCHAR(32) NOT NULL DEFAULT 'archived',
					processing_level VARCHAR(32) NOT NULL DEFAULT 'archive',
					quality_score DOUBLE PRECISION NULL,
					metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					permission_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_by VARCHAR(255) NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ NULL
				)
				`,
				`
				CREATE TABLE IF NOT EXISTS data_library_document_versions (
					id UUID PRIMARY KEY,
					asset_id UUID NOT NULL REFERENCES data_library_document_assets(id) ON DELETE CASCADE,
					version_no INTEGER NOT NULL,
					source_file_id VARCHAR(255) NOT NULL,
					content_hash VARCHAR(255) NULL,
					file_name TEXT NULL,
					file_size BIGINT NOT NULL DEFAULT 0,
					mime_type VARCHAR(255) NULL,
					parse_artifact_id UUID NULL REFERENCES content_parse_artifacts(id) ON DELETE SET NULL,
					chunk_artifact_set_id UUID NULL REFERENCES content_parse_chunk_artifact_sets(id) ON DELETE SET NULL,
					status VARCHAR(32) NOT NULL DEFAULT 'archived',
					quality_score DOUBLE PRECISION NULL,
					uploaded_by VARCHAR(255) NULL,
					metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ NULL
				)
				`,
				`
				CREATE UNIQUE INDEX IF NOT EXISTS uq_data_library_document_versions_asset_version
				ON data_library_document_versions (asset_id, version_no)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_assets_org_workspace_status
				ON data_library_document_assets (organization_id, workspace_id, status, updated_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_assets_source_file
				ON data_library_document_assets (source_file_id)
				`,
				`
				CREATE UNIQUE INDEX IF NOT EXISTS uq_data_library_assets_org_source_file_active
				ON data_library_document_assets (organization_id, source_file_id)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_assets_content_hash
				ON data_library_document_assets (content_hash)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_assets_deleted_at
				ON data_library_document_assets (deleted_at)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_versions_asset_created
				ON data_library_document_versions (asset_id, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_versions_source_file
				ON data_library_document_versions (source_file_id)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_versions_content_hash
				ON data_library_document_versions (content_hash)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_versions_parse_artifact
				ON data_library_document_versions (parse_artifact_id)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_versions_chunk_artifact
				ON data_library_document_versions (chunk_artifact_set_id)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_versions_deleted_at
				ON data_library_document_versions (deleted_at)
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
				`DROP TABLE IF EXISTS data_library_document_versions`,
				`DROP TABLE IF EXISTS data_library_document_assets`,
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
