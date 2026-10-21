package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260902140000ID           = "20260902140000_add_connector_route_rollout_snapshot"
	addConnectorRouteRolloutSnapshotSQL = `
		ALTER TABLE public.connector_execution_route_claims
			ADD COLUMN provider_phase varchar(16),
			ADD COLUMN route_ownership varchar(16),
			ADD COLUMN rollout_revision bigint;

		DROP TRIGGER connector_execution_route_claims_immutable
			ON public.connector_execution_route_claims;
		UPDATE public.connector_execution_route_claims
		SET provider_phase = CASE WHEN path = 'connector_native' THEN 'connector' ELSE 'legacy' END,
			route_ownership = CASE WHEN path = 'connector_native' THEN 'native_only' ELSE 'legacy_only' END,
			rollout_revision = 0;
		CREATE TRIGGER connector_execution_route_claims_immutable
			BEFORE UPDATE OR DELETE ON public.connector_execution_route_claims
			FOR EACH ROW EXECUTE FUNCTION public.reject_connector_immutable_mutation();

		ALTER TABLE public.connector_execution_route_claims
			ALTER COLUMN provider_phase SET NOT NULL,
			ALTER COLUMN route_ownership SET NOT NULL,
			ALTER COLUMN rollout_revision SET NOT NULL,
			ADD CONSTRAINT connector_execution_route_claims_provider_phase_check
				CHECK (provider_phase IN ('legacy', 'shadow', 'canary', 'connector', 'reversing')),
			ADD CONSTRAINT connector_execution_route_claims_route_ownership_check
				CHECK (route_ownership IN ('native_only', 'legacy_only', 'migrated')),
			ADD CONSTRAINT connector_execution_route_claims_rollout_revision_check
				CHECK (rollout_revision >= 0),
			ADD CONSTRAINT connector_execution_route_claims_rollout_snapshot_check
				CHECK (
					(route_ownership = 'native_only' AND path = 'connector_native' AND provider_phase = 'connector' AND rollout_revision = 0)
					OR (route_ownership = 'legacy_only' AND path = 'legacy' AND provider_phase = 'legacy' AND rollout_revision = 0)
					OR (route_ownership = 'migrated' AND rollout_revision >= 1 AND (
						(provider_phase IN ('legacy', 'shadow', 'reversing') AND path = 'legacy')
						OR (provider_phase = 'canary' AND path IN ('legacy', 'connector_native'))
						OR (provider_phase = 'connector' AND path = 'connector_native')
					))
				);
	`
	dropConnectorRouteRolloutSnapshotSQL = `
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM public.connector_execution_route_claims LIMIT 1) THEN
				RAISE EXCEPTION 'cannot remove Connector route rollout snapshots while claims exist';
			END IF;
		END $$;
		ALTER TABLE public.connector_execution_route_claims
			DROP CONSTRAINT IF EXISTS connector_execution_route_claims_rollout_snapshot_check,
			DROP CONSTRAINT IF EXISTS connector_execution_route_claims_rollout_revision_check,
			DROP CONSTRAINT IF EXISTS connector_execution_route_claims_route_ownership_check,
			DROP CONSTRAINT IF EXISTS connector_execution_route_claims_provider_phase_check,
			DROP COLUMN IF EXISTS rollout_revision,
			DROP COLUMN IF EXISTS route_ownership,
			DROP COLUMN IF EXISTS provider_phase;
	`
)

func init() {
	registerSchemaMigration(migration20260902140000ID, up20260902140000, down20260902140000)
}

func up20260902140000(schema *mschema.Builder) error {
	return schema.Raw(addConnectorRouteRolloutSnapshotSQL)
}

func down20260902140000(schema *mschema.Builder) error {
	return schema.Raw(dropConnectorRouteRolloutSnapshotSQL)
}
