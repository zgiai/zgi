package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddRuntimeRequestLeasesID = "202609231200000000_add_runtime_request_leases"

func init() {
	registerSchemaMigration(migrationAddRuntimeRequestLeasesID, upAddRuntimeRequestLeases, downAddRuntimeRequestLeases)
}

func upAddRuntimeRequestLeases(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.runtime_workspaces
			ADD COLUMN IF NOT EXISTS in_flight_requests bigint NOT NULL DEFAULT 0;
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conrelid = 'public.runtime_workspaces'::regclass
				  AND conname = 'ck_runtime_workspace_in_flight_requests'
			) THEN
				ALTER TABLE public.runtime_workspaces
					ADD CONSTRAINT ck_runtime_workspace_in_flight_requests
					CHECK (in_flight_requests >= 0);
			END IF;
		END
		$$;
	`)
}

func downAddRuntimeRequestLeases(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.runtime_workspaces
			DROP CONSTRAINT IF EXISTS ck_runtime_workspace_in_flight_requests;
		ALTER TABLE public.runtime_workspaces
			DROP COLUMN IF EXISTS in_flight_requests;
	`)
}
