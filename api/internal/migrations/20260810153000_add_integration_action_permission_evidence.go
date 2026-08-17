package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260810153000ID                 = "20260810153000_add_integration_action_permission_evidence"
	addIntegrationActionPermissionEvidenceSQL = `
		ALTER TABLE public.integration_connections
			ADD COLUMN verified_action_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
			ADD COLUMN denied_action_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
			ADD CONSTRAINT integration_connections_verified_action_ids_check
				CHECK (jsonb_typeof(verified_action_ids) = 'array' AND jsonb_array_length(verified_action_ids) <= 256),
			ADD CONSTRAINT integration_connections_denied_action_ids_check
				CHECK (jsonb_typeof(denied_action_ids) = 'array' AND jsonb_array_length(denied_action_ids) <= 256);

		ALTER TABLE public.integration_connection_health_events
			ADD COLUMN action_id varchar(128),
			ADD CONSTRAINT integration_connection_health_events_action_id_check
				CHECK (action_id IS NULL OR action_id ~ '^[a-z][a-z0-9_.-]{0,127}$');

		CREATE INDEX idx_integration_connection_health_events_action_observed
			ON public.integration_connection_health_events (connection_id, action_id, observed_at DESC)
			WHERE action_id IS NOT NULL;

		UPDATE public.integration_connections
		SET granted_scopes = '[]'::jsonb,
			missing_required_scopes = '[]'::jsonb,
			scope_status = 'unknown',
			scope_checked_at = NULL,
			verified_action_ids = CASE
				WHEN status = 'active' AND auth_status = 'valid' AND last_tested_at IS NOT NULL AND last_error_code IS NULL
					THEN '["dingtalk.department.list"]'::jsonb
				ELSE '[]'::jsonb
			END,
			denied_action_ids = '[]'::jsonb
		WHERE integration_id = 'dingtalk'
			AND auth_method_id = 'organization_dingtalk_internal_app';
	`
	dropIntegrationActionPermissionEvidenceSQL = `
		DROP INDEX IF EXISTS public.idx_integration_connection_health_events_action_observed;
		ALTER TABLE public.integration_connection_health_events
			DROP CONSTRAINT integration_connection_health_events_action_id_check;
		ALTER TABLE public.integration_connections
			DROP CONSTRAINT integration_connections_denied_action_ids_check,
			DROP CONSTRAINT integration_connections_verified_action_ids_check;
	`
)

func init() {
	registerSchemaMigration(migration20260810153000ID, up20260810153000, down20260810153000)
}

func up20260810153000(schema *mschema.Builder) error {
	return schema.Raw(addIntegrationActionPermissionEvidenceSQL)
}

func down20260810153000(schema *mschema.Builder) error {
	if err := schema.Raw(dropIntegrationActionPermissionEvidenceSQL); err != nil {
		return err
	}
	if err := schema.Table("integration_connection_health_events", func(table *mschema.Blueprint) {
		table.DropColumn("action_id")
	}); err != nil {
		return err
	}
	return schema.Table("integration_connections", func(table *mschema.Blueprint) {
		table.DropColumn("denied_action_ids")
		table.DropColumn("verified_action_ids")
	})
}
