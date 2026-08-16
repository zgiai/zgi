package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const (
	migration20260723160000ID                 = "20260723160000_redact_acknowledged_oauth_recovery_payload"
	redactAcknowledgedOAuthRecoveryPayloadSQL = `
		LOCK TABLE public.integration_oauth_recovery_operations IN ACCESS EXCLUSIVE MODE;

		ALTER TABLE public.integration_oauth_recovery_operations
			DROP CONSTRAINT integration_oauth_recovery_payload_check,
			ALTER COLUMN payload DROP NOT NULL;

		UPDATE public.integration_oauth_recovery_operations
			SET payload = NULL
			WHERE acknowledged_at IS NOT NULL;

		ALTER TABLE public.integration_oauth_recovery_operations
			ADD CONSTRAINT integration_oauth_recovery_payload_check
				CHECK (
					(
						acknowledged_at IS NULL
						AND payload IS NOT NULL
						AND jsonb_typeof(payload) = 'object'
						AND payload ? 'encrypted_credentials'
						AND char_length(payload ->> 'encrypted_credentials') > 3
						AND (
							kind <> 'revoke'
							OR (
								payload ? 'encrypted_client_credentials'
								AND char_length(payload ->> 'encrypted_client_credentials') > 3
							)
						)
					)
					OR
					(
						status = 'dead_letter'
						AND acknowledged_at IS NOT NULL
						AND payload IS NULL
					)
				);
	`
	restoreAcknowledgedOAuthRecoveryPayloadSQL = `
		LOCK TABLE public.integration_oauth_recovery_operations IN ACCESS EXCLUSIVE MODE;

		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1
				FROM public.integration_oauth_recovery_operations
				WHERE acknowledged_at IS NOT NULL OR payload IS NULL
				LIMIT 1
			) THEN
				RAISE EXCEPTION 'cannot restore OAuth recovery payload retention while redacted audit tombstones exist';
			END IF;
		END
		$$;

		ALTER TABLE public.integration_oauth_recovery_operations
			DROP CONSTRAINT integration_oauth_recovery_payload_check,
			ALTER COLUMN payload SET NOT NULL,
			ADD CONSTRAINT integration_oauth_recovery_payload_check
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
				);
	`
)

func init() {
	registerSchemaMigration(migration20260723160000ID, up20260723160000, down20260723160000)
}

func up20260723160000(schema *mschema.Builder) error {
	return schema.Raw(redactAcknowledgedOAuthRecoveryPayloadSQL)
}

func down20260723160000(schema *mschema.Builder) error {
	return schema.Raw(restoreAcknowledgedOAuthRecoveryPayloadSQL)
}
