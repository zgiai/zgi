package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260620090000ID = "20260620090000_create_published_runtime_authorization"

const createPublishedRuntimeSurfacesSQL = `
CREATE TABLE IF NOT EXISTS public.published_runtime_surfaces (
	id uuid DEFAULT public.uuid_generate_v4() NOT NULL PRIMARY KEY,
	resource_type varchar(32) NOT NULL,
	resource_id uuid NOT NULL,
	organization_id uuid NOT NULL,
	workspace_id uuid,
	surface varchar(32) NOT NULL,
	enabled boolean DEFAULT false NOT NULL,
	compatibility_source varchar(32) DEFAULT 'grant' NOT NULL,
	metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
	created_at timestamptz DEFAULT CURRENT_TIMESTAMP NOT NULL,
	updated_at timestamptz DEFAULT CURRENT_TIMESTAMP NOT NULL,
	deleted_at timestamptz,
	CONSTRAINT published_runtime_surfaces_resource_type_check CHECK (resource_type IN ('agent', 'builtin_workflow')),
	CONSTRAINT published_runtime_surfaces_surface_check CHECK (surface IN ('webapp', 'api', 'builtin_app', 'internal')),
	CONSTRAINT published_runtime_surfaces_source_check CHECK (compatibility_source IN ('legacy_agent_fields', 'grant', 'system_default'))
)
`

const createPublishedRuntimeSurfaceGrantsSQL = `
CREATE TABLE IF NOT EXISTS public.published_runtime_surface_grants (
	id uuid DEFAULT public.uuid_generate_v4() NOT NULL PRIMARY KEY,
	surface_id uuid NOT NULL REFERENCES public.published_runtime_surfaces(id) ON DELETE CASCADE,
	subject_type varchar(32) NOT NULL,
	subject_id uuid,
	enabled boolean DEFAULT true NOT NULL,
	created_at timestamptz DEFAULT CURRENT_TIMESTAMP NOT NULL,
	updated_at timestamptz DEFAULT CURRENT_TIMESTAMP NOT NULL,
	deleted_at timestamptz,
	CONSTRAINT published_runtime_surface_grants_subject_check CHECK (subject_type IN ('public', 'organization', 'department', 'account', 'internal'))
)
`

const createPublishedRuntimeAuthorizationIndexesSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_published_runtime_surfaces_active_unique
ON public.published_runtime_surfaces (resource_type, resource_id, surface)
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_published_runtime_surfaces_org
ON public.published_runtime_surfaces (organization_id, surface)
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_published_runtime_surfaces_workspace
ON public.published_runtime_surfaces (workspace_id, surface)
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_published_runtime_surface_grants_subject
ON public.published_runtime_surface_grants (subject_type, subject_id)
WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_published_runtime_surface_grants_subject_active
ON public.published_runtime_surface_grants (surface_id, subject_type, subject_id)
WHERE deleted_at IS NULL AND subject_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_published_runtime_surface_grants_null_subject_active
ON public.published_runtime_surface_grants (surface_id, subject_type)
WHERE deleted_at IS NULL AND subject_id IS NULL
`

const seedPublishedRuntimeAgentSurfacesSQL = `
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
	surfaces.surface,
	true,
	'legacy_agent_fields',
	jsonb_build_object('seeded_from', 'agents')
FROM public.agents
LEFT JOIN public.workspaces ON workspaces.id = agents.tenant_id
CROSS JOIN (VALUES ('webapp'), ('api'), ('builtin_app'), ('internal')) AS surfaces(surface)
WHERE agents.deleted_at IS NULL
ON CONFLICT DO NOTHING
`

const seedPublishedRuntimeAgentSurfaceGrantsSQL = `
INSERT INTO public.published_runtime_surface_grants (
	surface_id,
	subject_type,
	subject_id,
	enabled
)
SELECT
	surfaces.id,
	CASE surfaces.surface
		WHEN 'internal' THEN 'internal'
		WHEN 'api' THEN 'public'
		ELSE 'organization'
	END,
	CASE surfaces.surface
		WHEN 'webapp' THEN surfaces.organization_id
		WHEN 'builtin_app' THEN surfaces.organization_id
		ELSE NULL
	END,
	surfaces.enabled
FROM public.published_runtime_surfaces surfaces
WHERE surfaces.resource_type = 'agent'
  AND surfaces.surface IN ('webapp', 'api', 'builtin_app', 'internal')
  AND surfaces.deleted_at IS NULL
ON CONFLICT DO NOTHING
`

func init() {
	registerSchemaMigration(
		migration20260620090000ID,
		upCreatePublishedRuntimeAuthorization,
		downCreatePublishedRuntimeAuthorization,
	)
}

func upCreatePublishedRuntimeAuthorization(schema *mschema.Builder) error {
	for _, statement := range []string{
		createPublishedRuntimeSurfacesSQL,
		createPublishedRuntimeSurfaceGrantsSQL,
		createPublishedRuntimeAuthorizationIndexesSQL,
		seedPublishedRuntimeAgentSurfacesSQL,
		seedPublishedRuntimeAgentSurfaceGrantsSQL,
	} {
		if err := schema.Raw(statement); err != nil {
			return err
		}
	}
	return nil
}

func downCreatePublishedRuntimeAuthorization(schema *mschema.Builder) error {
	if err := schema.DropIfExists("published_runtime_surface_grants"); err != nil {
		return err
	}
	return schema.DropIfExists("published_runtime_surfaces")
}
