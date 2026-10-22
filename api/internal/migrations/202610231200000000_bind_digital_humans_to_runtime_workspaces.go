package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Runtime Workspaces bind DigitalHumans next to Agents (ADR 0010). A binding
// names its application type, so its application id no longer references
// agents only; the access log records the type of the application called;
// and end-user instances may be issued by a Runtime Workspace.
func init() {
	registerSchemaMigration("202610231200000000_bind_digital_humans_to_runtime_workspaces", upBindDigitalHumansToRuntimeWorkspaces, downBindDigitalHumansToRuntimeWorkspaces)
}

func upBindDigitalHumansToRuntimeWorkspaces(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.runtime_workspace_agents DROP CONSTRAINT IF EXISTS runtime_workspace_agents_agent_id_fkey;
		ALTER TABLE public.runtime_workspace_agents ADD COLUMN IF NOT EXISTS app_type varchar(32) NOT NULL DEFAULT 'agent';
		ALTER TABLE public.runtime_workspace_agents DROP CONSTRAINT IF EXISTS ck_runtime_workspace_agent_app_type;
		ALTER TABLE public.runtime_workspace_agents ADD CONSTRAINT ck_runtime_workspace_agent_app_type CHECK (app_type IN ('agent', 'digital_human'));
		ALTER TABLE public.runtime_workspace_access_events ADD COLUMN IF NOT EXISTS app_type varchar(32);
		ALTER TABLE public.digital_human_end_user_instances DROP CONSTRAINT IF EXISTS digital_human_end_user_instances_issuer_check;
		ALTER TABLE public.digital_human_end_user_instances ADD CONSTRAINT digital_human_end_user_instances_issuer_check
			CHECK (issuer_type IN ('digital_human_api', 'digital_human_webapp', 'runtime_workspace'));
	`)
}

func downBindDigitalHumansToRuntimeWorkspaces(schema *mschema.Builder) error {
	return schema.Raw(`
		DELETE FROM public.digital_human_end_user_instances WHERE issuer_type = 'runtime_workspace';
		ALTER TABLE public.digital_human_end_user_instances DROP CONSTRAINT IF EXISTS digital_human_end_user_instances_issuer_check;
		ALTER TABLE public.digital_human_end_user_instances ADD CONSTRAINT digital_human_end_user_instances_issuer_check
			CHECK (issuer_type IN ('digital_human_api', 'digital_human_webapp'));
		ALTER TABLE public.runtime_workspace_access_events DROP COLUMN IF EXISTS app_type;
		DELETE FROM public.runtime_workspace_agents WHERE app_type <> 'agent';
		ALTER TABLE public.runtime_workspace_agents DROP CONSTRAINT IF EXISTS ck_runtime_workspace_agent_app_type;
		ALTER TABLE public.runtime_workspace_agents DROP COLUMN IF EXISTS app_type;
		ALTER TABLE public.runtime_workspace_agents ADD CONSTRAINT runtime_workspace_agents_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;
	`)
}
