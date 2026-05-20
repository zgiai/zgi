package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0150ID = "202605161950150"

func M0150_create_content_parse_policy_tables() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0150ID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
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
				`DROP TABLE IF EXISTS content_parse_route_policy_rules`,
				`DROP TABLE IF EXISTS content_parse_route_policies`,
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
