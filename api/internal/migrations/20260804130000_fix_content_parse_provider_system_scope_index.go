package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationFixContentParseProviderSystemScopeIndexID = "20260804130000_fix_content_parse_provider_system_scope_index"

func init() {
	registerSchemaMigration(
		migrationFixContentParseProviderSystemScopeIndexID,
		upFixContentParseProviderSystemScopeIndex,
		nil,
	)
}

// upFixContentParseProviderSystemScopeIndex keeps the system-provider
// uniqueness constraint separate from organization-scoped provider settings.
func upFixContentParseProviderSystemScopeIndex(schema *mschema.Builder) error {
	if err := schema.Raw(`
		DROP INDEX IF EXISTS public.uq_content_parse_provider_configs_system_provider
	`); err != nil {
		return err
	}
	return schema.Raw(`
		CREATE UNIQUE INDEX IF NOT EXISTS uq_content_parse_provider_configs_system_provider
		ON public.content_parse_provider_configs (scope, provider_key)
		WHERE workspace_id IS NULL
			AND organization_id IS NULL
			AND deleted_at IS NULL
	`)
}
