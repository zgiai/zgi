package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddRuntimeWorkspaceBindingsRevisionID = "202609221200000000_add_runtime_workspace_bindings_revision"

func init() {
	registerSchemaMigration(migrationAddRuntimeWorkspaceBindingsRevisionID, upAddRuntimeWorkspaceBindingsRevision, downAddRuntimeWorkspaceBindingsRevision)
}

func upAddRuntimeWorkspaceBindingsRevision(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.runtime_workspaces
			ADD COLUMN IF NOT EXISTS bindings_revision bigint NOT NULL DEFAULT 0;
	`)
}

func downAddRuntimeWorkspaceBindingsRevision(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.runtime_workspaces
			DROP COLUMN IF EXISTS bindings_revision;
	`)
}
