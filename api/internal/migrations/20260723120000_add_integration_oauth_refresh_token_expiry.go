package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260723120000ID                = "20260723120000_add_integration_oauth_refresh_token_expiry"
	addIntegrationOAuthRefreshTokenExpirySQL = `
		ALTER TABLE public.integration_connections
			ADD COLUMN refresh_token_expires_at timestamptz,
			ADD CONSTRAINT integration_connections_refresh_token_expiry_oauth_check
			CHECK (refresh_token_expires_at IS NULL OR auth_type = 'oauth2');

		CREATE INDEX idx_integration_connections_oauth_refresh_token_expiry
			ON public.integration_connections (refresh_token_expires_at, id)
			WHERE auth_type = 'oauth2' AND deleted_at IS NULL;
	`
	rollbackIntegrationOAuthRefreshTokenExpirySQL = `
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1
				FROM public.integration_connections
				WHERE refresh_token_expires_at IS NOT NULL
				LIMIT 1
			) THEN
				RAISE EXCEPTION 'cannot remove OAuth refresh token expiry while tracked expiry metadata exists';
			END IF;
		END
		$$;

		DROP INDEX IF EXISTS public.idx_integration_connections_oauth_refresh_token_expiry;
		ALTER TABLE public.integration_connections
			DROP CONSTRAINT IF EXISTS integration_connections_refresh_token_expiry_oauth_check,
			DROP COLUMN IF EXISTS refresh_token_expires_at;
	`
)

func init() {
	registerSchemaMigration(migration20260723120000ID, up20260723120000, down20260723120000)
}

func up20260723120000(schema *mschema.Builder) error {
	return schema.Raw(addIntegrationOAuthRefreshTokenExpirySQL)
}

func down20260723120000(schema *mschema.Builder) error {
	return schema.Raw(rollbackIntegrationOAuthRefreshTokenExpirySQL)
}
