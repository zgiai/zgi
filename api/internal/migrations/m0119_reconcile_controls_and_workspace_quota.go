package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0119_reconcile_controls_and_workspace_quota adds reconcile control columns
// and workspace-subject quota storage for LLM billing.
func M0119_reconcile_controls_and_workspace_quota() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260302000119",
		Migrate: func(tx *gorm.DB) error {
			var attemptsExists bool
			if err := tx.Raw(`
				SELECT EXISTS (
					SELECT 1
					FROM information_schema.tables
					WHERE table_schema = CURRENT_SCHEMA()
					  AND table_name = 'billing_attempts'
				)
			`).Scan(&attemptsExists).Error; err != nil {
				return err
			}

			if attemptsExists {
				if err := tx.Exec(`
					ALTER TABLE billing_attempts
						ADD COLUMN IF NOT EXISTS reconcile_attempts INTEGER NOT NULL DEFAULT 0,
						ADD COLUMN IF NOT EXISTS next_reconcile_at TIMESTAMPTZ,
						ADD COLUMN IF NOT EXISTS last_reconcile_at TIMESTAMPTZ;

					CREATE INDEX IF NOT EXISTS idx_billing_attempts_reconcile_queue
						ON billing_attempts(status, lane, next_reconcile_at, updated_at);
				`).Error; err != nil {
					return err
				}
			}

			return tx.Exec(`
				CREATE TABLE IF NOT EXISTS llm_workspace_quotas (
					workspace_id VARCHAR(255) PRIMARY KEY,
					organization_id UUID NOT NULL,
					used_quota BIGINT NOT NULL DEFAULT 0,
					remain_quota BIGINT NOT NULL DEFAULT 0,
					quota_limit BIGINT,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					CONSTRAINT chk_llm_workspace_quotas_used_non_negative
						CHECK (used_quota >= 0),
					CONSTRAINT chk_llm_workspace_quotas_limit_positive
						CHECK (quota_limit IS NULL OR quota_limit > 0)
				);

				CREATE INDEX IF NOT EXISTS idx_llm_workspace_quotas_org
					ON llm_workspace_quotas(organization_id);
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				DROP TABLE IF EXISTS llm_workspace_quotas;
				DROP INDEX IF EXISTS idx_billing_attempts_reconcile_queue;
			`).Error; err != nil {
				return err
			}
			return tx.Exec(`
				ALTER TABLE billing_attempts
					DROP COLUMN IF EXISTS last_reconcile_at,
					DROP COLUMN IF EXISTS next_reconcile_at,
					DROP COLUMN IF EXISTS reconcile_attempts;
			`).Error
		},
	}
}
