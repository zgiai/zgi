package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddLLMInvocationContentGovernanceID = "20260810123000_add_llm_invocation_content_governance"

func init() {
	registerSchemaMigration(
		migrationAddLLMInvocationContentGovernanceID,
		upAddLLMInvocationContentGovernance,
		downAddLLMInvocationContentGovernance,
	)
}

func upAddLLMInvocationContentGovernance(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.organizations
			ADD COLUMN IF NOT EXISTS llm_content_retention_days integer;
		DO $$ BEGIN
			ALTER TABLE public.organizations
				ADD CONSTRAINT ck_organizations_llm_content_retention_days
				CHECK (llm_content_retention_days IS NULL OR llm_content_retention_days BETWEEN 1 AND 30);
		EXCEPTION
			WHEN duplicate_object THEN NULL;
		END $$;

		ALTER TABLE public.llm_invocation_content_views
			ADD COLUMN IF NOT EXISTS action varchar(24) NOT NULL DEFAULT 'view';

		CREATE INDEX IF NOT EXISTS idx_llm_invocation_contents_expires
			ON public.llm_invocation_contents (expires_at, request_id)
	`)
}

func downAddLLMInvocationContentGovernance(schema *mschema.Builder) error {
	if err := schema.Raw(`DROP INDEX IF EXISTS public.idx_llm_invocation_contents_expires`); err != nil {
		return err
	}
	if err := schema.WhenTableHasColumn("llm_invocation_content_views", "action", func() error {
		return schema.Table("llm_invocation_content_views", func(table *mschema.Blueprint) {
			table.DropColumn("action")
		})
	}); err != nil {
		return err
	}
	return schema.WhenTableHasColumn("organizations", "llm_content_retention_days", func() error {
		return schema.Table("organizations", func(table *mschema.Blueprint) {
			table.DropColumn("llm_content_retention_days")
		})
	})
}
