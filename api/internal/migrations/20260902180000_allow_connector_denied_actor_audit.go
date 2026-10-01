package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260902180000ID         = "20260902180000_allow_connector_denied_actor_audit"
	allowConnectorDeniedActorAuditSQL = `
		ALTER TABLE public.connector_lifecycle_decisions
			DROP CONSTRAINT connector_lifecycle_decisions_owner_check,
			ADD CONSTRAINT connector_lifecycle_decisions_owner_check
			CHECK (acting_principal_id = owner_account_id OR status = 'denied');
	`
	restoreConnectorDeniedActorAuditSQL = `
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM public.connector_lifecycle_decisions WHERE acting_principal_id <> owner_account_id) THEN
				RAISE EXCEPTION 'cannot remove Connector denied-actor audit support while denied records exist';
			END IF;
		END $$;
		ALTER TABLE public.connector_lifecycle_decisions
			DROP CONSTRAINT connector_lifecycle_decisions_owner_check,
			ADD CONSTRAINT connector_lifecycle_decisions_owner_check
			CHECK (acting_principal_id = owner_account_id);
	`
)

func init() {
	registerSchemaMigration(migration20260902180000ID, up20260902180000, down20260902180000)
}

func up20260902180000(schema *mschema.Builder) error {
	return schema.Raw(allowConnectorDeniedActorAuditSQL)
}
func down20260902180000(schema *mschema.Builder) error {
	return schema.Raw(restoreConnectorDeniedActorAuditSQL)
}
