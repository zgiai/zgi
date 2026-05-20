package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0149ID = "202605161949149"

func M0149_create_content_parse_provider_tables() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0149ID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`
				CREATE TABLE IF NOT EXISTS content_parse_provider_configs (
					id UUID PRIMARY KEY,
					scope VARCHAR(32) NOT NULL,
					workspace_id UUID NULL,
					provider_key VARCHAR(64) NOT NULL,
					provider_type VARCHAR(32) NOT NULL,
					display_name VARCHAR(128) NOT NULL,
					enabled BOOLEAN NOT NULL DEFAULT TRUE,
					priority INTEGER NOT NULL DEFAULT 100,
					adapter_name VARCHAR(64) NOT NULL,
					engine_name VARCHAR(64) NULL,
					base_url TEXT NULL,
					credentials_ciphertext JSONB NOT NULL DEFAULT '{}'::jsonb,
					timeout_sec INTEGER NOT NULL DEFAULT 180,
					supports_file_types JSONB NOT NULL DEFAULT '[]'::jsonb,
					supports_profiles JSONB NOT NULL DEFAULT '[]'::jsonb,
					cost_level VARCHAR(32) NULL,
					privacy_level VARCHAR(32) NULL,
					metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_by UUID NULL,
					updated_by UUID NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ NULL
				)
				`,
				`
				CREATE UNIQUE INDEX IF NOT EXISTS uq_content_parse_provider_configs_system_provider
				ON content_parse_provider_configs (scope, provider_key)
				WHERE workspace_id IS NULL AND deleted_at IS NULL
				`,
				`
				CREATE UNIQUE INDEX IF NOT EXISTS uq_content_parse_provider_configs_workspace_provider
				ON content_parse_provider_configs (scope, workspace_id, provider_key)
				WHERE workspace_id IS NOT NULL AND deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_provider_configs_scope_enabled_priority
				ON content_parse_provider_configs (scope, workspace_id, enabled, priority)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_provider_configs_deleted_at
				ON content_parse_provider_configs (deleted_at)
				`,
				`
				CREATE TABLE IF NOT EXISTS content_parse_provider_health_checks (
					id UUID PRIMARY KEY,
					provider_config_id UUID NOT NULL REFERENCES content_parse_provider_configs(id) ON DELETE CASCADE,
					status VARCHAR(32) NOT NULL,
					latency_ms INTEGER NULL,
					error_message TEXT NULL,
					details JSONB NOT NULL DEFAULT '{}'::jsonb,
					checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_provider_health_checks_provider_checked
				ON content_parse_provider_health_checks (provider_config_id, checked_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_provider_health_checks_status_checked
				ON content_parse_provider_health_checks (status, checked_at DESC)
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
				`DROP TABLE IF EXISTS content_parse_provider_health_checks`,
				`DROP TABLE IF EXISTS content_parse_provider_configs`,
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
