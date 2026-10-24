package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// A Runtime Workspace end user may hold one plan of the workspace's policy,
// with an optional expiry (ADR 0011). The plan is a key into the policy, so
// editing a plan changes every end user on it; an expired or removed plan
// no longer applies.
func init() {
	registerSchemaMigration("202610241000000000_runtime_workspace_end_user_plans", upRuntimeWorkspaceEndUserPlans, downRuntimeWorkspaceEndUserPlans)
}

func upRuntimeWorkspaceEndUserPlans(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.runtime_workspace_end_users
			ADD COLUMN IF NOT EXISTS plan_key varchar(64) NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS plan_expires_at timestamptz;
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_end_users_plan
			ON public.runtime_workspace_end_users (runtime_workspace_id, plan_key)
			WHERE plan_key <> '';
	`)
}

func downRuntimeWorkspaceEndUserPlans(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_runtime_workspace_end_users_plan;
		ALTER TABLE public.runtime_workspace_end_users
			DROP COLUMN IF EXISTS plan_expires_at,
			DROP COLUMN IF EXISTS plan_key;
	`)
}
