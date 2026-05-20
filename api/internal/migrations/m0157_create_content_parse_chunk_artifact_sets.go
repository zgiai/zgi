package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0157ID = "20260518000157"

func M0157_create_content_parse_chunk_artifact_sets() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0157ID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`
				CREATE TABLE IF NOT EXISTS content_parse_chunk_artifact_sets (
					id UUID PRIMARY KEY,
					parse_artifact_id UUID NULL REFERENCES content_parse_artifacts(id) ON DELETE SET NULL,
					parse_run_id UUID NULL REFERENCES content_parse_runs(id) ON DELETE SET NULL,
					source_content_hash VARCHAR(255) NOT NULL,
					use_case VARCHAR(32) NOT NULL,
					planner_name VARCHAR(64) NOT NULL,
					parent_mode VARCHAR(64) NULL,
					segmentation VARCHAR(64) NULL,
					chunker_version VARCHAR(64) NOT NULL,
					signature VARCHAR(255) NOT NULL,
					status VARCHAR(32) NOT NULL DEFAULT 'succeeded',
					unit_count INTEGER NOT NULL DEFAULT 0,
					content_hash VARCHAR(255) NOT NULL,
					quality_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					summary_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					artifact_storage_key TEXT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ NULL
				)
				`,
				`
				CREATE UNIQUE INDEX IF NOT EXISTS uq_content_parse_chunk_artifact_sets_signature
				ON content_parse_chunk_artifact_sets (signature)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_chunk_artifact_sets_parse_artifact
				ON content_parse_chunk_artifact_sets (parse_artifact_id, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_chunk_artifact_sets_parse_run
				ON content_parse_chunk_artifact_sets (parse_run_id, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_chunk_artifact_sets_source_hash
				ON content_parse_chunk_artifact_sets (source_content_hash, use_case, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_chunk_artifact_sets_deleted_at
				ON content_parse_chunk_artifact_sets (deleted_at)
				`,
				`
				ALTER TABLE content_parse_chunking_runs
				ADD COLUMN IF NOT EXISTS chunk_artifact_set_id UUID NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_chunking_artifact_set
				ON content_parse_chunking_runs (chunk_artifact_set_id)
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
				`DROP INDEX IF EXISTS idx_content_parse_chunking_artifact_set`,
				`ALTER TABLE content_parse_chunking_runs DROP COLUMN IF EXISTS chunk_artifact_set_id`,
				`DROP TABLE IF EXISTS content_parse_chunk_artifact_sets`,
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
