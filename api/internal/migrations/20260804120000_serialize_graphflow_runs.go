package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationSerializeGraphFlowRunsID = "20260804120000_serialize_graphflow_runs"

func init() {
	registerSchemaMigration(
		migrationSerializeGraphFlowRunsID,
		upSerializeGraphFlowRuns,
		downSerializeGraphFlowRuns,
	)
}

func upSerializeGraphFlowRuns(schema *mschema.Builder) error {
	return schema.Raw(`
		WITH ranked AS (
			SELECT id,
				ROW_NUMBER() OVER (
					PARTITION BY dataset_id
					ORDER BY graph_revision ASC, created_at ASC, id ASC
				) AS position
			FROM public.graphflow_runs
			WHERE status = 'processing'
		)
		UPDATE public.graphflow_runs AS run
		SET status = 'pending',
			lease_expires_at = NULL,
			heartbeat_at = NULL,
			updated_at = now()
		FROM ranked
		WHERE run.id = ranked.id AND ranked.position > 1;

		CREATE UNIQUE INDEX IF NOT EXISTS idx_graphflow_runs_one_processing_per_dataset
			ON public.graphflow_runs (dataset_id)
			WHERE status = 'processing';
	`)
}

func downSerializeGraphFlowRuns(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_graphflow_runs_one_processing_per_dataset;
	`)
}
