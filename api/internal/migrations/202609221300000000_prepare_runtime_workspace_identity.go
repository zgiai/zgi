package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migrationPrepareRuntimeWorkspaceIdentityID = "202609221300000000_prepare_runtime_workspace_identity"

func init() {
	registerSchemaMigration(migrationPrepareRuntimeWorkspaceIdentityID, upPrepareRuntimeWorkspaceIdentity, downPrepareRuntimeWorkspaceIdentity)
}

// upPrepareRuntimeWorkspaceIdentity introduces a stable Runtime Workspace
// identity without changing the meaning of the currently attached resource.
// Existing attached rows deliberately use workspace_id as runtime_id so that
// existing credentials, files, conversations, memories, and Subjects remain
// addressable without a data rewrite. Future isolated rows may use a distinct
// runtime_id while retaining workspace_id as their resource scope.
func upPrepareRuntimeWorkspaceIdentity(schema *mschema.Builder) error {
	if err := schema.Raw(`
		ALTER TABLE public.runtime_workspaces
			ADD COLUMN IF NOT EXISTS runtime_id uuid;
	`); err != nil {
		return err
	}
	if err := schema.DataFix("seed Runtime Workspace logical identities", func(db *gorm.DB) error {
		return db.Exec("UPDATE public.runtime_workspaces SET runtime_id = workspace_id WHERE runtime_id IS NULL").Error
	}); err != nil {
		return err
	}
	return schema.Raw(`
		ALTER TABLE public.runtime_workspaces
			ALTER COLUMN runtime_id SET DEFAULT public.uuid_generate_v4(),
			ALTER COLUMN runtime_id SET NOT NULL;
		CREATE UNIQUE INDEX IF NOT EXISTS uq_runtime_workspaces_runtime_id
			ON public.runtime_workspaces (runtime_id);

		ALTER TABLE public.runtime_workspaces
			DROP CONSTRAINT IF EXISTS ck_runtime_workspace_mode;
		ALTER TABLE public.runtime_workspaces
			ADD CONSTRAINT ck_runtime_workspace_mode CHECK (mode IN ('attached', 'isolated'));
	`)
}

func downPrepareRuntimeWorkspaceIdentity(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.runtime_workspaces
			DROP CONSTRAINT IF EXISTS ck_runtime_workspace_mode;
		ALTER TABLE public.runtime_workspaces
			ADD CONSTRAINT ck_runtime_workspace_mode CHECK (mode IN ('attached'));
		DROP INDEX IF EXISTS public.uq_runtime_workspaces_runtime_id;
		ALTER TABLE public.runtime_workspaces
			DROP COLUMN IF EXISTS runtime_id;
	`)
}
