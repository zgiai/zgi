package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateAgentCatalogTagsID = "202609260900000000_create_agent_catalog_tags"

func init() {
	registerSchemaMigration(migrationCreateAgentCatalogTagsID, upCreateAgentCatalogTags, downCreateAgentCatalogTags)
}

func upCreateAgentCatalogTags(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.agent_catalog_tags (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			name varchar(64) NOT NULL,
			normalized_name varchar(64) NOT NULL,
			created_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT agent_catalog_tags_name_not_blank CHECK (btrim(name) <> ''),
			CONSTRAINT agent_catalog_tags_normalized_name_not_blank CHECK (btrim(normalized_name) <> '')
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_catalog_tags_organization_name
			ON public.agent_catalog_tags (organization_id, normalized_name);
		CREATE INDEX IF NOT EXISTS idx_agent_catalog_tags_organization
			ON public.agent_catalog_tags (organization_id, name, id);

		CREATE TABLE IF NOT EXISTS public.agent_catalog_tag_bindings (
			agent_id uuid NOT NULL REFERENCES public.agents(id) ON DELETE CASCADE,
			tag_id uuid NOT NULL REFERENCES public.agent_catalog_tags(id) ON DELETE CASCADE,
			created_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (agent_id, tag_id)
		);
		CREATE INDEX IF NOT EXISTS idx_agent_catalog_tag_bindings_tag_agent
			ON public.agent_catalog_tag_bindings (tag_id, agent_id);
	`)
}

func downCreateAgentCatalogTags(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP TABLE IF EXISTS public.agent_catalog_tag_bindings;
		DROP TABLE IF EXISTS public.agent_catalog_tags;
	`)
}
