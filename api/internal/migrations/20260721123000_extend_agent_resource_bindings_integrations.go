package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migrationExtendAgentResourceBindingsIntegrationsID = "20260721123000_extend_agent_resource_bindings_integrations"
	extendAgentResourceBindingsIntegrationsSQL         = `
		ALTER TABLE public.agent_resource_bindings
			ADD COLUMN IF NOT EXISTS metadata jsonb NOT NULL DEFAULT '{}'::jsonb;

		ALTER TABLE public.agent_resource_bindings
			DROP CONSTRAINT IF EXISTS agent_resource_bindings_type_check;
		ALTER TABLE public.agent_resource_bindings
			ADD CONSTRAINT agent_resource_bindings_type_check
			CHECK (binding_type IN ('skill', 'knowledge_dataset', 'database', 'database_table', 'workflow', 'integration_connection'));

		CREATE OR REPLACE FUNCTION public.enforce_agent_integration_connection_binding()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			IF NEW.binding_type = 'integration_connection' THEN
				PERFORM 1
				FROM public.integration_connections AS connection
				WHERE connection.id::text = lower(trim(NEW.resource_id))
					AND connection.organization_id = NEW.organization_id
					AND lower(connection.integration_id) = lower(trim(NEW.parent_resource_id))
					AND connection.status = 'active'
					AND connection.deleted_at IS NULL
					AND (connection.expires_at IS NULL OR connection.expires_at > CURRENT_TIMESTAMP)
				FOR SHARE;
				IF NOT FOUND THEN
					RAISE EXCEPTION 'integration connection binding is unavailable'
						USING ERRCODE = '23503';
				END IF;
			END IF;
			RETURN NEW;
		END
		$$;

		DROP TRIGGER IF EXISTS agent_resource_bindings_integration_connection_guard
			ON public.agent_resource_bindings;
		CREATE TRIGGER agent_resource_bindings_integration_connection_guard
			BEFORE INSERT OR UPDATE OF organization_id, binding_type, resource_id, parent_resource_id
			ON public.agent_resource_bindings
			FOR EACH ROW
			EXECUTE FUNCTION public.enforce_agent_integration_connection_binding();

		CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_resource_bindings_integration_unique
			ON public.agent_resource_bindings (
				agent_id,
				binding_scope,
				COALESCE(published_version_uuid, '00000000-0000-0000-0000-000000000000'::uuid),
				parent_resource_id
			)
			WHERE binding_type = 'integration_connection'
	`
)

func init() {
	registerSchemaMigration(
		migrationExtendAgentResourceBindingsIntegrationsID,
		upExtendAgentResourceBindingsIntegrations,
		downExtendAgentResourceBindingsIntegrations,
	)
}

func upExtendAgentResourceBindingsIntegrations(schema *mschema.Builder) error {
	return schema.Raw(extendAgentResourceBindingsIntegrationsSQL)
}

func downExtendAgentResourceBindingsIntegrations(schema *mschema.Builder) error {
	// Keep the additive metadata column and expanded type constraint on a
	// rollback so existing authorization evidence is never discarded.
	return schema.Raw(`
		DROP TRIGGER IF EXISTS agent_resource_bindings_integration_connection_guard
			ON public.agent_resource_bindings;
		DROP FUNCTION IF EXISTS public.enforce_agent_integration_connection_binding();
		DROP INDEX IF EXISTS public.idx_agent_resource_bindings_integration_unique
	`)
}
