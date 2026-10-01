package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migrationExtendAgentResourceBindingConsumersID = "20260827120000_extend_agent_resource_binding_consumers"
	extendAgentResourceBindingConsumersSQL         = `
		ALTER TABLE public.agent_resource_bindings
			ADD COLUMN IF NOT EXISTS consumer_type varchar(16) NOT NULL DEFAULT 'agent',
			ADD COLUMN IF NOT EXISTS consumer_id uuid,
			ADD COLUMN IF NOT EXISTS binding_instance_id varchar(255) NOT NULL DEFAULT '';

		UPDATE public.agent_resource_bindings
		SET consumer_type = 'agent',
			consumer_id = agent_id,
			binding_instance_id = ''
		WHERE consumer_id IS NULL;

		CREATE TABLE IF NOT EXISTS public.workflow_connector_binding_backfills (
			workflow_id uuid PRIMARY KEY REFERENCES public.workflows(id) ON DELETE CASCADE,
			attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
			last_error text,
			last_attempted_at timestamptz,
			completed_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		-- Queue only Workflows that predate this ledger extension. New graph
		-- writes maintain their binding rows transactionally. Completed rows are
		-- retained as tombstones so a rollback/reapply cannot resurrect a binding
		-- that was deliberately detached after the upgrade.
		INSERT INTO public.workflow_connector_binding_backfills (workflow_id)
		SELECT workflow.id
		FROM public.workflows AS workflow
		WHERE lower(workflow.graph) LIKE '%connector%'
		ON CONFLICT (workflow_id) DO NOTHING;

		CREATE OR REPLACE FUNCTION public.normalize_agent_resource_binding_consumer()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			IF NEW.consumer_type = 'agent' THEN
				NEW.consumer_id := NEW.agent_id;
				NEW.binding_instance_id := '';
			ELSIF NEW.consumer_type = 'workflow' THEN
				IF NEW.consumer_id IS NULL OR btrim(NEW.binding_instance_id) = '' THEN
					RAISE EXCEPTION 'workflow resource binding requires workflow and node identities'
						USING ERRCODE = '23514';
				END IF;
				NEW.binding_instance_id := btrim(NEW.binding_instance_id);
			ELSE
				RAISE EXCEPTION 'unsupported resource binding consumer type'
					USING ERRCODE = '23514';
			END IF;
			RETURN NEW;
		END
		$$;

		DROP TRIGGER IF EXISTS agent_resource_bindings_consumer_guard
			ON public.agent_resource_bindings;
		CREATE TRIGGER agent_resource_bindings_consumer_guard
			BEFORE INSERT OR UPDATE OF agent_id, consumer_type, consumer_id, binding_instance_id
			ON public.agent_resource_bindings
			FOR EACH ROW
			EXECUTE FUNCTION public.normalize_agent_resource_binding_consumer();

		ALTER TABLE public.agent_resource_bindings
			ALTER COLUMN consumer_id SET NOT NULL;
		ALTER TABLE public.agent_resource_bindings
			DROP CONSTRAINT IF EXISTS agent_resource_bindings_consumer_check;
		ALTER TABLE public.agent_resource_bindings
			ADD CONSTRAINT agent_resource_bindings_consumer_check CHECK (
				(consumer_type = 'agent' AND consumer_id = agent_id AND binding_instance_id = '')
				OR
				(consumer_type = 'workflow' AND btrim(binding_instance_id) <> '')
			);

		DROP INDEX IF EXISTS public.idx_agent_resource_bindings_scope_unique;
		DROP INDEX IF EXISTS public.idx_agent_resource_bindings_integration_unique;

		CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_resource_bindings_agent_scope_unique
			ON public.agent_resource_bindings (
				agent_id,
				binding_scope,
				COALESCE(published_version_uuid, '00000000-0000-0000-0000-000000000000'::uuid),
				binding_type,
				resource_id,
				parent_resource_id,
				access_mode
			)
			WHERE consumer_type = 'agent';

		CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_resource_bindings_workflow_scope_unique
			ON public.agent_resource_bindings (
				agent_id,
				consumer_id,
				binding_scope,
				COALESCE(published_version_uuid, '00000000-0000-0000-0000-000000000000'::uuid),
				binding_instance_id,
				binding_type,
				resource_id,
				parent_resource_id,
				access_mode
			)
			WHERE consumer_type = 'workflow';

		CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_resource_bindings_agent_integration_unique
			ON public.agent_resource_bindings (
				agent_id,
				binding_scope,
				COALESCE(published_version_uuid, '00000000-0000-0000-0000-000000000000'::uuid),
				lower(btrim(parent_resource_id))
			)
			WHERE consumer_type = 'agent' AND binding_type = 'integration_connection';

		CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_resource_bindings_workflow_integration_unique
			ON public.agent_resource_bindings (
				agent_id,
				consumer_id,
				binding_scope,
				COALESCE(published_version_uuid, '00000000-0000-0000-0000-000000000000'::uuid),
				binding_instance_id
			)
			WHERE consumer_type = 'workflow' AND binding_type = 'integration_connection';

		CREATE INDEX IF NOT EXISTS idx_agent_resource_bindings_consumer_scope
			ON public.agent_resource_bindings (
				consumer_type,
				consumer_id,
				binding_scope,
				published_version_uuid,
				binding_instance_id
			);
		CREATE INDEX IF NOT EXISTS idx_agent_resource_bindings_workflow_agent_scope
			ON public.agent_resource_bindings (agent_id, binding_scope, published_version_uuid)
			WHERE consumer_type = 'workflow';
	`
)

func init() {
	registerSchemaMigration(
		migrationExtendAgentResourceBindingConsumersID,
		upExtendAgentResourceBindingConsumers,
		downExtendAgentResourceBindingConsumers,
	)
}

func upExtendAgentResourceBindingConsumers(schema *mschema.Builder) error {
	return schema.Raw(extendAgentResourceBindingConsumersSQL)
}

func downExtendAgentResourceBindingConsumers(schema *mschema.Builder) error {
	// Keep the additive identity columns on rollback so Workflow authorization
	// evidence is not silently discarded. Keep the backfill tombstones as well:
	// dropping them would allow a later reapply to recreate deliberately revoked
	// Workflow authorization. Recreate compatibility indexes for Agent rows only;
	// Workflow rows cannot satisfy the legacy global indexes.
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_agent_resource_bindings_consumer_scope;
		DROP INDEX IF EXISTS public.idx_agent_resource_bindings_workflow_agent_scope;
		DROP INDEX IF EXISTS public.idx_agent_resource_bindings_workflow_integration_unique;
		DROP INDEX IF EXISTS public.idx_agent_resource_bindings_agent_integration_unique;
		DROP INDEX IF EXISTS public.idx_agent_resource_bindings_workflow_scope_unique;
		DROP INDEX IF EXISTS public.idx_agent_resource_bindings_agent_scope_unique;
		DROP TRIGGER IF EXISTS agent_resource_bindings_consumer_guard
			ON public.agent_resource_bindings;
		DROP FUNCTION IF EXISTS public.normalize_agent_resource_binding_consumer();
		ALTER TABLE public.agent_resource_bindings
			DROP CONSTRAINT IF EXISTS agent_resource_bindings_consumer_check;

		CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_resource_bindings_scope_unique
			ON public.agent_resource_bindings (
				agent_id,
				binding_scope,
				COALESCE(published_version_uuid, '00000000-0000-0000-0000-000000000000'::uuid),
				binding_type,
				resource_id,
				parent_resource_id,
				access_mode
			)
			WHERE consumer_type = 'agent';
		CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_resource_bindings_integration_unique
			ON public.agent_resource_bindings (
				agent_id,
				binding_scope,
				COALESCE(published_version_uuid, '00000000-0000-0000-0000-000000000000'::uuid),
				parent_resource_id
			)
			WHERE consumer_type = 'agent' AND binding_type = 'integration_connection';
	`)
}
