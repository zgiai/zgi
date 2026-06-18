package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddContentParseProviderOrganizationScopeID = "20260617090000_add_content_parse_provider_organization_scope"

func init() {
	registerSchemaMigration(
		migrationAddContentParseProviderOrganizationScopeID,
		upAddContentParseProviderOrganizationScope,
		downAddContentParseProviderOrganizationScope,
	)
}

func upAddContentParseProviderOrganizationScope(schema *mschema.Builder) error {
	if err := schema.Table("content_parse_provider_configs", func(table *mschema.Blueprint) {
		table.UUID("organization_id").Nullable()
		table.Index("idx_content_parse_provider_configs_org", "organization_id")
	}); err != nil {
		return err
	}

	return schema.Raw(`
		CREATE UNIQUE INDEX IF NOT EXISTS uq_content_parse_provider_configs_org_provider
		ON public.content_parse_provider_configs (scope, organization_id, provider_key)
		WHERE organization_id IS NOT NULL AND deleted_at IS NULL
	`)
}

func downAddContentParseProviderOrganizationScope(schema *mschema.Builder) error {
	schema.AllowDestructive()
	if err := schema.Raw(`DROP INDEX IF EXISTS public.uq_content_parse_provider_configs_org_provider`); err != nil {
		return err
	}
	if err := schema.Raw(`DROP INDEX IF EXISTS public.idx_content_parse_provider_configs_org`); err != nil {
		return err
	}
	return schema.Table("content_parse_provider_configs", func(table *mschema.Blueprint) {
		table.DropColumn("organization_id")
	})
}
