package migrationsv2

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0003_add_llm_usage_bills() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2AddLLMUsageBillsID,
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE IF NOT EXISTS llm_usage_bills (
					id UUID PRIMARY KEY,
					attempt_id VARCHAR(120) NOT NULL UNIQUE,
					request_id VARCHAR(100) NOT NULL,
					organization_id UUID NOT NULL,
					app_id UUID NULL,
					app_type VARCHAR(50) NULL,
					workspace_id VARCHAR(255) NULL,
					api_key_id UUID NOT NULL,
					quota_subject_type VARCHAR(20) NULL,
					quota_subject_id VARCHAR(255) NULL,
					model_id UUID NOT NULL,
					model_name VARCHAR(100) NOT NULL,
					provider_id UUID NOT NULL,
					provider_name VARCHAR(100) NOT NULL,
					route_id UUID NULL,
					channel_id UUID NULL,
					use_system_provider BOOLEAN NOT NULL DEFAULT false,
					status VARCHAR(20) NOT NULL,
					prompt_tokens BIGINT NOT NULL DEFAULT 0,
					completion_tokens BIGINT NOT NULL DEFAULT 0,
					total_tokens BIGINT NOT NULL DEFAULT 0,
					official_points BIGINT NOT NULL DEFAULT 0,
					private_points BIGINT NOT NULL DEFAULT 0,
					total_points BIGINT NOT NULL DEFAULT 0,
					response_time_ms BIGINT NOT NULL DEFAULT 0,
					error_code VARCHAR(100) NULL,
					error_message TEXT NULL,
					request_created_at TIMESTAMPTZ NOT NULL,
					settled_at TIMESTAMPTZ NOT NULL,
					CONSTRAINT ck_llm_usage_bills_app_pair CHECK (
						(app_id IS NULL AND app_type IS NULL) OR
						(app_id IS NOT NULL AND app_type IS NOT NULL)
					),
					CONSTRAINT ck_llm_usage_bills_quota_pair CHECK (
						(quota_subject_type IS NULL AND quota_subject_id IS NULL) OR
						(quota_subject_type IS NOT NULL AND quota_subject_id IS NOT NULL)
					),
					CONSTRAINT ck_llm_usage_bills_status CHECK (
						status IN ('success', 'failed', 'partial')
					),
					CONSTRAINT ck_llm_usage_bills_non_negative CHECK (
						prompt_tokens >= 0 AND
						completion_tokens >= 0 AND
						total_tokens >= 0 AND
						official_points >= 0 AND
						private_points >= 0 AND
						total_points >= 0 AND
						response_time_ms >= 0
					),
					CONSTRAINT ck_llm_usage_bills_total_tokens CHECK (
						total_tokens = prompt_tokens + completion_tokens
					),
					CONSTRAINT ck_llm_usage_bills_total_points CHECK (
						total_points = official_points + private_points
					),
					CONSTRAINT ck_llm_usage_bills_provider_points CHECK (
						(use_system_provider = true AND private_points = 0) OR
						(use_system_provider = false AND official_points = 0)
					),
					CONSTRAINT ck_llm_usage_bills_time_order CHECK (
						settled_at >= request_created_at
					)
				);

				CREATE INDEX IF NOT EXISTS idx_usage_bills_org_created
					ON llm_usage_bills(organization_id, request_created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_usage_bills_org_model_created
					ON llm_usage_bills(organization_id, model_name, request_created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_usage_bills_org_app_type_created
					ON llm_usage_bills(organization_id, app_type, request_created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_usage_bills_org_app_created
					ON llm_usage_bills(organization_id, app_id, request_created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_usage_bills_org_source_created
					ON llm_usage_bills(organization_id, use_system_provider, request_created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_usage_bills_request_id
					ON llm_usage_bills(request_id);
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE IF EXISTS llm_usage_bills CASCADE;`).Error
		},
	}
}
