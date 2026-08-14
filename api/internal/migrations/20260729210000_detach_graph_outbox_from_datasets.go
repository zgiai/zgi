package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationDetachGraphOutboxFromDatasetsID = "20260729210000_detach_graph_outbox_from_datasets"

func init() {
	registerSchemaMigration(
		migrationDetachGraphOutboxFromDatasetsID,
		upDetachGraphOutboxFromDatasets,
		downDetachGraphOutboxFromDatasets,
	)
}

func upDetachGraphOutboxFromDatasets(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.graph_outbox_events
		DROP CONSTRAINT IF EXISTS graph_outbox_events_dataset_id_fkey
	`)
}

func downDetachGraphOutboxFromDatasets(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.graph_outbox_events
		ADD CONSTRAINT graph_outbox_events_dataset_id_fkey
		FOREIGN KEY (dataset_id) REFERENCES public.datasets(id) ON DELETE CASCADE
		NOT VALID
	`)
}
