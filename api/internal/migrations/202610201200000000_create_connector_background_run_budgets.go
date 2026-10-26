package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const createConnectorBackgroundRunBudgetsSQL = `
CREATE TABLE public.connector_background_run_budgets (
 run_id uuid PRIMARY KEY,
 organization_id uuid NOT NULL,
 owner_id uuid NOT NULL,
 workspace_id uuid NOT NULL,
 employee_id uuid NOT NULL,
 version_id uuid NOT NULL,
 max_invocations integer NOT NULL CHECK (max_invocations BETWEEN 1 AND 100000),
 used_invocations integer NOT NULL DEFAULT 0 CHECK (used_invocations >= 0 AND used_invocations <= max_invocations),
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL
);
CREATE INDEX idx_background_run_budgets_scope ON public.connector_background_run_budgets
 (organization_id, owner_id, employee_id, version_id);
`

func init() {
	registerSchemaMigration("202610201200000000_create_connector_background_run_budgets", up202610201200000000, down202610201200000000)
}

func up202610201200000000(schema *mschema.Builder) error {
	return schema.Raw(createConnectorBackgroundRunBudgetsSQL)
}

func down202610201200000000(schema *mschema.Builder) error {
	return schema.Raw(`DROP TABLE IF EXISTS public.connector_background_run_budgets;`)
}
