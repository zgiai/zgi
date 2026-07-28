package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260728173000ID            = "20260728173000_add_integration_provider_diagnostics"
	addIntegrationProviderDiagnosticsSQL = `
		ALTER TABLE public.integration_executions
			ADD COLUMN provider_error_code varchar(128),
			ADD COLUMN provider_http_status integer,
			ADD COLUMN retry_after_at timestamptz,
			ADD CONSTRAINT integration_executions_provider_http_status_check
				CHECK (provider_http_status IS NULL OR provider_http_status BETWEEN 100 AND 599);

		ALTER TABLE public.integration_connection_health_events
			ADD COLUMN provider_error_code varchar(128);
	`
	rollbackIntegrationProviderDiagnosticsSQL = `
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1
				FROM public.integration_executions
				WHERE provider_error_code IS NOT NULL
					OR provider_http_status IS NOT NULL
					OR retry_after_at IS NOT NULL
				LIMIT 1
			) OR EXISTS (
				SELECT 1
				FROM public.integration_connection_health_events
				WHERE provider_error_code IS NOT NULL
				LIMIT 1
			) THEN
				RAISE EXCEPTION 'cannot remove integration provider diagnostics while diagnostic history exists';
			END IF;
		END
		$$;
	`
	dropIntegrationProviderDiagnosticsConstraintSQL = `
		ALTER TABLE public.integration_executions
			DROP CONSTRAINT integration_executions_provider_http_status_check;
	`
)

func init() {
	registerSchemaMigration(migration20260728173000ID, up20260728173000, down20260728173000)
}

func up20260728173000(schema *mschema.Builder) error {
	return schema.Raw(addIntegrationProviderDiagnosticsSQL)
}

func down20260728173000(schema *mschema.Builder) error {
	if err := schema.Raw(rollbackIntegrationProviderDiagnosticsSQL); err != nil {
		return err
	}
	if err := schema.Raw(dropIntegrationProviderDiagnosticsConstraintSQL); err != nil {
		return err
	}
	if err := schema.Table("integration_connection_health_events", func(table *mschema.Blueprint) {
		table.DropColumn("provider_error_code")
	}); err != nil {
		return err
	}
	return schema.Table("integration_executions", func(table *mschema.Blueprint) {
		table.DropColumn("retry_after_at")
		table.DropColumn("provider_http_status")
		table.DropColumn("provider_error_code")
	})
}
