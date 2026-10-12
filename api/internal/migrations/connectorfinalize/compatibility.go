package connectorfinalize

import (
	"fmt"

	"gorm.io/gorm"
)

const pendingSetupsMigrationID = "20260904160000_connector_pending_setups"

// ensurePendingSetupsCompatibility repairs a deployment that recorded later
// migration IDs without actually applying the pending-setups migration. The
// statements are deliberately additive and fixed; this is not a general
// migration runner or a caller-supplied SQL path.
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
		`INSERT INTO public.migrations (id) VALUES ('` + pendingSetupsMigrationID + `') ON CONFLICT (id) DO NOTHING`,
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
