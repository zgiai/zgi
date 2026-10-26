package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Step 3 of ADR 0012: access events are purged after a retention period, so
// they are indexed by age; credentials may be limited to scopes and apps
// (null keeps full access); and end-user exports run as jobs whose
// compressed result is kept for a few days.
func init() {
	registerSchemaMigration("202610241300000000_runtime_workspace_access_limits", upRuntimeWorkspaceAccessLimits, downRuntimeWorkspaceAccessLimits)
}

func upRuntimeWorkspaceAccessLimits(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE INDEX IF NOT EXISTS idx_runtime_access_events_created
			ON public.runtime_workspace_access_events (created_at);

		ALTER TABLE public.runtime_workspace_credentials
			ADD COLUMN IF NOT EXISTS scopes jsonb,
			ADD COLUMN IF NOT EXISTS app_keys jsonb;

		CREATE TABLE IF NOT EXISTS public.runtime_workspace_end_user_exports (
			id uuid PRIMARY KEY,
			runtime_workspace_id uuid NOT NULL,
			subject_id uuid NOT NULL,
			actor_kind varchar(16) NOT NULL,
			actor_id uuid,
			status varchar(16) NOT NULL DEFAULT 'pending',
			attempts integer NOT NULL DEFAULT 0,
			lease_until timestamptz,
			content bytea,
			size_bytes bigint NOT NULL DEFAULT 0,
			error varchar(500) NOT NULL DEFAULT '',
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			finished_at timestamptz,
			expires_at timestamptz NOT NULL,
			CONSTRAINT ck_runtime_workspace_end_user_exports_status CHECK (status IN ('pending', 'running', 'succeeded', 'failed'))
		);
		CREATE UNIQUE INDEX IF NOT EXISTS uq_runtime_workspace_end_user_exports_active
			ON public.runtime_workspace_end_user_exports (subject_id)
			WHERE status IN ('pending', 'running');
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_end_user_exports_due
			ON public.runtime_workspace_end_user_exports (created_at)
			WHERE status IN ('pending', 'running');
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_end_user_exports_subject
			ON public.runtime_workspace_end_user_exports (subject_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_end_user_exports_expires
			ON public.runtime_workspace_end_user_exports (expires_at);
	`)
}

func downRuntimeWorkspaceAccessLimits(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP TABLE IF EXISTS public.runtime_workspace_end_user_exports;
		ALTER TABLE public.runtime_workspace_credentials
			DROP COLUMN IF EXISTS app_keys,
			DROP COLUMN IF EXISTS scopes;
		DROP INDEX IF EXISTS public.idx_runtime_access_events_created;
	`)
}
