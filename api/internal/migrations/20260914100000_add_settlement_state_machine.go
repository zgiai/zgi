package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const settlementStateMachineMigrationID = "20260914100000_add_settlement_state_machine"
const settlementStateMachineSchema = `ALTER TABLE public.billing_attempts
 ADD COLUMN IF NOT EXISTS settlement_state varchar(32) NOT NULL DEFAULT 'INIT',
 ADD COLUMN IF NOT EXISTS console_settlement_id varchar(120),
 ADD COLUMN IF NOT EXISTS console_settlement_status varchar(32),
 ADD COLUMN IF NOT EXISTS settlement_last_error_code varchar(100),
 ADD COLUMN IF NOT EXISTS settlement_last_error_message text,
 ADD COLUMN IF NOT EXISTS settlement_next_retry_at timestamptz,
 ADD COLUMN IF NOT EXISTS settlement_retry_count integer NOT NULL DEFAULT 0,
 ADD COLUMN IF NOT EXISTS settlement_version bigint NOT NULL DEFAULT 1,
 ADD COLUMN IF NOT EXISTS settlement_claimed_by varchar(120),
 ADD COLUMN IF NOT EXISTS settlement_claimed_until timestamptz;
CREATE INDEX IF NOT EXISTS idx_billing_attempts_settlement_queue ON public.billing_attempts(settlement_state, settlement_next_retry_at, updated_at);`
const settlementStateMachineDown = `DROP INDEX IF EXISTS public.idx_billing_attempts_settlement_queue;
ALTER TABLE public.billing_attempts DROP COLUMN IF EXISTS settlement_state, DROP COLUMN IF EXISTS console_settlement_id, DROP COLUMN IF EXISTS console_settlement_status, DROP COLUMN IF EXISTS settlement_last_error_code, DROP COLUMN IF EXISTS settlement_last_error_message, DROP COLUMN IF EXISTS settlement_next_retry_at, DROP COLUMN IF EXISTS settlement_retry_count, DROP COLUMN IF EXISTS settlement_version, DROP COLUMN IF EXISTS settlement_claimed_by, DROP COLUMN IF EXISTS settlement_claimed_until;`

func init() {
	registerSchemaMigration(settlementStateMachineMigrationID, upSettlementStateMachine, func(s *mschema.Builder) error { return s.Raw(settlementStateMachineDown) })
}

func upSettlementStateMachine(s *mschema.Builder) error {
	if err := s.Raw(settlementStateMachineSchema); err != nil {
		return err
	}
	return nil
}
