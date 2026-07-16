package migrations

import (
	"strings"
	"testing"
)

func TestAgentResourceBindingsMigrationDefinesStableContract(t *testing.T) {
	sql := compactSQL(createAgentResourceBindingsSQL)
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS public.agent_resource_bindings",
		"agent_id uuid NOT NULL REFERENCES public.agents(id) ON DELETE CASCADE",
		"binding_scope varchar(16) NOT NULL",
		"organization_id uuid NOT NULL",
		"workspace_id uuid NOT NULL",
		"published_version_uuid uuid",
		"authorized_by uuid",
		"authorized_at timestamptz",
		"CHECK (binding_type IN ('skill', 'knowledge_dataset', 'database', 'database_table', 'workflow'))",
		"CHECK (binding_scope IN ('draft', 'published'))",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_resource_bindings_scope_unique",
		"COALESCE(published_version_uuid, '00000000-0000-0000-0000-000000000000'::uuid)",
		"SELECT DISTINCT ON (published.agent_id)",
		"ORDER BY published.agent_id, published.created_at DESC, published.id DESC",
		"CREATE OR REPLACE FUNCTION pg_temp.safe_agent_binding_jsonb(value text)",
		"EXCEPTION WHEN others THEN RETURN '{}'::jsonb",
		"pg_temp.safe_agent_binding_jsonb(config.agent_mode) AS binding_config",
		"FROM public.agents_configs AS candidate",
		"WHERE candidate.agents_id = agent.id AND candidate.deleted_at IS NULL",
		"ORDER BY (candidate.id = agent.agents_model_config_id) DESC NULLS LAST",
		"candidate.updated_at DESC, candidate.created_at DESC, candidate.id DESC",
		"LIMIT 1",
		"published.config_snapshot AS binding_config",
		"jsonb_array_elements_text(CASE WHEN jsonb_typeof(source.binding_config->'enabled_skill_ids') = 'array'",
		"jsonb_array_elements_text(CASE WHEN jsonb_typeof(source.binding_config->'knowledge_dataset_ids') = 'array'",
		"jsonb_array_elements(CASE WHEN jsonb_typeof(source.binding_config->'database_bindings') = 'array'",
		"jsonb_array_elements(CASE WHEN jsonb_typeof(source.binding_config->'workflow_bindings') = 'array'",
		"COALESCE(NULLIF(workflow_binding->>'agent_id', ''), workflow_binding->>'workflow_id', '')",
		"source.knowledge_authorized_by, source.knowledge_authorized_at",
		"source.database_authorized_by, source.database_authorized_at",
		"source.workflow_authorized_by, source.workflow_authorized_at",
		"ON CONFLICT DO NOTHING",
		"SET config_snapshot = jsonb_set(published.config_snapshot, '{binding_indexed}', 'true'::jsonb, true)",
		"WHERE published.id = latest_published.id",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("agent resource bindings SQL missing %q: %s", want, sql)
		}
	}
}
