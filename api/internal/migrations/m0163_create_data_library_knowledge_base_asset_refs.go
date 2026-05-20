package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0163ID = "20260519000163"

func M0163_create_data_library_knowledge_base_asset_refs() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0163ID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`
				CREATE TABLE IF NOT EXISTS data_library_knowledge_base_asset_refs (
					id UUID PRIMARY KEY,
					organization_id VARCHAR(255) NOT NULL,
					workspace_id VARCHAR(255) NULL,
					dataset_id UUID NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
					asset_id UUID NOT NULL REFERENCES data_library_document_assets(id) ON DELETE CASCADE,
					version_id UUID NOT NULL REFERENCES data_library_document_versions(id) ON DELETE CASCADE,
					chunk_artifact_set_id UUID NULL REFERENCES content_parse_chunk_artifact_sets(id) ON DELETE SET NULL,
					vector_artifact_id UUID NULL REFERENCES data_library_vector_artifacts(id) ON DELETE SET NULL,
					status VARCHAR(32) NOT NULL DEFAULT 'active',
					metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_by VARCHAR(255) NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ NULL
				)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_org_dataset
				ON data_library_knowledge_base_asset_refs (organization_id, dataset_id, status)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_asset
				ON data_library_knowledge_base_asset_refs (asset_id)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_version
				ON data_library_knowledge_base_asset_refs (version_id)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_chunk_set
				ON data_library_knowledge_base_asset_refs (chunk_artifact_set_id)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_vector
				ON data_library_knowledge_base_asset_refs (vector_artifact_id)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_workspace
				ON data_library_knowledge_base_asset_refs (workspace_id)
				WHERE deleted_at IS NULL
				`,
				`
				CREATE UNIQUE INDEX IF NOT EXISTS uniq_data_library_kb_asset_refs_active
				ON data_library_knowledge_base_asset_refs (organization_id, dataset_id, asset_id, version_id)
				WHERE deleted_at IS NULL AND status = 'active'
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
			return tx.Exec(`DROP TABLE IF EXISTS data_library_knowledge_base_asset_refs`).Error
		},
	}
}
