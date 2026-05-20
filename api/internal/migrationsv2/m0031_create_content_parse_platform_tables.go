package migrationsv2

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0031_create_content_parse_platform_tables() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2ContentParsePlatformID,
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
				`
				CREATE TABLE IF NOT EXISTS content_parse_route_policies (
					id UUID PRIMARY KEY,
					scope VARCHAR(32) NOT NULL,
					workspace_id UUID NULL,
					policy_key VARCHAR(64) NOT NULL,
					display_name VARCHAR(128) NOT NULL,
					enabled BOOLEAN NOT NULL DEFAULT TRUE,
					allow_remote BOOLEAN NOT NULL DEFAULT TRUE,
					allow_fallback BOOLEAN NOT NULL DEFAULT TRUE,
					metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_by UUID NULL,
					updated_by UUID NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ NULL
				)
				`,
				`
				CREATE UNIQUE INDEX IF NOT EXISTS uq_content_parse_route_policies_system_policy
				ON content_parse_route_policies (scope, policy_key)
				WHERE workspace_id IS NULL AND deleted_at IS NULL
				`,
				`
				CREATE UNIQUE INDEX IF NOT EXISTS uq_content_parse_route_policies_workspace_policy
				ON content_parse_route_policies (scope, workspace_id, policy_key)
				WHERE workspace_id IS NOT NULL AND deleted_at IS NULL
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_route_policies_deleted_at
				ON content_parse_route_policies (deleted_at)
				`,
				`
				CREATE TABLE IF NOT EXISTS content_parse_route_policy_rules (
					id UUID PRIMARY KEY,
					policy_id UUID NOT NULL REFERENCES content_parse_route_policies(id) ON DELETE CASCADE,
					match_file_types JSONB NOT NULL DEFAULT '[]'::jsonb,
					match_mime_prefix VARCHAR(128) NULL,
					match_is_scanned BOOLEAN NULL,
					preferred_provider_order JSONB NOT NULL DEFAULT '[]'::jsonb,
					fallback_provider_order JSONB NOT NULL DEFAULT '[]'::jsonb,
					require_local BOOLEAN NOT NULL DEFAULT FALSE,
					allow_vlm BOOLEAN NOT NULL DEFAULT TRUE,
					max_timeout_sec INTEGER NULL,
					sort_order INTEGER NOT NULL DEFAULT 100,
					metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_content_parse_route_policy_rules_policy_order
				ON content_parse_route_policy_rules (policy_id, sort_order)
				`,
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
			statements := []string{
				`DROP TABLE IF EXISTS content_parse_playground_runs`,
				`DROP TABLE IF EXISTS content_parse_artifacts`,
				`DROP TABLE IF EXISTS content_parse_chunking_runs`,
				`DROP TABLE IF EXISTS content_parse_runs`,
				`DROP TABLE IF EXISTS content_parse_route_policy_rules`,
				`DROP TABLE IF EXISTS content_parse_route_policies`,
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
