package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0151ID = "202605161951151"

func M0151_create_content_parse_run_tables() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0151ID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`
				CREATE TABLE IF NOT EXISTS content_parse_runs (
					id UUID PRIMARY KEY,
					workspace_id UUID NULL,
					dataset_id UUID NULL,
					document_id UUID NULL,
					file_id UUID NULL,
					artifact_id UUID NULL,
					source_type VARCHAR(32) NOT NULL,
					source_ref TEXT NULL,
					file_name TEXT NULL,
					intent VARCHAR(32) NOT NULL,
					profile VARCHAR(64) NOT NULL,
					policy_key VARCHAR(64) NULL,
					route_policy_id UUID NULL REFERENCES content_parse_route_policies(id) ON DELETE SET NULL,
					requested_provider_key VARCHAR(64) NULL,
					planned_provider_order JSONB NOT NULL DEFAULT '[]'::jsonb,
					attempted_provider_order JSONB NOT NULL DEFAULT '[]'::jsonb,
					final_provider_key VARCHAR(64) NULL,
					adapter_name VARCHAR(64) NULL,
					engine_name VARCHAR(64) NULL,
					status VARCHAR(32) NOT NULL,
					quality_level VARCHAR(32) NOT NULL,
					fallback_used BOOLEAN NOT NULL DEFAULT FALSE,
					duration_ms INTEGER NULL,
					artifact_storage_key TEXT NULL,
					diagnostics_storage_key TEXT NULL,
					summary_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_runs_workspace_created
				ON content_parse_runs (workspace_id, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_runs_dataset_document_created
				ON content_parse_runs (dataset_id, document_id, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_runs_status_quality_created
				ON content_parse_runs (status, quality_level, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_runs_provider_created
				ON content_parse_runs (final_provider_key, created_at DESC)
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
			return tx.Exec(`DROP TABLE IF EXISTS content_parse_runs`).Error
		},
	}
}
