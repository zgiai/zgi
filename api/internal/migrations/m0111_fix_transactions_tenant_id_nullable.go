package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0111_fix_transactions_schema_compat fixes schema compatibility issues in transactions table:
// 1. tenant_id: ensure uuid type and make nullable (only needed for AI consumption)
// 2. group_id: ensure NOT NULL (always required as wallet owner)
// 3. Make legacy "type" column nullable to prevent insert failures (model uses "transaction_type")
func M0111_fix_transactions_tenant_id_nullable() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260209000111",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE transactions ALTER COLUMN tenant_id TYPE uuid USING tenant_id::uuid;
				ALTER TABLE transactions ALTER COLUMN tenant_id DROP NOT NULL;
				ALTER TABLE transactions ALTER COLUMN group_id SET NOT NULL;

				DO $$
				BEGIN
					IF EXISTS (
						SELECT 1 FROM information_schema.columns
						WHERE table_name = 'transactions' AND column_name = 'type'
					) THEN
						ALTER TABLE transactions ALTER COLUMN "type" DROP NOT NULL;
					END IF;
				END $$;
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE transactions ALTER COLUMN tenant_id SET NOT NULL;
			`).Error
		},
	}
}
