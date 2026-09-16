package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260902100000ID            = "20260902100000_create_connector_plugin_catalog_state"
	createConnectorPluginCatalogStateSQL = `
		CREATE TABLE public.connector_plugin_versions (
			plugin_id varchar(128) NOT NULL,
			plugin_version varchar(64) NOT NULL,
			plugin_digest varchar(128) NOT NULL,
			source_dialect varchar(32) NOT NULL,
			schema_version varchar(32) NOT NULL,
			status varchar(32) NOT NULL,
			accepted_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			activated_at timestamptz,
			retired_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (plugin_id, plugin_version, plugin_digest),
			CONSTRAINT connector_plugin_versions_plugin_id_check
				CHECK (plugin_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_plugin_versions_digest_check
				CHECK (plugin_digest ~ '^[a-f0-9]{64}$'),
			CONSTRAINT connector_plugin_versions_source_check
				CHECK (source_dialect = 'agentplugins-v1' AND schema_version = '1.0.0'),
			CONSTRAINT connector_plugin_versions_status_check
				CHECK (status IN ('active', 'superseded', 'disabled', 'unavailable'))
		);

		CREATE UNIQUE INDEX idx_connector_plugin_versions_one_active
			ON public.connector_plugin_versions (plugin_id)
			WHERE status = 'active';

		CREATE TABLE public.connector_definition_versions (
			connector_id varchar(128) NOT NULL,
			plugin_id varchar(128) NOT NULL,
			plugin_version varchar(64) NOT NULL,
			plugin_digest varchar(128) NOT NULL,
			contract_digest varchar(128) NOT NULL,
			definition jsonb NOT NULL,
			status varchar(32) NOT NULL,
			accepted_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			activated_at timestamptz,
			retired_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (connector_id, plugin_version, plugin_digest),
			CONSTRAINT connector_definition_versions_plugin_fk
				FOREIGN KEY (plugin_id, plugin_version, plugin_digest)
				REFERENCES public.connector_plugin_versions(plugin_id, plugin_version, plugin_digest)
				ON DELETE RESTRICT,
			CONSTRAINT connector_definition_versions_connector_id_check
				CHECK (connector_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_definition_versions_digests_check
				CHECK (plugin_digest ~ '^[a-f0-9]{64}$' AND contract_digest ~ '^[a-f0-9]{64}$'),
			CONSTRAINT connector_definition_versions_definition_check
				CHECK (jsonb_typeof(definition) = 'object'),
			CONSTRAINT connector_definition_versions_status_check
				CHECK (status IN ('active', 'superseded', 'disabled', 'unavailable'))
		);

		CREATE UNIQUE INDEX idx_connector_definition_versions_one_active
			ON public.connector_definition_versions (connector_id)
			WHERE status = 'active';
		CREATE INDEX idx_connector_definition_versions_plugin_history
			ON public.connector_definition_versions (plugin_id, accepted_at DESC, connector_id);

		-- Preserve Connector definitions accepted before version history became
		-- durable. The unique active indexes deliberately make an inconsistent
		-- pre-existing package fail migration instead of selecting a version.
		INSERT INTO public.connector_plugin_versions (
			plugin_id, plugin_version, plugin_digest, source_dialect, schema_version,
			status, accepted_at, activated_at, retired_at, created_at, updated_at
		)
		SELECT
			plugin_id, plugin_version, plugin_digest, source_dialect, schema_version,
			status, MIN(accepted_at),
			CASE WHEN status = 'active' THEN MIN(accepted_at) ELSE NULL END,
			CASE WHEN status <> 'active' THEN MAX(updated_at) ELSE NULL END,
			MIN(created_at), MAX(updated_at)
		FROM public.connector_definitions
		GROUP BY plugin_id, plugin_version, plugin_digest, source_dialect, schema_version, status;

		INSERT INTO public.connector_definition_versions (
			connector_id, plugin_id, plugin_version, plugin_digest, contract_digest,
			definition, status, accepted_at, activated_at, retired_at, created_at, updated_at
		)
		SELECT
			connector_id, plugin_id, plugin_version, plugin_digest, contract_digest,
			jsonb_build_object(
				'id', connector_id,
				'plugin_id', plugin_id,
				'plugin_version', plugin_version,
				'source_dialect', source_dialect,
				'source_schema_version', schema_version,
				'provider_id', provider_id,
				'driver_id', driver_id,
				'auth_method_id', key_schema->>'auth_method_id',
				'name', display->>'name',
				'description', display->>'description',
				'icon', COALESCE(display->>'icon', ''),
				'classification', classification,
				'capability_types', COALESCE(capability_summary->'types', '[]'::jsonb),
				'setup_method', key_schema->>'setup_method',
				'ownership', key_schema->>'ownership',
				'surfaces', COALESCE(capability_summary->'surfaces', '[]'::jsonb),
				'key_fields', COALESCE(key_schema->'fields', '[]'::jsonb),
				'operations', COALESCE(capability_summary->'operations', '[]'::jsonb)
			),
			status, accepted_at,
			CASE WHEN status = 'active' THEN accepted_at ELSE NULL END,
			CASE WHEN status <> 'active' THEN updated_at ELSE NULL END,
			created_at, updated_at
		FROM public.connector_definitions;
	`
	dropConnectorPluginCatalogStateSQL = `
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM public.connector_definition_versions LIMIT 1)
				OR EXISTS (SELECT 1 FROM public.connector_plugin_versions LIMIT 1) THEN
				RAISE EXCEPTION 'cannot remove Connector plugin catalog state while accepted versions exist';
			END IF;
		END $$;
		DROP TABLE IF EXISTS public.connector_definition_versions;
		DROP TABLE IF EXISTS public.connector_plugin_versions;
	`
)

func init() {
	registerSchemaMigration(migration20260902100000ID, up20260902100000, down20260902100000)
}

func up20260902100000(schema *mschema.Builder) error {
	return schema.Raw(createConnectorPluginCatalogStateSQL)
}

func down20260902100000(schema *mschema.Builder) error {
	return schema.Raw(dropConnectorPluginCatalogStateSQL)
}
