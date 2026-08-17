package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260723100000ID                = "20260723100000_create_integration_oauth_flows_and_clients"
	createIntegrationOAuthFlowsAndClientsSQL = `
		CREATE TABLE public.integration_oauth_client_configs (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			integration_id varchar(64) NOT NULL,
			driver_id varchar(64) NOT NULL,
			auth_method_id varchar(128) NOT NULL,
			encrypted_credentials text NOT NULL,
			config jsonb NOT NULL DEFAULT '{}'::jsonb,
			enabled boolean NOT NULL DEFAULT true,
			credential_version integer NOT NULL DEFAULT 1,
			revision integer NOT NULL DEFAULT 1,
			created_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
			updated_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT integration_oauth_client_unique
				UNIQUE (organization_id, integration_id, auth_method_id),
			CONSTRAINT integration_oauth_client_identity_check
				CHECK (
					integration_id ~ '^[a-z0-9][a-z0-9._-]{0,63}$'
					AND driver_id ~ '^[a-z0-9][a-z0-9._-]{0,63}$'
					AND auth_method_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'
				),
			CONSTRAINT integration_oauth_client_encrypted_check
				CHECK (char_length(encrypted_credentials) > 3),
			CONSTRAINT integration_oauth_client_config_object_check
				CHECK (jsonb_typeof(config) = 'object'),
			CONSTRAINT integration_oauth_client_versions_positive_check
				CHECK (credential_version >= 1 AND revision >= 1)
		);

		CREATE TABLE public.integration_oauth_flows (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			flow_digest varchar(64) NOT NULL UNIQUE,
			encrypted_flow_token text NOT NULL,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE CASCADE,
			connection_id uuid REFERENCES public.integration_connections(id) ON DELETE SET NULL,
			completed_connection_id uuid REFERENCES public.integration_connections(id) ON DELETE SET NULL,
			integration_id varchar(64) NOT NULL,
			driver_id varchar(64) NOT NULL,
			auth_method_id varchar(128) NOT NULL,
			credential_source varchar(32) NOT NULL,
			intent varchar(32) NOT NULL,
			connection_name varchar(128) NOT NULL,
			requested_action_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
			requested_scopes jsonb NOT NULL DEFAULT '[]'::jsonb,
			return_path varchar(2048) NOT NULL,
			status varchar(32) NOT NULL DEFAULT 'pending',
			failure_code varchar(64),
			account_display_name varchar(255),
			expires_at timestamptz NOT NULL,
			completed_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT integration_oauth_flows_digest_check
				CHECK (flow_digest ~ '^[0-9a-f]{64}$'),
			CONSTRAINT integration_oauth_flows_token_encrypted_check
				CHECK (char_length(encrypted_flow_token) > 3),
			CONSTRAINT integration_oauth_flows_identity_check
				CHECK (
					integration_id ~ '^[a-z0-9][a-z0-9._-]{0,63}$'
					AND driver_id ~ '^[a-z0-9][a-z0-9._-]{0,63}$'
					AND auth_method_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'
				),
			CONSTRAINT integration_oauth_flows_credential_source_check
				CHECK (credential_source IN ('organization', 'account')),
			CONSTRAINT integration_oauth_flows_intent_check
				CHECK (intent IN ('connect', 'reconnect', 'scope_upgrade')),
			CONSTRAINT integration_oauth_flows_status_check
				CHECK (status IN ('pending', 'succeeded', 'failed', 'cancelled', 'expired')),
			CONSTRAINT integration_oauth_flows_actions_array_check
				CHECK (jsonb_typeof(requested_action_ids) = 'array'),
			CONSTRAINT integration_oauth_flows_scopes_array_check
				CHECK (jsonb_typeof(requested_scopes) = 'array'),
			CONSTRAINT integration_oauth_flows_completion_check
				CHECK (
					(status = 'pending' AND completed_at IS NULL AND failure_code IS NULL)
					OR
					(status = 'succeeded' AND completed_at IS NOT NULL AND completed_connection_id IS NOT NULL AND failure_code IS NULL)
					OR
					(status IN ('failed', 'expired') AND completed_at IS NOT NULL AND failure_code IS NOT NULL)
					OR
					(status = 'cancelled' AND completed_at IS NOT NULL)
				)
		);

		CREATE INDEX idx_integration_oauth_flows_actor
			ON public.integration_oauth_flows (organization_id, account_id, created_at DESC);
		CREATE INDEX idx_integration_oauth_flows_status_expiry
			ON public.integration_oauth_flows (status, expires_at);

		ALTER TABLE public.integration_oauth_states
			ADD COLUMN flow_id uuid REFERENCES public.integration_oauth_flows(id) ON DELETE CASCADE;
		CREATE INDEX idx_integration_oauth_states_flow
			ON public.integration_oauth_states (flow_id);
	`
	rollbackIntegrationOAuthFlowsAndClientsSQL = `
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM public.integration_oauth_client_configs LIMIT 1)
				OR EXISTS (SELECT 1 FROM public.integration_oauth_flows LIMIT 1) THEN
				RAISE EXCEPTION 'cannot roll back OAuth flow/client schema while OAuth data exists';
			END IF;
		END
		$$;

		DROP INDEX IF EXISTS public.idx_integration_oauth_states_flow;
		ALTER TABLE public.integration_oauth_states DROP COLUMN IF EXISTS flow_id;
		DROP TABLE IF EXISTS public.integration_oauth_flows;
		DROP TABLE IF EXISTS public.integration_oauth_client_configs;
	`
)

func init() {
	registerSchemaMigration(migration20260723100000ID, up20260723100000, down20260723100000)
}

func up20260723100000(schema *mschema.Builder) error {
	return schema.Raw(createIntegrationOAuthFlowsAndClientsSQL)
}

func down20260723100000(schema *mschema.Builder) error {
	return schema.Raw(rollbackIntegrationOAuthFlowsAndClientsSQL)
}
