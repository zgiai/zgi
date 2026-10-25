package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Runtime Workspaces keep a registry of their end users (ADR 0010), so the
// console and the integrating server can list, cap, suspend and erase them.
// The registry holds the external user ID the integrator sent, which no
// other table stores; erasure clears it and keeps only identifiers.
// Every change to an end user is an event with identifiers and enums only.
func init() {
	registerSchemaMigration("202610231500000000_runtime_workspace_end_users", upRuntimeWorkspaceEndUsers, downRuntimeWorkspaceEndUsers)
}

func upRuntimeWorkspaceEndUsers(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.runtime_workspace_end_users (
			subject_id uuid PRIMARY KEY,
			runtime_workspace_id uuid NOT NULL,
			external_user_id varchar(255) NOT NULL DEFAULT '',
			status varchar(16) NOT NULL DEFAULT 'active',
			quota_json jsonb NOT NULL DEFAULT '{}',
			first_seen_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_seen_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			suspended_at timestamptz,
			erased_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT ck_runtime_workspace_end_users_status CHECK (status IN ('active', 'suspended', 'erased'))
		);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_end_users_recent
			ON public.runtime_workspace_end_users (runtime_workspace_id, last_seen_at DESC);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_end_users_external
			ON public.runtime_workspace_end_users (runtime_workspace_id, external_user_id);

		CREATE TABLE IF NOT EXISTS public.runtime_workspace_end_user_events (
			id uuid PRIMARY KEY,
			runtime_workspace_id uuid NOT NULL,
			subject_id uuid NOT NULL,
			action varchar(32) NOT NULL,
			actor_kind varchar(16) NOT NULL,
			actor_id uuid,
			details jsonb NOT NULL DEFAULT '{}',
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_end_user_events_subject
			ON public.runtime_workspace_end_user_events (subject_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_end_user_events_runtime
			ON public.runtime_workspace_end_user_events (runtime_workspace_id, created_at DESC);

		-- End users seen before the registry have no stored external user ID;
		-- they are listed by subject until their next request names them.
		INSERT INTO public.runtime_workspace_end_users (subject_id, runtime_workspace_id, first_seen_at, last_seen_at, created_at, updated_at)
		SELECT s.id, s.issuer_id, s.created_at,
			COALESCE((SELECT max(e.created_at) FROM public.runtime_workspace_access_events e WHERE e.subject_id = s.id), s.updated_at),
			s.created_at, s.updated_at
		FROM public.user_subjects s
		WHERE s.kind = 'api_end_user' AND s.issuer_type = 'runtime_workspace_api' AND s.issuer_id IS NOT NULL
		ON CONFLICT (subject_id) DO NOTHING;
	`)
}

func downRuntimeWorkspaceEndUsers(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP TABLE IF EXISTS public.runtime_workspace_end_user_events;
		DROP TABLE IF EXISTS public.runtime_workspace_end_users;
	`)
}
