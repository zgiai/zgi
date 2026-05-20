package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0023_add_workflow_approval_runtime creates runtime tables for approval nodes.
func M0023_add_workflow_approval_runtime() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2WorkflowApprovalRuntimeID,
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS public.workflow_approval_forms (
					id UUID PRIMARY KEY,
					tenant_id UUID NOT NULL,
					app_id UUID NOT NULL,
					workflow_run_id VARCHAR(255) NOT NULL,
					node_id VARCHAR(255) NOT NULL,
					node_title VARCHAR(255),
					form_definition TEXT NOT NULL,
					rendered_content TEXT NOT NULL,
					status VARCHAR(32) NOT NULL DEFAULT 'waiting',
					expiration_time TIMESTAMP NOT NULL,
					selected_action_id VARCHAR(200),
					submitted_data TEXT,
					submitted_at TIMESTAMP,
					submission_user_id UUID,
					submission_end_user_id UUID,
					completed_by_recipient_id UUID,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
				);

				CREATE INDEX IF NOT EXISTS idx_workflow_approval_forms_tenant_id ON public.workflow_approval_forms(tenant_id);
				CREATE INDEX IF NOT EXISTS idx_workflow_approval_forms_app_id ON public.workflow_approval_forms(app_id);
				CREATE INDEX IF NOT EXISTS idx_workflow_approval_forms_workflow_run_id ON public.workflow_approval_forms(workflow_run_id);
				CREATE INDEX IF NOT EXISTS idx_workflow_approval_forms_node_id ON public.workflow_approval_forms(node_id);
				CREATE INDEX IF NOT EXISTS idx_workflow_approval_forms_status_expiration_time ON public.workflow_approval_forms(status, expiration_time);
				CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_approval_forms_run_node ON public.workflow_approval_forms(workflow_run_id, node_id);

				CREATE TABLE IF NOT EXISTS public.workflow_approval_deliveries (
					id UUID PRIMARY KEY,
					form_id UUID NOT NULL,
					delivery_method_type VARCHAR(32) NOT NULL,
					delivery_config_id UUID,
					channel_payload TEXT NOT NULL,
					last_error TEXT,
					sent_at TIMESTAMP,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
				);

				CREATE INDEX IF NOT EXISTS idx_workflow_approval_deliveries_form_id ON public.workflow_approval_deliveries(form_id);

				CREATE TABLE IF NOT EXISTS public.workflow_approval_recipients (
					id UUID PRIMARY KEY,
					form_id UUID NOT NULL,
					delivery_id UUID NOT NULL,
					recipient_type VARCHAR(64) NOT NULL,
					recipient_payload TEXT NOT NULL,
					access_token VARCHAR(64) NOT NULL UNIQUE,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
				);

				CREATE INDEX IF NOT EXISTS idx_workflow_approval_recipients_form_id ON public.workflow_approval_recipients(form_id);
				CREATE INDEX IF NOT EXISTS idx_workflow_approval_recipients_delivery_id ON public.workflow_approval_recipients(delivery_id);

				CREATE TABLE IF NOT EXISTS public.workflow_run_pauses (
					id UUID PRIMARY KEY,
					tenant_id UUID NOT NULL,
					app_id UUID NOT NULL,
					workflow_run_id VARCHAR(255) NOT NULL,
					node_id VARCHAR(255) NOT NULL,
					reason VARCHAR(64) NOT NULL,
					state_json TEXT NOT NULL,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					resumed_at TIMESTAMP
				);

				CREATE INDEX IF NOT EXISTS idx_workflow_run_pauses_tenant_id ON public.workflow_run_pauses(tenant_id);
				CREATE INDEX IF NOT EXISTS idx_workflow_run_pauses_app_id ON public.workflow_run_pauses(app_id);
				CREATE INDEX IF NOT EXISTS idx_workflow_run_pauses_workflow_run_id ON public.workflow_run_pauses(workflow_run_id);
				CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_run_pauses_active_run ON public.workflow_run_pauses(workflow_run_id) WHERE resumed_at IS NULL;

				CREATE TABLE IF NOT EXISTS public.workflow_run_pause_reasons (
					id UUID PRIMARY KEY,
					pause_id UUID NOT NULL,
					type VARCHAR(64) NOT NULL,
					node_id VARCHAR(255) NOT NULL DEFAULT '',
					form_id VARCHAR(255) NOT NULL DEFAULT '',
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
				);

				CREATE INDEX IF NOT EXISTS idx_workflow_run_pause_reasons_pause_id ON public.workflow_run_pause_reasons(pause_id);

				CREATE TABLE IF NOT EXISTS public.workflow_run_events (
					id UUID PRIMARY KEY,
					tenant_id UUID NOT NULL,
					app_id UUID NOT NULL,
					workflow_run_id VARCHAR(255) NOT NULL,
					sequence INT NOT NULL,
					event_type VARCHAR(100) NOT NULL,
					event_data TEXT NOT NULL,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
				);

				CREATE INDEX IF NOT EXISTS idx_workflow_run_events_tenant_run_sequence ON public.workflow_run_events(tenant_id, workflow_run_id, sequence);
			`).Error; err != nil {
				return fmt.Errorf("create workflow approval runtime tables: %w", err)
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				DROP TABLE IF EXISTS public.workflow_run_events;
				DROP TABLE IF EXISTS public.workflow_run_pause_reasons;
				DROP TABLE IF EXISTS public.workflow_run_pauses;
				DROP TABLE IF EXISTS public.workflow_approval_recipients;
				DROP TABLE IF EXISTS public.workflow_approval_deliveries;
				DROP TABLE IF EXISTS public.workflow_approval_forms;
			`).Error; err != nil {
				return fmt.Errorf("drop workflow approval runtime tables: %w", err)
			}

			return nil
		},
	}
}
