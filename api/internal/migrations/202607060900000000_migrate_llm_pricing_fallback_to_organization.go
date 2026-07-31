package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationMigrateLLMPricingFallbackToOrganizationID = "202607060900000000_migrate_llm_pricing_fallback_to_organization"

const ensurePricingFallbackOrganizationUniqueSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS uq_llm_pricing_fallback_org
ON public.llm_pricing_fallback_overrides (organization_id)
`

const ensurePricingFallbackLegacyIDDefaultSQL = `
ALTER TABLE public.llm_pricing_fallback_overrides
ALTER COLUMN id SET DEFAULT public.uuid_generate_v4()::text
`

const copyPricingFallbackLegacyRowsToOrganizationsSQL = `
INSERT INTO public.llm_pricing_fallback_overrides (id, organization_id, enabled, rules, updated_by, created_at, updated_at)
SELECT organizations.id::text, organizations.id, legacy.enabled, legacy.rules, legacy.updated_by, legacy.created_at, legacy.updated_at
FROM public.organizations
CROSS JOIN LATERAL (
	SELECT enabled, rules, updated_by, created_at, updated_at
	FROM public.llm_pricing_fallback_overrides
	ORDER BY updated_at DESC
	LIMIT 1
) AS legacy
ON CONFLICT (id) DO NOTHING
`

const dropPricingFallbackOrganizationUniqueSQL = `
DROP INDEX IF EXISTS public.uq_llm_pricing_fallback_org
`

const dropPricingFallbackLegacyIDDefaultSQL = `
ALTER TABLE public.llm_pricing_fallback_overrides
ALTER COLUMN id DROP DEFAULT
`

func init() {
	registerSchemaMigration(
		migrationMigrateLLMPricingFallbackToOrganizationID,
		upMigrateLLMPricingFallbackToOrganization,
		downMigrateLLMPricingFallbackToOrganization,
	)
}

func upMigrateLLMPricingFallbackToOrganization(schema *mschema.Builder) error {
	exists, err := schema.HasTable("llm_pricing_fallback_overrides")
	if err != nil || !exists {
		return err
	}
	hasOrganizationID, err := schema.HasColumn("llm_pricing_fallback_overrides", "organization_id")
	if err != nil {
		return err
	}
	if !hasOrganizationID {
		if err := schema.Table("llm_pricing_fallback_overrides", func(table *mschema.Blueprint) {
			table.UUID("organization_id").Nullable()
		}); err != nil {
			return err
		}
	}
	hasLegacyID, err := schema.HasColumn("llm_pricing_fallback_overrides", "id")
	if err != nil {
		return err
	}
	if hasLegacyID {
		if err := schema.Raw(ensurePricingFallbackLegacyIDDefaultSQL); err != nil {
			return err
		}
	}
	if err := schema.Raw(ensurePricingFallbackOrganizationUniqueSQL); err != nil {
		return err
	}
	if hasLegacyID {
		return schema.Raw(copyPricingFallbackLegacyRowsToOrganizationsSQL)
	}
	return nil
}

func downMigrateLLMPricingFallbackToOrganization(schema *mschema.Builder) error {
	exists, err := schema.HasTable("llm_pricing_fallback_overrides")
	if err != nil || !exists {
		return err
	}
	if err := schema.Raw(dropPricingFallbackOrganizationUniqueSQL); err != nil {
		return err
	}
	hasLegacyID, err := schema.HasColumn("llm_pricing_fallback_overrides", "id")
	if err != nil {
		return err
	}
	if hasLegacyID {
		if err := schema.Raw(dropPricingFallbackLegacyIDDefaultSQL); err != nil {
			return err
		}
	}
	return schema.WhenTableHasColumn("llm_pricing_fallback_overrides", "organization_id", func() error {
		return schema.Table("llm_pricing_fallback_overrides", func(table *mschema.Blueprint) {
			table.DropColumn("organization_id")
		})
	})
}
