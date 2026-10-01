package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260901100000ID                = "20260901100000_harden_connector_agent_binding_projection"
	hardenConnectorAgentBindingProjectionSQL = `
		CREATE OR REPLACE FUNCTION public.enforce_agent_integration_connection_binding()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			IF NEW.binding_type = 'integration_connection' THEN
				IF NEW.metadata->>'connector_projection_compatibility' = 'native' THEN
					-- Connector-native projection is valid only for one ready active
					-- Personal Connection owned by the authorizing Acting Principal.
					-- It never falls through to the legacy organization/shared path.
					IF NEW.authorized_by IS NOT NULL THEN
						PERFORM 1
						FROM public.connector_connections AS connection
						JOIN public.connector_definitions AS definition
							ON definition.connector_id = connection.connector_id
						WHERE connection.id::text = lower(btrim(NEW.resource_id))
							AND connection.organization_id = NEW.organization_id
							AND connection.owner_account_id = NEW.authorized_by
							AND (
								connection.connector_id = lower(btrim(NEW.parent_resource_id))
								OR connection.provider_id = lower(btrim(NEW.parent_resource_id))
							)
							AND connection.state = 'ready'
							AND connection.deleted_at IS NULL
							AND definition.status = 'active'
						FOR SHARE OF connection;
					END IF;
				ELSE
					-- Preserve the legacy predicate exactly. In particular, this
					-- branch does not reinterpret organization/shared Connections as
					-- Personal Connector Connections or change their existing ACLs.
					PERFORM 1
					FROM public.integration_connections AS connection
					WHERE connection.id::text = lower(btrim(NEW.resource_id))
						AND connection.organization_id = NEW.organization_id
						AND lower(connection.integration_id) = lower(btrim(NEW.parent_resource_id))
						AND connection.status = 'active'
						AND connection.deleted_at IS NULL
						AND (connection.expires_at IS NULL OR connection.expires_at > CURRENT_TIMESTAMP)
					FOR SHARE;
				END IF;
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
			BEFORE INSERT OR UPDATE OF organization_id, binding_type, resource_id, parent_resource_id, metadata, authorized_by
			ON public.agent_resource_bindings
			FOR EACH ROW
			EXECUTE FUNCTION public.enforce_agent_integration_connection_binding();

		CREATE OR REPLACE FUNCTION public.enforce_legacy_agent_integration_projection_read_only()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			IF OLD.binding_type = 'integration_connection'
				AND OLD.metadata->>'connector_projection_compatibility' = 'legacy_read_only'
				AND (
					NEW.organization_id IS DISTINCT FROM OLD.organization_id
					OR NEW.workspace_id IS DISTINCT FROM OLD.workspace_id
					OR NEW.agent_id IS DISTINCT FROM OLD.agent_id
					OR NEW.consumer_type IS DISTINCT FROM OLD.consumer_type
					OR NEW.consumer_id IS DISTINCT FROM OLD.consumer_id
					OR NEW.binding_instance_id IS DISTINCT FROM OLD.binding_instance_id
					OR NEW.binding_scope IS DISTINCT FROM OLD.binding_scope
					OR NEW.published_version_uuid IS DISTINCT FROM OLD.published_version_uuid
					OR NEW.binding_type IS DISTINCT FROM OLD.binding_type
					OR NEW.resource_id IS DISTINCT FROM OLD.resource_id
					OR NEW.parent_resource_id IS DISTINCT FROM OLD.parent_resource_id
					OR NEW.access_mode IS DISTINCT FROM OLD.access_mode
					OR NEW.metadata IS DISTINCT FROM OLD.metadata
					OR NEW.authorized_by IS DISTINCT FROM OLD.authorized_by
					OR NEW.authorized_at IS DISTINCT FROM OLD.authorized_at
				) THEN
				RAISE EXCEPTION 'legacy integration projection is read-only'
					USING ERRCODE = '55000';
			END IF;
			RETURN NEW;
		END
		$$;

		DROP TRIGGER IF EXISTS agent_resource_bindings_legacy_projection_read_only_guard
			ON public.agent_resource_bindings;
		CREATE TRIGGER agent_resource_bindings_legacy_projection_read_only_guard
			BEFORE UPDATE OF organization_id, workspace_id, agent_id, consumer_type, consumer_id,
				binding_instance_id, binding_scope, published_version_uuid, binding_type,
				resource_id, parent_resource_id, access_mode, metadata, authorized_by, authorized_at
			ON public.agent_resource_bindings
			FOR EACH ROW
			EXECUTE FUNCTION public.enforce_legacy_agent_integration_projection_read_only();
	`
	downHardenConnectorAgentBindingProjectionSQL = `
		DROP TRIGGER IF EXISTS agent_resource_bindings_legacy_projection_read_only_guard
			ON public.agent_resource_bindings;
		DROP FUNCTION IF EXISTS public.enforce_legacy_agent_integration_projection_read_only();

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
				IF FOUND THEN
					RETURN NEW;
				END IF;

				PERFORM 1
				FROM public.connector_connections AS connection
				JOIN public.connector_definitions AS definition
					ON definition.connector_id = connection.connector_id
				WHERE connection.id::text = lower(trim(NEW.resource_id))
					AND connection.organization_id = NEW.organization_id
					AND connection.owner_account_id = NEW.authorized_by
					AND (
						connection.connector_id = lower(trim(NEW.parent_resource_id))
						OR connection.provider_id = lower(trim(NEW.parent_resource_id))
					)
					AND connection.state = 'ready'
					AND connection.deleted_at IS NULL
					AND definition.status = 'active'
				FOR SHARE OF connection;
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
	`
)

func init() {
	registerSchemaMigration(migration20260901100000ID, up20260901100000, down20260901100000)
}

func up20260901100000(schema *mschema.Builder) error {
	return schema.Raw(hardenConnectorAgentBindingProjectionSQL)
}

func down20260901100000(schema *mschema.Builder) error {
	return schema.Raw(downHardenConnectorAgentBindingProjectionSQL)
}
