package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0066_fix_transactions_schema ensures transactions table has all required columns
// This migration is needed because some environments may have an older transactions table
// created before m0013_payment_system, and CREATE TABLE doesn't overwrite existing tables.
func M0066_fix_transactions_schema() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "0066_fix_transactions_schema",
		Migrate: func(tx *gorm.DB) error {
			// Add missing columns to transactions table if they don't exist
			// All columns are nullable to avoid breaking existing data
			if err := tx.Exec(`
				-- Add missing columns from m0013_payment_system
				ALTER TABLE transactions ADD COLUMN IF NOT EXISTS group_id UUID;
				ALTER TABLE transactions ADD COLUMN IF NOT EXISTS batch_id VARCHAR(255);
				ALTER TABLE transactions ADD COLUMN IF NOT EXISTS currency_type VARCHAR(20);
				ALTER TABLE transactions ADD COLUMN IF NOT EXISTS transaction_type VARCHAR(30);
				ALTER TABLE transactions ADD COLUMN IF NOT EXISTS transaction_detail JSONB;
				ALTER TABLE transactions ADD COLUMN IF NOT EXISTS currency VARCHAR(10);
				ALTER TABLE transactions ADD COLUMN IF NOT EXISTS reference_type VARCHAR(50);
				ALTER TABLE transactions ADD COLUMN IF NOT EXISTS description VARCHAR(500);
				
				-- Ensure balance columns exist with proper precision
				ALTER TABLE transactions 
					ALTER COLUMN balance_before TYPE DECIMAL(16,4) USING balance_before::DECIMAL(16,4),
					ALTER COLUMN balance_after TYPE DECIMAL(16,4) USING balance_after::DECIMAL(16,4),
					ALTER COLUMN amount TYPE DECIMAL(16,4) USING amount::DECIMAL(16,4);
			`).Error; err != nil {
				return err
			}

			// Create indexes if they don't exist
			indexes := []string{
				`CREATE INDEX IF NOT EXISTS idx_tx_group ON transactions(group_id)`,
				`CREATE INDEX IF NOT EXISTS idx_tx_group_currency ON transactions(group_id, currency_type)`,
				`CREATE INDEX IF NOT EXISTS idx_tx_type ON transactions(transaction_type)`,
				`CREATE INDEX IF NOT EXISTS idx_tx_batch ON transactions(batch_id)`,
				`CREATE INDEX IF NOT EXISTS idx_tx_tenant ON transactions(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_tx_reference ON transactions(reference_type, reference_id)`,
			}

			for _, idx := range indexes {
				if err := tx.Exec(idx).Error; err != nil {
					// Ignore errors for existing indexes
					continue
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// We don't rollback column additions as they may contain data
			return nil
		},
	}
}
