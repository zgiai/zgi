package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260721090000ID                  = "20260721090000_create_integration_connections_and_policies"
	createIntegrationConnectionsAndPoliciesSQL = `
		CREATE TABLE public.integration_connections (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			integration_id varchar(64) NOT NULL,
			driver_id varchar(64) NOT NULL,
			name varchar(128) NOT NULL,
			credential_source varchar(32) NOT NULL,
			auth_type varchar(32) NOT NULL,
			encrypted_credentials text,
			config jsonb NOT NULL DEFAULT '{}'::jsonb,
			account_id varchar(255),
			display_name varchar(255),
			granted_scopes jsonb NOT NULL DEFAULT '[]'::jsonb,
			status varchar(32) NOT NULL DEFAULT 'pending',
			is_default boolean NOT NULL DEFAULT false,
			credential_version integer NOT NULL DEFAULT 1,
			revision integer NOT NULL DEFAULT 1,
			last_tested_at timestamptz,
			last_error_code varchar(64),
			expires_at timestamptz,
			created_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
			updated_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at timestamptz,
			CONSTRAINT integration_connections_credential_source_check
				CHECK (credential_source IN ('platform', 'organization')),
			CONSTRAINT integration_connections_auth_type_check
				CHECK (auth_type IN ('platform', 'api_key')),
			CONSTRAINT integration_connections_status_check
				CHECK (status IN ('pending', 'active', 'invalid', 'disabled')),
			CONSTRAINT integration_connections_credential_version_positive
				CHECK (credential_version >= 1),
			CONSTRAINT integration_connections_revision_positive
				CHECK (revision >= 1),
			CONSTRAINT integration_connections_config_object_check
				CHECK (jsonb_typeof(config) = 'object'),
			CONSTRAINT integration_connections_scopes_array_check
				CHECK (jsonb_typeof(granted_scopes) = 'array'),
			CONSTRAINT integration_connections_credential_storage_check
				CHECK (
					(credential_source = 'platform' AND auth_type = 'platform' AND encrypted_credentials IS NULL)
					OR
					(credential_source = 'organization' AND auth_type = 'api_key' AND encrypted_credentials IS NOT NULL AND char_length(encrypted_credentials) > 3)
				),
			CONSTRAINT integration_connections_default_active_check
				CHECK (NOT is_default OR status = 'active')
		);
		CREATE UNIQUE INDEX idx_integration_connections_org_name_active
			ON public.integration_connections (organization_id, integration_id, lower(name))
			WHERE deleted_at IS NULL;
		CREATE UNIQUE INDEX idx_integration_connections_org_default_active
			ON public.integration_connections (organization_id, integration_id)
			WHERE is_default = true AND deleted_at IS NULL;
		CREATE INDEX idx_integration_connections_org_integration
			ON public.integration_connections (organization_id, integration_id, status)
			WHERE deleted_at IS NULL;

		CREATE TABLE public.integration_action_policies (
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			integration_id varchar(64) NOT NULL,
			action_id varchar(128) NOT NULL,
			enabled boolean NOT NULL DEFAULT true,
			approval_policy varchar(32) NOT NULL DEFAULT 'inherit',
			data_egress_allowed boolean NOT NULL DEFAULT true,
			updated_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (organization_id, integration_id, action_id),
			CONSTRAINT integration_action_policies_approval_check
				CHECK (approval_policy IN ('inherit', 'always_ask'))
		);
		CREATE INDEX idx_integration_action_policies_org_integration
			ON public.integration_action_policies (organization_id, integration_id)
	`
)

func init() {
	registerSchemaMigration(migration20260721090000ID, up20260721090000, down20260721090000)
}

func up20260721090000(schema *mschema.Builder) error {
	return schema.Raw(createIntegrationConnectionsAndPoliciesSQL)
}

func down20260721090000(schema *mschema.Builder) error {
	if err := schema.DropIfExists("integration_action_policies"); err != nil {
		return err
	}
	return schema.DropIfExists("integration_connections")
}
