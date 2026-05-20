package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0118_billing_ledger_and_channel_wallet adds attempt/entry ledgers and private channel wallet tables.
func M0118_billing_ledger_and_channel_wallet() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260302000118",
		Migrate: func(tx *gorm.DB) error {
			tableExists := func(name string) (bool, error) {
				var exists bool
				err := tx.Raw(`
					SELECT EXISTS (
						SELECT 1
						FROM information_schema.tables
						WHERE table_schema = CURRENT_SCHEMA()
						  AND table_name = ?
					)
				`, name).Scan(&exists).Error
				return exists, err
			}

			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS billing_attempts (
					attempt_id VARCHAR(120) PRIMARY KEY,
					request_id VARCHAR(100) NOT NULL,
					organization_id UUID NOT NULL,
					lane VARCHAR(20) NOT NULL,
					route_id UUID,
					provider_id UUID,
					model_id UUID,
					quota_subject_type VARCHAR(20) NOT NULL,
					quota_subject_id VARCHAR(64) NOT NULL,
					status VARCHAR(30) NOT NULL,
					invocation_result VARCHAR(20),
					error_code VARCHAR(100),
					error_message TEXT,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE INDEX IF NOT EXISTS idx_billing_attempts_request_id
					ON billing_attempts(request_id);
				CREATE INDEX IF NOT EXISTS idx_billing_attempts_org_created
					ON billing_attempts(organization_id, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_billing_attempts_status
					ON billing_attempts(status);
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS billing_attempt_entries (
					id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					attempt_id VARCHAR(120) NOT NULL REFERENCES billing_attempts(attempt_id) ON DELETE CASCADE,
					entry_type VARCHAR(20) NOT NULL,
					ledger_type VARCHAR(30) NOT NULL,
					ledger_ref_id VARCHAR(120) NOT NULL,
					reserved_amount BIGINT NOT NULL DEFAULT 0,
					actual_amount BIGINT NOT NULL DEFAULT 0,
					refunded_amount BIGINT NOT NULL DEFAULT 0,
					status VARCHAR(20) NOT NULL,
					error_code VARCHAR(100),
					error_message TEXT,
					idempotency_key VARCHAR(160),
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					CONSTRAINT uq_billing_attempt_entry UNIQUE (attempt_id, entry_type, ledger_type)
				);

				CREATE INDEX IF NOT EXISTS idx_billing_attempt_entries_attempt
					ON billing_attempt_entries(attempt_id);
				CREATE INDEX IF NOT EXISTS idx_billing_attempt_entries_status
					ON billing_attempt_entries(status);
			`).Error; err != nil {
				return err
			}

			routesExists, err := tableExists("llm_routes")
			if err != nil {
				return err
			}
			if !routesExists {
				return nil
			}

			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS channel_wallets (
					channel_id UUID PRIMARY KEY REFERENCES llm_routes(id) ON DELETE CASCADE,
					organization_id UUID NOT NULL,
					balance BIGINT NOT NULL DEFAULT 0,
					status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE INDEX IF NOT EXISTS idx_channel_wallets_org_status
					ON channel_wallets(organization_id, status);
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS channel_wallet_transactions (
					id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					channel_id UUID NOT NULL REFERENCES channel_wallets(channel_id) ON DELETE CASCADE,
					attempt_id VARCHAR(120),
					type VARCHAR(40) NOT NULL,
					amount BIGINT NOT NULL,
					balance_before BIGINT NOT NULL,
					balance_after BIGINT NOT NULL,
					metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE INDEX IF NOT EXISTS idx_channel_wallet_transactions_channel
					ON channel_wallet_transactions(channel_id, created_at DESC);
				CREATE INDEX IF NOT EXISTS idx_channel_wallet_transactions_attempt
					ON channel_wallet_transactions(attempt_id, created_at DESC);
			`).Error; err != nil {
				return err
			}

			// Backfill private channel wallet from llm_routes.balance snapshot.
			return tx.Exec(`
				INSERT INTO channel_wallets (channel_id, organization_id, balance, status, created_at, updated_at)
				SELECT
					r.id,
					r.organization_id,
					COALESCE(ROUND(r.balance)::bigint, 0) AS balance,
					CASE WHEN COALESCE(ROUND(r.balance)::bigint, 0) < 0 THEN 'DEBT' ELSE 'ACTIVE' END AS status,
					NOW(),
					NOW()
				FROM llm_routes r
				WHERE r.deleted_at IS NULL
				  AND r.type = 'PRIVATE'
				ON CONFLICT (channel_id) DO NOTHING;
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				DROP TABLE IF EXISTS channel_wallet_transactions CASCADE;
				DROP TABLE IF EXISTS channel_wallets CASCADE;
				DROP TABLE IF EXISTS billing_attempt_entries CASCADE;
				DROP TABLE IF EXISTS billing_attempts CASCADE;
			`).Error
		},
	}
}
