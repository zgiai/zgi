package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Usage bills name the Runtime Workspace end user a model call was made for
// (ADR 0010), so an end user's points over a window are one indexed sum.
func init() {
	registerSchemaMigration("202610231400000000_meter_runtime_workspace_end_users", upMeterRuntimeWorkspaceEndUsers, downMeterRuntimeWorkspaceEndUsers)
}

func upMeterRuntimeWorkspaceEndUsers(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.llm_usage_bills ADD COLUMN IF NOT EXISTS runtime_workspace_id uuid;
		ALTER TABLE public.llm_usage_bills ADD COLUMN IF NOT EXISTS end_user_subject_id uuid;
		CREATE INDEX IF NOT EXISTS idx_usage_bills_end_user
			ON public.llm_usage_bills (end_user_subject_id, request_created_at)
			WHERE end_user_subject_id IS NOT NULL;
	`)
}

func downMeterRuntimeWorkspaceEndUsers(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_usage_bills_end_user;
		ALTER TABLE public.llm_usage_bills DROP COLUMN IF EXISTS end_user_subject_id;
		ALTER TABLE public.llm_usage_bills DROP COLUMN IF EXISTS runtime_workspace_id;
	`)
}
