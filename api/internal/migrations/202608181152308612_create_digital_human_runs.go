package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const migration202608181152308612ID = "202608181152308612_create_digital_human_runs"

const createDigitalHumanRunsSQL = `
	CREATE TABLE IF NOT EXISTS public.digital_human_runs (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE RESTRICT,
		digital_human_version_id uuid NOT NULL REFERENCES public.digital_human_versions(id) ON DELETE RESTRICT,
		actor_id uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
		idempotency_key varchar(160) NOT NULL,
		invocation_event jsonb NOT NULL,
		run_spec jsonb NOT NULL,
		run_spec_revision varchar(71) NOT NULL,
		execution_snapshot jsonb NOT NULL,
		execution_snapshot_revision varchar(71) NOT NULL,
		execution_trace jsonb NOT NULL DEFAULT '{}'::jsonb,
		query text NOT NULL,
		status varchar(16) NOT NULL DEFAULT 'queued',
		conversation_id uuid,
		message_id uuid,
		result_summary text NOT NULL DEFAULT '',
		error_code varchar(128) NOT NULL DEFAULT '',
		error_message text NOT NULL DEFAULT '',
		cancel_requested_at timestamptz,
		started_at timestamptz,
		completed_at timestamptz,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT digital_human_runs_status_check CHECK (status IN (
			'queued', 'running', 'canceling', 'succeeded', 'failed', 'canceled', 'unknown'
		)),
		CONSTRAINT digital_human_runs_revision_check CHECK (
			run_spec_revision <> '' AND execution_snapshot_revision <> ''
		)
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_digital_human_runs_idempotency
		ON public.digital_human_runs (organization_id, digital_human_id, idempotency_key);
	CREATE INDEX IF NOT EXISTS idx_digital_human_runs_scope_created
		ON public.digital_human_runs (organization_id, workspace_id, digital_human_id, created_at DESC, id DESC);
	CREATE INDEX IF NOT EXISTS idx_digital_human_runs_active
		ON public.digital_human_runs (status, updated_at) WHERE status IN ('queued', 'running', 'canceling');

	CREATE TABLE IF NOT EXISTS public.digital_human_run_outbox (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		run_id uuid NOT NULL REFERENCES public.digital_human_runs(id) ON DELETE CASCADE,
		status varchar(16) NOT NULL DEFAULT 'pending',
		attempt_count integer NOT NULL DEFAULT 0,
		available_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		claimed_at timestamptz,
		confirmed_at timestamptz,
		error_message text NOT NULL DEFAULT '',
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT digital_human_run_outbox_status_check CHECK (status IN ('pending', 'processing', 'confirmed', 'failed')),
		CONSTRAINT digital_human_run_outbox_attempt_check CHECK (attempt_count >= 0)
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_digital_human_run_outbox_run
		ON public.digital_human_run_outbox (run_id);
	CREATE INDEX IF NOT EXISTS idx_digital_human_run_outbox_claim
		ON public.digital_human_run_outbox (status, available_at, created_at)
		WHERE status IN ('pending', 'processing')
`

func init() {
	registerSchemaMigration(migration202608181152308612ID, up202608181152308612, down202608181152308612)
}

func up202608181152308612(schema *mschema.Builder) error {
	return schema.Raw(createDigitalHumanRunsSQL)
}

func down202608181152308612(schema *mschema.Builder) error {
	if err := schema.DropIfExists("digital_human_run_outbox"); err != nil {
		return err
	}
	return schema.DropIfExists("digital_human_runs")
}
