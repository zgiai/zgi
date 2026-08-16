package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260723130000ID            = "20260723130000_add_integration_oauth_browser_binding"
	addIntegrationOAuthBrowserBindingSQL = `
		ALTER TABLE public.integration_oauth_flows
			ADD COLUMN browser_binding_digest varchar(64) NOT NULL
				DEFAULT '0000000000000000000000000000000000000000000000000000000000000000';
		ALTER TABLE public.integration_oauth_states
			ADD COLUMN browser_binding_digest varchar(64) NOT NULL
				DEFAULT '0000000000000000000000000000000000000000000000000000000000000000';

		-- Pre-upgrade browser sessions do not possess a matching HttpOnly
		-- binding. Invalidate their temporary artifacts rather than allowing a
		-- callback to bypass the new browser possession check.
		UPDATE public.integration_oauth_states
			SET status = 'consumed',
				consumed_at = COALESCE(consumed_at, CURRENT_TIMESTAMP),
				encrypted_verifier = ''
			WHERE status = 'pending';
		UPDATE public.integration_oauth_flows
			SET status = 'failed',
				failure_code = 'integration_auth_invalid',
				completed_at = COALESCE(completed_at, CURRENT_TIMESTAMP),
				encrypted_flow_token = '',
				updated_at = CURRENT_TIMESTAMP
			WHERE status = 'pending';

		ALTER TABLE public.integration_oauth_flows
			ALTER COLUMN browser_binding_digest DROP DEFAULT,
			ADD CONSTRAINT integration_oauth_flows_browser_binding_digest_check
				CHECK (browser_binding_digest ~ '^[0-9a-f]{64}$');
		ALTER TABLE public.integration_oauth_states
			ALTER COLUMN browser_binding_digest DROP DEFAULT,
			ADD CONSTRAINT integration_oauth_states_browser_binding_digest_check
				CHECK (browser_binding_digest ~ '^[0-9a-f]{64}$');
	`
	rollbackIntegrationOAuthBrowserBindingSQL = `
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM public.integration_oauth_flows
				WHERE status = 'pending'
				LIMIT 1
			) OR EXISTS (
				SELECT 1 FROM public.integration_oauth_states
				WHERE status = 'pending'
				LIMIT 1
			) THEN
				RAISE EXCEPTION 'cannot remove OAuth browser binding while authorization flows are pending';
			END IF;
		END
		$$;

		ALTER TABLE public.integration_oauth_states
			DROP CONSTRAINT IF EXISTS integration_oauth_states_browser_binding_digest_check,
			DROP COLUMN IF EXISTS browser_binding_digest;
		ALTER TABLE public.integration_oauth_flows
			DROP CONSTRAINT IF EXISTS integration_oauth_flows_browser_binding_digest_check,
			DROP COLUMN IF EXISTS browser_binding_digest;
	`
)

func init() {
	registerSchemaMigration(migration20260723130000ID, up20260723130000, down20260723130000)
}

func up20260723130000(schema *mschema.Builder) error {
	return schema.Raw(addIntegrationOAuthBrowserBindingSQL)
}

func down20260723130000(schema *mschema.Builder) error {
	return schema.Raw(rollbackIntegrationOAuthBrowserBindingSQL)
}
