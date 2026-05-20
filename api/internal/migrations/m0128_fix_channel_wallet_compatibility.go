package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0128_fix_channel_wallet_compatibility restores private channel wallet tables
// in environments where the billing ledger migration only applied partially.
func M0128_fix_channel_wallet_compatibility() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260306000128",
		Migrate: func(tx *gorm.DB) error {
			routesExists, err := tableExists(tx, "llm_routes")
			if err != nil {
				return err
			}
			if !routesExists {
				return nil
			}

			sqls := []string{
				`CREATE TABLE IF NOT EXISTS channel_wallets (
					channel_id UUID PRIMARY KEY REFERENCES llm_routes(id) ON DELETE CASCADE,
					organization_id UUID NOT NULL,
					balance BIGINT NOT NULL DEFAULT 0,
					status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				)`,
				`CREATE INDEX IF NOT EXISTS idx_channel_wallets_org_status
					ON channel_wallets(organization_id, status)`,
				`CREATE TABLE IF NOT EXISTS channel_wallet_transactions (
					id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					channel_id UUID NOT NULL REFERENCES channel_wallets(channel_id) ON DELETE CASCADE,
					attempt_id VARCHAR(120),
					type VARCHAR(40) NOT NULL,
					amount BIGINT NOT NULL,
					balance_before BIGINT NOT NULL,
					balance_after BIGINT NOT NULL,
					metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				)`,
				`CREATE INDEX IF NOT EXISTS idx_channel_wallet_transactions_channel
					ON channel_wallet_transactions(channel_id, created_at DESC)`,
				`CREATE INDEX IF NOT EXISTS idx_channel_wallet_transactions_attempt
					ON channel_wallet_transactions(attempt_id, created_at DESC)`,
				`INSERT INTO channel_wallets (channel_id, organization_id, balance, status, created_at, updated_at)
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
				 ON CONFLICT (channel_id) DO NOTHING`,
			}

			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
