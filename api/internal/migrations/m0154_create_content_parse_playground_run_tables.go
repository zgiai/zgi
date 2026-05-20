package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0154ID = "202605161953154"

func M0154_create_content_parse_playground_run_tables() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0154ID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`
				CREATE TABLE IF NOT EXISTS content_parse_playground_runs (
					id UUID PRIMARY KEY,
					workspace_id UUID NULL,
					account_id UUID NULL,
					file_name TEXT NOT NULL,
					file_size BIGINT NOT NULL DEFAULT 0,
					source_content_hash VARCHAR(255) NOT NULL,
					requested_provider_key VARCHAR(64) NOT NULL,
					final_provider_key VARCHAR(64) NULL,
					adapter_name VARCHAR(64) NULL,
					engine_name VARCHAR(64) NULL,
					profile VARCHAR(64) NOT NULL,
					ocr_engine VARCHAR(64) NULL,
					status VARCHAR(32) NOT NULL,
					quality_level VARCHAR(32) NOT NULL,
					fallback_used BOOLEAN NOT NULL DEFAULT FALSE,
					duration_ms INTEGER NULL,
					artifact_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					route_plan_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					chunk_source_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					chunk_plan_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					quality_summary_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					summary_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					share_token VARCHAR(64) NOT NULL,
					is_share_enabled BOOLEAN NOT NULL DEFAULT FALSE,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ NULL
				)
				`,
				`
				CREATE UNIQUE INDEX IF NOT EXISTS uq_content_parse_playground_runs_share_token
				ON content_parse_playground_runs (share_token)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_playground_runs_workspace_created
				ON content_parse_playground_runs (workspace_id, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_playground_runs_hash_created
				ON content_parse_playground_runs (source_content_hash, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_playground_runs_hash_provider_created
				ON content_parse_playground_runs (source_content_hash, final_provider_key, profile, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_playground_runs_deleted_at
				ON content_parse_playground_runs (deleted_at)
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
			return tx.Exec(`DROP TABLE IF EXISTS content_parse_playground_runs`).Error
		},
	}
}
