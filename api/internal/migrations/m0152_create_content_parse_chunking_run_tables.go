package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0152ID = "202605161952152"

func M0152_create_content_parse_chunking_run_tables() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0152ID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`
				CREATE TABLE IF NOT EXISTS content_parse_chunking_runs (
					id UUID PRIMARY KEY,
					parse_run_id UUID NOT NULL REFERENCES content_parse_runs(id) ON DELETE CASCADE,
					use_case VARCHAR(32) NOT NULL,
					planner_name VARCHAR(64) NOT NULL,
					parent_mode VARCHAR(64) NULL,
					segmentation VARCHAR(64) NULL,
					unit_count INTEGER NOT NULL DEFAULT 0,
					plan_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					artifact_storage_key TEXT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_chunking_runs_parse_created
				ON content_parse_chunking_runs (parse_run_id, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_chunking_runs_use_case_created
				ON content_parse_chunking_runs (use_case, created_at DESC)
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
			return tx.Exec(`DROP TABLE IF EXISTS content_parse_chunking_runs`).Error
		},
	}
}
