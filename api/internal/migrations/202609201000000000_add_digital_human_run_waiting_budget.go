package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202609201000000000ID = "202609201000000000_add_digital_human_run_waiting_budget"

func init() {
	registerSchemaMigration(migration202609201000000000ID, up202609201000000000, down202609201000000000)
}

func up202609201000000000(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.digital_human_runs
			DROP CONSTRAINT IF EXISTS digital_human_runs_waiting_check;
		ALTER TABLE public.digital_human_runs
			ADD CONSTRAINT digital_human_runs_waiting_check CHECK (
				(status = 'waiting' AND waiting_reason IN ('waiting_approval', 'waiting_question', 'waiting_client_action', 'waiting_budget') AND
					((waiting_reason = 'waiting_approval' AND waiting_receipt IS NOT NULL) OR
					 (waiting_reason <> 'waiting_approval' AND waiting_receipt IS NULL))) OR
				(status <> 'waiting' AND waiting_reason = '' AND waiting_receipt IS NULL)
			);
	`)
}

func down202609201000000000(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.digital_human_runs
			DROP CONSTRAINT IF EXISTS digital_human_runs_waiting_check;
		ALTER TABLE public.digital_human_runs
			ADD CONSTRAINT digital_human_runs_waiting_check CHECK (
				(status = 'waiting' AND waiting_reason = 'waiting_approval' AND waiting_receipt IS NOT NULL) OR
				(status = 'waiting' AND waiting_reason IN ('waiting_question', 'waiting_client_action') AND waiting_receipt IS NULL) OR
				(status <> 'waiting' AND waiting_reason = '' AND waiting_receipt IS NULL)
			);
	`)
}
