package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateDeveloperAccessID = "202609021800000000_create_developer_access"

const createDeveloperAccessSQL = `
	ALTER TABLE public.llm_organization_api_keys
		ALTER COLUMN key DROP NOT NULL,
		ADD COLUMN IF NOT EXISTS environment varchar(20) NOT NULL DEFAULT 'development',
		ADD COLUMN IF NOT EXISTS workspace_id uuid,
		ADD COLUMN IF NOT EXISTS principal_type varchar(32),
		ADD COLUMN IF NOT EXISTS principal_id varchar(255),
		ADD COLUMN IF NOT EXISTS access_grant_id uuid,
		ADD COLUMN IF NOT EXISTS created_by_account_id uuid,
		ADD COLUMN IF NOT EXISTS key_prefix varchar(16) NOT NULL DEFAULT '',
		ADD COLUMN IF NOT EXISTS key_suffix varchar(8) NOT NULL DEFAULT '',
		ADD COLUMN IF NOT EXISTS secret_format_version integer NOT NULL DEFAULT 1,
		ADD COLUMN IF NOT EXISTS revoked_at timestamptz,
		ADD COLUMN IF NOT EXISTS revoked_by_account_id uuid,
		ADD COLUMN IF NOT EXISTS revoked_reason varchar(500),
		ADD COLUMN IF NOT EXISTS rotated_from_key_id uuid,
		ADD COLUMN IF NOT EXISTS authorization_version bigint NOT NULL DEFAULT 1,
		ADD CONSTRAINT llm_api_keys_principal_pair_check CHECK (
			(principal_type IS NULL AND principal_id IS NULL)
			OR (principal_type IN ('user', 'service_account') AND principal_id IS NOT NULL)
		),
		ADD CONSTRAINT llm_api_keys_environment_check CHECK (environment IN ('development', 'production'));
	CREATE INDEX IF NOT EXISTS idx_llm_api_keys_workspace_principal
		ON public.llm_organization_api_keys (workspace_id, principal_type, principal_id, created_at DESC)
		WHERE deleted_at IS NULL;

	CREATE TABLE public.llm_developer_access_policies (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		mode varchar(32) NOT NULL DEFAULT 'approval_required',
		default_quota bigint,
		max_quota bigint,
		max_keys integer NOT NULL DEFAULT 3,
		default_ttl_seconds bigint,
		max_ttl_seconds bigint,
		allowed_models jsonb NOT NULL DEFAULT '[]'::jsonb,
		version bigint NOT NULL DEFAULT 1,
		updated_by_account_id uuid,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		deleted_at timestamptz,
		CONSTRAINT llm_developer_access_policies_mode_check CHECK (mode IN ('self_service', 'approval_required', 'disabled')),
		CONSTRAINT llm_developer_access_policies_quota_check CHECK (
			(default_quota IS NULL OR default_quota >= 0) AND
			(max_quota IS NULL OR max_quota >= 0) AND
			(default_quota IS NULL OR max_quota IS NULL OR default_quota <= max_quota)
		),
		CONSTRAINT llm_developer_access_policies_max_keys_check CHECK (max_keys BETWEEN 1 AND 100)
	);
	CREATE UNIQUE INDEX idx_llm_developer_access_policies_workspace
		ON public.llm_developer_access_policies (workspace_id) WHERE deleted_at IS NULL;

	CREATE TABLE public.llm_developer_access_requests (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		requester_account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE CASCADE,
		purpose text NOT NULL,
		environment varchar(20) NOT NULL DEFAULT 'development',
		requested_quota bigint,
		requested_models jsonb NOT NULL DEFAULT '[]'::jsonb,
		requested_ttl_seconds bigint,
		status varchar(20) NOT NULL DEFAULT 'pending',
		reviewer_account_id uuid,
		review_reason text,
		reviewed_at timestamptz,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		deleted_at timestamptz,
		CONSTRAINT llm_developer_access_requests_status_check CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
		CONSTRAINT llm_developer_access_requests_environment_check CHECK (environment IN ('development', 'production')),
		CONSTRAINT llm_developer_access_requests_quota_check CHECK (requested_quota IS NULL OR requested_quota >= 0)
	);
	CREATE UNIQUE INDEX idx_llm_developer_access_requests_one_pending
		ON public.llm_developer_access_requests (workspace_id, requester_account_id)
		WHERE status = 'pending' AND deleted_at IS NULL;
	CREATE INDEX idx_llm_developer_access_requests_review
		ON public.llm_developer_access_requests (workspace_id, status, created_at DESC);

	CREATE TABLE public.llm_developer_access_grants (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		principal_type varchar(32) NOT NULL,
		principal_id varchar(255) NOT NULL,
		source varchar(32) NOT NULL,
		status varchar(20) NOT NULL DEFAULT 'active',
		quota_limit bigint,
		used_quota bigint NOT NULL DEFAULT 0,
		remain_quota bigint NOT NULL DEFAULT 0,
		max_keys integer NOT NULL DEFAULT 3,
		allowed_models jsonb NOT NULL DEFAULT '[]'::jsonb,
		expires_at timestamptz,
		authorization_version bigint NOT NULL DEFAULT 1,
		created_by_account_id uuid,
		updated_by_account_id uuid,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		deleted_at timestamptz,
		CONSTRAINT llm_developer_access_grants_principal_check CHECK (principal_type IN ('user', 'service_account')),
		CONSTRAINT llm_developer_access_grants_status_check CHECK (status IN ('active', 'disabled', 'revoked')),
		CONSTRAINT llm_developer_access_grants_quota_check CHECK (
			quota_limit IS NULL OR (quota_limit >= 0 AND used_quota >= 0 AND remain_quota >= 0)
		),
		CONSTRAINT llm_developer_access_grants_max_keys_check CHECK (max_keys BETWEEN 1 AND 100)
	);
	CREATE UNIQUE INDEX idx_llm_developer_access_grants_principal
		ON public.llm_developer_access_grants (workspace_id, principal_type, principal_id) WHERE deleted_at IS NULL;

	ALTER TABLE public.llm_organization_api_keys
		ADD CONSTRAINT fk_llm_api_keys_access_grant FOREIGN KEY (access_grant_id)
		REFERENCES public.llm_developer_access_grants(id) ON DELETE RESTRICT;

	ALTER TABLE public.llm_usage_bills
		ADD COLUMN IF NOT EXISTS account_id uuid,
		ADD COLUMN IF NOT EXISTS principal_type varchar(32),
		ADD COLUMN IF NOT EXISTS principal_id varchar(255),
		ADD COLUMN IF NOT EXISTS access_grant_id uuid,
		ADD COLUMN IF NOT EXISTS auth_method varchar(32) NOT NULL DEFAULT 'legacy_api_key';
	CREATE INDEX IF NOT EXISTS idx_llm_usage_bills_workspace_principal
		ON public.llm_usage_bills (workspace_id, principal_type, principal_id, request_created_at DESC);
`

const rollbackDeveloperAccessSQL = `
	DO $$
	BEGIN
		IF EXISTS (SELECT 1 FROM public.llm_developer_access_requests LIMIT 1)
			OR EXISTS (SELECT 1 FROM public.llm_developer_access_grants LIMIT 1)
			OR EXISTS (SELECT 1 FROM public.llm_developer_access_policies LIMIT 1) THEN
			RAISE EXCEPTION 'cannot remove developer access schema while records exist';
		END IF;
	END
	$$;
	ALTER TABLE public.llm_organization_api_keys DROP CONSTRAINT IF EXISTS fk_llm_api_keys_access_grant;
	DROP TABLE public.llm_developer_access_grants;
	DROP TABLE public.llm_developer_access_requests;
	DROP TABLE public.llm_developer_access_policies;
	DROP INDEX IF EXISTS public.idx_llm_usage_bills_workspace_principal;
	ALTER TABLE public.llm_usage_bills
		DROP COLUMN IF EXISTS auth_method,
		DROP COLUMN IF EXISTS access_grant_id,
		DROP COLUMN IF EXISTS principal_id,
		DROP COLUMN IF EXISTS principal_type,
		DROP COLUMN IF EXISTS account_id;
	DROP INDEX IF EXISTS public.idx_llm_api_keys_workspace_principal;
	ALTER TABLE public.llm_organization_api_keys
		DROP CONSTRAINT IF EXISTS llm_api_keys_principal_pair_check,
		DROP CONSTRAINT IF EXISTS llm_api_keys_environment_check,
		DROP COLUMN IF EXISTS authorization_version,
		DROP COLUMN IF EXISTS rotated_from_key_id,
		DROP COLUMN IF EXISTS revoked_reason,
		DROP COLUMN IF EXISTS revoked_by_account_id,
		DROP COLUMN IF EXISTS revoked_at,
		DROP COLUMN IF EXISTS secret_format_version,
		DROP COLUMN IF EXISTS key_suffix,
		DROP COLUMN IF EXISTS key_prefix,
		DROP COLUMN IF EXISTS created_by_account_id,
		DROP COLUMN IF EXISTS access_grant_id,
		DROP COLUMN IF EXISTS principal_id,
		DROP COLUMN IF EXISTS principal_type,
		DROP COLUMN IF EXISTS workspace_id,
		DROP COLUMN IF EXISTS environment,
		ALTER COLUMN key SET NOT NULL;
`

func init() {
	registerSchemaMigration(migrationCreateDeveloperAccessID, upCreateDeveloperAccess, downCreateDeveloperAccess)
}

func upCreateDeveloperAccess(schema *mschema.Builder) error {
	return schema.Raw(createDeveloperAccessSQL)
}
func downCreateDeveloperAccess(schema *mschema.Builder) error {
	return schema.Raw(rollbackDeveloperAccessSQL)
}
