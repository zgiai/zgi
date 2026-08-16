package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260722090000ID                   = "20260722090000_close_integration_connection_security_health"
	closeIntegrationConnectionSecurityHealthSQL = `
		ALTER TABLE public.integration_connections
			DROP CONSTRAINT IF EXISTS integration_connections_credential_source_check,
			DROP CONSTRAINT IF EXISTS integration_connections_auth_type_check,
			DROP CONSTRAINT IF EXISTS integration_connections_credential_storage_check;

		ALTER TABLE public.integration_connections
			ADD COLUMN owner_account_id uuid REFERENCES public.accounts(id) ON DELETE CASCADE,
			ADD COLUMN auth_method_id varchar(128),
			ADD COLUMN health_status varchar(32) NOT NULL DEFAULT 'unknown',
			ADD COLUMN auth_status varchar(32) NOT NULL DEFAULT 'unknown',
			ADD COLUMN scope_status varchar(32) NOT NULL DEFAULT 'unknown',
			ADD COLUMN attention_code varchar(64),
			ADD COLUMN missing_required_scopes jsonb NOT NULL DEFAULT '[]'::jsonb,
			ADD COLUMN last_health_checked_at timestamptz,
			ADD COLUMN last_healthy_at timestamptz,
			ADD COLUMN last_runtime_success_at timestamptz,
			ADD COLUMN last_runtime_failure_at timestamptz,
			ADD COLUMN scope_checked_at timestamptz,
			ADD COLUMN consecutive_failures integer NOT NULL DEFAULT 0,
			ADD COLUMN health_revision integer NOT NULL DEFAULT 1,
			ADD COLUMN token_expires_at timestamptz,
			ADD COLUMN next_token_refresh_at timestamptz,
			ADD CONSTRAINT integration_connections_credential_source_check
				CHECK (credential_source IN ('platform', 'organization', 'account')),
			ADD CONSTRAINT integration_connections_auth_type_check
				CHECK (auth_type IN ('platform', 'api_key', 'oauth2', 'custom_credential', 'service_account')),
			ADD CONSTRAINT integration_connections_auth_method_id_check
				CHECK (auth_method_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			ADD CONSTRAINT integration_connections_health_status_check
				CHECK (health_status IN ('unknown', 'healthy', 'degraded', 'unhealthy')),
			ADD CONSTRAINT integration_connections_auth_status_check
				CHECK (auth_status IN ('unknown', 'valid', 'reconnect_required', 'expired')),
			ADD CONSTRAINT integration_connections_scope_status_check
				CHECK (scope_status IN ('unknown', 'verified', 'drifted')),
			ADD CONSTRAINT integration_connections_attention_code_check
				CHECK (attention_code IS NULL OR attention_code IN ('reconnect_required', 'scope_update_required', 'billing_required', 'provider_incident', 'admin_check_required')),
			ADD CONSTRAINT integration_connections_missing_scopes_array_check
				CHECK (jsonb_typeof(missing_required_scopes) = 'array'),
			ADD CONSTRAINT integration_connections_failure_count_nonnegative
				CHECK (consecutive_failures >= 0),
			ADD CONSTRAINT integration_connections_health_revision_positive
				CHECK (health_revision >= 1),
			ADD CONSTRAINT integration_connections_owner_check
				CHECK (
					(credential_source = 'account' AND owner_account_id IS NOT NULL)
					OR (credential_source IN ('platform', 'organization') AND owner_account_id IS NULL)
				),
			ADD CONSTRAINT integration_connections_credential_storage_check
				CHECK (
					(credential_source = 'platform' AND auth_type = 'platform' AND encrypted_credentials IS NULL)
					OR
					(credential_source IN ('organization', 'account')
						AND auth_type IN ('api_key', 'oauth2', 'custom_credential', 'service_account')
						AND encrypted_credentials IS NOT NULL
						AND char_length(encrypted_credentials) > 3)
				);

		UPDATE public.integration_connections
			SET auth_method_id = auth_type
			WHERE auth_method_id IS NULL;
		ALTER TABLE public.integration_connections
			ALTER COLUMN auth_method_id SET NOT NULL;

		CREATE UNIQUE INDEX idx_integration_connections_id_organization
			ON public.integration_connections (id, organization_id);
		CREATE INDEX idx_integration_connections_oauth_refresh_due
			ON public.integration_connections (next_token_refresh_at, id)
			WHERE auth_type = 'oauth2' AND status = 'active' AND deleted_at IS NULL;
		CREATE INDEX idx_integration_connections_owner
			ON public.integration_connections (organization_id, owner_account_id, integration_id)
			WHERE credential_source = 'account' AND deleted_at IS NULL;

		CREATE TABLE public.integration_connection_grants (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			connection_id uuid NOT NULL,
			principal_type varchar(32) NOT NULL,
			principal_id uuid,
			access_mode varchar(16) NOT NULL,
			allowed_action_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
			resource_constraints jsonb NOT NULL DEFAULT '{}'::jsonb,
			revision integer NOT NULL DEFAULT 1,
			created_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
			updated_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT integration_connection_grants_connection_fk
				FOREIGN KEY (connection_id, organization_id)
				REFERENCES public.integration_connections(id, organization_id) ON DELETE CASCADE,
			CONSTRAINT integration_connection_grants_principal_type_check
				CHECK (principal_type IN ('organization', 'workspace', 'account')),
			CONSTRAINT integration_connection_grants_principal_check
				CHECK (
					(principal_type = 'organization' AND principal_id IS NULL)
					OR (principal_type IN ('workspace', 'account') AND principal_id IS NOT NULL)
				),
			CONSTRAINT integration_connection_grants_access_mode_check
				CHECK (access_mode IN ('read', 'write')),
			CONSTRAINT integration_connection_grants_actions_array_check
				CHECK (
					jsonb_typeof(allowed_action_ids) = 'array'
					AND jsonb_array_length(allowed_action_ids) > 0
					AND NOT (allowed_action_ids ? '*')
				),
			CONSTRAINT integration_connection_grants_resources_object_check
				CHECK (jsonb_typeof(resource_constraints) = 'object'),
			CONSTRAINT integration_connection_grants_revision_positive
				CHECK (revision >= 1)
		);
		CREATE UNIQUE INDEX idx_integration_connection_grants_principal
			ON public.integration_connection_grants (
				connection_id,
				principal_type,
				COALESCE(principal_id, '00000000-0000-0000-0000-000000000000'::uuid)
			);
		CREATE INDEX idx_integration_connection_grants_organization
			ON public.integration_connection_grants (organization_id, connection_id);

		CREATE TABLE public.aichat_integration_preferences (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE CASCADE,
			workspace_id uuid REFERENCES public.workspaces(id) ON DELETE CASCADE,
			integration_id varchar(64) NOT NULL,
			selected_connection_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
			preferred_connection_id uuid NOT NULL REFERENCES public.integration_connections(id) ON DELETE CASCADE,
			revision integer NOT NULL DEFAULT 1,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT aichat_integration_preferences_connections_array_check
				CHECK (jsonb_typeof(selected_connection_ids) = 'array' AND jsonb_array_length(selected_connection_ids) BETWEEN 1 AND 20),
			CONSTRAINT aichat_integration_preferences_revision_positive
				CHECK (revision >= 1)
		);
		CREATE UNIQUE INDEX idx_aichat_integration_preferences_identity
			ON public.aichat_integration_preferences (
				organization_id,
				account_id,
				COALESCE(workspace_id, '00000000-0000-0000-0000-000000000000'::uuid),
				integration_id
			);
		CREATE INDEX idx_aichat_integration_preferences_account
			ON public.aichat_integration_preferences (organization_id, account_id, workspace_id);

		CREATE TABLE public.integration_connection_health_events (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			connection_id uuid NOT NULL,
			integration_id varchar(64) NOT NULL,
			driver_id varchar(64) NOT NULL,
			source varchar(32) NOT NULL,
			check_kind varchar(32) NOT NULL,
			classification varchar(64) NOT NULL,
			reason_code varchar(64),
			health_status_after varchar(32) NOT NULL,
			auth_status_after varchar(32) NOT NULL,
			scope_status_after varchar(32) NOT NULL,
			attention_code_after varchar(64),
			credential_version integer NOT NULL,
			health_revision integer NOT NULL,
			execution_id uuid REFERENCES public.integration_executions(id) ON DELETE SET NULL,
			actor_id uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
			provider_request_id varchar(128),
			provider_http_status integer,
			latency_ms bigint NOT NULL DEFAULT 0,
			retry_after_at timestamptz,
			granted_scopes jsonb NOT NULL DEFAULT '[]'::jsonb,
			added_scopes jsonb NOT NULL DEFAULT '[]'::jsonb,
			removed_scopes jsonb NOT NULL DEFAULT '[]'::jsonb,
			missing_scopes jsonb NOT NULL DEFAULT '[]'::jsonb,
			error_fingerprint varchar(64),
			applied boolean NOT NULL DEFAULT false,
			observed_at timestamptz NOT NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT integration_connection_health_events_connection_fk
				FOREIGN KEY (connection_id, organization_id)
				REFERENCES public.integration_connections(id, organization_id) ON DELETE CASCADE,
			CONSTRAINT integration_connection_health_events_source_check
				CHECK (source IN ('manual', 'runtime', 'oauth_refresh')),
			CONSTRAINT integration_connection_health_events_kind_check
				CHECK (check_kind IN ('full', 'auth', 'scope', 'passive')),
			CONSTRAINT integration_connection_health_events_classification_check
				CHECK (classification IN ('success', 'auth_invalid', 'oauth_expired', 'scope_drift', 'access_denied', 'budget_exhausted', 'rate_limited', 'transient', 'provider_incident', 'ignored')),
			CONSTRAINT integration_connection_health_events_health_check
				CHECK (health_status_after IN ('unknown', 'healthy', 'degraded', 'unhealthy')),
			CONSTRAINT integration_connection_health_events_auth_check
				CHECK (auth_status_after IN ('unknown', 'valid', 'reconnect_required', 'expired')),
			CONSTRAINT integration_connection_health_events_scope_check
				CHECK (scope_status_after IN ('unknown', 'verified', 'drifted')),
			CONSTRAINT integration_connection_health_events_version_positive
				CHECK (credential_version >= 1 AND health_revision >= 1),
			CONSTRAINT integration_connection_health_events_http_status_check
				CHECK (provider_http_status IS NULL OR provider_http_status BETWEEN 100 AND 599),
			CONSTRAINT integration_connection_health_events_latency_nonnegative
				CHECK (latency_ms >= 0),
			CONSTRAINT integration_connection_health_events_scope_arrays_check
				CHECK (
					jsonb_typeof(granted_scopes) = 'array'
					AND jsonb_typeof(added_scopes) = 'array'
					AND jsonb_typeof(removed_scopes) = 'array'
					AND jsonb_typeof(missing_scopes) = 'array'
				),
			CONSTRAINT integration_connection_health_events_fingerprint_check
				CHECK (error_fingerprint IS NULL OR char_length(error_fingerprint) = 64)
		);
		CREATE UNIQUE INDEX idx_integration_connection_health_events_execution
			ON public.integration_connection_health_events (connection_id, execution_id, source)
			WHERE execution_id IS NOT NULL;
		CREATE INDEX idx_integration_connection_health_events_connection_observed
			ON public.integration_connection_health_events (connection_id, observed_at DESC);
		CREATE INDEX idx_integration_connection_health_events_org_observed
			ON public.integration_connection_health_events (organization_id, observed_at DESC);

		CREATE TABLE public.integration_oauth_states (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			state_digest varchar(64) NOT NULL UNIQUE,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE CASCADE,
			connection_id uuid REFERENCES public.integration_connections(id) ON DELETE CASCADE,
			integration_id varchar(64) NOT NULL,
			driver_id varchar(64) NOT NULL,
			auth_method_id varchar(128) NOT NULL,
			redirect_uri varchar(2048) NOT NULL,
			requested_scopes jsonb NOT NULL DEFAULT '[]'::jsonb,
			encrypted_verifier text NOT NULL,
			status varchar(32) NOT NULL DEFAULT 'pending',
			expires_at timestamptz NOT NULL,
			consumed_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT integration_oauth_states_status_check
				CHECK (status IN ('pending', 'consumed')),
			CONSTRAINT integration_oauth_states_auth_method_check
				CHECK (auth_method_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT integration_oauth_states_digest_check
				CHECK (char_length(state_digest) = 64),
			CONSTRAINT integration_oauth_states_scopes_array_check
				CHECK (jsonb_typeof(requested_scopes) = 'array'),
			CONSTRAINT integration_oauth_states_verifier_encrypted_check
				CHECK (char_length(encrypted_verifier) > 3),
			CONSTRAINT integration_oauth_states_consumption_check
				CHECK ((status = 'pending' AND consumed_at IS NULL) OR (status = 'consumed' AND consumed_at IS NOT NULL))
		);
		CREATE INDEX idx_integration_oauth_states_expiry
			ON public.integration_oauth_states (status, expires_at);
	`
	rollbackIntegrationConnectionSecurityHealthSQL = `
		LOCK TABLE public.integration_connections IN ACCESS EXCLUSIVE MODE;
		DO $rollback_guard$
		BEGIN
			IF EXISTS (
				SELECT 1
				FROM public.integration_connections
				WHERE NOT (
					(credential_source = 'platform' AND auth_type = 'platform' AND encrypted_credentials IS NULL)
					OR
					(credential_source = 'organization' AND auth_type = 'api_key' AND encrypted_credentials IS NOT NULL AND char_length(encrypted_credentials) > 3)
				)
			) THEN
				RAISE EXCEPTION 'cannot roll back integration connection security schema while incompatible connections exist'
					USING HINT = 'Explicitly migrate or remove account-owned, OAuth2, custom-credential, service-account, or invalid credential-storage connections before retrying.';
			END IF;
		END
		$rollback_guard$;

		DROP TABLE IF EXISTS public.integration_oauth_states;
		DROP TABLE IF EXISTS public.integration_connection_health_events;
		DROP TABLE IF EXISTS public.aichat_integration_preferences;
		DROP TABLE IF EXISTS public.integration_connection_grants;
		DROP INDEX IF EXISTS public.idx_integration_connections_owner;
		DROP INDEX IF EXISTS public.idx_integration_connections_oauth_refresh_due;
		DROP INDEX IF EXISTS public.idx_integration_connections_id_organization;
		ALTER TABLE public.integration_connections
			DROP CONSTRAINT IF EXISTS integration_connections_credential_storage_check,
			DROP CONSTRAINT IF EXISTS integration_connections_owner_check,
			DROP CONSTRAINT IF EXISTS integration_connections_health_revision_positive,
			DROP CONSTRAINT IF EXISTS integration_connections_failure_count_nonnegative,
			DROP CONSTRAINT IF EXISTS integration_connections_missing_scopes_array_check,
			DROP CONSTRAINT IF EXISTS integration_connections_attention_code_check,
			DROP CONSTRAINT IF EXISTS integration_connections_scope_status_check,
			DROP CONSTRAINT IF EXISTS integration_connections_auth_status_check,
			DROP CONSTRAINT IF EXISTS integration_connections_health_status_check,
			DROP CONSTRAINT IF EXISTS integration_connections_auth_type_check,
			DROP CONSTRAINT IF EXISTS integration_connections_auth_method_id_check,
			DROP CONSTRAINT IF EXISTS integration_connections_credential_source_check,
			DROP COLUMN IF EXISTS next_token_refresh_at,
			DROP COLUMN IF EXISTS token_expires_at,
			DROP COLUMN IF EXISTS health_revision,
			DROP COLUMN IF EXISTS consecutive_failures,
			DROP COLUMN IF EXISTS scope_checked_at,
			DROP COLUMN IF EXISTS last_runtime_failure_at,
			DROP COLUMN IF EXISTS last_runtime_success_at,
			DROP COLUMN IF EXISTS last_healthy_at,
			DROP COLUMN IF EXISTS last_health_checked_at,
			DROP COLUMN IF EXISTS missing_required_scopes,
			DROP COLUMN IF EXISTS attention_code,
			DROP COLUMN IF EXISTS scope_status,
			DROP COLUMN IF EXISTS auth_status,
			DROP COLUMN IF EXISTS health_status,
			DROP COLUMN IF EXISTS auth_method_id,
			DROP COLUMN IF EXISTS owner_account_id,
			ADD CONSTRAINT integration_connections_credential_source_check
				CHECK (credential_source IN ('platform', 'organization')),
			ADD CONSTRAINT integration_connections_auth_type_check
				CHECK (auth_type IN ('platform', 'api_key')),
			ADD CONSTRAINT integration_connections_credential_storage_check
				CHECK (
					(credential_source = 'platform' AND auth_type = 'platform' AND encrypted_credentials IS NULL)
					OR
					(credential_source = 'organization' AND auth_type = 'api_key' AND encrypted_credentials IS NOT NULL AND char_length(encrypted_credentials) > 3)
				);
	`
)

func init() {
	registerSchemaMigration(migration20260722090000ID, up20260722090000, down20260722090000)
}

func up20260722090000(schema *mschema.Builder) error {
	return schema.Raw(closeIntegrationConnectionSecurityHealthSQL)
}

func down20260722090000(schema *mschema.Builder) error {
	return schema.Raw(rollbackIntegrationConnectionSecurityHealthSQL)
}
