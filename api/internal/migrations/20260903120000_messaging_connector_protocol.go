package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Forward-only expansion. Rollback of application code requires draining new
// flows and retaining this schema; no connection or secret data is dropped.
const messagingConnectorProtocolSQL = `
ALTER TABLE public.connector_onboarding_flows
 ADD COLUMN protocol_version integer NOT NULL DEFAULT 0,
 ADD COLUMN handler varchar(128) NOT NULL DEFAULT '',
 ADD COLUMN step varchar(32) NOT NULL DEFAULT '',
 ADD COLUMN step_dispatched boolean NOT NULL DEFAULT false,
 ADD COLUMN expected_identity varchar(300),
 ADD COLUMN challenge_revision bigint NOT NULL DEFAULT 0 CHECK (challenge_revision >= 0),
 ADD COLUMN challenge_attempts integer NOT NULL DEFAULT 0 CHECK (challenge_attempts BETWEEN 0 AND 3),
 ADD COLUMN expiry_source varchar(32) NOT NULL DEFAULT '',
 DROP CONSTRAINT connector_onboarding_flows_intent_check,
 DROP CONSTRAINT connector_onboarding_flows_region_check,
 DROP CONSTRAINT connector_onboarding_flows_requested_region_check,
 DROP CONSTRAINT connector_onboarding_flows_poll_interval_seconds_check,
 DROP CONSTRAINT connector_onboarding_flows_check,
 DROP CONSTRAINT connector_onboarding_flows_check1;
ALTER TABLE public.connector_onboarding_flows
 ADD CONSTRAINT connector_onboarding_contract_check CHECK (
  (protocol_version = 0 AND intent IN ('create_app','update_app') AND region IN ('feishu','lark')
   AND requested_region IN ('feishu','lark') AND template_digest ~ '^[a-f0-9]{64}$'
   AND poll_interval_seconds BETWEEN 5 AND 300 AND handler = '' AND step = '' AND expected_identity IS NULL)
  OR (protocol_version = 1 AND connector_id IN ('dingtalk','wecom','weixin')
   AND handler = connector_id || '-registration-v1'
   AND ((connector_id = 'weixin' AND intent IN ('login_session','reauthorize'))
     OR (connector_id IN ('dingtalk','wecom') AND intent IN ('register_bot','reauthorize')))
   AND region = '' AND requested_region = '' AND template_id = '' AND template_digest = ''
   AND poll_interval_seconds BETWEEN 1 AND 300 AND step IN ('init','begin','poll','awaiting_code','verify')
   AND (step <> 'init' OR connector_id = 'dingtalk')
   AND (step <> 'awaiting_code' OR connector_id = 'weixin')
   AND expected_app_id IS NULL)),
 ADD CONSTRAINT connector_onboarding_digest_check CHECK (accepted_definition_digest ~ '^[a-f0-9]{64}$'
   AND idempotency_key_hash ~ '^[a-f0-9]{64}$' AND request_fingerprint ~ '^[a-f0-9]{64}$'),
 ADD CONSTRAINT connector_onboarding_identity_check CHECK (
  (intent IN ('create_app','register_bot','login_session') AND connection_id IS NULL AND expected_connection_revision IS NULL AND expected_credential_version IS NULL AND expected_app_id IS NULL AND expected_identity IS NULL)
  OR (intent = 'update_app' AND connection_id IS NOT NULL AND expected_connection_revision > 0 AND expected_credential_version > 0 AND expected_app_id IS NOT NULL)
  OR (intent = 'reauthorize' AND connection_id IS NOT NULL AND expected_connection_revision > 0 AND expected_credential_version > 0 AND expected_identity IS NOT NULL));
DROP INDEX public.connector_onboarding_one_create;
DROP INDEX public.connector_onboarding_one_update;
CREATE UNIQUE INDEX connector_onboarding_one_create ON public.connector_onboarding_flows (organization_id,owner_account_id,connector_id)
 WHERE state IN ('starting','awaiting_authorization','verifying') AND connection_id IS NULL;
CREATE UNIQUE INDEX connector_onboarding_one_update ON public.connector_onboarding_flows (organization_id,owner_account_id,connection_id)
 WHERE state IN ('starting','awaiting_authorization','verifying') AND connection_id IS NOT NULL;

CREATE TABLE public.connector_messaging_states (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL, owner_account_id uuid NOT NULL,
 connector_id varchar(128) NOT NULL CHECK (connector_id IN ('weixin','wecom')),
 connection_id uuid NOT NULL, credential_digest varchar(64) NOT NULL CHECK (credential_digest ~ '^[a-f0-9]{64}$'),
 flow_id uuid, revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
 encrypted_state text NOT NULL CHECK (encrypted_state LIKE 'c1.%' AND octet_length(encrypted_state) < 524288),
 expires_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
 UNIQUE (organization_id,owner_account_id,connector_id,connection_id,credential_digest)
);
CREATE INDEX connector_messaging_states_expiry ON public.connector_messaging_states (expires_at);
CREATE TABLE public.connector_messaging_contexts (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL, owner_account_id uuid NOT NULL,
 connector_id varchar(128) NOT NULL CHECK (connector_id IN ('weixin','wecom')),
 connection_id uuid NOT NULL, credential_version integer NOT NULL CHECK (credential_version > 0),
 binding_id uuid NOT NULL, binding_revision integer NOT NULL CHECK (binding_revision > 0),
 target_digest varchar(64) NOT NULL CHECK (target_digest ~ '^[a-f0-9]{64}$'),
 encrypted_context text NOT NULL CHECK (encrypted_context LIKE 'c1.%' AND octet_length(encrypted_context) < 131072),
 expires_at timestamptz NOT NULL,
 FOREIGN KEY (connection_id,organization_id,owner_account_id,connector_id)
 REFERENCES public.connector_connections (id,organization_id,owner_account_id,connector_id) ON DELETE CASCADE
);
CREATE INDEX connector_messaging_contexts_expiry ON public.connector_messaging_contexts (expires_at);
`

func init() {
	registerSchemaMigration("20260903120000_messaging_connector_protocol", upMessagingConnectorProtocol, nil)
}
func upMessagingConnectorProtocol(schema *mschema.Builder) error {
	return schema.Raw(messagingConnectorProtocolSQL)
}
