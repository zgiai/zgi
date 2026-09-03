package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const createConnectorOnboardingSQL = `
CREATE TABLE public.connector_onboarding_flows (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL, owner_account_id uuid NOT NULL,
 connector_id varchar(128) NOT NULL, intent varchar(24) NOT NULL CHECK (intent IN ('create_app','update_app')),
 connection_id uuid, expected_connection_revision integer, expected_credential_version integer,
 expected_app_id varchar(128), region varchar(8) NOT NULL CHECK (region IN ('feishu','lark')),
 requested_region varchar(8) NOT NULL CHECK (requested_region IN ('feishu','lark')),
 template_id varchar(128) NOT NULL, template_digest varchar(64) NOT NULL,
 accepted_definition_digest varchar(64) NOT NULL,
 idempotency_key_hash varchar(64) NOT NULL, request_fingerprint varchar(64) NOT NULL,
 state varchar(32) NOT NULL CHECK (state IN ('starting','awaiting_authorization','verifying','ready','cancelled','expired','failed','recovery_required')),
 safe_error_code varchar(128), result_connection_id uuid,
 generation integer NOT NULL DEFAULT 1 CHECK (generation > 0), revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
 lease_owner uuid, lease_until timestamptz, fencing bigint NOT NULL DEFAULT 0 CHECK (fencing >= 0),
 begin_dispatched boolean NOT NULL DEFAULT false, switched_region boolean NOT NULL DEFAULT false,
 poll_interval_seconds integer NOT NULL DEFAULT 5 CHECK (poll_interval_seconds BETWEEN 5 AND 300),
 expires_at timestamptz NOT NULL, next_poll_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL, terminal_at timestamptz,
 UNIQUE (id, organization_id, owner_account_id),
 UNIQUE (organization_id, owner_account_id, idempotency_key_hash),
 CHECK (template_digest ~ '^[a-f0-9]{64}$' AND accepted_definition_digest ~ '^[a-f0-9]{64}$'
  AND idempotency_key_hash ~ '^[a-f0-9]{64}$' AND request_fingerprint ~ '^[a-f0-9]{64}$'),
 CHECK ((intent = 'create_app' AND connection_id IS NULL AND expected_connection_revision IS NULL AND expected_credential_version IS NULL AND expected_app_id IS NULL)
  OR (intent = 'update_app' AND connection_id IS NOT NULL AND expected_connection_revision > 0 AND expected_credential_version > 0 AND expected_app_id IS NOT NULL)),
 CHECK ((lease_owner IS NULL) = (lease_until IS NULL)),
 CHECK ((state = 'ready') = (result_connection_id IS NOT NULL)),
 CHECK ((state IN ('starting','awaiting_authorization','verifying')) = (terminal_at IS NULL))
);
CREATE UNIQUE INDEX connector_onboarding_one_create ON public.connector_onboarding_flows (organization_id,owner_account_id,connector_id)
 WHERE state IN ('starting', 'awaiting_authorization', 'verifying') AND intent = 'create_app';
CREATE UNIQUE INDEX connector_onboarding_one_update ON public.connector_onboarding_flows (organization_id,owner_account_id,connection_id)
 WHERE state IN ('starting', 'awaiting_authorization', 'verifying') AND intent = 'update_app';
CREATE INDEX connector_onboarding_due ON public.connector_onboarding_flows (next_poll_at,lease_until)
 WHERE state IN ('starting', 'awaiting_authorization', 'verifying');
CREATE INDEX connector_onboarding_admission ON public.connector_onboarding_flows (organization_id,owner_account_id,created_at);
CREATE TABLE public.connector_onboarding_secrets (
 flow_id uuid PRIMARY KEY, organization_id uuid NOT NULL, owner_account_id uuid NOT NULL,
 generation integer NOT NULL CHECK (generation > 0), encrypted_device text NOT NULL CHECK (encrypted_device LIKE 'c1.%' AND octet_length(encrypted_device) < 131072),
 expires_at timestamptz NOT NULL,
 FOREIGN KEY (flow_id, organization_id, owner_account_id)
 REFERENCES public.connector_onboarding_flows (id, organization_id, owner_account_id) ON DELETE CASCADE
);
CREATE TABLE public.connector_connection_provider_evidence (
 connection_id uuid PRIMARY KEY, organization_id uuid NOT NULL, owner_account_id uuid NOT NULL,
 connector_id varchar(128) NOT NULL, credential_version integer NOT NULL CHECK (credential_version > 0),
 evidence jsonb NOT NULL CHECK (jsonb_typeof(evidence) = 'object' AND octet_length(evidence::text) <= 32768),
 verified_at timestamptz NOT NULL,
 FOREIGN KEY (connection_id, organization_id, owner_account_id, connector_id)
 REFERENCES public.connector_connections (id, organization_id, owner_account_id, connector_id) ON DELETE CASCADE
);
`

func init() {
	registerSchemaMigration("20260903110000_create_connector_onboarding", upCreateConnectorOnboarding, downCreateConnectorOnboarding)
}
func upCreateConnectorOnboarding(schema *mschema.Builder) error {
	return schema.Raw(createConnectorOnboardingSQL)
}
func downCreateConnectorOnboarding(schema *mschema.Builder) error {
	return schema.Raw("DROP TABLE public.connector_connection_provider_evidence; DROP TABLE public.connector_onboarding_secrets; DROP TABLE public.connector_onboarding_flows;")
}
