package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Forward-only. Credentials and existing connections remain untouched.
const messagingReliabilitySQL = `
ALTER TABLE public.connector_onboarding_flows
 ADD COLUMN candidate_connection_id uuid,
 ADD COLUMN candidate_accepted_at timestamptz,
 ADD COLUMN candidate_expires_at timestamptz,
 ADD COLUMN verification_phase varchar(24) NOT NULL DEFAULT '',
 ADD COLUMN diagnostic_trace_id varchar(36) NOT NULL DEFAULT '',
 ADD COLUMN retry_key_hash varchar(64),
	 ADD COLUMN retry_expected_revision bigint CHECK (retry_expected_revision > 0),
 DROP CONSTRAINT connector_onboarding_flows_state_check,
 DROP CONSTRAINT connector_onboarding_contract_check,
 DROP CONSTRAINT connector_onboarding_pending_check;
DO $$ DECLARE c record; BEGIN
 FOR c IN SELECT conname FROM pg_constraint WHERE conrelid='public.connector_onboarding_flows'::regclass AND contype='c' AND pg_get_constraintdef(oid) LIKE '%terminal_at%' LOOP
 EXECUTE format('ALTER TABLE public.connector_onboarding_flows DROP CONSTRAINT %I',c.conname);
 END LOOP;
END $$;
ALTER TABLE public.connector_onboarding_flows
 ADD CONSTRAINT connector_onboarding_flows_state_check CHECK (state IN ('starting','awaiting_authorization','verifying','verification_waiting','ready','cancelled','expired','failed','recovery_required','pending_completion')),
 ADD CONSTRAINT connector_onboarding_terminal_check CHECK ((state IN ('starting','awaiting_authorization','verifying','verification_waiting')) = (terminal_at IS NULL)),
 ADD CONSTRAINT connector_onboarding_candidate_check CHECK (
  (candidate_connection_id IS NULL AND candidate_accepted_at IS NULL AND candidate_expires_at IS NULL)
  OR (protocol_version=1 AND connector_id IN ('weixin','wecom') AND step IN ('verify','commit') AND candidate_connection_id IS NOT NULL AND candidate_accepted_at IS NOT NULL AND candidate_expires_at IS NOT NULL AND candidate_expires_at > candidate_accepted_at AND candidate_expires_at <= candidate_accepted_at + interval '15 minutes' AND (connection_id IS NULL OR candidate_connection_id=connection_id))),
 ADD CONSTRAINT connector_onboarding_waiting_check CHECK (state <> 'verification_waiting' OR candidate_expires_at IS NOT NULL),
 ADD CONSTRAINT connector_onboarding_pending_check CHECK (
  (pending_setup_id IS NULL OR (protocol_version=0 AND connector_id='feishu' AND intent IN ('create_app','resume_app')))
  AND (state <> 'pending_completion' OR pending_setup_id IS NOT NULL)
  AND (NOT manual_verification OR intent='resume_app')
  AND (verification_deadline IS NULL OR (protocol_version=0 AND pending_setup_id IS NOT NULL) OR (protocol_version=1 AND candidate_expires_at IS NOT NULL))),
ADD CONSTRAINT connector_onboarding_contract_check CHECK (
  (protocol_version = 0 AND intent IN ('create_app','update_app','resume_app') AND region IN ('feishu','lark')
   AND requested_region IN ('feishu','lark') AND template_digest ~ '^[a-f0-9]{64}$'
   AND poll_interval_seconds BETWEEN 5 AND 300 AND handler = '' AND step = '' AND expected_identity IS NULL)
  OR (protocol_version = 1 AND connector_id IN ('dingtalk','wecom','weixin')
   AND handler = connector_id || '-registration-v1'
   AND ((connector_id = 'weixin' AND intent IN ('login_session','reauthorize'))
     OR (connector_id IN ('dingtalk','wecom') AND intent IN ('register_bot','reauthorize')))
   AND region = '' AND requested_region = '' AND template_id = '' AND template_digest = ''
   AND poll_interval_seconds BETWEEN 1 AND 300 AND step IN ('init','begin','poll','awaiting_code','verify','commit')
   AND (step <> 'init' OR connector_id = 'dingtalk')
   AND (step <> 'awaiting_code' OR connector_id = 'weixin') AND expected_app_id IS NULL));
DROP INDEX public.connector_onboarding_one_create;
DROP INDEX public.connector_onboarding_one_update;
CREATE UNIQUE INDEX connector_onboarding_one_create ON public.connector_onboarding_flows (organization_id,owner_account_id,connector_id) WHERE state IN ('starting','awaiting_authorization','verifying','verification_waiting') AND connection_id IS NULL;
CREATE UNIQUE INDEX connector_onboarding_one_update ON public.connector_onboarding_flows (organization_id,owner_account_id,connection_id) WHERE state IN ('starting','awaiting_authorization','verifying','verification_waiting') AND connection_id IS NOT NULL;
CREATE TABLE public.connector_messaging_read_leases (
 organization_id uuid NOT NULL, owner_account_id uuid NOT NULL, connection_id uuid NOT NULL,
 connector_id varchar(128) NOT NULL CHECK (connector_id='weixin'),
 owner uuid, until timestamptz, fence bigint NOT NULL DEFAULT 0 CHECK (fence>=0), uncertain boolean NOT NULL DEFAULT false,
 credential_digest varchar(64) NOT NULL DEFAULT '' CHECK (credential_digest='' OR credential_digest ~ '^[a-f0-9]{64}$'),
 PRIMARY KEY(organization_id,owner_account_id,connection_id,connector_id), CHECK ((owner IS NULL)=(until IS NULL))
);
CREATE INDEX connector_messaging_read_leases_until ON public.connector_messaging_read_leases(until);
-- A pre-upgrade dispatched verification has no authenticated checkpoint. Its
-- candidate must be rescanned; no existing connection is changed.
UPDATE public.connector_onboarding_flows SET state='recovery_required',safe_error_code='connector_verification_uncertain',terminal_at=now(),updated_at=now(),revision=revision+1,lease_owner=NULL,lease_until=NULL
 WHERE protocol_version=1 AND connector_id IN ('weixin','wecom') AND step='verify' AND candidate_expires_at IS NULL AND state IN ('starting','awaiting_authorization','verifying');
DELETE FROM public.connector_onboarding_secrets s USING public.connector_onboarding_flows f WHERE s.flow_id=f.id AND f.state='recovery_required' AND f.protocol_version=1 AND f.connector_id IN ('weixin','wecom');
`

func init() {
	registerSchemaMigration("20260906120000_messaging_auth_reliability", upMessagingReliability, nil)
}
func upMessagingReliability(schema *mschema.Builder) error {
	return schema.Raw(messagingReliabilitySQL)
}
