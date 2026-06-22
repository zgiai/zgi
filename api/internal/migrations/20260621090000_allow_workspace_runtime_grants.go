package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260621090000ID = "20260621090000_allow_workspace_runtime_grants"

const allowWorkspacePublishedRuntimeGrantSubjectsSQL = `
ALTER TABLE public.published_runtime_surface_grants
DROP CONSTRAINT IF EXISTS published_runtime_surface_grants_subject_check;

ALTER TABLE public.published_runtime_surface_grants
ADD CONSTRAINT published_runtime_surface_grants_subject_check
CHECK (subject_type IN ('public', 'organization', 'department', 'workspace', 'account', 'internal'))
`

const disallowWorkspacePublishedRuntimeGrantSubjectsSQL = `
ALTER TABLE public.published_runtime_surface_grants
DROP CONSTRAINT IF EXISTS published_runtime_surface_grants_subject_check;

ALTER TABLE public.published_runtime_surface_grants
ADD CONSTRAINT published_runtime_surface_grants_subject_check
CHECK (subject_type IN ('public', 'organization', 'department', 'account', 'internal'))
`

func init() {
	registerSchemaMigration(
		migration20260621090000ID,
		upAllowWorkspaceRuntimeGrants,
		downAllowWorkspaceRuntimeGrants,
	)
}

func upAllowWorkspaceRuntimeGrants(schema *mschema.Builder) error {
	return schema.Raw(allowWorkspacePublishedRuntimeGrantSubjectsSQL)
}

func downAllowWorkspaceRuntimeGrants(schema *mschema.Builder) error {
	return schema.Raw(disallowWorkspacePublishedRuntimeGrantSubjectsSQL)
}
