package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260821100000ID                 = "20260821100000_add_provider_install_notification_recovery"
	addProviderInstallNotificationRecoverySQL = `
		ALTER TABLE public.integration_oauth_recovery_operations
			DROP CONSTRAINT integration_oauth_recovery_kind_check;
		ALTER TABLE public.integration_oauth_recovery_operations
			ADD CONSTRAINT integration_oauth_recovery_kind_check
				CHECK (kind IN ('revoke', 'refresh', 'provider_install'));
	`
	removeProviderInstallNotificationRecoverySQL = `
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1
				FROM public.integration_oauth_recovery_operations
				WHERE kind = 'provider_install'
				LIMIT 1
			) THEN
				RAISE EXCEPTION 'cannot remove provider-install recovery while operations exist';
			END IF;
		END
		$$;

		ALTER TABLE public.integration_oauth_recovery_operations
			DROP CONSTRAINT integration_oauth_recovery_kind_check;
		ALTER TABLE public.integration_oauth_recovery_operations
			ADD CONSTRAINT integration_oauth_recovery_kind_check
				CHECK (kind IN ('revoke', 'refresh'));
	`
)

func init() {
	registerSchemaMigration(migration20260821100000ID, up20260821100000, down20260821100000)
}

func up20260821100000(schema *mschema.Builder) error {
	return schema.Raw(addProviderInstallNotificationRecoverySQL)
}

func down20260821100000(schema *mschema.Builder) error {
	return schema.Raw(removeProviderInstallNotificationRecoverySQL)
}
