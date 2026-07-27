package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260723140000ID               = "20260723140000_create_integration_oauth_recovery_outbox"
	createIntegrationOAuthRecoveryOutboxSQL = `
		CREATE TABLE public.integration_oauth_recovery_operations (
			id varchar(80) PRIMARY KEY,
			kind varchar(16) NOT NULL,
			organization_id uuid NOT NULL,
			connection_id uuid NOT NULL,
			integration_id varchar(64) NOT NULL,
			driver_id varchar(64) NOT NULL,
			auth_method_id varchar(128) NOT NULL,
			payload jsonb NOT NULL,
			status varchar(24) NOT NULL DEFAULT 'pending',
			attempts integer NOT NULL DEFAULT 0,
			available_at timestamptz NOT NULL,
			lease_owner uuid,
			lease_until timestamptz,
			last_error_code varchar(64),
			dead_lettered_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT integration_oauth_recovery_kind_check
				CHECK (kind IN ('revoke', 'refresh')),
			CONSTRAINT integration_oauth_recovery_status_check
				CHECK (status IN ('pending', 'processing', 'dead_letter')),
			CONSTRAINT integration_oauth_recovery_attempts_check
				CHECK (attempts >= 0 AND attempts <= 168),
			CONSTRAINT integration_oauth_recovery_payload_check
				CHECK (
					jsonb_typeof(payload) = 'object'
					AND payload ? 'encrypted_credentials'
					AND char_length(payload ->> 'encrypted_credentials') > 3
					AND (
						kind <> 'revoke'
						OR (
							payload ? 'encrypted_client_credentials'
							AND char_length(payload ->> 'encrypted_client_credentials') > 3
						)
					)
				),
			CONSTRAINT integration_oauth_recovery_lease_check
				CHECK (
					(status = 'processing' AND lease_owner IS NOT NULL AND lease_until IS NOT NULL)
					OR
					(status <> 'processing' AND lease_owner IS NULL AND lease_until IS NULL)
				),
			CONSTRAINT integration_oauth_recovery_dead_letter_check
				CHECK (
					(status = 'dead_letter' AND dead_lettered_at IS NOT NULL)
					OR
					(status <> 'dead_letter' AND dead_lettered_at IS NULL)
				)
		);

		CREATE INDEX idx_integration_oauth_recovery_ready
			ON public.integration_oauth_recovery_operations
			(status, available_at, created_at)
			WHERE status = 'pending';
		CREATE INDEX idx_integration_oauth_recovery_expired_lease
			ON public.integration_oauth_recovery_operations
			(lease_until)
			WHERE status = 'processing';
		CREATE INDEX idx_integration_oauth_recovery_client_impact
			ON public.integration_oauth_recovery_operations
			(organization_id, integration_id, auth_method_id, kind, status);
	`
	rollbackIntegrationOAuthRecoveryOutboxSQL = `
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1
				FROM public.integration_oauth_recovery_operations
				LIMIT 1
			) THEN
				RAISE EXCEPTION 'cannot remove durable OAuth recovery outbox while recovery operations exist';
			END IF;
		END
		$$;

		DROP TABLE public.integration_oauth_recovery_operations;
	`
)

func init() {
	registerSchemaMigration(migration20260723140000ID, up20260723140000, down20260723140000)
}

func up20260723140000(schema *mschema.Builder) error {
	return schema.Raw(createIntegrationOAuthRecoveryOutboxSQL)
}

func down20260723140000(schema *mschema.Builder) error {
	return schema.Raw(rollbackIntegrationOAuthRecoveryOutboxSQL)
}
