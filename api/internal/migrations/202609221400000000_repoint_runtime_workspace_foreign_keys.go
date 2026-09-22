package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationRepointRuntimeWorkspaceForeignKeysID = "202609221400000000_repoint_runtime_workspace_foreign_keys"

func init() {
	registerSchemaMigration(migrationRepointRuntimeWorkspaceForeignKeysID, upRepointRuntimeWorkspaceForeignKeys, downRepointRuntimeWorkspaceForeignKeys)
}

// upRepointRuntimeWorkspaceForeignKeys makes runtime_id the stable parent key
// for Runtime Workspace-owned rows. workspace_id remains the resource scope
// used by the currently attached implementation, while future isolated
// Runtime Workspaces can use a different resource workspace without changing
// the child tables again.
func upRepointRuntimeWorkspaceForeignKeys(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.runtime_workspaces
			DROP CONSTRAINT IF EXISTS uq_runtime_workspaces_runtime_id;
		DROP INDEX IF EXISTS public.uq_runtime_workspaces_runtime_id;
		ALTER TABLE public.runtime_workspaces
			ADD CONSTRAINT uq_runtime_workspaces_runtime_id UNIQUE (runtime_id);

		DO $$
		DECLARE
			fk record;
		BEGIN
			FOR fk IN
				SELECT
					n.nspname AS schema_name,
					c.relname AS table_name,
					con.conname AS constraint_name
				FROM pg_constraint con
				JOIN pg_class c ON c.oid = con.conrelid
				JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE con.contype = 'f'
				  AND con.confrelid = 'public.runtime_workspaces'::regclass
				  AND c.relname IN (
					'runtime_workspace_credentials',
					'runtime_workspace_credential_operations',
					'runtime_workspace_agents',
					'runtime_external_files'
				  )
				  AND EXISTS (
					SELECT 1
					FROM pg_attribute a
					WHERE a.attrelid = con.conrelid
					  AND a.attnum = con.conkey[1]
					  AND a.attname = 'runtime_workspace_id'
				  )
			LOOP
				EXECUTE format(
					'ALTER TABLE %I.%I DROP CONSTRAINT %I',
					fk.schema_name,
					fk.table_name,
					fk.constraint_name
				);
			END LOOP;
		END $$;

		ALTER TABLE public.runtime_workspace_credentials
			ADD CONSTRAINT fk_runtime_workspace_credentials_runtime_workspace
			FOREIGN KEY (runtime_workspace_id)
			REFERENCES public.runtime_workspaces(runtime_id)
			ON DELETE CASCADE;
		ALTER TABLE public.runtime_workspace_credential_operations
			ADD CONSTRAINT fk_runtime_workspace_credential_operations_runtime_workspace
			FOREIGN KEY (runtime_workspace_id)
			REFERENCES public.runtime_workspaces(runtime_id)
			ON DELETE CASCADE;
		ALTER TABLE public.runtime_workspace_agents
			ADD CONSTRAINT fk_runtime_workspace_agents_runtime_workspace
			FOREIGN KEY (runtime_workspace_id)
			REFERENCES public.runtime_workspaces(runtime_id)
			ON DELETE CASCADE;
		ALTER TABLE public.runtime_external_files
			ADD CONSTRAINT fk_runtime_external_files_runtime_workspace
			FOREIGN KEY (runtime_workspace_id)
			REFERENCES public.runtime_workspaces(runtime_id)
			ON DELETE CASCADE;
	`)
}

func downRepointRuntimeWorkspaceForeignKeys(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.runtime_workspace_credentials
			DROP CONSTRAINT IF EXISTS fk_runtime_workspace_credentials_runtime_workspace;
		ALTER TABLE public.runtime_workspace_credential_operations
			DROP CONSTRAINT IF EXISTS fk_runtime_workspace_credential_operations_runtime_workspace;
		ALTER TABLE public.runtime_workspace_agents
			DROP CONSTRAINT IF EXISTS fk_runtime_workspace_agents_runtime_workspace;
		ALTER TABLE public.runtime_external_files
			DROP CONSTRAINT IF EXISTS fk_runtime_external_files_runtime_workspace;

		ALTER TABLE public.runtime_workspace_credentials
			ADD CONSTRAINT runtime_workspace_credentials_runtime_workspace_id_fkey
			FOREIGN KEY (runtime_workspace_id)
			REFERENCES public.runtime_workspaces(workspace_id)
			ON DELETE CASCADE;
		ALTER TABLE public.runtime_workspace_credential_operations
			ADD CONSTRAINT runtime_workspace_credential_operations_runtime_workspace_id_fkey
			FOREIGN KEY (runtime_workspace_id)
			REFERENCES public.runtime_workspaces(workspace_id)
			ON DELETE CASCADE;
		ALTER TABLE public.runtime_workspace_agents
			ADD CONSTRAINT runtime_workspace_agents_runtime_workspace_id_fkey
			FOREIGN KEY (runtime_workspace_id)
			REFERENCES public.runtime_workspaces(workspace_id)
			ON DELETE CASCADE;
		ALTER TABLE public.runtime_external_files
			ADD CONSTRAINT runtime_external_files_runtime_workspace_id_fkey
			FOREIGN KEY (runtime_workspace_id)
			REFERENCES public.runtime_workspaces(workspace_id)
			ON DELETE CASCADE;

		ALTER TABLE public.runtime_workspaces
			DROP CONSTRAINT IF EXISTS uq_runtime_workspaces_runtime_id;
		CREATE UNIQUE INDEX IF NOT EXISTS uq_runtime_workspaces_runtime_id
			ON public.runtime_workspaces (runtime_id);
	`)
}
