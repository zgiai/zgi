package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260623090000ID = "20260623090000_add_agent_app_center_runtime_authorization"

const allowAppCenterPublishedRuntimeSurfacesSQL = `
ALTER TABLE public.published_runtime_surfaces
DROP CONSTRAINT IF EXISTS published_runtime_surfaces_surface_check;

ALTER TABLE public.published_runtime_surfaces
ADD CONSTRAINT published_runtime_surfaces_surface_check
CHECK (surface IN ('webapp', 'api', 'app_center', 'builtin_app', 'internal'))
`

const seedAgentAppCenterRuntimeSurfacesSQL = `
INSERT INTO public.published_runtime_surfaces (
	resource_type,
	resource_id,
	organization_id,
	workspace_id,
	surface,
	enabled,
	compatibility_source,
	metadata
)
SELECT
	'agent',
	agents.id,
	COALESCE(workspaces.organization_id, agents.tenant_id),
	agents.tenant_id,
	'app_center',
	true,
	'legacy_agent_fields',
	jsonb_build_object('seeded_from', 'agents_app_center')
FROM public.agents
LEFT JOIN public.workspaces ON workspaces.id = agents.tenant_id
WHERE agents.deleted_at IS NULL
ON CONFLICT DO NOTHING
`

const seedAgentAppCenterRuntimeGrantsSQL = `
INSERT INTO public.published_runtime_surface_grants (
	surface_id,
	subject_type,
	subject_id,
	enabled
)
SELECT
	surfaces.id,
	'workspace',
	surfaces.workspace_id,
	surfaces.enabled
FROM public.published_runtime_surfaces surfaces
WHERE surfaces.resource_type = 'agent'
  AND surfaces.surface = 'app_center'
  AND surfaces.workspace_id IS NOT NULL
  AND surfaces.deleted_at IS NULL
ON CONFLICT DO NOTHING
`

func init() {
	registerSchemaMigration(
		migration20260623090000ID,
		upAddAgentAppCenterRuntimeAuthorization,
		downAddAgentAppCenterRuntimeAuthorization,
	)
}

func upAddAgentAppCenterRuntimeAuthorization(schema *mschema.Builder) error {
	for _, statement := range []string{
		allowAppCenterPublishedRuntimeSurfacesSQL,
		seedAgentAppCenterRuntimeSurfacesSQL,
		seedAgentAppCenterRuntimeGrantsSQL,
	} {
		if err := schema.Raw(statement); err != nil {
			return err
		}
	}
	return nil
}

func downAddAgentAppCenterRuntimeAuthorization(schema *mschema.Builder) error {
	return nil
}
