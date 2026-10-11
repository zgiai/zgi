package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const settlementClaimLeaseMigrationID = "20260914110000_add_settlement_claim_lease"

const settlementClaimLeaseUp = `ALTER TABLE public.billing_attempts
 ADD COLUMN IF NOT EXISTS settlement_claimed_by varchar(120),
 ADD COLUMN IF NOT EXISTS settlement_claimed_until timestamptz;
CREATE INDEX IF NOT EXISTS idx_billing_attempts_settlement_claim_lease ON public.billing_attempts(settlement_claimed_until, settlement_state, updated_at);`

const settlementClaimLeaseDown = `DROP INDEX IF EXISTS public.idx_billing_attempts_settlement_claim_lease;
ALTER TABLE public.billing_attempts DROP COLUMN IF EXISTS settlement_claimed_by, DROP COLUMN IF EXISTS settlement_claimed_until;`

func init() {
	registerSchemaMigration(settlementClaimLeaseMigrationID, func(s *mschema.Builder) error {
		return s.Raw(settlementClaimLeaseUp)
	}, func(s *mschema.Builder) error {
		return s.Raw(settlementClaimLeaseDown)
	})
}
