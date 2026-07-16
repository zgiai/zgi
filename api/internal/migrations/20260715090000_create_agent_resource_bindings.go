package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migrationCreateAgentResourceBindingsID = "20260715090000_create_agent_resource_bindings"
	createAgentResourceBindingsSQL         = `
		CREATE TABLE IF NOT EXISTS public.agent_resource_bindings (
			id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
			agent_id uuid NOT NULL REFERENCES public.agents(id) ON DELETE CASCADE,
			binding_scope varchar(16) NOT NULL,
			organization_id uuid NOT NULL,
			workspace_id uuid NOT NULL,
			published_version_uuid uuid,
			binding_type varchar(32) NOT NULL,
			resource_id varchar(255) NOT NULL,
			parent_resource_id varchar(255) NOT NULL DEFAULT '',
			display_name varchar(255) NOT NULL DEFAULT '',
			access_mode varchar(16) NOT NULL DEFAULT '',
			authorized_by uuid,
			authorized_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT agent_resource_bindings_type_check
				CHECK (binding_type IN ('skill', 'knowledge_dataset', 'database', 'database_table', 'workflow')),
			CONSTRAINT agent_resource_bindings_scope_check
				CHECK (binding_scope IN ('draft', 'published')),
			CONSTRAINT agent_resource_bindings_version_scope_check
				CHECK ((binding_scope = 'draft' AND published_version_uuid IS NULL) OR (binding_scope = 'published' AND published_version_uuid IS NOT NULL))
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_resource_bindings_scope_unique
			ON public.agent_resource_bindings (
				agent_id,
				binding_scope,
				COALESCE(published_version_uuid, '00000000-0000-0000-0000-000000000000'::uuid),
				binding_type,
				resource_id,
				parent_resource_id,
				access_mode
			);
		CREATE INDEX IF NOT EXISTS idx_agent_resource_bindings_agent_scope
			ON public.agent_resource_bindings (agent_id, binding_scope, published_version_uuid);
		CREATE INDEX IF NOT EXISTS idx_agent_resource_bindings_resource
			ON public.agent_resource_bindings (organization_id, workspace_id, binding_type, resource_id)
		;
		CREATE OR REPLACE FUNCTION pg_temp.safe_agent_binding_jsonb(value text)
		RETURNS jsonb
		LANGUAGE plpgsql
		AS $$
		BEGIN
			RETURN COALESCE(NULLIF(btrim(value), ''), '{}')::jsonb;
		EXCEPTION WHEN others THEN
			RETURN '{}'::jsonb;
		END;
		$$;

		WITH latest_published AS (
			SELECT DISTINCT ON (published.agent_id)
				published.agent_id,
				published.workspace_id,
				published.version_uuid,
				published.config_snapshot,
				published.created_by,
				published.created_at
			FROM public.agent_published_versions AS published
			JOIN public.agents AS agent ON agent.id = published.agent_id
			WHERE published.deleted_at IS NULL
				AND agent.deleted_at IS NULL
				AND agent.agent_type = 'AGENT'
			ORDER BY published.agent_id, published.created_at DESC, published.id DESC
		), raw_sources AS (
			SELECT
				agent.id AS agent_id,
				'draft'::varchar AS binding_scope,
				COALESCE(workspace.organization_id, agent.tenant_id) AS organization_id,
				agent.tenant_id AS workspace_id,
				NULL::uuid AS published_version_uuid,
				pg_temp.safe_agent_binding_jsonb(config.agent_mode) AS binding_config,
				COALESCE(config.updated_by, config.created_by, agent.updated_by, agent.created_by) AS default_authorized_by,
				COALESCE(config.updated_at, config.created_at, agent.updated_at, agent.created_at) AS default_authorized_at
			FROM public.agents AS agent
			JOIN LATERAL (
				SELECT candidate.*
				FROM public.agents_configs AS candidate
				WHERE candidate.agents_id = agent.id AND candidate.deleted_at IS NULL
				ORDER BY (candidate.id = agent.agents_model_config_id) DESC NULLS LAST,
					candidate.updated_at DESC, candidate.created_at DESC, candidate.id DESC
				LIMIT 1
			) AS config ON true
			LEFT JOIN public.workspaces AS workspace ON workspace.id::text = agent.tenant_id::text
			WHERE agent.deleted_at IS NULL AND agent.agent_type = 'AGENT'
			UNION ALL
			SELECT
				published.agent_id,
				'published'::varchar AS binding_scope,
				COALESCE(workspace.organization_id, published.workspace_id) AS organization_id,
				published.workspace_id,
				published.version_uuid AS published_version_uuid,
				published.config_snapshot AS binding_config,
				published.created_by AS default_authorized_by,
				published.created_at AS default_authorized_at
			FROM latest_published AS published
			LEFT JOIN public.workspaces AS workspace ON workspace.id::text = published.workspace_id::text
		), binding_sources AS (
			SELECT
				raw_sources.*,
				CASE WHEN binding_config->>'knowledge_bound_by_account_id' ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
					THEN (binding_config->>'knowledge_bound_by_account_id')::uuid ELSE default_authorized_by END AS knowledge_authorized_by,
				CASE WHEN binding_config->>'knowledge_bound_at_unix' ~ '^[0-9]{1,10}$' AND (binding_config->>'knowledge_bound_at_unix')::bigint > 0
					THEN to_timestamp((binding_config->>'knowledge_bound_at_unix')::bigint) ELSE default_authorized_at END AS knowledge_authorized_at,
				CASE WHEN binding_config->>'database_bound_by_account_id' ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
					THEN (binding_config->>'database_bound_by_account_id')::uuid ELSE default_authorized_by END AS database_authorized_by,
				CASE WHEN binding_config->>'database_bound_at_unix' ~ '^[0-9]{1,10}$' AND (binding_config->>'database_bound_at_unix')::bigint > 0
					THEN to_timestamp((binding_config->>'database_bound_at_unix')::bigint) ELSE default_authorized_at END AS database_authorized_at,
				CASE WHEN binding_config->>'workflow_bound_by_account_id' ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
					THEN (binding_config->>'workflow_bound_by_account_id')::uuid ELSE default_authorized_by END AS workflow_authorized_by,
				CASE WHEN binding_config->>'workflow_bound_at_unix' ~ '^[0-9]{1,10}$' AND (binding_config->>'workflow_bound_at_unix')::bigint > 0
					THEN to_timestamp((binding_config->>'workflow_bound_at_unix')::bigint) ELSE default_authorized_at END AS workflow_authorized_at
			FROM raw_sources
		)
		INSERT INTO public.agent_resource_bindings (
			agent_id, binding_scope, organization_id, workspace_id, published_version_uuid,
			binding_type, resource_id, parent_resource_id, display_name, access_mode,
			authorized_by, authorized_at
		)
		SELECT
			source.agent_id, source.binding_scope, source.organization_id, source.workspace_id, source.published_version_uuid,
			binding.binding_type, binding.resource_id, binding.parent_resource_id, binding.display_name, binding.access_mode,
			binding.authorized_by, binding.authorized_at
		FROM binding_sources AS source
		CROSS JOIN LATERAL (
			SELECT
				'skill'::varchar AS binding_type,
				skill_id AS resource_id,
				''::varchar AS parent_resource_id,
				''::varchar AS display_name,
				'execute'::varchar AS access_mode,
				source.default_authorized_by AS authorized_by,
				source.default_authorized_at AS authorized_at
			FROM jsonb_array_elements_text(CASE WHEN jsonb_typeof(source.binding_config->'enabled_skill_ids') = 'array' THEN source.binding_config->'enabled_skill_ids' ELSE '[]'::jsonb END) AS skills(skill_id)
			WHERE btrim(skill_id) <> ''
			UNION ALL
			SELECT
				'knowledge_dataset', dataset_id, '', '', 'read',
				source.knowledge_authorized_by, source.knowledge_authorized_at
			FROM jsonb_array_elements_text(CASE WHEN jsonb_typeof(source.binding_config->'knowledge_dataset_ids') = 'array' THEN source.binding_config->'knowledge_dataset_ids' ELSE '[]'::jsonb END) AS datasets(dataset_id)
			WHERE btrim(dataset_id) <> ''
			UNION ALL
			SELECT
				'database', database_binding->>'data_source_id', '', '', 'read',
				source.database_authorized_by, source.database_authorized_at
			FROM jsonb_array_elements(CASE WHEN jsonb_typeof(source.binding_config->'database_bindings') = 'array' THEN source.binding_config->'database_bindings' ELSE '[]'::jsonb END) AS database_rows(database_binding)
			WHERE btrim(database_binding->>'data_source_id') <> ''
			UNION ALL
			SELECT
				'database_table', table_id, database_binding->>'data_source_id', '',
				CASE WHEN (CASE WHEN jsonb_typeof(database_binding->'writable_table_ids') = 'array' THEN database_binding->'writable_table_ids' ELSE '[]'::jsonb END) ? table_id THEN 'write' ELSE 'read' END,
				source.database_authorized_by, source.database_authorized_at
			FROM jsonb_array_elements(CASE WHEN jsonb_typeof(source.binding_config->'database_bindings') = 'array' THEN source.binding_config->'database_bindings' ELSE '[]'::jsonb END) AS database_rows(database_binding)
			CROSS JOIN LATERAL jsonb_array_elements_text(CASE WHEN jsonb_typeof(database_binding->'table_ids') = 'array' THEN database_binding->'table_ids' ELSE '[]'::jsonb END) AS tables(table_id)
			WHERE btrim(database_binding->>'data_source_id') <> '' AND btrim(table_id) <> ''
			UNION ALL
			SELECT
				'workflow', workflow_binding->>'binding_id', COALESCE(NULLIF(workflow_binding->>'agent_id', ''), workflow_binding->>'workflow_id', ''), COALESCE(workflow_binding->>'label', ''), 'execute',
				source.workflow_authorized_by, source.workflow_authorized_at
			FROM jsonb_array_elements(CASE WHEN jsonb_typeof(source.binding_config->'workflow_bindings') = 'array' THEN source.binding_config->'workflow_bindings' ELSE '[]'::jsonb END) AS workflow_rows(workflow_binding)
			WHERE btrim(workflow_binding->>'binding_id') <> ''
		) AS binding
		ON CONFLICT DO NOTHING;

		WITH latest_published AS (
			SELECT DISTINCT ON (published.agent_id) published.id
			FROM public.agent_published_versions AS published
			JOIN public.agents AS agent ON agent.id = published.agent_id
			WHERE published.deleted_at IS NULL
				AND agent.deleted_at IS NULL
				AND agent.agent_type = 'AGENT'
			ORDER BY published.agent_id, published.created_at DESC, published.id DESC
		)
		UPDATE
			public.agent_published_versions AS published
		SET config_snapshot = jsonb_set(published.config_snapshot, '{binding_indexed}', 'true'::jsonb, true)
		FROM latest_published
		WHERE published.id = latest_published.id
	`
)

func init() {
	registerSchemaMigration(
		migrationCreateAgentResourceBindingsID,
		func(schema *mschema.Builder) error { return schema.Raw(createAgentResourceBindingsSQL) },
		func(schema *mschema.Builder) error { return schema.DropIfExists("agent_resource_bindings") },
	)
}
