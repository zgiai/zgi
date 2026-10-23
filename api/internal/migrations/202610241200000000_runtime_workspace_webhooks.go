package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// A Runtime Workspace may send its integrating server webhooks about end
// users (ADR 0011): near or at a limit, credits running low, a plan expired.
// The endpoint keeps its signing secret encrypted. Events wait in an outbox
// that a worker delivers with retries, and notices keep an end user from
// being reported twice for the same state.
func init() {
	registerSchemaMigration("202610241200000000_runtime_workspace_webhooks", upRuntimeWorkspaceWebhooks, downRuntimeWorkspaceWebhooks)
}

func upRuntimeWorkspaceWebhooks(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.runtime_workspace_webhooks (
			runtime_workspace_id uuid PRIMARY KEY,
			organization_id uuid NOT NULL,
			url varchar(2048) NOT NULL,
			secret_envelope text NOT NULL,
			events jsonb NOT NULL DEFAULT '[]',
			credits_low_points bigint NOT NULL DEFAULT 0,
			enabled boolean NOT NULL DEFAULT true,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS public.runtime_workspace_webhook_deliveries (
			id uuid PRIMARY KEY,
			runtime_workspace_id uuid NOT NULL,
			subject_id uuid,
			event varchar(32) NOT NULL,
			payload jsonb NOT NULL,
			status varchar(16) NOT NULL DEFAULT 'pending',
			attempts integer NOT NULL DEFAULT 0,
			next_attempt_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_status integer NOT NULL DEFAULT 0,
			last_error varchar(500) NOT NULL DEFAULT '',
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			delivered_at timestamptz,
			CONSTRAINT ck_runtime_workspace_webhook_deliveries_status CHECK (status IN ('pending', 'delivered', 'failed'))
		);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_webhook_deliveries_due
			ON public.runtime_workspace_webhook_deliveries (next_attempt_at)
			WHERE status = 'pending';
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_webhook_deliveries_recent
			ON public.runtime_workspace_webhook_deliveries (runtime_workspace_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_webhook_deliveries_subject
			ON public.runtime_workspace_webhook_deliveries (subject_id)
			WHERE subject_id IS NOT NULL;

		CREATE TABLE IF NOT EXISTS public.runtime_workspace_end_user_notices (
			subject_id uuid NOT NULL,
			kind varchar(32) NOT NULL,
			scope varchar(96) NOT NULL,
			runtime_workspace_id uuid NOT NULL,
			notified_at timestamptz NOT NULL,
			PRIMARY KEY (subject_id, kind, scope)
		);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_end_user_notices_workspace
			ON public.runtime_workspace_end_user_notices (runtime_workspace_id);
	`)
}

func downRuntimeWorkspaceWebhooks(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP TABLE IF EXISTS public.runtime_workspace_end_user_notices;
		DROP TABLE IF EXISTS public.runtime_workspace_webhook_deliveries;
		DROP TABLE IF EXISTS public.runtime_workspace_webhooks;
	`)
}
