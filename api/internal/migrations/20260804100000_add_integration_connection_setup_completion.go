package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260804100000ID                  = "20260804100000_add_integration_connection_setup_completion"
	addIntegrationConnectionSetupCompletionSQL = `
		ALTER TABLE public.integration_connections
			ADD COLUMN setup_version integer NOT NULL DEFAULT 1,
			ADD COLUMN setup_completed_at timestamptz,
			ADD COLUMN setup_completed_by uuid,
			ADD CONSTRAINT integration_connections_setup_version_check
				CHECK (setup_version >= 1);

		UPDATE public.integration_connections AS connection
		SET setup_completed_at = COALESCE(connection.last_tested_at, connection.updated_at),
			setup_completed_by = COALESCE(connection.updated_by, connection.created_by)
		WHERE connection.status = 'active'
			AND connection.auth_status = 'valid'
			AND (
				connection.credential_source = 'account'
				OR EXISTS (
					SELECT 1
					FROM public.integration_connection_grants AS connection_grant
					WHERE connection_grant.organization_id = connection.organization_id
						AND connection_grant.connection_id = connection.id
						AND jsonb_array_length(connection_grant.allowed_action_ids) > 0
				)
			);
	`
	dropIntegrationConnectionSetupCompletionConstraintSQL = `
		ALTER TABLE public.integration_connections
			DROP CONSTRAINT integration_connections_setup_version_check;
	`
)

func init() {
	registerSchemaMigration(migration20260804100000ID, up20260804100000, down20260804100000)
}

func up20260804100000(schema *mschema.Builder) error {
	return schema.Raw(addIntegrationConnectionSetupCompletionSQL)
}

func down20260804100000(schema *mschema.Builder) error {
	if err := schema.Raw(dropIntegrationConnectionSetupCompletionConstraintSQL); err != nil {
		return err
	}
	return schema.Table("integration_connections", func(table *mschema.Blueprint) {
		table.DropColumn("setup_completed_by")
		table.DropColumn("setup_completed_at")
		table.DropColumn("setup_version")
	})
}
