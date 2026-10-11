package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migrationAttachRuntimeWorkspacesID = "202609221100000000_attach_runtime_workspaces"

func init() {
	registerSchemaMigration(migrationAttachRuntimeWorkspacesID, upAttachRuntimeWorkspaces, downAttachRuntimeWorkspaces)
}

func upAttachRuntimeWorkspaces(schema *mschema.Builder) error {
	if err := schema.Raw(`
		ALTER TABLE public.runtime_workspaces
			ADD COLUMN IF NOT EXISTS mode varchar(24);
	`); err != nil {
		return err
	}
	if err := schema.DataFix("attach existing Runtime Workspaces", func(db *gorm.DB) error {
		return db.Exec("UPDATE public.runtime_workspaces SET mode = 'attached' WHERE mode IS NULL OR btrim(mode) = ''").Error
	}); err != nil {
		return err
	}
	return schema.Raw(`
		ALTER TABLE public.runtime_workspaces
			ALTER COLUMN mode SET DEFAULT 'attached',
			ALTER COLUMN mode SET NOT NULL;
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'ck_runtime_workspace_mode'
				  AND conrelid = 'public.runtime_workspaces'::regclass
			) THEN
				ALTER TABLE public.runtime_workspaces
					ADD CONSTRAINT ck_runtime_workspace_mode CHECK (mode IN ('attached'));
			END IF;
		END $$;

		CREATE TABLE IF NOT EXISTS public.runtime_workspace_credential_operations (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			runtime_workspace_id uuid NOT NULL REFERENCES public.runtime_workspaces(workspace_id) ON DELETE CASCADE,
			created_by_account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			idempotency_key_hash varchar(64) NOT NULL,
			request_fingerprint varchar(64) NOT NULL,
			state varchar(20) NOT NULL DEFAULT 'processing',
			credential_id uuid REFERENCES public.runtime_workspace_credentials(id) ON DELETE SET NULL,
			secret_envelope text,
			secret_expires_at timestamptz,
			expires_at timestamptz NOT NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT uq_runtime_workspace_credential_operation_key UNIQUE (runtime_workspace_id, created_by_account_id, idempotency_key_hash),
			CONSTRAINT ck_runtime_workspace_credential_operation_state CHECK (state IN ('processing', 'succeeded'))
		);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_credential_operations_expiry ON public.runtime_workspace_credential_operations (expires_at);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_credential_operations_credential ON public.runtime_workspace_credential_operations (credential_id);
	`)
}

func downAttachRuntimeWorkspaces(schema *mschema.Builder) error {
	if err := schema.DropIfExists("runtime_workspace_credential_operations"); err != nil {
		return err
	}
	return schema.Raw(`
		ALTER TABLE public.runtime_workspaces
			ADD COLUMN IF NOT EXISTS runtime_only boolean NOT NULL DEFAULT true,
			DROP CONSTRAINT IF EXISTS ck_runtime_workspace_mode,
			DROP COLUMN IF EXISTS mode;
	`)
}
