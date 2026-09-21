package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260902160000ID = "20260902160000_authorize_connector_reversal_projection"

	// The transaction-local intent is necessary, not sufficient. Both ordinary
	// guards independently verify the exact persisted Personal mapping, current
	// reversing rollout, unchanged host identity and frozen access restrictions.
	authorizeConnectorReversalProjectionSQL = `
		CREATE OR REPLACE FUNCTION public.connector_reversal_projection_authorized(
			old_row public.agent_resource_bindings, new_row public.agent_resource_bindings
		) RETURNS boolean LANGUAGE plpgsql AS $$
		DECLARE
			projection_context jsonb;
			provider_allowed jsonb;
			frozen_allowed jsonb;
			native_allowed jsonb;
			allowed jsonb;
			frozen_binding jsonb;
		BEGIN
			BEGIN
				projection_context := nullif(current_setting('zgi.connector_reversal_projection', true), '')::jsonb;
			EXCEPTION WHEN invalid_text_representation THEN
				RETURN false;
			END;
			IF projection_context IS NULL
				OR old_row.binding_type IS DISTINCT FROM 'integration_connection'
				OR new_row.binding_type IS DISTINCT FROM 'integration_connection'
				OR new_row.authorized_by IS NULL
				OR projection_context->>'canonical_binding_id' IS NULL
				OR new_row.id::text IS DISTINCT FROM projection_context->>'binding_id'
				OR lower(btrim(new_row.resource_id)) IS DISTINCT FROM projection_context->>'connection_id'
				OR lower(btrim(new_row.parent_resource_id)) IS DISTINCT FROM projection_context->>'connector_id'
				OR (to_jsonb(old_row) - ARRAY['metadata', 'updated_at']) IS DISTINCT FROM
					(to_jsonb(new_row) - ARRAY['metadata', 'updated_at'])
				OR NOT coalesce(new_row.metadata->>'connector_projection_compatibility' = 'legacy_read_only', false)
				OR (new_row.metadata - ARRAY['connector_projection_compatibility', 'allowed_action_ids', 'tool_name']) <> '{}'::jsonb
				OR coalesce(new_row.metadata->>'tool_name', '') IS DISTINCT FROM btrim(coalesce(old_row.metadata->>'tool_name', ''))
			THEN RETURN false; END IF;

			SELECT inventory.restriction_snapshot->'allowed_operation_ids',
				binding.compatibility_state->'allowed_operation_ids',
				(SELECT item FROM jsonb_array_elements(inventory.restriction_snapshot->'bindings') item
					WHERE item->>'binding_id' IN (new_row.id::text, binding.id::text) LIMIT 1)
			INTO provider_allowed, native_allowed, frozen_binding
			FROM public.connector_provider_rollouts rollout
			JOIN public.connector_legacy_identity_mappings mapping ON mapping.connector_id = rollout.connector_id
			JOIN public.connector_connections native ON native.id = mapping.connection_id
				AND native.organization_id = mapping.organization_id AND native.owner_account_id = mapping.owner_account_id
				AND native.connector_id = mapping.connector_id
			JOIN public.integration_connections legacy ON legacy.id = mapping.connection_id
				AND legacy.organization_id = mapping.organization_id AND legacy.owner_account_id = mapping.owner_account_id
			JOIN public.connector_migration_inventory inventory ON inventory.connection_id = mapping.connection_id
				AND inventory.organization_id = mapping.organization_id AND inventory.owner_account_id = mapping.owner_account_id
				AND inventory.connector_id = mapping.connector_id AND inventory.restriction_digest = mapping.restriction_digest
			JOIN public.connector_target_bindings binding ON binding.id::text = projection_context->>'canonical_binding_id'
				AND binding.connection_id = native.id AND binding.organization_id = native.organization_id
				AND binding.owner_account_id = native.owner_account_id AND binding.connector_id = native.connector_id
			WHERE rollout.connector_id = projection_context->>'connector_id' AND rollout.phase = 'reversing'
				AND rollout.revision::text = projection_context->>'rollout_revision' AND NOT rollout.reversal_materialized
				AND rollout.in_flight_connector_claims = 0 AND rollout.in_flight_reconnects = 0
				AND mapping.connection_id::text = projection_context->>'connection_id'
				AND mapping.organization_id = new_row.organization_id AND mapping.owner_account_id = new_row.authorized_by
				AND mapping.cutover_state IN ('connector', 'shadow') AND inventory.eligibility = 'backfilled'
				AND native.deleted_at IS NULL AND legacy.deleted_at IS NULL AND legacy.credential_source = 'account'
				AND native.provider_id = mapping.legacy_provider_id AND native.driver_id = mapping.legacy_driver_id
				AND lower(legacy.integration_id) = native.provider_id AND lower(legacy.driver_id) = native.driver_id
				AND lower(legacy.auth_method_id) = native.auth_method_id
				AND legacy.credential_version = native.credential_version AND legacy.encrypted_credentials = native.encrypted_credentials
				AND legacy.status = CASE native.state WHEN 'ready' THEN 'active' WHEN 'reconnect_required' THEN 'invalid' ELSE 'disabled' END
				AND binding.status = 'active' AND binding.compatibility = 'native'
				AND NOT EXISTS (SELECT 1 FROM public.connector_target_bindings other_binding
					WHERE other_binding.id <> binding.id AND other_binding.connection_id = binding.connection_id
						AND other_binding.organization_id = binding.organization_id AND other_binding.owner_account_id = binding.owner_account_id
						AND other_binding.connector_id = binding.connector_id AND other_binding.target_type = binding.target_type
						AND other_binding.target_id = binding.target_id AND coalesce(other_binding.binding_instance_id, '') = coalesce(binding.binding_instance_id, '')
						AND other_binding.status = 'active' AND other_binding.compatibility = 'native')
				AND NOT EXISTS (SELECT 1 FROM public.agent_resource_bindings other_ledger
					WHERE other_ledger.id <> new_row.id AND other_ledger.organization_id = new_row.organization_id
						AND other_ledger.authorized_by = new_row.authorized_by AND other_ledger.binding_type = 'integration_connection'
						AND lower(btrim(other_ledger.resource_id)) = lower(btrim(new_row.resource_id))
						AND lower(btrim(other_ledger.parent_resource_id)) = lower(btrim(new_row.parent_resource_id))
						AND other_ledger.consumer_type = new_row.consumer_type AND other_ledger.binding_scope = new_row.binding_scope
						AND coalesce(other_ledger.binding_instance_id, '') = new_row.binding_instance_id
						AND ((new_row.binding_scope = 'draft' AND other_ledger.published_version_uuid IS NULL AND other_ledger.consumer_id = new_row.consumer_id)
							OR (new_row.binding_scope = 'published' AND other_ledger.published_version_uuid = new_row.published_version_uuid)))
				AND coalesce(binding.binding_instance_id, '') = new_row.binding_instance_id
				AND ((binding.target_type = 'agent' AND new_row.consumer_type = 'agent'
					AND binding.target_id = new_row.agent_id::text AND binding.target_id = new_row.consumer_id::text
					AND new_row.binding_scope = 'draft' AND new_row.published_version_uuid IS NULL)
				OR (binding.target_type = 'workflow' AND new_row.consumer_type = 'workflow'
					AND ((new_row.binding_scope = 'draft' AND new_row.published_version_uuid IS NULL AND binding.target_id = new_row.consumer_id::text)
					OR (new_row.binding_scope = 'published' AND binding.target_id = new_row.published_version_uuid::text))))
			FOR SHARE OF rollout, mapping, native, legacy, inventory, binding;
			IF NOT FOUND THEN RETURN false; END IF;
			allowed := new_row.metadata->'allowed_action_ids';
			IF jsonb_typeof(allowed) IS DISTINCT FROM 'array' OR jsonb_typeof(provider_allowed) IS DISTINCT FROM 'array' THEN
				RETURN false;
			END IF;
			IF jsonb_array_length(allowed) = 0 OR NOT (provider_allowed @> allowed)
				OR EXISTS (SELECT 1 FROM jsonb_array_elements(allowed) value WHERE jsonb_typeof(value) <> 'string')
			THEN RETURN false; END IF;
			IF frozen_binding IS NOT NULL THEN
				IF new_row.id::text IS DISTINCT FROM projection_context->>'canonical_binding_id' THEN RETURN false; END IF;
				frozen_allowed := frozen_binding->'allowed_operation_ids';
				IF jsonb_typeof(frozen_allowed) IS DISTINCT FROM 'array' OR NOT (frozen_allowed @> allowed)
					OR frozen_binding->>'access_mode' IS DISTINCT FROM new_row.access_mode
					OR frozen_binding->>'audience' IS DISTINCT FROM 'owner_only'
					OR frozen_binding->>'status' IS DISTINCT FROM 'active'
					OR frozen_binding->>'target_type' IS DISTINCT FROM new_row.consumer_type
					OR coalesce(frozen_binding->>'binding_instance_id', '') IS DISTINCT FROM new_row.binding_instance_id
					OR frozen_binding->>'target_id' IS DISTINCT FROM
						(CASE WHEN new_row.binding_scope = 'published' THEN new_row.published_version_uuid::text ELSE new_row.consumer_id::text END)
				THEN RETURN false; END IF;
			ELSE
				frozen_allowed := provider_allowed;
			END IF;
			IF native_allowed IS NOT NULL THEN
				IF jsonb_typeof(native_allowed) IS DISTINCT FROM 'array' OR jsonb_array_length(native_allowed) = 0 THEN
					RETURN false;
				END IF;
				RETURN allowed = native_allowed;
			END IF;
			RETURN allowed = frozen_allowed;
		END $$;

		CREATE OR REPLACE FUNCTION public.enforce_agent_integration_connection_binding()
		RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF TG_OP = 'UPDATE' AND nullif(current_setting('zgi.connector_reversal_projection', true), '') IS NOT NULL THEN
				IF public.connector_reversal_projection_authorized(OLD, NEW) THEN RETURN NEW; END IF;
				RAISE EXCEPTION 'connector reversal projection is not authorized' USING ERRCODE = '23503';
			END IF;
			IF NEW.binding_type = 'integration_connection' THEN
				IF NEW.metadata->>'connector_projection_compatibility' = 'native' THEN
					IF NEW.authorized_by IS NOT NULL THEN
						PERFORM 1 FROM public.connector_connections AS connection
						JOIN public.connector_definitions AS definition ON definition.connector_id = connection.connector_id
						WHERE connection.id::text = lower(btrim(NEW.resource_id)) AND connection.organization_id = NEW.organization_id
							AND connection.owner_account_id = NEW.authorized_by
							AND (connection.connector_id = lower(btrim(NEW.parent_resource_id)) OR connection.provider_id = lower(btrim(NEW.parent_resource_id)))
							AND connection.state = 'ready' AND connection.deleted_at IS NULL AND definition.status = 'active'
						FOR SHARE OF connection;
					END IF;
				ELSE
					PERFORM 1 FROM public.integration_connections AS connection
					WHERE connection.id::text = lower(btrim(NEW.resource_id)) AND connection.organization_id = NEW.organization_id
						AND lower(connection.integration_id) = lower(btrim(NEW.parent_resource_id)) AND connection.status = 'active'
						AND connection.deleted_at IS NULL AND (connection.expires_at IS NULL OR connection.expires_at > CURRENT_TIMESTAMP)
					FOR SHARE;
				END IF;
				IF NOT FOUND THEN RAISE EXCEPTION 'integration connection binding is unavailable' USING ERRCODE = '23503'; END IF;
			END IF;
			RETURN NEW;
		END $$;

		CREATE OR REPLACE FUNCTION public.enforce_legacy_agent_integration_projection_read_only()
		RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF public.connector_reversal_projection_authorized(OLD, NEW) THEN RETURN NEW; END IF;
			IF OLD.binding_type = 'integration_connection'
				AND OLD.metadata->>'connector_projection_compatibility' = 'legacy_read_only'
				AND (NEW.organization_id IS DISTINCT FROM OLD.organization_id OR NEW.workspace_id IS DISTINCT FROM OLD.workspace_id
					OR NEW.agent_id IS DISTINCT FROM OLD.agent_id OR NEW.consumer_type IS DISTINCT FROM OLD.consumer_type
					OR NEW.consumer_id IS DISTINCT FROM OLD.consumer_id OR NEW.binding_instance_id IS DISTINCT FROM OLD.binding_instance_id
					OR NEW.binding_scope IS DISTINCT FROM OLD.binding_scope OR NEW.published_version_uuid IS DISTINCT FROM OLD.published_version_uuid
					OR NEW.binding_type IS DISTINCT FROM OLD.binding_type OR NEW.resource_id IS DISTINCT FROM OLD.resource_id
					OR NEW.parent_resource_id IS DISTINCT FROM OLD.parent_resource_id OR NEW.access_mode IS DISTINCT FROM OLD.access_mode
					OR NEW.metadata IS DISTINCT FROM OLD.metadata OR NEW.authorized_by IS DISTINCT FROM OLD.authorized_by
					OR NEW.authorized_at IS DISTINCT FROM OLD.authorized_at) THEN
				RAISE EXCEPTION 'legacy integration projection is read-only' USING ERRCODE = '55000';
			END IF;
			RETURN NEW;
		END $$;
	`
)

func init() {
	registerSchemaMigration(migration20260902160000ID, up20260902160000, down20260902160000)
}

func up20260902160000(schema *mschema.Builder) error {
	return schema.Raw(authorizeConnectorReversalProjectionSQL)
}

func down20260902160000(schema *mschema.Builder) error {
	return schema.Raw(hardenConnectorAgentBindingProjectionSQL + `
		DROP FUNCTION IF EXISTS public.connector_reversal_projection_authorized(public.agent_resource_bindings, public.agent_resource_bindings);
	`)
}
