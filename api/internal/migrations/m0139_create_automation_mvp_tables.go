package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0139ID_1 = "202604081321139_1"

// M0139_create_automation_mvp_tables creates the MVP tables for scheduled automation tasks.
func M0139_create_automation_mvp_tables() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0139ID_1,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`
				CREATE TABLE IF NOT EXISTS automation_tasks (
					id UUID PRIMARY KEY,
					organization_id UUID NOT NULL,
					workspace_id UUID NOT NULL,
					name VARCHAR(255) NOT NULL,
					description TEXT NULL,
					status VARCHAR(32) NOT NULL,
					trigger_type VARCHAR(32) NOT NULL DEFAULT 'schedule',
					schedule_type VARCHAR(32) NOT NULL,
					timezone VARCHAR(64) NOT NULL,
					schedule_config JSONB NOT NULL,
					next_run_at TIMESTAMPTZ NULL,
					last_run_at TIMESTAMPTZ NULL,
					last_run_status VARCHAR(32) NULL,
					source_type VARCHAR(32) NOT NULL,
					source_ref VARCHAR(255) NULL,
					source_snapshot JSONB NULL,
					created_by UUID NOT NULL,
					updated_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				)
				`,
				`
				CREATE TABLE IF NOT EXISTS automation_task_actions (
					id UUID PRIMARY KEY,
					task_id UUID NOT NULL REFERENCES automation_tasks(id) ON DELETE CASCADE,
					action_type VARCHAR(32) NOT NULL,
					action_order INTEGER NOT NULL DEFAULT 1,
					enabled BOOLEAN NOT NULL DEFAULT TRUE,
					config JSONB NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				)
				`,
				`
				CREATE TABLE IF NOT EXISTS automation_task_runs (
					id UUID PRIMARY KEY,
					task_id UUID NOT NULL REFERENCES automation_tasks(id) ON DELETE CASCADE,
					trigger_source VARCHAR(32) NOT NULL,
					scheduled_for TIMESTAMPTZ NOT NULL,
					started_at TIMESTAMPTZ NULL,
					finished_at TIMESTAMPTZ NULL,
					status VARCHAR(32) NOT NULL,
					runtime_context JSONB NULL,
					error_summary TEXT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				)
				`,
				`
				CREATE TABLE IF NOT EXISTS automation_action_runs (
					id UUID PRIMARY KEY,
					task_run_id UUID NOT NULL REFERENCES automation_task_runs(id) ON DELETE CASCADE,
					task_action_id UUID NOT NULL REFERENCES automation_task_actions(id) ON DELETE CASCADE,
					action_type VARCHAR(32) NOT NULL,
					channel_type VARCHAR(32) NULL,
					request_payload JSONB NULL,
					response_payload JSONB NULL,
					error_message TEXT NULL,
					status VARCHAR(32) NOT NULL,
					started_at TIMESTAMPTZ NULL,
					finished_at TIMESTAMPTZ NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_automation_tasks_scope_status_updated_at
				ON automation_tasks (organization_id, workspace_id, status, updated_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_automation_tasks_status_next_run_at
				ON automation_tasks (status, next_run_at)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_automation_tasks_scope_next_run_at
				ON automation_tasks (organization_id, workspace_id, next_run_at)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_automation_task_actions_task_id_action_order
				ON automation_task_actions (task_id, action_order)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_automation_task_runs_task_id_created_at
				ON automation_task_runs (task_id, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_automation_task_runs_task_id_scheduled_for
				ON automation_task_runs (task_id, scheduled_for)
				`,
				`
				CREATE UNIQUE INDEX IF NOT EXISTS uq_automation_task_runs_task_id_scheduled_for_trigger_source
				ON automation_task_runs (task_id, scheduled_for, trigger_source)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_automation_action_runs_task_run_id_created_at
				ON automation_action_runs (task_run_id, created_at)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_automation_action_runs_task_action_id_created_at
				ON automation_action_runs (task_action_id, created_at DESC)
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
				`DROP TABLE IF EXISTS automation_action_runs`,
				`DROP TABLE IF EXISTS automation_task_runs`,
				`DROP TABLE IF EXISTS automation_task_actions`,
				`DROP TABLE IF EXISTS automation_tasks`,
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
