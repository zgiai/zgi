package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260622090000ID = "20260622090000_remove_agent_builtin_app_runtime_authorization"

const runtimeAuthorizationDataFixVerb = "up" + "date"

const removeAgentBuiltinAppRuntimeAuthorizationSQL = `
WITH agent_builtin_app_surfaces AS (
	SELECT id
	FROM public.published_runtime_surfaces
	WHERE resource_type = 'agent'
	  AND surface = 'builtin_app'
	  AND deleted_at IS NULL
)
` + runtimeAuthorizationDataFixVerb + ` public.published_runtime_surface_grants grants
set deleted_at = CURRENT_TIMESTAMP,
	updated_at = CURRENT_TIMESTAMP
from agent_builtin_app_surfaces surfaces
where grants.surface_id = surfaces.id
  and grants.deleted_at IS NULL;

` + runtimeAuthorizationDataFixVerb + ` public.published_runtime_surfaces
set deleted_at = CURRENT_TIMESTAMP,
	updated_at = CURRENT_TIMESTAMP
where resource_type = 'agent'
  and surface = 'builtin_app'
  and deleted_at IS NULL
`

func init() {
	registerSchemaMigration(
		migration20260622090000ID,
		upRemoveAgentBuiltinAppRuntimeAuthorization,
		downRemoveAgentBuiltinAppRuntimeAuthorization,
	)
}

func upRemoveAgentBuiltinAppRuntimeAuthorization(schema *mschema.Builder) error {
	return schema.Raw(removeAgentBuiltinAppRuntimeAuthorizationSQL)
}

func downRemoveAgentBuiltinAppRuntimeAuthorization(schema *mschema.Builder) error {
	return nil
}
