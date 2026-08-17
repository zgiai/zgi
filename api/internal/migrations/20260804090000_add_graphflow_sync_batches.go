package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddGraphFlowSyncBatchesID = "20260804090000_add_graphflow_sync_batches"

func init() {
	registerSchemaMigration(
		migrationAddGraphFlowSyncBatchesID,
		upAddGraphFlowSyncBatches,
		downAddGraphFlowSyncBatches,
	)
}

func upAddGraphFlowSyncBatches(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.data_library_knowledge_base_asset_refs
			ADD COLUMN IF NOT EXISTS sync_batch_id uuid;
		CREATE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_sync_batch
			ON public.data_library_knowledge_base_asset_refs (sync_batch_id);

		ALTER TABLE public.graphflow_runs
			ADD COLUMN IF NOT EXISTS sync_batch_id uuid;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_graphflow_runs_dataset_sync_batch
			ON public.graphflow_runs (dataset_id, sync_batch_id)
			WHERE sync_batch_id IS NOT NULL;

		CREATE TABLE IF NOT EXISTS public.graphflow_run_items (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			run_id uuid NOT NULL REFERENCES public.graphflow_runs(id) ON DELETE CASCADE,
			organization_id uuid NOT NULL,
			dataset_id uuid NOT NULL REFERENCES public.datasets(id) ON DELETE CASCADE,
			source_ref_id uuid,
			sync_run_id uuid,
			sync_batch_id uuid NOT NULL,
			operation varchar(32) NOT NULL,
			document_id uuid NOT NULL,
			paired_document_id uuid,
			asset_generation_no bigint,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			CONSTRAINT chk_graphflow_run_items_operation CHECK (operation IN ('add', 'cleanup'))
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_graphflow_run_items_operation_document
			ON public.graphflow_run_items (run_id, operation, document_id);
		CREATE INDEX IF NOT EXISTS idx_graphflow_run_items_source_ref
			ON public.graphflow_run_items (source_ref_id);
		CREATE INDEX IF NOT EXISTS idx_graphflow_run_items_sync_batch
			ON public.graphflow_run_items (sync_batch_id);

		ALTER TABLE public.graphflow_tasks
			ADD COLUMN IF NOT EXISTS run_item_id uuid REFERENCES public.graphflow_run_items(id) ON DELETE SET NULL,
			ADD COLUMN IF NOT EXISTS source_ref_id uuid;
		CREATE INDEX IF NOT EXISTS idx_graphflow_tasks_run_item
			ON public.graphflow_tasks (run_item_id);
		CREATE INDEX IF NOT EXISTS idx_graphflow_tasks_source_ref
			ON public.graphflow_tasks (source_ref_id);
	`)
}

func downAddGraphFlowSyncBatches(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_graphflow_tasks_source_ref;
		DROP INDEX IF EXISTS public.idx_graphflow_tasks_run_item;
		ALTER TABLE public.graphflow_tasks
			DROP COLUMN IF EXISTS source_ref_id,
			DROP COLUMN IF EXISTS run_item_id;
		DROP TABLE IF EXISTS public.graphflow_run_items;
		DROP INDEX IF EXISTS public.idx_graphflow_runs_dataset_sync_batch;
		ALTER TABLE public.graphflow_runs DROP COLUMN IF EXISTS sync_batch_id;
		DROP INDEX IF EXISTS public.idx_data_library_kb_asset_refs_sync_batch;
		ALTER TABLE public.data_library_knowledge_base_asset_refs DROP COLUMN IF EXISTS sync_batch_id;
	`)
}
