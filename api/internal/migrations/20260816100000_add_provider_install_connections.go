package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260816100000ID        = "20260816100000_add_provider_install_connections"
	addProviderInstallConnectionsSQL = `
		ALTER TABLE public.integration_connections
			DROP CONSTRAINT IF EXISTS integration_connections_auth_type_check,
			DROP CONSTRAINT IF EXISTS integration_connections_credential_storage_check;

		ALTER TABLE public.integration_connections
			ADD CONSTRAINT integration_connections_auth_type_check
				CHECK (auth_type IN ('platform', 'api_key', 'oauth2', 'custom_credential', 'service_account', 'provider_install')),
			ADD CONSTRAINT integration_connections_credential_storage_check
				CHECK (
					(credential_source = 'platform' AND auth_type = 'platform' AND encrypted_credentials IS NULL)
					OR
					(credential_source IN ('organization', 'account')
						AND auth_type IN ('api_key', 'oauth2', 'custom_credential', 'service_account', 'provider_install')
						AND (
							(encrypted_credentials IS NOT NULL AND char_length(encrypted_credentials) > 3)
							OR (
								status = 'disabled'
								AND auth_status = 'reconnect_required'
								AND encrypted_credentials IS NULL
								AND (
									auth_type = 'provider_install'
									OR (
										integration_id = 'wecom'
										AND driver_id = 'wecom-open-api'
										AND auth_method_id = 'organization_wecom_custom_app'
									)
								)
							)
						))
				);

		CREATE TABLE public.integration_provider_install_flows (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			flow_digest varchar(64) NOT NULL UNIQUE,
			state_digest varchar(64) NOT NULL UNIQUE,
			browser_binding_digest varchar(64) NOT NULL,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE CASCADE,
			completed_connection_id uuid REFERENCES public.integration_connections(id) ON DELETE SET NULL,
			integration_id varchar(64) NOT NULL,
			driver_id varchar(64) NOT NULL,
			auth_method_id varchar(128) NOT NULL,
			credential_source varchar(32) NOT NULL,
			connection_name varchar(128) NOT NULL,
			return_path varchar(2048) NOT NULL,
			status varchar(32) NOT NULL,
			failure_code varchar(64),
			account_display_name varchar(255),
			expires_at timestamptz NOT NULL,
			completed_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT integration_provider_install_flow_status_check
				CHECK (status IN ('pending', 'succeeded', 'failed', 'expired', 'cancelled')),
			CONSTRAINT integration_provider_install_flow_source_check
				CHECK (credential_source = 'organization'),
			CONSTRAINT integration_provider_install_flow_digest_check
				CHECK (flow_digest ~ '^[a-f0-9]{64}$' AND state_digest ~ '^[a-f0-9]{64}$' AND browser_binding_digest ~ '^[a-f0-9]{64}$')
		);
		CREATE INDEX idx_integration_provider_install_flows_actor
			ON public.integration_provider_install_flows (organization_id, account_id, created_at DESC);
		CREATE INDEX idx_integration_provider_install_flows_expiry
			ON public.integration_provider_install_flows (status, expires_at);

		CREATE TABLE public.integration_provider_authorizations (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			integration_id varchar(64) NOT NULL,
			external_tenant_hash varchar(64) NOT NULL,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			connection_id uuid NOT NULL UNIQUE REFERENCES public.integration_connections(id) ON DELETE CASCADE,
			active boolean NOT NULL DEFAULT true,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT integration_provider_authorization_hash_check
				CHECK (external_tenant_hash ~ '^[a-f0-9]{64}$'),
			CONSTRAINT integration_provider_authorization_tenant_unique
				UNIQUE (integration_id, external_tenant_hash)
		);
		CREATE INDEX idx_integration_provider_authorizations_org
			ON public.integration_provider_authorizations (organization_id, integration_id, active);

		CREATE TABLE public.integration_provider_suite_states (
			integration_id varchar(64) PRIMARY KEY,
			suite_id varchar(128) NOT NULL,
			encrypted_ticket text NOT NULL,
			credential_version integer NOT NULL DEFAULT 1,
			received_at timestamptz NOT NULL,
			expires_at timestamptz NOT NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT integration_provider_suite_state_version_check CHECK (credential_version >= 1)
		);

		UPDATE public.integration_connections
		SET status = 'disabled',
			auth_status = 'reconnect_required',
			attention_code = 'reconnect_required',
			last_error_code = 'provider_install_required',
			encrypted_credentials = NULL,
			is_default = false,
			credential_version = credential_version + 1,
			revision = revision + 1,
			updated_at = CURRENT_TIMESTAMP
		WHERE integration_id = 'wecom'
			AND driver_id = 'wecom-open-api'
			AND auth_method_id = 'organization_wecom_custom_app';
	`
)

func init() {
	registerSchemaMigration(migration20260816100000ID, up20260816100000, nil)
}

func up20260816100000(schema *mschema.Builder) error {
	return schema.Raw(addProviderInstallConnectionsSQL)
}
