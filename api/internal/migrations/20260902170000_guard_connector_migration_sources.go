package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260902170000ID = "20260902170000_guard_connector_migration_sources"

// The source barrier is intentionally independent of rollout row locks: legacy
// writers take source -> inventory, while control-plane transactions take
// rollout -> source -> inventory. BEFORE STATEMENT also covers inserted rows
// and deletes of whole binding/policy sets, not merely previously observed rows.
// Every phase invalidates the source attestation: native connector execution
// ignores this flag, but a later reversal must not resurrect a revoked source.
// A reversal may clear only its own projection's flag, within the same source
// barrier transaction and only after rejecting any pre-existing dirty source.
const guardConnectorMigrationSourcesSQL = `
	ALTER TABLE public.connector_migration_inventory
		ADD COLUMN source_digest varchar(64),
		ADD COLUMN source_changed boolean NOT NULL DEFAULT true,
		ADD CONSTRAINT connector_migration_inventory_source_digest_check
			CHECK (source_digest IS NULL OR source_digest ~ '^[a-f0-9]{64}$');

	CREATE FUNCTION public.connector_legacy_source_digest(p_organization uuid, p_connection uuid)
	RETURNS text LANGUAGE sql STABLE SET timezone TO 'UTC' AS $$
		SELECT encode(sha256(convert_to(jsonb_build_object(
			'connection', to_jsonb(c),
			'policies', COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p.action_id)
				FROM public.integration_action_policies p WHERE p.organization_id = c.organization_id
				AND lower(p.integration_id) = lower(c.integration_id)), '[]'::jsonb),
			'grants', COALESCE((SELECT jsonb_agg(to_jsonb(g) ORDER BY g.id)
				FROM public.integration_connection_grants g WHERE g.organization_id = c.organization_id
				AND g.connection_id = c.id), '[]'::jsonb),
			'bindings', COALESCE((SELECT jsonb_agg(to_jsonb(b) ORDER BY b.id)
				FROM public.agent_resource_bindings b WHERE b.organization_id = c.organization_id
				AND b.binding_type = 'integration_connection' AND lower(b.resource_id) = c.id::text
				AND lower(b.parent_resource_id) = lower(c.integration_id)), '[]'::jsonb),
			'preferences', COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p.id)
				FROM public.aichat_integration_preferences p WHERE p.organization_id = c.organization_id
				AND lower(p.integration_id) = lower(c.integration_id)), '[]'::jsonb)
		)::text, 'UTF8')), 'hex')
		FROM public.integration_connections c
		WHERE c.organization_id = p_organization AND c.id = p_connection AND c.deleted_at IS NULL
	$$;

	CREATE FUNCTION public.lock_connector_legacy_source_statement()
	RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
		PERFORM pg_advisory_xact_lock(726914053);
		IF TG_OP = 'TRUNCATE' THEN
			UPDATE public.connector_migration_inventory SET source_changed = true;
		END IF;
		RETURN NULL;
	END $$;

	-- The host ledger is shared by legacy access and canonical projections.
	-- A caller-supplied native marker alone never exempts a legacy source write.
	-- Canonical ownership is checked without new row locks: source writers must
	-- not invert the rollout -> source -> canonical control-plane lock order.
	CREATE FUNCTION public.connector_native_host_projection_owned(item jsonb, departing boolean)
	RETURNS boolean LANGUAGE sql STABLE AS $$
		SELECT EXISTS (
			SELECT 1 FROM public.connector_connections native
			JOIN public.connector_legacy_identity_mappings mapping ON mapping.connection_id = native.id
				AND mapping.organization_id = native.organization_id AND mapping.owner_account_id = native.owner_account_id
				AND mapping.connector_id = native.connector_id AND mapping.cutover_state = 'connector'
			JOIN public.connector_provider_rollouts rollout ON rollout.connector_id = mapping.connector_id AND rollout.phase = 'connector'
			JOIN public.connector_target_bindings binding ON binding.organization_id = native.organization_id
				AND binding.owner_account_id = native.owner_account_id AND binding.connector_id = native.connector_id
				AND binding.compatibility = 'native'
			WHERE item->>'binding_type' = 'integration_connection'
				AND native.id::text = lower(btrim(item->>'resource_id'))
				AND native.organization_id::text = item->>'organization_id'
				AND native.owner_account_id::text = item->>'authorized_by'
				AND lower(btrim(item->>'parent_resource_id')) IN (native.connector_id, native.provider_id)
				AND coalesce(binding.binding_instance_id, '') = coalesce(item->>'binding_instance_id', '')
				AND ((binding.target_type = 'agent' AND item->>'consumer_type' = 'agent'
					AND binding.target_id = item->>'agent_id' AND binding.target_id = item->>'consumer_id'
					AND item->>'binding_scope' = 'draft' AND item->>'published_version_uuid' IS NULL)
				OR (binding.target_type = 'workflow' AND item->>'consumer_type' = 'workflow'
					AND ((item->>'binding_scope' = 'draft' AND item->>'published_version_uuid' IS NULL AND binding.target_id = item->>'consumer_id')
					OR (item->>'binding_scope' = 'published' AND binding.target_id = item->>'published_version_uuid'))))
				AND ((item->'metadata'->>'connector_projection_compatibility' = 'native' AND binding.connection_id = native.id)
					OR (departing
						AND (item->'metadata'->>'connector_projection_compatibility' = 'native'
							OR (item->'metadata'->>'connector_projection_compatibility' = 'legacy_read_only' AND binding.id::text = item->>'id'))
						AND (binding.status = 'disabled' OR binding.connection_id <> native.id)
						AND (SELECT journal.mutation_type = 'binding_detached' AND journal.tombstone
							FROM public.connector_mutation_journal journal
							WHERE journal.connection_id = native.id AND journal.organization_id = native.organization_id
								AND journal.owner_account_id = native.owner_account_id AND journal.connector_id = native.connector_id
								AND journal.details->>'binding_id' = binding.id::text
								AND journal.mutation_type IN ('binding_upserted', 'binding_detached')
							ORDER BY journal.sequence_number DESC LIMIT 1)))
		)
	$$;

	CREATE FUNCTION public.invalidate_connector_legacy_source_row()
	RETURNS trigger LANGUAGE plpgsql AS $$
	DECLARE
		previous jsonb;
		current_row jsonb;
		item jsonb;
	BEGIN
		IF TG_OP <> 'INSERT' THEN previous := to_jsonb(OLD); END IF;
		IF TG_OP <> 'DELETE' THEN current_row := to_jsonb(NEW); END IF;
		IF previous IS NOT DISTINCT FROM current_row THEN RETURN NULL; END IF;
		IF TG_TABLE_NAME = 'agent_resource_bindings' THEN
			IF (TG_OP = 'INSERT' AND public.connector_native_host_projection_owned(current_row, false))
				OR (TG_OP = 'DELETE' AND public.connector_native_host_projection_owned(previous, true))
				OR (TG_OP = 'UPDATE'
					AND previous->'metadata'->>'connector_projection_compatibility' = 'native'
					AND current_row->'metadata'->>'connector_projection_compatibility' = 'native'
					AND public.connector_native_host_projection_owned(previous, true)
					AND public.connector_native_host_projection_owned(current_row, false))
			THEN RETURN NULL; END IF;
		END IF;
		FOR item IN SELECT value FROM jsonb_array_elements(jsonb_build_array(previous, current_row)) LOOP
			IF item = 'null'::jsonb THEN CONTINUE; END IF;
			UPDATE public.connector_migration_inventory i SET source_changed = true
			WHERE i.organization_id::text = item->>'organization_id'
				AND (
					(TG_TABLE_NAME = 'integration_connections' AND i.connection_id::text = item->>'id') OR
					(TG_TABLE_NAME = 'integration_connection_grants' AND i.connection_id::text = item->>'connection_id') OR
					(TG_TABLE_NAME = 'agent_resource_bindings' AND item->>'binding_type' = 'integration_connection'
						AND i.connection_id::text = lower(item->>'resource_id')) OR
					(TG_TABLE_NAME IN ('integration_action_policies', 'aichat_integration_preferences')
						AND i.provider_id = lower(item->>'integration_id'))
				);
		END LOOP;
		RETURN NULL;
	END $$;

	DO $$ DECLARE source_table text; BEGIN
		FOREACH source_table IN ARRAY ARRAY['integration_connections', 'integration_action_policies',
			'integration_connection_grants', 'agent_resource_bindings', 'aichat_integration_preferences'] LOOP
			EXECUTE format('CREATE TRIGGER connector_source_barrier BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON public.%I FOR EACH STATEMENT EXECUTE FUNCTION public.lock_connector_legacy_source_statement()', source_table);
			EXECUTE format('CREATE TRIGGER connector_source_invalidation AFTER INSERT OR UPDATE OR DELETE ON public.%I FOR EACH ROW EXECUTE FUNCTION public.invalidate_connector_legacy_source_row()', source_table);
		END LOOP;
	END $$;
`

const dropConnectorMigrationSourceGuardSQL = `
	DO $$ DECLARE source_table text; BEGIN
		IF EXISTS (SELECT 1 FROM public.connector_migration_inventory) THEN
			RAISE EXCEPTION 'cannot remove Connector source barriers while inventory exists';
		END IF;
		FOREACH source_table IN ARRAY ARRAY['integration_connections', 'integration_action_policies',
			'integration_connection_grants', 'agent_resource_bindings', 'aichat_integration_preferences'] LOOP
			EXECUTE format('DROP TRIGGER connector_source_invalidation ON public.%I', source_table);
			EXECUTE format('DROP TRIGGER connector_source_barrier ON public.%I', source_table);
		END LOOP;
	END $$;
	DROP FUNCTION public.invalidate_connector_legacy_source_row();
	DROP FUNCTION public.connector_native_host_projection_owned(jsonb, boolean);
	DROP FUNCTION public.lock_connector_legacy_source_statement();
	DROP FUNCTION public.connector_legacy_source_digest(uuid, uuid);
	ALTER TABLE public.connector_migration_inventory
		DROP CONSTRAINT connector_migration_inventory_source_digest_check,
		DROP COLUMN source_changed, DROP COLUMN source_digest;
`

func init() {
	registerSchemaMigration(migration20260902170000ID, up20260902170000, down20260902170000)
}

func up20260902170000(schema *mschema.Builder) error {
	return schema.Raw(guardConnectorMigrationSourcesSQL)
}

func down20260902170000(schema *mschema.Builder) error {
	return schema.Raw(dropConnectorMigrationSourceGuardSQL)
}
