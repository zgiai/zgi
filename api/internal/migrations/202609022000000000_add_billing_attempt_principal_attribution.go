package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddBillingAttemptPrincipalAttributionID = "202609022000000000_add_billing_attempt_principal_attribution"

const addBillingAttemptPrincipalAttributionSQL = `
	ALTER TABLE public.billing_attempts
		ADD COLUMN IF NOT EXISTS api_key_id uuid,
		ADD COLUMN IF NOT EXISTS workspace_id uuid,
		ADD COLUMN IF NOT EXISTS account_id uuid,
		ADD COLUMN IF NOT EXISTS principal_type varchar(32),
		ADD COLUMN IF NOT EXISTS principal_id varchar(255),
		ADD COLUMN IF NOT EXISTS access_grant_id uuid,
		ADD COLUMN IF NOT EXISTS auth_method varchar(32) NOT NULL DEFAULT 'legacy_api_key';
	CREATE INDEX IF NOT EXISTS idx_billing_attempts_workspace_principal
		ON public.billing_attempts (workspace_id, principal_type, principal_id, created_at DESC);
	ALTER TABLE public.llm_organization_api_keys
		DROP CONSTRAINT IF EXISTS fk_llm_api_keys_access_grant;
	ALTER TABLE public.llm_organization_api_keys
		ADD CONSTRAINT fk_llm_api_keys_access_grant
		FOREIGN KEY (access_grant_id) REFERENCES public.llm_developer_access_grants(id) ON DELETE CASCADE;
`

const rollbackBillingAttemptPrincipalAttributionSQL = `
	ALTER TABLE public.llm_organization_api_keys
		DROP CONSTRAINT IF EXISTS fk_llm_api_keys_access_grant;
	ALTER TABLE public.llm_organization_api_keys
		ADD CONSTRAINT fk_llm_api_keys_access_grant
		FOREIGN KEY (access_grant_id) REFERENCES public.llm_developer_access_grants(id) ON DELETE CASCADE;
	DROP INDEX IF EXISTS public.idx_billing_attempts_workspace_principal;
	ALTER TABLE public.billing_attempts
		DROP COLUMN IF EXISTS auth_method,
		DROP COLUMN IF EXISTS access_grant_id,
		DROP COLUMN IF EXISTS principal_id,
		DROP COLUMN IF EXISTS principal_type,
		DROP COLUMN IF EXISTS account_id,
		DROP COLUMN IF EXISTS workspace_id,
		DROP COLUMN IF EXISTS api_key_id;
`

func init() {
	registerSchemaMigration(
		migrationAddBillingAttemptPrincipalAttributionID,
		upAddBillingAttemptPrincipalAttribution,
		downAddBillingAttemptPrincipalAttribution,
	)
}

func upAddBillingAttemptPrincipalAttribution(schema *mschema.Builder) error {
	return schema.Raw(addBillingAttemptPrincipalAttributionSQL)
}

func downAddBillingAttemptPrincipalAttribution(schema *mschema.Builder) error {
	return schema.Raw(rollbackBillingAttemptPrincipalAttributionSQL)
}
