package connectorfinalize

import (
	"fmt"

	"gorm.io/gorm"
)

// RepairPendingSetupsCompatibility repairs a deployment that recorded the
// pending-setups migration without actually applying its schema. It is a
// no-op when the migration marker is absent so the normal migration runner can
// apply the real migration in order.
func RepairPendingSetupsCompatibility(tx *gorm.DB) error {
	recorded, err := pendingSetupsMigrationRecorded(tx)
	if err != nil {
		return err
	}
	if !recorded {
		return nil
	}
	return ensurePendingSetupsCompatibility(tx)
}

// EnsurePendingSetupsCompatibility is used by the explicit discard command.
// Unlike the normal runner, discard may start at the checkpoint, so it can
// apply the complete additive schema and record the marker when it is absent.
func EnsurePendingSetupsCompatibility(tx *gorm.DB) error {
	if !tx.Migrator().HasTable("connector_onboarding_flows") {
		return nil
	}
	if err := ensurePendingSetupsCompatibility(tx); err != nil {
		return err
	}
	recorded, err := pendingSetupsMigrationRecorded(tx)
	if err != nil {
		return err
	}
	if !recorded {
		if err := tx.Table("migrations").Create(map[string]any{"id": pendingSetupsMigrationID}).Error; err != nil {
			return fmt.Errorf("record Connector pending-setups compatibility migration: %w", err)
		}
	}
	return nil
}

func pendingSetupsMigrationRecorded(tx *gorm.DB) (bool, error) {
	if !tx.Migrator().HasTable("migrations") {
		return false, nil
	}
	var count int64
	if err := tx.Table("migrations").Where("id = ?", pendingSetupsMigrationID).Count(&count).Error; err != nil {
		return false, fmt.Errorf("check Connector pending-setups migration marker: %w", err)
	}
	return count > 0, nil
}

// ensurePendingSetupsCompatibility contains the full shape of the published
// migration, including the onboarding constraints. The statements are fixed
// and additive; this is not a general migration runner or caller SQL path.
func ensurePendingSetupsCompatibility(tx *gorm.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS public.connector_pending_setups (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL, owner_account_id uuid NOT NULL,
 connector_id varchar(128) NOT NULL CHECK (connector_id = 'feishu'),
 region varchar(8) NOT NULL CHECK (region IN ('feishu','lark')),
 app_id varchar(128) NOT NULL CHECK (app_id ~ '^[A-Za-z0-9_-]{1,128}$'),
 source_flow_id uuid NOT NULL UNIQUE, template_id varchar(128) NOT NULL,
 template_digest varchar(64) NOT NULL CHECK (template_digest ~ '^[a-f0-9]{64}$'),
 accepted_definition_digest varchar(64) NOT NULL CHECK (accepted_definition_digest ~ '^[a-f0-9]{64}$'),
 state varchar(32) NOT NULL CHECK (state IN ('verification_required','awaiting_application','completed','cancelled','expired')),
 safe_reason_code varchar(128), last_verification_at timestamptz,
 revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0), result_connection_id uuid,
 resume_expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL, terminal_at timestamptz,
 UNIQUE (id,organization_id,owner_account_id),
 CHECK ((state = 'completed') = (result_connection_id IS NOT NULL)),
 CHECK ((state IN ('verification_required','awaiting_application')) = (terminal_at IS NULL)),
 CHECK (resume_expires_at > created_at)
)`,
		`CREATE INDEX IF NOT EXISTS connector_pending_owner ON public.connector_pending_setups (organization_id,owner_account_id,created_at,id)`,
		`CREATE INDEX IF NOT EXISTS connector_pending_expiry ON public.connector_pending_setups (resume_expires_at) WHERE terminal_at IS NULL`,
		`ALTER TABLE public.connector_onboarding_flows ADD COLUMN IF NOT EXISTS pending_setup_id uuid`,
		`ALTER TABLE public.connector_onboarding_flows ADD COLUMN IF NOT EXISTS expected_pending_revision bigint CHECK (expected_pending_revision > 0)`,
		`ALTER TABLE public.connector_onboarding_flows ADD COLUMN IF NOT EXISTS manual_verification boolean NOT NULL DEFAULT false`,
		`ALTER TABLE public.connector_onboarding_flows ADD COLUMN IF NOT EXISTS verification_deadline timestamptz`,
		`DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='connector_onboarding_pending_fk') THEN
  ALTER TABLE public.connector_onboarding_flows ADD CONSTRAINT connector_onboarding_pending_fk
   FOREIGN KEY (pending_setup_id,organization_id,owner_account_id)
   REFERENCES public.connector_pending_setups (id,organization_id,owner_account_id);
 END IF;
END $$`,
		`CREATE UNIQUE INDEX IF NOT EXISTS connector_pending_one_attempt ON public.connector_onboarding_flows (pending_setup_id) WHERE pending_setup_id IS NOT NULL AND state IN ('starting','awaiting_authorization','verifying')`,
		`DO $$ DECLARE definition text; BEGIN
 ALTER TABLE public.connector_onboarding_flows
  DROP CONSTRAINT IF EXISTS connector_onboarding_flows_check,
  DROP CONSTRAINT IF EXISTS connector_onboarding_flows_check1,
  DROP CONSTRAINT IF EXISTS connector_onboarding_flows_intent_check,
  DROP CONSTRAINT IF EXISTS connector_onboarding_flows_region_check,
  DROP CONSTRAINT IF EXISTS connector_onboarding_flows_requested_region_check,
  DROP CONSTRAINT IF EXISTS connector_onboarding_flows_poll_interval_seconds_check;
 SELECT pg_get_constraintdef(oid) INTO definition FROM pg_constraint WHERE conrelid='public.connector_onboarding_flows'::regclass AND conname='connector_onboarding_flows_state_check';
 IF definition IS NOT NULL AND position('pending_completion' in definition) = 0 THEN
  ALTER TABLE public.connector_onboarding_flows DROP CONSTRAINT connector_onboarding_flows_state_check;
 END IF;
 SELECT pg_get_constraintdef(oid) INTO definition FROM pg_constraint WHERE conrelid='public.connector_onboarding_flows'::regclass AND conname='connector_onboarding_contract_check';
 IF definition IS NOT NULL AND position('resume_app' in definition) = 0 THEN
  ALTER TABLE public.connector_onboarding_flows DROP CONSTRAINT connector_onboarding_contract_check;
 END IF;
 SELECT pg_get_constraintdef(oid) INTO definition FROM pg_constraint WHERE conrelid='public.connector_onboarding_flows'::regclass AND conname='connector_onboarding_identity_check';
 IF definition IS NOT NULL AND position('resume_app' in definition) = 0 THEN
  ALTER TABLE public.connector_onboarding_flows DROP CONSTRAINT connector_onboarding_identity_check;
 END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.connector_onboarding_flows'::regclass AND conname='connector_onboarding_flows_state_check') THEN
  ALTER TABLE public.connector_onboarding_flows ADD CONSTRAINT connector_onboarding_flows_state_check CHECK (state IN ('starting','awaiting_authorization','verifying','ready','cancelled','expired','failed','recovery_required','pending_completion'));
 END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.connector_onboarding_flows'::regclass AND conname='connector_onboarding_contract_check') THEN
  ALTER TABLE public.connector_onboarding_flows ADD CONSTRAINT connector_onboarding_contract_check CHECK (
   (protocol_version = 0 AND intent IN ('create_app','update_app','resume_app') AND region IN ('feishu','lark') AND requested_region IN ('feishu','lark') AND template_digest ~ '^[a-f0-9]{64}$' AND poll_interval_seconds BETWEEN 5 AND 300 AND handler = '' AND step = '' AND expected_identity IS NULL)
   OR (protocol_version = 1 AND connector_id IN ('dingtalk','wecom','weixin')));
 END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.connector_onboarding_flows'::regclass AND conname='connector_onboarding_identity_check') THEN
  ALTER TABLE public.connector_onboarding_flows ADD CONSTRAINT connector_onboarding_identity_check CHECK (
   (intent IN ('create_app','register_bot','login_session') AND connection_id IS NULL)
   OR (intent IN ('update_app','reauthorize') AND connection_id IS NOT NULL)
   OR (intent = 'resume_app' AND pending_setup_id IS NOT NULL));
 END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.connector_onboarding_flows'::regclass AND conname='connector_onboarding_pending_check') THEN
  ALTER TABLE public.connector_onboarding_flows ADD CONSTRAINT connector_onboarding_pending_check CHECK (
   (pending_setup_id IS NULL OR (protocol_version = 0 AND connector_id = 'feishu' AND intent IN ('create_app','resume_app')))
   AND (state <> 'pending_completion' OR pending_setup_id IS NOT NULL)
   AND (NOT manual_verification OR intent = 'resume_app')
   AND (verification_deadline IS NULL OR (protocol_version = 0 AND pending_setup_id IS NOT NULL)));
 END IF;
END $$`,
	}
	for _, statement := range statements {
		if err := tx.Exec(statement).Error; err != nil {
			return fmt.Errorf("ensure Connector pending-setups compatibility: %w", err)
		}
	}
	return nil
}

func ensureCheckpointApplied(tx *gorm.DB) error {
	var count int64
	if err := tx.Table("migrations").Where("id = ?", CheckpointID).Count(&count).Error; err != nil {
		return fmt.Errorf("check Connector checkpoint: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("Connector checkpoint %s is not applied", CheckpointID)
	}
	return nil
}
