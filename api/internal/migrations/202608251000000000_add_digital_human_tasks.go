package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202608251000000000ID = "202608251000000000_add_digital_human_tasks"

const addDigitalHumanTasksSQL = `
	ALTER TABLE public.automation_tasks
		ADD COLUMN IF NOT EXISTS task_kind varchar(32) NOT NULL DEFAULT 'automation',
		ADD COLUMN IF NOT EXISTS subject_type varchar(32),
		ADD COLUMN IF NOT EXISTS subject_id uuid,
		ADD COLUMN IF NOT EXISTS work_status varchar(32),
		ADD COLUMN IF NOT EXISTS priority varchar(16),
		ADD COLUMN IF NOT EXISTS goal text,
		ADD COLUMN IF NOT EXISTS success_criteria jsonb NOT NULL DEFAULT '[]'::jsonb,
		ADD COLUMN IF NOT EXISTS execution_timeout_seconds integer NOT NULL DEFAULT 300,
		ADD COLUMN IF NOT EXISTS max_attempts integer NOT NULL DEFAULT 1,
		ADD COLUMN IF NOT EXISTS revision bigint NOT NULL DEFAULT 1,
		ADD COLUMN IF NOT EXISTS delivery_config jsonb,
		ADD COLUMN IF NOT EXISTS source_operation_id uuid,
		ADD COLUMN IF NOT EXISTS proposal_expires_at timestamptz;

	ALTER TABLE public.automation_tasks
		DROP CONSTRAINT IF EXISTS automation_tasks_task_kind_check,
		DROP CONSTRAINT IF EXISTS automation_tasks_subject_check,
		DROP CONSTRAINT IF EXISTS automation_tasks_work_status_check,
		DROP CONSTRAINT IF EXISTS automation_tasks_priority_check,
		DROP CONSTRAINT IF EXISTS automation_tasks_execution_timeout_check,
		DROP CONSTRAINT IF EXISTS automation_tasks_max_attempts_check,
		DROP CONSTRAINT IF EXISTS automation_tasks_revision_check;
	ALTER TABLE public.automation_tasks
		ADD CONSTRAINT automation_tasks_task_kind_check CHECK (task_kind IN ('automation', 'digital_human')),
		ADD CONSTRAINT automation_tasks_subject_check CHECK (
			(subject_type IS NULL AND subject_id IS NULL) OR
			(subject_type = 'digital_human' AND subject_id IS NOT NULL)
		),
		ADD CONSTRAINT automation_tasks_work_status_check CHECK (
			work_status IS NULL OR work_status IN ('proposed', 'pending', 'in_progress', 'waiting', 'blocked', 'completed', 'canceled')
		),
		ADD CONSTRAINT automation_tasks_priority_check CHECK (
			priority IS NULL OR priority IN ('low', 'normal', 'high', 'urgent')
		),
		ADD CONSTRAINT automation_tasks_execution_timeout_check CHECK (execution_timeout_seconds BETWEEN 30 AND 1800),
		ADD CONSTRAINT automation_tasks_max_attempts_check CHECK (max_attempts BETWEEN 1 AND 3),
		ADD CONSTRAINT automation_tasks_revision_check CHECK (revision > 0);

	CREATE INDEX IF NOT EXISTS idx_automation_tasks_subject_work_status
		ON public.automation_tasks (organization_id, workspace_id, subject_type, subject_id, work_status, updated_at DESC);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_automation_tasks_subject_operation
		ON public.automation_tasks (organization_id, workspace_id, subject_id, source_operation_id)
		WHERE source_operation_id IS NOT NULL;

	ALTER TABLE public.automation_action_runs
		ADD COLUMN IF NOT EXISTS external_run_type varchar(32),
		ADD COLUMN IF NOT EXISTS external_run_id uuid;
	CREATE UNIQUE INDEX IF NOT EXISTS idx_automation_action_runs_task_action_unique
		ON public.automation_action_runs (task_run_id, task_action_id);
	CREATE INDEX IF NOT EXISTS idx_automation_action_runs_pending_external
		ON public.automation_action_runs (status, external_run_type, updated_at)
		WHERE status IN ('queued', 'running');

	ALTER TABLE public.channel_delivery_outbox
		ALTER COLUMN inbound_event_id DROP NOT NULL;

	CREATE TABLE IF NOT EXISTS public.automation_task_activities (
		id uuid PRIMARY KEY,
		task_id uuid NOT NULL REFERENCES public.automation_tasks(id) ON DELETE CASCADE,
		operation_id uuid NOT NULL UNIQUE,
		actor_type varchar(32) NOT NULL,
		actor_ref varchar(255) NOT NULL,
		action varchar(64) NOT NULL,
		summary text NOT NULL,
		changes jsonb NOT NULL DEFAULT '{}'::jsonb,
		source_run_id uuid,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT automation_task_activities_actor_type_check CHECK (
			actor_type IN ('account', 'digital_human', 'external', 'system')
		)
	);
	CREATE INDEX IF NOT EXISTS idx_automation_task_activities_task_created
		ON public.automation_task_activities (task_id, created_at DESC);

	CREATE TABLE IF NOT EXISTS public.digital_human_channel_task_policies (
		installation_id uuid PRIMARY KEY REFERENCES public.channel_installations(id) ON DELETE CASCADE,
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE CASCADE,
		enabled boolean NOT NULL DEFAULT false,
		revision bigint NOT NULL DEFAULT 1,
		updated_by uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT digital_human_channel_task_policies_revision_check CHECK (revision > 0)
	);
	CREATE INDEX IF NOT EXISTS idx_digital_human_channel_task_policies_subject
		ON public.digital_human_channel_task_policies (organization_id, workspace_id, digital_human_id, enabled);
`

func init() {
	registerSchemaMigration(migration202608251000000000ID, up202608251000000000, down202608251000000000)
}

func up202608251000000000(schema *mschema.Builder) error {
	return schema.Raw(addDigitalHumanTasksSQL)
}

func down202608251000000000(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP TABLE IF EXISTS public.digital_human_channel_task_policies;
		DROP TABLE IF EXISTS public.automation_task_activities;
		DROP INDEX IF EXISTS public.idx_automation_action_runs_pending_external;
		DROP INDEX IF EXISTS public.idx_automation_action_runs_task_action_unique;
		ALTER TABLE public.automation_action_runs
			DROP COLUMN IF EXISTS external_run_id,
			DROP COLUMN IF EXISTS external_run_type;
		DELETE FROM public.channel_delivery_outbox WHERE inbound_event_id IS NULL;
		ALTER TABLE public.channel_delivery_outbox ALTER COLUMN inbound_event_id SET NOT NULL;
		DROP INDEX IF EXISTS public.idx_automation_tasks_subject_operation;
		DROP INDEX IF EXISTS public.idx_automation_tasks_subject_work_status;
		ALTER TABLE public.automation_tasks
			DROP CONSTRAINT IF EXISTS automation_tasks_revision_check,
			DROP CONSTRAINT IF EXISTS automation_tasks_max_attempts_check,
			DROP CONSTRAINT IF EXISTS automation_tasks_execution_timeout_check,
			DROP CONSTRAINT IF EXISTS automation_tasks_priority_check,
			DROP CONSTRAINT IF EXISTS automation_tasks_work_status_check,
			DROP CONSTRAINT IF EXISTS automation_tasks_subject_check,
			DROP CONSTRAINT IF EXISTS automation_tasks_task_kind_check,
			DROP COLUMN IF EXISTS proposal_expires_at,
			DROP COLUMN IF EXISTS source_operation_id,
			DROP COLUMN IF EXISTS delivery_config,
			DROP COLUMN IF EXISTS revision,
			DROP COLUMN IF EXISTS max_attempts,
			DROP COLUMN IF EXISTS execution_timeout_seconds,
			DROP COLUMN IF EXISTS success_criteria,
			DROP COLUMN IF EXISTS goal,
			DROP COLUMN IF EXISTS priority,
			DROP COLUMN IF EXISTS work_status,
			DROP COLUMN IF EXISTS subject_id,
			DROP COLUMN IF EXISTS subject_type,
			DROP COLUMN IF EXISTS task_kind;
	`)
}
