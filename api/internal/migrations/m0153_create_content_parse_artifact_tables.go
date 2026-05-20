package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0153ID = "202605161953153"

func M0153_create_content_parse_artifact_tables() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0153ID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`
				CREATE TABLE IF NOT EXISTS content_parse_artifacts (
					id UUID PRIMARY KEY,
					source_content_hash VARCHAR(255) NOT NULL,
					profile VARCHAR(64) NOT NULL,
					canonical_ir_version VARCHAR(64) NOT NULL,
					provider_signature VARCHAR(128) NOT NULL,
					artifact_storage_key TEXT NULL,
					diagnostics_storage_key TEXT NULL,
					summary_json JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ NULL
				)
				`,
				`
				CREATE UNIQUE INDEX IF NOT EXISTS uq_content_parse_artifacts_signature
				ON content_parse_artifacts (source_content_hash, profile, canonical_ir_version, provider_signature)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_artifacts_deleted_at
				ON content_parse_artifacts (deleted_at)
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
			return tx.Exec(`DROP TABLE IF EXISTS content_parse_artifacts`).Error
		},
	}
}
