package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const createConnectorBackgroundGrantsSQL = `
CREATE TABLE public.connector_background_grants (
 id uuid PRIMARY KEY,
 organization_id uuid NOT NULL,
 owner_id uuid NOT NULL,
 workspace_id uuid NOT NULL,
 employee_id uuid NOT NULL,
 version_id uuid NOT NULL,
 binding_id uuid NOT NULL,
 connection_id uuid NOT NULL,
 binding_revision integer NOT NULL CHECK (binding_revision > 0),
 connection_revision integer NOT NULL CHECK (connection_revision > 0),
 credential_version integer NOT NULL CHECK (credential_version > 0),
 contract_digest varchar(64) NOT NULL CHECK (contract_digest ~ '^[0-9a-f]{64}$'),
 operations_json text NOT NULL,
 destinations_json text NOT NULL,
 expires_at timestamptz NOT NULL,
 max_invocations integer NOT NULL CHECK (max_invocations BETWEEN 1 AND 100000),
 used_invocations integer NOT NULL DEFAULT 0 CHECK (used_invocations >= 0 AND used_invocations <= max_invocations),
 revoked_at timestamptz,
 created_at timestamptz NOT NULL,
 CHECK (expires_at > created_at)
);
CREATE INDEX idx_background_grants_active ON public.connector_background_grants
 (organization_id, owner_id, binding_id, created_at) WHERE revoked_at IS NULL;
CREATE TABLE public.connector_background_consumptions (
 id uuid PRIMARY KEY,
 grant_id uuid NOT NULL REFERENCES public.connector_background_grants(id),
 binding_id uuid NOT NULL,
 logical_operation_id varchar(128) NOT NULL,
 operation_id varchar(128) NOT NULL,
 created_at timestamptz NOT NULL,
 UNIQUE (binding_id, logical_operation_id)
);
CREATE INDEX idx_background_consumptions_grant ON public.connector_background_consumptions(grant_id);
`

func init() {
	registerSchemaMigration("202610201000000000_create_connector_background_grants", up202610201000000000, down202610201000000000)
}
func up202610201000000000(schema *mschema.Builder) error {
	return schema.Raw(createConnectorBackgroundGrantsSQL)
}
func down202610201000000000(schema *mschema.Builder) error {
	return schema.Raw(`DROP TABLE IF EXISTS public.connector_background_consumptions; DROP TABLE IF EXISTS public.connector_background_grants;`)
}
