package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260723110000ID                 = "20260723110000_close_integration_oauth_artifact_lifecycle"
	closeIntegrationOAuthArtifactLifecycleSQL = `
		ALTER TABLE public.integration_oauth_flows
			DROP CONSTRAINT integration_oauth_flows_token_encrypted_check;
		ALTER TABLE public.integration_oauth_flows
			ADD CONSTRAINT integration_oauth_flows_token_lifecycle_check
			CHECK (
				(status = 'pending' AND char_length(encrypted_flow_token) > 3)
				OR
				(status <> 'pending' AND encrypted_flow_token = '')
			) NOT VALID;

		ALTER TABLE public.integration_oauth_states
			DROP CONSTRAINT integration_oauth_states_verifier_encrypted_check;
		ALTER TABLE public.integration_oauth_states
			ADD CONSTRAINT integration_oauth_states_verifier_lifecycle_check
			CHECK (
				(status = 'pending' AND char_length(encrypted_verifier) > 3)
				OR
				(status = 'consumed' AND encrypted_verifier = '')
			) NOT VALID;

		CREATE INDEX idx_integration_oauth_flows_actor_provider_created
			ON public.integration_oauth_flows
			(organization_id, account_id, integration_id, created_at DESC);
	`
	rollbackIntegrationOAuthArtifactLifecycleSQL = `
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM public.integration_oauth_flows
				WHERE encrypted_flow_token = ''
				LIMIT 1
			) OR EXISTS (
				SELECT 1 FROM public.integration_oauth_states
				WHERE encrypted_verifier = ''
				LIMIT 1
			) THEN
				RAISE EXCEPTION 'cannot restore legacy OAuth secret constraints after temporary secrets were erased';
			END IF;
		END
		$$;

		DROP INDEX IF EXISTS public.idx_integration_oauth_flows_actor_provider_created;

		ALTER TABLE public.integration_oauth_states
			DROP CONSTRAINT integration_oauth_states_verifier_lifecycle_check;
		ALTER TABLE public.integration_oauth_states
			ADD CONSTRAINT integration_oauth_states_verifier_encrypted_check
			CHECK (char_length(encrypted_verifier) > 3);

		ALTER TABLE public.integration_oauth_flows
			DROP CONSTRAINT integration_oauth_flows_token_lifecycle_check;
		ALTER TABLE public.integration_oauth_flows
			ADD CONSTRAINT integration_oauth_flows_token_encrypted_check
			CHECK (char_length(encrypted_flow_token) > 3);
	`
)

func init() {
	registerSchemaMigration(migration20260723110000ID, up20260723110000, down20260723110000)
}

func up20260723110000(schema *mschema.Builder) error {
	return schema.Raw(closeIntegrationOAuthArtifactLifecycleSQL)
}

func down20260723110000(schema *mschema.Builder) error {
	return schema.Raw(rollbackIntegrationOAuthArtifactLifecycleSQL)
}
