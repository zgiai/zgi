package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Forward-only: preserve all existing connections, bindings and protocol-1 flows.
const connectorPendingSetupsSQL = `
CREATE TABLE public.connector_pending_setups (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL, owner_account_id uuid NOT NULL,
 connector_id varchar(128) NOT NULL CHECK (connector_id = 'feishu'),
 region varchar(8) NOT NULL CHECK (region IN ('feishu','lark')),
 app_id varchar(128) NOT NULL CHECK (app_id ~ '^[A-Za-z0-9_-]{1,128}$'),
 source_flow_id uuid NOT NULL UNIQUE,
 template_id varchar(128) NOT NULL, template_digest varchar(64) NOT NULL CHECK (template_digest ~ '^[a-f0-9]{64}$'),
 accepted_definition_digest varchar(64) NOT NULL CHECK (accepted_definition_digest ~ '^[a-f0-9]{64}$'),
 state varchar(32) NOT NULL CHECK (state IN ('verification_required','awaiting_application','completed','cancelled','expired')),
 safe_reason_code varchar(128), last_verification_at timestamptz,
 revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0), result_connection_id uuid,
 resume_expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL, terminal_at timestamptz,
 UNIQUE (id,organization_id,owner_account_id),
 CHECK ((state = 'completed') = (result_connection_id IS NOT NULL)),
 CHECK ((state IN ('verification_required','awaiting_application')) = (terminal_at IS NULL)),
 CHECK (resume_expires_at > created_at)
);
CREATE INDEX connector_pending_owner ON public.connector_pending_setups (organization_id,owner_account_id,created_at,id);
CREATE INDEX connector_pending_expiry ON public.connector_pending_setups (resume_expires_at) WHERE terminal_at IS NULL;
ALTER TABLE public.connector_onboarding_flows
 ADD COLUMN pending_setup_id uuid,
 ADD COLUMN expected_pending_revision bigint CHECK (expected_pending_revision > 0),
 ADD COLUMN manual_verification boolean NOT NULL DEFAULT false,
 ADD COLUMN verification_deadline timestamptz,
 ADD CONSTRAINT connector_onboarding_pending_fk FOREIGN KEY (pending_setup_id,organization_id,owner_account_id)
 REFERENCES public.connector_pending_setups (id,organization_id,owner_account_id),
 DROP CONSTRAINT connector_onboarding_flows_state_check,
 DROP CONSTRAINT connector_onboarding_contract_check,
 DROP CONSTRAINT connector_onboarding_identity_check;
ALTER TABLE public.connector_onboarding_flows
 ADD CONSTRAINT connector_onboarding_flows_state_check CHECK (state IN ('starting','awaiting_authorization','verifying','ready','cancelled','expired','failed','recovery_required','pending_completion')),
 ADD CONSTRAINT connector_onboarding_contract_check CHECK (
  (protocol_version = 0 AND intent IN ('create_app','update_app','resume_app') AND region IN ('feishu','lark')
   AND requested_region IN ('feishu','lark') AND template_digest ~ '^[a-f0-9]{64}$'
   AND poll_interval_seconds BETWEEN 5 AND 300 AND handler = '' AND step = '' AND expected_identity IS NULL)
  OR (protocol_version = 1 AND connector_id IN ('dingtalk','wecom','weixin')
   AND handler = connector_id || '-registration-v1'
   AND ((connector_id = 'weixin' AND intent IN ('login_session','reauthorize'))
     OR (connector_id IN ('dingtalk','wecom') AND intent IN ('register_bot','reauthorize')))
   AND region = '' AND requested_region = '' AND template_id = '' AND template_digest = ''
   AND poll_interval_seconds BETWEEN 1 AND 300 AND step IN ('init','begin','poll','awaiting_code','verify')
   AND (step <> 'init' OR connector_id = 'dingtalk')
   AND (step <> 'awaiting_code' OR connector_id = 'weixin') AND expected_app_id IS NULL)),
 ADD CONSTRAINT connector_onboarding_identity_check CHECK (
  (intent IN ('create_app','register_bot','login_session') AND connection_id IS NULL AND expected_connection_revision IS NULL AND expected_credential_version IS NULL AND expected_app_id IS NULL AND expected_identity IS NULL)
  OR (intent = 'update_app' AND connection_id IS NOT NULL AND expected_connection_revision > 0 AND expected_credential_version > 0 AND expected_app_id IS NOT NULL)
  OR (intent = 'reauthorize' AND connection_id IS NOT NULL AND expected_connection_revision > 0 AND expected_credential_version > 0 AND expected_identity IS NOT NULL)
  OR (intent = 'resume_app' AND connector_id = 'feishu' AND connection_id IS NULL AND expected_connection_revision IS NULL AND expected_credential_version IS NULL AND expected_app_id IS NOT NULL AND pending_setup_id IS NOT NULL AND expected_pending_revision IS NOT NULL)),
 ADD CONSTRAINT connector_onboarding_pending_check CHECK (
  (pending_setup_id IS NULL OR (protocol_version = 0 AND connector_id = 'feishu' AND intent IN ('create_app','resume_app')))
  AND (state <> 'pending_completion' OR pending_setup_id IS NOT NULL)
  AND (NOT manual_verification OR intent = 'resume_app')
  AND (verification_deadline IS NULL OR (protocol_version = 0 AND pending_setup_id IS NOT NULL)));
CREATE UNIQUE INDEX connector_pending_one_attempt ON public.connector_onboarding_flows (pending_setup_id)
 WHERE pending_setup_id IS NOT NULL AND state IN ('starting','awaiting_authorization','verifying');
`

func init() {
	registerSchemaMigration("20260904160000_connector_pending_setups", upConnectorPendingSetups, nil)
}
func upConnectorPendingSetups(schema *mschema.Builder) error {
	return schema.Raw(connectorPendingSetupsSQL)
}
