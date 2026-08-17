package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationDetachGraphCleanupTasksFromDocumentsID = "20260728161000_detach_graph_cleanup_tasks_from_documents"

func init() {
	registerSchemaMigration(
		migrationDetachGraphCleanupTasksFromDocumentsID,
		upDetachGraphCleanupTasksFromDocuments,
		downDetachGraphCleanupTasksFromDocuments,
	)
}

func upDetachGraphCleanupTasksFromDocuments(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.graphflow_tasks
		DROP CONSTRAINT IF EXISTS fk_graphflow_tasks_document
	`)
}

func downDetachGraphCleanupTasksFromDocuments(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.graphflow_tasks
		ADD CONSTRAINT fk_graphflow_tasks_document
		FOREIGN KEY (document_id) REFERENCES public.documents(id) ON DELETE CASCADE
		NOT VALID
	`)
}
