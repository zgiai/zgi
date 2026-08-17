package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260723150000ID                     = "20260723150000_add_integration_oauth_recovery_acknowledgement"
	addIntegrationOAuthRecoveryAcknowledgementSQL = `
		ALTER TABLE public.integration_oauth_recovery_operations
			ADD COLUMN acknowledged_at timestamptz,
			ADD COLUMN acknowledged_by uuid,
			ADD COLUMN resolution_code varchar(64),
			ADD CONSTRAINT integration_oauth_recovery_acknowledgement_check
				CHECK (
					(
						acknowledged_at IS NULL
						AND acknowledged_by IS NULL
						AND resolution_code IS NULL
					)
					OR
					(
						status = 'dead_letter'
						AND acknowledged_at IS NOT NULL
						AND acknowledged_by IS NOT NULL
						AND resolution_code IN ('provider_access_removed', 'token_confirmed_expired')
					)
				);

		CREATE INDEX idx_integration_oauth_recovery_unresolved
			ON public.integration_oauth_recovery_operations
			(organization_id, dead_lettered_at DESC)
			WHERE status = 'dead_letter' AND acknowledged_at IS NULL;
	`
	rollbackIntegrationOAuthRecoveryAcknowledgementSQL = `
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1
				FROM public.integration_oauth_recovery_operations
				WHERE acknowledged_at IS NOT NULL
				LIMIT 1
			) THEN
				RAISE EXCEPTION 'cannot remove OAuth recovery acknowledgement history while acknowledged operations exist';
			END IF;
		END
		$$;

		DROP INDEX public.idx_integration_oauth_recovery_unresolved;
		ALTER TABLE public.integration_oauth_recovery_operations
			DROP CONSTRAINT integration_oauth_recovery_acknowledgement_check,
			DROP COLUMN resolution_code,
			DROP COLUMN acknowledged_by,
			DROP COLUMN acknowledged_at;
	`
)

func init() {
	registerSchemaMigration(migration20260723150000ID, up20260723150000, down20260723150000)
}

func up20260723150000(schema *mschema.Builder) error {
	return schema.Raw(addIntegrationOAuthRecoveryAcknowledgementSQL)
}

func down20260723150000(schema *mschema.Builder) error {
	return schema.Raw(rollbackIntegrationOAuthRecoveryAcknowledgementSQL)
}
