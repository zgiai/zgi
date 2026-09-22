package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateRuntimeWorkspacesID = "202609221000000000_create_runtime_workspaces"

func init() {
	registerSchemaMigration(migrationCreateRuntimeWorkspacesID, upCreateRuntimeWorkspaces, downCreateRuntimeWorkspaces)
}

func upCreateRuntimeWorkspaces(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.user_subjects
			DROP CONSTRAINT IF EXISTS ck_user_memory_subject_identity;
		ALTER TABLE public.user_subjects
			ADD CONSTRAINT ck_user_memory_subject_identity CHECK (
				(kind = 'account' AND account_id = id AND issuer_id IS NULL) OR
				(kind = 'api_end_user' AND account_id IS NULL AND issuer_type IN ('agent_api', 'digital_human_api', 'runtime_workspace_api') AND issuer_id IS NOT NULL)
			);

		CREATE TABLE IF NOT EXISTS public.runtime_workspaces (
			workspace_id uuid PRIMARY KEY REFERENCES public.workspaces(id) ON DELETE CASCADE,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			managed_by_account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			status varchar(24) NOT NULL DEFAULT 'active',
			mode varchar(24) NOT NULL DEFAULT 'attached',
			policy_json jsonb NOT NULL DEFAULT '{}'::jsonb,
			bindings_revision bigint NOT NULL DEFAULT 0,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT ck_runtime_workspace_status CHECK (status IN ('active', 'disabled')),
			CONSTRAINT ck_runtime_workspace_mode CHECK (mode IN ('attached'))
		);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspaces_org ON public.runtime_workspaces (organization_id, status);

		CREATE TABLE IF NOT EXISTS public.runtime_workspace_credentials (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			runtime_workspace_id uuid NOT NULL REFERENCES public.runtime_workspaces(workspace_id) ON DELETE CASCADE,
			key_prefix varchar(20) NOT NULL,
			key_hash varchar(64) NOT NULL UNIQUE,
			name varchar(255) NOT NULL DEFAULT '',
			status varchar(20) NOT NULL DEFAULT 'active',
			expires_at timestamptz,
			usage_count bigint NOT NULL DEFAULT 0,
			last_used_at timestamptz,
			revoked_at timestamptz,
			created_by_account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT ck_runtime_workspace_credential_status CHECK (status IN ('active', 'revoked'))
		);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_credentials_owner ON public.runtime_workspace_credentials (runtime_workspace_id, created_at DESC);

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

		CREATE TABLE IF NOT EXISTS public.runtime_workspace_agents (
			runtime_workspace_id uuid NOT NULL REFERENCES public.runtime_workspaces(workspace_id) ON DELETE CASCADE,
			agent_id uuid NOT NULL REFERENCES public.agents(id) ON DELETE CASCADE,
			public_app_key varchar(128) NOT NULL,
			status varchar(20) NOT NULL DEFAULT 'active',
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (runtime_workspace_id, agent_id),
			CONSTRAINT uq_runtime_workspace_app_key UNIQUE (runtime_workspace_id, public_app_key),
			CONSTRAINT ck_runtime_workspace_agent_status CHECK (status IN ('active', 'disabled'))
		);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_agents_app ON public.runtime_workspace_agents (public_app_key, status);
		CREATE INDEX IF NOT EXISTS idx_runtime_workspace_agents_listing ON public.runtime_workspace_agents (runtime_workspace_id, status, created_at);
		CREATE INDEX IF NOT EXISTS idx_agent_published_versions_agent_workspace_active ON public.agent_published_versions (agent_id, workspace_id) WHERE deleted_at IS NULL;

		CREATE TABLE IF NOT EXISTS public.runtime_external_files (
			file_id uuid PRIMARY KEY REFERENCES public.upload_files(id) ON DELETE CASCADE,
			runtime_workspace_id uuid NOT NULL REFERENCES public.runtime_workspaces(workspace_id) ON DELETE CASCADE,
			workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			subject_id uuid NOT NULL REFERENCES public.user_subjects(id) ON DELETE RESTRICT,
			subject_kind varchar(32) NOT NULL DEFAULT 'api_end_user',
			surface varchar(32) NOT NULL DEFAULT 'runtime_workspace_api',
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_runtime_external_files_subject ON public.runtime_external_files (runtime_workspace_id, subject_id, created_at DESC);
	`)
}

func downCreateRuntimeWorkspaces(schema *mschema.Builder) error {
	if err := schema.DropIfExists("runtime_external_files"); err != nil {
		return err
	}
	if err := schema.DropIfExists("runtime_workspace_agents"); err != nil {
		return err
	}
	if err := schema.DropIfExists("runtime_workspace_credential_operations"); err != nil {
		return err
	}
	if err := schema.DropIfExists("runtime_workspace_credentials"); err != nil {
		return err
	}
	if err := schema.DropIfExists("runtime_workspaces"); err != nil {
		return err
	}
	return schema.Raw(`
		ALTER TABLE public.user_subjects
			DROP CONSTRAINT IF EXISTS ck_user_memory_subject_identity;
		ALTER TABLE public.user_subjects
			ADD CONSTRAINT ck_user_memory_subject_identity CHECK (
				(kind = 'account' AND account_id = id AND issuer_id IS NULL) OR
				(kind = 'api_end_user' AND account_id IS NULL AND issuer_type IN ('agent_api', 'digital_human_api') AND issuer_id IS NOT NULL)
			);
	`)
}
